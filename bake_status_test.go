package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverAllIDs(t *testing.T) {
	dir := t.TempDir()
	// Asset "aaa...": has billboard + tilted + volumetric + bake.json
	touch(t, dir, "aaaa1234_billboard.glb")
	touch(t, dir, "aaaa1234_billboard_tilted.glb")
	touch(t, dir, "aaaa1234_volumetric.glb")
	touch(t, dir, "aaaa1234_bake.json")
	touch(t, dir, "aaaa1234.glb")
	// Asset "bbbb...": only has the base .glb (mid-bake)
	touch(t, dir, "bbbb5678.glb")
	// Asset "cccc...": has billboard only
	touch(t, dir, "cccc9012_billboard.glb")
	// Non-matching file (should be ignored)
	touch(t, dir, "README.md")

	ids, err := discoverAllIDs(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"aaaa1234", "bbbb5678", "cccc9012"}
	if len(ids) != len(want) {
		t.Fatalf("got %d ids %v, want %d %v", len(ids), ids, len(want), want)
	}
	for i, id := range ids {
		if id != want[i] {
			t.Errorf("ids[%d] = %q, want %q", i, id, want[i])
		}
	}
}

func TestDiscoverAllIDs_Empty(t *testing.T) {
	dir := t.TempDir()
	ids, err := discoverAllIDs(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 0 {
		t.Fatalf("expected empty, got %v", ids)
	}
}

func TestCheckIntermediates(t *testing.T) {
	dir := t.TempDir()
	id := "deadbeef"
	touch(t, dir, id+"_billboard.glb")
	touch(t, dir, id+"_volumetric.glb")
	// no _billboard_tilted.glb

	billboard, tilted, dome := checkIntermediates(dir, id)
	if !billboard {
		t.Error("expected billboard=true")
	}
	if tilted {
		t.Error("expected tilted=false")
	}
	if !dome {
		t.Error("expected dome=true")
	}
}

func TestCheckIntermediates_None(t *testing.T) {
	dir := t.TempDir()
	billboard, tilted, dome := checkIntermediates(dir, "nonexistent")
	if billboard || tilted || dome {
		t.Errorf("expected all false, got billboard=%v tilted=%v dome=%v", billboard, tilted, dome)
	}
}

func touch(t *testing.T, dir, name string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte{}, 0644); err != nil {
		t.Fatal(err)
	}
}
