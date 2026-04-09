# Research — T-005-01: slice-distribution-and-shape-restoration

## Ticket recap

Three known visual problems on the current bake:

1. **Dome curvature gone** — vertex-quantile slicing flattened the umbrella.
2. **Ground clipping** — bottom slice's lowest vertex sits below `Y=0`.
3. **Center-of-mass drag** — vertex-quantile boundaries are pulled down by
   dense lower foliage, so the visual top of the plant gets squashed.

The fix is a new slice-distribution mode (`visual-density`), a wired
`dome_height_factor`, a `ground_align` flag, all surfaced through the
T-002-01 settings system and the T-003-02 analytics auto-instrumentation.

UI sliders are explicitly out of scope (T-005-02).

## Where the relevant code lives

### Slicing & dome (frontend)

| File | Line | Symbol | Role |
|---|---|---|---|
| `static/app.js` | 1188 | `createDomeGeometry(size, domeHeight, segments)` | Builds the parabolic-bulge plane used as one volumetric layer quad. UVs come from the plane projection. |
| `static/app.js` | 1207 | `computeAdaptiveSliceBoundaries(model, numLayers)` | Vertex-quantile boundary picker. Walks every mesh's `position` attribute, world-transforms each vertex, sorts the Y values, and returns `numLayers+1` boundaries at quantile indices. |
| `static/app.js` | 1249 | `pickAdaptiveLayerCount(model, baseLayers)` | Adds +1 or +2 layers for tall, narrow models (heightToWidth > 1.5 / 2.5). Independent from the new mode. |
| `static/app.js` | 1258 | `renderHorizontalLayerGLB(model, numLayers, resolution)` | Top-level: picks layer count, computes boundaries, bakes each layer top-down, builds the dome quads, exports GLB. The single chokepoint that ticket changes plug into. |

`renderHorizontalLayerGLB` is called from three places:

- `generateVolumetric` (line 1088)
- `generateVolumetricLODs` (line 1323) — iterates `VOLUMETRIC_LOD_CONFIGS`
- `generateProductionAsset` (line 1365)

All three pass the model + layer + resolution; none pass the settings
object — the function reads `currentSettings` directly for
`dome_height_factor` (line 1278) and `alpha_test` (line 1285), the
pattern T-002-02 introduced.

### Settings system (Go side)

| File | Symbol | Role |
|---|---|---|
| `settings.go` | `AssetSettings` struct (lines 22–35) | Field-ordered struct that round-trips to/from JSON. New fields go at the end so the on-disk order stays append-only. |
| `settings.go` | `DefaultSettings()` (lines 39–54) | Canonical defaults, mirrored by JS `makeDefaults()`. |
| `settings.go` | `Validate()` (lines 68–106) | Per-field range/enum check; first failure wins. |
| `settings.go` | `validResolutions`, `validLightingPresets` | Two existing enum maps — the pattern for the new slice-mode enum. |
| `settings_test.go` | `TestValidate_RejectsOutOfRange` etc. | Table-driven coverage; new fields slot in here. |

