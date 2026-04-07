package main

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"strconv"

	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"path"
	"time"
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

// handleUpload handles POST /api/upload. Each successfully written
// file is auto-classified (T-004-02): the result lands in the asset's
// settings file and emits a "classification" analytics event. Any
// classifier failure is logged and swallowed; the upload still
// succeeds with shape_category="unknown".
func handleUpload(store *FileStore, originalsDir, settingsDir string, logger *AnalyticsLogger) http.HandlerFunc {
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
			autoClassify(id, originalsDir, settingsDir, store, logger)
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
		case "volumetric":
			filePath = filepath.Join(outputsDir, id+"_volumetric.glb")
		case "vlod0", "vlod1", "vlod2", "vlod3":
			filePath = filepath.Join(outputsDir, id+"_"+version+".glb")
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

// handleUploadVolumetric handles POST /api/upload-volumetric/:id
func handleUploadVolumetric(store *FileStore, outputsDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		id := strings.TrimPrefix(r.URL.Path, "/api/upload-volumetric/")
		if _, ok := store.Get(id); !ok {
			jsonError(w, http.StatusNotFound, "file not found")
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20)) // 10MB max
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to read body")
			return
		}

		outputPath := filepath.Join(outputsDir, id+"_volumetric.glb")
		if err := os.WriteFile(outputPath, body, 0644); err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to save volumetric")
			return
		}

		var volumetricSize int64
		if info, err := os.Stat(outputPath); err == nil {
			volumetricSize = info.Size()
		}

		store.Update(id, func(r *FileRecord) {
			r.HasVolumetric = true
		})

		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"status": "ok",
			"size":   volumetricSize,
		})
	}
}

// handleUploadReference handles POST /api/upload-reference/:id
// Stores a reference image (PNG/JPEG) used for environment-map calibration.
func handleUploadReference(store *FileStore, outputsDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		id := strings.TrimPrefix(r.URL.Path, "/api/upload-reference/")
		if _, ok := store.Get(id); !ok {
			jsonError(w, http.StatusNotFound, "file not found")
			return
		}

		// Multipart upload — single file under "image"
		r.ParseMultipartForm(20 << 20) // 20MB
		fh, _, err := r.FormFile("image")
		if err != nil {
			jsonError(w, http.StatusBadRequest, "missing image file")
			return
		}
		defer fh.Close()

		body, err := io.ReadAll(io.LimitReader(fh, 20<<20))
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to read image")
			return
		}

		// Detect extension from first bytes (PNG vs JPEG)
		ext := ".png"
		if len(body) >= 3 && body[0] == 0xff && body[1] == 0xd8 && body[2] == 0xff {
			ext = ".jpg"
		}
		outputPath := filepath.Join(outputsDir, id+"_reference"+ext)
		if err := os.WriteFile(outputPath, body, 0644); err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to save reference image")
			return
		}

		store.Update(id, func(r *FileRecord) {
			r.HasReference = true
			r.ReferenceExt = ext
		})

		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"status": "ok",
			"size":   len(body),
		})
	}
}

// handleReferenceImage handles GET /api/reference/:id — serves the stored reference image
func handleReferenceImage(store *FileStore, outputsDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/reference/")
		rec, ok := store.Get(id)
		if !ok || !rec.HasReference {
			http.NotFound(w, r)
			return
		}
		ext := rec.ReferenceExt
		if ext == "" {
			ext = ".png"
		}
		path := filepath.Join(outputsDir, id+"_reference"+ext)
		f, err := os.Open(path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		if ext == ".jpg" {
			w.Header().Set("Content-Type", "image/jpeg")
		} else {
			w.Header().Set("Content-Type", "image/png")
		}
		w.Header().Set("Cache-Control", "no-cache")
		io.Copy(w, f)
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
		os.Remove(filepath.Join(outputsDir, id+"_volumetric.glb"))
		os.Remove(filepath.Join(outputsDir, id+"_vlod0.glb"))
		os.Remove(filepath.Join(outputsDir, id+"_vlod1.glb"))
		os.Remove(filepath.Join(outputsDir, id+"_vlod2.glb"))
		os.Remove(filepath.Join(outputsDir, id+"_vlod3.glb"))
		store.Delete(id)

		jsonResponse(w, http.StatusOK, map[string]string{"status": "deleted"})
	}
}

// processJob is used for the sequential processing queue (reserved for future async use).
type processJob struct {
	ID       string
	Settings Settings
}

