# T-004-02 — Plan

Step-by-step execution plan. Each step is small enough to commit
atomically and verify in isolation.

## Step 1 — Settings schema fields

Modify `settings.go`:

- Append `ShapeCategory string` and `ShapeConfidence float64` to
  `AssetSettings` (declaration order = JSON order; appending preserves
  prefix order).
- Add `validShapeCategories` map: round-bush, directional, tall-narrow,
  planar, hard-surface, unknown.
- `DefaultSettings()`: `ShapeCategory: "unknown"`.
- `Validate()`: enum check + `[0,1]` range on confidence.
- `SettingsDifferFromDefaults()`: include both fields.
- `LoadSettings()`: forward-compat normalization — empty
  `ShapeCategory` → `"unknown"`.

Add to `settings_test.go`:

- `TestValidate_RejectsBadShapeCategory`
- `TestValidate_RejectsConfidenceOutOfRange`
- `TestSaveLoad_RoundtripShapeFields`
- `TestLoad_OldDocMissingShapeCategory_DefaultsToUnknown`
- `TestSettingsDifferFromDefaults_FlipsOnShapeFields`

**Verification**: `go test ./...` passes. **Commit.**

## Step 2 — Analytics event type

Modify `analytics.go`:

- Add `"classification": true` to `validEventTypes`.

Add to `analytics_test.go`:

- `TestEventValidate_AcceptsClassificationType`.

Modify `docs/knowledge/analytics-schema.md`:

- New `### classification` section with payload table:
  `category` (enum string, required), `confidence` (float, required),
  `features` (object, required, schema documented but not validated).

**Verification**: `go test ./...` passes. **Commit.**

## Step 3 — Production classifier script

Create `scripts/classify_shape.py`. Per `structure.md`:

- GLB loader walks node tree, composes transforms, concatenates
  positions.
- PCA + softmax confidence + AABB-fallback hard-surface overlay.
- Calibrated centroids and thresholds from T-004-01 review.
- `--self-test` mode prints a table for the bundled synthetic suite.
- Single-input mode prints exactly one JSON line on stdout.

Create `scripts/classify_shape_test.py` covering the cases in
`structure.md`. Bare-`assert` style (no pytest dep).

**Verification (manual)**:

```
python3 scripts/classify_shape.py --self-test
python3 scripts/classify_shape.py assets/rose_julia_child.glb
python3 scripts/classify_shape.py assets/wood_raised_bed.glb
python3 scripts/classify_shape_test.py
```

Expectations (from spike calibration):
- rose → `round-bush`, conf ≳ 0.7
- bed → `planar`, `is_hard_surface=true`
- self-test cube → `is_hard_surface=true`
- self-test pole → `tall-narrow`

**Commit.**

## Step 4 — Go wrapper

Create `classify.go` with `RunClassifier(glbPath) (*ClassificationResult, error)`:

- `exec.Command("python3", "scripts/classify_shape.py", glbPath)`.
- Capture stdout via `.Output()`; on `*exec.ExitError`, surface
  `string(err.Stderr)` in the wrapped error.
- `json.Unmarshal` into `ClassificationResult`.
- Validate the returned `Category` against the same enum used in
  `settings.go` (defensive: a script that prints an unknown category
  must not poison settings on disk).

Create `classify_test.go`:

- `TestRunClassifier_Rose` — runs on `assets/rose_julia_child.glb`
  if present, otherwise `t.Skip`. Skips entirely if `python3` not on
  PATH.
- `TestRunClassifier_RejectsUnknownCategory` — uses a fake script
  via `t.TempDir()` PATH manipulation. Optional; skip if too fragile.

**Verification**: `go test ./...` passes (test will skip when
python3/asset missing — that is fine for CI).

**Commit.**

## Step 5 — Classify HTTP handler

Modify `handlers.go`:

- New `handleClassify(store, originalsDir, settingsDir, logger)` per
  the spec in `structure.md`.
- Helper `applyClassificationToSettings(id, settingsDir, result)` that
  loads, mutates, validates, saves. Reusable from both the explicit
  endpoint and the upload-time hook.
