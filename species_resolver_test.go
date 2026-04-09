package main

import (
	"bytes"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// silenceResolverLog is a small wrapper around log.SetOutput so the
// fall-through and warning paths in ResolveSpeciesIdentity don't
// pollute test output. Returns the captured buffer for tests that
// want to assert on the warning text.
func silenceResolverLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := log.Writer()
	log.SetOutput(&buf)
	t.Cleanup(func() { log.SetOutput(prev) })
	return &buf
}

// resolverFixture stages an empty outputs directory and a fresh
// FileStore. Each test populates only the tiers it cares about.
type resolverFixture struct {
	id         string
	outputsDir string
	store      *FileStore
}

func newResolverFixture(t *testing.T, id string) *resolverFixture {
	t.Helper()
	f := &resolverFixture{
		id:         id,
		outputsDir: t.TempDir(),
		store:      NewFileStore(),
	}
	return f
}

// writeSidecar writes a `_meta.json` to the fixture's outputs dir.
func (f *resolverFixture) writeSidecar(t *testing.T, body string) {
	t.Helper()
	p := filepath.Join(f.outputsDir, f.id+"_meta.json")
	if err := os.WriteFile(p, []byte(body), 0644); err != nil {
		t.Fatalf("write sidecar: %v", err)
	}
}

// putRecord seeds the FileStore tier with a filename for the
// fixture's id.
func (f *resolverFixture) putRecord(t *testing.T, filename string) {
	t.Helper()
	f.store.Add(&FileRecord{ID: f.id, Filename: filename})
}

func TestResolver_CLIOverrideWins(t *testing.T) {
	silenceResolverLog(t)
	f := newResolverFixture(t, "abc123")
	f.writeSidecar(t, `{"species":"sidecar_value","common_name":"Sidecar"}`)
	f.putRecord(t, "filestore_value.glb")
	opts := ResolverOptions{
		CLISpecies:    "achillea_millefolium",
		CLICommonName: "Common Yarrow",
		Mapping: map[string]SpeciesIdentity{
			"abc123": {Species: "mapping_value", CommonName: "Mapping"},
		},
	}
	ident, src, err := ResolveSpeciesIdentity(f.id, f.outputsDir, f.store, opts)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if src != SourceCLIOverride {
		t.Errorf("source = %s, want cli-override", src)
	}
	if ident.Species != "achillea_millefolium" || ident.CommonName != "Common Yarrow" {
		t.Errorf("identity = %+v, want achillea_millefolium / Common Yarrow", ident)
	}
}

func TestResolver_MappingFileBeatsSidecar(t *testing.T) {
	silenceResolverLog(t)
	f := newResolverFixture(t, "abc123")
	f.writeSidecar(t, `{"species":"sidecar_value","common_name":"Sidecar"}`)
	f.putRecord(t, "filestore_value.glb")
	opts := ResolverOptions{
		Mapping: map[string]SpeciesIdentity{
			"abc123": {Species: "rosa_mundi", CommonName: "Rosa Mundi"},
		},
	}
	ident, src, _ := ResolveSpeciesIdentity(f.id, f.outputsDir, f.store, opts)
	if src != SourceMappingFile {
		t.Errorf("source = %s, want mapping-file", src)
	}
	if ident.Species != "rosa_mundi" {
		t.Errorf("species = %q, want rosa_mundi", ident.Species)
	}
}

func TestResolver_SidecarBeatsFileStore(t *testing.T) {
	silenceResolverLog(t)
	f := newResolverFixture(t, "abc123")
	f.writeSidecar(t, `{"species":"dahlia_blush","common_name":"Dahlia Blush"}`)
	f.putRecord(t, "wrong_value.glb")
	ident, src, _ := ResolveSpeciesIdentity(f.id, f.outputsDir, f.store, ResolverOptions{})
	if src != SourceMetaJSON {
		t.Errorf("source = %s, want meta-json", src)
	}
	if ident.Species != "dahlia_blush" {
		t.Errorf("species = %q, want dahlia_blush", ident.Species)
	}
}

func TestResolver_FileStoreFallback(t *testing.T) {
	silenceResolverLog(t)
	f := newResolverFixture(t, "abc123")
	f.putRecord(t, "achillea_millefolium.glb")
	ident, src, _ := ResolveSpeciesIdentity(f.id, f.outputsDir, f.store, ResolverOptions{})
	if src != SourceFileStore {
		t.Errorf("source = %s, want file-store", src)
	}
	if ident.Species != "achillea_millefolium" {
		t.Errorf("species = %q, want achillea_millefolium", ident.Species)
	}
	if ident.CommonName != "Achillea Millefolium" {
		t.Errorf("common_name = %q, want Achillea Millefolium", ident.CommonName)
	}
}

