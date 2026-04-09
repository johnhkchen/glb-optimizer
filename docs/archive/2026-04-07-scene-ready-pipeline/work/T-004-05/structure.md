# T-004-05 — Structure

## Files created

### `scripts/make_trellis_asset.py` (new)
Standalone Python generator for `assets/trellis_synthetic.glb`. No
Blender. Builds an axis-aligned trellis panel as a single triangulated
mesh and writes a v2 GLB using the same hand-rolled writer pattern as
`scripts/parametric_reconstruct.py::write_glb`.

Public surface:
- `def build_trellis_mesh(width=2.0, height=0.8, depth=0.04, n_horizontals=4, n_verticals=7) -> (positions, normals, uvs, indices)`
- `def write_glb(path, positions, normals, uvs, indices)`
- `if __name__ == "__main__": main()` — invocation:
  `python3 scripts/make_trellis_asset.py assets/trellis_synthetic.glb`

The script is idempotent: re-running overwrites the asset. The exact
slat/stile counts are checked-in constants, so the GLB is byte-stable
across runs (same RNG-free deterministic geometry).

### `assets/trellis_synthetic.glb` (new, generated artifact)
The output of the generator. Roughly 200 KB; checked into the repo
because the rest of `assets/` is checked in too (`rose_julia_child.glb`,
`wood_raised_bed*.glb`).

### `docs/active/work/T-004-05/{research,design,structure,plan,progress,review}.md`
RDSPI artifacts. Standard layout.

## Files modified

### `strategy_handlers_test.go`
Add `TestTrellisAssetClassifiesAsDirectional` at the bottom of the
file, next to the existing
`TestApplyClassificationStampsStrategy_Directional`. Public test
function; no helper extraction needed.

Behavior:
1. Resolve `assets/trellis_synthetic.glb` relative to the test working
   directory. Skip with `t.Skip` if the file is missing — the asset is
   checked in, so a skip is a CI environment problem, not a test
   failure.
2. Call `RunClassifier` (the production wrapper that shells out to
   `python3`). On `exec.ErrNotFound` or any subprocess failure, skip
   with a clear message — `python3` is a soft dep on this repo and
   the existing classifier tests follow the same convention.
3. Assert `result.Category == "directional"`.
4. Pipe the result through `applyClassificationToSettings` against a
   `t.TempDir`.
5. Pin all four directional fields: `ShapeCategory`, `SliceAxis`,
   `SliceDistributionMode`, `VolumetricLayers`.

No new helpers in test code. The test is intentionally a single linear
function so a future reader can read it top-to-bottom and see the full
chain.

### `static/app.js`
Two surgical edits.

1. **STRATEGY_TABLE** (around line 349) — add
   `instance_orientation_rule` to each row. Mirror values from
   `strategy.go`'s table:
   - `round-bush`   → `'random-y'`
   - `directional`  → `'fixed'`
   - `tall-narrow`  → `'random-y'`
   - `planar`       → `'aligned-to-row'`
   - `hard-surface` → `'fixed'`
   - `unknown`      → `'random-y'`

2. **runStressTest** (around line 3284) — replace the literal `true`
   with a derived expression:
   ```js
   const orientationRule = STRATEGY_TABLE[currentSettings.shape_category]?.instance_orientation_rule || 'random-y';
   const randomRotate = orientationRule !== 'fixed' && orientationRule !== 'aligned-to-row';
   const instances = createInstancedFromModel(currentModel, count, positions, randomRotate);
   ```
   Same change at line 3343 (the no-LODs fallback inside
   `runLodStressTest`) and line 3430 (the per-LOD-level fallback).

The decision to gate on category-derived `instance_orientation_rule`
rather than persisting a per-asset orientation field keeps the schema
unchanged. S-006 will revisit when scene templates need per-instance
control.

## Files NOT modified

- **`strategy.go`** / **`settings.go`** — the strategy table is already
  correct; the `InstanceOrientationRule` field is already declared.
- **`scripts/classify_shape.py`** — the classifier already handles a
  horizontal-trellis shape correctly per its centroids; no calibration
  needed.
- **`handlers.go`** — the upload → classify → stamp path is already
  the production code path. The new test exercises it directly, so no
  handler edits.
- **`docs/knowledge/*.md`** — no public schema or contract changes.

## Ordering of changes

The plan executes them in this order so each step is independently
verifiable:

1. Write `scripts/make_trellis_asset.py`.
2. Run it; produce `assets/trellis_synthetic.glb`.
3. Sanity-run `python3 scripts/classify_shape.py assets/trellis_synthetic.glb`
   and visually confirm `category == "directional"`. If not, tune the
   slat/stile counts in step 1 and regenerate. **Stop here and fix**
   before adding the Go test — a failing classifier sanity check
   means the asset is wrong, not the test.
4. Add `TestTrellisAssetClassifiesAsDirectional` to
   `strategy_handlers_test.go`. Run `go test ./...` and confirm green.
5. Apply the two `static/app.js` edits.
6. Write `progress.md` in tandem with steps 1–5.
7. Write `review.md` after step 5 lands.

## Module boundaries / interfaces

No new public Go types, no new HTTP routes, no new analytics events.
The change is additive at three points and the existing contracts are
the same.
