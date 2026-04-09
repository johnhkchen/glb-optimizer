package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestBuildProduction_MethodNotAllowed(t *testing.T) {
	store := NewFileStore()
	handler := handleBuildProduction(store, t.TempDir(), t.TempDir(), BlenderInfo{Available: true, Path: "/usr/bin/true"}, "scripts/render_production.py")

	req := httptest.NewRequest(http.MethodGet, "/api/build-production/abc123", nil)
	rr := httptest.NewRecorder()
	handler(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("GET: status = %d, want %d", rr.Code, http.StatusMethodNotAllowed)
	}
}

func TestBuildProduction_BlenderNotAvailable(t *testing.T) {
	store := NewFileStore()
	handler := handleBuildProduction(store, t.TempDir(), t.TempDir(), BlenderInfo{Available: false}, "")

	req := httptest.NewRequest(http.MethodPost, "/api/build-production/abc123", nil)
	rr := httptest.NewRecorder()
	handler(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rr.Code, http.StatusInternalServerError)
	}

	var body map[string]string
	json.Unmarshal(rr.Body.Bytes(), &body)
	if body["error"] != "blender not installed" {
		t.Errorf("error = %q, want %q", body["error"], "blender not installed")
	}
}

func TestBuildProduction_AssetNotFound(t *testing.T) {
	store := NewFileStore()
	settingsDir := t.TempDir()
	outputsDir := t.TempDir()

	// Write a dummy script so the script-exists check passes.
	scriptPath := filepath.Join(t.TempDir(), "render_production.py")
	os.WriteFile(scriptPath, []byte("# dummy"), 0644)

	handler := handleBuildProduction(store, settingsDir, outputsDir, BlenderInfo{Available: true, Path: "/usr/bin/true"}, scriptPath)

	req := httptest.NewRequest(http.MethodPost, "/api/build-production/nonexistent", nil)
	rr := httptest.NewRecorder()
	handler(rr, req)

	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d; body=%s", rr.Code, http.StatusNotFound, rr.Body.String())
	}
}

func TestBuildProduction_AssetNotOptimized(t *testing.T) {
	store := NewFileStore()
	const id = "test-asset-1"
	store.Add(&FileRecord{ID: id, Filename: id + ".glb", Status: StatusPending})

	settingsDir := t.TempDir()
	outputsDir := t.TempDir()

	scriptPath := filepath.Join(t.TempDir(), "render_production.py")
	os.WriteFile(scriptPath, []byte("# dummy"), 0644)

	handler := handleBuildProduction(store, settingsDir, outputsDir, BlenderInfo{Available: true, Path: "/usr/bin/true"}, scriptPath)

	req := httptest.NewRequest(http.MethodPost, "/api/build-production/"+id, nil)
	rr := httptest.NewRecorder()
	handler(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d; body=%s", rr.Code, http.StatusBadRequest, rr.Body.String())
	}

	var body map[string]string
	json.Unmarshal(rr.Body.Bytes(), &body)
	if body["error"] != "asset must be optimized first (status=done)" {
		t.Errorf("error = %q", body["error"])
	}
}

func TestBuildProduction_ConfigGeneration(t *testing.T) {
	store := NewFileStore()
	const id = "test-asset-cfg"
	store.Add(&FileRecord{ID: id, Filename: id + ".glb", Status: StatusDone})

	settingsDir := t.TempDir()
	outputsDir := t.TempDir()

	// Save settings so LoadSettings returns non-defaults.
	s := DefaultSettings()
	s.ShapeCategory = "tall-narrow"
	s.DomeHeightFactor = 0.6
	SaveSettings(id, settingsDir, s)

	// Use /bin/sh -c "exit 1" as a fake blender — it will fail,
	// but we can inspect the config file before it's cleaned up.
	// Instead, we use a script that just exits 0 but doesn't produce files.
	scriptPath := filepath.Join(t.TempDir(), "render_production.py")
	os.WriteFile(scriptPath, []byte("# dummy"), 0644)

	// Use /usr/bin/true as a stand-in for blender: exits 0, produces nothing.
	handler := handleBuildProduction(store, settingsDir, outputsDir, BlenderInfo{Available: true, Path: "/usr/bin/true"}, scriptPath)

	req := httptest.NewRequest(http.MethodPost, "/api/build-production/"+id, nil)
	rr := httptest.NewRecorder()
	handler(rr, req)

	// Will get 500 because intermediates are missing (true exits 0 but writes no files).
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d; body=%s", rr.Code, http.StatusInternalServerError, rr.Body.String())
	}

	var body map[string]string
	json.Unmarshal(rr.Body.Bytes(), &body)
	if msg := body["error"]; msg == "" {
		t.Error("expected error about missing intermediates")
	}
}

func TestBuildProduction_HardSurfaceSkipsVolumetric(t *testing.T) {
	store := NewFileStore()
	const id = "test-hard-surface"
	store.Add(&FileRecord{ID: id, Filename: id + ".glb", Status: StatusDone})

	settingsDir := t.TempDir()
	outputsDir := t.TempDir()

	scriptPath := filepath.Join(t.TempDir(), "render_production.py")
	os.WriteFile(scriptPath, []byte("# dummy"), 0644)

	// Create the two billboard intermediates that hard-surface should produce.
	os.WriteFile(filepath.Join(outputsDir, id+"_billboard.glb"), []byte("glb"), 0644)
	os.WriteFile(filepath.Join(outputsDir, id+"_billboard_tilted.glb"), []byte("glb"), 0644)
	// Deliberately no volumetric file.

	// Use /usr/bin/true (exits 0) as fake blender.
	handler := handleBuildProduction(store, settingsDir, outputsDir, BlenderInfo{Available: true, Path: "/usr/bin/true"}, scriptPath)

	req := httptest.NewRequest(http.MethodPost, "/api/build-production/"+id+"?category=hard-surface", nil)
	rr := httptest.NewRecorder()
	handler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(rr.Body.Bytes(), &resp)

	if resp["billboard"] != true {
		t.Errorf("billboard = %v, want true", resp["billboard"])
	}
	if resp["tilted"] != true {
		t.Errorf("tilted = %v, want true", resp["tilted"])
	}
	if resp["volumetric"] != false {
		t.Errorf("volumetric = %v, want false (hard-surface skips volumetric)", resp["volumetric"])
	}

	// Verify FileStore was updated.
	rec, _ := store.Get(id)
	if !rec.HasBillboard {
		t.Error("HasBillboard should be true")
	}
	if !rec.HasBillboardTilted {
		t.Error("HasBillboardTilted should be true")
	}
	if rec.HasVolumetric {
		t.Error("HasVolumetric should be false for hard-surface")
	}
}
