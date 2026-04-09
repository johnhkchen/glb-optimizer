# Design — T-002-03: tuning-panel-ui-skeleton

## Decision Summary

Add a single new "Tuning" `.settings-section` block at the bottom of the
right panel in `static/index.html` with eleven hand-written controls
sharing the existing CSS idiom. In `static/app.js`, add a small
"Tuning UI" module section that holds (a) a spec table mapping each
field to its DOM element, (b) `populateTuningUI()` (called after every
`loadSettings`), (c) per-control `input` listeners that mutate
`currentSettings` and call the (renamed-debounce) `saveSettings`,
(d) a "Reset to defaults" handler, and (e) a `updateTuningDirty()`
helper that toggles a single `.dirty` class on the section header.

Reset to defaults uses the existing client-side `applyDefaults()` —
**no new backend endpoint**.

Debounce window is dropped from 500 ms (T-002-02 placeholder) to 300 ms
(ticket AC) by changing one constant.

## Options Considered

### Option A — Hand-written HTML, spec-driven JS (CHOSEN)
Eleven `<div class="setting-row">` blocks in `index.html`, each with a
stable id matching its `AssetSettings` field name. JS uses a single
`TUNING_SPEC` array of `{field, type, valueFmt}` to wire up listeners
and value display in one loop.

- ✅ HTML stays grep-able and matches the existing Mesh/Texture/Output
  pattern, which is also hand-written.
- ✅ JS stays one screen of code.
- ✅ Designers can tweak labels/ranges in HTML without touching JS.
- ⚠️ Twelve ids to keep in sync with the schema. Mitigated by spec
  table: a typo on either side fails fast in the browser console.

### Option B — Fully JS-generated DOM
A single `TUNING_SPEC` array drives both DOM creation and wiring.
`index.html` gets only an empty `<div id="tuningSection">`.

- ✅ Single source of truth in JS.
- ❌ Diverges from the visual idiom of the other three sections.
- ❌ Style review now has to read JS, not HTML.
- ❌ Slightly more code overall (DOM creation boilerplate).

Rejected: violates the "match existing right-panel CSS conventions"
note in the ticket.

### Option C — Web component / template literal block
A `<template>` in HTML rendered N times with field substitution.

- ❌ No other component in this codebase uses templates. Net new
  complexity for one section.

Rejected.

### Option D — Add `/api/settings/defaults` and have Reset fetch
A new GET endpoint serves `DefaultSettings()` as JSON, and Reset does
`fetch + apply + save`.