func TestResolver_FileStoreSentinelSkipped(t *testing.T) {
	silenceResolverLog(t)
	f := newResolverFixture(t, "abc123")
	// post-restart sentinel: scanExistingFiles writes Filename = id+".glb"
	f.putRecord(t, "abc123.glb")
	ident, src, _ := ResolveSpeciesIdentity(f.id, f.outputsDir, f.store, ResolverOptions{})
	if src != SourceContentHash {
		t.Errorf("source = %s, want content-hash (sentinel should fall through)", src)
	}
	// "abc123" is not a hex hash (has 'g'... wait, abc123 IS hex). Reuse a non-hex id below.
	if ident.Species == "" {
		t.Error("species is empty")
	}
}

func TestResolver_UploadManifestTier(t *testing.T) {
	silenceResolverLog(t)
	f := newResolverFixture(t, "0b5820c3aaf51ee5cff6373ef9565935")
	manifest := filepath.Join(t.TempDir(), "uploads.jsonl")
	body := `{"hash":"other","original_filename":"ignore.glb"}` + "\n" +
		`{"hash":"0b5820c3aaf51ee5cff6373ef9565935","original_filename":"Achillea Millefolium.glb"}` + "\n" +
		`{"hash":"another","original_filename":"x.glb"}` + "\n"
	if err := os.WriteFile(manifest, []byte(body), 0644); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
	opts := ResolverOptions{UploadManifestPath: manifest}
	ident, src, _ := ResolveSpeciesIdentity(f.id, f.outputsDir, f.store, opts)
	if src != SourceUploadManifest {
		t.Errorf("source = %s, want upload-manifest", src)
	}
	if ident.Species != "achillea_millefolium" {
		t.Errorf("species = %q, want achillea_millefolium (normalised from filename)", ident.Species)
	}
}

