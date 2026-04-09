# Progress — T-005-03: color-calibration-mode

## Done

### Step 1 — Backend schema, tests, schema doc

- `settings.go`:
  - Appended `ColorCalibrationMode string` and
    `ReferenceImagePath string` to `AssetSettings`.
  - Added `validColorCalibrationModes` map literal.
  - Initialized both fields in `DefaultSettings()`.
  - Added enum check to `Validate()`.
  - Added empty-string normalization to `LoadSettings`, after the
    ground-align re-decode block.
  - Extended `SettingsDifferFromDefaults` with both new fields.
- `settings_test.go`:
  - Added `empty calibration mode` and `unknown calibration mode`
    cases to `TestValidate_RejectsOutOfRange`.
  - Added `color_calibration_mode` and `reference_image_path` cases
    to `TestSettingsDifferFromDefaults` `single_field_mutated`.
  - Added new test
    `TestLoadSettings_NormalizesColorCalibrationMode` covering the
    pre-T-005-03 doc shape.
- `docs/knowledge/settings-schema.md`:
  - Added two table rows for the new fields.
  - Updated the JSON example to include `color_calibration_mode`.
- `go test ./...` and `go build ./...` clean.
- Committed: `Add color_calibration_mode and reference_image_path settings (T-005-03)`

### Step 2 — Frontend wiring

- `static/index.html`:
  - Added `<select id="tuneColorCalibrationMode">` row inside
    `#tuningSection`, just above the reset-button row.
  - Added conditional `#referenceImageRow` (initial
    `display:none`) containing the in-panel
    `tuneReferenceImageBtn`.
- `static/app.js`:
  - `makeDefaults()`: added `color_calibration_mode` and
    `reference_image_path` keys.
  - `TUNING_SPEC`: added the `color_calibration_mode` row (free
    analytics + dirty-dot enrollment).
  - `populateTuningUI`: appended `syncReferenceImageRow()`.
  - `wireTuningUI`: added the post-`setting_changed` branch that
    runs `syncReferenceImageRow()` and `applyColorCalibration()` for
    the calibration mode field, and added the in-panel button click
    listener.
  - Reset-to-defaults handler: now also calls
    `applyColorCalibration()` so tearing down the mode tears down
    the live calibration.
  - New helpers `syncReferenceImageRow()` and
    `applyColorCalibration(id)` placed alongside `resetSceneLights`.
  - `selectFile`: reordered so `loadSettings` runs before the
    calibration decision and gated `loadReferenceEnvironment` on
    the mode + `has_reference`.
  - `uploadReferenceImage`: writes
    `currentSettings.reference_image_path = "outputs/{id}_reference{ext}"`
    + `saveSettings(id)`. Live-scene mutation now gated on the mode.
- `node -c static/app.js` clean.
- Committed: `Wire color_calibration_mode into tuning panel (T-005-03)`

## Deviations from plan

None of substance.

- Plan called for adding `applyColorCalibration` to the reset handler
  as part of "JS edits" — not explicitly listed in the step 2 bullet
  list but covered by the chosen approach in design.md. Implemented
  as planned.
- The structure doc described the
  `TestLoadSettings_NormalizesColorCalibrationMode` body inline. The
  implementation matches that body modulo cosmetic indentation.

## Verification performed

- `go test ./...` — all green (`ok glb-optimizer 0.396s`).
- `go build ./...` — clean.
- `node -c static/app.js` — clean.
- Manual UI / `curl` round-trip / rose verification — **deferred to
  operator**, same posture as every prior tuning ticket.

## Outstanding

- Operator-side manual verification per the ticket's last AC bullet
  (load rose, set mode, upload rose photo, regenerate, observe
  calibration; flip back to none, regenerate, observe neutral).
