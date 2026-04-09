package main

// SliceAxis sentinel values used by the shape strategy router.
//
//   - "y" is the literal vertical axis (current bake behavior).
//   - "auto-horizontal" is resolved by the bake to whichever of X or Z
//     is the longer horizontal extent of the model.
//   - "auto-thin" is resolved by the bake to the shortest of the three
//     bounding-box extents.
//   - "n/a" means the category does not slice — hard-surface routes to
//     the parametric pipeline (S-001) instead. Stamping skips slice
//     fields when the strategy carries this sentinel.
const (
	SliceAxisY              = "y"
	SliceAxisAutoHorizontal = "auto-horizontal"
	SliceAxisAutoThin       = "auto-thin"
	SliceAxisNA             = "n/a"
)

// ShapeStrategy is the per-category bake/orientation policy returned
// by getStrategyForCategory. Two of its fields (SliceCount,
// SliceDistributionMode) are mirrored onto AssetSettings during
// classification; the other two (InstanceOrientationRule,
// DefaultBudgetPriority) are not yet persisted — they exist on the
// struct so the lookup table is the canonical source for downstream
// tickets (T-004-04, S-006).
type ShapeStrategy struct {
	Category                string `json:"category"`
	SliceAxis               string `json:"slice_axis"`
	SliceCount              int    `json:"slice_count"`
	SliceDistributionMode   string `json:"slice_distribution_mode"`
	InstanceOrientationRule string `json:"instance_orientation_rule"`
	DefaultBudgetPriority   string `json:"default_budget_priority"`
}

// shapeStrategyTable is the closed lookup table mapping the S-004
// shape taxonomy onto a concrete bake / orientation policy. Entries
// are deliberately written out long-form (rather than constructed
// programmatically) so a future reader sees the full policy at a
// glance, and so the strategy_test.go pinning tests catch silent
// drift.
//
// Defaults rationale, per design.md:
//
//   - round-bush, unknown: match DefaultSettings() so the rose-asset
//     baseline behavior is preserved. round-bush is the historical
//     default this codebase grew up around.
//   - directional: slice perpendicular to the long horizontal axis
//     (auto-horizontal) so beds, fences, and other elongated assets
//     get sensible cross-sections instead of useless top-down slabs.
//   - tall-narrow: more layers (6 vs 4) along Y because vertical
//     structure is the load-bearing thing to capture for poles,
//     trees, etc. equal-height because vertex-quantile under-samples
//     the top of a tall asset.
//   - planar: slice perpendicular to the thin axis (auto-thin) with
//     fewer layers; aligned-to-row orientation is for downstream
//     scene placement (S-006).
//   - hard-surface: routes to the parametric pipeline; slice fields
//     are sentinels that the stamping helper ignores.
var shapeStrategyTable = map[string]ShapeStrategy{
	"round-bush": {
		Category:                "round-bush",
		SliceAxis:               SliceAxisY,
		SliceCount:              4,
		SliceDistributionMode:   "visual-density",
		InstanceOrientationRule: "random-y",
		DefaultBudgetPriority:   "mid",
	},
	"directional": {
		Category:                "directional",
		SliceAxis:               SliceAxisAutoHorizontal,
		SliceCount:              4,
		SliceDistributionMode:   "equal-height",
		InstanceOrientationRule: "fixed",
		DefaultBudgetPriority:   "mid",
	},
	"tall-narrow": {
		Category:                "tall-narrow",
		SliceAxis:               SliceAxisY,
		SliceCount:              6,
		SliceDistributionMode:   "equal-height",
		InstanceOrientationRule: "random-y",
		DefaultBudgetPriority:   "mid",
	},
	"planar": {
		Category:                "planar",
		SliceAxis:               SliceAxisAutoThin,
		SliceCount:              3,
		SliceDistributionMode:   "equal-height",
		InstanceOrientationRule: "aligned-to-row",
		DefaultBudgetPriority:   "low",
	},
	"hard-surface": {
		Category:                "hard-surface",
		SliceAxis:               SliceAxisNA,
		SliceCount:              0,
		SliceDistributionMode:   SliceAxisNA,
		InstanceOrientationRule: "fixed",
		DefaultBudgetPriority:   "high",
	},
	"unknown": {
		Category:                "unknown",
		SliceAxis:               SliceAxisY,
		SliceCount:              4,
		SliceDistributionMode:   "visual-density",
		InstanceOrientationRule: "random-y",
		DefaultBudgetPriority:   "mid",
	},
}

// getStrategyForCategory returns the canonical bake / orientation
// strategy for a classified shape category. Inputs outside the closed
// taxonomy (including the empty string) fall through to the "unknown"
// entry — never panics, never returns the zero value.
func getStrategyForCategory(category string) ShapeStrategy {
	if s, ok := shapeStrategyTable[category]; ok {
		return s
	}
	return shapeStrategyTable["unknown"]
}
