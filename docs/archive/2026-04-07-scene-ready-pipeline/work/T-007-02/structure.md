# Structure — T-007-02

## File-level changes

### Modified

#### `static/presets/lighting.js`

- Per-preset, hand-tune `preview_config` where the live preview's
  five-light topology blows out vs the bake's four-light. The
  `makePreset` helper currently clones `bake_config` into
  `preview_config`; we extend it to accept an optional explicit
  `preview_config` override and merge into the clone.
- Default preset: `preview_config` ≡ `bake_config` (already a no-op).
- `overcast`: attenuate `ambient` and `hemisphere_intensity` ~0.65 in
  `preview_config` to compensate for the extra directional lights.
- `golden-hour`: attenuate `key_intensity` ~0.85 (the live key sits
  off-axis so it pools light differently than the bake's top-down).
- `dusk`: bump `key_intensity` from 0.40 → 0.55 in `preview_config`
  so the perspective camera has enough fill to see the model.
- `indoor` / `midday-sun`: equal to `bake_config`.
- All color fields stay identical between bake_config and
  preview_config — the user must recognise the same preset in both
  surfaces.

#### `static/app.js`

New module-level state:
- `let bakeStale = false;` — set true when the user changes preset
  after the last regenerate.

New helpers:
- `resolvePresetColors(cfg)` — pure function. Given a
  `bake_config`/`preview_config`-shaped object, returns
  `{bright, mid, dark, key, fill}` of `{r,g,b}` 0..1. Refactor:
  `getActiveBakePalette` becomes a thin wrapper that selects
  `bake_config` and calls `resolvePresetColors`. (Avoids three
  copies of the tuple-to-object dance.)
- `getActivePreviewPalette()` — same priority as
  `getActiveBakePalette` but reads `preview_config`. Returns the
  same `{bright, mid, dark, key, fill}` shape.
- `applyPresetToLiveScene()` — traverses `scene` and mutates light
  colors/intensities in place using `getActivePreviewPalette()` and
  the corresponding intensity fields from the active preset's
  `preview_config`. Mapping per design.md D3:
  - `AmbientLight`     → color = bright, intensity = preview.ambient
  - `HemisphereLight`  → color = bright, groundColor = dark,
                          intensity = preview.hemisphere_intensity
  - `DirectionalLight` at +x (≈5,10,7)  → color = key,
                                          intensity = preview.key_intensity
  - `DirectionalLight` at -x (≈-5,5,-5) → color = bright,
                                          intensity = preview.key_intensity * 0.55
  - `DirectionalLight` at -y (≈0,-3,5)  → color = fill,
                                          intensity = preview.fill_intensity
  - guard: if `!scene` (init not done), no-op
  - **does NOT** touch `scene.environment` (that's owned by the
    reference / RoomEnvironment path).
- `setBakeStale(stale)` — toggles `bakeStale` flag and shows/hides
  `#bakeStaleHint` DOM element. Idempotent.

Changes to existing functions:

- `applyLightingPreset(id)` (~line 168): after the cascade and
  `populateTuningUI()`, call `applyPresetToLiveScene()` and
  `setBakeStale(true)`. Position: between `populateTuningUI()` and
  `saveSettings(...)`.

- `applyReferenceTint(palette)` (~line 2130): extend the traverse
  to also tint the three DirectionalLights. The mapping mirrors the
  preset path: dirLight (key) ← `palette.bright`, dirLight2 (rim) ←
  `palette.bright`, dirLight3 (under-fill) ← `palette.mid`. Keep
  the existing intensities intact (T-007-02 does not retune the
  reference path).

  Rationale: closes a pre-existing gap so the reference-image
  calibration mode looks consistent with the new preset application.

- `resetSceneLights()` (~line 2996): keep as-is for the no-asset case
  (it's still called from `applyColorCalibration` when calibration is
  torn down without a current preset). Add a guard so callers can
  decide between "default neutral" (this) and "active preset"
  (`applyPresetToLiveScene`).

- `applyColorCalibration(id)` (~line 2969): in the
  *tear-down* branch (where calibration is being removed), replace
  `resetSceneLights()` with `applyPresetToLiveScene()`. The user
  expects to see whatever preset they picked, not raw white.

- `selectFile(id)` (~line 2918): in the `loadSettings(id).then(...)`
  callback, after `populateTuningUI()`, call
  `applyPresetToLiveScene()`. Order matters — must run AFTER
  `currentSettings` is populated. Also call `setBakeStale(false)`
  there to clear any stale flag from a previous asset.

- All four regenerate code paths (`generateBillboard`,
  `generateVolumetric`, `generateVolumetricLODs`,
  `generateProductionAsset`): in the `success` branch (right before
  the `logEvent('regenerate', ...)` call), call
  `setBakeStale(false)`. Strict definition: any successful
  regenerate clears the hint.

- Init block at ~line 3227: no change; `initThreeJS()` already runs
  before any `selectFile` so the live scene exists by the time
  `applyPresetToLiveScene()` is invoked from settings load.

#### `static/index.html`

- Add a small `<span id="bakeStaleHint" class="bake-stale-hint"
  style="display:none">Bake out of date — regenerate to apply
  preset</span>` inside the `.toolbar-actions` div in the preview
  toolbar (around line 60), positioned between
  `generateProductionBtn` and `uploadReferenceBtn`. The element
  starts hidden; `setBakeStale(true)` toggles `display: ''`.

- No new `<style>` block; piggyback on existing toolbar typography
  via the `bake-stale-hint` class. We add 4 lines of CSS inline (or
  to the existing `<style>` block) for the warning color/padding —
  see the implement step.

### Created

None. All wiring goes into existing files.

### Deleted

None.

## Public surface

No HTTP, JSON, or Go API changes.

ES module exports from `static/presets/lighting.js` are unchanged
(still `LIGHTING_PRESETS`, `getLightingPreset`, `listLightingPresets`,
`PRESET_FIELD_MAP`). Only the per-preset `preview_config` *values*
change.

## Internal organization

The new helpers cluster near the existing bake-light code:

```
static/app.js
  ...
  applyLightingPreset()           # existing — append calls
  ...
  resolvePresetColors()           # NEW — pure
  getActiveBakePalette()          # existing — refactor body
  getActivePreviewPalette()       # NEW
  applyPresetToLiveScene()        # NEW
  setBakeStale()                  # NEW
  setupBakeLights()               # existing
  ...
```

Order of definitions inside `applyLightingPreset` (post-change):

```js
function applyLightingPreset(id) {
    const preset = getLightingPreset(id) || getLightingPreset('default');
    if (!preset || !currentSettings) return;
    // ... existing cascade ...
    populateTuningUI();
    applyPresetToLiveScene();      // NEW
    setBakeStale(true);             // NEW
    if (selectedFileId) saveSettings(selectedFileId);
    logEvent('preset_applied', { ... }, selectedFileId);
}
```

## Ordering of changes (commit boundaries)

Step ordering is captured in plan.md but the structural intent is:

1. Refactor `getActiveBakePalette` to use `resolvePresetColors`.
   (Pure refactor; tests still pass.)
2. Add `getActivePreviewPalette`, `applyPresetToLiveScene`,
   `setBakeStale`. (Dead code; loadable but no behavior change.)
3. Hand-tune `preview_config` per preset in `lighting.js`.
4. Add `#bakeStaleHint` DOM element + minimal CSS.
5. Wire `applyPresetToLiveScene` and `setBakeStale` into
   `applyLightingPreset`, `selectFile`, the four regenerate paths,
   and `applyColorCalibration`.
6. Extend `applyReferenceTint` to tint the three DirectionalLights.

Each step is independently revertable.

## Test boundaries

- Go tests are unchanged — no Go code is touched.
- No JS test harness; the existing dev-time assertion in
  `lighting.js` is enough for the registry. Manual verification per
  AC: load rose, switch presets, watch live preview update,
  regenerate production asset, confirm bake matches.
