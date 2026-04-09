package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

// TestHandleUpload_AppendsManifest covers the T-012-04 happy path:
// posting a multipart upload should leave a record in the
// uploads.jsonl that maps the assigned hash → original filename.
//
// The test does not validate GLB content; the upload pipeline accepts
// any *.glb-suffixed payload and the auto-classifier failure for a
// 4-byte payload is intentionally swallowed by handleUpload.
func TestHandleUpload_AppendsManifest(t *testing.T) {
	resetManifestCache()

	originalsDir := t.TempDir()
	settingsDir := t.TempDir()
	manifestPath := filepath.Join(t.TempDir(), "uploads.jsonl")
	store := NewFileStore()

	body, contentType := buildUploadBody(t, "Achillea Millefolium.glb", []byte("fake glb bytes"))
	req := httptest.NewRequest(http.MethodPost, "/api/upload", body)
	req.Header.Set("Content-Type", contentType)
	rr := httptest.NewRecorder()

	handleUpload(store, originalsDir, settingsDir, manifestPath, nil)(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}

	var records []FileRecord
	if err := json.Unmarshal(rr.Body.Bytes(), &records); err != nil {
		t.Fatalf("decode response: %v body=%s", err, rr.Body.String())
	}
	if len(records) != 1 {
		t.Fatalf("records = %d, want 1", len(records))
	}
	hash := records[0].ID

	// Manifest should now have one record matching that hash.
	got, err := LookupUploadFilename(manifestPath, hash)
	if err != nil {
		t.Fatalf("lookup after upload: %v", err)
	}
	if got != "Achillea Millefolium.glb" {
		t.Errorf("filename = %q, want Achillea Millefolium.glb", got)
	}
}

// TestHandleUpload_ManifestWriteFailureIsNonFatal points the manifest
// path at a path the OS cannot append to (a directory). The upload
// handler must still return 200 — the manifest is best-effort.
func TestHandleUpload_ManifestWriteFailureIsNonFatal(t *testing.T) {
	resetManifestCache()

	originalsDir := t.TempDir()
	settingsDir := t.TempDir()
	// Use a directory as the manifest path so OpenFile fails.
	badPath := t.TempDir()
	store := NewFileStore()

	body, ctype := buildUploadBody(t, "rosa_canina.glb", []byte("xxxx"))
	req := httptest.NewRequest(http.MethodPost, "/api/upload", body)
	req.Header.Set("Content-Type", ctype)
	rr := httptest.NewRecorder()

	handleUpload(store, originalsDir, settingsDir, badPath, nil)(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s — manifest failure must not fail upload",
			rr.Code, rr.Body.String())
	}
	// And the file itself should still be on disk.
	entries, _ := os.ReadDir(originalsDir)
	if len(entries) != 1 {
		t.Errorf("originalsDir entries = %d, want 1 (.glb saved)", len(entries))
	}
}

// TestHandleUpload_RestartScenario uploads a file, drops the in-
// memory cache to simulate a server restart, then verifies the
// original filename is recoverable from the on-disk manifest. This is
// the integration AC: provenance survives restart.
func TestHandleUpload_RestartScenario(t *testing.T) {
	resetManifestCache()

	originalsDir := t.TempDir()
	settingsDir := t.TempDir()
	manifestPath := filepath.Join(t.TempDir(), "uploads.jsonl")
	store := NewFileStore()

	body, ctype := buildUploadBody(t, "salvia_officinalis.glb", []byte("data"))
	req := httptest.NewRequest(http.MethodPost, "/api/upload", body)
	req.Header.Set("Content-Type", ctype)
	rr := httptest.NewRecorder()
	handleUpload(store, originalsDir, settingsDir, manifestPath, nil)(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("upload status = %d", rr.Code)
	}
	var records []FileRecord
	if err := json.Unmarshal(rr.Body.Bytes(), &records); err != nil {
		t.Fatalf("decode: %v", err)
	}
	hash := records[0].ID

	// Simulate restart: blow away the in-memory FileStore + manifest
	// cache, then look up using only the on-disk manifest.
	resetManifestCache()

	got, err := LookupUploadFilename(manifestPath, hash)
	if err != nil {
		t.Fatalf("lookup after restart: %v", err)
	}
	if got != "salvia_officinalis.glb" {
		t.Errorf("filename = %q, want salvia_officinalis.glb", got)
	}

	// Sanity: a hash that was never uploaded must not produce a
	// false positive after the cache reset.
	if _, err := LookupUploadFilename(manifestPath, "deadbeefdeadbeefdeadbeefdeadbeef"); !errors.Is(err, ErrManifestNotFound) {
		t.Errorf("missing hash err = %v, want ErrManifestNotFound", err)
	}
}

// buildUploadBody constructs a multipart request body that
// handleUpload will accept (one part named "files" with the given
// filename + payload).
func buildUploadBody(t *testing.T, filename string, payload []byte) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("files", filename)
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write(payload); err != nil {
		t.Fatalf("part.Write: %v", err)
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("mw.Close: %v", err)
	}
	return &buf, mw.FormDataContentType()
}
