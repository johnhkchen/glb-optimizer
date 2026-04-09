# T-004-04 — Research

Multi-strategy comparison UI. The classifier (T-004-02) and the
strategy router (T-004-03) are already in place; this ticket adds a
front-end loop that lets a human resolve ambiguous classifications and
captures every decision as labeled training data.

## What exists

### Classifier (T-004-02)

`scripts/classify_shape.py` reads a GLB, runs PCA + a hard-surface
overlay, and prints one JSON line:

```json
{
  "category": "round-bush",
  "confidence": 0.83,
  "is_hard_surface": false,
  "features": { "ratios": {...}, "axis_alignment": ..., "peakiness": ... }
}
```

Internally `classify()` (classify_shape.py:302) computes a *full
ranking* over the four geometric centroids (round-bush, planar,
directional, tall-narrow) and a softmax-style confidence. The ranking
is computed and immediately discarded (`_ranking` at line 334) — the
top entry is the only thing returned today.

`is_hard_surface` is an overlay flag, not a ranking entry, but
T-004-02 §"Open concerns #1" notes that the wood-bed asset measures as
`planar` even though the overlay fires — so a comparison UI that
exposes the ranking should add `hard-surface` as a candidate when the
overlay is set, regardless of distance to the planar centroid.

### Go subprocess wrapper (`classify.go`)

`ClassificationResult` has `Category`, `Confidence`, `IsHardSurface`,
and an opaque `Features map[string]interface{}`. The struct already
ignores unknown top-level fields and forwards all of `features.*`
verbatim — so anything we add inside `features` flows through Go
without code change, and the analytics event payload also receives it
because `emitClassificationEvent` (handlers.go:737) puts the whole
features map onto the wire.

### Strategy router (`strategy.go`, T-004-03)

`getStrategyForCategory(category)` returns a `ShapeStrategy` with
`slice_axis`, `slice_count`, `slice_distribution_mode`,
`instance_orientation_rule`, `default_budget_priority`. The lookup
table is the canonical source for "what does picking this category
mean". Five non-`unknown` entries: round-bush, directional,
tall-narrow, planar, hard-surface. Hard-surface uses the `n/a`
sentinel for slice fields — the bake stamper skips it (handlers.go:702
`applyShapeStrategyToSettings`).

### Settings & stamping pipeline

`applyClassificationToSettings` (handlers.go:715) is the single point
that mutates persisted settings from a classification result. It:

1. loads or defaults the asset's settings
2. overwrites `ShapeCategory` and `ShapeConfidence`
3. calls `applyShapeStrategyToSettings` which only touches
   `slice_distribution_mode`, `slice_axis`, `volumetric_layers` *when
   they still match the defaults* (so user overrides survive)
4. validates and saves atomically.

This is the same code path the override button must use — but with
`ShapeConfidence = 1.0` and the user-chosen category overriding the
classifier's pick.

### HTTP surface

- `POST /api/classify/:id` (handlers.go:820) — re-runs the classifier
  and returns the updated `AssetSettings` JSON. **Does not return the
  full classifier features or ranking today.**
- `POST /api/upload` (handlers.go:45) — auto-classifies on upload via
  `autoClassify` (handlers.go:795); upload response is the
  `FileRecord`, not the settings.
- `GET /api/settings/:id` — returns `AssetSettings`. No classifier
  metadata beyond `shape_category` + `shape_confidence`.
- `POST /api/analytics/event` — opaque envelope writer, accepts any
  registered `event_type`.

The frontend learns the asset's category and confidence only via
`/api/settings/:id` on selection. There is no current path that
exposes the classifier's per-category ranking to JS.

### Frontend bake / offscreen render

`renderHorizontalLayerGLB(model, numLayers, resolution)`
(static/app.js:1647) reads `currentSettings.slice_axis`,
`slice_distribution_mode`, `volumetric_layers`,
`dome_height_factor`, `alpha_test`, `ground_align`, plus the bake
lighting fields, and returns an exported GLB byte buffer. The function
already wraps the model in a temporary rotation Group when
`slice_axis !== 'y'` and unwinds it on the way out (T-004-03).

`renderLayerTopDown(model, resolution, floorY, ceilingY)` (line 1359)
is the lower-level helper that renders one slice canvas. It is called
N times by `renderHorizontalLayerGLB`.

`runPipelineRoundtrip` (line 1990) demonstrates the bake-→GLB→reload→
render-snapshot pattern that the comparison view needs: it bakes a
canvas, exports it as a GLB, reloads via GLTFLoader, and renders the
result to a fresh offscreen canvas. This is the existing template for
"render an asset under settings X to a small canvas".

### Frontend tuning panel

`TUNING_SPEC` (static/app.js:330) plus `populateTuningUI` /
`wireTuningUI` is the auto-instrumented control rack. The panel's
HTML lives in `static/index.html` `<div id="tuningSection">`. There is
no existing place for "Reclassify..." — the panel ends with
`tuneResetBtn`. A new `setting-row` button is the precedent.

### Frontend session / analytics

`logEvent(type, payload, assetId)` (static/app.js:288) is the
canonical fire-and-forget event sender. New event types only need:

1. one entry in `validEventTypes` (analytics.go:25)
2. one section in `docs/knowledge/analytics-schema.md`

No frontend type registry to update. `selectFile` (static/app.js:3134)
is the natural place to detect "low-confidence asset just opened" and
trigger the comparison view auto-open.

### Modal / overlay precedent

There is no existing modal dialog in `static/index.html`. The
diagnostic `Test Lighting` flow renders a report into the preview
area, not a modal. We will need to introduce a modal pattern. CSS in
`static/style.css` already defines `--panel-bg`, `--panel-border`,
`--accent` colors that a modal can reuse.

## Boundaries and constraints

- **Closed taxonomy.** `validShapeCategories` is enforced both
  Python-side and Go-side. Any candidate the user can pick must be in
  this set; the modal cannot invent new categories.
- **Settings overrides survive classification re-runs.** The
  `applyShapeStrategyToSettings` "still default" check is the
  load-bearing invariant. The override button must call into the same
  helper, not write fields directly, otherwise tests in
  `strategy_handlers_test.go` regress.
- **Analytics envelope is frozen at v1.** New event types are
  additive; new payload fields are additive. No schema bump.
- **Bake is slow (~1–3s per call).** Comparison view will run it 2–3
  times sequentially per open. The ticket explicitly accepts this.
- **`features` map is opaque.** Anything new the Python side puts
  there flows through Go to the analytics event payload without a
  Go-side code change — the ranking is the natural place to attach
  candidate scores.
- **`shape_confidence == 1.0` is the human-confirmed sentinel.** Set
  by the override path, used by `selectFile` to skip auto-opening the
  modal on subsequent selections.
- **No JS test harness.** Same constraint as T-004-03; all UI logic
  is verified manually.
- **No third-party JS deps.** The project already pulls Three.js +
  GLTFExporter/Loader from unpkg; nothing new should be added.

## Open questions to resolve in Design

1. Where do candidate categories come from? Three options:
   (a) Python emits a ranking inside `features`,
   (b) Frontend hard-codes 3 candidates,
   (c) New Go endpoint that returns the ranking directly.
2. What does the `/api/classify/:id` response look like after this
   ticket? Today it returns just `AssetSettings`.
3. Auto-open trigger: only on explicit Reclassify, or also on
   `selectFile` when `shape_confidence < 0.7`?
4. Thumbnail rendering: full bake roundtrip per candidate, or a
   cheaper "axis-overlay" render? Cost vs. fidelity.
