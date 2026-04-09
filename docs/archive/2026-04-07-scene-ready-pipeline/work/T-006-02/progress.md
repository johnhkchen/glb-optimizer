# Progress — T-006-02

## Status

All implementation steps complete. `go test ./...` and
`node --check static/app.js` clean. Manual verification gates
inherited from T-006-01 still apply (this codebase has no JS
test infrastructure) — flagged in review.md.

## Steps completed

### Step 1 — Settings schema (Go side)

- `settings.go`: added `SceneTemplateId`, `SceneInstanceCount`,
  `SceneGroundPlane` fields with comments + tags. Added
  `validSceneTemplates` map (`grid`, `hedge-row`, `mixed-bed`,
  `rock-garden`, `container`). Wired into `DefaultSettings`,
  `Validate`, `LoadSettings` (forward-compat normalization),
  and `SettingsDifferFromDefaults`.
- `settings_test.go`: added six tests:
  `TestDefaultSettings_SceneFields`,
  `TestValidate_AcceptsAllSceneTemplates`,
  `TestValidate_RejectsBadSceneTemplate`,
  `TestValidate_RejectsSceneCountOutOfRange`,
  `TestSettingsDifferFromDefaults_SceneFields`,
  `TestLoadSettings_OldDocMissingSceneFields`,
  `TestSaveLoad_RoundtripSceneFields`. All pass.

### Step 2 — Analytics allow-list

- `analytics.go`: added `"scene_template_selected": true` to
  `validEventTypes`.
- `docs/knowledge/analytics-schema.md`: added new event-type
  section above `strategy_selected`, documenting payload fields
  (`from`, `to`, `instance_count`, `ground_plane`).

### Step 3 — JS template implementations

- `static/app.js` Scene Templates section: replaced the
  `benchmark` + `debug-scatter` registry with five new templates
  (`grid`, `hedge-row`, `mixed-bed`, `rock-garden`, `container`).
  Added a tiny `_ctxSize(ctx)` helper to deduplicate the bbox
  fallback. `activeSceneTemplate` default changed from
  `'benchmark'` to `'grid'`.
- `runStressTest`'s fallback line updated from
  `SCENE_TEMPLATES.benchmark` to `SCENE_TEMPLATES.grid`.

### Step 4 — HTML + CSS scaffolding

- `static/index.html` `.stress-controls`: removed the legacy
  `stressCount` range slider + value span, replaced with
  `<select id="sceneTemplateSelect">`, `<input
  id="sceneInstanceCount" type="number">`, and `<input
  id="sceneGroundToggle" type="checkbox">`. LOD checkbox /
  quality slider untouched. Run button text changed to
  `Run scene`.
- `static/style.css`: added `.scene-select` and `.scene-count`
  rules using existing CSS variables (`--panel-bg`, `--text`,
  `--panel-border`).

### Step 5 — JS wiring (the behavioral commit)

- New module-level `let groundPlane = null;` near the other
  state vars.
- New DOM refs for the three new controls.
- `makeDefaults()`: three new keys.
- `initThreeJS()`: ground plane creation just below the
  `GridHelper` line. `MeshStandardMaterial`, brown
  (`#6b5544`), 100×100 m, hidden by default,
  `frustumCulled = false`.
- New helpers `populateScenePreviewSelect()` and
  `populateScenePreviewUI()`.
- `applyDefaults()`: now calls `populateScenePreviewUI()` so
  cold-start state hydrates the controls.
- `selectFile`'s `loadSettings(...).then` chain: added
  `populateScenePreviewUI()` next to `populateTuningUI()`.
- Init block: added `populateScenePreviewSelect()` before
  `applyDefaults()` so the `<select>` has options when the
  hydration helper sets its `.value`.
- Replaced the stress-test wiring block with picker / count /
  ground change handlers + the new stress button click handler
  that reads `sceneInstanceCount.value`.
- `clearStressInstances()`: removed the two lines that reset
  the deleted slider.

### Step 6 — Cleanup pass

- Grepped for `stressCount`, `stressValueEl`, `stressSlider`,
  `debug-scatter`, `benchmark`. Only the new comments referencing
  these legacy names remain (intentional historical context).
  Updated the only functional reference (`SCENE_TEMPLATES.benchmark`
  fallback in `runStressTest`) to `SCENE_TEMPLATES.grid`.

## Verifications run

- `go build ./...` — clean.
- `go test ./...` — clean (`ok glb-optimizer 0.7s`).
- `node --check static/app.js` — clean.
- Grep audit for legacy symbols — only comments remain.

## Deviations from plan

None of substance. The plan said to add `populateScenePreviewUI`
to `applyDefaults`; I noticed during implementation that
`applyDefaults` runs in the boot order *before*
`populateScenePreviewSelect()`, so the `<select>` would have no
options when `populateScenePreviewUI` set its `.value`. Fixed
by moving `populateScenePreviewSelect()` to run before
`applyDefaults()` in the init block. Documented in review.md.

## What was NOT verified

- **End-to-end manual verification in a live browser.** The AC
  asks for a "load rose, pick mixed-bed, see scattered varied
  bushes" check and the trellis row check. These require a
  running server + browser session, which is outside the
  automated harness. Flagged as the only outstanding
  verification gate in review.md.
