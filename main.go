package main

import (
	"embed"
	"flag"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

//go:embed static
var staticFiles embed.FS

func main() {
	port := flag.Int("port", 8787, "HTTP server port")
	dir := flag.String("dir", "", "Working directory (default: ~/.glb-optimizer)")
	flag.Parse()

	// Check gltfpack is available
	gltfpackPath, err := exec.LookPath("gltfpack")
	if err != nil {
		fmt.Fprintln(os.Stderr, "Error: gltfpack not found on PATH.")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Install the pre-built binary:")
		fmt.Fprintln(os.Stderr, "  curl -L https://github.com/zeux/meshoptimizer/releases/latest/download/gltfpack-macos.zip > gltfpack.zip")
		fmt.Fprintln(os.Stderr, "  unzip -o gltfpack.zip && chmod a+x gltfpack && sudo mv gltfpack /usr/local/bin/")
		fmt.Fprintln(os.Stderr, "  # If macOS blocks it: xattr -d com.apple.quarantine /usr/local/bin/gltfpack")
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Or install via npm (no texture compression support):")
		fmt.Fprintln(os.Stderr, "  npm install -g gltfpack")
		os.Exit(1)
	}

	versionOut, _ := exec.Command(gltfpackPath, "--version").CombinedOutput()
	versionStr := strings.TrimSpace(string(versionOut))
	if versionStr == "" {
		versionStr = "(version unknown)"
	}
	fmt.Printf("Found gltfpack: %s — %s\n", gltfpackPath, versionStr)

	// Set up working directory
	workDir := *dir
	if workDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error: cannot determine home directory: %v\n", err)
			os.Exit(1)
		}
		workDir = filepath.Join(home, ".glb-optimizer")
	}

	originalsDir := filepath.Join(workDir, "originals")
	outputsDir := filepath.Join(workDir, "outputs")
	settingsDir := filepath.Join(workDir, "settings")
	tuningDir := filepath.Join(workDir, "tuning")
	profilesDir := filepath.Join(workDir, "profiles")
	acceptedDir := filepath.Join(workDir, "accepted")
	acceptedThumbsDir := filepath.Join(acceptedDir, "thumbs")

	for _, d := range []string{originalsDir, outputsDir, settingsDir, tuningDir, profilesDir, acceptedDir, acceptedThumbsDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			fmt.Fprintf(os.Stderr, "Error: cannot create directory %s: %v\n", d, err)
			os.Exit(1)
		}
	}
	fmt.Printf("Working directory: %s\n", workDir)

	// Detect Blender
	blenderInfo := DetectBlender()
	if blenderInfo.Available {
		fmt.Printf("Found Blender: %s — %s\n", blenderInfo.Path, blenderInfo.Version)
	} else {
		fmt.Println("Blender not found (optional — enables high-quality remesh LODs)")
	}

	// Write embedded Blender script to working directory
	var blenderScriptPath string
	if blenderInfo.Available {
		var err error
		blenderScriptPath, err = WriteEmbeddedScript(workDir)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to write Blender script: %v\n", err)
			blenderInfo.Available = false
		}
	}

	// Initialize file store and scan existing files
	store := NewFileStore()
	scanExistingFiles(store, originalsDir, outputsDir, settingsDir, acceptedDir)

	// Analytics logger writes per-session JSONL into tuningDir.
	analyticsLogger := NewAnalyticsLogger(tuningDir)

	// Processing queue channel (unused for now — processing is synchronous)
	queue := make(chan processJob, 100)
	_ = queue

	// Routes
	mux := http.NewServeMux()

	mux.HandleFunc("/api/upload", handleUpload(store, originalsDir))
	mux.HandleFunc("/api/files", func(w http.ResponseWriter, r *http.Request) {
		// Route DELETE /api/files/:id vs GET /api/files
		if r.Method == http.MethodDelete || strings.Count(r.URL.Path, "/") > 2 {
			handleDeleteFile(store, originalsDir, outputsDir)(w, r)
			return
		}
		handleFiles(store)(w, r)
	})
	mux.HandleFunc("/api/files/", handleDeleteFile(store, originalsDir, outputsDir))
	mux.HandleFunc("/api/process-all", handleProcessAll(store, originalsDir, outputsDir))
	mux.HandleFunc("/api/process/", handleProcess(store, originalsDir, outputsDir, queue))
	mux.HandleFunc("/api/download-all", handleDownloadAll(store, outputsDir))
	mux.HandleFunc("/api/download/", handleDownload(store, outputsDir))
	mux.HandleFunc("/api/generate-lods/", handleGenerateLODs(store, originalsDir, outputsDir))
	mux.HandleFunc("/api/generate-blender-lods/", handleGenerateBlenderLODs(store, originalsDir, outputsDir, blenderInfo, blenderScriptPath))
	mux.HandleFunc("/api/upload-billboard/", handleUploadBillboard(store, outputsDir))
	mux.HandleFunc("/api/upload-volumetric/", handleUploadVolumetric(store, outputsDir))
	mux.HandleFunc("/api/upload-volumetric-lod/", handleUploadVolumetricLOD(store, outputsDir))
	mux.HandleFunc("/api/upload-reference/", handleUploadReference(store, outputsDir))
	mux.HandleFunc("/api/reference/", handleReferenceImage(store, outputsDir))
	mux.HandleFunc("/api/optimize-scene", handleOptimizeScene(store, originalsDir, outputsDir))
	mux.HandleFunc("/api/status", handleStatus(blenderInfo))
	mux.HandleFunc("/api/settings/", handleSettings(store, settingsDir))
	mux.HandleFunc("/api/preview/", handlePreview(store, originalsDir, outputsDir))
	mux.HandleFunc("/api/analytics/event", handleAnalyticsEvent(analyticsLogger))
	mux.HandleFunc("/api/analytics/start-session", handleAnalyticsStartSession(analyticsLogger))
	mux.HandleFunc("/api/profiles", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			handleProfilesList(profilesDir)(w, r)
		case http.MethodPost:
			handleProfileSave(profilesDir)(w, r)
		default:
			jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	})
	mux.HandleFunc("/api/profiles/", handleProfile(profilesDir))
	mux.HandleFunc("/api/accept/", handleAccept(store, settingsDir, acceptedDir, acceptedThumbsDir, analyticsLogger))

	// Serve embedded static files
	staticFS, _ := fs.Sub(staticFiles, "static")
	fileServer := http.FileServer(http.FS(staticFS))
	mux.Handle("/", fileServer)

	addr := fmt.Sprintf("localhost:%d", *port)
	url := fmt.Sprintf("http://%s", addr)
	fmt.Printf("GLB Optimizer running at %s\n", url)

	// Open browser (macOS)
	go exec.Command("open", url).Run()

	if err := http.ListenAndServe(addr, mux); err != nil {
		fmt.Fprintf(os.Stderr, "Error starting server: %v\n", err)
		os.Exit(1)
	}
}

