# Progress — T-007-01

## Step 1 — Create the preset registry module ✓

- Added `static/presets/lighting.js` with the 6 presets, `getLightingPreset`,
  `listLightingPresets`, `PRESET_FIELD_MAP`, and a dev-time
  default-drift assertion.
- Module is an ES module (`export const`/`export function`) since
  app.js is also a module (`type="module"` in index.html). The design
  doc said "global script"; corrected to ES module on discovery.
