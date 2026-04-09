package main

import (
	"bufio"
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// fixtureAccepted returns a valid AcceptedSettings for use as a test
// starting point.
func fixtureAccepted(id string) *AcceptedSettings {
	return &AcceptedSettings{
		SchemaVersion: AcceptedSchemaVersion,
		AssetID:       id,
		AcceptedAt:    "2026-04-07T12:00:00Z",
		Comment:       "ship it",
		ThumbnailPath: "accepted/thumbs/" + id + ".jpg",
		Settings:      DefaultSettings(),
	}
}

func TestAcceptedRoundtrip(t *testing.T) {
	dir := t.TempDir()
	a := fixtureAccepted("asset-abc")
	a.Settings.BakeExposure = 1.5
	a.Settings.VolumetricLayers = 6

	if err := SaveAccepted(a, dir); err != nil {
		t.Fatalf("SaveAccepted: %v", err)
	}
	loaded, err := LoadAccepted("asset-abc", dir)
	if err != nil {
		t.Fatalf("LoadAccepted: %v", err)
	}
	if !reflect.DeepEqual(a, loaded) {
		t.Errorf("round-trip mismatch:\n  got:  %+v\n  want: %+v", loaded, a)
	}
}

func TestSaveAccepted_StampsAcceptedAtIfEmpty(t *testing.T) {
	dir := t.TempDir()
	a := fixtureAccepted("asset-stamp")
	a.AcceptedAt = ""
	if err := SaveAccepted(a, dir); err != nil {
		t.Fatalf("SaveAccepted: %v", err)
	}
	if a.AcceptedAt == "" {
		t.Errorf("AcceptedAt was not stamped")
	}
}

func TestAcceptedValidate_RejectsBadSettings(t *testing.T) {
	a := fixtureAccepted("asset-bad")
	a.Settings.VolumetricLayers = 9999
	if err := a.Validate(); err == nil {
		t.Errorf("expected validation error for bad settings")
	}
}

func TestAcceptedValidate_RejectsNilSettings(t *testing.T) {
	a := fixtureAccepted("asset-nil")
	a.Settings = nil
	if err := a.Validate(); err == nil {
		t.Errorf("expected validation error for nil settings")
	}
}

func TestAcceptedValidate_RejectsBadSchemaVersion(t *testing.T) {
	a := fixtureAccepted("asset-ver")
	a.SchemaVersion = 999
	if err := a.Validate(); err == nil {
		t.Errorf("expected validation error for bad schema version")
	}
}

func TestAcceptedValidate_RejectsOversizedComment(t *testing.T) {
	a := fixtureAccepted("asset-cmt")
	a.Comment = strings.Repeat("x", acceptedCommentMaxLen+1)
	if err := a.Validate(); err == nil {
		t.Errorf("expected validation error for oversized comment")
	}
}

func TestAcceptedValidate_RejectsEmptyAssetID(t *testing.T) {
	a := fixtureAccepted("")
	if err := a.Validate(); err == nil {
		t.Errorf("expected validation error for empty asset_id")
	}
}

func TestLoadAccepted_MissingReturnsNotExist(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadAccepted("nope", dir)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("expected fs.ErrNotExist, got %v", err)
	}
}

func TestSaveAccepted_Overwrite(t *testing.T) {
	dir := t.TempDir()
	a := fixtureAccepted("asset-ow")
	a.Comment = "first"
	if err := SaveAccepted(a, dir); err != nil {
		t.Fatalf("first save: %v", err)
	}
	a.Comment = "second"
	if err := SaveAccepted(a, dir); err != nil {
		t.Fatalf("second save: %v", err)
	}
	loaded, err := LoadAccepted("asset-ow", dir)
	if err != nil {
		t.Fatalf("LoadAccepted: %v", err)
	}
	if loaded.Comment != "second" {
		t.Errorf("overwrite did not stick: got %q", loaded.Comment)
	}
}