// handleSettings handles GET and PUT /api/settings/:id.
//
//	GET  → returns the asset's saved settings, or DefaultSettings() if none
//	       are persisted on disk yet.
//	PUT  → decodes the request body into AssetSettings, validates it, writes
//	       it atomically to disk, marks the file record as having saved
//	       settings, and returns the canonical settings.
func handleSettings(store *FileStore, settingsDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/settings/")
		if id == "" {
			jsonError(w, http.StatusBadRequest, "missing file id")
			return
		}
		if _, ok := store.Get(id); !ok {
			jsonError(w, http.StatusNotFound, "file not found")
			return
		}

		switch r.Method {
		case http.MethodGet:
			s, err := LoadSettings(id, settingsDir)
			if err != nil {
				jsonError(w, http.StatusInternalServerError, "failed to load settings: "+err.Error())
				return
			}
			jsonResponse(w, http.StatusOK, s)

		case http.MethodPut:
			var s AssetSettings
			if err := json.NewDecoder(r.Body).Decode(&s); err != nil {
				jsonError(w, http.StatusBadRequest, "invalid settings: "+err.Error())
				return
			}
			if err := s.Validate(); err != nil {
				jsonError(w, http.StatusBadRequest, err.Error())
				return
			}
			if err := SaveSettings(id, settingsDir, &s); err != nil {
				jsonError(w, http.StatusInternalServerError, "failed to save settings: "+err.Error())
				return
			}
			dirty := SettingsDifferFromDefaults(&s)
			store.Update(id, func(r *FileRecord) {
				r.HasSavedSettings = true
				r.SettingsDirty = dirty
			})
			jsonResponse(w, http.StatusOK, &s)

		default:
			jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

// applyShapeStrategyToSettings stamps the strategy router's defaults
// onto the asset's settings, but only for fields that are still at
// their factory default. The user-override semantics are: if the
// user has tuned a strategy-shaped field away from defaults, the
// classification leaves their value alone; otherwise the strategy
// fills it in. Slice fields carrying the SliceAxisNA sentinel
// (hard-surface routes to the parametric pipeline) are skipped
// entirely. Added in T-004-03.
func applyShapeStrategyToSettings(s *AssetSettings, strategy ShapeStrategy) {
	d := DefaultSettings()
	if strategy.SliceAxis != SliceAxisNA && strategy.SliceAxis != "" {
		if s.SliceAxis == d.SliceAxis {
			s.SliceAxis = strategy.SliceAxis
		}
	}
	if strategy.SliceDistributionMode != SliceAxisNA && strategy.SliceDistributionMode != "" {
		if s.SliceDistributionMode == d.SliceDistributionMode {
			s.SliceDistributionMode = strategy.SliceDistributionMode
		}
	}
	if strategy.SliceCount > 0 {
		if s.VolumetricLayers == d.VolumetricLayers {
			s.VolumetricLayers = strategy.SliceCount
		}
	}
}

// applyClassificationToSettings merges a classifier result into the
// asset's persisted settings. Loads the current settings (or defaults
// if none exist), overwrites the two shape fields, stamps the
// strategy router's defaults onto any still-default strategy-shaped
// fields (T-004-03), validates, and writes atomically. Returns the
// new settings on success.
func applyClassificationToSettings(id, settingsDir string, result *ClassificationResult) (*AssetSettings, error) {
	s, err := LoadSettings(id, settingsDir)
	if err != nil {
		return nil, fmt.Errorf("load settings: %w", err)
	}
	s.ShapeCategory = result.Category
	s.ShapeConfidence = result.Confidence
	applyShapeStrategyToSettings(s, getStrategyForCategory(result.Category))
	if err := s.Validate(); err != nil {
		return nil, fmt.Errorf("validate settings: %w", err)
	}
	if err := SaveSettings(id, settingsDir, s); err != nil {
		return nil, fmt.Errorf("save settings: %w", err)
	}
	return s, nil
}

// emitClassificationEvent appends a "classification" analytics event
// for the asset. Mirrors handleAccept's pattern: failure to emit logs
// to stderr but does not fail the caller — the persisted settings are
// the load-bearing artifact, the event is a nice-to-have for the
// export pipeline.
func emitClassificationEvent(logger *AnalyticsLogger, id string, result *ClassificationResult) {
	sessionID, _, err := logger.LookupOrStartSession(id)
	if err != nil || sessionID == "" {
		if err != nil {
			fmt.Fprintf(os.Stderr, "classify: lookup session for %s: %v\n", id, err)
		}
		return
	}
	ev := Event{
		SchemaVersion: AnalyticsSchemaVersion,
		EventType:     "classification",
		Timestamp:     time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:     sessionID,
		AssetID:       id,
		Payload: map[string]interface{}{
			"category":   result.Category,
			"confidence": result.Confidence,
			"features":   result.Features,
		},
	}
	if err := logger.AppendEvent(sessionID, ev); err != nil {
		fmt.Fprintf(os.Stderr, "classify: append event for %s: %v\n", id, err)
	}
}

// candidate is the typed projection of one entry from the classifier's
// features.candidates ranking. Used in the /api/classify response so
// the comparison-UI modal can render top-N candidate strategies. The
// score is whatever the Python side computed (softmax in [0,1] today).
type candidate struct {
	Category string  `json:"category"`
	Score    float64 `json:"score"`
}

// extractCandidates pulls the typed candidate list out of the opaque
// classifier features map. Returns nil when the key is missing or any
// entry is malformed — the response then carries `null`, and the
// frontend treats that as "no ranking available, hide the modal". The
// per-asset settings stamping path is unaffected by a nil return.
func extractCandidates(features map[string]interface{}) []candidate {
	raw, ok := features["candidates"]
	if !ok {
		return nil
	}
	list, ok := raw.([]interface{})
	if !ok {
		return nil
	}
	out := make([]candidate, 0, len(list))
	for _, item := range list {
		m, ok := item.(map[string]interface{})
		if !ok {
			return nil
		}
		cat, _ := m["category"].(string)
		score, _ := m["score"].(float64)
		if cat == "" {
			return nil
		}
		out = append(out, candidate{Category: cat, Score: score})
	}
	return out
}

// emitClassificationOverrideEvent appends a "classification_override"
// analytics event for the asset. Mirrors emitClassificationEvent in
// best-effort posture — the persisted settings are the load-bearing
// artifact, the event is the training-data signal. T-004-04. The
// payload is the canonical training pair: classifier-original
// (category, confidence), the full candidate list the user picked
// from, the chosen category, and the asset features for the model.
func emitClassificationOverrideEvent(logger *AnalyticsLogger, id, originalCategory string, originalConfidence float64, result *ClassificationResult) {
	sessionID, _, err := logger.LookupOrStartSession(id)
	if err != nil || sessionID == "" {
		if err != nil {
			fmt.Fprintf(os.Stderr, "classification_override: lookup session for %s: %v\n", id, err)
		}
		return
	}
	ev := Event{
		SchemaVersion: AnalyticsSchemaVersion,
		EventType:     "classification_override",
		Timestamp:     time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:     sessionID,
		AssetID:       id,
		Payload: map[string]interface{}{
			"original_category":   originalCategory,
			"original_confidence": originalConfidence,
			"candidates":          result.Features["candidates"],
			"chosen_category":     result.Category,
			"features":            result.Features,
		},
	}
	if err := logger.AppendEvent(sessionID, ev); err != nil {
		fmt.Fprintf(os.Stderr, "classification_override: append event for %s: %v\n", id, err)
	}
}

// emitStrategySelectedEvent appends a "strategy_selected" analytics
// event for the asset. Mirrors emitClassificationEvent: failure to
// emit logs to stderr but does not fail the caller. Added in
// T-004-03; the event captures which strategy the router picked for
// a given classification so the export pipeline can correlate user
// outcomes with router decisions.
func emitStrategySelectedEvent(logger *AnalyticsLogger, id string, strategy ShapeStrategy) {
	sessionID, _, err := logger.LookupOrStartSession(id)
	if err != nil || sessionID == "" {
		if err != nil {
			fmt.Fprintf(os.Stderr, "strategy_selected: lookup session for %s: %v\n", id, err)
		}
		return
	}
	ev := Event{
		SchemaVersion: AnalyticsSchemaVersion,
		EventType:     "strategy_selected",
		Timestamp:     time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:     sessionID,
		AssetID:       id,
		Payload: map[string]interface{}{
			"category": strategy.Category,
			"strategy": strategy,
		},
	}
	if err := logger.AppendEvent(sessionID, ev); err != nil {
		fmt.Fprintf(os.Stderr, "strategy_selected: append event for %s: %v\n", id, err)
	}
}

// autoClassify is the upload-time best-effort hook. Any failure
// (missing python3, classifier crash, settings write error) is logged
// and swallowed — a classifier outage must never block an upload.
func autoClassify(id, originalsDir, settingsDir string, store *FileStore, logger *AnalyticsLogger) {
	glbPath := filepath.Join(originalsDir, id+".glb")
	result, err := RunClassifier(glbPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "autoClassify %s: %v\n", id, err)
		return
	}
	s, err := applyClassificationToSettings(id, settingsDir, result)
	if err != nil {
		fmt.Fprintf(os.Stderr, "autoClassify %s: %v\n", id, err)
		return
	}
	dirty := SettingsDifferFromDefaults(s)
	store.Update(id, func(r *FileRecord) {
		r.HasSavedSettings = true
		r.SettingsDirty = dirty
	})
	emitClassificationEvent(logger, id, result)
	emitStrategySelectedEvent(logger, id, getStrategyForCategory(result.Category))
}

// handleClassify handles POST /api/classify/:id. Re-runs the shape
// classifier on the asset's original GLB, persists the result into the
// asset's settings, marks the FileRecord dirty, and emits a
// "classification" analytics event.
//
// T-004-04: the response body is now {settings, candidates}, and an
// optional `?override=<category>` query parameter switches the handler
// into override mode: the freshly-measured features are kept (so the
// stamped strategy reflects current geometry), but the category is
// replaced by the user's pick and confidence is pinned to 1.0. The
// override branch emits a "classification_override" event in lieu of
// the normal "classification" event — the split is intentional so
// downstream training distinguishes the system's pick from the
// human correction.
func handleClassify(store *FileStore, originalsDir, settingsDir string, logger *AnalyticsLogger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/classify/")
		if id == "" {
			jsonError(w, http.StatusBadRequest, "missing file id")
			return
		}
		if _, ok := store.Get(id); !ok {
			jsonError(w, http.StatusNotFound, "file not found")
			return
		}
		override := r.URL.Query().Get("override")
		if override != "" && !validShapeCategories[override] {
			jsonError(w, http.StatusBadRequest, "unknown override category: "+override)
			return
		}
		glbPath := filepath.Join(originalsDir, id+".glb")
		result, err := RunClassifier(glbPath)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "classify: "+err.Error())
			return
		}
		var (
			isOverride        = override != ""
			originalCategory  = result.Category
			originalConfidence = result.Confidence
		)
		if isOverride {
			// Synthesize the override result. Re-use the just-measured
			// features so persisted state and the emitted event both
			// reflect current geometry, not stale data.
			result.Category = override
			result.Confidence = 1.0
		}
		s, err := applyClassificationToSettings(id, settingsDir, result)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, err.Error())
			return
		}
		dirty := SettingsDifferFromDefaults(s)
		store.Update(id, func(r *FileRecord) {
			r.HasSavedSettings = true
			r.SettingsDirty = dirty
		})
		if isOverride {
			emitClassificationOverrideEvent(logger, id, originalCategory, originalConfidence, result)
		} else {
			emitClassificationEvent(logger, id, result)
		}
		emitStrategySelectedEvent(logger, id, getStrategyForCategory(result.Category))
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"settings":   s,
			"candidates": extractCandidates(result.Features),
		})
	}
}