// scanExistingFiles loads file state from disk on startup.
func scanExistingFiles(store *FileStore, originalsDir, outputsDir, settingsDir, acceptedDir string) {
	entries, err := os.ReadDir(originalsDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".glb") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".glb")
		info, err := e.Info()
		if err != nil {
			continue
		}

		record := &FileRecord{
			ID:           id,
			Filename:     e.Name(), // We lose original filename on restart
			OriginalSize: info.Size(),
			Status:       StatusPending,
		}

		// Check if output exists
		outputPath := filepath.Join(outputsDir, e.Name())
		if outInfo, err := os.Stat(outputPath); err == nil {
			record.Status = StatusDone
			record.OutputSize = outInfo.Size()
		}

		record.HasSavedSettings = SettingsExist(id, settingsDir)
		if record.HasSavedSettings {
			// T-005-02: also surface whether the on-disk settings
			// diverge from defaults so the file list can render a
			// "tuned" indicator without an extra round trip. Errors
			// are intentionally swallowed: a corrupt settings file
			// should not block the scan, and SettingsDirty=false is
			// the right conservative default.
			if s, err := LoadSettings(id, settingsDir); err == nil {
				record.SettingsDirty = SettingsDifferFromDefaults(s)
			}
		}
		record.IsAccepted = AcceptedExists(id, acceptedDir)

		store.Add(record)
	}
}
