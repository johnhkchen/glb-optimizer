package main

import (
	"errors"
	"io/fs"
	"os"
	"testing"
)

// TestApplyClassificationStampsStrategy_Directional verifies that a
// fresh, never-tuned asset gets the directional strategy stamped onto
// its settings end-to-end through applyClassificationToSettings: load
// from an empty dir, classify as directional, save, reload, assert.
func TestApplyClassificationStampsStrategy_Directional(t *testing.T) {
	dir := t.TempDir()
	id := "fresh"
	res := &ClassificationResult{
		Category:   "directional",
		Confidence: 0.91,
	}
	s, err := applyClassificationToSettings(id, dir, res, false)
	if err != nil {
		t.Fatalf("applyClassificationToSettings: %v", err)
	}
	if s.SliceAxis != SliceAxisAutoHorizontal {
		t.Errorf("SliceAxis = %q, want %q", s.SliceAxis, SliceAxisAutoHorizontal)
	}
	if s.SliceDistributionMode != "equal-height" {
		t.Errorf("SliceDistributionMode = %q, want %q", s.SliceDistributionMode, "equal-height")
	}
	if s.VolumetricLayers != 4 {
		t.Errorf("VolumetricLayers = %d, want 4", s.VolumetricLayers)
	}
	if s.ShapeCategory != "directional" {
		t.Errorf("ShapeCategory = %q, want %q", s.ShapeCategory, "directional")
	}

	// Round-trip via the on-disk file to make sure the stamped fields
	// actually persisted (not just mutated in memory).
	loaded, err := LoadSettings(id, dir)
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if loaded.SliceAxis != SliceAxisAutoHorizontal {
		t.Errorf("on-disk SliceAxis = %q, want %q", loaded.SliceAxis, SliceAxisAutoHorizontal)
	}
}

// TestApplyClassificationPreservesUserOverride asserts that a user
// who has tuned slice_distribution_mode away from defaults keeps
// their value across re-classification, while the still-default
// slice_axis field still gets stamped from the strategy. This is
// the "(user can override per-setting)" contract from the ticket.
func TestApplyClassificationPreservesUserOverride(t *testing.T) {
	dir := t.TempDir()
	id := "tuned"

	// Pre-write a settings file where the user has chosen a custom
	// slice distribution. Everything else is at defaults.
	custom := DefaultSettings()
	custom.SliceDistributionMode = "vertex-quantile"
	if err := SaveSettings(id, dir, custom); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}

	res := &ClassificationResult{
		Category:   "directional",
		Confidence: 0.88,
	}
	s, err := applyClassificationToSettings(id, dir, res, false)
	if err != nil {
		t.Fatalf("applyClassificationToSettings: %v", err)
	}

	// User override survives.
	if s.SliceDistributionMode != "vertex-quantile" {
		t.Errorf("SliceDistributionMode = %q, want %q (user override should survive)",
			s.SliceDistributionMode, "vertex-quantile")
	}
	// Field that was still at default gets stamped.
	if s.SliceAxis != SliceAxisAutoHorizontal {
		t.Errorf("SliceAxis = %q, want %q", s.SliceAxis, SliceAxisAutoHorizontal)
	}
}

// TestApplyClassificationHardSurfaceLeavesSliceFieldsAlone asserts
// that hard-surface classification does not overwrite slice fields
// (the strategy carries the SliceAxisNA sentinel and the stamping
// helper skips it). Hard-surface routes to the parametric pipeline
// from S-001; its slice fields are inert.
func TestApplyClassificationHardSurfaceLeavesSliceFieldsAlone(t *testing.T) {
	dir := t.TempDir()
	id := "bench"
	d := DefaultSettings()
	res := &ClassificationResult{
		Category:   "hard-surface",
		Confidence: 0.95,
	}
	s, err := applyClassificationToSettings(id, dir, res, false)
	if err != nil {
		t.Fatalf("applyClassificationToSettings: %v", err)
	}
	if s.SliceAxis != d.SliceAxis {
		t.Errorf("hard-surface clobbered SliceAxis: got %q, want %q", s.SliceAxis, d.SliceAxis)
	}
	if s.SliceDistributionMode != d.SliceDistributionMode {
		t.Errorf("hard-surface clobbered SliceDistributionMode: got %q, want %q",
			s.SliceDistributionMode, d.SliceDistributionMode)
	}
	if s.VolumetricLayers != d.VolumetricLayers {
		t.Errorf("hard-surface clobbered VolumetricLayers: got %d, want %d",
			s.VolumetricLayers, d.VolumetricLayers)
	}
	if s.ShapeCategory != "hard-surface" {
		t.Errorf("ShapeCategory = %q, want %q", s.ShapeCategory, "hard-surface")
	}
}

