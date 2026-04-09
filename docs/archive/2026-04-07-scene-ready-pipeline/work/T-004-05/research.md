# T-004-05 — Research

End-to-end validation of the trellis case across the S-004 pipeline:
classifier → strategy router → settings stamping → bake → row-instance
stress test. This is the first real exercise of every link in the chain
on a single asset, so the research starts by mapping the chain.

## The pipeline, end-to-end

1. **Upload** — `handleUpload` (handlers.go:45) writes the GLB to
   `originalsDir/<id>.glb` and calls `autoClassify` (handlers.go:868).
2. **Classifier** — `RunClassifier` (classify.go:27) shells out to
   `scripts/classify_shape.py`. The Python side does PCA on the world-space
   point cloud, computes (r2, r3) eigenvalue ratios, picks the closest
   centroid, and applies a vertical/horizontal disambiguation that splits
   the colocated `directional` and `tall-narrow` centroids by principal-
   axis orientation (classify_shape.py:302-328). Returns category +
   confidence + features (incl. top-N candidates for the comparison UI).
3. **Strategy router** — `getStrategyForCategory` (strategy.go:115) maps
   the category onto a `ShapeStrategy`. For `directional`:
   - `SliceAxis = "auto-horizontal"` (the longer of X / Z),
   - `SliceCount = 4`,
   - `SliceDistributionMode = "equal-height"`,
   - `InstanceOrientationRule = "fixed"`,
   - `DefaultBudgetPriority = "mid"`.
4. **Settings stamping** — `applyClassificationToSettings`
   (handlers.go:715) loads the asset's persisted `AssetSettings`,
   overwrites `ShapeCategory` / `ShapeConfidence`, then calls
   `applyShapeStrategyToSettings` (handlers.go:690) which copies the
   strategy's slice-shaped fields onto the asset *only when those fields
   are still at their factory default*. User overrides survive.
5. **Bake** — Volumetric bake lives entirely on the JS side, in
   `renderHorizontalLayerGLB` (static/app.js:1974). It reads
   `currentSettings.slice_axis`, calls `resolveSliceAxisRotation`
   (app.js:1941) which returns a quaternion that rotates the chosen
   axis onto +Y, slices in the Y-aligned working frame, then applies
   the inverse rotation to the export scene root so the produced GLB
   sits back in world space.
6. **Stress test** — `runStressTest` (app.js:3242) lays out a square
   grid of instances and calls `createInstancedFromModel(..., true)`
   (app.js:2989) for the regular preview path. The hardcoded `true` is
   `randomRotateY`: every instance gets its own seeded-random Y
   rotation. There is **no** wiring from the strategy router's
   `InstanceOrientationRule = "fixed"` into this call.

## Where the directional category comes from

`CLASS_CENTROIDS` in classify_shape.py:34 places `directional` and
`tall-narrow` at the same `(r2, r3) = (0.05, 0.05)` point — i.e. both
look like a near-1D distribution. Disambiguation (classify_shape.py:312)
adds 0.5 to the loser's distance:

- principal axis vertical → `directional` is penalized → `tall-narrow`
- principal axis horizontal → `tall-narrow` is penalized → `directional`

A garden trellis is wide-and-tall plus thin in depth. If width > height
the principal axis is horizontal → `directional`. If height > width the
principal axis is vertical → `tall-narrow`. The ticket presupposes the
former. The synthetic test asset must therefore be **wider than it is
tall** (e.g. 2.0 × 0.8 × 0.04) to land in the directional bucket.

`scripts/classify_shape.py` already has a `synth_lattice` (planar) and a
`synth_row` (tall axis-aligned box → directional). Neither is a true
trellis: `synth_lattice` is a flat lattice and classifies as planar;
`synth_row` is a single elongated box, no slats. We need a horizontal
lattice — slats forming a panel that's wider than tall.

## Existing tests that already cover parts of the chain

- `strategy_test.go` pins the strategy table — directional → `auto-horizontal`,
  4 layers, equal-height, fixed orientation, mid budget priority.
- `strategy_handlers_test.go::TestApplyClassificationStampsStrategy_Directional`
  pins the stamping path with a synthesized `ClassificationResult`. Does
  **not** exercise the real Python classifier.
- `scripts/classify_shape_test.py::test_directional_row` runs the
  Python side on `synth_row` and asserts category=directional. Does
  **not** touch the Go strategy / settings layer.

The gap T-004-05 fills: a single test that takes a real trellis-shaped
GLB on disk, runs the actual `python3 scripts/classify_shape.py`
subprocess, lets the strategy router stamp the settings, and asserts
the full surface — proving that nothing in the seam between Python and
Go is silently mistranslating directional.

## The instance-orientation gap

Acceptance criterion: "All 5 instances have the same Y rotation". As
mapped above, `runStressTest` always passes `randomRotateY=true` for
the regular preview path (app.js:3284). The S-004 strategy table
introduced `InstanceOrientationRule` specifically for this — see
strategy.go:24-34's note that "[these fields] are not yet persisted —
they exist on the struct so the lookup table is the canonical source
for downstream tickets". S-006 is the eventual landing zone for scene
templates, but the per-stress-test plumbing the AC asks for is in
scope here: it's a one-line change to read the orientation rule and
pass `false` for fixed-orientation categories.

The JS strategy mirror (`STRATEGY_TABLE` in app.js:349-356) currently
omits `instance_orientation_rule` entirely. Adding it is the obvious
fix.

## Asset placement

`assets/` already holds two scene assets (`rose_julia_child.glb`,
`wood_raised_bed.glb`) with no particular naming convention. The new
synthetic trellis fits there. A small script that constructs the GLB
in pure Python (re-using the GLB-writing pattern from
`parametric_reconstruct.py::write_glb`) keeps the asset reproducible
without dragging in Blender.

## Constraints / out of scope

- **Visual baking happens in the browser.** Headless validation of the
  produced GLB shape (slice rotation correctness, slice texture quality)
  is not feasible from this CLI environment. The ticket explicitly
  documents review.md as "with screenshots", and the review will
  flag this as the manual portion of acceptance.
- **No new shape categories**, no scene templates, no asymmetric
  placement (S-006 land).
- **First-pass scope** allows bake artifacts as long as orientation
  and classification are correct.
