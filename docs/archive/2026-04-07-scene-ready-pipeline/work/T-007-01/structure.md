# Structure — T-007-01: lighting-preset-schema-and-set

## File-level changes

### NEW: `static/presets/lighting.js` (~150 lines)

Plain global script. No imports, no exports — defines globals on
`window`. Loaded before `app.js`.

Public API:

```js
// Object keyed by preset id. Frozen to prevent accidental mutation.
window.LIGHTING_PRESETS = { default: {...}, 'midday-sun': {...}, ... };

// O(1) lookup; returns undefined for unknown ids. Callers MUST handle
// undefined (corrupt settings file, hand-edit, etc.) by falling back
// to LIGHTING_PRESETS.default.
window.getLightingPreset = function(id) { ... };

// List of {id, name, description} for the dropdown UI. Order is the
// declaration order of LIGHTING_PRESETS (insertion order is stable).
window.listLightingPresets = function() { ... };
```

Internal helper (not exported):

```js
// Build a preset by deep-cloning bake_config into preview_config so
// the two are independently mutable but seeded equal. T-007-02 will
// start to differ them where the live preview needs cheaper lighting.
function makePreset({id, name, description, bake_config}) { ... }
```

Module-level dev assertion:

```js
// Sanity check: the 'default' preset's mapped intensities must match
// the hardcoded defaults in app.js (makeDefaults()) so applying it is
// a no-op for unmodified assets. Logs a console.warn on drift; does
// not throw (avoids breaking the page on a typo).
(function assertDefaultMatches() { ... })();
```

The 6 preset literals live in this file in the order specified by the
ticket: `default`, `midday-sun`, `overcast`, `golden-hour`, `dusk`,
`indoor`.

### MODIFIED: `static/index.html`

Two edits:

1. Add `<script src="/static/presets/lighting.js"></script>` BEFORE
   the existing `<script src="/static/app.js"></script>` line.
2. Replace the hardcoded `<option value="default">default</option>`
   inside `#tuneLightingPreset` (line 297) with an empty `<select>` —
   `populateLightingPresetSelect()` in app.js fills it on init.

### MODIFIED: `static/app.js`

Approximate insertion points and changes:

- After `makeDefaults()` (~line 132): add `populateLightingPresetSelect()`
  helper that walks `window.listLightingPresets()` and appends `<option>`
  rows to `#tuneLightingPreset`. Called once from the existing init
  block (the one that calls `wireTuningUI()`).
- In `wireTuningUI()` at the existing `if (spec.field === 'color_calibration_mode')`
  branch (line ~342): add a sibling branch
  `if (spec.field === 'lighting_preset')` that calls
  `applyLightingPreset(v)` BEFORE the existing `saveSettings` /
  `setting_changed` line. The cascade swallows the per-field
  `setting_changed` events and emits one `preset_applied` analytics
  event instead — that requires routing the lighting_preset row
  through a slightly different path. See plan.md for the exact
  ordering.
- Add `applyLightingPreset(id)` (~30 lines): looks up preset, falls
  back to `default` on miss, computes the diff against
  `currentSettings`, mutates `currentSettings` to match the preset's
  mapped fields, calls `populateTuningUI()` to refresh all controls
  and dirty dot, calls `saveSettings(selectedFileId)`, and emits a
  single `preset_applied` event with `{from, to, changed}`.
- Add `getActiveBakePalette()` (~20 lines): returns
  `referencePalette` if truthy; else converts the active preset's
  `bake_config` colors into the `{bright, mid, dark}` shape (mapping
  bright=hemisphere_sky, mid=key_color, dark=hemisphere_ground); else
  returns the all-white fallback. Place near `setupBakeLights()` so
  the bake-only helpers stay clustered.
- Modify `setupBakeLights()` (line 920): replace the inline
  `referencePalette ? ... : 0xffffff` ternaries with calls to
  `getActiveBakePalette()`.
- Modify the inline duplicate of the same logic inside
  `renderLayerTopDown()` (lines 1181–1196): same swap.
- Modify `createBakeEnvironment()` (line 948): add an `else if` for
  the active preset case — when no `referencePalette` but the active
  preset is non-default, build the gradient from the preset's
  `env_gradient`. Same canvas/PMREM scaffolding the
  `referencePalette` branch already uses; factor the canvas-build
  step into a tiny `buildGradientEnvTexture(stops)` helper if it
  reads better.

### MODIFIED: `settings.go`

One change:

- Extend `validLightingPresets` (line 70) from `{"default": true}` to
  the full 6-id set:
  ```go
  var validLightingPresets = map[string]bool{
      "default":     true,
      "midday-sun":  true,
      "overcast":    true,
      "golden-hour": true,
      "dusk":        true,
      "indoor":      true,
  }
  ```
- Update the doc-comment to drop "S-007 will extend this".

### MODIFIED: `settings_test.go`

- Add `TestValidate_AcceptsAllPresets` that loops over the 6 ids,
  sets `s.LightingPreset = id`, and asserts `Validate() == nil`.
- The existing `TestValidate_RejectsOutOfRange` row for `"studio"`
  remains valid (still not a known preset).

### MODIFIED: `docs/knowledge/settings-schema.md`

- Update the `lighting_preset` row to enumerate the 6 valid values.
  No migration entry needed — schema_version stays at 1, and the
  existing default (`"default"`) is still valid.

## Module boundaries

- `static/presets/lighting.js` is data + lookup. No DOM, no THREE, no
  fetch. Loads cleanly in any context (testable in isolation).
- `app.js` owns: how the registry is wired into the tuning UI, the
  bake palette resolution, and the cascade application. The registry
  module never reaches into app.js globals.
- `settings.go` owns: enum validation. The Go side does NOT know
  preset bake_config contents — only that the id is one of six.

## Public interfaces

| Surface                            | Caller                  |
|------------------------------------|-------------------------|
| `window.getLightingPreset(id)`     | app.js                  |
| `window.listLightingPresets()`     | app.js (dropdown init)  |
| `applyLightingPreset(id)`          | wireTuningUI input hook |
| `getActiveBakePalette()`           | bake render helpers     |
| `validLightingPresets[id]`         | Validate() (settings.go)|

## Ordering of changes

The Go change is independent and can land first or last. The
frontend changes have an internal order:

1. Add `static/presets/lighting.js`.
2. Add the `<script>` tag in `index.html` and clear the hardcoded
   `<option>`.
3. Wire `populateLightingPresetSelect()` and `applyLightingPreset()`
   in `app.js`. At this point picking a preset already updates the
   intensity sliders and saves.
4. Wire `getActiveBakePalette()` into `setupBakeLights()`,
   `renderLayerTopDown()`, and `createBakeEnvironment()`. At this
   point regenerate produces the warmer-tones output.
5. Bump `validLightingPresets` in Go + add test.

Steps 1–2, 3, 4, 5 each commit atomically. Order matters between 1
and 2 (script must exist before being referenced) and between 3 and
the rest of the frontend changes (the dropdown needs to be populated
before the user can select anything).

## What is NOT changing

- `AssetSettings` struct shape — no new persisted field.
- `SchemaVersion` — stays at 1.
- `SettingsDifferFromDefaults()` — already covers all six dependent
  fields; no additions.
- Profile system — preset id is part of `currentSettings` and so is
  saved/loaded by profiles already; no profile-specific change.
- Live preview light setup — T-007-02.