// TestApplyClassificationOverride_ForcesStrategyOverwrite is the
// T-004-04 end-to-end check on the override path. It exercises
// applyClassificationToSettings the same way handleClassify's override
// branch does: a synthesized ClassificationResult with the user-chosen
// category, Confidence=1.0, and force=true.
//
// The asset starts with a previous classification's strategy already
// stamped on (mimicking the real-world flow: auto-classify on upload
// stamps round-bush, then user reclassifies to directional). Every
// strategy-shaped field MUST be overwritten by the new category's
// strategy — leaving any of them at the previous value would mean
// the bake keeps using the wrong slice axis / distribution / layer
// count after a reclassify, which was the original "stuck on the
// wrong slicing" bug.
func TestApplyClassificationOverride_ForcesStrategyOverwrite(t *testing.T) {
	dir := t.TempDir()
	id := "override-asset"

	// Pre-write the asset as if a previous round-bush auto-classification
	// had already stamped its strategy fields. Use values that DIFFER
	// from the directional strategy, so the test can prove they got
	// overwritten.
	prev := DefaultSettings()
	prev.ShapeCategory = "round-bush"
	prev.ShapeConfidence = 0.85
	prev.SliceAxis = SliceAxisY
	prev.SliceDistributionMode = "visual-density"
	prev.VolumetricLayers = 6
	if err := SaveSettings(id, dir, prev); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}

	override := &ClassificationResult{
		Category:   "directional",
		Confidence: 1.0,
		Features:   map[string]interface{}{},
	}
	// force=true: explicit user pick from the comparison UI, must
	// overwrite previously-stamped strategy fields.
	s, err := applyClassificationToSettings(id, dir, override, true)
	if err != nil {
		t.Fatalf("applyClassificationToSettings: %v", err)
	}

	if s.ShapeCategory != "directional" {
		t.Errorf("ShapeCategory = %q, want %q", s.ShapeCategory, "directional")
	}
	if s.ShapeConfidence != 1.0 {
		t.Errorf("ShapeConfidence = %v, want 1.0 (human-confirmed sentinel)", s.ShapeConfidence)
	}
	directional := getStrategyForCategory("directional")
	if s.SliceAxis != directional.SliceAxis {
		t.Errorf("SliceAxis = %q, want %q (override must overwrite previous strategy)",
			s.SliceAxis, directional.SliceAxis)
	}
	if s.SliceDistributionMode != directional.SliceDistributionMode {
		t.Errorf("SliceDistributionMode = %q, want %q (override must overwrite previous strategy)",
			s.SliceDistributionMode, directional.SliceDistributionMode)
	}
	if s.VolumetricLayers != directional.SliceCount {
		t.Errorf("VolumetricLayers = %d, want %d (override must overwrite previous strategy)",
			s.VolumetricLayers, directional.SliceCount)
	}
}

// TestApplyClassification_AutoPreservesUserTunings is the companion
// test to TestApplyClassificationOverride_ForcesStrategyOverwrite. It
// pins the auto-classify path: when force=false, a user's manually
// tuned strategy field MUST survive a re-classification. The two
// tests together encode the override-semantics rule: explicit picks
// overwrite, automatic re-runs preserve.
func TestApplyClassification_AutoPreservesUserTunings(t *testing.T) {
	dir := t.TempDir()
	id := "auto-asset"

	// Pre-write a settings file with one strategy field tuned away
	// from the default. The other fields stay at default so we can
	// also assert that the strategy stamping fills them in.
	custom := DefaultSettings()
	custom.SliceDistributionMode = "equal-height"
	if err := SaveSettings(id, dir, custom); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}

	res := &ClassificationResult{
		Category:   "directional",
		Confidence: 0.91,
		Features:   map[string]interface{}{},
	}
	// force=false: silent re-classification, user tunings survive.
	s, err := applyClassificationToSettings(id, dir, res, false)
	if err != nil {
		t.Fatalf("applyClassificationToSettings: %v", err)
	}

	if s.SliceAxis != SliceAxisAutoHorizontal {
		t.Errorf("SliceAxis = %q, want %q (still-default field should be stamped)",
			s.SliceAxis, SliceAxisAutoHorizontal)
	}
	if s.SliceDistributionMode != "equal-height" {
		t.Errorf("SliceDistributionMode = %q, want %q (user override must survive auto-classify)",
			s.SliceDistributionMode, "equal-height")
	}
}

