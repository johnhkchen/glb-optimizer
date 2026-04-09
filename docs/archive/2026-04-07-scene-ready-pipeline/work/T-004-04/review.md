# T-004-04 — Review

Self-assessment of the multi-strategy comparison UI work.

## Summary

The classifier now surfaces a per-category candidate ranking; the
classify HTTP endpoint exposes that ranking and accepts an override
mode; the frontend opens a modal when confidence is low, bakes
2–3 candidate strategies as thumbnails, and turns the user's pick
into a `classification_override` analytics event. Every resolved
comparison becomes a labeled training example linking
`(asset features → human-preferred strategy)` — the most valuable
training signal in S-004.

## What changed

### Files created

- `docs/active/work/T-004-04/{research,design,structure,plan,progress,review}.md`
  — RDSPI artifacts.

### Files modified

- **`scripts/classify_shape.py`** — `classify()` now returns the
  raw distances dict alongside the existing ranking; new helper
  `_build_candidates(distances, is_hs, top_n=3)` softmax-normalizes
  and sorts; `classify_points` writes the result into
  `features["candidates"]`. The hard-surface overlay always wins
  with score `1.0` when set, addressing the T-004-02 wood-bed open
  concern (planar geometrically but hard-surface by overlay).
- **`scripts/classify_shape_test.py`** — two new tests:
  `test_classify_points_emits_candidates_ranking` (shape, ordering,
  enum, top entry matches `result["category"]`) and
  `test_classify_points_candidates_promotes_hard_surface_overlay`.
- **`analytics.go`** — `"classification_override"` registered as a
  v1 event type.
- **`analytics_test.go`** —
  `TestEventValidate_AcceptsClassificationOverrideType`.
- **`handlers.go`** — three new pieces:
  1. `candidate` struct + `extractCandidates(features)` helper
     that pulls the typed list out of the opaque feature map.
  2. `emitClassificationOverrideEvent` mirroring
     `emitClassificationEvent`, with the canonical training-data
     payload (`original_category`, `original_confidence`,
     `candidates`, `chosen_category`, `features`).
  3. `handleClassify` reshape: response is now
     `{settings, candidates}`; `?override=<category>` query
     switches into override mode (re-runs classifier so features
     are current, replaces category, pins confidence to 1.0,
     emits the override event in lieu of the normal one). The
     stamping path is unchanged — `applyClassificationToSettings`
     handles both branches identically.
- **`strategy_handlers_test.go`** — two new tests:
  `TestApplyClassificationOverride_StampsStrategyAndPreservesOverrides`
  end-to-end on the override path (stamps strategy on still-default
  fields, preserves user-customized SliceDistributionMode, persists
  ShapeConfidence=1.0); table-driven `TestExtractCandidates`
  covering missing-key, happy-path, empty-category, and wrong-type
  branches.
- **`docs/knowledge/analytics-schema.md`** — new
  `### classification_override` section with payload table and
  JSON example; classification's features paragraph extended to
  mention the new optional `features.candidates` array.
- **`docs/knowledge/settings-schema.md`** — `shape_confidence` row
  extended to document the `1.0` human-confirmed sentinel.
- **`static/index.html`** — Reclassify… button + hint span inside
  the tuning section; comparison modal markup at the bottom of
  `<div class="app">`.
- **`static/style.css`** — `── T-004-04: Comparison modal ──`
  section: `.modal`, `.modal-backdrop`, `.modal-card`,
  `.comparison-slots`, `.comparison-slot`, slot button,
  `.comparison-error`, `.shape-hint`. ~115 lines, all using
  existing color variables.
