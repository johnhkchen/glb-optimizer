# Review — T-007-03: reference-image-as-preset-option

## What changed

### Files modified (5)

- `static/presets/lighting.js`:
  - New seventh entry `from-reference-image` in `LIGHTING_PRESETS`,
    inserted after `indoor`. `name: "From Reference Image"`,
    `description: "Calibrated from your reference image"`.
    `bake_config` is a numerical clone of `default` (neutral
    baseline) — see Open concern #2.
  - Comment notes that the colors are a runtime fallback overridden
    by `referencePalette` whenever an image is loaded.

- `static/app.js`:
  - `makeDefaults()`: removed `color_calibration_mode: 'none'`
    field. `reference_image_path` retained.
  - `TUNING_SPEC`: removed the `color_calibration_mode` entry.
  - `wireTuningUI()`: removed the
    `if (spec.field === 'color_calibration_mode')` branch (the
    dropdown is gone, so the handler is dead code).
  - `tuneResetBtn` handler: was
    `applyDefaults() + populateTuningUI() + saveSettings + applyColorCalibration`;
    is now `applyDefaults() + applyLightingPreset('default')`. The
    cascade through `applyLightingPreset` re-uses the same code path
    as a manual preset pick — it rewrites the sliders, refreshes the
    live scene, marks the bake stale, AND calls
    `applyColorCalibration` internally to tear down any active
    reference-image state.
  - `applyLightingPreset()`: now calls
    `applyColorCalibration(selectedFileId)` after `setBakeStale(true)`
    and before save / `logEvent('preset_applied', ...)`. This is the
    single new wiring point: picking the `from-reference-image`
    preset triggers `loadReferenceEnvironment` if the asset has an
    uploaded image, and switching away from it tears the calibration
    down. The function is idempotent on assets without an uploaded
    image (the tear-down branch is a no-op when there was nothing to
    tear down).
  - `applyColorCalibration()`: predicate flipped from
    `currentSettings.color_calibration_mode === 'from-reference-image'`
    to `currentSettings.lighting_preset === 'from-reference-image'`.
    No body change.
  - `selectFile()` (line ~3053): the asset-open path's
    `if (mode === 'from-reference-image' && file.has_reference)`
    predicate flipped to read the lighting preset id.
  - `uploadReferenceImage()` (line ~2029): the post-upload "should I
    apply now?" gate flipped to read the lighting preset id.
  - `syncReferenceImageRow()`: predicate flipped to read the
    lighting preset id. Function name and docstring updated.

- `static/index.html`:
  - Deleted the `<div class="setting-row">` containing
    `<select id="tuneColorCalibrationMode">` (was lines 318-324).
  - `referenceImageRow` retained as-is — its visibility is now
    driven by the lighting-preset id in `syncReferenceImageRow`.

- `settings.go`:
  - Deleted the `ColorCalibrationMode string` field from
    `AssetSettings`. `ReferenceImagePath` retained.
  - Deleted `ColorCalibrationMode: "none"` from `DefaultSettings()`.
  - Deleted `validColorCalibrationModes` map and the matching
    `Validate()` clause.
  - Deleted the `SettingsDifferFromDefaults` row for the removed
    field.
  - Added `"from-reference-image": true` to `validLightingPresets`.
  - Replaced the `if s.ColorCalibrationMode == ""` forward-compat
    block in `LoadSettings` with a new T-007-03 migration block:
    re-decode the same byte slice into
    `struct { ColorCalibrationMode *string }` and, if the legacy key
    is present and equals `"from-reference-image"` AND the explicit
    `LightingPreset` is still the bare default `"default"`, rewrite
    `LightingPreset` to `"from-reference-image"`. Explicit
    non-default presets win.
  - Updated the struct doc comment to drop "color calibration mode."

