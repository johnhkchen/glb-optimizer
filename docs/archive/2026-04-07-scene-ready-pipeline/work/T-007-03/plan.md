# Plan — T-007-03

Five commits, each independently revertable. Each step ends with
`go build ./... && go test ./...` clean.

## Step 1 — Add `from-reference-image` preset to the JS registry

**Files:** `static/presets/lighting.js`

**Change:** insert a seventh entry in `LIGHTING_PRESETS` after
`"indoor"`:

```js
'from-reference-image': makePreset({
    id: 'from-reference-image',
    name: 'From Reference Image',
    description: 'Calibrated from your reference image',
    bake_config: { /* neutral baseline ≈ default */ },
}),
```

with a short comment that the colors are a fallback overridden at
runtime by `referencePalette` whenever an image is loaded. No
`preview_overrides`.

**Verify:** `go build ./... && go test ./...` clean (Go
unaffected). Page loads; new option appears at the bottom of the
preset dropdown but selecting it does NOT yet trigger the
calibration flow — that wires up in step 3.

**Commit:** `Add from-reference-image to lighting preset registry (T-007-03)`

## Step 2 — Accept the new preset in the Go validator

**Files:** `settings.go`, `settings_test.go`

**Change:** add `"from-reference-image": true` to
`validLightingPresets`. Extend `TestValidate_AcceptsAllPresets` (or
add a sibling) to include the new id.

**Verify:** `go build ./... && go test ./...`. The test grows by
one case; everything else still passes.

**Commit:** `Accept from-reference-image lighting preset (T-007-03)`

## Step 3 — Wire the new preset to the calibration side effect

**Files:** `static/app.js`

