package main

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"crypto/rand"
	"encoding/hex"
)

func generateID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func jsonResponse(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func jsonError(w http.ResponseWriter, status int, msg string) {
	jsonResponse(w, status, map[string]string{"error": msg})
}

// handleUpload handles POST /api/upload
func handleUpload(store *FileStore, originalsDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		// 100MB max
		r.ParseMultipartForm(100 << 20)

		files := r.MultipartForm.File["files"]
		if len(files) == 0 {
			jsonError(w, http.StatusBadRequest, "no files uploaded")
			return
		}

		var records []*FileRecord

		for _, fh := range files {
			if !strings.HasSuffix(strings.ToLower(fh.Filename), ".glb") {
				continue
			}

			src, err := fh.Open()
			if err != nil {
				jsonError(w, http.StatusInternalServerError, "failed to read uploaded file")
				return
			}

			id := generateID()
			destPath := filepath.Join(originalsDir, id+".glb")
			dst, err := os.Create(destPath)
			if err != nil {
				src.Close()
				jsonError(w, http.StatusInternalServerError, "failed to save file")
				return
			}

			written, err := io.Copy(dst, src)
			src.Close()
			dst.Close()
			if err != nil {
				jsonError(w, http.StatusInternalServerError, "failed to write file")
				return
			}

			record := &FileRecord{
				ID:           id,
				Filename:     fh.Filename,
				OriginalSize: written,
				Status:       StatusPending,
			}
			store.Add(record)
			records = append(records, record)
		}

		if len(records) == 0 {
			jsonError(w, http.StatusBadRequest, "no valid .glb files uploaded")
			return
		}

		jsonResponse(w, http.StatusOK, records)
	}
}

// handleFiles handles GET /api/files
func handleFiles(store *FileStore) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		jsonResponse(w, http.StatusOK, store.All())
	}
}

// handleProcess handles POST /api/process/:id
func handleProcess(store *FileStore, originalsDir, outputsDir string, queue chan<- processJob) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		id := strings.TrimPrefix(r.URL.Path, "/api/process/")
		if id == "" {
			jsonError(w, http.StatusBadRequest, "missing file id")
			return
		}

		if _, ok := store.Get(id); !ok {
			jsonError(w, http.StatusNotFound, "file not found")
			return
		}

		var settings Settings
		if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid settings: "+err.Error())
			return
		}

		store.Update(id, func(r *FileRecord) {
			r.Status = StatusProcessing
		})

		inputPath := filepath.Join(originalsDir, id+".glb")
		outputPath := filepath.Join(outputsDir, id+".glb")

		args := BuildCommand(inputPath, outputPath, settings)
		cmdStr := FormatCommand(args)

		output, err := RunGltfpack(args)
		if err != nil {
			store.Update(id, func(r *FileRecord) {
				r.Status = StatusError
				r.Error = output
				r.Command = cmdStr
				r.SettingsUsed = &settings
			})
			result, _ := store.Get(id)
			jsonResponse(w, http.StatusOK, result)
			return
		}

		var outputSize int64
		if info, err := os.Stat(outputPath); err == nil {
			outputSize = info.Size()
		}

		store.Update(id, func(r *FileRecord) {
			r.Status = StatusDone
			r.OutputSize = outputSize
			r.Command = cmdStr
			r.SettingsUsed = &settings
			r.Error = ""
		})

		result, _ := store.Get(id)
		jsonResponse(w, http.StatusOK, result)
	}
}

// handleProcessAll handles POST /api/process-all
func handleProcessAll(store *FileStore, originalsDir, outputsDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		var settings Settings
		if err := json.NewDecoder(r.Body).Decode(&settings); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid settings: "+err.Error())
			return
		}

		allFiles := store.All()
		var results []*FileRecord

		for _, f := range allFiles {
			if f.Status != StatusPending {
				continue
			}

			store.Update(f.ID, func(r *FileRecord) {
				r.Status = StatusProcessing
			})

			inputPath := filepath.Join(originalsDir, f.ID+".glb")
			outputPath := filepath.Join(outputsDir, f.ID+".glb")

			args := BuildCommand(inputPath, outputPath, settings)
			cmdStr := FormatCommand(args)

			output, err := RunGltfpack(args)
			if err != nil {
				store.Update(f.ID, func(r *FileRecord) {
					r.Status = StatusError
					r.Error = output
					r.Command = cmdStr
					r.SettingsUsed = &settings
				})
			} else {
				var outputSize int64
				if info, statErr := os.Stat(outputPath); statErr == nil {
					outputSize = info.Size()
				}
				store.Update(f.ID, func(r *FileRecord) {
					r.Status = StatusDone
					r.OutputSize = outputSize
					r.Command = cmdStr
					r.SettingsUsed = &settings
					r.Error = ""
				})
			}

			updated, _ := store.Get(f.ID)
			results = append(results, updated)
		}

		jsonResponse(w, http.StatusOK, results)
	}
}

