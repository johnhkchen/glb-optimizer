# Structure — T-007-03

File-level changes for Option A. Six files touched, none created or
deleted.

## `settings.go` (modified)

### Removed
- Field `ColorCalibrationMode string` (line 37).
- Field default `ColorCalibrationMode: "none"` in `DefaultSettings()`
  (line 59).
- `validColorCalibrationModes` map (lines 91-96).
- `if !validColorCalibrationModes[...]` check in `Validate()` (lines
  140-142).
- `if s.ColorCalibrationMode != d.ColorCalibrationMode || ...` row
  in `SettingsDifferFromDefaults` (line 178).
- `if s.ColorCalibrationMode == "" { s.ColorCalibrationMode = "none" }`
  forward-compat block in `LoadSettings` (lines 233-238).

### Added
- New entry `"from-reference-image": true` to
  `validLightingPresets` (after `"indoor": true`).
- New normalization in `LoadSettings`, after the JSON decode and
  before `Validate`: re-decode `data` into a probe struct with
  `ColorCalibrationMode *string` and, if it equals
  `"from-reference-image"` and the explicit `LightingPreset` is the
  default `"default"`, overwrite `s.LightingPreset = "from-reference-image"`.
  Same `*pointer` trick as the existing `ground_align` migration so
  we can distinguish "key absent" from "key present and empty."

### Unchanged
- `ReferenceImagePath` field (still used as the upload tag).
- `LightingPreset` field shape, default, validator, json tag.

## `settings_test.go` (modified)

### Removed
- Cases `"empty calibration mode"` and `"unknown calibration mode"`
  in `TestValidate_RejectsOutOfRange` (lines 77-78).
- Case `"color_calibration_mode"` in
  `TestSettingsDifferFromDefaults > single_field_mutated` (line 222).
- Test `TestLoadSettings_NormalizesColorCalibrationMode` (lines
  245-285) — its target is gone.

### Added
- New test `TestValidate_AcceptsFromReferenceImagePreset`
  (or extend `TestValidate_AcceptsAllPresets` to include the new id).
- New test `TestLoadSettings_MigratesColorCalibrationMode`: write a
  legacy doc with `color_calibration_mode: "from-reference-image"`
  and `lighting_preset: "default"`; expect `loaded.LightingPreset
  == "from-reference-image"` after `LoadSettings`.
- New test `TestLoadSettings_ExplicitPresetWinsOverLegacyMode`:
  legacy doc with both `color_calibration_mode: "from-reference-image"`
  AND a non-default `lighting_preset` (e.g. `"midday-sun"`); expect
  the explicit preset wins.

### Unchanged
- All existing pre-T-007-03 fixture documents that omit the new
  preset id (they default to `"default"` and continue to load).
- The `case "color_calibration_mode"` mention in
  `TestSettingsDifferFromDefaults` is removed; the test still
  exists for the other fields.

## `static/presets/lighting.js` (modified)

### Added
- New preset entry `"from-reference-image"` inserted after `"indoor"`
  in `LIGHTING_PRESETS`. `bake_config` is a near-clone of `default`
  (neutral white). `description` is "Calibrated from your reference
  image" (the AC's exact wording).
- A short comment explaining that this preset's colors are a
  *fallback* — the runtime `referencePalette` overrides them
  whenever an image is loaded.

### Unchanged
- Schema, `makePreset`, `getLightingPreset`, `listLightingPresets`,
  `PRESET_FIELD_MAP`, the dev-time default-drift assertion.

## `static/app.js` (modified)

### Removed
- `color_calibration_mode: 'none'` from `makeDefaults()` (line 136).
  `reference_image_path` stays.
- TUNING_SPEC entry for `color_calibration_mode` (line 343).
- The `if (spec.field === 'color_calibration_mode')` branch inside
  the `wireTuningUI` input handler (lines 409-412).
- The post-reset `applyColorCalibration(selectedFileId)` call (line
  430), replaced — see below.
- The standalone `applyColorCalibration(id)` function (lines 3084-
  3109) **stays as a function** but loses its public role; it is
  called only from the new branch inside `applyLightingPreset` and
  from `selectFile`. (Could rename to `syncReferenceCalibration` for
  clarity but that's pure churn — keeping the name.)

### Added
- New branch inside `applyLightingPreset(id)`: after the cascade,
  after `populateTuningUI`, after `applyPresetToLiveScene`, after
  `setBakeStale(true)`, but before save / `logEvent`, call
  `applyColorCalibration(selectedFileId)`. The function already
  inspects `currentSettings.color_calibration_mode`; we change it
  to inspect `currentSettings.lighting_preset === 'from-reference-image'`.
- New `applyColorCalibration` body uses
  `currentSettings.lighting_preset === 'from-reference-image'` as
  its predicate instead of `color_calibration_mode === 'from-reference-image'`.
  No other body change.
- `syncReferenceImageRow` predicate flipped to
  `currentSettings.lighting_preset === 'from-reference-image'`.
- `selectFile` predicate at line 3053 flipped to the same.
- `uploadReferenceImage` gate at line 2029 flipped to the same.

### Changed (cleanup)
- `tuneResetBtn` handler at line 421-432: replace
  `applyDefaults(); populateTuningUI(); save; applyColorCalibration`
  with `applyDefaults(); applyLightingPreset('default');` so the
  reset path goes through the same cascade as a manual preset pick
  (which also tears down calibration). Net: -3 lines.

### Unchanged
- `getActiveBakePalette`, `getActivePreviewPalette`,
  `applyPresetToLiveScene`, `applyReferenceTint`,
  `loadReferenceEnvironment`, `extractPalette`,
  `setupBakeLights`, `createBakeEnvironment`, the analytics layer,
  the cascade in `applyLightingPreset` itself.

## `static/index.html` (modified)

### Removed
- The entire `<div class="setting-row">` for `tuneColorCalibrationMode`
  (lines 318-324).

### Unchanged
- `referenceImageRow` (lines 326-328) — its visibility is now driven
  off the preset dropdown but the markup is the same.
- `tuneLightingPreset` markup.

## `docs/knowledge/settings-schema.md` (modified)

### Removed
- The `color_calibration_mode` row from the field table (line 39).
- The `"color_calibration_mode": "none"` line from the JSON example.

### Updated
- The `lighting_preset` row's enum literal grows to include
  `"from-reference-image"`, and a sentence is added: "When set to
  `from-reference-image`, the active per-asset reference image (if
  any) is used to derive the bake/preview lighting via the
  palette-extraction path; the preset's own colors are a neutral
  fallback when no image is loaded yet."
- A short note in "Migration Policy / Forward-compat normalization"
  describing the legacy `color_calibration_mode` → `lighting_preset`
  hop.

### Unchanged
- `reference_image_path` row (still relevant).
- Versioning, storage, endpoint sections.

## Module boundaries

No new modules. The preset registry stays the single source of
truth for the dropdown (which is the whole point of this ticket).
The Go validator stays a pass-through enum check; the JS does the
actual cascade and the side effects.

## Ordering

Strict per `plan.md` step order. The Go schema change is last
because the on-disk shape changes (field removed) and we want the
JS code to already know not to write the old key. In practice the
Go and JS sides don't read each other's defaults, so any order
works — but writing the migration test FIRST and then deleting
the field is the safest sequence for catching regressions.