- **`static/app.js`** — the bulk of the user-visible work:
  - `STRATEGY_TABLE` JS-side mirror of `strategy.go`.
  - `fetchClassification(id, overrideCategory)` — single source
    of truth for the endpoint.
  - `openComparisonModal`, `renderCandidateThumbnail`,
    `renderModelToCanvas`, `_replaceThumbWithCanvas`,
    `pickCandidate`, `closeComparisonModal`.
  - `loadModel` returns a Promise so `selectFile` can sequence
    the auto-reclassify path after `currentModel` is populated.
    Backwards-compatible (existing call sites ignore the return).
  - `selectFile` low-confidence auto-open: triggers when
    `shape_confidence ∈ (0, 0.7)`. Skipped when ` 0` (never
    classified) or `1.0` (human-confirmed sentinel) — re-prompt
    suppression.
  - `wireTuningUI` Reclassify… button handler, modal cancel +
    backdrop close handlers.
  - `populateTuningUI` refreshes `tuneShapeCategoryHint`.

### Files deleted

None.

## Test coverage

| Layer            | Where                                  | Status                          |
|------------------|----------------------------------------|---------------------------------|
| Python unit      | `classify_shape_test.py`               | 13/13 passing (was 11)          |
| Go unit (analytics envelope) | `analytics_test.go`        | passing                         |
| Go unit (extractCandidates)  | `strategy_handlers_test.go` | 4 sub-tests, passing           |
| Go integration (override stamping) | `strategy_handlers_test.go` | passing                  |
| Frontend         | none (no JS test harness)              | covered by manual verification  |

`go test -count=1 ./...` and `python3 scripts/classify_shape_test.py`
both pass cleanly.

### Tests not added

- **No HTTP-level test of `handleClassify` override branch.** Same
  posture as T-004-02/03: the `handleClassify` body is a thin
  composition over `RunClassifier`, `applyClassificationToSettings`,
  and the `emit*` helpers, all of which are individually exercised.
  Adding an end-to-end HTTP test would require mocking
  `RunClassifier` (which shells out to Python) — left as a manual
  gate.
- **No direct test for `emitClassificationOverrideEvent`.** Mirrors
  `emitClassificationEvent` (which itself has no direct test); the
  failure mode is "stderr log on best-effort failure".
- **No frontend tests.** Same constraint as every prior frontend
  ticket on this project. The comparison modal flow is covered by
  the manual verification step in plan.md §"Manual verification".

## Open concerns

1. **Manual verification not yet performed.** The full multi-asset
   walkthrough (auto-open, pick, analytics inspection,
   re-selection skip, manual Reclassify, high-confidence skip,
   hard-surface) is documented in plan.md and is the load-bearing
   acceptance check. Same posture as T-004-03's open concern #1 —
   needs a human pass before the ticket is mergeable.

2. **`STRATEGY_TABLE` (JS) duplicates `shapeStrategyTable` (Go).**
   Same drift risk as the existing Python ↔ Go duplication of
   `validShapeCategories` and the JS ↔ Go duplication of
   `makeDefaults` ↔ `DefaultSettings()`. Mitigation is the inline
   doc comment pointing at `strategy.go`. A future ticket could
   serve the table from `/api/status` or from a generated file,
   but it's a lot of plumbing for a 6-row constant.

3. **Per-candidate bake is expensive.** Worst case 3 sequential
   `renderHorizontalLayerGLB` calls + GLTF roundtrip + render-to-
   canvas, each 1–3 seconds. Acceptable per ticket scope, but
   the modal becomes unresponsive (no spinner beyond the
   "Rendering…" placeholder text) during the renders. If the
   user clicks "Cancel" while a slot is mid-bake, the bake
   continues to completion and then writes into a no-longer-
   visible thumbnail — wasteful but harmless. A future
   AbortController-style cancellation would be a nice
   improvement but is out of scope.

4. **Hard-surface candidate fallback is a flat render of the
   model, not the parametric output.** The parametric pipeline
   (S-001) is not yet integrated with this preview path, so the
   thumbnail shows the raw model with a "(parametric — no
   slicing)" caption. This is honest about the strategy (no
   slicing happens) but does not actually preview the parametric
   reconstruction. When S-001 lands a callable parametric
   reconstructor, this branch should call into it.

