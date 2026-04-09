package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"
)

// stalePackTestEnv builds a tempdir layout with separate outputs/
// and dist/ subdirs and returns absolute paths.
type stalePackTestEnv struct {
	root, outputs, dist string
}

func newStalePackEnv(t *testing.T) stalePackTestEnv {
	t.Helper()
	root := t.TempDir()
	outputs := filepath.Join(root, "outputs")
	dist := filepath.Join(root, "dist")
	if err := os.MkdirAll(outputs, 0755); err != nil {
		t.Fatalf("mkdir outputs: %v", err)
	}
	if err := os.MkdirAll(dist, 0755); err != nil {
		t.Fatalf("mkdir dist: %v", err)
	}
	return stalePackTestEnv{root: root, outputs: outputs, dist: dist}
}

// writeIntermediate drops a synthetic _billboard.glb under outputs/
// so the id becomes discoverable by discoverPackableIDs.
func writeIntermediate(t *testing.T, dir, id string) {
	t.Helper()
	p := filepath.Join(dir, id+"_billboard.glb")
	if err := os.WriteFile(p, []byte("x"), 0644); err != nil {
		t.Fatalf("write intermediate %s: %v", id, err)
	}
}

// writePack drops a synthetic species pack under dist/.
func writePack(t *testing.T, dir, species string) string {
	t.Helper()
	p := filepath.Join(dir, species+".glb")
	if err := os.WriteFile(p, []byte("xyz"), 0644); err != nil {
		t.Fatalf("write pack %s: %v", species, err)
	}
	return p
}

// safeOpts returns ResolverOptions whose UploadManifestPath points
// at a nonexistent path inside a tempdir, so tests never accidentally
// read the developer's real ~/.glb-optimizer/uploads.jsonl.
func safeOpts(t *testing.T) ResolverOptions {
	t.Helper()
	return ResolverOptions{
		UploadManifestPath: filepath.Join(t.TempDir(), "uploads-not-present.jsonl"),
	}
}

func TestIdentifyStalePacks_AllLive(t *testing.T) {
	env := newStalePackEnv(t)
	for _, id := range []string{"alpha", "beta", "gamma"} {
		writeIntermediate(t, env.outputs, id)
		writePack(t, env.dist, id) // species == id for non-hex ids
	}
	stale, err := IdentifyStalePacks(env.dist, env.outputs, nil, safeOpts(t))
	if err != nil {
		t.Fatalf("identify: %v", err)
	}
	if len(stale) != 0 {
		t.Fatalf("expected empty stale list, got %v", stale)
	}
}