// handleStatus handles GET /api/status — returns server capabilities.
func handleStatus(blender BlenderInfo) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"blender": blender,
		})
	}
}

// handleUploadVolumetricLOD handles POST /api/upload-volumetric-lod/:id?level=0-3
func handleUploadVolumetricLOD(store *FileStore, outputsDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		id := strings.TrimPrefix(r.URL.Path, "/api/upload-volumetric-lod/")
		if _, ok := store.Get(id); !ok {
			jsonError(w, http.StatusNotFound, "file not found")
			return
		}

		levelStr := r.URL.Query().Get("level")
		level, err := strconv.Atoi(levelStr)
		if err != nil || level < 0 || level > 3 {
			jsonError(w, http.StatusBadRequest, "level must be 0-3")
			return
		}

		body, err := io.ReadAll(io.LimitReader(r.Body, 10<<20)) // 10MB max
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to read body")
			return
		}

		outputPath := filepath.Join(outputsDir, fmt.Sprintf("%s_vlod%d.glb", id, level))
		if err := os.WriteFile(outputPath, body, 0644); err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to save volumetric LOD")
			return
		}

		var fileSize int64
		if info, err := os.Stat(outputPath); err == nil {
			fileSize = info.Size()
		}

		store.Update(id, func(r *FileRecord) {
			// Initialize slice if needed
			if len(r.VolumetricLODs) < 4 {
				r.VolumetricLODs = make([]LODLevel, 4)
				for i := range r.VolumetricLODs {
					r.VolumetricLODs[i].Level = i
				}
			}
			r.VolumetricLODs[level] = LODLevel{Level: level, Size: fileSize}

			// Compute metadata when all levels are present
			var totalSize int64
			allPresent := true
			for _, lod := range r.VolumetricLODs {
				if lod.Size == 0 {
					allPresent = false
					break
				}
				totalSize += lod.Size
			}
			if allPresent {
				r.VolumetricLODMeta = &LODMeta{
					Distances: []float64{5.0, 15.0, 30.0},
					TotalSize: totalSize,
				}
			}
		})

		record, _ := store.Get(id)
		jsonResponse(w, http.StatusOK, record)
	}
}

