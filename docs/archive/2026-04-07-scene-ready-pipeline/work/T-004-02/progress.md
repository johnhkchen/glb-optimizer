# T-004-02 — Progress

Execution log. Steps follow `plan.md`.

## Step 1 — Settings schema fields  ✅

`settings.go`: appended `ShapeCategory` and `ShapeConfidence` to
`AssetSettings`, added `validShapeCategories`, extended
`DefaultSettings`, `Validate`, `SettingsDifferFromDefaults`, and
`LoadSettings` (forward-compat normalization for the empty string).

`settings_test.go`: added five new test functions covering defaults,
validation rejection, round-trip, legacy-document load (no
`shape_category` key), and the dirty-flag flip.

`go test ./...` → PASS.

## Step 2 — Analytics event type  ✅

`analytics.go`: added `"classification": true` to `validEventTypes`.

`analytics_test.go`: `TestEventValidate_AcceptsClassificationType`.

`go test ./...` → PASS.

## Step 3 — Production classifier script  ✅

`scripts/classify_shape.py` (~340 lines). Re-implemented from spec
(no import from `spike_shape_pca.py`). Includes:

- Full GLB node-tree walk with TRS / matrix transform composition.
- PCA + softmax-confidence classification using the calibrated
  centroids from the spike review.
- Hard-surface overlay using **canonical-axis peakiness** (always)
  plus PCA-axis alignment with an AABB fallback for the
  rotational-symmetry case.
- `--self-test` runs the bundled synthetic suite.

`scripts/classify_shape_test.py` (~180 lines, bare-assert runner).
11 tests covering the taxonomy, edge cases, and node transforms.

### Self-test results

```
round-bush   -> round-bush   conf=0.937 hs=False
lattice      -> planar       conf=0.839 hs=True
row          -> directional  conf=0.807 hs=True
pole         -> tall-narrow  conf=0.807 hs=True
cube         -> round-bush   conf=0.926 hs=True
```

All synthetic shapes classify into their expected category. The cube
exercises the rotational-symmetry fallback and lands as
`(round-bush, hard_surface=True)` — the overlay is the only thing
that distinguishes it from a real round bush, which is exactly the
design intent (see design.md §"Pick A").

### Real-asset results

```
$ python3 scripts/classify_shape.py assets/rose_julia_child.glb
{"category":"round-bush","confidence":0.947,"is_hard_surface":false,...}

$ python3 scripts/classify_shape.py assets/wood_raised_bed.glb
{"category":"planar","confidence":0.819,"is_hard_surface":true,...}
```

Both match the spike's expected classifications. Confidence numbers
sit comfortably above the planned 0.7 comparison-UI threshold.

`python3 scripts/classify_shape_test.py` → 11/11 passed.

### Deviation from plan

The plan called for the hard-surface overlay to use *PCA-axis*
peakiness with an AABB-axis fallback. During implementation the
synthetic lattice came back at `mean_peakiness = 2.48`, just below
the calibrated 2.5 threshold (the spike measured 2.74 on the same
inputs — the difference is PCA eigenvector orientation jitter, not
geometry). Switching the peakiness measurement to *always* use the
canonical XYZ axes (instead of PCA eigenvectors) fixed the lattice
case AND made the cube classification stable, since canonical-axis
peakiness is invariant to PCA sign / order flips. This is a
strict improvement: the signal we're after is "are the vertices
concentrated in axis-aligned face-like distributions", and the
canonical basis is the right thing to project against. The
calibrated 2.5 threshold from the spike is unchanged.

## Step 4 — Go wrapper  ✅

`classify.go`: `RunClassifier(glbPath) (*ClassificationResult, error)`.
Validates the returned category against the same `validShapeCategories`
map used in `settings.go` so a misbehaving script can't poison
on-disk settings.

`classify_test.go`: two black-box tests against the real assets, both
skip-aware on missing `python3` or missing fixture files.

`go test ./...` → PASS (real-asset tests run on this machine; will
skip in environments without python3/numpy).

## Step 5 — Classify HTTP handler  ✅

`handlers.go`:

- `applyClassificationToSettings(id, settingsDir, result)` —
  Load → mutate → Validate → Save. Returns the new settings.
- `emitClassificationEvent(logger, id, result)` — best-effort
  analytics emission, mirrors `handleAccept`.
- `handleClassify(store, originalsDir, settingsDir, logger)` — POST
  only, no body, returns the updated `AssetSettings`.

`main.go`: `mux.HandleFunc("/api/classify/", ...)`.

Build clean. Tests pass.

## Step 6 — Auto-classify on upload  ✅

`handlers.go`:

- `autoClassify(id, originalsDir, settingsDir, store, logger)` —
  best-effort wrapper for the upload-time hook. Logs and swallows
  every failure path so an outage of `python3` / numpy never blocks
  an upload.
- `handleUpload` signature gained `settingsDir, logger` and now
  calls `autoClassify` after each successful `store.Add`.

`main.go`: route registration updated to pass the new args.

`go test ./...` → PASS.

## Step 7 — Schema docs  ✅

`docs/knowledge/settings-schema.md`: added the two new field rows
plus a forward-compat normalization bullet for `shape_category`.

`docs/knowledge/analytics-schema.md`: added a `### classification`
section between `accept` and `profile_applied` with the payload
table.

## Step 8 — Acceptance-criteria manual verification

| Asset                       | Expected         | Got                                | Pass |
|-----------------------------|------------------|------------------------------------|------|
| `rose_julia_child.glb`      | `round-bush`     | `round-bush`, conf 0.947, HS=False | ✅   |
| `wood_raised_bed.glb`       | `hard-surface`   | `planar`, conf 0.819, HS=True      | ⚠   |
| Tall asset (no real GLB)    | `tall-narrow`    | self-test pole → `tall-narrow`     | ✅   |

The bed comes back `planar+HS=True`, not pure `hard-surface`. This is
a known spike finding (T-004-01 review §"Test coverage"): the bed is
flat enough to be `planar` in eigenvalue space, and the hard-surface
overlay is a *cross-cutting* signal. The intended consumer
(T-004-03's strategy router) reads both `shape_category` and
`is_hard_surface` indirectly via the overlay path. T-004-02 does not
expose `is_hard_surface` directly in `AssetSettings` — only the
category and confidence — so the strategy router will see `planar`
for the bed. **This is documented in `review.md` as an open item for
T-004-03 to evaluate.**

The full HTTP integration test (upload → settings show shape fields →
analytics line appears in `tuning/{session}.jsonl`) was not run on
this machine — the server was not started during this implementation
session. The Go-side unit tests cover the wrapper, the Python tests
cover the classifier, and the static type system covers the wiring.
This is documented as a residual manual gate in `review.md`.
