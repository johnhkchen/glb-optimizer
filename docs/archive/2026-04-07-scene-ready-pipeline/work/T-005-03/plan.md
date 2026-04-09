# Plan — T-005-03: color-calibration-mode

## Step sequence

### Step 1 — Backend schema, tests, schema doc

**Files:** `settings.go`, `settings_test.go`,
`docs/knowledge/settings-schema.md`

**Goal:** Land the two new fields on disk and in memory, with
validation, normalization, dirty-detection, and a regression test for
the legacy-load path. Ship the schema doc update in the same commit
so the source-of-truth table never lags the struct.

- Append `ColorCalibrationMode string` and
  `ReferenceImagePath string` to `AssetSettings`, after `GroundAlign`.
  Tag the first as `json:"color_calibration_mode"` and the second as
  `json:"reference_image_path,omitempty"`.
- Add `validColorCalibrationModes` map literal with the two legal
  values.
- Initialize both fields in `DefaultSettings()`
  (`"none"` / `""`).
- Add the enum check to `Validate()`.
- Add the empty-string normalization to `LoadSettings`, **after** the
  ground-align re-decode.
- Extend `SettingsDifferFromDefaults` with both field comparisons.
- Add the legacy-doc test
  (`TestLoadSettings_NormalizesColorCalibrationMode`) and the two new
  cases in `TestValidate_RejectsOutOfRange`. Extend
  `TestSettingsDifferFromDefaults` `single_field_mutated` to mutate
  `ColorCalibrationMode` and `ReferenceImagePath`.
- Update `docs/knowledge/settings-schema.md`: two table rows + JSON
  example update.
- `go test ./... -run TestSettings` and `go test ./...` and
  `go build ./...` all green.

**Verification:**
1. `go test ./...` passes.
2. `go build ./...` clean.
3. Manual: `curl GET /api/settings/<id>` returns the two new fields
   with their defaults. `curl PUT` with a body that omits
   `color_calibration_mode` decodes, normalizes, and round-trips with
   `"none"` set on the response.
4. Manual: `curl PUT` with `"color_calibration_mode": "preset-x"` is
   rejected with HTTP 400.

**Commit:** `Add color_calibration_mode and reference_image_path settings (T-005-03)`

### Step 2 — Frontend wiring (HTML, TUNING_SPEC, helpers, gates)

**Files:** `static/index.html`, `static/app.js`

**Goal:** Surface the dropdown, hide/show the in-panel reference
upload button, gate calibration application on the mode in three
places (selectFile, dropdown change, upload).

- HTML: insert the two new `setting-row` blocks (`tuneColorCalibrationMode`
  select + `referenceImageRow` button) just above the reset-button
  row inside `#tuningSection`.
- JS:
  - `makeDefaults()`: add the two new keys.
  - `TUNING_SPEC`: append the `color_calibration_mode` row.
  - Add `syncReferenceImageRow()` and `applyColorCalibration(id)`.
  - `populateTuningUI`: call `syncReferenceImageRow()` after the
    walker.
  - `wireTuningUI`: in the input handler, after the analytics emit,
    branch on `spec.field === 'color_calibration_mode'` and call
    `syncReferenceImageRow()` + `applyColorCalibration(selectedFileId)`.
  - `wireTuningUI`: at the bottom (next to the reset button wiring),
    add the `#tuneReferenceImageBtn` click listener that triggers
    `referenceFileInput.click()`.
  - `selectFile`: reorder so `loadSettings(id)` runs *before* the
    calibration decision; gate `loadReferenceEnvironment` on
    `currentSettings.color_calibration_mode === 'from-reference-image'`
    AND `file.has_reference`. Keep `populateTuningUI` and the model
    load after.
  - `uploadReferenceImage`: write
    `currentSettings.reference_image_path = "outputs/{id}_reference{ext}"`
    and call `saveSettings`. Gate the live-scene mutation on the
    mode.
- `node -c static/app.js` clean.

**Verification:**
1. `node -c static/app.js` clean.
2. Open the page, select an asset → the dropdown is rendered with
   `none` selected (default), the in-panel upload row is hidden.
3. Switch the dropdown to `from-reference-image` → `setting_changed`
   fires (devtools network), the in-panel upload button row appears.
4. Click the in-panel button → file picker opens. Pick the rose
   reference image. Upload completes, `reference_image_path` is
   PUT, and the live preview tints to the new palette.
5. Switch the dropdown back to `none` → live preview returns to
   neutral, in-panel button row hides.
6. Toolbar "Reference Image" button still works (regression check
   from T-005-02 era).
7. Reset-to-defaults button → mode returns to `none`, in-panel
   button hides.
8. Panel-header dirty dot reflects the mode field's divergence from
   defaults (turns on when set to `from-reference-image`, off when
   reset).
