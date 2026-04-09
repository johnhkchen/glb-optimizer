# Structure — T-009-03

## File-by-file change list

### `settings.go` — modified

- Add three fields to `AssetSettings` (after `AlphaTest`, before
  `LightingPreset`, json tags `tilted_fade_low_start`,
  `tilted_fade_low_end`, `tilted_fade_high_start`, all `,omitempty`).
- Add three lines to `DefaultSettings()` returning 0.30 / 0.55 / 0.75.
- Add three `checkRange("tilted_fade_*", ..., 0, 1)` calls in
  `Validate()` after the existing `alpha_test` check.

### `settings_test.go` — modified

- Extend `TestDefaultSettings_NewFields` (or add a sibling
  `TestDefaultSettings_TiltedFadeFields`) asserting the three defaults.
- Extend `TestValidate_RejectsOutOfRange` table with three rows
  (`s.TiltedFadeLowStart = -0.1`, etc.).
- The existing `TestLoadSettings_MigratesOldFile` legacy JSON does
  not include the new keys; with `omitempty` and zero-tolerant
  validation, that test continues to pass unmodified — confirm by
  re-reading after edit.

### `static/index.html` — modified

Three new `<div class="setting-row">` blocks inside the tuning panel,
inserted after the `tuneAlphaTest` row. Pattern matches the existing
slider rows exactly:

```html
<div class="setting-row" data-help-id="tuneTiltedFadeLowStart">
    <label>Tilted fade-in start
        <span class="range-value" id="tuneTiltedFadeLowStartValue">0.30</span>
    </label>
    <input type="range" id="tuneTiltedFadeLowStart" min="0" max="1" step="0.01" value="0.30">
</div>
```

Three rows, ids `tuneTiltedFadeLowStart` / `tuneTiltedFadeLowEnd` /
`tuneTiltedFadeHighStart`.

### `static/help_text.js` — modified