func TestResolver_UploadManifestLastWins(t *testing.T) {
	silenceResolverLog(t)
	f := newResolverFixture(t, "abc")
	manifest := filepath.Join(t.TempDir(), "uploads.jsonl")
	body := `{"hash":"abc","original_filename":"first.glb"}` + "\n" +
		`{"hash":"abc","original_filename":"renamed_value.glb"}` + "\n"
	if err := os.WriteFile(manifest, []byte(body), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	ident, _, _ := ResolveSpeciesIdentity(f.id, f.outputsDir, f.store,
		ResolverOptions{UploadManifestPath: manifest})
	if ident.Species != "renamed_value" {
		t.Errorf("species = %q, want renamed_value (last entry wins)", ident.Species)
	}
}

func TestResolver_HashFallback_HexId(t *testing.T) {
	buf := silenceResolverLog(t)
	f := newResolverFixture(t, "0b5820c3aaf51ee5cff6373ef9565935")
	// Point at a tempdir manifest path that doesn't exist so the
	// developer's real ~/.glb-optimizer/uploads.jsonl doesn't bleed in.
	opts := ResolverOptions{UploadManifestPath: filepath.Join(t.TempDir(), "missing.jsonl")}
	ident, src, _ := ResolveSpeciesIdentity(f.id, f.outputsDir, f.store, opts)
	if src != SourceContentHash {
		t.Errorf("source = %s, want content-hash", src)
	}
	if ident.Species != "species_0b5820c3" {
		t.Errorf("species = %q, want species_0b5820c3", ident.Species)
	}
	if !strings.Contains(ident.CommonName, "0b5820c3") {
		t.Errorf("common_name = %q, want it to mention the prefix", ident.CommonName)
	}
	if !strings.Contains(buf.String(), "WARNING") {
		t.Errorf("hash fallback should log a WARNING; log was: %s", buf.String())
	}
}

func TestResolver_HashFallback_NonHexId(t *testing.T) {
	silenceResolverLog(t)
	f := newResolverFixture(t, "salvia_officinalis")
	opts := ResolverOptions{UploadManifestPath: filepath.Join(t.TempDir(), "missing.jsonl")}
	ident, src, _ := ResolveSpeciesIdentity(f.id, f.outputsDir, f.store, opts)
	if src != SourceContentHash {
		t.Errorf("source = %s, want content-hash", src)
	}
	if ident.Species != "salvia_officinalis" {
		t.Errorf("species = %q, want salvia_officinalis (id-derived)", ident.Species)
	}
}

func TestResolver_NormalisesMessyMappingValues(t *testing.T) {
	silenceResolverLog(t)
	f := newResolverFixture(t, "abc")
	opts := ResolverOptions{
		Mapping: map[string]SpeciesIdentity{
			"abc": {Species: "Achillea Millefolium!", CommonName: ""},
		},
	}
	ident, src, _ := ResolveSpeciesIdentity(f.id, f.outputsDir, f.store, opts)
	if src != SourceMappingFile {
		t.Errorf("source = %s, want mapping-file", src)
	}
	if ident.Species != "achillea_millefolium" {
		t.Errorf("species = %q, want achillea_millefolium (normalised)", ident.Species)
	}
	if ident.CommonName != "Achillea Millefolium" {
		t.Errorf("common_name = %q, want Achillea Millefolium (titlecased fallback)", ident.CommonName)
	}
}

func TestResolver_CLIOverridePartialFallsThrough(t *testing.T) {
	silenceResolverLog(t)
	f := newResolverFixture(t, "abc")
	f.putRecord(t, "filestore_value.glb")
	// CLISpecies present but CLICommonName empty → CLI tier skipped.
	opts := ResolverOptions{CLISpecies: "achillea_millefolium"}
	_, src, _ := ResolveSpeciesIdentity(f.id, f.outputsDir, f.store, opts)
	if src != SourceFileStore {
		t.Errorf("source = %s, want file-store (CLI tier needs both flags)", src)
	}
}

func TestLoadMappingFile_HappyPath(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "m.json")
	body := `{
		"0b5820c3aaf51ee5cff6373ef9565935": {"species": "achillea_millefolium", "common_name": "Common Yarrow"},
		"deadbeef": {"species": "rosa_mundi", "common_name": "Rosa Mundi"}
	}`
	if err := os.WriteFile(p, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
	m, err := LoadMappingFile(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(m) != 2 {
		t.Errorf("len = %d, want 2", len(m))
	}
	got := m["0b5820c3aaf51ee5cff6373ef9565935"]
	if got.Species != "achillea_millefolium" || got.CommonName != "Common Yarrow" {
		t.Errorf("entry = %+v", got)
	}
}

func TestLoadMappingFile_EmptyPathReturnsNil(t *testing.T) {
	m, err := LoadMappingFile("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if m != nil {
		t.Errorf("expected nil map for empty path, got %v", m)
	}
}

func TestLoadMappingFile_MissingFileIsError(t *testing.T) {
	_, err := LoadMappingFile(filepath.Join(t.TempDir(), "no-such.json"))
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
}

func TestLoadMappingFile_BadJSONIsError(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "m.json")
	if err := os.WriteFile(p, []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := LoadMappingFile(p)
	if err == nil {
		t.Fatal("expected parse error, got nil")
	}
}

func TestResolver_MalformedSidecarFallsThrough(t *testing.T) {
	buf := silenceResolverLog(t)
	f := newResolverFixture(t, "abc")
	f.writeSidecar(t, "{not valid json")
	f.putRecord(t, "achillea_millefolium.glb")
	ident, src, _ := ResolveSpeciesIdentity(f.id, f.outputsDir, f.store, ResolverOptions{})
	if src != SourceFileStore {
		t.Errorf("source = %s, want file-store (malformed sidecar should fall through)", src)
	}
	if ident.Species != "achillea_millefolium" {
		t.Errorf("species = %q", ident.Species)
	}
	if !strings.Contains(buf.String(), "malformed") {
		t.Errorf("expected malformed-sidecar log; got: %s", buf.String())
	}
}

func TestResolverSource_String(t *testing.T) {
	cases := map[ResolverSource]string{
		SourceCLIOverride:    "cli-override",
		SourceMappingFile:    "mapping-file",
		SourceMetaJSON:       "meta-json",
		SourceFileStore:      "file-store",
		SourceUploadManifest: "upload-manifest",
		SourceContentHash:    "content-hash",
		SourceUnknown:        "unknown",
	}
	for src, want := range cases {
		if got := src.String(); got != want {
			t.Errorf("%d: got %q, want %q", src, got, want)
		}
	}
}

// Compile-time guard: silence the unused-import linter for io if a
// future test no longer needs it. (io.Discard is referenced in
// pack_meta_capture_test.go via silenceLog already, but we keep this
// file self-contained.)
var _ = io.Discard