9. File-list `settings_dirty` marker (from T-005-02) appears when
   mode is non-default after the next natural list refresh.

**Commit:** `Wire color_calibration_mode into tuning panel (T-005-03)`

## Testing strategy

| Layer | What runs | Where |
|---|---|---|
| Go unit | `TestValidate_RejectsOutOfRange` (+2 cases) | `settings_test.go` (Step 1) |
| Go unit | `TestSettingsDifferFromDefaults` `single_field_mutated` (+2 cases) | `settings_test.go` (Step 1) |
| Go unit | `TestLoadSettings_NormalizesColorCalibrationMode` (new) | `settings_test.go` (Step 1) |
| Go unit | All existing settings tests (defaults, validate, round-trip, migration) | unchanged — must remain green |
| Go build | `go build ./...` | After Step 1 |
| JS syntax | `node -c static/app.js` | After Step 2 |
| Manual integration | `curl` PUT/GET round-trip with the new field | After Step 1 |
| Manual UI | dropdown wiring, in-panel upload button, calibration apply/teardown, dirty dot, reset | After Step 2 |
| Manual rose verification (operator) | Load rose, mode → from-reference-image, upload rose photo, regenerate, observe tinted bake; flip back to none, regenerate, observe neutral | After Step 2 |

The project still has no JS test runner (per T-002-03 / T-005-01 /
T-005-02 reviews), so JS-side controls are validated by reading +
manual exercise.

## Verification criteria (full ticket)

- [ ] `go test ./...` and `go build ./...` clean.
- [ ] `node -c static/app.js` clean.
- [ ] `AssetSettings` carries `ColorCalibrationMode` and
      `ReferenceImagePath` with documented defaults.
- [ ] `Validate()` rejects unknown enum values.
- [ ] `LoadSettings` normalizes a missing `color_calibration_mode`
      key to `"none"`.
- [ ] `SettingsDifferFromDefaults` participates in both new fields
      (covered by `single_field_mutated` extension).
- [ ] `docs/knowledge/settings-schema.md` documents both fields and
      the JSON example shows the new key.
- [ ] Tuning panel renders the new dropdown with the two enum values.
- [ ] Switching the dropdown to `from-reference-image` shows the
      in-panel upload button row; switching back hides it.
- [ ] In-panel upload button triggers the same file input as the
      toolbar button.
- [ ] After upload, `reference_image_path` is persisted via PUT.
- [ ] When mode is `none`, calibration is fully bypassed:
      `referencePalette` stays null, the live scene shows neutral
      lighting, and the bake renderer reads neutral colors.
- [ ] When mode is `from-reference-image` and the file has a
      reference image, calibration applies on selection AND on mode
      flip AND on fresh upload.
- [ ] `setting_changed` analytics event fires on every dropdown
      change (free via `wireTuningUI`).
- [ ] Reset-to-defaults returns the mode to `none`.

## Risks and mitigations

- **Risk:** `selectFile` reorder accidentally regresses the existing
  reference-image flow for assets that worked before this ticket.
  **Mitigation:** the reorder is mechanical — `loadSettings` already
  ran inside the same `.then()` chain; the change moves the
  calibration decision *after* it. Verify by reading: there is no
  consumer of `referencePalette` between the existing reset and the
  new gate.
- **Risk:** `populateTuningUI` runs *after* `loadReferenceEnvironment`
  in the new ordering, so the dropdown briefly shows the wrong
  state. **Mitigation:** `populateTuningUI` is a synchronous DOM
  write that runs before the next animation frame. The window in
  which a wrong state is observable is sub-frame and irrelevant.
- **Risk:** `applyColorCalibration` triggers a `loadModel` reload
  that races with the model load in `selectFile`. **Mitigation:** the
  function is only called from the dropdown change handler and the
  upload handler — both happen *after* selection, when no
  selection-time `loadModel` is in flight. The selection-time
  calibration path is handled directly inside the `selectFile`
  continuation, not via `applyColorCalibration`.
- **Risk:** A user uploads a reference image while mode is `none`
  and is confused that nothing visibly happens. **Mitigation:**
  documented in review. The in-panel upload button only appears when
  mode is `from-reference-image`, so the only way to land here is via
  the toolbar button while mode is still `none`. The toolbar button
  is left for backwards compatibility; S-007 will collapse it.
- **Risk:** `SettingsDifferFromDefaults` extension is correct but
  the test case enumeration missed a field. **Mitigation:** the
  helper enumerates fields explicitly so the compiler catches a
  missed-by-rename. The test extension covers a representative case
  for each new field.

## Atomic commits

Two commits, in order:
1. `Add color_calibration_mode and reference_image_path settings (T-005-03)`
2. `Wire color_calibration_mode into tuning panel (T-005-03)`

Each commit leaves the tree buildable and the test suite green.
