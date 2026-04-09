# T-004-03 — Structure

File-level blueprint. Code shapes only — no implementation.

## Files created

### `strategy.go` (~110 lines)

The shape-strategy router. Pure Go, no I/O, no globals beyond the
const lookup table.

```go
package main

// SliceAxis sentinel values. "y" is the literal vertical axis;
// "auto-horizontal" and "auto-thin" are resolved by the bake against
// the model bounding box at slice time. "n/a" means the category
// does not slice (hard-surface routes to the parametric pipeline).
const (
    SliceAxisY              = "y"
    SliceAxisAutoHorizontal = "auto-horizontal"
    SliceAxisAutoThin       = "auto-thin"
    SliceAxisNA             = "n/a"
)

// ShapeStrategy is the per-category bake/orientation policy.
// Returned by getStrategyForCategory; consumed by
// applyClassificationToSettings (slice fields) and reserved for
// downstream tickets (orientation, budget priority).
type ShapeStrategy struct {
    Category                string `json:"category"`
    SliceAxis               string `json:"slice_axis"`
    SliceCount              int    `json:"slice_count"`
    SliceDistributionMode   string `json:"slice_distribution_mode"`
    InstanceOrientationRule string `json:"instance_orientation_rule"`
    DefaultBudgetPriority   string `json:"default_budget_priority"`
}

// getStrategyForCategory returns the canonical strategy for a
// classified shape category. Unknown / empty input falls through to
// the same defaults as the "unknown" entry — never panics.
func getStrategyForCategory(category string) ShapeStrategy { ... }

// shapeStrategyTable is the closed lookup table. Declared at package
// level so tests can iterate it without re-running the function.
var shapeStrategyTable = map[string]ShapeStrategy{ ... }
```

The five entries match the design.md table verbatim. Hard-surface
sets slice fields to sentinels; the stamping helper skips them.

### `strategy_test.go` (~100 lines)

Black-box unit tests for the router:

- `TestGetStrategyForCategory_AllKnown` — every member of
  `validShapeCategories` returns a non-empty strategy whose
  `Category` field matches the lookup key.
- `TestGetStrategyForCategory_Unknown` — `""` and `"spirals"` both
  return the same defaults as `"unknown"`.
- `TestStrategyTable_DirectionalAxis` — the `directional` entry
  uses `SliceAxisAutoHorizontal`. Pinning so a future careless
  refactor doesn't silently break the manual verification path.
- `TestStrategyTable_HardSurfaceMarkedNA` — slice fields are the
  `SliceAxisNA` sentinel for hard-surface.
- `TestStrategyTable_RoundBushMatchesDefaults` — the round-bush
  entry's slice_distribution_mode and slice_count equal
  `DefaultSettings()`'s, preserving the rose-asset baseline.

## Files modified

### `settings.go`

- Append a new field at the **end** of `AssetSettings` (declaration
  order = on-disk JSON order; appending preserves existing files'
  field order):
  ```go
  // SliceAxis is the bake-time slicing axis chosen by the S-004
  // strategy router (T-004-03). One of "y", "auto-horizontal",
  // "auto-thin". Populated when the asset is classified; the user
  // may override via the tuning UI. Empty string on disk is
  // normalized to "y" at load time.
  SliceAxis string `json:"slice_axis,omitempty"`
  ```
- Add `validSliceAxes` map: `{"y", "auto-horizontal", "auto-thin"}`.
  Hard-surface stores `"y"` because it does not slice — the bake
  ignores the field for hard-surface assets.
- `DefaultSettings()`: `SliceAxis: "y"`.
- `Validate()`: enum check via `validSliceAxes`.
- `SettingsDifferFromDefaults()`: include `SliceAxis`.
- `LoadSettings()`: forward-compat — `if s.SliceAxis == "" {
  s.SliceAxis = "y" }`. Mirrors the existing
  `slice_distribution_mode` and `shape_category` patterns.

### `analytics.go`

- Add `"strategy_selected": true` to `validEventTypes`.

### `handlers.go`

- New helper: `applyShapeStrategyToSettings(s *AssetSettings,
  strategy ShapeStrategy)`. Stamps strategy fields onto `s`
  according to the override-semantics rule from design.md:
  ```
  If s.<Field> == DefaultSettings().<Field>
     AND strategy.<Field> != "n/a" / 0
  → s.<Field> = strategy.<Field>
  ```
  Operates on `SliceAxis`, `VolumetricLayers`, `SliceDistributionMode`.
  Does not touch any other field.
