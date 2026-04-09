# T-004-04 — Plan

Eight ordered, atomically-committable steps. Each step ends with a
verification gate. Steps 1–4 are backend; steps 5–8 are frontend.

## Step 1 — Python: emit `features.candidates`

**Files:** `scripts/classify_shape.py`, `scripts/classify_shape_test.py`

**Changes:**

- Refactor `classify()` to return `(best, confidence, ranking,
  distances)`.
- Add `_build_candidates(distances, is_hs, top_n=3)` helper. Reuses
  the same softmax temperature constant. Appends synthetic
  `hard-surface` entry with `score=1.0` when `is_hs=True`.
- `classify_points` writes `features["candidates"] =
  _build_candidates(distances, is_hs)`.
- New unit test `test_classify_points_emits_candidates` covering
  shape, ordering, and the hard-surface overlay branch.

**Verify:**

- `python3 scripts/classify_shape_test.py` — all tests pass
- `python3 scripts/classify_shape.py assets/rose_julia_child.glb`
  prints a JSON line with a non-empty `features.candidates` array
  whose first entry is `round-bush`.

**Commit:** `T-004-04: classifier emits candidates ranking`

## Step 2 — Go: register `classification_override` event type

**Files:** `analytics.go`, `analytics_test.go`

**Changes:**

- Add `"classification_override": true` to `validEventTypes`.
- New test `TestEventValidate_AcceptsClassificationOverrideType`
  mirroring the strategy_selected test.

**Verify:** `go test ./... -run EventValidate` passes.

**Commit:** `T-004-04: register classification_override event type`

## Step 3 — Go: handler reshape + override branch

**Files:** `handlers.go`

**Changes:**

- Define unexported `candidate` struct (`Category string; Score
  float64`).
- New unexported helper `extractCandidates(features
  map[string]interface{}) []candidate`. Returns `nil` on missing or
  malformed input.
- New helper `emitClassificationOverrideEvent(logger, id,
  origCat, origConf, result)` mirroring `emitClassificationEvent`.
- Rewrite `handleClassify`:
  - Read `override := r.URL.Query().Get("override")`.
  - Run classifier first (always — features must be current).
  - If override is non-empty: validate against
    `validShapeCategories`, capture `origCat`/`origConf`, then
    overwrite `result.Category` and `result.Confidence = 1.0`.
  - Call `applyClassificationToSettings` (unchanged signature).
  - Update `store` dirty flag (unchanged).
  - Emit either `classification_override` (override branch) or
    `classification` (normal branch).
  - Always emit `strategy_selected`.
  - Respond with `{"settings": s, "candidates":
    extractCandidates(result.Features)}`.
- `autoClassify` is **not** changed.

**Verify:** `go build ./...` and `go vet ./...` clean.

**Commit:** `T-004-04: handleClassify supports override + returns candidates`

## Step 4 — Go: end-to-end test of override path

**File:** `strategy_handlers_test.go`

**Changes:**

- New test
  `TestApplyClassificationOverride_StampsStrategyAndPreservesOverrides`.
  Builds an `AssetSettings` in a temp settings dir with a
  user-customized `SliceDistributionMode` (e.g.
  `"equal-height"`). Constructs a synthetic
  `*ClassificationResult` with `Category="directional"`,
  `Confidence=1.0`, an empty features map. Calls
  `applyClassificationToSettings`. Asserts `ShapeCategory =
  "directional"`, `ShapeConfidence == 1.0`, `SliceAxis ==
  "auto-horizontal"` (stamped because still default), and
  `SliceDistributionMode == "equal-height"` (preserved).

**Verify:** `go test -count=1 ./... -run Override` passes; full
`go test ./...` stays green.

**Commit:** `T-004-04: end-to-end test for classification override`

## Step 5 — Docs: analytics-schema event section

**File:** `docs/knowledge/analytics-schema.md`

**Changes:**

- New `### classification_override` section between
  `### classification` and `### strategy_selected`. Documents
  the payload table:
  `original_category`, `original_confidence`, `candidates`,
  `chosen_category`, `features`. Includes a JSON example.
- Update `### classification` payload table to mention that
  `features.candidates` is now an additive field carrying the
  per-category ranking; downstream consumers should treat it as
  optional.
- One-line addition to `docs/knowledge/settings-schema.md`
  noting that `shape_confidence == 1.0` indicates a human
  override (under the existing `shape_confidence` row).

**Verify:** Markdown renders. No code change.

**Commit:** `T-004-04: document classification_override event`

## Step 6 — Frontend: modal markup + Reclassify button

**File:** `static/index.html`

**Changes:**

- Add `<button id="tuneReclassifyBtn">Reclassify…</button>` row +
  `<span id="tuneShapeCategoryHint">` inside `tuningSection`,
  before `tuneResetBtn`.
- Add `<div id="comparisonModal">` markup at the bottom of
  `<div class="app">`.

**Verify:** Reload the page, modal is hidden, Reclassify button
visible at the bottom of the tuning panel.

**Commit:** `T-004-04: comparison modal markup + Reclassify button`

## Step 7 — Frontend: modal styles

**File:** `static/style.css`

