# Design — T-005-01: slice-distribution-and-shape-restoration

## Decision summary

1. **Add a `slice_distribution_mode` enum** with values `equal-height`,
   `vertex-quantile`, `visual-density`. Default: `visual-density`.
2. **Implement `visual-density` as: trunk-filter + radial-weighted
   quantile.** Trunk filter discards the bottom 10% of bounding-box
   height. Radial weight = `distFromAxis / maxRadius` clamped to
   `[0.05, 1.0]`. Quantiles are computed over the cumulative *weight*
   array, not vertex count.
3. **Wire `dome_height_factor`** through `createDomeGeometry`. (Already
   read at the call site as of T-002-02; wire it through the parameter
   list so the function is testable in isolation.)
4. **Ground alignment** is a single Y translate of the export scene
   root applied after all quads are placed. Default `true`.
5. **Forward-compat normalize at load time.** Empty
   `slice_distribution_mode` and missing `ground_align` get filled in
   `LoadSettings` *before* `Validate` runs, so existing on-disk files
   keep working.

## Options considered (and why each lost or won)

### Question 1: How to express the new mode

**A. Free-form string + enum map.** ✅ Chosen.
Mirrors the existing `LightingPreset` / `validLightingPresets` pattern
exactly. Cheap to extend (S-007 will add more modes).

**B. Boolean `use_visual_density`.** Rejected.
The ticket requires three values (`equal-height`, `vertex-quantile`,
`visual-density`); a boolean cannot represent the back-compat
`equal-height` fallback alongside the new mode and the legacy mode.

**C. Numeric "weight exponent".** Rejected.
A continuous knob looks elegant but is impossible to validate, hard to
analytics-bin, and conflates two orthogonal axes (trunk filter on/off
and radial weight on/off). Three discrete modes give the analytics
team something to count.

### Question 2: Algorithm for `visual-density`

**A. Trunk filter only.** Rejected — the ticket asks for the radial
weight as the *core* mechanism. Trunk filter alone is barely better
than vertex-quantile when the dense lower mass is foliage rather than
a clean stem.

**B. Trunk filter + radial weight (chosen).**
- Step 1: world-transform every vertex; compute the model's
  bounding-box `minY`, `maxY`, and the X/Z radius `R = max(|x|,|z|)`
  over the same vertices.
- Step 2: define `trunkY = minY + 0.10 * (maxY - minY)`. Discard any
  vertex with `y < trunkY`.
- Step 3: assign each surviving vertex a weight
  `w = clamp(sqrt(x² + z²) / R, 0.05, 1.0)`. The `0.05` floor keeps
  central canopy vertices contributing instead of vanishing entirely;
  the `1.0` ceiling is just `min`.
- Step 4: sort surviving vertices by Y. Build a cumulative-weight
  array. For boundary `i ∈ [1, N-1]`, find the smallest index `k`
  such that `cumWeight[k] >= (i/N) * totalWeight`. Boundary 0 = first
  surviving Y; boundary N = last surviving Y (or `maxY` if you want
  the top-most original geometry covered — see "Edge cases" below).

Why this is the right tradeoff: it's two passes over the vertex array
(one to gather, one for cum-sum/lookup). No KD-trees, no PCA, no
clustering. The trunk filter gets the obvious win; the radial weight
fixes the sneakier "dense central foliage" case the ticket explicitly
calls out.

**C. PCA / spectral clustering.** Rejected — explicit "don't
over-engineer the heuristic" in the ticket. Save it for after the
analytics tell us what users override.

**D. Per-mesh weighting.** Rejected — assets often have one giant
mesh; per-mesh granularity buys nothing here.

### Question 3: Ground alignment placement

**A. Apply offset to each `quad.position.y`.** Rejected.
Subtle: `boundaries[]` and `floorY`/`ceilingY` are still passed into
`renderLayerTopDown`, which uses them as **bake-time camera bounds**,
not export-time placement. Mutating only the placement would shift
quads but keep the *contents* of each layer at their original Y, which
visually breaks. Mutating both is two changes to keep in sync.