- `settings_test.go`:
  - Removed the two calibration-mode rejection cases from
    `TestValidate_RejectsOutOfRange`.
  - Replaced the `color_calibration_mode` case in
    `TestSettingsDifferFromDefaults > single_field_mutated` with a
    `lighting_preset` case using the new preset id.
  - Added `"from-reference-image"` to the id list in
    `TestValidate_AcceptsAllPresets`.
  - Deleted `TestLoadSettings_NormalizesColorCalibrationMode` (its
    target — the empty-string normalization — is gone).
  - Added `TestLoadSettings_MigratesColorCalibrationMode`: writes a
    legacy doc with `color_calibration_mode: "from-reference-image"`
    + `lighting_preset: "default"`, asserts that
    `loaded.LightingPreset == "from-reference-image"` after load.
  - Added `TestLoadSettings_ExplicitPresetWinsOverLegacyMode`:
    writes a legacy doc with both keys but
    `lighting_preset: "midday-sun"`, asserts the explicit preset is
    preserved.

- `docs/knowledge/settings-schema.md`:
  - Removed the `color_calibration_mode` row from the field table.
  - Removed the `"color_calibration_mode": "none"` line from the
    JSON example.
  - Extended the `lighting_preset` row enum to include
    `"from-reference-image"` and added a sentence describing the
    runtime palette override.
  - Added a paragraph in "Forward-compat normalization" describing
    the new T-007-03 migration hop.

### Files created / deleted

None.

## Commit history

```
f6fa156 Remove ColorCalibrationMode field, migrate legacy presets (T-007-03)
dd4c7d8 Remove color_calibration_mode from frontend (T-007-03)
b0a58dc Trigger reference calibration from lighting preset (T-007-03)
37939a0 Accept from-reference-image lighting preset (T-007-03)
89dc166 Add from-reference-image to lighting preset registry (T-007-03)
```

Five atomic commits matching plan.md exactly. Each is independently
revertable. Reverting just `f6fa156` (the Go field removal) leaves
the JS-side preset wiring intact and reintroduces the on-disk
field, which is the cleanest rollback boundary if a downstream
consumer turns out to depend on the legacy shape.

## Acceptance criteria check

- [x] **New preset `from-reference-image` added to the preset enum
      from T-007-01.** Added to `LIGHTING_PRESETS` in
      `static/presets/lighting.js` and to `validLightingPresets` in
      `settings.go`.
- [x] **When this preset is selected, the bake/preview lights are
      tinted by the palette extracted from the reference image
      (existing code from T-005-03).** `applyLightingPreset` now
      calls `applyColorCalibration`, which calls
      `loadReferenceEnvironment` when
      `lighting_preset === 'from-reference-image'` and the asset has
      `has_reference`. The bake side reuses
      `getActiveBakePalette` which already prioritizes
      `referencePalette` over the preset's `bake_config` (T-007-01
      did this work).
- [x] **The tuning panel's lighting preset dropdown shows this
      option alongside the named presets, with a one-line
      description ("Calibrated from your reference image").**
      `populateLightingPresetSelect` reads from
      `listLightingPresets()`, which now includes the seventh entry.
      The dropdown's `option.title` carries the description.
- [x] **Selecting this preset shows the reference image upload
      control inline.** `syncReferenceImageRow()` now toggles
      `#referenceImageRow` visibility based on
      `currentSettings.lighting_preset === 'from-reference-image'`.
      The same row already contained the upload button from T-005-03.
- [x] **The `color_calibration_mode` setting from T-005-03 is
      removed.** Removed from `AssetSettings`, `DefaultSettings`,
      `Validate`, `SettingsDifferFromDefaults`, the JS
      `makeDefaults`/`TUNING_SPEC`, and the HTML row. Per the
      ticket's First-Pass Scope this is acceptable for a single-user
      dev tool.
- [x] **Backwards compatibility: existing assets with
      `color_calibration_mode: from-reference-image` are
      auto-migrated to `lighting_preset: from-reference-image`.**
      `LoadSettings` runs the migration via a `*string` re-decode
      probe; covered by `TestLoadSettings_MigratesColorCalibrationMode`.
      `TestLoadSettings_ExplicitPresetWinsOverLegacyMode` covers
      the precedence rule.