// handleDownload handles GET /api/download/:id
func handleDownload(store *FileStore, outputsDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		id := strings.TrimPrefix(r.URL.Path, "/api/download/")
		record, ok := store.Get(id)
		if !ok {
			jsonError(w, http.StatusNotFound, "file not found")
			return
		}
		if record.Status != StatusDone {
			jsonError(w, http.StatusBadRequest, "file has not been processed")
			return
		}

		outputPath := filepath.Join(outputsDir, id+".glb")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="opt_%s"`, record.Filename))
		w.Header().Set("Content-Type", "application/octet-stream")
		http.ServeFile(w, r, outputPath)
	}
}

// handleDownloadAll handles GET /api/download-all
func handleDownloadAll(store *FileStore, outputsDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		w.Header().Set("Content-Type", "application/zip")
		w.Header().Set("Content-Disposition", `attachment; filename="optimized_models.zip"`)

		zw := zip.NewWriter(w)
		defer zw.Close()

		for _, f := range store.All() {
			if f.Status != StatusDone {
				continue
			}
			outputPath := filepath.Join(outputsDir, f.ID+".glb")
			file, err := os.Open(outputPath)
			if err != nil {
				continue
			}
			writer, err := zw.Create("opt_" + f.Filename)
			if err != nil {
				file.Close()
				continue
			}
			io.Copy(writer, file)
			file.Close()
		}
	}
}

// handlePreview handles GET /api/preview/:id?version=original|optimized|lod0|lod1|lod2|lod3|billboard
func handlePreview(store *FileStore, originalsDir, outputsDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		id := strings.TrimPrefix(r.URL.Path, "/api/preview/")
		_, ok := store.Get(id)
		if !ok {
			jsonError(w, http.StatusNotFound, "file not found")
			return
		}

		version := r.URL.Query().Get("version")
		var filePath string
		switch version {
		case "optimized":
			filePath = filepath.Join(outputsDir, id+".glb")
		case "lod0", "lod1", "lod2", "lod3":
			filePath = filepath.Join(outputsDir, id+"_"+version+".glb")
		case "billboard":
			filePath = filepath.Join(outputsDir, id+"_billboard.glb")
		default:
			filePath = filepath.Join(originalsDir, id+".glb")
		}

		if _, err := os.Stat(filePath); os.IsNotExist(err) {
			jsonError(w, http.StatusNotFound, "file not found on disk")
			return
		}

		w.Header().Set("Content-Type", "model/gltf-binary")
		w.Header().Set("Cache-Control", "no-store")
		http.ServeFile(w, r, filePath)
	}
}

// LOD level definitions: progressively more aggressive
var lodConfigs = []struct {
	Label          string
	Simplification float64
	Aggressive     bool
	Permissive     bool
}{
	{"lod0", 0.5, false, false},
	{"lod1", 0.2, true, false},
	{"lod2", 0.05, true, true},
	{"lod3", 0.01, true, true},
}

// handleGenerateLODs handles POST /api/generate-lods/:id
func handleGenerateLODs(store *FileStore, originalsDir, outputsDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		id := strings.TrimPrefix(r.URL.Path, "/api/generate-lods/")
		if _, ok := store.Get(id); !ok {
			jsonError(w, http.StatusNotFound, "file not found")
			return
		}

		// Accept base settings for texture/compression options
		var baseSettings Settings
		if err := json.NewDecoder(r.Body).Decode(&baseSettings); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid settings: "+err.Error())
			return
		}

		inputPath := filepath.Join(originalsDir, id+".glb")
		var lods []LODLevel

		for i, cfg := range lodConfigs {
			outputPath := filepath.Join(outputsDir, fmt.Sprintf("%s_%s.glb", id, cfg.Label))

			s := baseSettings
			s.Simplification = cfg.Simplification
			s.AggressiveSimplify = cfg.Aggressive
			s.PermissiveSimplify = cfg.Permissive

			args := BuildCommand(inputPath, outputPath, s)
			cmdStr := FormatCommand(args)

			output, err := RunGltfpack(args)
			lod := LODLevel{Level: i, Command: cmdStr}

			if err != nil {
				lod.Error = output
			} else if info, statErr := os.Stat(outputPath); statErr == nil {
				lod.Size = info.Size()
			}

			lods = append(lods, lod)
		}

		store.Update(id, func(r *FileRecord) {
			r.LODs = lods
		})

		record, _ := store.Get(id)
		jsonResponse(w, http.StatusOK, record)
	}
}

// handleUploadBillboard handles POST /api/upload-billboard/:id
func handleUploadBillboard(store *FileStore, outputsDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		id := strings.TrimPrefix(r.URL.Path, "/api/upload-billboard/")
		if _, ok := store.Get(id); !ok {
			jsonError(w, http.StatusNotFound, "file not found")
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20)) // 10MB max
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to read body")
			return
		}

		outputPath := filepath.Join(outputsDir, id+"_billboard.glb")
		if err := os.WriteFile(outputPath, body, 0644); err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to save billboard")
			return
		}

		var billboardSize int64
		if info, err := os.Stat(outputPath); err == nil {
			billboardSize = info.Size()
		}

		store.Update(id, func(r *FileRecord) {
			r.HasBillboard = true
		})

		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"status": "ok",
			"size":   billboardSize,
		})
	}
}

// handleDeleteFile handles DELETE /api/files/:id
func handleDeleteFile(store *FileStore, originalsDir, outputsDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		id := strings.TrimPrefix(r.URL.Path, "/api/files/")
		if _, ok := store.Get(id); !ok {
			jsonError(w, http.StatusNotFound, "file not found")
			return
		}

		os.Remove(filepath.Join(originalsDir, id+".glb"))
		os.Remove(filepath.Join(outputsDir, id+".glb"))
		os.Remove(filepath.Join(outputsDir, id+"_lod0.glb"))
		os.Remove(filepath.Join(outputsDir, id+"_lod1.glb"))
		os.Remove(filepath.Join(outputsDir, id+"_lod2.glb"))
		os.Remove(filepath.Join(outputsDir, id+"_lod3.glb"))
		os.Remove(filepath.Join(outputsDir, id+"_billboard.glb"))
		store.Delete(id)

		jsonResponse(w, http.StatusOK, map[string]string{"status": "deleted"})
	}
}

// processJob is used for the sequential processing queue (reserved for future async use).
type processJob struct {
	ID       string
	Settings Settings
}

// handleStatus handles GET /api/status — returns server capabilities.
func handleStatus(blender BlenderInfo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"blender": blender,
		})
	}
}

// handleGenerateBlenderLODs handles POST /api/generate-blender-lods/:id
func handleGenerateBlenderLODs(store *FileStore, originalsDir, outputsDir string, blender BlenderInfo, scriptPath string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		if !blender.Available {
			jsonError(w, http.StatusServiceUnavailable, "Blender not available")
			return
		}

		id := strings.TrimPrefix(r.URL.Path, "/api/generate-blender-lods/")
		if _, ok := store.Get(id); !ok {
			jsonError(w, http.StatusNotFound, "file not found")
			return
		}

		inputPath := filepath.Join(originalsDir, id+".glb")
		var lods []LODLevel

		for i, cfg := range DefaultBlenderLODs {
			outputPath := filepath.Join(outputsDir, fmt.Sprintf("%s_lod%d.glb", id, i))

			output, err := RunBlenderLOD(blender, scriptPath, inputPath, outputPath, cfg)
			lod := LODLevel{
				Level:   i,
				Command: fmt.Sprintf("blender -b --python remesh_lod.py -- --mode %s --decimate-ratio %.4f", cfg.Mode, cfg.DecimateRatio),
			}

			if err != nil {
				lod.Error = output
			} else if info, statErr := os.Stat(outputPath); statErr == nil {
				lod.Size = info.Size()
			}

			lods = append(lods, lod)
		}

		store.Update(id, func(r *FileRecord) {
			r.LODs = lods
		})

		record, _ := store.Get(id)
		jsonResponse(w, http.StatusOK, record)
	}
}