5. **Auto-open trigger races a fast user.** The auto-reclassify
   `selectFile` hook fires after `loadModel` resolves. If the user
   clicks a different asset before the previous model finishes
   loading + the network round-trip + the modal opens, we check
   `selectedFileId !== id` before mutating state — but the
   open-modal call could still slip through if the user clicks
   *during* the `await openComparisonModal(...)` itself. The
   guard is in two places (before classify and after) but not
   inside the modal's per-slot loop. Acceptable for v1 — the
   user just sees a stale modal they can dismiss.

6. **`shape_confidence == 0` never auto-opens.** This is
   intentional: confidence-zero means the asset has not been
   classified at all (legacy file or classifier outage on
   upload). The user can always click Reclassify… to force the
   path. Worth flagging because a naive read of the threshold
   logic might assume all "low confidence" assets prompt.

7. **Override branch always re-runs the classifier.** Even though
   the user has already chosen a category, the classifier runs so
   the persisted strategy is stamped against current geometry and
   the override event carries fresh features. This is the right
   trade — features are the load-bearing training signal — but
   it means picking is ~1s slower than a pure database write.
   Acceptable.

## Critical issues for human attention

- **Run the manual verification before merging** (concern #1).
  Specifically, the auto-open + pick + analytics-event sequence
  on a low-confidence asset is the only end-to-end exercise of
  the new HTTP shape, the modal lifecycle, and the override
  stamping path. Without it the ticket has not been observed
  end-to-end.
- **`assets/` does not currently contain a known low-confidence
  asset.** T-004-02 found that `wood_raised_bed.glb` classifies
  as `planar` with the hard-surface overlay set; check what its
  measured confidence is — if it's > 0.7, the human verifier
  needs to either lower `COMPARISON_AUTO_THRESHOLD` temporarily
  or use the manual Reclassify… path instead.

## Build / test status

- `go build ./...` — clean.
- `go vet ./...` — clean.
- `go test -count=1 ./...` — green (5 new tests across
  `analytics_test.go` and `strategy_handlers_test.go`).
- `python3 scripts/classify_shape_test.py` — 13/13 passing.
- `node --check static/app.js` — clean.

## Acceptance criteria — status

| Criterion                                                                              | Status |
|----------------------------------------------------------------------------------------|--------|
| `POST /api/classify/:id` returns confidence < 0.7 → frontend opens comparison view     | done (auto-open in `selectFile` low-confidence branch) |
| Comparison view renders 2–3 candidate strategies as thumbnails (~256px)                | done (modal slots, `renderCandidateThumbnail` + `renderModelToCanvas`) |
| Picking a strategy sets `shape_category` to chosen value                               | done (override branch of `applyClassificationToSettings`) |
| Picking a strategy sets `shape_confidence` to 1.0                                      | done (override branch pins confidence) |
| Analytics: `classification_override` event with original/candidates/chosen + features  | done (`emitClassificationOverrideEvent`) |
| Comparison view accessible via "Reclassify…" in tuning panel                           | done (`tuneReclassifyBtn` wiring) |
| Thumbnails generated via existing offscreen render pipeline                            | done (`renderHorizontalLayerGLB` + `renderModelToCanvas`) |
| Manual verification: ambiguous asset → modal opens → pick → override stored + event    | **pending human verification** |

## Handoff notes

- The `features.candidates` array is the contract for any future
  consumer that wants the per-asset ranking. It is *additive* and
  *optional* — old classification events on disk do not have it,
  and downstream readers must tolerate `null`.
- The `classification_override` event is the primary training data
  for the S-004 strategy router. Pair it with the preceding
  `classification` event (same `asset_id`, earlier `timestamp`) to
  reconstruct the `(classifier_pick, human_pick)` decision pair.
- The override branch of `handleClassify` is the *only* path that
  persists `shape_confidence == 1.0`. Anything else writing 1.0
  into settings should be regarded as a bug; the sentinel is
  reserved for human confirmations.
- `STRATEGY_TABLE` in `static/app.js` and `shapeStrategyTable` in
  `strategy.go` must be edited together when the S-004 taxonomy
  evolves.
