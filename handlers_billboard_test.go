package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestHandleUploadBillboardTilted is the focused unit test for
// T-009-01's new endpoint. Mirrors the structure of the existing
// billboard upload code path: write a small payload, assert the file
// lands on disk, assert the FileRecord flag flips, and assert a 404
// for unknown ids.
func TestHandleUploadBillboardTilted(t *testing.T) {
	outputsDir := t.TempDir()
	store := NewFileStore()

	const id = "asset-tilted-1"
	store.Add(&FileRecord{ID: id, Filename: id + ".glb", Status: StatusPending})

	handler := handleUploadBillboardTilted(store, outputsDir)

	// Happy path.
	payload := []byte("not a real glb but enough bytes to write")
	req := httptest.NewRequest(http.MethodPost, "/api/upload-billboard-tilted/"+id, bytes.NewReader(payload))
	rr := httptest.NewRecorder()
	handler(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	want := filepath.Join(outputsDir, id+"_billboard_tilted.glb")
	got, err := os.ReadFile(want)
	if err != nil {
		t.Fatalf("expected file at %s: %v", want, err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("file contents mismatch: got %q want %q", got, payload)
	}

	rec, ok := store.Get(id)
	if !ok {
		t.Fatalf("record disappeared from store")
	}
	if !rec.HasBillboardTilted {
		t.Errorf("HasBillboardTilted not set")
	}

	// 404 for unknown id.
	req404 := httptest.NewRequest(http.MethodPost, "/api/upload-billboard-tilted/nope", bytes.NewReader(payload))
	rr404 := httptest.NewRecorder()
	handler(rr404, req404)
	if rr404.Code != http.StatusNotFound {
		t.Errorf("expected 404 for unknown id, got %d", rr404.Code)
	}

	// Wrong method.
	reqWrong := httptest.NewRequest(http.MethodGet, "/api/upload-billboard-tilted/"+id, nil)
	rrWrong := httptest.NewRecorder()
	handler(rrWrong, reqWrong)
	if rrWrong.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", rrWrong.Code)
	}
}