- ✅ Removes the hand-sync risk between Go and JS defaults (one of
  T-002-02's open concerns).
- ❌ Adds backend scope to a UI ticket.
- ❌ The hand-sync risk still exists for the dirty-compare path
  unless I cache the fetched defaults too.

Rejected for this ticket. Worth filing as a follow-up in `review.md`.

### Option E — Per-control debounce
Each control has its own debounce timer.

- ❌ More state, no benefit. The user only manipulates one control at
  a time. Single timer is correct.

Rejected.

## Chosen Architecture (Option A) — Details

### HTML structure
A new section, after the existing "Output" block, before the closing
`</div>` of `.panel-right`:

```html
<div class="settings-section" id="tuningSection">
    <h3>Tuning <span class="dirty-dot" id="tuningDirtyDot"></span></h3>
    <!-- one .setting-row per AssetSettings field -->
    <div class="setting-row">
        <label>Volumetric layers
            <span class="range-value" id="tuneVolumetricLayersValue">4</span>
        </label>
        <input type="range" id="tuneVolumetricLayers" min="1" max="12" step="1" value="4">
    </div>
    ...
    <div class="setting-row">
        <button class="preset-btn" id="tuneResetBtn">Reset to defaults</button>
    </div>
</div>
```

Id naming: `tune` + PascalCase(field) for the input, plus `Value` suffix
for the live readout span. Predictable and grep-able.

### JS structure
New section in `app.js` between the existing Asset Settings block
(ending ~line 128) and `getSettings()` (~line 130):

```js
// ── Tuning UI (T-002-03) ──
const TUNING_SPEC = [
    { field: 'volumetric_layers',     id: 'tuneVolumetricLayers',     parse: parseInt,    fmt: v => v },
    { field: 'volumetric_resolution', id: 'tuneVolumetricResolution', parse: parseInt,    fmt: v => v, kind: 'select' },
    { field: 'dome_height_factor',    id: 'tuneDomeHeightFactor',     parse: parseFloat,  fmt: v => v.toFixed(2) },
    { field: 'bake_exposure',         id: 'tuneBakeExposure',         parse: parseFloat,  fmt: v => v.toFixed(2) },
    { field: 'ambient_intensity',     id: 'tuneAmbientIntensity',     parse: parseFloat,  fmt: v => v.toFixed(2) },
    { field: 'hemisphere_intensity',  id: 'tuneHemisphereIntensity',  parse: parseFloat,  fmt: v => v.toFixed(2) },
    { field: 'key_light_intensity',   id: 'tuneKeyLightIntensity',    parse: parseFloat,  fmt: v => v.toFixed(2) },
    { field: 'bottom_fill_intensity', id: 'tuneBottomFillIntensity',  parse: parseFloat,  fmt: v => v.toFixed(2) },
    { field: 'env_map_intensity',     id: 'tuneEnvMapIntensity',      parse: parseFloat,  fmt: v => v.toFixed(2) },
    { field: 'alpha_test',            id: 'tuneAlphaTest',            parse: parseFloat,  fmt: v => v.toFixed(2) },
    { field: 'lighting_preset',       id: 'tuneLightingPreset',       parse: v => v,      fmt: v => v, kind: 'select' },
];

function populateTuningUI() {
    if (!currentSettings) return;
    for (const spec of TUNING_SPEC) {
        const el = document.getElementById(spec.id);
        const valEl = document.getElementById(spec.id + 'Value');
        if (!el) continue;
        el.value = currentSettings[spec.field];
        if (valEl) valEl.textContent = spec.fmt(currentSettings[spec.field]);
    }
    updateTuningDirty();
}

function wireTuningUI() {
    for (const spec of TUNING_SPEC) {
        const el = document.getElementById(spec.id);
        if (!el) continue;
        el.addEventListener('input', () => {
            if (!currentSettings || !selectedFileId) return;
            const v = spec.parse(el.value);
            currentSettings[spec.field] = v;
            const valEl = document.getElementById(spec.id + 'Value');
            if (valEl) valEl.textContent = spec.fmt(v);
            updateTuningDirty();
            saveSettings(selectedFileId);
        });
    }
    document.getElementById('tuneResetBtn').addEventListener('click', () => {
        if (!selectedFileId) return;
        applyDefaults();
        populateTuningUI();
        // Immediate save — explicit user intent, skip debounce wait.
        saveSettings(selectedFileId);
    });
}

function updateTuningDirty() {
    const dot = document.getElementById('tuningDirtyDot');
    if (!dot || !currentSettings) return;
    const defs = (function () {
        const prev = currentSettings;
        const fresh = applyDefaults();
        currentSettings = prev; // restore — applyDefaults mutates
        return fresh;
    })();
    let dirty = false;
    for (const spec of TUNING_SPEC) {
        if (currentSettings[spec.field] !== defs[spec.field]) {
            dirty = true;
            break;
        }
    }
    dot.classList.toggle('dirty', dirty);
}
```

The `applyDefaults` round-trip in `updateTuningDirty` is mildly ugly
but cheap and correct: we briefly stash `currentSettings`, build a
fresh defaults object via the canonical helper, then restore. Avoids
duplicating the literal a third time. Alternative: extract a pure
`makeDefaults()` returning a new object. **Picked the extract.** See
Structure phase.

### Hook into `selectFile`
After the existing `await loadSettings(id);` at `app.js:2152`, append
`populateTuningUI();`. Single line.

### Init
`wireTuningUI()` runs once at module bottom, after `applyDefaults()`
provides an initial in-memory state so the dirty dot reads correctly
even before any file is selected.

### Reset = client-side
`applyDefaults()` is the canonical local mirror. Calling it +
`saveSettings(selectedFileId)` writes the canonical defaults to disk.
The user sees the same outcome they would from a backend defaults
endpoint. Documented in `research.md`'s Q1 and `review.md`'s open
concerns.

### Debounce 500 → 300
Change the magic number on `app.js:108`. No other behavior change.

### CSS additions
Two rules in `style.css`:

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

Subtle, doesn't shift layout, matches the existing `--accent` token.

## What This Design Does NOT Do
- No live re-render on slider drag.
- No tooltips or inline help.
- No named profiles, undo, or unsaved-changes warning.
- No tooling to surface validation errors from PUT (failures are still
  console-logged via the existing `saveSettings` warning path).
- No backend changes.

## Rationale Recap
The ticket explicitly says "skeleton, functional > pretty". The chosen
design adds the smallest possible UI surface that satisfies all eleven
acceptance criteria, reuses every existing CSS hook, and adds zero
backend code. The biggest judgment call is staying client-side for
"Reset to defaults"; that call is grounded in the existing
`applyDefaults()` contract from T-002-02 and is documented for the
human reviewer.