func TestWriteThumbnail_LandsAtPath(t *testing.T) {
	dir := t.TempDir()
	thumbsDir := filepath.Join(dir, "thumbs")
	bytesIn := []byte{0xff, 0xd8, 0xff, 0xe0} // JPEG SOI + APP0 marker
	if err := WriteThumbnail("asset-x", thumbsDir, bytesIn); err != nil {
		t.Fatalf("WriteThumbnail: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(thumbsDir, "asset-x.jpg"))
	if err != nil {
		t.Fatalf("read thumb: %v", err)
	}
	if !bytes.Equal(got, bytesIn) {
		t.Errorf("thumb bytes mismatch: got %v want %v", got, bytesIn)
	}
}

func TestWriteThumbnail_RejectsEmpty(t *testing.T) {
	if err := WriteThumbnail("x", t.TempDir(), nil); err == nil {
		t.Errorf("expected error on empty bytes")
	}
}

func TestAcceptedExists_TrueAfterSave(t *testing.T) {
	dir := t.TempDir()
	a := fixtureAccepted("asset-ex")
	if AcceptedExists("asset-ex", dir) {
		t.Errorf("AcceptedExists true before save")
	}
	if err := SaveAccepted(a, dir); err != nil {
		t.Fatalf("SaveAccepted: %v", err)
	}
	if !AcceptedExists("asset-ex", dir) {
		t.Errorf("AcceptedExists false after save")
	}
}

// ── Handler tests ──

// acceptHandlerHarness sets up a temporary workdir layout, a FileStore
// containing one file, a saved settings file for that id, and an
// AnalyticsLogger writing into the temp tuning dir. Returns the harness
// pieces the individual tests need.
type acceptHarness struct {
	id           string
	workDir      string
	settingsDir  string
	acceptedDir  string
	thumbsDir    string
	tuningDir    string
	store        *FileStore
	logger       *AnalyticsLogger
}

func newAcceptHarness(t *testing.T) *acceptHarness {
	t.Helper()
	work := t.TempDir()
	h := &acceptHarness{
		id:          "abc123",
		workDir:     work,
		settingsDir: filepath.Join(work, "settings"),
		acceptedDir: filepath.Join(work, "accepted"),
		thumbsDir:   filepath.Join(work, "accepted", "thumbs"),
		tuningDir:   filepath.Join(work, "tuning"),
	}
	for _, d := range []string{h.settingsDir, h.acceptedDir, h.thumbsDir, h.tuningDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	h.store = NewFileStore()
	h.store.Add(&FileRecord{
		ID:       h.id,
		Filename: "asset.glb",
		Status:   StatusDone,
	})
	if err := SaveSettings(h.id, h.settingsDir, DefaultSettings()); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	h.logger = NewAnalyticsLogger(h.tuningDir)
	return h
}

func (h *acceptHarness) handler() http.HandlerFunc {
	return handleAccept(h.store, h.settingsDir, h.acceptedDir, h.thumbsDir, h.logger)
}

// fakeJPEG returns a minimal byte sequence the handler will accept as a
// thumbnail. The handler does not validate JPEG structure.
func fakeJPEG() []byte {
	return []byte{0xff, 0xd8, 0xff, 0xe0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}
}

func TestHandleAccept_GetMissingReturns404(t *testing.T) {
	h := newAcceptHarness(t)
	req := httptest.NewRequest(http.MethodGet, "/api/accept/"+h.id, nil)
	w := httptest.NewRecorder()
	h.handler()(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", w.Code)
	}
}

func TestHandleAccept_PostHappyPath(t *testing.T) {
	h := newAcceptHarness(t)

	body := map[string]string{
		"comment":       "ship it",
		"thumbnail_b64": base64.StdEncoding.EncodeToString(fakeJPEG()),
	}
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/accept/"+h.id, bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.handler()(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("got %d, want 200; body=%s", w.Code, w.Body.String())
	}

	var got AcceptedSettings
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.AssetID != h.id {
		t.Errorf("response asset_id = %q want %q", got.AssetID, h.id)
	}
	if got.Comment != "ship it" {
		t.Errorf("response comment = %q", got.Comment)
	}
	if got.ThumbnailPath == "" {
		t.Errorf("response thumbnail_path empty")
	}
	if got.Settings == nil || got.Settings.SchemaVersion != SettingsSchemaVersion {
		t.Errorf("response settings missing or wrong schema_version")
	}

	// File on disk
	if !AcceptedExists(h.id, h.acceptedDir) {
		t.Errorf("accepted file not on disk")
	}
	thumbBytes, err := os.ReadFile(AcceptedThumbPath(h.id, h.thumbsDir))
	if err != nil {
		t.Errorf("read thumb: %v", err)
	}
	if !bytes.Equal(thumbBytes, fakeJPEG()) {
		t.Errorf("thumb bytes mismatch")
	}

	// FileRecord flag flipped
	rec, _ := h.store.Get(h.id)
	if !rec.IsAccepted {
		t.Errorf("FileRecord.IsAccepted not flipped")
	}

	// Analytics: exactly one accept event in the (single) session JSONL.
	entries, _ := os.ReadDir(h.tuningDir)
	var jsonlPath string
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), ".jsonl") {
			jsonlPath = filepath.Join(h.tuningDir, e.Name())
			break
		}
	}
	if jsonlPath == "" {
		t.Fatalf("no session jsonl was created")
	}
	f, err := os.Open(jsonlPath)
	if err != nil {
		t.Fatalf("open jsonl: %v", err)
	}
	defer f.Close()
	var acceptCount int
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var ev Event
		if err := json.Unmarshal(sc.Bytes(), &ev); err != nil {
			t.Errorf("bad json line: %v", err)
			continue
		}
		if ev.EventType == "accept" {
			acceptCount++
			if ev.AssetID != h.id {
				t.Errorf("accept event asset_id = %q want %q", ev.AssetID, h.id)
			}
			if _, ok := ev.Payload["settings"]; !ok {
				t.Errorf("accept event payload missing settings")
			}
			if _, ok := ev.Payload["thumbnail_path"]; !ok {
				t.Errorf("accept event payload missing thumbnail_path")
			}
		}
	}
	if acceptCount != 1 {
		t.Errorf("expected exactly 1 accept event, got %d", acceptCount)
	}
}

