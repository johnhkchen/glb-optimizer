package main

import "testing"

// TestGetStrategyForCategory_AllKnown asserts every member of the
// closed shape taxonomy resolves to a non-empty strategy whose
// Category field matches the lookup key. Drift between
// validShapeCategories (settings.go) and shapeStrategyTable
// (strategy.go) is the failure mode this test catches.
func TestGetStrategyForCategory_AllKnown(t *testing.T) {
	for cat := range validShapeCategories {
		s := getStrategyForCategory(cat)
		if s.Category != cat {
			t.Errorf("category %q: strategy.Category = %q, want %q", cat, s.Category, cat)
		}
		if s.SliceAxis == "" {
			t.Errorf("category %q: empty SliceAxis", cat)
		}
		if s.InstanceOrientationRule == "" {
			t.Errorf("category %q: empty InstanceOrientationRule", cat)
		}
		if s.DefaultBudgetPriority == "" {
			t.Errorf("category %q: empty DefaultBudgetPriority", cat)
		}
	}
}

// TestGetStrategyForCategory_Unknown asserts that empty input and
// inputs outside the taxonomy fall through to the "unknown" entry.
// The router must never panic on a stale or corrupt category value.
func TestGetStrategyForCategory_Unknown(t *testing.T) {
	want := shapeStrategyTable["unknown"]
	for _, in := range []string{"", "spirals", "RoundBush", "round_bush"} {
		got := getStrategyForCategory(in)
		if got != want {
			t.Errorf("getStrategyForCategory(%q) = %+v, want %+v", in, got, want)
		}
	}
}

// TestStrategyTable_DirectionalAxis pins the directional → auto-horizontal
// mapping. The ticket's manual verification step depends on this row;
// a refactor that silently changes it would invalidate the
// acceptance criterion.
func TestStrategyTable_DirectionalAxis(t *testing.T) {
	s := getStrategyForCategory("directional")
	if s.SliceAxis != SliceAxisAutoHorizontal {
		t.Errorf("directional SliceAxis = %q, want %q", s.SliceAxis, SliceAxisAutoHorizontal)
	}
	if s.SliceDistributionMode != "equal-height" {
		t.Errorf("directional SliceDistributionMode = %q, want %q", s.SliceDistributionMode, "equal-height")
	}
}

// TestStrategyTable_HardSurfaceMarkedNA asserts hard-surface uses
// the "n/a" sentinel for slice fields. The stamping helper relies on
// this to leave AssetSettings slice fields untouched for parametric
// pipeline assets.
func TestStrategyTable_HardSurfaceMarkedNA(t *testing.T) {
	s := getStrategyForCategory("hard-surface")
	if s.SliceAxis != SliceAxisNA {
		t.Errorf("hard-surface SliceAxis = %q, want %q", s.SliceAxis, SliceAxisNA)
	}
	if s.SliceDistributionMode != SliceAxisNA {
		t.Errorf("hard-surface SliceDistributionMode = %q, want %q", s.SliceDistributionMode, SliceAxisNA)
	}
	if s.SliceCount != 0 {
		t.Errorf("hard-surface SliceCount = %d, want 0", s.SliceCount)
	}
}

// TestStrategyTable_RoundBushMatchesDefaults pins the round-bush
// entry to DefaultSettings(). round-bush is the historical default
// this codebase grew up around (the rose asset); regressions here
// would silently change the baseline behavior of the rose bake.
func TestStrategyTable_RoundBushMatchesDefaults(t *testing.T) {
	d := DefaultSettings()
	s := getStrategyForCategory("round-bush")
	if s.SliceAxis != d.SliceAxis {
		t.Errorf("round-bush SliceAxis = %q, want %q", s.SliceAxis, d.SliceAxis)
	}
	if s.SliceCount != d.VolumetricLayers {
		t.Errorf("round-bush SliceCount = %d, want %d", s.SliceCount, d.VolumetricLayers)
	}
	if s.SliceDistributionMode != d.SliceDistributionMode {
		t.Errorf("round-bush SliceDistributionMode = %q, want %q", s.SliceDistributionMode, d.SliceDistributionMode)
	}
}
