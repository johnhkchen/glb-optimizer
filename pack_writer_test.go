package main

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
)

func TestWritePack_HappyPath(t *testing.T) {
	dir := t.TempDir()
	payload := []byte("glTF\x02\x00\x00\x00fake-pack-bytes")

	if err := WritePack(dir, "fern", payload); err != nil {
		t.Fatalf("WritePack: %v", err)
	}

	final := filepath.Join(dir, "fern.glb")
	got, err := os.ReadFile(final)
	if err != nil {
		t.Fatalf("read final: %v", err)
	}
	if !bytes.Equal(got, payload) {
		t.Errorf("payload mismatch: got %q want %q", got, payload)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 1 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("expected exactly 1 file in dir, got %d: %v", len(entries), names)
	}
}

func TestWritePack_OverwritesExisting(t *testing.T) {
	dir := t.TempDir()
	final := filepath.Join(dir, "moss.glb")

	if err := os.WriteFile(final, []byte("STALE-CONTENT"), 0644); err != nil {
		t.Fatalf("seed: %v", err)
	}

	fresh := []byte("FRESH-PACK-BYTES")
	if err := WritePack(dir, "moss", fresh); err != nil {
		t.Fatalf("WritePack: %v", err)
	}

	got, err := os.ReadFile(final)
	if err != nil {
		t.Fatalf("read final: %v", err)
	}
	if !bytes.Equal(got, fresh) {
		t.Errorf("overwrite failed: got %q want %q", got, fresh)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected exactly 1 file after overwrite, got %d", len(entries))
	}
}

func TestWritePack_CreatesMissingDir(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "deep", "nested", "dist", "plants")

	if _, err := os.Stat(nested); !os.IsNotExist(err) {
		t.Fatalf("precondition: nested dir should not exist, got err=%v", err)
	}

	if err := WritePack(nested, "lichen", []byte("payload")); err != nil {
		t.Fatalf("WritePack: %v", err)
	}

	if _, err := os.Stat(filepath.Join(nested, "lichen.glb")); err != nil {
		t.Errorf("expected lichen.glb in created dir: %v", err)
	}
}

func TestWritePack_EmptySpeciesError(t *testing.T) {
	dir := t.TempDir()
	if err := WritePack(dir, "", []byte("x")); err == nil {
		t.Fatal("expected error for empty species, got nil")
	}
}
