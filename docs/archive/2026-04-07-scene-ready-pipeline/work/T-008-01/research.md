# T-008-01 Research

## Goal recap

Replace the cluster of technique buttons (`LODs gltfpack`, `LODs Blender`,
`Billboard`, `Volumetric`, `Vol LODs`, `Production Asset`, `Test Lighting`,
`Reference Image`, plus the left-panel `Process All`) with a single
goal-oriented primary action `Prepare for scene` that runs the full pipeline
end-to-end against the currently selected asset, shows progress, surfaces
errors clearly, and emits a `prepare_for_scene` analytics event. Existing
buttons stay wired in this ticket — they get hidden behind the Advanced
disclosure in T-008-02.

## Where the relevant pieces live

### HTML structure (`static/index.html`)

- Left panel `panel-actions` (line 26) holds `#processAllBtn` and
  `#downloadAllBtn`. This is the existing "primary" surface in the left
  panel — Prepare-for-scene logically belongs here too, but is per-asset
  rather than per-batch.
- Center-panel toolbar `#previewToolbar` (line 34) hosts the per-asset
  technique buttons inside `.toolbar-actions` (lines 53–64): `generateLodsBtn`,
  `generateBlenderLodsBtn`, `generateBillboardBtn`, `generateVolumetricBtn`,
  `generateVolumetricLodsBtn`, `generateProductionBtn`, `uploadReferenceBtn`,
  `testLightingBtn`. The toolbar is `display:none` until a file is selected.
- Stress controls (`#stressBtn` "Run scene", template select, count input,
  ground/LOD checkboxes) live in `.stress-controls` (lines 65–76). These are
  the "view in scene" plumbing — there is no standalone API call, the button
  just calls `runStressTest()` against the currently loaded `currentModel`.

### Frontend pipeline functions (`static/app.js`)

The orchestrator needs to call these in order. All are async and already
exist; some hit Go endpoints, some are pure client-side bake passes.

| Stage              | Function (in app.js)               | Mechanism                                         |
|--------------------|------------------------------------|---------------------------------------------------|
| gltfpack cleanup   | `processFile(id)` (1176)           | `POST /api/process/:id` with `getSettings()`      |
| Classify shape     | `fetchClassification(id)` (417)    | `POST /api/classify/:id` (re-runs classifier)     |
| LOD chain          | `generateLODs(id)` (1239)          | `POST /api/generate-lods/:id` with settings       |
| Production asset   | `generateProductionAsset(id)` (2197) | Renders billboard + volumetric in-browser, posts each blob to `/api/upload-billboard/:id` and `/api/upload-volumetric/:id` |
| Scene preview thumb | (none)                            | Optional per AC; deferred — runStressTest path renders live, no thumbnail capture exists |

`generateProductionAsset` already encapsulates "billboard + volumetric for
the current shape" — it reads `currentSettings.volumetric_layers` /
`volumetric_resolution` which are stamped by the strategy router after
classification. Hard-surface (`volumetric_layers: 0`) flows through the
same call without special-casing in this ticket.

### "View in scene" plumbing

- `sceneTemplateSelect` is the toolbar select; the active template id lives
  in `currentSettings.scene_template_id` and is mirrored via
  `getActiveSceneTemplate()` / `setSceneTemplate()`.
- The "Run scene" action is `#stressBtn` (line 4221) — it reads
  `sceneInstanceCount`, `stressUseLods`, `lodQualitySlider`, then calls
  either `clearStressInstances()` or `runStressTest(count, useLods, quality)`
  (defined at app.js:3611). There is no separate "run last template"
  function — the orchestrator's "View in scene" affordance can simply
  programmatically click `#stressBtn` (or call `runStressTest` directly with
  the same inputs the button reads).

### Settings and classification state

- `currentSettings` is the per-asset `AssetSettings` blob loaded by
  `loadSettings(id)` in `selectFile`. It carries `shape_category`,
  `shape_confidence`, `volumetric_layers`, `volumetric_resolution`,
  `scene_template_id`, `scene_instance_count`, `scene_ground_plane`, etc.