Three one-line entries explaining the bands in the same plain-English
voice as the existing tooltips (e.g. "Camera-tilt where the tilted
impostor begins to fade in over the horizontal billboard").

### `static/app.js` — modified (the bulk of the change)

Edit points, in source order:

1. **`makeDefaults()` (line ~142)** — add three keys
   `tilted_fade_low_start: 0.30`, `tilted_fade_low_end: 0.55`,
   `tilted_fade_high_start: 0.75`.

2. **Settings load normalization** — wherever `currentSettings` is
   pulled from `/api/settings` (line ~113), normalize zeros after the
   fetch:
   ```js
   if (!currentSettings.tilted_fade_low_start)  currentSettings.tilted_fade_low_start  = 0.30;
   if (!currentSettings.tilted_fade_low_end)    currentSettings.tilted_fade_low_end    = 0.55;
   if (!currentSettings.tilted_fade_high_start) currentSettings.tilted_fade_high_start = 0.75;
   ```
   Mirrors the existing pattern for `scene_template_id` / `scene_instance_count`
   (whichever local helper handles those, or do it inline at the
   fetch site if no helper exists).

3. **`TUNING_SPEC` (line ~687)** — three new entries with
   `parse: parseFloat` and `fmt: v => v.toFixed(2)`. This wires
   populate, persist, dirty-dot, and `setting_changed` analytics
   automatically.

4. **`generateProductionAsset` (line ~2389)** — between the
   billboard-upload and volumetric-upload blocks, insert a tilted
   bake block:
   ```js
   const tiltedGlb = await renderTiltedBillboardGLB(
       currentModel,
       TILTED_BILLBOARD_ANGLES,
       TILTED_BILLBOARD_ELEVATION_RAD,
       TILTED_BILLBOARD_RESOLUTION,
   );
   await fetch(`/api/upload-billboard-tilted/${id}`, {
       method: 'POST',
       headers: { 'Content-Type': 'application/octet-stream' },
       body: tiltedGlb,
   });
   store_update(id, f => f.has_billboard_tilted = true);
   ```
   The single `regenerate` event with `trigger: 'production'` already
   fires from the existing finally block — no analytics changes here.
   The tilted bake does NOT emit its own `billboard_tilted` event in
   this code path; the production trigger covers it.

5. **`prepareForScene` stage 4 (line ~2540)** — wrap
   `generateProductionAsset(id)` so the row's running label updates
   between sub-bakes. Cleanest approach: don't try to instrument from
   inside the orchestrator; instead, hoist `generateProductionAsset`
   into a small inline implementation in the orchestrator that calls
   `markPrepareStage('production', 'running', 'horizontal bake…')` /
   `'tilted bake…'` / `'volumetric bake…'` between awaits.
   To avoid duplicating bake code, refactor `generateProductionAsset`
   to accept an optional progress callback:
   ```js
   async function generateProductionAsset(id, onSubstage = () => {}) {
       ...
       onSubstage('horizontal');
       // billboard bake
       onSubstage('tilted');
       // tilted bake
       onSubstage('volumetric');
       // volumetric bake
       ...
   }
   ```
   `prepareForScene` passes a callback that calls `markPrepareStage`.
   Direct callers (`generateProductionBtn` click) pass nothing.
   Failure check at the end now requires all three flags.

6. **`runProductionStressTest` (line ~4231)** — extend to three
   parallel loads and three instance creations:
   ```js
   if (!file || !file.has_billboard || !file.has_billboard_tilted || !file.has_volumetric) return;
   const [bbGltf, tiltedGltf, volGltf] = await Promise.all([
       loadAsync(`...?version=billboard...`),
       loadAsync(`...?version=billboard-tilted...`),
       loadAsync(`...?version=volumetric...`),
   ]);
   applyEnvironmentToModel(...) ×3;
   stressInstances.push(...createBillboardInstances(bbGltf.scene, positions));
   stressInstances.push(...createTiltedBillboardInstances(tiltedGltf.scene, positions));
   stressInstances.push(...createVolumetricInstances(volGltf.scene, positions, true));
   productionHybridFade = true;
   ```

7. **`updateBillboardVisibility` / `updateVolumetricVisibility`
   (lines 4024 / 3984)** — leave intact for standalone preview modes.

8. **New `updateHybridVisibility()`** — placed adjacent to the legacy
   functions. Computes the four opacities per the design.md formulas
   and applies them to `billboardInstances`, `billboardTopInstances`,
   `tiltedBillboardInstances`, `volumetricInstances`.

9. **New module-level `let productionHybridFade = false;`** — beside
   `volumetricHybridFade`. Reset to false in `clearStressInstances`.

10. **`animate()` (line ~3340)** — replace the current
    `updateBillboardVisibility` / `updateTiltedBillboardFacing` /
    `updateVolumetricVisibility` block with a dispatch:
    ```js
    if (stressActive) {
        if (productionHybridFade) {
            updateBillboardFacing();
            updateTiltedBillboardFacing();
            updateHybridVisibility();
        } else {
            if (billboardInstances.length > 0 || billboardTopInstances.length > 0) {
                updateBillboardFacing();
                updateBillboardVisibility();
            }
            if (tiltedBillboardInstances.length > 0) {
                updateTiltedBillboardFacing();
            }
            if (volumetricInstances.length > 0 && volumetricHybridFade) {
                updateVolumetricVisibility();
            }
        }
    }
    ```

### Files NOT modified

- `handlers.go`, `main.go`, `models.go` — backend already complete.
- `analytics.go`, `docs/knowledge/analytics-schema.md` — no new event
  types; `setting_changed` rows for the three sliders are auto-instrumented.
- `static/style.css` — slider rows use existing `.setting-row`
  styling.

## Ordering of changes

1. Backend: `settings.go` field + default + validate + test → run
   `go test ./...`.
2. JS data plumbing: `makeDefaults`, settings load normalize,
   `TUNING_SPEC`, HTML rows, help text → `node --check static/app.js`.
3. JS visibility math: `updateHybridVisibility`, module flag, animate
   dispatch → `node --check`.
4. JS bake bundle: refactor `generateProductionAsset` for substage
   callback, update `prepareForScene` stage-4 wrapper, extend
   `runProductionStressTest` for three layers → `node --check`.
5. Re-run `go test ./...` and `go build ./...`.
6. Manual verification per ticket (rose stress test).

## Public interface deltas

- `AssetSettings` JSON gains three optional float fields. On-disk
  back-compat preserved by `omitempty` + JS load-time normalization.
- `generateProductionAsset` gains an optional second parameter
  `onSubstage` (callback). Existing direct callers continue to work.
- `window.generateTiltedBillboard` (devtools) is unchanged.
- No new HTTP routes, no new analytics event names.