// handleOptimizeScene handles POST /api/optimize-scene
func handleOptimizeScene(store *FileStore, originalsDir, outputsDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}

		var req SceneRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid request: "+err.Error())
			return
		}

		// Validate budget
		if req.Budget.MaxTriangles <= 0 {
			jsonError(w, http.StatusBadRequest, "max_triangles must be positive")
			return
		}

		// Validate assets
		if len(req.Assets) == 0 {
			jsonError(w, http.StatusBadRequest, "at least one asset is required")
			return
		}

		validTypes := map[string]bool{"hard-surface": true, "organic": true}
		validRoles := map[string]bool{"hero": true, "mid-ground": true, "background": true}
		labels := make(map[string]bool)

		for _, a := range req.Assets {
			if a.FileID == "" || a.Label == "" {
				jsonError(w, http.StatusBadRequest, "each asset must have file_id and label")
				return
			}
			if !validTypes[a.AssetType] {
				jsonError(w, http.StatusBadRequest, fmt.Sprintf("invalid asset_type %q: must be hard-surface or organic", a.AssetType))
				return
			}
			if !validRoles[a.SceneRole] {
				jsonError(w, http.StatusBadRequest, fmt.Sprintf("invalid scene_role %q: must be hero, mid-ground, or background", a.SceneRole))
				return
			}
			if _, ok := store.Get(a.FileID); !ok {
				jsonError(w, http.StatusNotFound, fmt.Sprintf("file %q not found", a.FileID))
				return
			}
			if labels[a.Label] {
				jsonError(w, http.StatusBadRequest, fmt.Sprintf("duplicate label %q", a.Label))
				return
			}
			labels[a.Label] = true
		}

		// Generate scene ID and output directory
		sceneID := "scene_" + generateID()
		sceneDir := filepath.Join(outputsDir, sceneID)
		if err := os.MkdirAll(sceneDir, 0755); err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to create scene directory")
			return
		}

		// Allocate budget
		budgetAlloc := AllocateBudget(req.Budget, req.Assets)

		var results []SceneAssetResult
		totalTris := 0
		totalTexKB := 0

		for _, asset := range req.Assets {
			strategy := SelectStrategy(asset.AssetType, asset.SceneRole)
			inputPath := filepath.Join(originalsDir, asset.FileID+".glb")
			outputPath := filepath.Join(sceneDir, asset.Label+".glb")

			var processErr error
			var strategyName string

			switch strategy.Name {
			case "parametric":
				strategyName = "parametric_reconstruct"
				output, err := RunParametricReconstruct(inputPath, outputPath)
				if err != nil {
					processErr = fmt.Errorf("parametric reconstruction failed: %s: %w", output, err)
				}

			case "gltfpack":
				strategyName = fmt.Sprintf("gltfpack_si%.2f", strategy.Simplification)
				settings := Settings{
					Simplification:     strategy.Simplification,
					AggressiveSimplify: strategy.Aggressive,
					PermissiveSimplify: strategy.Permissive,
					Compression:        "cc",
				}
				args := BuildCommand(inputPath, outputPath, settings)
				output, err := RunGltfpack(args)
				if err != nil {
					processErr = fmt.Errorf("gltfpack failed: %s: %w", output, err)
				}

			case "volumetric":
				strategyName = fmt.Sprintf("volumetric_lod%d", strategy.VolumetricLOD)
				// Look for pre-generated volumetric LOD file
				vlodPath := filepath.Join(outputsDir, fmt.Sprintf("%s_vlod%d.glb", asset.FileID, strategy.VolumetricLOD))
				if _, err := os.Stat(vlodPath); os.IsNotExist(err) {
					// Fall back to base volumetric
					volPath := filepath.Join(outputsDir, fmt.Sprintf("%s_volumetric.glb", asset.FileID))
					if _, err := os.Stat(volPath); os.IsNotExist(err) {
						processErr = fmt.Errorf("volumetric LOD not pre-generated for asset %q (file_id=%s); generate volumetric LODs first via the UI", asset.Label, asset.FileID)
					} else {
						strategyName = "volumetric"
						copyFile(volPath, outputPath)
					}
				} else {
					copyFile(vlodPath, outputPath)
				}
			}

			if processErr != nil {
				jsonError(w, http.StatusUnprocessableEntity, processErr.Error())
				return
			}

			// Count triangles and file size
			triCount, _ := CountTrianglesGLB(outputPath)
			var texSizeKB int
			if info, err := os.Stat(outputPath); err == nil {
				texSizeKB = int(info.Size() / 1024)
			}

			_ = budgetAlloc // budget allocation is informational; we report actual usage

			totalTris += triCount
			totalTexKB += texSizeKB

			results = append(results, SceneAssetResult{
				Label:         asset.Label,
				FileID:        asset.FileID,
				AssetType:     asset.AssetType,
				SceneRole:     asset.SceneRole,
				Strategy:      strategyName,
				TriangleCount: triCount,
				TextureSizeKB: texSizeKB,
				OutputFile:    filepath.Join(sceneID, asset.Label+".glb"),
			})
		}

		manifest := SceneResult{
			SceneID: sceneID,
			BudgetUsed: SceneBudget{
				MaxTriangles:      totalTris,
				MaxTextureMemoryKB: totalTexKB,
			},
			BudgetTotal: req.Budget,
			Assets:      results,
		}

		// Write manifest to scene directory
		manifestData, _ := json.MarshalIndent(manifest, "", "  ")
		os.WriteFile(filepath.Join(sceneDir, "manifest.json"), manifestData, 0644)

		jsonResponse(w, http.StatusOK, manifest)
	}
}

