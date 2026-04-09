# Structure — T-005-01

## Files touched

| File | Action | Net effect |
|---|---|---|
| `settings.go` | MODIFY | +3 fields on `AssetSettings`, +3 defaults, +1 enum map, +2 validation rules, normalization in `LoadSettings` |
| `settings_test.go` | MODIFY | +3 cases for new fields and migration |
| `docs/knowledge/settings-schema.md` | MODIFY | +3 rows in the field table; new "Forward-compat normalization" subsection |
| `static/app.js` | MODIFY | +3 keys in `makeDefaults()`, +3 rows in `TUNING_SPEC`, new `computeVisualDensityBoundaries()` and `computeEqualHeightBoundaries()` helpers, dispatch in `renderHorizontalLayerGLB`, ground-align translate |
| `docs/active/work/T-005-01/{research,design,structure,plan,progress,review}.md` | CREATE | RDSPI artifacts |

No new Go files. No new JS modules. No `index.html` / `style.css`
edits — UI surfacing is T-005-02.

## `settings.go` — change map

### New struct fields (appended in declaration order)

```go
type AssetSettings struct {
    // ... existing fields unchanged ...
    LightingPreset        string  `json:"lighting_preset"`
    SliceDistributionMode string  `json:"slice_distribution_mode"`
    GroundAlign           bool    `json:"ground_align"`
}
```

`DomeHeightFactor` is **not** moved or duplicated; it already exists
from T-002-01 and is wired to the JS path. The ticket text claiming
it is "currently hardcoded at 0.5" is stale (see design.md
"Question 4"). The structure document acknowledges this and the
implementation will not touch the field except to add a code-comment
breadcrumb.

### New defaults (in `DefaultSettings`)

```go
SliceDistributionMode: "visual-density",
GroundAlign:           true,
```

### New enum map

```go
var validSliceDistributionModes = map[string]bool{
    "equal-height":    true,
    "vertex-quantile": true,
    "visual-density":  true,
}
```

### New validation lines (in `Validate`, after `lighting_preset` block)

```go
if !validSliceDistributionModes[s.SliceDistributionMode] {
    return fmt.Errorf("slice_distribution_mode %q is not a known mode", s.SliceDistributionMode)
}
// GroundAlign: bool, no constraint.
```

### `LoadSettings` normalization (after `json.Unmarshal`, before return)

```go
// Forward-compat normalization for files written before T-005-01.
// Old files lack these keys; their zero values are unsafe.
if s.SliceDistributionMode == "" {
    s.SliceDistributionMode = "visual-density"
}
// Note: GroundAlign zero value is `false`, but the migration default
// is `true`. We can't distinguish "explicit false" from "absent" with
// a plain bool, so old files get `false` here and only newly-written
// files get the desired default. Documented in settings-schema.md.
```

After re-reading: the asymmetry between the two new fields is ugly.
**Implementation tactic:** decode into a temporary struct that uses
`*bool` for `GroundAlign`, then promote: `nil → true`, otherwise the
explicit value. Confined to `LoadSettings`; `AssetSettings` itself
keeps the plain `bool` for ergonomic Go usage everywhere else.

Concretely:

```go
type assetSettingsWire struct {
    AssetSettings
    GroundAlign *bool `json:"ground_align"`
}
var w assetSettingsWire
if err := json.Unmarshal(data, &w); err != nil { ... }
s := w.AssetSettings
if w.GroundAlign != nil {
    s.GroundAlign = *w.GroundAlign
} else {
    s.GroundAlign = true
}
```

Wait — embedding `AssetSettings` and shadowing the field interacts
poorly with Go's tag-based unmarshal. The embedded struct still has
its own `json:"ground_align"` tag, which conflicts with the outer.
**Alternative:** unmarshal twice — once into `AssetSettings`, once
into a tiny `struct { GroundAlign *bool }`. Both paths use the same
`data []byte`. Keeps the wire format clean and the migration explicit.
This is what the implementation should do.

## `settings_test.go` — change map

Add three test functions:

1. `TestDefaultSettings_NewFields` — asserts the two new defaults.
2. `TestValidate_RejectsBadSliceMode` — sets the field to a garbage
   string, expects validation failure. Append cases to the existing
   `TestValidate_RejectsOutOfRange` table for symmetry.
3. `TestLoadSettings_MigratesOldFile` — writes a JSON file that lacks
   both new keys, calls `LoadSettings`, asserts:
   - `slice_distribution_mode == "visual-density"`
   - `ground_align == true`
   - subsequent `Validate()` returns nil.

