package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestWriteBakeStamp_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	id := "asset1"

	written, err := WriteBakeStamp(dir, id)
	if err != nil {
		t.Fatalf("WriteBakeStamp: %v", err)
	}
	if written == "" {
		t.Fatal("WriteBakeStamp returned empty bake_id")
	}

	stamp, err := ReadBakeStamp(dir, id)
	if err != nil {
		t.Fatalf("ReadBakeStamp: %v", err)
	}
	if stamp.BakeID != written {
		t.Errorf("BakeID = %q, want %q", stamp.BakeID, written)
	}
	if stamp.CompletedAt != written {
		t.Errorf("CompletedAt = %q, want %q (same as BakeID)", stamp.CompletedAt, written)
	}
}

func TestWriteBakeStamp_Format(t *testing.T) {
	dir := t.TempDir()
	written, err := WriteBakeStamp(dir, "asset2")
	if err != nil {
		t.Fatalf("WriteBakeStamp: %v", err)
	}
	parsed, err := time.Parse(time.RFC3339, written)
	if err != nil {
		t.Fatalf("returned bake_id %q does not parse as RFC3339: %v", written, err)
	}
	if parsed.Location() != time.UTC {
		t.Errorf("bake_id %q is not UTC", written)
	}

	stamp, err := ReadBakeStamp(dir, "asset2")
	if err != nil {
		t.Fatalf("ReadBakeStamp: %v", err)
	}
	if stamp.BakeID != stamp.CompletedAt {
		t.Errorf("BakeID (%q) and CompletedAt (%q) should be identical at write time",
			stamp.BakeID, stamp.CompletedAt)
	}
}

func TestReadBakeStamp_Missing(t *testing.T) {
	dir := t.TempDir()
	stamp, err := ReadBakeStamp(dir, "nope")
	if err != nil {
		t.Fatalf("expected nil error for missing file, got %v", err)
	}
	if stamp.BakeID != "" || stamp.CompletedAt != "" {
		t.Errorf("expected zero stamp, got %+v", stamp)
	}
}

func TestReadBakeStamp_Malformed(t *testing.T) {
	dir := t.TempDir()
	id := "broken"
	if err := os.WriteFile(filepath.Join(dir, id+"_bake.json"), []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := ReadBakeStamp(dir, id)
	if err == nil {
		t.Fatal("expected error for malformed JSON, got nil")
	}
	if !strings.Contains(err.Error(), "decode") {
		t.Errorf("error %q should mention decode", err)
	}
	if !strings.Contains(err.Error(), id) {
		t.Errorf("error %q should mention asset id", err)
	}
}

func TestWriteBakeStamp_Overwrite(t *testing.T) {
	dir := t.TempDir()
	id := "overwrite"

	first, err := WriteBakeStamp(dir, id)
	if err != nil {
		t.Fatalf("first write: %v", err)
	}
	// Sleep one second so the RFC3339 representation differs.
	time.Sleep(1100 * time.Millisecond)
	second, err := WriteBakeStamp(dir, id)
	if err != nil {
		t.Fatalf("second write: %v", err)
	}
	if first == second {
		t.Fatalf("expected distinct timestamps across writes, both = %q", first)
	}

	stamp, err := ReadBakeStamp(dir, id)
	if err != nil {
		t.Fatalf("ReadBakeStamp: %v", err)
	}
	if stamp.BakeID != second {
		t.Errorf("after overwrite BakeID = %q, want %q", stamp.BakeID, second)
	}
}
