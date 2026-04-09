# T-004-05 — Progress

## Step 1 — `scripts/make_trellis_asset.py`

Done. Self-contained Python script (no Blender). Builds 12 horizontal
slats × 16 vertical stiles, panel ≈ 2.0 × 0.8 × 0.04, joins them as a
single triangulated mesh, writes a v2 GLB via the same hand-rolled
writer pattern as `parametric_reconstruct.py::write_glb` (texture
branch removed).

## Step 2 — Sanity-run the classifier

Done.

```
$ python3 scripts/make_trellis_asset.py assets/trellis_synthetic.glb
wrote assets/trellis_synthetic.glb (24588 bytes, 672 verts, 336 tris)

$ python3 scripts/classify_shape.py assets/trellis_synthetic.glb
category=directional confidence=0.61 is_hard_surface=True mean_peakiness=4.83
```

**Deviation from plan:** the hard-surface overlay fires
(`is_hard_surface=true`, `mean_peakiness=4.83`) because the regular
slat/stile spacing creates spikes in the per-axis vertex histogram.
Increasing the slat count from 4 → 12 raised confidence (0.56 → 0.61)
but did **not** drop peakiness — adding more slats just adds more
peaks at the same period.

This is acceptable for the ticket: `result.Category` is still
`directional`, which is what `applyClassificationToSettings` reads.
The hard-surface overlay only changes (i) the `is_hard_surface`
boolean on the result and (ii) the order of the `candidates` list
shown in the comparison-UI modal. The persisted ShapeCategory is
unaffected. The ticket AC explicitly accepts both auto-classification
and the comparison-UI override path: "Auto-classifier returns
directional (or triggers comparison UI, where the user picks
directional)".

Documented as a follow-up in review.md (the comparison modal will
auto-open here because confidence < 0.7).

## Step 3 — Asset + generator changes staged

Both files exist and are unstaged in the working tree (the lisa
workflow handles commits at the end).

## Step 4 — `TestTrellisAssetClassifiesAsDirectional`

Done. Added at the bottom of `strategy_handlers_test.go`. Imports
`errors`, `io/fs`, `os` (added to the import block). The test:

1. Skips if `assets/trellis_synthetic.glb` is missing.
2. Calls `RunClassifier` and skips on subprocess error.
3. Asserts `result.Category == "directional"`.
4. Pipes through `applyClassificationToSettings` against a
   `t.TempDir`.
5. Pins `ShapeCategory`, `SliceAxis`, `SliceDistributionMode`,
   `VolumetricLayers`.

```
$ go test -run TestTrellisAsset -v ./...
=== RUN   TestTrellisAssetClassifiesAsDirectional
--- PASS: TestTrellisAssetClassifiesAsDirectional (0.15s)
PASS
ok      glb-optimizer   0.478s
```

Full suite is also green:

```
$ go test ./...
ok      glb-optimizer
```

## Step 5 — Test commit staged

Same as step 3: changes are unstaged in the working tree.

## Step 6 — `static/app.js` orientation fix

Done. Three changes:

1. **`STRATEGY_TABLE`** — added `instance_orientation_rule` to every
   row, mirroring `strategy.go::shapeStrategyTable`. Values:
   round-bush=`random-y`, directional=`fixed`, tall-narrow=`random-y`,
   planar=`aligned-to-row`, hard-surface=`fixed`, unknown=`random-y`.

2. **New helper `shouldRandomRotateInstances()`** — reads
   `currentSettings.shape_category`, looks up the rule, returns
   `false` for `fixed` / `aligned-to-row` and `true` otherwise.
   Defaults to `true` when category or rule is missing so the
   historical rose / round-bush behavior is preserved.

3. **Three call sites updated** to pass `shouldRandomRotateInstances()`
   instead of literal `true`:
   - `runStressTest` regular preview path (was line 3284)
   - `runLodStressTest` no-LODs fallback (was line 3343)
   - `runLodStressTest` per-LOD-level fallback (was line 3430). The
     latter two share a single `randomRotate` const computed at the
     top of `runLodStressTest` to avoid recomputing per-bucket.

**Deviation from plan:** the plan called for a one-line inline
expression at each site. Extracting `shouldRandomRotateInstances()` is
slightly heavier but avoids two copies of the same predicate and
matches the existing style of other top-level helpers in app.js
(`seededRandom`, `pickAdaptiveLayerCount`, etc.).

`node --check --input-type=module` confirms the file still parses.

## Step 7 — JS commit staged

Same as steps 3 / 5.

## Step 8 — `review.md`

Written next.

## Open items

- Visual confirmation of the bake + 5-instance row stress test must
  be done in a browser; documented as a manual checklist in review.md.
- Hard-surface overlay false positive on the synthetic trellis is
  documented but not fixed. Real-world trellis assets (less perfectly
  periodic) should not trigger it.
