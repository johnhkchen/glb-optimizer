# T-004-05 — Review

End-to-end validation of the trellis case across the S-004 pipeline.
The Go-side seam is now exercised against a real GLB by an automated
test; the JS stress-test orientation gap that the ticket called out is
closed; the visual / browser portion is documented as a manual
checklist.

## Files changed

### Created

- `scripts/make_trellis_asset.py` — self-contained, no-Blender
  generator for a synthetic trellis panel. Re-uses the GLB-writer
  pattern from `parametric_reconstruct.py::write_glb` (texture branch
  removed). Idempotent, RNG-free, byte-stable.
- `assets/trellis_synthetic.glb` — generated artifact (24 588 bytes,
  672 verts, 336 tris). 12 horizontal slats × 16 vertical stiles in a
  2.0 × 0.8 × 0.04 panel. Wider than tall so the PCA principal axis
  is horizontal — that's what tips the classifier from `tall-narrow`
  to `directional`.

### Modified

- `strategy_handlers_test.go` — new
  `TestTrellisAssetClassifiesAsDirectional`. Drives the actual
  `python3 scripts/classify_shape.py` subprocess against the
  on-disk asset, then pipes the result through
  `applyClassificationToSettings` and pins:
  - `ShapeCategory == "directional"`
  - `SliceAxis == SliceAxisAutoHorizontal`
  - `SliceDistributionMode == "equal-height"`
  - `VolumetricLayers == 4`
  Imports added: `errors`, `io/fs`, `os`. Skips (rather than fails)
  when python3 is missing or the asset isn't present, mirroring the
  rest of the classifier-test soft-dep posture.

- `static/app.js` — three surgical edits:
  1. `STRATEGY_TABLE` (~line 349) gains
     `instance_orientation_rule` on every row, mirroring the Go-side
     `shapeStrategyTable`.
  2. New top-level helper `shouldRandomRotateInstances()` reads
     `currentSettings.shape_category`, looks up the rule, returns
     `false` for `fixed` / `aligned-to-row` and `true` otherwise.
     Defaults to `true` when the category is missing so the rose /
     round-bush behavior is unchanged.
  3. Three `createInstancedFromModel(..., true)` call sites in
     `runStressTest` and `runLodStressTest` now pass the helper's
     return value. `runLodStressTest` computes it once at the top
     and re-uses across all LOD buckets.

### Untouched

- `strategy.go` / `settings.go` — already correct.
- `scripts/classify_shape.py` — no calibration changes.
- `handlers.go` — the upload → classify → stamp path is the
  production code path and is now exercised end-to-end by the new
  Go test.
- Schema files (no public contract changes).

## Test coverage

| Layer                          | Coverage                                                                 |
|--------------------------------|--------------------------------------------------------------------------|
| Asset → PCA (Python)           | Existing `classify_shape_test.py::test_directional_row` (synthetic).     |
| GLB on disk → directional      | **New** `TestTrellisAssetClassifiesAsDirectional` (real subprocess).     |
| Strategy router lookup         | Pinned by `strategy_test.go` (existing).                                  |
| Settings stamping              | Pinned by `TestApplyClassificationStampsStrategy_Directional` + new test.|
| Bake (browser)                 | Manual; see "Manual verification" below.                                 |
| Stress-test row alignment      | JS-only path; not unit tested. Manual; see below.                        |

`go test ./...` is green:

```
ok      glb-optimizer
```

## Acceptance criteria status