func TestIdentifyStalePacks_AllStale(t *testing.T) {
	env := newStalePackEnv(t)
	writePack(t, env.dist, "orphan_one")
	writePack(t, env.dist, "orphan_two")
	writePack(t, env.dist, "orphan_three")

	stale, err := IdentifyStalePacks(env.dist, env.outputs, nil, safeOpts(t))
	if err != nil {
		t.Fatalf("identify: %v", err)
	}
	if len(stale) != 3 {
		t.Fatalf("expected 3 stale, got %d: %v", len(stale), stale)
	}
	got := make([]string, len(stale))
	for i, p := range stale {
		got[i] = filepath.Base(p)
	}
	sort.Strings(got)
	want := []string{"orphan_one.glb", "orphan_three.glb", "orphan_two.glb"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestIdentifyStalePacks_Mixed(t *testing.T) {
	env := newStalePackEnv(t)
	writeIntermediate(t, env.outputs, "live_alpha")
	writeIntermediate(t, env.outputs, "live_beta")

	writePack(t, env.dist, "live_alpha")
	writePack(t, env.dist, "stale_one")
	writePack(t, env.dist, "stale_two")
	writePack(t, env.dist, "live_beta")

	stale, err := IdentifyStalePacks(env.dist, env.outputs, nil, safeOpts(t))
	if err != nil {
		t.Fatalf("identify: %v", err)
	}
	got := make([]string, len(stale))
	for i, p := range stale {
		got[i] = filepath.Base(p)
	}
	want := []string{"stale_one.glb", "stale_two.glb"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestIdentifyStalePacks_EmptyDist(t *testing.T) {
	env := newStalePackEnv(t)
	writeIntermediate(t, env.outputs, "alpha")
	stale, err := IdentifyStalePacks(env.dist, env.outputs, nil, safeOpts(t))
	if err != nil {
		t.Fatalf("identify: %v", err)
	}
	if len(stale) != 0 {
		t.Fatalf("expected empty, got %v", stale)
	}
}

func TestIdentifyStalePacks_MissingDist(t *testing.T) {
	env := newStalePackEnv(t)
	missing := filepath.Join(env.root, "no_such_dir")
	stale, err := IdentifyStalePacks(missing, env.outputs, nil, safeOpts(t))
	if err != nil {
		t.Fatalf("expected nil error for missing dist, got %v", err)
	}
	if len(stale) != 0 {
		t.Fatalf("expected empty, got %v", stale)
	}
}

func TestIdentifyStalePacks_IgnoresNonGLB(t *testing.T) {
	env := newStalePackEnv(t)
	writeIntermediate(t, env.outputs, "alpha")
	writePack(t, env.dist, "alpha")
	// Drop noise files that must NOT be classified as stale.
	if err := os.WriteFile(filepath.Join(env.dist, ".DS_Store"), []byte("noise"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(env.dist, "notes.txt"), []byte("noise"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(env.dist, "thumbs"), 0755); err != nil {
		t.Fatal(err)
	}

	stale, err := IdentifyStalePacks(env.dist, env.outputs, nil, safeOpts(t))
	if err != nil {
		t.Fatalf("identify: %v", err)
	}
	if len(stale) != 0 {
		t.Fatalf("expected empty (noise files ignored), got %v", stale)
	}
}

func TestRemoveStalePacks_DryRunLeavesFiles(t *testing.T) {
	env := newStalePackEnv(t)
	a := writePack(t, env.dist, "stale_a")
	b := writePack(t, env.dist, "stale_b")

	var buf bytes.Buffer
	if err := RemoveStalePacks(&buf, []string{a, b}, true); err != nil {
		t.Fatalf("dry-run returned error: %v", err)
	}

	for _, p := range []string{a, b} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("file %s should still exist after dry-run: %v", p, err)
		}
	}
	out := buf.String()
	for _, want := range []string{"dry-run", "stale_a.glb", "stale_b.glb", "TOTAL: 2 stale, 0 removed"} {
		if !strings.Contains(out, want) {
			t.Errorf("dry-run output missing %q\nfull:\n%s", want, out)
		}
	}
}

func TestRemoveStalePacks_ApplyDeletes(t *testing.T) {
	env := newStalePackEnv(t)
	a := writePack(t, env.dist, "stale_a")
	b := writePack(t, env.dist, "stale_b")

	var buf bytes.Buffer
	if err := RemoveStalePacks(&buf, []string{a, b}, false); err != nil {
		t.Fatalf("apply returned error: %v", err)
	}

	for _, p := range []string{a, b} {
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Errorf("file %s should be removed, stat err = %v", p, err)
		}
	}
	out := buf.String()
	for _, want := range []string{"removed: stale_a.glb", "removed: stale_b.glb", "TOTAL: 2 stale, 2 removed"} {
		if !strings.Contains(out, want) {
			t.Errorf("apply output missing %q\nfull:\n%s", want, out)
		}
	}
	if strings.Contains(out, "dry-run") {
		t.Errorf("apply output should not mention dry-run\nfull:\n%s", out)
	}
}

func TestRemoveStalePacks_MissingFileLogsContinues(t *testing.T) {
	env := newStalePackEnv(t)
	good := writePack(t, env.dist, "good")
	missing := filepath.Join(env.dist, "ghost.glb") // never created

	var buf bytes.Buffer
	err := RemoveStalePacks(&buf, []string{missing, good}, false)
	if err == nil {
		t.Fatalf("expected aggregated error for missing file, got nil")
	}
	if !errors.Is(err, os.ErrNotExist) && !strings.Contains(err.Error(), "ghost.glb") {
		t.Errorf("expected error mentioning ghost.glb, got %v", err)
	}

	if _, statErr := os.Stat(good); !os.IsNotExist(statErr) {
		t.Errorf("good file should have been removed despite earlier failure, stat err = %v", statErr)
	}
	out := buf.String()
	if !strings.Contains(out, "FAILED: ghost.glb") {
		t.Errorf("output should log FAILED for missing file:\n%s", out)
	}
	if !strings.Contains(out, "removed: good.glb") {
		t.Errorf("output should log removal of good file:\n%s", out)
	}
}

// TestStalePackCleanup_RoundTrip mirrors the pack-all --clean
// scenario at unit level: a previously-baked species is removed,
// only its pack file lingers in dist/, and IdentifyStalePacks +
// RemoveStalePacks should restore parity. Verifies the live pack
// is preserved.
func TestStalePackCleanup_RoundTrip(t *testing.T) {
	env := newStalePackEnv(t)
	// One live intermediate + matching pack.
	writeIntermediate(t, env.outputs, "still_here")
	livePack := writePack(t, env.dist, "still_here")
	// One stale pack with no matching intermediate (the species was
	// removed before the next pack-all).
	stalePack := writePack(t, env.dist, "removed_species")

	stale, err := IdentifyStalePacks(env.dist, env.outputs, nil, safeOpts(t))
	if err != nil {
		t.Fatalf("identify: %v", err)
	}
	if len(stale) != 1 || filepath.Base(stale[0]) != "removed_species.glb" {
		t.Fatalf("expected one stale (removed_species.glb), got %v", stale)
	}

	var buf bytes.Buffer
	if err := RemoveStalePacks(&buf, stale, false); err != nil {
		t.Fatalf("remove: %v", err)
	}

	if _, err := os.Stat(livePack); err != nil {
		t.Errorf("live pack must survive cleanup: %v", err)
	}
	if _, err := os.Stat(stalePack); !os.IsNotExist(err) {
		t.Errorf("stale pack must be deleted, stat err = %v", err)
	}
}

func TestRemoveStalePacks_EmptyList(t *testing.T) {
	var buf bytes.Buffer
	if err := RemoveStalePacks(&buf, nil, false); err != nil {
		t.Fatalf("empty list returned error: %v", err)
	}
	if got := strings.TrimSpace(buf.String()); got != "No stale packs." {
		t.Errorf("expected 'No stale packs.', got %q", got)
	}
}
