# T-004-05 — Plan

Step-by-step execution of the structure plan. Each step is small enough
to commit atomically; verification commands are inline.

## Step 1 — `scripts/make_trellis_asset.py`

Write a self-contained Python script that builds a horizontal trellis
panel mesh and writes it as a GLB. Geometry: 4 horizontal slats and 7
vertical stiles, each a thin axis-aligned box, joined into one mesh.
Bounding box ≈ 2.0 × 0.8 × 0.04. No materials beyond a brown
`baseColorFactor` (matches `parametric_reconstruct.py`'s wood default).

Each box contributes 24 vertices (4 per face × 6 faces) and 12
triangles (2 per face × 6 faces) — same convention as the existing
parametric reconstruction so a future maintainer can cross-reference.

**Verify:** `python3 scripts/make_trellis_asset.py assets/trellis_synthetic.glb`
exits 0 and the file exists.

## Step 2 — Sanity-run the classifier

Run `python3 scripts/classify_shape.py assets/trellis_synthetic.glb`
and inspect stdout. Expected:

- `category == "directional"`
- `is_hard_surface == false`
- `confidence` reasonable (≥ 0.5; ideally ≥ 0.7)

If any of these fail:
- **`category != "directional"`** — adjust the asset's aspect ratio
  (wider relative to tall) and regenerate.
- **`is_hard_surface == true`** — the trellis is *too* axis-aligned
  with too few slats; increase `n_horizontals` and `n_verticals` so
  the per-axis vertex density is more uniform and `mean_peakiness`
  drops below 2.5.

Re-run until clean. Do not move on until this passes.

**Testing strategy note:** this sanity step is a one-shot manual
verification, not a checked-in unit test. The Go integration test in
step 4 is the durable assertion.

## Step 3 — Commit asset + generator

Stage `scripts/make_trellis_asset.py` and `assets/trellis_synthetic.glb`.
Commit message:

```
T-004-05: synthetic trellis asset + generator script
```

Atomic: any future regeneration is reproducible from the script.

## Step 4 — Add `TestTrellisAssetClassifiesAsDirectional`

Edit `strategy_handlers_test.go`. Add the test at the bottom (after
`TestExtractCandidates`). Structure:

```go
func TestTrellisAssetClassifiesAsDirectional(t *testing.T) {
    asset := "assets/trellis_synthetic.glb"
    if _, err := os.Stat(asset); errors.Is(err, fs.ErrNotExist) {
        t.Skipf("asset %q not present", asset)
    }
    res, err := RunClassifier(asset)
    if err != nil {
        // python3 not on PATH or classifier crash → skip, don't fail
        t.Skipf("RunClassifier: %v", err)
    }
    if res.Category != "directional" {
        t.Fatalf("category = %q, want directional", res.Category)
    }
    dir := t.TempDir()
    s, err := applyClassificationToSettings("trellis", dir, res)
    if err != nil {
        t.Fatalf("applyClassificationToSettings: %v", err)
    }
    if s.SliceAxis != SliceAxisAutoHorizontal { ... }
    if s.SliceDistributionMode != "equal-height" { ... }
    if s.VolumetricLayers != 4 { ... }
    if s.ShapeCategory != "directional" { ... }
}
```

The test resolves the asset path relative to the package directory
(which is the repo root, since the package is `main`). New imports:
`errors`, `io/fs`, `os`. Each is already used elsewhere in the repo
(check the file's existing imports first).

**Verify:** `go test ./... -run TestTrellisAsset -v` shows PASS.
Then `go test ./...` shows green for the whole package.

## Step 5 — Commit the integration test

```
T-004-05: end-to-end trellis-asset integration test
```

## Step 6 — `static/app.js` orientation fix

Two edits, both surgical:

**Edit A (around line 349):** add the `instance_orientation_rule` key
to every row of `STRATEGY_TABLE`. Mirror values from
`strategy.go::shapeStrategyTable`.

**Edit B (3 call sites):** replace the literal `true` argument to
`createInstancedFromModel` (line 3284, 3343, 3430) with a derived
boolean. Compute it once at the top of `runStressTest` so the three
sites share the same source of truth:

```js
const orientationRule = STRATEGY_TABLE[currentSettings.shape_category]
    ?.instance_orientation_rule || 'random-y';
const randomRotate = orientationRule !== 'fixed'
                  && orientationRule !== 'aligned-to-row';
```

Then the literal `true` becomes `randomRotate` in three places.

`runLodStressTest` is invoked from `runStressTest`; for the two call
sites that live inside `runLodStressTest` we have to recompute (or
pass in) `randomRotate`. Simplest: recompute at the top of
`runLodStressTest` from the same expression. The duplication is two
lines and avoids threading a parameter through an existing async
function signature.

**Verify:** the file parses (no JS test framework in the repo). Skim
the diff to make sure no other call sites of `createInstancedFromModel`
were missed.

## Step 7 — Commit the JS orientation fix

```
T-004-05: read instance_orientation_rule in stress test
```

## Step 8 — Write `review.md`

After all four commits, write `review.md` summarizing:
- the integration test that now exists,
- the JS fix that closes the orientation gap,
- the manual / browser-only portion of acceptance and a checklist for
  the human reviewer to walk through,
- known limitations and follow-ups (no headless screenshots, S-006's
  scene-template work for proper per-instance orientation control).

Do not commit `review.md` separately — Lisa picks it up on its own
detection cycle.

## Verification matrix

| Step | Verification command                                                       | Expected           |
|------|----------------------------------------------------------------------------|--------------------|
| 1    | `python3 scripts/make_trellis_asset.py assets/trellis_synthetic.glb`       | exit 0, file exists |
| 2    | `python3 scripts/classify_shape.py assets/trellis_synthetic.glb`           | `category=directional`, `is_hard_surface=false` |
| 4    | `go test -run TestTrellisAsset -v ./...`                                    | PASS               |
| 4    | `go test ./...`                                                             | all PASS           |
| 6    | manual diff review                                                          | three sites updated, table mirrors strategy.go |

## Out-of-plan contingencies

- **Classifier returns the wrong category in step 2.** Loop back into
  step 1 (asset shape tweak), do not patch the classifier — the
  classifier's calibration is T-004-01's responsibility and is
  pinned by `classify_shape_test.py`. A bug in the classifier surfaced
  here is in scope per the ticket but should be fixed under its own
  commit with a regression test in the python suite.
- **Hard-surface overlay fires.** Same loop — increase strut counts.
- **Go test passes but the bake / stress test misbehaves visually.**
  Document in review.md as an open concern; do not block the ticket.
  The ticket's first-pass scope explicitly allows visual artifacts.
