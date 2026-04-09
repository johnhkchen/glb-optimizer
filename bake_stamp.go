package main

// bake_stamp.go owns the on-disk {id}_bake.json file written when
// "Build hybrid impostor" finishes. The file is the single source of
// truth for PackMeta.BakeID — the stable handle the future asset
// server will use as a cache-busting URL component. See T-011-03.

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// bakeStamp is the on-disk shape of {outputsDir}/{id}_bake.json. Both
// fields hold an RFC3339 UTC timestamp captured at bake completion.
// They are separate keys because "the id" and "when the bake
// finished" are conceptually distinct, even if they coincide in v1.
type bakeStamp struct {
	BakeID      string `json:"bake_id"`
	CompletedAt string `json:"completed_at"`
}

// bakeStampPath returns {outputsDir}/{id}_bake.json — the canonical
// file location used by both the writer and the reader.
func bakeStampPath(outputsDir, id string) string {
	return filepath.Join(outputsDir, id+"_bake.json")
}

// WriteBakeStamp creates/overwrites {outputsDir}/{id}_bake.json with
// a fresh RFC3339 UTC timestamp in BOTH fields, captured exactly
// once. Returns the bake_id it wrote so callers can echo it. Uses
// atomic temp+rename so concurrent readers never see a partial file.
func WriteBakeStamp(outputsDir, id string) (string, error) {
	now := time.Now().UTC().Format(time.RFC3339)
	stamp := bakeStamp{BakeID: now, CompletedAt: now}

	data, err := json.MarshalIndent(stamp, "", "  ")
	if err != nil {
		return "", fmt.Errorf("bake_stamp: marshal: %w", err)
	}

	final := bakeStampPath(outputsDir, id)
	tmp := final + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return "", fmt.Errorf("bake_stamp: write %s: %w", tmp, err)
	}
	if err := os.Rename(tmp, final); err != nil {
		_ = os.Remove(tmp)
		return "", fmt.Errorf("bake_stamp: rename %s: %w", final, err)
	}
	return now, nil
}

// ReadBakeStamp reads the bake stamp for an asset. Missing file →
// (zero value, nil) so callers can fall back. Malformed JSON →
// wrapped error.
func ReadBakeStamp(outputsDir, id string) (bakeStamp, error) {
	path := bakeStampPath(outputsDir, id)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return bakeStamp{}, nil
		}
		return bakeStamp{}, fmt.Errorf("bake_stamp: read %s: %w", path, err)
	}
	var stamp bakeStamp
	if err := json.Unmarshal(data, &stamp); err != nil {
		return bakeStamp{}, fmt.Errorf("bake_stamp: decode %s: %w", path, err)
	}
	return stamp, nil
}
