# Progress — T-007-01

## Step 1 — Create the preset registry module ✓

- Added `static/presets/lighting.js` with the 6 presets, `getLightingPreset`,
  `listLightingPresets`, `PRESET_FIELD_MAP`, and a dev-time
  default-drift assertion.
- **Deviation**: design.md said "non-module global script". On
  inspection, `static/app.js` is loaded with `type="module"` and uses
  ES imports. Switched lighting.js to also be an ES module
  (`export const`/`export function`). app.js imports it via
  `./presets/lighting.js`.
- Updated `//go:embed static/*` → `//go:embed static` in main.go so
  the new subdirectory is embedded into the binary. `go build`
  passes.

## Step 2 — Wire script tag and clear hardcoded option ✓

- index.html no longer needs an extra `<script>` tag (the import
  graph from app.js's module pulls lighting.js automatically).
- Replaced the inline `<option value="default">default</option>` in
  `#tuneLightingPreset` with a comment marker; the dropdown is now
  populated at init by `populateLightingPresetSelect()`.

## Step 3 — Populate dropdown and apply cascade ✓

- Added `populateLightingPresetSelect()` after `applyDefaults()`.
- Added `applyLightingPreset(id)` that diffs the preset against
  `currentSettings`, mutates the mapped intensity fields,
  refreshes the UI via `populateTuningUI()`, debounce-saves, and
  emits a single `preset_applied` analytics event with `{from, to,
  changed}` instead of N `setting_changed` events.
- Hooked the existing `wireTuningUI()` input listener: when
  `spec.field === 'lighting_preset'`, short-circuit to
  `applyLightingPreset(v)` and skip the per-field analytics path.
- Wired `populateLightingPresetSelect()` into the init block (right
  before `wireTuningUI()`).

## Step 4 — Apply preset colors to the bake ✓

- Added `getActiveBakePalette()` near `setupBakeLights()`. Priority:
  reference palette > active preset's bake_config colors > neutral
  fallback.
- Replaced the `referencePalette ? ... : ...` ternaries in
  `setupBakeLights()` and the inline duplicate inside
  `renderLayerTopDown()` with calls to `getActiveBakePalette()`.
- Extracted `buildGradientEnvTexture(stops, pmrem)` from the
  reference-palette branch of `createBakeEnvironment()`. Added a
  third branch: when no reference palette but the active preset is
  non-`default` and has an `env_gradient`, build the env from those
  stops. `default` still uses `RoomEnvironment()` so the regression
  guardrail (default-preset bake unchanged) holds.

## Step 5 — Extend backend enum + tests ✓

- `validLightingPresets` in `settings.go` extended from `{default}`
  to all 6 ids. Doc comment updated to drop the "S-007 will extend
  this" note.
- Added `TestValidate_AcceptsAllPresets` (table-driven, one subtest
  per id).
- `go test ./...` passes.

## Step 6 — Schema documentation ✓

- Updated the `lighting_preset` row in
  `docs/knowledge/settings-schema.md` to enumerate the 6 valid
  values and explain the cascade. No migration row — schema_version
  stays at 1, the existing default is still valid.
