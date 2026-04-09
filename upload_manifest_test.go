package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// resetManifestCache wipes the package-level cache so each test
// starts from a clean slate. Without it, a cached index from an
// earlier test could shadow the on-disk file built by the next.
func resetManifestCache() {
	manifestCache.Lock()
	manifestCache.path = ""
	manifestCache.mtime = time.Time{}
	manifestCache.index = nil
	manifestCache.Unlock()
}

func TestAppendUploadRecord_RoundTrip(t *testing.T) {
	resetManifestCache()
	path := filepath.Join(t.TempDir(), "uploads.jsonl")

	now := time.Date(2026, 4, 8, 19, 30, 0, 0, time.UTC)
	entry := UploadManifestEntry{
		Hash:             "0b5820c3aaf51ee5cff6373ef9565935",
		OriginalFilename: "achillea_millefolium.glb",
		UploadedAt:       now,
		Size:             14266164,
	}
	if err := AppendUploadRecord(path, entry); err != nil {
		t.Fatalf("append: %v", err)
	}

	got, err := LookupUploadFilename(path, entry.Hash)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if got != entry.OriginalFilename {
		t.Errorf("filename = %q, want %q", got, entry.OriginalFilename)
	}

	// Also verify the on-disk JSON has all four fields by re-reading
	// raw — round-trips through scanManifest only check the index map.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read raw: %v", err)
	}
	for _, want := range []string{`"hash"`, `"original_filename"`, `"uploaded_at"`, `"size"`} {
		if !strings.Contains(string(data), want) {
			t.Errorf("raw manifest missing field %s: %s", want, string(data))
		}
	}
}

func TestAppendUploadRecord_LastWriteWins(t *testing.T) {
	resetManifestCache()
	path := filepath.Join(t.TempDir(), "uploads.jsonl")

	if err := AppendUploadRecord(path, UploadManifestEntry{
		Hash: "abc", OriginalFilename: "first.glb",
		UploadedAt: time.Now(), Size: 100,
	}); err != nil {
		t.Fatalf("append 1: %v", err)
	}
	// Bump mtime explicitly so the cache invalidates even on
	// 1-second-granularity filesystems.
	bumpMtime(t, path)
	if err := AppendUploadRecord(path, UploadManifestEntry{
		Hash: "abc", OriginalFilename: "second.glb",
		UploadedAt: time.Now(), Size: 200,
	}); err != nil {
		t.Fatalf("append 2: %v", err)
	}
	bumpMtime(t, path)

	got, err := LookupUploadFilename(path, "abc")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if got != "second.glb" {
		t.Errorf("filename = %q, want second.glb", got)
	}
}

func TestLookupUploadFilename_NotFoundFile(t *testing.T) {
	resetManifestCache()
	path := filepath.Join(t.TempDir(), "does-not-exist.jsonl")

	_, err := LookupUploadFilename(path, "abc")
	if !errors.Is(err, ErrManifestNotFound) {
		t.Errorf("err = %v, want ErrManifestNotFound", err)
	}
}

func TestLookupUploadFilename_NotFoundHash(t *testing.T) {
	resetManifestCache()
	path := filepath.Join(t.TempDir(), "uploads.jsonl")
	if err := AppendUploadRecord(path, UploadManifestEntry{
		Hash: "abc", OriginalFilename: "x.glb",
		UploadedAt: time.Now(), Size: 1,
	}); err != nil {
		t.Fatalf("append: %v", err)
	}

	_, err := LookupUploadFilename(path, "different-hash")
	if !errors.Is(err, ErrManifestNotFound) {
		t.Errorf("err = %v, want ErrManifestNotFound", err)
	}
}

func TestLookupUploadFilename_CacheInvalidatedByMtime(t *testing.T) {
	resetManifestCache()
	path := filepath.Join(t.TempDir(), "uploads.jsonl")

	if err := AppendUploadRecord(path, UploadManifestEntry{
		Hash: "first", OriginalFilename: "a.glb",
		UploadedAt: time.Now(), Size: 1,
	}); err != nil {
		t.Fatalf("append 1: %v", err)
	}
	// First lookup populates the cache.
	if _, err := LookupUploadFilename(path, "first"); err != nil {
		t.Fatalf("lookup 1: %v", err)
	}

	if err := AppendUploadRecord(path, UploadManifestEntry{
		Hash: "second", OriginalFilename: "b.glb",
		UploadedAt: time.Now(), Size: 2,
	}); err != nil {
		t.Fatalf("append 2: %v", err)
	}
	// Force the mtime forward so the cache observes a real change
	// regardless of filesystem timestamp granularity.
	bumpMtime(t, path)

	got, err := LookupUploadFilename(path, "second")
	if err != nil {
		t.Fatalf("lookup 2: %v", err)
	}
	if got != "b.glb" {
		t.Errorf("filename = %q, want b.glb (cache should have re-scanned)", got)
	}
}

func TestScanManifest_SkipsMalformedLines(t *testing.T) {
	resetManifestCache()
	path := filepath.Join(t.TempDir(), "uploads.jsonl")

	// Hand-craft a file that mixes a valid record with garbage in
	// front and a truncated/half-written final line — the partial-
	// crash recovery contract from design.md.
	body := "this is not json\n" +
		`{"hash":"abc","original_filename":"good.glb","uploaded_at":"2026-04-08T00:00:00Z","size":42}` + "\n" +
		`{"hash":"trunc","original_file` // missing newline + truncated
	if err := os.WriteFile(path, []byte(body), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := LookupUploadFilename(path, "abc")
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if got != "good.glb" {
		t.Errorf("filename = %q, want good.glb", got)
	}
	// The truncated line should not have produced an entry.
	if _, err := LookupUploadFilename(path, "trunc"); !errors.Is(err, ErrManifestNotFound) {
		t.Errorf("trunc lookup err = %v, want ErrManifestNotFound", err)
	}
}

func TestAppendUploadRecord_RestartScenario(t *testing.T) {
	// Simulate a server lifecycle: write a record, drop the in-memory
	// cache (as would happen across a process restart), look it up
	// from a "fresh" process via the same on-disk file.
	resetManifestCache()
	path := filepath.Join(t.TempDir(), "uploads.jsonl")

	if err := AppendUploadRecord(path, UploadManifestEntry{
		Hash: "deadbeefcafe", OriginalFilename: "rosa_canina.glb",
		UploadedAt: time.Now().UTC(), Size: 1024,
	}); err != nil {
		t.Fatalf("append: %v", err)
	}

	resetManifestCache() // pretend we restarted

	got, err := LookupUploadFilename(path, "deadbeefcafe")
	if err != nil {
		t.Fatalf("lookup after restart: %v", err)
	}
	if got != "rosa_canina.glb" {
		t.Errorf("filename = %q, want rosa_canina.glb", got)
	}
}

// --- helpers ---

func bumpMtime(t *testing.T, path string) {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	future := info.ModTime().Add(2 * time.Second)
	if err := os.Chtimes(path, future, future); err != nil {
		t.Fatalf("chtimes: %v", err)
	}
}

