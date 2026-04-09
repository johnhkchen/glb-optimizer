# T-004-04 — Structure

## File-level changes

### `scripts/classify_shape.py` (modified)

Add a top-N candidates list to `classify_points`'s feature dict.

- New helper `_build_candidates(distances, is_hs, top_n=3) -> list`:
  takes the `distances` dict computed inside `classify()`, normalizes
  the distances into a softmax (re-using `CONFIDENCE_TEMPERATURE`),
  sorts descending, optionally appends a synthetic `hard-surface`
  entry with score `1.0` when `is_hs` is true (so the overlay always
  beats whatever the geometric classifier picked — matches the
  T-004-02 open concern that a hard-surface bed measures as planar
  geometrically). Truncated to top-N.
- `classify()` is modified to return `(best, confidence, ranking,
  distances)` so `classify_points` has the raw distances for the
  helper.
- `classify_points` calls `_build_candidates(distances,
  is_hs)` and writes the result into `features["candidates"]`.

No public API change beyond the additional `features.candidates`
key. The unit-test runner stays bare-asserts; one new test asserts
shape + ordering.

### `scripts/classify_shape_test.py` (modified)

One new test:
`test_classify_points_emits_candidates`. Calls `classify_points` on
synthetic round-bush points; asserts:

- `result["features"]["candidates"]` is a list of length ≤3
- each entry is `{"category": str, "score": float}` with score in
  `[0, 1]`
- `category` is in `VALID_CATEGORIES`
- entries are sorted descending by `score`
- the top entry's category equals `result["category"]`

### `classify.go` (no change)

`ClassificationResult.Features` is `map[string]interface{}` and
already round-trips arbitrary keys.

### `handlers.go` (modified)

Two reshaped handlers and one new helper.

#### `handleClassify` — new response shape and override branch

```go
func handleClassify(store, originalsDir, settingsDir, logger) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        // ... existing method/id checks ...
        override := r.URL.Query().Get("override")  // "" = normal classify

        result, err := RunClassifier(glbPath)
        if err != nil { /* 500 */ }

        if override != "" {
            if !validShapeCategories[override] {
                jsonError(w, 400, "unknown override category")
                return
            }
            originalCategory := result.Category
            originalConfidence := result.Confidence
            // Synthesize the override result. Re-use the just-measured
            // features so the persisted state and the emitted event
            // both reflect the current geometry, not stale data.
            result.Category = override
            result.Confidence = 1.0
        }

        s, err := applyClassificationToSettings(...)
        // ... existing dirty/store update ...

        if override != "" {
            emitClassificationOverrideEvent(logger, id, originalCategory, originalConfidence, result)
        } else {
            emitClassificationEvent(logger, id, result)
        }
        emitStrategySelectedEvent(logger, id, getStrategyForCategory(result.Category))

        candidates := extractCandidates(result.Features)  // []candidate
        jsonResponse(w, 200, map[string]interface{}{
            "settings":   s,
            "candidates": candidates,
        })
    }
}
```

The `originalCategory` / `originalConfidence` capture happens before
the synthetic mutation so the override event preserves the
classifier's actual prior decision.

#### `extractCandidates(features map[string]interface{}) []candidate`

Pure helper. Reads `features["candidates"]` if present, type-asserts
each entry, and returns a typed slice. Returns `nil` (encoded as
`null` in JSON) if the key is missing or the shape is unexpected.
This is the seam between the opaque Python feature dump and the
typed HTTP response.

```go
type candidate struct {
    Category string  `json:"category"`
    Score    float64 `json:"score"`
}
```

#### `emitClassificationOverrideEvent(logger, id, origCat, origConf, result *ClassificationResult)`

Mirrors `emitClassificationEvent` (handlers.go:737). Payload:

```go
map[string]interface{}{
    "original_category":   origCat,
    "original_confidence": origConf,
    "candidates":          result.Features["candidates"], // forward as-is
    "chosen_category":     result.Category,               // == override
    "features":            result.Features,
}
```

