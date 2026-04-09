# Progress — T-009-03

All five implementation steps completed in one continuous Implement
phase. Step 6 is manual verification on the rose, deferred to the
ticket owner.

## Completed steps

- [x] **Step 1 — backend settings fields.**
  `settings.go`: added `TiltedFadeLowStart` / `TiltedFadeLowEnd` /
  `TiltedFadeHighStart` to `AssetSettings` (omitempty), default 0.30
  / 0.55 / 0.75, three `checkRange(..., 0, 1)` calls in `Validate()`.
  `settings_test.go`: added `TestDefaultSettings_TiltedFadeFields`
  and three table rows in `TestValidate_RejectsOutOfRange`.
  `go test ./...` green.

- [x] **Step 2 — JS settings data plumbing.**
  `static/app.js`: three keys in `makeDefaults()`,
  `normalizeTiltedFadeFields(currentSettings)` called from
  `loadSettings`, three `TUNING_SPEC` entries (auto-instrumented for
  `setting_changed`).
  `static/index.html`: three `<div class="setting-row">` blocks
  inserted after `tuneAlphaTest`.
  `static/help_text.js`: three tooltip strings.
  `node --check static/app.js` green.

- [x] **Step 3 — unified visibility function.**
  `static/app.js`: declared `let productionHybridFade = false;` next
  to `volumetricHybridFade`, reset in `clearStressInstances`.
  Added `updateHybridVisibility()` (and a small `applyOpacityToMeshes`
  helper used only by it) implementing the four-opacity smoothstep
  math from design.md. `animate()` now dispatches: when
  `productionHybridFade` is set, the unified pass runs; otherwise
  the legacy 2-state functions still drive standalone preview modes.

- [x] **Step 4 — generateProductionAsset triple bake.**
  Refactored signature to `generateProductionAsset(id, onSubstage = () => {})`.
  Calls `onSubstage('horizontal' | 'tilted' | 'volumetric')` between
  the three uploads. New tilted bake block reuses
  `renderTiltedBillboardGLB` + `/api/upload-billboard-tilted/:id` +
  `store_update(id, f => f.has_billboard_tilted = true)`.
  Existing direct callers (the toolbar `generateProductionBtn` click
  handler) still pass nothing — the default no-op callback keeps
  them working.
  Single `regenerate` event with `trigger: 'production'` continues
  to fire from the existing `finally` block (no new analytics event
  introduced; the production trigger covers the bundled tilted
  bake).

- [x] **Step 5 — prepareForScene + runProductionStressTest wiring.**
  `prepareForScene` stage 4 now passes a callback that updates the
  running label via `markPrepareStage('production', 'running', \`${substage} bake…\`)`.
  Stage 4 success check expanded to require all three flags
  (`has_billboard && has_billboard_tilted && has_volumetric`).
  `runProductionStressTest` gates on all three flags, fetches three
  GLBs in parallel via `Promise.all`, instantiates all three layers,
  then sets `productionHybridFade = true`.

- [ ] **Step 6 — manual verification on the rose.** Deferred to
  ticket owner. Procedure documented in `plan.md`.

## Deviations from the plan

None substantive.

A small helper `applyOpacityToMeshes(arr, opacity)` was extracted
inside `updateHybridVisibility` since the same four-line opacity-write
loop runs four times (once per layer). It is local to the hybrid path
and not called elsewhere — no broader refactor.

## Build / test status

- `go build ./...` clean.
- `go test ./...` green (cached after settings step 1, no new test
  failures introduced by JS-only steps).
- `node --check static/app.js` clean after every JS-touching step.

## Not committed

Per ticket owner workflow this Implement pass produced a coherent
working tree but did not split commits. Reviewer can collapse or
split as desired against the step boundaries above.