- [ ] **Manual verification not yet performed.** Per the AC: "load
      rose, set lighting preset to `from-reference-image`, upload
      rose photo, regenerate, see calibrated bake; switch to
      `midday-sun`, regenerate, see neutral white bake." See
      "Manual checklist" below.

## Test coverage

- **Go**: `go test ./...` clean (`ok glb-optimizer`) after every
  step. Two new tests cover the migration; one cleans up the
  removed-field cases.
  - `TestValidate_AcceptsAllPresets/from-reference-image` —
    validator accepts the new preset.
  - `TestLoadSettings_MigratesColorCalibrationMode` — legacy doc
    with the bare-default preset gets migrated.
  - `TestLoadSettings_ExplicitPresetWinsOverLegacyMode` — explicit
    non-default preset is preserved.
  - `TestSettingsDifferFromDefaults > single_field_mutated/lighting_preset`
    — flag still fires when the preset is mutated to the new id.
- **JS**: no test harness in the repo. Verifications relied on:
  - Reading the new `applyLightingPreset` flow against
    `applyColorCalibration` to confirm the predicate swap is
    consistent across all five touch sites.
  - Grep verification that the only remaining mention of
    `color_calibration_mode` in the JS sources is the absence of
    the field — i.e. the field is gone from `makeDefaults`,
    `TUNING_SPEC`, and the wireTuningUI handler. Spot-checked with:
    `grep -n color_calibration_mode static/app.js static/index.html`
    → no matches.
  - Tracing the reset-button new path: `applyDefaults` resets
    currentSettings; `applyLightingPreset('default')` rewrites
    sliders, calls `applyPresetToLiveScene`, `setBakeStale(true)`,
    `applyColorCalibration` (which falls into the tear-down branch
    because the new preset id is now `default`), saves, emits
    `preset_applied`. End state: identical to the old reset path
    plus correct calibration tear-down.
- **Manual checklist** (for the human reviewer):
  1. Load any asset with no reference image yet. Pick
     `From Reference Image` from the lighting preset dropdown.
     Expected: the upload row appears below the dropdown; the
     intensity sliders snap to the neutral baseline (matching the
     Default preset numbers); the live preview shows the neutral
     baseline (no calibration tint, because no image is loaded);
     the bake-out-of-date hint appears.
  2. Click the inline "Upload reference image" button and pick a
     photo. Expected: live preview adopts the calibrated tint
     (ambient/hemi/directionals all warm or cool to match the
     image); the upload row stays visible.
  3. Click "Production Asset" → wait for completion. Expected: the
     bake reflects the calibrated palette; the bake-stale hint
     disappears.
  4. Switch the lighting preset to `Midday Sun`. Expected: live
     preview drops the calibration tint and adopts the midday-sun
     preview palette; the upload row disappears; the bake-stale
     hint reappears.
  5. Click the Reset button. Expected: lighting preset returns to
     `Default`; sliders snap to defaults; any active calibration is
     torn down (live preview returns to neutral); the bake-stale
     hint reappears.
  6. Hand-edit a settings JSON file in `~/.glb-optimizer/settings/`
     to add `"color_calibration_mode": "from-reference-image"` and
     leave `"lighting_preset": "default"`. Restart the server and
     reopen that asset. Expected: the loaded settings show
     `lighting_preset: from-reference-image` (migration succeeded);
     if the asset also has an uploaded image, the calibration is
     applied automatically on selectFile.
  7. Reload the page after picking the new preset on an asset with
     an uploaded reference image. Expected: the asset still shows
     the calibrated preview after `selectFile` (the preset id is
     persisted in the settings JSON; `selectFile` honors it).
  8. Switch from a calibrated asset to a different asset that does
     NOT have a reference image. Expected: the preset selection on
     the new asset is loaded from its own settings file (not
     leaked from the previous asset); calibration state from the
     previous asset is cleared.