Best-effort: stderr-log on session lookup or append failure, no
HTTP error.

### `analytics.go` (modified)

```go
var validEventTypes = map[string]bool{
    // ... existing ...
    "classification_override": true, // T-004-04
}
```

### `analytics_test.go` (modified)

One new test mirroring
`TestEventValidate_AcceptsStrategySelectedType`:
`TestEventValidate_AcceptsClassificationOverrideType`.

### `strategy_handlers_test.go` (modified)

One new end-to-end test
`TestApplyClassificationOverride_StampsStrategyAndPreservesOverrides`:
seeds settings with a custom slice_distribution_mode, simulates an
override-path call into `applyClassificationToSettings` with
`Confidence=1.0` and the chosen category, and asserts:

- `s.ShapeCategory` == chosen
- `s.ShapeConfidence` == 1.0
- `s.SliceAxis` == strategy's slice_axis (stamped because still
  default)
- `s.SliceDistributionMode` == user value (preserved)

This is a *direct* test of the helper, not an HTTP test — same
posture as the existing handlers tests.

### `docs/knowledge/analytics-schema.md` (modified)

New `### classification_override` section under
`### classification`. Documents the payload table; adds one line in
the §"Out of scope" section deletion (none) and notes the new event
type alongside the existing list (none — additive only).

### `docs/knowledge/settings-schema.md` (no change)

No new fields. Override semantics live in the existing
`shape_category` / `shape_confidence` rows: the latter doc note
already says "0 on a never-classified asset"; add one bullet
clarifying that "1.0 indicates a human override".

### `static/index.html` (modified)

Two additions:

1. Inside `<div id="tuningSection">`, just above `tuneResetBtn`:

   ```html
   <div class="setting-row">
       <button class="preset-btn" id="tuneReclassifyBtn">Reclassify…</button>
       <span id="tuneShapeCategoryHint" class="shape-hint"></span>
   </div>
   ```

   The hint span shows the current `shape_category (confidence)` so
   the user knows what they're about to override.

2. At the end of `<div class="app">` (before the import-map script):

   ```html
   <div id="comparisonModal" class="modal" style="display:none">
       <div class="modal-backdrop"></div>
       <div class="modal-card">
           <h2>Pick the best strategy</h2>
           <p class="modal-subtitle" id="comparisonSubtitle"></p>
           <div class="comparison-slots" id="comparisonSlots"></div>
           <div class="modal-actions">
               <button id="comparisonCancelBtn">Cancel</button>
           </div>
       </div>
   </div>
   ```

   Slot DOM is built dynamically from JS so the slot count matches
   the candidate count (1–3).

### `static/style.css` (modified)

Append a `── Comparison modal ──` section. Rules for `.modal`,
`.modal-backdrop`, `.modal-card`, `.comparison-slots`,
`.comparison-slot`, `.comparison-slot img`, `.comparison-slot
button`, `.shape-hint`. Reuses existing color variables. ~50 lines.

### `static/app.js` (modified)

Five new functions and two modifications.

#### New: `async fetchClassification(id, overrideCategory=null)`

Wraps `POST /api/classify/:id` (with optional `?override=...`).
Returns `{settings, candidates}` on success, throws on HTTP error.
Single source of truth for the endpoint shape.

#### New: `async openComparisonModal(id, candidates, originalCategory, originalConfidence)`

Builds the slot DOM (one slot per candidate, capped at 3), shows the
modal, then sequentially calls `renderCandidateThumbnail` for each
slot. Each slot shows a "Rendering…" placeholder until its thumbnail
returns. Stores `originalCategory` / `originalConfidence` on the
modal element for the pick handler to read.

#### New: `async renderCandidateThumbnail(slotEl, candidate)`

For one candidate:

1. Compute the strategy's bake parameters via a JS-side
   `STRATEGY_TABLE` constant (mirror of `strategy.go`'s
   `shapeStrategyTable`, kept in sync by hand — three string
   fields per row, same as `validShapeCategories` is duplicated).
