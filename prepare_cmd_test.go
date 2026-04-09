package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestHashFile(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "test.bin")
	if err := os.WriteFile(path, []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}

	hash, err := hashFile(path)
	if err != nil {
		t.Fatal(err)
	}

	if len(hash) != 32 {
		t.Errorf("expected 32-char hex hash, got %d chars: %q", len(hash), hash)
	}

	// Same content → same hash (idempotent)
	path2 := filepath.Join(tmp, "test2.bin")
	if err := os.WriteFile(path2, []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}
	hash2, err := hashFile(path2)
	if err != nil {
		t.Fatal(err)
	}
	if hash != hash2 {
		t.Errorf("same content should produce same hash: %q != %q", hash, hash2)
	}

	// Different content → different hash
	path3 := filepath.Join(tmp, "test3.bin")
	if err := os.WriteFile(path3, []byte("goodbye world"), 0644); err != nil {
		t.Fatal(err)
	}
	hash3, err := hashFile(path3)
	if err != nil {
		t.Fatal(err)
	}
	if hash == hash3 {
		t.Error("different content should produce different hash")
	}
}

func TestHashFile_NotFound(t *testing.T) {
	_, err := hashFile("/nonexistent/file.glb")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestSpeciesFromFilename(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"dahlia_blush.glb", "dahlia_blush"},
		{"My Plant (v2).glb", "my_plant_v2"},
		{"hello-world.glb", "hello_world"},
		{"UPPER_CASE.glb", "upper_case"},
		{" spaced name .glb", "spaced_name"},
		{"a.b.c.glb", "abc"},
		{".glb", "unknown"},
		{"path/to/dahlia_blush.glb", "dahlia_blush"},
		{"a_really_long_species_name_that_exceeds_the_sixty_four_character_limit_by_a_lot.glb", "a_really_long_species_name_that_exceeds_the_sixty_four_character"},
	}

	for _, tt := range tests {
		got := speciesFromFilename(tt.input)
		if got != tt.want {
			t.Errorf("speciesFromFilename(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestFormatSize(t *testing.T) {
	tests := []struct {
		input int64
		want  string
	}{
		{500, "500 B"},
		{1024, "1.0 KB"},
		{1536, "1.5 KB"},
		{1048576, "1.0 MB"},
		{28000000, "26.7 MB"},
	}
	for _, tt := range tests {
		got := formatSize(tt.input)
		if got != tt.want {
			t.Errorf("formatSize(%d) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestPrintPrepareSummary_Success(t *testing.T) {
	r := prepareResult{
		Source:        "inbox/dahlia_blush.glb",
		ID:            "a7c19366abcdef01",
		Species:       "dahlia_blush",
		Category:      "round-bush",
		SourceSize:    28000000,
		OptimizedSize: 6800000,
		BillboardSize: 1800000,
		PackSize:      2400000,
		PackPath:      "dist/plants/dahlia_blush.glb",
		Verified:      true,
		DurationMS:    47000,
		Status:        "ok",
	}

	var buf bytes.Buffer
	printPrepareSummary(&buf, r)
	out := buf.String()

	for _, want := range []string{"✓ dahlia_blush.glb", "26.7 MB", "verified", "47s"} {
		if !bytes.Contains([]byte(out), []byte(want)) {
			t.Errorf("summary missing %q:\n%s", want, out)
		}
	}
}

func TestPrintPrepareSummary_Failure(t *testing.T) {
	r := prepareResult{
		Source:     "inbox/bad.glb",
		Status:     "failed",
		FailedStep: "optimize",
		Error:      "gltfpack crashed",
	}

	var buf bytes.Buffer
	printPrepareSummary(&buf, r)
	out := buf.String()

	for _, want := range []string{"✗ bad.glb", "optimize", "gltfpack crashed"} {
		if !bytes.Contains([]byte(out), []byte(want)) {
			t.Errorf("summary missing %q:\n%s", want, out)
		}
	}
}

func TestPrintPrepareJSON(t *testing.T) {
	r := prepareResult{
		Source:   "inbox/test.glb",
		ID:       "abc123",
		Species:  "test_plant",
		Status:   "ok",
		PackSize: 1234,
	}

	var buf bytes.Buffer
	printPrepareJSON(&buf, r)

	var parsed prepareResult
	if err := json.Unmarshal(buf.Bytes(), &parsed); err != nil {
		t.Fatalf("JSON output not parseable: %v\nOutput: %s", err, buf.String())
	}
	if parsed.Status != "ok" {
		t.Errorf("status = %q, want ok", parsed.Status)
	}
	if parsed.Species != "test_plant" {
		t.Errorf("species = %q, want test_plant", parsed.Species)
	}
}

func TestRunPrepare_MissingSource(t *testing.T) {
	r := runPrepare("/nonexistent/file.glb", prepareOptions{
		workDir: t.TempDir(),
	})
	if r.Status != "failed" {
		t.Errorf("expected failed status, got %q", r.Status)
	}
	if r.FailedStep != "copy" {
		t.Errorf("expected failed step 'copy', got %q", r.FailedStep)
	}
}

func TestRunPrepareCmd_NoArgs(t *testing.T) {
	exit := runPrepareCmd(nil)
	if exit != 2 {
		t.Errorf("expected exit code 2, got %d", exit)
	}
}

func TestRunPrepareCmd_MissingFile(t *testing.T) {
	exit := runPrepareCmd([]string{"/nonexistent/file.glb"})
	if exit != 2 {
		t.Errorf("expected exit code 2, got %d", exit)
	}
}

func TestRunPrepareAllCmd_NoArgs(t *testing.T) {
	exit := runPrepareAllCmd(nil)
	if exit != 2 {
		t.Errorf("expected exit code 2, got %d", exit)
	}
}

func TestRunPrepareAllCmd_EmptyDir(t *testing.T) {
	tmp := t.TempDir()
	exit := runPrepareAllCmd([]string{tmp})
	if exit != 0 {
		t.Errorf("expected exit code 0 for empty dir, got %d", exit)
	}
}