**B. Translate the export scene root after population.** ✅ Chosen.
After all quads are added, set `exportScene.position.y = -bottomMin`.
The bake textures are unchanged, the inter-quad spacing is unchanged,
and the GLTFExporter writes the translated root verbatim into a node
transform. Single line, single source of truth.

**C. Move the model before baking.** Rejected.
Mutates user state. The bake function takes `model` by reference and
the calling code does not expect side effects.

The "lowest vertex" we align to is `min(boundaries[0])` — i.e. the
floor of the bottom slice. That is the bottom of the geometry the
volumetric bake actually represents. (The dome bulge of the bottom
slice peaks *above* `boundaries[0]`, never below it; the parabolic
formula `(1 - dist²) * domeHeight` is non-negative.)

### Question 4: `dome_height_factor` plumbing

**A. Continue reading `currentSettings` inside `createDomeGeometry`.**
Rejected. The dome helper already takes `domeHeight` as a parameter;
the *caller* (renderHorizontalLayerGLB) is what reads the factor. The
ticket says "wire `dome_height_factor` through `createDomeGeometry`,
currently hardcoded at 0.5 of layer thickness". Reading the existing
code, the 0.5 is **already not hardcoded** as of T-002-02 — line 1278
reads `currentSettings.dome_height_factor`. The ticket text is stale.
Design records this and treats it as already-done; the implementation
phase will verify and add a code comment, no behavior change.

## Mode behavior matrix

| Mode | Trunk filter | Radial weight | Boundary spacing |
|---|---|---|---|
| `equal-height` | no | no | linear over `[minY, maxY]` |
| `vertex-quantile` | no | no | unweighted vertex quantile (current behavior, retained for backwards compat) |
| `visual-density` | bottom 10% | yes | weight-quantile over surviving vertices |

`equal-height` is the *original* simple horizontal slicing the ticket
specifies for backwards compat.

## Validation rules (additions to `Validate()`)

- `slice_distribution_mode` must be in
  `{"equal-height", "vertex-quantile", "visual-density"}`.
- `ground_align` is a `bool`, no range. Validation passes for both
  values; the field exists so the JSON is explicit and the dirty-dot
  works.
- `dome_height_factor` already validated `[0.0, 2.0]` from T-002-01.

## Forward-compat at load time

`LoadSettings` reads the JSON, then before validating, normalizes:

```go
if s.SliceDistributionMode == "" {
    s.SliceDistributionMode = "visual-density"
}
// ground_align: zero value is false; we want true. The on-disk JSON
// for an old file will not contain the key, so json.Unmarshal leaves
// it false. To distinguish "absent" from "explicit false" we decode
// into a *bool first then promote, OR we accept that "absent" → true
// and document it as the migration rule. We choose the latter — old
// files predate the ticket so the original behavior was implicitly
// unaligned, and the user-visible improvement of forcing alignment on
// is the right default for the migration window.
```

This normalization is *not* a `SchemaVersion` bump because nothing
about the existing fields changes.

## Edge cases

- **No vertices survive trunk filter.** Pathological model (entire
  thing in the bottom 10%). Fall back to vertex-quantile over the
  unfiltered set. Logged via `console.warn` — no analytics event,
  this is a developer signal.
- **All surviving weights collapse to the floor (e.g. all vertices
  on the central axis).** The clamp to `0.05` ensures totalWeight > 0,
  so the cum-sum lookup is well-defined and degrades to a uniform
  vertex-quantile over the survivors.
- **`numLayers = 1`.** No interior boundaries; only `[bottom, top]`.
  The mode is irrelevant. All three modes converge.
- **Bottom slice's lowest vertex equals `Y=0` already.** Offset is 0;
  the translate is a no-op.
- **`ground_align = false`.** Skip the translate entirely. Existing
  scene-graph behavior preserved.

## What rejection of "radial weight v1" would have cost us

Briefly considered: ship trunk filter only and add radial weight in
T-005-02. Rejected because:
1. The test asset (rose) is exactly the "dense central foliage"
   case where trunk filter alone is insufficient.
2. The radial weight is ~10 lines of code on top of the trunk filter.
3. T-005-02 is about UI surfacing, not algorithm work; bundling the
   algorithm change into the UI ticket would conflate two reviews.
