# Structure — T-002-03: tuning-panel-ui-skeleton

## Files Touched

| File                     | Action  | Approx. Lines |
|--------------------------|---------|---------------|
| `static/index.html`      | MODIFY  | +60           |
| `static/app.js`          | MODIFY  | +95 / -1      |
| `static/style.css`       | MODIFY  | +14           |

No new files. No deletions. No backend changes. No test files (the
project has no JS test infra; see T-002-02 review §coverage gaps).

## `static/index.html` — additions

### Where
Insert one new `<div class="settings-section">` immediately after the
existing "Output" section (currently ends at the closing `</div>` on
line ~217), still inside `.panel-right`.

### What
A new section block:

```
<div class="settings-section" id="tuningSection">
    <h3>Tuning <span class="dirty-dot" id="tuningDirtyDot"></span></h3>

    <div class="setting-row">
        <label>Volumetric layers
            <span class="range-value" id="tuneVolumetricLayersValue">4</span>
        </label>
        <input type="range" id="tuneVolumetricLayers" min="1" max="12" step="1" value="4">
    </div>

    <div class="setting-row">
        <label>Volumetric resolution</label>
        <select id="tuneVolumetricResolution">
            <option value="256">256</option>
            <option value="512" selected>512</option>
            <option value="1024">1024</option>
        </select>
    </div>

    <div class="setting-row">
        <label>Dome height factor
            <span class="range-value" id="tuneDomeHeightFactorValue">0.50</span>
        </label>
        <input type="range" id="tuneDomeHeightFactor" min="0" max="1" step="0.01" value="0.5">
    </div>

    <div class="setting-row">
        <label>Bake exposure
            <span class="range-value" id="tuneBakeExposureValue">1.00</span>
        </label>
        <input type="range" id="tuneBakeExposure" min="0.5" max="2.5" step="0.01" value="1.0">
    </div>

    <div class="setting-row">
        <label>Ambient intensity
            <span class="range-value" id="tuneAmbientIntensityValue">0.50</span>
        </label>
        <input type="range" id="tuneAmbientIntensity" min="0" max="2" step="0.01" value="0.5">
    </div>

    <div class="setting-row">
        <label>Hemisphere intensity
            <span class="range-value" id="tuneHemisphereIntensityValue">1.00</span>
        </label>
        <input type="range" id="tuneHemisphereIntensity" min="0" max="2" step="0.01" value="1.0">
    </div>

    <div class="setting-row">
        <label>Key light intensity
            <span class="range-value" id="tuneKeyLightIntensityValue">1.40</span>
        </label>
        <input type="range" id="tuneKeyLightIntensity" min="0" max="3" step="0.01" value="1.4">
    </div>

    <div class="setting-row">
        <label>Bottom fill intensity
            <span class="range-value" id="tuneBottomFillIntensityValue">0.40</span>
        </label>
        <input type="range" id="tuneBottomFillIntensity" min="0" max="1.5" step="0.01" value="0.4">
    </div>

    <div class="setting-row">
        <label>Env map intensity
            <span class="range-value" id="tuneEnvMapIntensityValue">1.20</span>
        </label>
        <input type="range" id="tuneEnvMapIntensity" min="0" max="3" step="0.01" value="1.2">
    </div>

    <div class="setting-row">
        <label>Alpha test
            <span class="range-value" id="tuneAlphaTestValue">0.10</span>
        </label>
        <input type="range" id="tuneAlphaTest" min="0" max="0.5" step="0.005" value="0.1">
    </div>

    <div class="setting-row">
        <label>Lighting preset</label>
        <select id="tuneLightingPreset">
            <option value="default" selected>default</option>
        </select>
    </div>

    <div class="setting-row">
        <button class="preset-btn" id="tuneResetBtn">Reset to defaults</button>
    </div>
</div>
```

Initial values are the schema defaults, hand-mirrored. After a file
selection, `populateTuningUI()` overwrites them with `currentSettings`.

## `static/app.js` — modifications

### Refactor 1 (mechanical) — extract `makeDefaults()`
Pull the literal object out of `applyDefaults()` into a pure helper:

```js
function makeDefaults() {
    return {
        schema_version: 1,
        volumetric_layers: 4,
        volumetric_resolution: 512,
        dome_height_factor: 0.5,
        bake_exposure: 1.0,
        ambient_intensity: 0.5,
        hemisphere_intensity: 1.0,
        key_light_intensity: 1.4,
        bottom_fill_intensity: 0.4,
        env_map_intensity: 1.2,
        alpha_test: 0.10,
        lighting_preset: 'default',
    };
}

function applyDefaults() {
    currentSettings = makeDefaults();
    return currentSettings;
}
```

`updateTuningDirty()` calls `makeDefaults()` directly, avoiding the
mutate-and-restore dance from the design draft.

### Refactor 2 (one constant) — debounce 500 → 300
`saveSettings`'s `setTimeout` interval changes from `500` to `300` to
match the ticket AC. No other change.

### Addition — Tuning UI block
Insert after `applyDefaults()` (~line 128), before `getSettings()`:

- `TUNING_SPEC` constant array (eleven entries).
- `populateTuningUI()` — sets each control's `value` and adjacent
  `*Value` text from `currentSettings`. Calls `updateTuningDirty()`.
- `wireTuningUI()` — attaches `input` listeners and the reset button
  handler.
- `updateTuningDirty()` — compares `currentSettings` to `makeDefaults()`
  field by field, toggles `.dirty` on `#tuningDirtyDot`.

### Hook — `selectFile` populate
At `app.js:2152` (after `await loadSettings(id);`), append a single
line:

```js
populateTuningUI();
```

### Hook — module init
Near the end of the file, after the existing event-listener block,
add:

```js
applyDefaults();   // initial in-memory state so dirty dot reads correct
wireTuningUI();
populateTuningUI();
```

`applyDefaults()` here is safe — it only sets the JS variable; it
makes no HTTP calls and writes nothing to disk. As soon as the user
selects a file, `loadSettings` overwrites it.

## `static/style.css` — additions

Append after the existing `.tooltip` rule (~line 644):

```css
.dirty-dot {
    display: inline-block;
    width: 6px;
    height: 6px;
    border-radius: 50%;
    background: transparent;
    margin-left: 6px;
    vertical-align: middle;
    transition: background 0.15s;
}
.dirty-dot.dirty {
    background: var(--accent);
}
```

## Public Interfaces

The module-level identifiers added to `app.js`:

| Identifier            | Kind     | Purpose                              |
|-----------------------|----------|--------------------------------------|
| `makeDefaults`        | function | Pure factory used by `applyDefaults` and the dirty compare |
| `TUNING_SPEC`         | const    | Field/element/format mapping table   |
| `populateTuningUI`    | function | DOM ← state                          |
| `wireTuningUI`        | function | Attaches listeners (called once)     |
| `updateTuningDirty`   | function | State → dirty dot                    |

None are exported (the module has no `export` statements). All are
top-level for ergonomics, matching the existing `app.js` style.

## Ordering / Dependencies

1. CSS first — additive, can't break anything.
2. HTML next — adds dormant DOM. Without the JS, controls do nothing
   but the page still loads.
3. JS last — wires the dormant DOM.

Implementation will commit each step atomically.

## Things This Does NOT Change

- `getSettings()` (the gltfpack settings serializer) — untouched.
- Any bake function — they already consume `currentSettings`.
- `selectFile`'s structure — only one new line is appended.
- `loadSettings` / `saveSettings` semantics — only the debounce
  numeric literal moves.
- Backend handlers, store, persistence layer.
- Any file outside `static/`.