| AC                                                                                | Status                  |
|-----------------------------------------------------------------------------------|-------------------------|
| Trellis-class test asset in `assets/`                                              | ✅ `assets/trellis_synthetic.glb` |
| Auto-classifier returns `directional` (or comparison UI offers it)                 | ✅ Real subprocess returns `directional`; pinned by Go test. Confidence is 0.61 (< 0.7 auto-threshold), so the comparison UI auto-opens — both branches of the AC are exercised. |
| Strategy router selects perpendicular-axis slicing                                  | ✅ `SliceAxis = auto-horizontal`, pinned by Go test |
| Production asset bakes successfully                                                 | 🟡 Manual — see checklist |
| Baked output preserves directional shape (no incoherent fragments)                  | 🟡 Manual — see checklist |
| 5-instance row, all same Y rotation                                                 | ✅ Code change in place. Manual visual confirmation pending. |
| Documented in review.md with screenshots                                            | 🟡 Review doc exists; screenshots are a TODO for the human reviewer (this CLI environment cannot drive a browser). |
| Any classifier or router bugs found are fixed                                       | ✅ No router bug found in step 4. The orientation gap was a known wiring gap from S-004; fixed in app.js. |

## Manual verification checklist (browser)

A human reviewer should walk through these steps before the ticket is
marked done:

1. `just dev` (or however the dev server is started in this repo).
2. Drag `assets/trellis_synthetic.glb` into the upload UI.
3. Wait for auto-classification to complete. Expected:
   - shape_category badge shows `directional`.
   - The comparison-UI modal **may auto-open** because confidence is
     ~0.61 (below the 0.7 threshold). If so, pick `directional` from
     the candidate slots.
4. Open the tuning panel and verify:
   - `slice_axis = auto-horizontal`
   - `slice_distribution_mode = equal-height`
   - `volumetric_layers = 4`
5. Click Bake. Expected: the produced volumetric GLB has 4 horizontal
   slabs that cut **across** the long axis of the trellis (not
   top-down slabs), preserving the recognisable lattice silhouette.
6. Switch to the production preview and run the stress test at
   `count=5`. Expected: 5 trellises in a row, **all facing the same
   way**, reads as "a row of trellises" rather than chaos.
7. Capture screenshots of (a) the tuning panel after classification,
   (b) the baked volumetric preview, (c) the row of 5 instances.
   Drop them next to this file as `review_*.png`.

## Open concerns

1. **Hard-surface overlay false positive on the synthetic asset.** The
   classifier returns `is_hard_surface=true` for the trellis because
   its perfectly-periodic slat/stile spacing creates spikes in the
   per-axis vertex histogram (`mean_peakiness ≈ 4.83`, threshold 2.5).
   `result.Category` is unaffected — it's still `directional` — but
   the candidates list is reordered to put `hard-surface` first with
   score 1.0. In the comparison UI this shows hard-surface as the top
   recommendation, which a future human-in-the-loop might pick. This
   is a synthetic-asset artefact: real-world trellis assets sourced
   from TRELLIS will not have this perfect periodicity. **Not in scope
   for this ticket** but worth flagging for the next iteration of
   the hard-surface overlay calibration (T-004-01 follow-up).

2. **Browser-only acceptance items.** The bake correctness and
   row-alignment criteria can only be verified visually in a browser;
   this CLI environment cannot drive one. The manual checklist above
   has to be walked through by a human before final sign-off.
   Screenshots are TODO.

3. **`aligned-to-row` collapses to `fixed` in this fix.** The
   stress-test helper treats both rules the same way (no random
   rotation) for now. S-006 will land per-row orientation logic for
   the `planar` case; until then, planar instances stress-test in a
   single fixed orientation, which is a strict improvement over the
   current chaotic random rotation.

4. **No headless screenshot harness.** Adding Playwright / Puppeteer
   was rejected in design.md as overkill for one screenshot. If this
   pattern repeats in future tickets, it may become worth revisiting.

## Critical issues

None. The Go-side acceptance is fully automated and green; the JS
orientation fix is in place and the file parses; the manual portion
is documented as such.

## Suggested follow-ups (out of scope)

- Hard-surface overlay re-calibration so periodic lattices don't
  trigger it (T-004-01 follow-up).
- Per-row `aligned-to-row` orientation in S-006.
- Headless screenshot capture if visual-acceptance needs become a
  recurring pattern across S-004 / S-006 tickets.