**Change:** at the top of `applyColorCalibration`, switch the
predicate from `currentSettings.color_calibration_mode === 'from-reference-image'`
to `currentSettings.lighting_preset === 'from-reference-image'`.
Same swap in `selectFile` (line 3053), `uploadReferenceImage` (line
2029), and `syncReferenceImageRow` (line 3076). Then add a single
call to `applyColorCalibration(selectedFileId)` inside
`applyLightingPreset`, after `setBakeStale(true)` and before the
save / `logEvent`. The dual gate persists for now: the OLD
`color_calibration_mode` field still exists in the struct and
`makeDefaults` still emits it as `'none'`, so the predicate change
is the only behavioral change. (The "you must also have the old
field set" trap is broken in the next step.)

Important: this step intentionally does NOT remove the
`color_calibration_mode` UI/field. It only redirects the predicate.
Manually picking `from-reference-image` from the *preset* dropdown
now triggers the calibration; the old `tuneColorCalibrationMode`
dropdown is now a no-op.

**Verify:** `go build ./... && go test ./...`. JS still loads.

**Commit:** `Trigger reference calibration from lighting preset (T-007-03)`

## Step 4 — Remove `color_calibration_mode` from the JS / HTML

**Files:** `static/app.js`, `static/index.html`

**Change:**
- Drop `color_calibration_mode: 'none'` from `makeDefaults()`.
  `reference_image_path` stays.
- Drop the TUNING_SPEC entry for `color_calibration_mode` (line
  343).
- Drop the `if (spec.field === 'color_calibration_mode')` branch
  in the wireTuningUI handler (lines 409-412).
- Replace the reset-button body
  `{applyDefaults(); populateTuningUI(); save; applyColorCalibration}`
  with `{applyDefaults(); applyLightingPreset('default');}`. The
  cascade through applyLightingPreset rewrites the sliders, marks
  bake stale, refreshes the live scene, AND tears down any
  calibration in the same call.
- Delete the `<div class="setting-row">` for `tuneColorCalibrationMode`
  in `static/index.html` (lines 318-324). Leave `referenceImageRow`
  as-is — `syncReferenceImageRow` already reads the preset id
  after step 3.

**Verify:** `go build ./... && go test ./...`. Page loads, the
calibration-mode dropdown is gone, the upload row appears when the
new preset is selected, disappears otherwise.

**Commit:** `Remove color_calibration_mode from frontend (T-007-03)`

## Step 5 — Remove `ColorCalibrationMode` from the Go schema and migrate legacy files

**Files:** `settings.go`, `settings_test.go`,
`docs/knowledge/settings-schema.md`

**Change:**
- Delete the `ColorCalibrationMode` field from `AssetSettings`
  (struct + json tag), the literal in `DefaultSettings()`,
  the `validColorCalibrationModes` map, the `Validate()` clause,
  and the diff-helper line in `SettingsDifferFromDefaults`.
- Delete the `if s.ColorCalibrationMode == ""` forward-compat
  block in `LoadSettings`. Replace it with a NEW migration block:

  ```go
  // Forward-compat hop for T-007-03: legacy files used a separate
  // color_calibration_mode enum. Fold the only meaningful value
  // (from-reference-image) into the lighting preset, but only if
  // the explicit lighting_preset is still the bare default —
  // explicit user choice always wins.
  var legacy struct {
      ColorCalibrationMode *string `json:"color_calibration_mode"`
  }
  if err := json.Unmarshal(data, &legacy); err == nil &&
      legacy.ColorCalibrationMode != nil &&
      *legacy.ColorCalibrationMode == "from-reference-image" &&
      s.LightingPreset == "default" {
      s.LightingPreset = "from-reference-image"
  }
  ```

- Tests:
  - Drop `"empty calibration mode"` and `"unknown calibration mode"`
    from `TestValidate_RejectsOutOfRange`.
  - Drop the `"color_calibration_mode"` and `"reference_image_path"`-
    related case in `TestSettingsDifferFromDefaults > single_field_mutated`?
    NO — `reference_image_path` stays. Only the calibration_mode case
    goes.
  - Delete `TestLoadSettings_NormalizesColorCalibrationMode`.
  - Add `TestLoadSettings_MigratesColorCalibrationMode`: legacy doc
    with `color_calibration_mode: "from-reference-image"` and
    `lighting_preset: "default"`; assert
    `loaded.LightingPreset == "from-reference-image"`.
  - Add `TestLoadSettings_ExplicitPresetWinsOverLegacyMode`: legacy
    doc with both `color_calibration_mode: "from-reference-image"`
    AND `lighting_preset: "midday-sun"`; assert the explicit preset
    wins.
- Update `docs/knowledge/settings-schema.md`: remove the
  `color_calibration_mode` table row and the JSON-example line;
  extend the `lighting_preset` enum literal to include
  `"from-reference-image"` and add a one-line description of the
  preset's runtime semantics; add a paragraph in the
  forward-compat-normalization section noting the legacy hop.

**Verify:** `go build ./... && go test ./...`. New tests pass;
the existing legacy fixtures still load (they happen to omit the
calibration field, so the migration is a no-op for them).

**Commit:** `Remove ColorCalibrationMode field, migrate legacy presets (T-007-03)`

## Testing strategy

- **Go unit tests** for validator + migration: covered by the new
  tests in step 2 and step 5.
- **JS:** no test harness in repo. Verification is by code reading
  + the manual checklist in `review.md`.
- **Manual verification** (per AC):
  1. Load any asset with no reference image. Pick `from-reference-image`
     from the preset dropdown. The upload row should appear inline.
     Live preview should NOT calibrate (no image yet); sliders snap
     to neutral baseline; bake-stale hint appears.
  2. Click the inline "Upload reference image" button, pick a
     photo. Live preview should calibrate; baking should produce a
     calibrated bake.
  3. Switch back to `midday-sun`. Live preview should drop the
     calibration tint and adopt the midday-sun preview palette;
     upload row should disappear; bake-stale hint stays until
     regenerate.
  4. Reload the page. The asset's preset selection persists. If it
     was `from-reference-image` AND the asset has an uploaded
     reference image, the calibration is reapplied automatically
     (selectFile path).
  5. Hand-edit a settings JSON file to use the legacy
     `color_calibration_mode: "from-reference-image"` (with
     `lighting_preset: "default"`), reload the asset, confirm
     the loaded settings show `lighting_preset: "from-reference-image"`.

## Risk register

- **Risk:** Removing the Go field changes the on-disk shape;
  any in-flight settings saves from a still-running browser tab
  with a stale `color_calibration_mode` value would be silently
  dropped. **Mitigation:** dev tool, single user; reload after
  upgrade.
- **Risk:** A user picks `from-reference-image` on an asset with
  no reference image and is confused by the no-op preview.
  **Mitigation:** the upload row appears inline; the description
  ("Calibrated from your reference image") + the row visibility
  is the affordance.