2. Snapshot `currentSettings`, mutate the slice fields in place to
   the strategy's values, call
   `renderHorizontalLayerGLB(currentModel, layers, 256)`, restore
   `currentSettings`.
3. Load the returned GLB via `GLTFLoader.parse` into a small scene,
   render with a 256×256 perspective camera, `toDataURL("image/png")`,
   stuff into the slot's `<img>`.
4. Special case `hard-surface`: skip the bake (slice_axis is `n/a`),
   render the original `currentModel` directly to a 256px canvas
   with a "(parametric path — no slicing)" caption.

The mutate-and-restore around `currentSettings` is sequential (we
await each candidate before starting the next) so there is no race
between concurrent bakes touching the shared `currentSettings`
object. The hard-surface special case avoids feeding `n/a` into the
slice-axis resolver.

#### New: `async pickCandidate(id, category)`

1. Calls `fetchClassification(id, category)` (the override path).
2. On success: assigns the returned `settings` to `currentSettings`,
   calls `populateTuningUI()`, refreshes the file list, hides the
   modal.
3. On failure: keeps the modal open, shows an inline error.

The analytics event is emitted *server-side* by the override
branch — JS does not double-fire.

#### New: `closeComparisonModal()`

Hides the modal and clears its inner state. Called by the Cancel
button, the backdrop click, and `pickCandidate` on success.

#### Modification: `selectFile`

After `loadSettings(id)` resolves, before the `loadModel` call:

```js
const conf = currentSettings.shape_confidence;
if (conf > 0 && conf < 0.7) {
    // fire-and-forget; don't block the model load
    fetchClassification(id).then(({settings, candidates}) => {
        currentSettings = settings;
        populateTuningUI();
        if (candidates && candidates.length > 0) {
            openComparisonModal(id, candidates, settings.shape_category, settings.shape_confidence);
        }
    }).catch(err => console.warn('auto-reclassify failed', err));
}
```

The auto-reclassify intentionally happens *after* `currentModel` is
loaded so the modal's thumbnail render pipeline has a Three.js
model to bake. This means we hook the auto-open into the
`loadModel` completion callback, not into `selectFile`'s top-level
flow — see plan.md for the exact ordering.

#### Modification: tuning panel wiring

In `wireTuningUI` (or alongside it), add a `click` handler for
`tuneReclassifyBtn` that calls `fetchClassification(id)` and opens
the modal regardless of the returned confidence. Also update
`populateTuningUI` to refresh `tuneShapeCategoryHint` text from
`currentSettings.shape_category` + `shape_confidence`.

## Module boundaries / public interfaces

- The Python ranking lives inside `features.candidates`. This is
  the *only* schema commitment for downstream consumers. Score
  values are not specified beyond "in [0,1], higher = better".
- The HTTP shape `{settings, candidates}` is internal — no
  external client. Future ML tooling reads from the
  `classification_override` analytics event, not the HTTP response.
- The JS-side `STRATEGY_TABLE` mirrors `strategy.go` exactly. Same
  staleness risk as the existing `validShapeCategories` Python ↔ Go
  duplication. Tests on neither side cross the boundary; the
  contract is "hand-edit both when adding a category".

## Ordering of changes

1. Python: `classify_shape.py` + `classify_shape_test.py`. Verify
   `python3 scripts/classify_shape.py <asset>` produces a
   `features.candidates` array.
2. Go: `analytics.go` event registration; `analytics_test.go`.
3. Go: `handlers.go` reshape + override branch; new helper;
   `strategy_handlers_test.go` end-to-end.
4. Docs: `analytics-schema.md` new event section.
5. Frontend: `index.html` modal + Reclassify button.
6. Frontend: `style.css` modal styles.
7. Frontend: `app.js` candidate functions + selectFile hook +
   tuning panel wiring.

Each step is independently committable. The frontend is the
load-bearing payoff and lands last so the backend work is exercised
end-to-end on the manual verification step.
