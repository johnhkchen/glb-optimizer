# T-004-03 — Research

Map of the codebase as it relates to wiring a shape-strategy router on top
of the T-004-02 classifier.

## Existing classification surface (T-004-02)

- `classify.go` — `RunClassifier(glbPath)` shells out to
  `scripts/classify_shape.py` and returns a `*ClassificationResult`
  `{Category, Confidence, IsHardSurface, Features}`. The Go layer
  enforces the closed enum via `validShapeCategories` in `settings.go`.
- `settings.go` —
  - `validShapeCategories`: `round-bush`, `directional`, `tall-narrow`,
    `planar`, `hard-surface`, `unknown`. The router must treat `unknown`
    as "use defaults".
  - `AssetSettings.ShapeCategory` / `ShapeConfidence` are persisted
    per-asset; `LoadSettings` forward-compats older docs by promoting
    an absent value to `"unknown"`.
- `handlers.go`:
  - `applyClassificationToSettings(id, settingsDir, result)` (line 686)
    is the single mutation point that loads, sets the two shape fields,
    validates, and writes. **This is the natural seam to also apply
    strategy defaults to the persisted settings.**
  - `emitClassificationEvent(...)` (line 707) is the canonical pattern
    for best-effort analytics emission: lookup-or-start session, build
    an `Event{}`, append, swallow errors to stderr.
  - `autoClassify(...)` (line 735) is the upload-time hook;
    `handleClassify(...)` (line 759) is the explicit
    `POST /api/classify/:id` endpoint. Both call
    `applyClassificationToSettings` and `emitClassificationEvent` —
    so a single change at those two seams gets us the strategy plumb
    on both code paths for free.

## Existing bake / slicing surface (S-005)

The "bake function" referenced in the ticket lives in
`static/app.js`, not in Go. Slicing happens browser-side via
three.js:

- `renderHorizontalLayerGLB(model, numLayers, resolution)` (line 1595)
  is the volumetric bake. It already dispatches on
  `currentSettings.slice_distribution_mode` (line 1605):
  `equal-height`, `visual-density`, `vertex-quantile` (legacy default).
- `pickAdaptiveLayerCount(model, baseLayers)` (line 1586) bumps slice
  count by aspect ratio. Independent of category.
- The bake **always slices along Y today.** `renderLayerTopDown` and
  the boundary helpers all assume the vertical axis. To honor the
  router's `slice_axis` for `directional` / `planar`, we need either
  (a) generalised slicing helpers or (b) a pre-slice rotation that
  brings the chosen axis to Y, slice as before, then apply the inverse
  rotation to the export scene root.

The bake reads `currentSettings` directly. Frontend defaults live in
`makeDefaults()` (`static/app.js:120`). The tuning UI is wired through
`TUNING_SPEC` (`static/app.js:329`), where `slice_distribution_mode`
already has an entry. New strategy-derived fields would need entries
here only if we expose them as user-tunable.

## Existing "Strategy" type (collision warning)

`scene.go` already defines a `Strategy` type (line 11) used by
`SelectStrategy(assetType, sceneRole)` for the
parametric/gltfpack/volumetric distillation router. This is a
**different abstraction** (output-pipeline routing) from what T-004-03
needs (slicing/orientation parameters per shape category). To avoid
confusion the new router should pick a different type name —
`ShapeStrategy` is the obvious choice.

## Analytics surface

- `analytics.go` enumerates `validEventTypes` (line 25). Adding
  `"strategy_selected"` is a one-line additive change; no schema
  bump per the analytics-schema.md migration policy.
- `analytics-schema.md` documents each event type's payload. T-004-03
  must add a new section there.
- The session lookup pattern (`LookupOrStartSession`) is already in
  use by `emitClassificationEvent`. The router emit can mirror it
  one-for-one.

## Settings file format and on-disk concerns

- `settings.go:21` — declaration order of `AssetSettings` fields **is
  also the on-disk JSON order**. Any new fields should be appended at
  the end so existing files don't reorder mid-line on write.
- `Validate()` is the choke point for new enums. Adding a new field
  means: a new enum map, a new `Validate()` clause, a new
  `SettingsDifferFromDefaults()` clause, and a `LoadSettings`
  forward-compat normalization branch.
- The router's strategy contains five fields per the ticket:
  `slice_axis`, `slice_count`, `slice_distribution_mode`,
  `instance_orientation_rule`, `default_budget_priority`. Two of
  these (`slice_count` → `volumetric_layers`,
  `slice_distribution_mode`) already exist in `AssetSettings`. The
  other three are net-new — but only `slice_axis` is consumed in this
  ticket's manual verification ("confirm the slice axis is
  perpendicular to the long horizontal axis"). The remaining two
  belong to S-006 / T-004-04 and have no consumer yet.

## What this ticket must touch

Necessary, based on the acceptance criteria:

1. A Go-side router function `getStrategyForCategory(category)` →
   `ShapeStrategy`. Lookup table over the closed enum.
2. Hook into `applyClassificationToSettings` so the strategy's
   "default" fields (`slice_count`, `slice_distribution_mode`,
   `slice_axis`) are stamped onto the settings when they have not
   yet been customized away from defaults. User overrides survive.
3. New analytics event `strategy_selected`, emitted alongside
   `classification` from both `autoClassify` and `handleClassify`.
4. New persisted field `slice_axis` in `AssetSettings` so the bake
   can read it. Consumed in `static/app.js`'s
   `renderHorizontalLayerGLB` via a pre-slice axis-rotation
   approach.
5. Documentation: `settings-schema.md`, `analytics-schema.md`.
6. Tests: Go unit tests for the router; settings round-trip;
   analytics validate; an end-to-end Go test that runs
   `applyClassificationToSettings` against a directional fixture and
   asserts the resulting `slice_axis`.

## Out of scope (per ticket)

- Multi-strategy comparison UI (T-004-04).
- Trellis instance orientation in scene preview (S-006). The router
  will *return* `instance_orientation_rule` and
  `default_budget_priority` so the lookup table is the canonical
  source — but no consumer is wired in this ticket and they are not
  persisted to `AssetSettings` yet.
- Adding new shape categories.

## Constraints / assumptions

- Single-user local tool: no concurrent classification, no migration
  burden across versions, no network round-trips.
- Strategy router is intentionally a lookup table — the ticket's
  "First-Pass Scope" rejects per-category tuning beyond the table.
- The classifier may return `unknown`; the router must accept it
  without crashing and return a benign default strategy (current
  bake-as-is behavior).
- Frontend bake is browser JS; any new strategy field consumed by
  the bake must travel via `AssetSettings` JSON, not via a separate
  API call. Settings already round-trip the new field on
  save/load — the router populates the field during classification.
