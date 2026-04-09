package main

import (
	"os"
	"os/exec"
	"testing"
)

// TestRunClassifier_Rose runs the classifier against the canonical
// rose asset. Skips when python3 or the asset is missing — the unit
// suite must remain runnable in environments without the optional
// Python dependency.
func TestRunClassifier_Rose(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not on PATH")
	}
	const path = "assets/rose_julia_child.glb"
	if _, err := os.Stat(path); err != nil {
		t.Skipf("test asset missing: %s", path)
	}
	res, err := RunClassifier(path)
	if err != nil {
		t.Fatalf("RunClassifier: %v", err)
	}
	if res.Category != "round-bush" {
		t.Errorf("category = %q, want %q", res.Category, "round-bush")
	}
	if res.Confidence < 0.5 || res.Confidence > 1.0 {
		t.Errorf("confidence = %v, expected reasonable [0.5, 1.0]", res.Confidence)
	}
	if res.IsHardSurface {
		t.Error("rose should not be hard-surface")
	}
	if res.Features == nil {
		t.Error("features should be populated")
	}
}

// TestRunClassifier_Bed sanity-checks the planar/HS classification on
// the wood raised bed. Same skip rules.
func TestRunClassifier_Bed(t *testing.T) {
	if _, err := exec.LookPath("python3"); err != nil {
		t.Skip("python3 not on PATH")
	}
	const path = "assets/wood_raised_bed.glb"
	if _, err := os.Stat(path); err != nil {
		t.Skipf("test asset missing: %s", path)
	}
	res, err := RunClassifier(path)
	if err != nil {
		t.Fatalf("RunClassifier: %v", err)
	}
	if res.Category != "planar" {
		t.Errorf("category = %q, want %q", res.Category, "planar")
	}
	if !res.IsHardSurface {
		t.Error("bed should be flagged hard-surface")
	}
}