- `applyClassificationToSettings`: after the existing
  `s.ShapeCategory = ... ; s.ShapeConfidence = ...`, call
  `applyShapeStrategyToSettings(s, getStrategyForCategory(result.Category))`
  before `Validate()`.
- New helper:
  `emitStrategySelectedEvent(logger, id, strategy ShapeStrategy)`.
  Mirror of `emitClassificationEvent` — same
  `LookupOrStartSession`-then-`AppendEvent` pattern, payload
  `{"category": ..., "strategy": ...}` where strategy is the
  marshalled `ShapeStrategy`.
- `autoClassify`: after the existing `emitClassificationEvent` call,
  also `emitStrategySelectedEvent(logger, id, getStrategyForCategory(result.Category))`.
- `handleClassify`: same addition immediately after
  `emitClassificationEvent`.

### `static/app.js`

- `makeDefaults()`: add `slice_axis: 'y'` so a freshly created
  asset round-trips with the field present.
- `TUNING_SPEC`: append a row for `slice_axis` so future tuning UI
  picks it up. The DOM id (`tuneSliceAxis`) is reserved; the
  `populateTuningUI` / `wireTuningUI` short-circuit on missing
  elements (per the existing T-005-01 pattern), so this is dormant
  until a downstream ticket adds the control.
- `renderHorizontalLayerGLB`: pre-slice axis rotation. Determine
  the rotation from `currentSettings.slice_axis` and the model AABB:
  - `'y'` → identity (current behavior).
  - `'auto-horizontal'` → resolve to whichever of X/Z is the
    longest horizontal axis, then rotate that axis to Y.
  - `'auto-thin'` → resolve to the *shortest* axis (which may be
    X, Y, or Z), then rotate it to Y.
  After slicing, set the export scene root rotation to the inverse
  so the resulting GLB sits in the same world frame as the
  original. New helper `resolveSliceAxisRotation(model, mode)`
  isolates the math.

### `docs/knowledge/settings-schema.md`

- Add `slice_axis` to the field table with the enum and
  forward-compat note.

### `docs/knowledge/analytics-schema.md`

- Add a `strategy_selected` section after `classification`. Payload
  shape `{category, strategy: { slice_axis, slice_count,
  slice_distribution_mode, instance_orientation_rule,
  default_budget_priority }}`.

### `settings_test.go`

- New cases:
  - Validate rejects unknown `slice_axis`.
  - All members of `validSliceAxes` validate.
  - `LoadSettings` of an old document missing `slice_axis` returns
    `"y"`.
  - `SettingsDifferFromDefaults` flips on a `SliceAxis` mutation.

### `analytics_test.go`

- New case: `strategy_selected` event type passes `Validate()`.

### `handlers.go` (test side: no separate file unless one exists)

- A small Go-side end-to-end test in a new
  `handlers_strategy_test.go` (or fold into existing
  `classify_test.go`):
  - `TestApplyClassificationStampsStrategy_Directional` — start from
    `DefaultSettings()`, simulate a `directional` classification,
    assert `s.SliceAxis == "auto-horizontal"` and
    `s.SliceDistributionMode == "equal-height"`.
  - `TestApplyClassificationPreservesUserOverride` — start from
    settings where the user has set `SliceDistributionMode` to a
    non-default, classify as `directional`, assert the user value
    survives.

## Files deleted

None.

## Public interface notes for downstream tickets

- T-004-04 (multi-strategy comparison UI) reads `ShapeStrategy` via
  `getStrategyForCategory(category)` directly from Go and renders
  the alternates.
- S-006 scene preview reads `instance_orientation_rule` and
  `default_budget_priority` off the strategy struct (not off
  `AssetSettings` — those two fields are not persisted in this
  ticket because no consumer needs them yet).

## Ordering

The order of changes is load-bearing for compile and test:

1. `strategy.go` — pure module, no dependencies.
2. `settings.go` — new field + validation.
3. `analytics.go` — new event type.
4. `handlers.go` — stamping helper, classification path, emission.
5. `static/app.js` — bake reads new field; defaults updated.
6. Tests follow each step.
7. Docs (`settings-schema.md`, `analytics-schema.md`) updated last.
