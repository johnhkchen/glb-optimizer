# Progress — T-007-03

## Step 1 — Add `from-reference-image` preset to JS registry — DONE
- `static/presets/lighting.js`: inserted seventh entry after `indoor`,
  with neutral baseline `bake_config` mirroring `default` and a
  comment that the colors are a runtime fallback.
- `go build ./... && go test ./...`: clean.
- Commit: `89dc166 Add from-reference-image to lighting preset registry (T-007-03)`

## Step 2 — Accept new preset in Go validator — DONE
- `settings.go`: added `"from-reference-image": true` to
  `validLightingPresets`.
- `settings_test.go`: extended `TestValidate_AcceptsAllPresets` to
  cover the new id.
- `go build ./... && go test ./...`: clean (`ok glb-optimizer 0.416s`).
- Commit: `37939a0 Accept from-reference-image lighting preset (T-007-03)`

## Step 3 — Wire calibration to lighting preset — DONE
- `static/app.js`:
  - `applyLightingPreset`: now calls `applyColorCalibration(selectedFileId)`
    after `setBakeStale(true)` and before save / `logEvent`. Idempotent
    on assets with no reference image.
  - `applyColorCalibration`: predicate flipped from
    `color_calibration_mode === 'from-reference-image'` to
    `lighting_preset === 'from-reference-image'`.
  - `selectFile` (line ~3053) and `uploadReferenceImage` (line ~2029)
    predicates flipped to the same.
  - `syncReferenceImageRow`: predicate flipped to read the preset id.
- `go build ./... && go test ./...`: clean (cached).
- Commit: `b0a58dc Trigger reference calibration from lighting preset (T-007-03)`

## Step 4 — Remove `color_calibration_mode` from frontend — DONE
- `static/app.js`:
  - Dropped `color_calibration_mode: 'none'` from `makeDefaults()`.
    `reference_image_path` retained.
  - Dropped TUNING_SPEC entry for `color_calibration_mode`.
  - Dropped the `if (spec.field === 'color_calibration_mode')` branch
    in `wireTuningUI` (no longer needed; the dropdown is gone).
  - `tuneResetBtn` handler now `applyDefaults() + applyLightingPreset('default')`,
    which both rewrites the sliders AND tears down any active
    calibration via the same cascade as a manual preset pick. Net -3
    lines.
- `static/index.html`:
  - Deleted the `<div class="setting-row">` for `tuneColorCalibrationMode`.
    `referenceImageRow` retained as-is.
- `go build ./... && go test ./...`: clean.
- Commit: `dd4c7d8 Remove color_calibration_mode from frontend (T-007-03)`

## Step 5 — Remove Go field, add migration — DONE
- `settings.go`:
  - Deleted `ColorCalibrationMode` field, `DefaultSettings()` literal,
    `validColorCalibrationModes` map, the `Validate()` clause, and the
    `SettingsDifferFromDefaults` row.
  - Replaced the `if s.ColorCalibrationMode == ""` forward-compat hop
    with a new T-007-03 hop: `*string` re-decode of the legacy
    `color_calibration_mode` key. If present and equal to
    `"from-reference-image"` AND the explicit `lighting_preset` is
    still `"default"`, rewrite `lighting_preset` to
    `"from-reference-image"`. Explicit non-default presets win.
  - Updated the struct doc comment to drop the "color calibration
    mode" mention.
- `settings_test.go`:
  - Dropped the `"empty calibration mode"` and `"unknown calibration mode"`
    cases in `TestValidate_RejectsOutOfRange`.
  - Replaced the `"color_calibration_mode"` mutation case in
    `TestSettingsDifferFromDefaults > single_field_mutated` with a
    `"lighting_preset"` mutation pointing to the new preset id.
  - Deleted `TestLoadSettings_NormalizesColorCalibrationMode`.
  - Added `TestLoadSettings_MigratesColorCalibrationMode`: legacy doc
    with `color_calibration_mode: "from-reference-image"` +
    `lighting_preset: "default"` → asserts the preset is rewritten.
  - Added `TestLoadSettings_ExplicitPresetWinsOverLegacyMode`: legacy
    doc with both keys, explicit `lighting_preset: "midday-sun"` →
    asserts the explicit preset wins.
- `docs/knowledge/settings-schema.md`:
  - Removed the `color_calibration_mode` table row.
  - Removed the `"color_calibration_mode": "none"` line from the JSON
    example.
  - Extended the `lighting_preset` row enum to include
    `"from-reference-image"` and added a sentence explaining the
    runtime palette override.
  - Added a paragraph in `Forward-compat normalization` documenting
    the new migration hop.
- `go build ./... && go test ./...`: clean.
- Commit: `f6fa156 Remove ColorCalibrationMode field, migrate legacy presets (T-007-03)`

## Deviations from plan

None of substance. The plan said the diff helper case would be
removed in step 5; I instead **replaced** it in step 5 with a
`lighting_preset` case so the test still exercises the dirty-flag
behavior on the relevant field. This is a strict improvement over
silent removal.
