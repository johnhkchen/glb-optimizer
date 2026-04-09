package main

import (
	"fmt"
	"os"
	"path/filepath"
)

// DistPlantsDir is the relative subpath, under workDir, where finished
// asset packs are written. The directory is the USB-drop source for
// the demo laptop. main.go composes it with workDir at startup.
const DistPlantsDir = "dist/plants"

// WritePack atomically writes a finished pack GLB to
// distDir/{species}.glb, creating distDir if missing. The write goes
// through a sibling .tmp file followed by os.Rename so a crashed
// process never leaves a half-written .glb on the USB-drop directory.
//
// distDir is passed in (rather than read from DistPlantsDir directly)
// because callers — handler wiring and the upcoming pack-all CLI —
// already hold the absolute, workDir-rooted form, and tests want a
// hermetic t.TempDir() without process-global state.
func WritePack(distDir, species string, pack []byte) error {
	if species == "" {
		return fmt.Errorf("WritePack: empty species")
	}
	if err := os.MkdirAll(distDir, 0755); err != nil {
		return fmt.Errorf("WritePack: mkdir %s: %w", distDir, err)
	}
	return writeAtomic(filepath.Join(distDir, species+".glb"), pack)
}