**Changes:** Append a Comparison Modal section. Rules for `.modal`,
`.modal-backdrop`, `.modal-card`, `.comparison-slots`,
`.comparison-slot`, slot `<img>`, slot button, `.shape-hint`. Use
existing color variables.

**Verify:** Toggle `display:flex` on `#comparisonModal` in devtools;
modal centers, slots align horizontally.

**Commit:** `T-004-04: comparison modal styles`

## Step 8 — Frontend: comparison modal JS + selectFile hook

**File:** `static/app.js`

**Changes:**

- Add JS-side `STRATEGY_TABLE` constant near the top of the file
  next to `makeDefaults` (mirror of `strategy.go`).
- New helper `fetchClassification(id, overrideCategory=null)`.
- New helpers `openComparisonModal`, `renderCandidateThumbnail`,
  `pickCandidate`, `closeComparisonModal`.
- Wire the Reclassify button in `wireTuningUI` (or alongside the
  init code). Add `tuneShapeCategoryHint` refresh to
  `populateTuningUI`.
- Hook auto-open into the model-loaded callback inside
  `selectFile`: after `loadModel` resolves and `currentModel` is
  set, if `currentSettings.shape_confidence > 0 &&
  currentSettings.shape_confidence < 0.7`, fire-and-forget
  `fetchClassification` + `openComparisonModal`.
- Cancel button + backdrop click: call `closeComparisonModal`. No
  analytics event.

**Verify:**

- `python3 scripts/classify_shape_test.py` and `go test ./...` still
  green (no Go changes in this step but a sanity-check is cheap).
- Manual: described in §"Manual verification" below.

**Commit:** `T-004-04: comparison modal JS + auto-open hook`

## Manual verification

The single load-bearing acceptance check from the ticket. Runs
end-to-end against the live UI.

1. Build and start the server: `go build && ./glb-optimizer`.
2. Browser opens `http://localhost:8787`.
3. **Auto-open path:** upload an ambiguous asset — a tall but
   bushy plant works, but a synthetic ambiguous shape is also
   acceptable in the absence of one in `assets/`. Specifically:
   - Upload the asset; observe the file enters the list.
   - Open the browser console and confirm
     `currentSettings.shape_confidence < 0.7` after the upload's
     auto-classify completes.
   - Click the asset to select it. The model loads in the preview
     area, then within ~5 seconds the comparison modal opens with
     2–3 candidate thumbnails labeled with their categories.
4. **Pick path:** click "Pick {category}" on one thumbnail.
   - Modal closes.
   - `currentSettings.shape_category` equals the chosen category.
   - `currentSettings.shape_confidence === 1.0`.
   - The tuning panel "Reclassify…" button's hint text reflects the
     new category.
5. **Analytics:** locate the asset's session JSONL under
   `~/.glb-optimizer/tuning/`. Confirm a `classification_override`
   event with `chosen_category` matching the pick, a non-empty
   `candidates` array, and the original category preserved as
   `original_category`.
6. **Re-selection skip:** select a different asset, then re-select
   the just-resolved asset. The modal does **not** auto-open
   (because `shape_confidence == 1.0` now).
7. **Manual reclassify:** with that same asset still selected, click
   "Reclassify…" in the tuning panel. The modal opens regardless
   of the high confidence. Cancel without picking. Confirm the
   asset's `currentSettings` are unchanged.
8. **High-confidence asset:** upload `assets/rose_julia_child.glb`.
   Auto-classify produces `round-bush` with confidence > 0.7.
   Selecting it does **not** auto-open the modal. The Reclassify
   button still works.
9. **Hard-surface:** upload `assets/wood_raised_bed.glb`. Per
   T-004-02 it classifies as `planar` geometrically with
   `is_hard_surface=true`. Confidence is in the auto-open range.
   Modal opens. The candidate list contains both `planar` and
   `hard-surface`. Pick `hard-surface`. Confirm the slice fields on
   the resulting settings are `n/a` per the strategy router.

## Test strategy summary

| Layer            | Where                                | What                                                |
|------------------|--------------------------------------|-----------------------------------------------------|
| Python unit      | `classify_shape_test.py`             | candidates shape, ordering, hard-surface overlay    |
| Go unit          | `analytics_test.go`                  | new event type passes envelope validate             |
| Go integration   | `strategy_handlers_test.go`          | override stamping + override-preservation invariant |
| Frontend         | none (no JS test harness)            | manual verification steps 3–9                       |

The Python test guards the schema commitment. The Go tests guard
the override semantics and the analytics envelope. The frontend is
covered by the manual gate — same posture as T-004-03.

## Risks and mitigations

- **Sequential bake is slow.** Mitigated by per-slot "Rendering…"
  placeholders and the ticket's explicit acceptance of "a few
  seconds". Hard-surface skips the bake entirely.
- **`STRATEGY_TABLE` JS / Go drift.** Mitigated by inline doc
  comment pointing at `strategy.go`. Same posture as the existing
  Python `VALID_CATEGORIES` duplication.
- **Modal opens before model loaded.** Mitigated by hooking into the
  `loadModel` resolution, not `selectFile`'s top-level flow.
- **Override fails silently.** Mitigated by the modal staying open
  on `pickCandidate` HTTP error, with an inline error message.
- **Repeat auto-open on every selection.** Mitigated by the
  `shape_confidence == 1.0` skip-sentinel.