- Classification has an auto-open comparison-modal hook at `selectFile`
  (3897): when `0 < shape_confidence < 0.7` it re-runs classify and pops
  the modal. The Prepare-for-scene orchestrator should NOT trigger that
  modal — it should just bake whatever the asset's current settings say
  to bake. Re-classification on every Prepare press would re-stamp settings
  and undo any tuning the user did since the previous classify; AC point 2
  says "Classify shape" but it also says "using current settings", so the
  intent is "ensure the asset has been classified at least once" rather
  than "always re-classify".
- Backstop: assets opened via `selectFile` already have `shape_confidence`
  populated by upload-time classify (see T-004-02 trace). So in practice the
  classify stage in Prepare is a no-op skip when `shape_confidence > 0`
  and a one-shot `fetchClassification` when it is zero.

### Analytics

- `logEvent(type, payload, assetId)` (328) is the single client-side
  emitter. It POSTs to `/api/analytics/event` only if there is a live
  `analyticsSessionId` (started by `selectFile` → `startAnalyticsSession`).
- Server-side `validEventTypes` (analytics.go:25) is a hard allow-list.
  **`prepare_for_scene` is not in the list**, so the event will be rejected
  with HTTP 400 unless we add it. The schema doc (analytics-schema.md)
  must also gain a `### prepare_for_scene` section per the doc's own
  versioning rules ("New event types ... require an additive change here
  and a documentation update").
- Existing `regenerate` events emit per-stage success/failure already
  (`billboard`, `volumetric`, `volumetric_lods`, `production`). The
  Prepare orchestrator doesn't need to suppress those — they remain
  useful per-stage telemetry — and adds the higher-level
  `prepare_for_scene` event on top.

### `getSettings()` vs `currentSettings`

There are two settings shapes in the app:
1. `getSettings()` (top of app.js) — returns the gltfpack-flag dict from
   the right panel (simplification ratio, compression, texture flags).
   Consumed by `processFile`, `generateLODs`.
2. `currentSettings` — the persistent `AssetSettings` per-asset blob from
   the tuning panel (volumetric layers, lighting preset, shape category,
   scene template). Consumed by `generateProductionAsset` and friends.

The orchestrator does not need to merge these — each stage already calls
the source it needs.

## Constraints and assumptions

- **Per-asset**, not batch. Prepare-for-scene operates on `selectedFileId`.
  `processAllBtn` is unrelated and remains for now (left-panel batch path).
- **Sync, foreground**. AC explicitly defers background execution. The
  orchestrator runs stages sequentially, awaiting each, and does not
  spawn workers.
- **Stop on first error**. AC says "the pipeline stops at that stage". No
  retry, no rollback, no resume.
- **`currentModel` must be loaded**. Stages that bake in-browser
  (`generateProductionAsset`) need `currentModel && threeReady`. Selecting
  a file does this asynchronously inside `selectFile`. The orchestrator
  must guard for it (or simply disable the button until `currentModel` is
  set, mirroring how `generateBillboardBtn.disabled = !file || !currentModel`
  is computed in `updatePreviewButtons` at line 4013).
- **Existing technique buttons keep working** (AC bullet 7). Hiding them
  is T-008-02; this ticket only adds the new button.
- **Manual verification gate**: AC bullet 8 ("upload a fresh rose, click
  Prepare for scene…") is a smoke test, not an automated test. There is
  no Playwright/JS test harness in this repo — the closest things are Go
  unit tests (`*_test.go`) for handlers and settings. JS code is not
  under test today.

## Out-of-scope reminders (from the ticket)

Advanced disclosure UI, label rename, inline help, retry/resume, background
execution. Don't accidentally pull any of these in. Scene preview
thumbnail capture is also explicitly "optional" in the AC and there is no
existing helper for it — leaving it out keeps scope tight.