func TestHandleAccept_PostUnknownIDReturns404(t *testing.T) {
	h := newAcceptHarness(t)
	buf, _ := json.Marshal(map[string]string{"comment": "x", "thumbnail_b64": ""})
	req := httptest.NewRequest(http.MethodPost, "/api/accept/missing", bytes.NewReader(buf))
	w := httptest.NewRecorder()
	h.handler()(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("got %d, want 404", w.Code)
	}
}

func TestHandleAccept_PostBadJSONReturns400(t *testing.T) {
	h := newAcceptHarness(t)
	req := httptest.NewRequest(http.MethodPost, "/api/accept/"+h.id, strings.NewReader("not json"))
	w := httptest.NewRecorder()
	h.handler()(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400", w.Code)
	}
}

func TestHandleAccept_PostEmptyThumbnailIsOK(t *testing.T) {
	h := newAcceptHarness(t)
	buf, _ := json.Marshal(map[string]string{"comment": "no thumb", "thumbnail_b64": ""})
	req := httptest.NewRequest(http.MethodPost, "/api/accept/"+h.id, bytes.NewReader(buf))
	w := httptest.NewRecorder()
	h.handler()(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("got %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var got AcceptedSettings
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got.ThumbnailPath != "" {
		t.Errorf("expected empty thumbnail_path, got %q", got.ThumbnailPath)
	}
	// And no thumb file should exist.
	if _, err := os.Stat(AcceptedThumbPath(h.id, h.thumbsDir)); !os.IsNotExist(err) {
		t.Errorf("thumb file should not exist; stat err = %v", err)
	}
}

func TestHandleAccept_PostOversizedThumbnailReturns400(t *testing.T) {
	h := newAcceptHarness(t)
	huge := make([]byte, (2<<20)+1) // 2 MB + 1
	body := map[string]string{
		"comment":       "",
		"thumbnail_b64": base64.StdEncoding.EncodeToString(huge),
	}
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/accept/"+h.id, bytes.NewReader(buf))
	w := httptest.NewRecorder()
	h.handler()(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("got %d, want 400; body=%s", w.Code, w.Body.String())
	}
}

func TestHandleAccept_GetAfterPostReturnsSnapshot(t *testing.T) {
	h := newAcceptHarness(t)
	body := map[string]string{"comment": "first take", "thumbnail_b64": ""}
	buf, _ := json.Marshal(body)
	req := httptest.NewRequest(http.MethodPost, "/api/accept/"+h.id, bytes.NewReader(buf))
	w := httptest.NewRecorder()
	h.handler()(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("post failed: %d", w.Code)
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/accept/"+h.id, nil)
	w2 := httptest.NewRecorder()
	h.handler()(w2, req2)
	if w2.Code != http.StatusOK {
		t.Fatalf("get failed: %d", w2.Code)
	}
	var got AcceptedSettings
	if err := json.Unmarshal(w2.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode get: %v", err)
	}
	if got.Comment != "first take" {
		t.Errorf("get comment = %q want %q", got.Comment, "first take")
	}
}