// copyFile copies a file from src to dst.
func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0644)
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

// handleAnalyticsEvent handles POST /api/analytics/event. It decodes the
// envelope, validates schema_version and event_type, and appends the event
// to the per-session JSONL file. Payload contents are not introspected —
// see docs/knowledge/analytics-schema.md for the per-type payload shapes.
func handleAnalyticsEvent(logger *AnalyticsLogger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var ev Event
		if err := json.NewDecoder(r.Body).Decode(&ev); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid event: "+err.Error())
			return
		}
		if err := ev.Validate(); err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		if err := logger.AppendEvent(ev.SessionID, ev); err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to append event: "+err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
	}
}

// handleAnalyticsStartSession handles POST /api/analytics/start-session.
// Body: {"asset_id":"..."}.
// Response: {"session_id":"...","resumed":bool}.
//
// Resumes the most recent session for the asset if one exists, otherwise
// mints a new one (which writes a session_start envelope to disk).
func handleAnalyticsStartSession(logger *AnalyticsLogger) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var body struct {
			AssetID string `json:"asset_id"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid body: "+err.Error())
			return
		}
		if body.AssetID == "" {
			jsonError(w, http.StatusBadRequest, "asset_id must be set")
			return
		}
		id, resumed, err := logger.LookupOrStartSession(body.AssetID)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "start session: "+err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, map[string]interface{}{
			"session_id": id,
			"resumed":    resumed,
		})
	}
}

// handleProfilesList handles GET /api/profiles. Returns []ProfileMetadata
// sorted by name. Empty list is encoded as `[]`, never `null`.
func handleProfilesList(profilesDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		list, err := ListProfiles(profilesDir)
		if err != nil {
			jsonError(w, http.StatusInternalServerError, "failed to list profiles: "+err.Error())
			return
		}
		if list == nil {
			list = []ProfileMetadata{}
		}
		jsonResponse(w, http.StatusOK, list)
	}
}

// profileSaveRequest is the wire shape for POST /api/profiles. CreatedAt
// is intentionally omitted — the server stamps it on save.
type profileSaveRequest struct {
	Name          string         `json:"name"`
	Comment       string         `json:"comment"`
	SourceAssetID string         `json:"source_asset_id"`
	Settings      *AssetSettings `json:"settings"`
}

// handleProfileSave handles POST /api/profiles.
func handleProfileSave(profilesDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
			return
		}
		var req profileSaveRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			jsonError(w, http.StatusBadRequest, "invalid profile: "+err.Error())
			return
		}
		p := &Profile{
			SchemaVersion: ProfilesSchemaVersion,
			Name:          req.Name,
			Comment:       req.Comment,
			SourceAssetID: req.SourceAssetID,
			Settings:      req.Settings,
		}
		if err := SaveProfile(p, profilesDir); err != nil {
			// Validation errors are user-actionable; disk errors are not.
			// Both surface as the same error type, so we differentiate
			// by inspecting the message — anything that came from
			// (*Profile).Validate or AssetSettings.Validate is a 400.
			if isProfileValidationError(err) {
				jsonError(w, http.StatusBadRequest, err.Error())
				return
			}
			jsonError(w, http.StatusInternalServerError, "failed to save profile: "+err.Error())
			return
		}
		jsonResponse(w, http.StatusOK, p)
	}
}

// handleProfile handles GET / DELETE on /api/profiles/:name.
func handleProfile(profilesDir string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		name := strings.TrimPrefix(r.URL.Path, "/api/profiles/")
		if err := ValidateProfileName(name); err != nil {
			jsonError(w, http.StatusBadRequest, err.Error())
			return
		}
		switch r.Method {
		case http.MethodGet:
			p, err := LoadProfile(name, profilesDir)
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					jsonError(w, http.StatusNotFound, "profile not found")
					return
				}
				jsonError(w, http.StatusInternalServerError, "failed to load profile: "+err.Error())
				return
			}
			jsonResponse(w, http.StatusOK, p)
		case http.MethodDelete:
			if err := DeleteProfile(name, profilesDir); err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					jsonError(w, http.StatusNotFound, "profile not found")
					return
				}
				jsonError(w, http.StatusInternalServerError, "failed to delete profile: "+err.Error())
				return
			}
			jsonResponse(w, http.StatusOK, map[string]string{"status": "deleted"})
		default:
			jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}

// isProfileValidationError reports whether err originated from
// (*Profile).Validate or one of its delegates. SaveProfile bundles
// validation and disk failures into the same error type, so we sniff
// for the validator's known prefixes to give the HTTP layer a 400 vs
// 500 split.
func isProfileValidationError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	switch {
	case strings.HasPrefix(msg, "profile name "),
		strings.HasPrefix(msg, "unsupported schema_version"),
		strings.HasPrefix(msg, "comment exceeds"),
		strings.HasPrefix(msg, "settings must not be null"),
		strings.HasPrefix(msg, "settings: "):
		return true
	}
	return false
}

// acceptRequest is the wire shape for POST /api/accept/:id.
//
// The server reads the canonical settings from disk via LoadSettings,
// not from the client — the client has already PUT them through
// /api/settings/:id by the time the user clicks "Mark as Accepted",
// and reading from disk avoids any debounce-flush race.
type acceptRequest struct {
	Comment       string `json:"comment"`
	ThumbnailB64  string `json:"thumbnail_b64"`
}

// maxAcceptedThumbnailBytes caps decoded thumbnail size. A 256px JPEG at
// quality 0.85 is ~10-20 KB; this is ~100x headroom for safety against
// pathological inputs.
const maxAcceptedThumbnailBytes = 2 << 20 // 2 MiB

// handleAccept handles GET / POST on /api/accept/:id.
//
//	GET  → returns the asset's accepted snapshot, 404 if none exists yet.
//	POST → snapshots the asset's currently saved settings, optionally
//	       writes a base64-encoded JPEG thumbnail, persists both, marks
//	       the FileRecord as accepted, and emits an `accept` analytics
//	       event into the asset's session JSONL.
//
// See docs/active/work/T-003-04/design.md for the rationale behind reading
// settings from disk and using base64 in JSON instead of multipart upload.
func handleAccept(
	store *FileStore,
	settingsDir, acceptedDir, acceptedThumbsDir string,
	logger *AnalyticsLogger,
) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(r.URL.Path, "/api/accept/")
		if id == "" {
			jsonError(w, http.StatusBadRequest, "missing file id")
			return
		}

		switch r.Method {
		case http.MethodGet:
			if _, ok := store.Get(id); !ok {
				jsonError(w, http.StatusNotFound, "file not found")
				return
			}
			a, err := LoadAccepted(id, acceptedDir)
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					jsonError(w, http.StatusNotFound, "no accepted snapshot for this asset")
					return
				}
				jsonError(w, http.StatusInternalServerError, "failed to load accepted: "+err.Error())
				return
			}
			jsonResponse(w, http.StatusOK, a)

		case http.MethodPost:
			if _, ok := store.Get(id); !ok {
				jsonError(w, http.StatusNotFound, "file not found")
				return
			}
			var req acceptRequest
			if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
				jsonError(w, http.StatusBadRequest, "invalid body: "+err.Error())
				return
			}
			if len(req.Comment) > acceptedCommentMaxLen {
				jsonError(w, http.StatusBadRequest, fmt.Sprintf("comment exceeds %d chars", acceptedCommentMaxLen))
				return
			}

			// Snapshot the canonical settings from disk.
			s, err := LoadSettings(id, settingsDir)
			if err != nil {
				jsonError(w, http.StatusInternalServerError, "failed to load settings: "+err.Error())
				return
			}
			if err := s.Validate(); err != nil {
				jsonError(w, http.StatusBadRequest, "saved settings invalid: "+err.Error())
				return
			}

			// Decode thumbnail (optional). Tolerate "data:image/jpeg;base64," prefix.
			var thumbBytes []byte
			thumbB64 := req.ThumbnailB64
			if i := strings.Index(thumbB64, ","); i >= 0 && strings.HasPrefix(thumbB64, "data:") {
				thumbB64 = thumbB64[i+1:]
			}
			if thumbB64 != "" {
				// Reject pre-decode if the encoded length already implies an
				// oversized payload — base64 expands by ~33%, so a strict
				// upper bound on raw bytes is encoded_len * 3 / 4.
				if len(thumbB64)*3/4 > maxAcceptedThumbnailBytes {
					jsonError(w, http.StatusBadRequest, fmt.Sprintf("thumbnail exceeds %d bytes", maxAcceptedThumbnailBytes))
					return
				}
				decoded, err := base64.StdEncoding.DecodeString(thumbB64)
				if err != nil {
					jsonError(w, http.StatusBadRequest, "invalid base64 thumbnail: "+err.Error())
					return
				}
				if len(decoded) > maxAcceptedThumbnailBytes {
					jsonError(w, http.StatusBadRequest, fmt.Sprintf("thumbnail exceeds %d bytes", maxAcceptedThumbnailBytes))
					return
				}
				thumbBytes = decoded
			}

			var relThumbPath string
			if len(thumbBytes) > 0 {
				if err := WriteThumbnail(id, acceptedThumbsDir, thumbBytes); err != nil {
					jsonError(w, http.StatusInternalServerError, "failed to write thumbnail: "+err.Error())
					return
				}
				// Always emit a forward-slash relative path so it is
				// portable into the export bundle and any downstream
				// tools that expect URL-shaped strings.
				relThumbPath = path.Join("accepted", "thumbs", id+".jpg")
			}

			snap := &AcceptedSettings{
				SchemaVersion: AcceptedSchemaVersion,
				AssetID:       id,
				Comment:       req.Comment,
				ThumbnailPath: relThumbPath,
				Settings:      s,
			}
			if err := SaveAccepted(snap, acceptedDir); err != nil {
				jsonError(w, http.StatusInternalServerError, "failed to save accepted: "+err.Error())
				return
			}

			store.Update(id, func(rec *FileRecord) {
				rec.IsAccepted = true
			})

			// Emit an analytics `accept` event into the asset's session.
			// LookupOrStartSession ensures the event lands somewhere even
			// if no client session is currently open (e.g. a curl invocation).
			sessionID, _, lookupErr := logger.LookupOrStartSession(id)
			if lookupErr == nil && sessionID != "" {
				ev := Event{
					SchemaVersion: AnalyticsSchemaVersion,
					EventType:     "accept",
					Timestamp:     time.Now().UTC().Format(time.RFC3339Nano),
					SessionID:     sessionID,
					AssetID:       id,
					Payload: map[string]interface{}{
						"settings":       s,
						"thumbnail_path": relThumbPath,
					},
				}
				if err := logger.AppendEvent(sessionID, ev); err != nil {
					// Don't fail the request — the snapshot is the
					// load-bearing artifact; the event is a nice-to-have
					// for the export pipeline.
					fmt.Fprintf(os.Stderr, "handleAccept: append accept event: %v\n", err)
				}
			}

			jsonResponse(w, http.StatusOK, snap)

		default:
			jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	}
}