`AssetSettings` is a flat struct, no nested groups. Adding three fields
is purely additive — `LoadSettings` already tolerates unknown JSON keys,
and existing on-disk files (which won't have the new keys) decode into
the Go zero value. That zero value is **wrong** for `ground_align`
(`false` rather than `true`) and **invalid** for `slice_distribution_mode`
(empty string is not in the enum). Both must be normalized at load time
or the loader becomes a regression risk.

### Settings mirror (JS side)

| File | Line | Symbol | Role |
|---|---|---|---|
| `static/app.js` | 113 | `makeDefaults()` | Hand-maintained mirror of `DefaultSettings()`. New keys *must* be added here or the analytics-dirty dot lies and `applyDefaults()` produces a stale shape. |
| `static/app.js` | 260 | `TUNING_SPEC[]` | Drives the tuning panel. `populateTuningUI` and `wireTuningUI` skip entries whose DOM ids are absent — so adding spec rows for the new keys is *safe even without index.html changes*, and the analytics auto-instrumentation in `wireTuningUI` (lines 286–318) gives `setting_changed` events for free. |
| `static/app.js` | 1278, 1285 | `currentSettings.dome_height_factor`, `currentSettings.alpha_test` | The two existing settings reads inside `renderHorizontalLayerGLB`. Pattern to follow for the new fields. |

### Analytics events

`logEvent('setting_changed', {key, old_value, new_value, ms_since_prev}, assetId)`
fires automatically from `wireTuningUI` (line 311) on every `input`
event. Provided each new field has a `TUNING_SPEC` row (the only
enrollment mechanism), it gets analytics for free. No backend change is
needed — `validEventTypes` in `analytics.go` already accepts
`setting_changed`.

### Profiles

`profiles.go` embeds `*AssetSettings` directly and pins
`ProfilesSchemaVersion = SettingsSchemaVersion`. No code change needed
to round-trip the new fields; only the schema-version contract matters.

## Existing slice algorithm — exact behavior

`computeAdaptiveSliceBoundaries` (app.js:1211):

1. Collect every vertex's world-space Y into a flat `ys` array. No
   filtering, no weighting; trunk vertices count the same as canopy.
2. If empty, fall back to equal-height boundaries from the bounding box.
3. Sort `ys` ascending.
4. Boundary `i` (for `i ∈ [1, N-1]`) is `ys[floor(i/N * ys.length)]`.
   Boundary 0 is `ys[0]`, boundary N is `ys[length-1]`.

This is a true vertex quantile: dense regions get thinner (more
closely-spaced) boundaries, sparse regions get thicker ones. For a
plant with a heavy lower stem mesh and a sparse leafy crown the result
is almost all boundaries packed into the bottom 30% of height, which is
exactly the "center-of-mass drag" symptom the ticket calls out.

## Constraints and assumptions

- **No three.js dependency churn.** All math is `THREE.Vector3` /
  `Box3` / `BufferAttribute`, already imported.
- **Single bake call site mutation.** All three callers go through
  `renderHorizontalLayerGLB`; the per-mode dispatch lives there.
- **Ground alignment must not break the boundary contract.** Boundaries
  are world-space Y values; if we shift the export scene by some
  `yOffset`, the per-quad `quad.position.y = floorY` is the boundary
  coordinate, *not* the post-shift value. Either subtract the offset
  from `floorY` before placement, or apply a single translate to the
  exportScene root.
- **`pickAdaptiveLayerCount` is orthogonal.** It runs before the
  boundary picker and is unaffected by the distribution mode. Keep it.
- **Backwards compatibility.** `equal-height` mode must reproduce the
  *original* simple horizontal slicing — i.e. ignore both the vertex
  quantile and the trunk filter, and place boundaries by linear
  interpolation across the bounding box Y range.
- **Settings on-disk forward compat.** Old JSON files lack the three
  new keys. Their decoded zero values are unsafe (see above), so the
  loader must normalize.
- **No UI work.** index.html stays unchanged in this ticket; T-005-02
  surfaces the controls.

## Open questions / risks

- **Trunk-filter heuristic.** "Bottom 10% of bounding-box height" is a
  fixed fraction. Some assets (low ground cover) have no real trunk and
  the filter discards meaningful canopy. The ticket explicitly says
  "don't over-engineer" the heuristic — accepted, but worth noting in
  Design that a future per-asset override exists in the settings DAG.
- **Radial weight.** "Optionally weights remaining vertices by distance
  from the central axis" — Design must decide whether to ship the weight
  in v1 or just the trunk filter. Both reduce the center-of-mass drag;
  the radial weight is the more invasive change.
- **Ground alignment ordering.** Whether the offset is applied
  pre-bake (move the model) or post-bake (move the exportScene) affects
  whether the per-layer renderLayerTopDown viewport is recomputed. The
  cheapest correct approach is post-bake (offset only the per-quad
  Y placement), since the top-down camera frames are computed in
  world space from `floor`/`ceiling` and need to stay consistent across
  modes.
- **Validation for the new enum.** Mirror the `validResolutions` /
  `validLightingPresets` pattern with a `validSliceDistributionModes`
  map. Empty string from a forward-compat decode must be coerced to the
  default at load time, *not* rejected by Validate.