## Open concerns

1. **Manual visual verification not performed.** The agent has no
   browser; the AC's "load rose, upload rose photo, regenerate,
   see calibrated bake" check is on the human reviewer. Per the
   manual checklist above. If the calibration doesn't apply when
   it should, the most likely cause is that
   `applyLightingPreset` → `applyColorCalibration` runs before
   `file.has_reference` is set on a freshly-uploaded image; in
   that case the upload-side path in `uploadReferenceImage` (which
   gates on the lighting preset id and calls
   `loadReferenceEnvironment` directly) is the safety net.

2. **The new preset's `bake_config` is a redundant copy of the
   `default` preset.** Intentionally — when no image is loaded,
   the user should see a neutral baseline rather than zeroed
   sliders. Two design alternatives were considered and rejected
   in `design.md` D1 (zero-everything sentinel; conditional
   cascade skip). The redundant copy means picking the new preset
   on a fresh asset is numerically identical to picking `default`,
   plus the upload-row affordance and the calibration trigger.
   Acceptable per First-Pass Scope, but worth flagging in case a
   future ticket wants to add a sentinel-mode helper that
   suppresses the cascade for "data-driven" presets.

3. **The migration is one-way and lossy.** Once a settings file is
   loaded by a server with the T-007-03 schema, the
   `color_calibration_mode` JSON key is silently dropped on the
   next save (the field no longer exists in the struct).
   Acceptable per First-Pass Scope ("the migration step is
   cosmetic"), but a forensic audit of an old file post-load
   would not show the original calibration state. Mitigation: the
   git history of this commit chain documents the rewrite rule.

4. **`reference_image_path` is now an orphan field.** It still
   gets set by the upload flow but no Go code reads it. It's
   useful as a tag for diagnostic purposes and as a forward
   compatibility hook for a future "multiple reference images"
   feature. Removing it is out of scope; flagging only because the
   field-removal pattern in this ticket might tempt a future
   reviewer to also remove it.

5. **`applyColorCalibration` is now called from inside
   `applyLightingPreset` AND from `selectFile` AND from
   `uploadReferenceImage`.** Three call sites; mostly orthogonal:
   `selectFile` is asset-open, `uploadReferenceImage` is the
   immediate-after-upload affordance, `applyLightingPreset` is the
   user picking the preset. They do NOT call each other (no
   recursion risk). Spot-checked.

6. **Reset button now emits a `preset_applied` analytics event
   instead of N `setting_changed` events.** This is a behavioral
   change for the reset path. The old reset emitted no events at
   all (it called `applyDefaults + populateTuningUI + save +
   applyColorCalibration`, none of which emit). The new path goes
   through `applyLightingPreset` which emits one
   `preset_applied { from: 'X', to: 'default', changed: {...} }`.
   This is arguably better for analytics — the reset becomes
   visible — but it's a deviation from the implicit "reset is
   silent" contract. Flag for the reviewer; trivial to revert
   that single line if undesirable.

7. **The legacy `color_calibration_mode` migration only fires
   once.** After the first save with the new schema the legacy
   key is gone from disk and the migration is a no-op forever
   after. This is fine — it's the desired terminal state — but if
   a user *manually* re-adds the legacy key to the JSON file
   between sessions, the migration would re-fire each load.
   Acceptable; harmless idempotent rewrite.

## Out of scope (for clarity)

- Improving the palette extraction itself (per ticket Out of Scope).
- Multiple calibration sources (HDR, color picker) — per ticket.
- Per-asset reference image library — per ticket.
- Removing `reference_image_path` (see Open concern #4).
- Real-time preview during the bake (deferred, see T-007-02 review).

## Build / test status

- `go build ./...` — clean.
- `go test ./...` — passes (`ok glb-optimizer 0.413s`).
- No JS test harness exists in the repo.
