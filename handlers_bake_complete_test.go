package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// TestHandleBakeComplete_Happy verifies that POST /api/bake-complete/:id
// stamps {outputsDir}/{id}_bake.json and returns the same bake_id in
// the response body. The downstream stability test lives in
// pack_meta_capture_test.go (TestBuildPackMetaFromBake_StableBakeID);
// this test focuses on the HTTP layer.
func TestHandleBakeComplete_Happy(t *testing.T) {
	outputsDir := t.TempDir()
	store := NewFileStore()
	const id = "asset-bake-1"
	store.Add(&FileRecord{ID: id, Filename: id + ".glb", Status: StatusPending})

	handler := handleBakeComplete(store, outputsDir)

	req := httptest.NewRequest(http.MethodPost, "/api/bake-complete/"+id, nil)
	rr := httptest.NewRecorder()
	handler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body=%s", rr.Code, rr.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	bakeID, _ := resp["bake_id"].(string)
	if bakeID == "" {
		t.Fatal("response missing bake_id")
	}
	if _, err := time.Parse(time.RFC3339, bakeID); err != nil {
		t.Errorf("bake_id %q does not parse as RFC3339: %v", bakeID, err)
	}

	stamp, err := ReadBakeStamp(outputsDir, id)
	if err != nil {
		t.Fatalf("ReadBakeStamp: %v", err)
	}
	if stamp.BakeID != bakeID {
		t.Errorf("file BakeID %q != response %q", stamp.BakeID, bakeID)
	}
	if stamp.CompletedAt != bakeID {
		t.Errorf("file CompletedAt %q should equal BakeID %q at write time",
			stamp.CompletedAt, bakeID)
	}
	if _, err := os.Stat(filepath.Join(outputsDir, id+"_bake.json")); err != nil {
		t.Errorf("expected bake stamp file on disk: %v", err)
	}
}

func TestHandleBakeComplete_NotFound(t *testing.T) {
	outputsDir := t.TempDir()
	store := NewFileStore()
	handler := handleBakeComplete(store, outputsDir)

	req := httptest.NewRequest(http.MethodPost, "/api/bake-complete/missing", nil)
	rr := httptest.NewRecorder()
	handler(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", rr.Code)
	}
}

func TestHandleBakeComplete_WrongMethod(t *testing.T) {
	outputsDir := t.TempDir()
	store := NewFileStore()
	store.Add(&FileRecord{ID: "x", Filename: "x.glb"})
	handler := handleBakeComplete(store, outputsDir)

	req := httptest.NewRequest(http.MethodGet, "/api/bake-complete/x", nil)
	rr := httptest.NewRecorder()
	handler(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", rr.Code)
	}
}

// TestHandleBakeComplete_OverwriteOnRebake confirms a second POST
// rewrites the file with a fresh timestamp. Rebaking SHOULD mint a
// new id — the stability property holds across combine runs of the
// SAME intermediates, not across re-bakes.
func TestHandleBakeComplete_OverwriteOnRebake(t *testing.T) {
	outputsDir := t.TempDir()
	store := NewFileStore()
	const id = "asset-rebake"
	store.Add(&FileRecord{ID: id, Filename: id + ".glb"})
	handler := handleBakeComplete(store, outputsDir)

	doPost := func() string {
		req := httptest.NewRequest(http.MethodPost, "/api/bake-complete/"+id, nil)
		rr := httptest.NewRecorder()
		handler(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
		}
		var resp map[string]interface{}
		if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
			t.Fatalf("decode: %v", err)
		}
		return resp["bake_id"].(string)
	}

	first := doPost()
	time.Sleep(1100 * time.Millisecond)
	second := doPost()
	if first == second {
		t.Errorf("expected distinct ids across rebakes, both = %q", first)
	}

	stamp, err := ReadBakeStamp(outputsDir, id)
	if err != nil {
		t.Fatal(err)
	}
	if stamp.BakeID != second {
		t.Errorf("on-disk BakeID = %q, want latest %q", stamp.BakeID, second)
	}
}