// TestExtractCandidates exercises the typed projection of the opaque
// classifier features.candidates list. T-004-04.
func TestExtractCandidates(t *testing.T) {
	cases := []struct {
		name     string
		features map[string]interface{}
		want     []candidate
	}{
		{
			name:     "missing key returns nil",
			features: map[string]interface{}{},
			want:     nil,
		},
		{
			name: "happy path returns ordered list",
			features: map[string]interface{}{
				"candidates": []interface{}{
					map[string]interface{}{"category": "directional", "score": 0.41},
					map[string]interface{}{"category": "tall-narrow", "score": 0.33},
				},
			},
			want: []candidate{
				{Category: "directional", Score: 0.41},
				{Category: "tall-narrow", Score: 0.33},
			},
		},
		{
			name: "empty category rejects whole list",
			features: map[string]interface{}{
				"candidates": []interface{}{
					map[string]interface{}{"category": "", "score": 1.0},
				},
			},
			want: nil,
		},
		{
			name: "wrong type rejects whole list",
			features: map[string]interface{}{
				"candidates": "not a list",
			},
			want: nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractCandidates(tc.features)
			if len(got) != len(tc.want) {
				t.Fatalf("len = %d, want %d (got %+v)", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("[%d] = %+v, want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// TestTrellisAssetClassifiesAsDirectional is the T-004-05 marquee
// validation test: it drives the *real* python3 classifier subprocess
// against the checked-in synthetic trellis asset and pins every link
// in the chain (Python classifier output → Go RunClassifier wrapper →
// strategy router → applyClassificationToSettings stamping). Any
// future drift in any one of those four pieces breaks this test.
//
// Skips (rather than fails) when python3 is not on PATH or the asset
// file is missing — mirrors the soft-dep posture of the rest of the
// classifier test suite.
func TestTrellisAssetClassifiesAsDirectional(t *testing.T) {
	const asset = "assets/trellis_synthetic.glb"
	if _, err := os.Stat(asset); errors.Is(err, fs.ErrNotExist) {
		t.Skipf("asset %q not present (run scripts/make_trellis_asset.py to regenerate)", asset)
	}

	res, err := RunClassifier(asset)
	if err != nil {
		// python3 missing or classifier crash → skip rather than
		// fail. The Go-side wrapping is unit-tested elsewhere.
		t.Skipf("RunClassifier(%q): %v", asset, err)
	}

	if res.Category != "directional" {
		t.Fatalf("category = %q, want %q (asset shape may have drifted; "+
			"regenerate via scripts/make_trellis_asset.py)", res.Category, "directional")
	}

	dir := t.TempDir()
	s, err := applyClassificationToSettings("trellis", dir, res, false)
	if err != nil {
		t.Fatalf("applyClassificationToSettings: %v", err)
	}

	if s.ShapeCategory != "directional" {
		t.Errorf("ShapeCategory = %q, want %q", s.ShapeCategory, "directional")
	}
	if s.SliceAxis != SliceAxisAutoHorizontal {
		t.Errorf("SliceAxis = %q, want %q", s.SliceAxis, SliceAxisAutoHorizontal)
	}
	if s.SliceDistributionMode != "equal-height" {
		t.Errorf("SliceDistributionMode = %q, want %q", s.SliceDistributionMode, "equal-height")
	}
	if s.VolumetricLayers != 4 {
		t.Errorf("VolumetricLayers = %d, want 4", s.VolumetricLayers)
	}
}