## `static/app.js` — change map

### 1. `makeDefaults()` (line 113)

Append two keys (after `lighting_preset`):

```js
slice_distribution_mode: 'visual-density',
ground_align: true,
```

Schema-version is unchanged; JS mirror order is informational only.

### 2. `TUNING_SPEC[]` (line 260)

Append two rows. The DOM ids are reserved for T-005-02; until then
`populateTuningUI` and `wireTuningUI` skip absent ids harmlessly:

```js
{ field: 'slice_distribution_mode', id: 'tuneSliceDistributionMode',
  parse: v => v,             fmt: v => v },
{ field: 'ground_align',            id: 'tuneGroundAlign',
  parse: v => v === 'true' || v === true,
  fmt: v => String(v) },
```

(Adding the row enrolls the field for `setting_changed` analytics
once T-005-02 wires the DOM controls. No dead code today; the
populate/wire loops short-circuit on `!el`.)

### 3. New helper: `computeEqualHeightBoundaries(model, numLayers)`

~10 lines. World-space bounding-box `min.y`/`max.y`, linear interpolate
N+1 boundaries. Used for `equal-height` mode.

### 4. New helper: `computeVisualDensityBoundaries(model, numLayers)`

~50 lines. Algorithm exactly as design.md "Question 2 — Option B":

1. Walk every mesh, world-transform every vertex into two parallel
   arrays: `ys[]` and `weights[]`. Track running `minY`, `maxY`,
   `maxRadius`.
2. If `ys.length === 0`, fall back to equal-height.
3. Compute `trunkY = minY + 0.10 * (maxY - minY)`.
4. Filter the two arrays in lockstep: drop pairs with `y < trunkY`.
   Compute weights in the same loop:
   `w = clamp(sqrt(x*x + z*z) / maxRadius, 0.05, 1.0)`.
5. If filtered set is empty, fall back to `computeAdaptiveSliceBoundaries`
   (the existing unfiltered quantile picker) and `console.warn`.
6. Sort `(y, w)` pairs by `y` ascending. Build cumulative weight array.
7. For `i ∈ [1, N-1]`, find smallest `k` such that
   `cumWeight[k] >= (i/N) * totalWeight`. Boundary[i] = `ys[k]`.
   Boundary[0] = `minY` (pre-filter, so the bottom of the geometry is
   covered). Boundary[N] = `maxY`.

(Note: using pre-filter `minY` and `maxY` for the outer boundaries
keeps the bake camera framing identical between modes; only the
*interior* boundaries shift.)

### 5. `renderHorizontalLayerGLB` (line 1258) — dispatch + alignment

Replace the single `computeAdaptiveSliceBoundaries` call with:

```js
const mode = currentSettings.slice_distribution_mode;
let boundaries;
switch (mode) {
    case 'equal-height':
        boundaries = computeEqualHeightBoundaries(model, actualLayers);
        break;
    case 'visual-density':
        boundaries = computeVisualDensityBoundaries(model, actualLayers);
        break;
    case 'vertex-quantile':
    default:
        boundaries = computeAdaptiveSliceBoundaries(model, actualLayers);
        break;
}
```

After the for-loop that builds the quads, before constructing the
exporter:

```js
if (currentSettings.ground_align) {
    exportScene.position.y = -boundaries[0];
}
```

(Boundary[0] is the floor of the bottom slice = lowest vertex of the
volumetric representation. Translating the scene root by `-boundaries[0]`
puts that floor at exactly Y=0.)

### 6. Code comment for `dome_height_factor`

Add a `// dome_height_factor wired through currentSettings (T-005-01)`
breadcrumb on line 1278 since the ticket text references this wiring.
No behavior change.

## `docs/knowledge/settings-schema.md` — change map

- Add three rows to the fields table (`slice_distribution_mode`,
  `ground_align`).
- Update the JSON example block to include the two new keys.
- Add a new subsection "Forward-compat normalization" under "Migration
  Policy" documenting the `nil-bool → true` and empty-string → default
  rules.

## Ordering constraints

1. Settings struct + defaults + validation come first (Go side
   compiles independently; tests pass on their own).
2. Schema doc updated alongside the struct (single review pair).
3. JS `makeDefaults` + `TUNING_SPEC` come next (no behavior change
   without the helpers).
4. JS helpers (`computeEqualHeightBoundaries`, `computeVisualDensityBoundaries`).
5. Dispatch + ground-align inside `renderHorizontalLayerGLB`.
6. Manual rebake to verify (out-of-band; not blocking the commit).