- Helper `emitClassificationEvent(logger, id, result)` that does the
  `LookupOrStartSession + AppendEvent` dance and swallows errors to
  stderr (mirrors `handleAccept`).

Modify `main.go`:

- Mount `/api/classify/` route.

**Verification**: build clean, manual `curl -X POST
http://localhost:8787/api/classify/<id>` against a running server with
an existing asset returns the populated `AssetSettings`. Verify the
`settings/{id}.json` file gets the new fields. Verify a
`classification` line appears in `tuning/{session}.jsonl`.

**Commit.**

## Step 6 — Auto-classify on upload

Modify `handlers.go`:

- `handleUpload` signature gains `settingsDir` and `logger`.
- After each successful `store.Add`, call a small inline helper:
  - `RunClassifier(destPath)`. On error, log to stderr with the
    asset id and continue.
  - On success, `applyClassificationToSettings(...)`. On error,
    log to stderr and continue.
  - On success, `emitClassificationEvent(...)`.

Modify `main.go`:

- `handleUpload(store, originalsDir, settingsDir, analyticsLogger)`.

Tests: a Go-side end-to-end test for the upload path is heavyweight
(needs python3 + a real GLB on disk). Manual verification only,
documented in `progress.md`.

**Verification (manual)**:

```
1. Start server.
2. POST a GLB to /api/upload.
3. GET /api/settings/<id> — confirm shape_category and shape_confidence
   are populated.
4. tail tuning/<session>.jsonl — confirm a classification line exists.
```

**Commit.**

## Step 7 — Schema docs

Modify `docs/knowledge/settings-schema.md`:

- Add `shape_category` and `shape_confidence` rows to the field table.
- Note that these are populated by the classifier (`/api/classify/:id`
  or auto on upload), not the user.
- Add a forward-compat normalization bullet for `shape_category`.

Modify `docs/knowledge/analytics-schema.md` (already done in step 2,
but double-check the cross-reference to settings).

**Verification**: docs render in Obsidian; cross-references resolve.

**Commit.**

## Step 8 — Manual verification per acceptance criteria

Per ticket §"Manual verification":

1. `python3 scripts/classify_shape.py assets/rose_julia_child.glb`
   → expect `round-bush`.
2. `python3 scripts/classify_shape.py assets/wood_raised_bed.glb`
   → expect `planar` + `is_hard_surface=true`. (Acceptance criteria
   says "expect hard-surface"; the spike already determined the bed
   is `planar` with the HS overlay, and the review explicitly calls
   this out as "both signals fired correctly". Document the
   discrepancy in `progress.md` and `review.md`: this is a known
   spike finding, not a regression.)
3. Tall asset: `python3 scripts/classify_shape.py --self-test` and
   note the synthetic pole's `tall-narrow` classification, since the
   repo has no real tree-trunk GLB. Document in `review.md`.
4. POST a fresh upload of the rose to a running server, then GET
   `/api/settings/<id>` and confirm the fields appear.

Record outputs in `progress.md`.

## Testing strategy summary

| Layer       | What's tested                                          | Where                          |
|-------------|--------------------------------------------------------|--------------------------------|
| Python unit | classifier on synthetic shapes; transform composition  | `scripts/classify_shape_test.py` |
| Python e2e  | classifier on real assets                              | `--self-test` mode             |
| Go unit     | settings round-trip, validation, normalization         | `settings_test.go`             |
| Go unit     | analytics event type                                   | `analytics_test.go`            |
| Go integration | `RunClassifier` against a real GLB                  | `classify_test.go` (skip-aware) |
| Manual      | upload → classify → settings → analytics line         | step 8                         |

The manual verification gate is the load-bearing one for the
acceptance criteria; the unit suite is the regression net.

## Known deviations to document on the way

- Hard-surface bed comes back `planar+HS=Y`, not pure `hard-surface`.
  Spike-known.
- Tall-narrow manual test uses `--self-test`'s synthetic pole until
  a real trunk asset lands.
- No backend-side verification that `python3` is on PATH at startup.
  Classifier failures degrade to `unknown` per the design.
