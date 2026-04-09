# Research — T-007-03: reference-image-as-preset-option

Map of the surfaces that touch the two enums this ticket is folding
together: the `lighting_preset` set introduced in T-007-01 and the
`color_calibration_mode` set introduced in T-005-03. Descriptive only.

## The two enums today

### `lighting_preset` (T-007-01)
- **Schema (Go)** — `settings.go:34` `LightingPreset string`,
  default `"default"` (line 56). Validated against
  `validLightingPresets` (line 73), the closed set
  `{default, midday-sun, overcast, golden-hour, dusk, indoor}`. The
  enum is also documented in `docs/knowledge/settings-schema.md:36`.
- **Schema (JS)** — `static/app.js:133` initial value in
  `makeDefaults()`. Auto-instrumented in `TUNING_SPEC` at line 333.
- **Preset registry** — `static/presets/lighting.js`. Each preset is
  a frozen `{id, name, description, bake_config, preview_config}`.
  `bake_config` is the source of truth for intensities + colors;
  `preview_config` is its sibling for the live three.js scene
  (T-007-02). `makePreset` at line 39 deep-clones bake_config into
  preview_config and shallow-merges optional `preview_overrides`.
  `getLightingPreset(id)` at line 194 returns `undefined` for
  unknown ids — callers MUST fall back to `default`.
- **UI** — `static/index.html:295` `<select id="tuneLightingPreset">`,
  populated at init by `populateLightingPresetSelect()` in
  `static/app.js:150` from `listLightingPresets()`.
  `option.title = description` so hover surfaces the one-liner.
- **Application** — `applyLightingPreset(id)` at `static/app.js:169`
  is the cascade: it rewrites the dependent intensity fields via
  `PRESET_FIELD_MAP`, repopulates the tuning UI, refreshes the live
  scene via `applyPresetToLiveScene()`, marks the bake stale, saves,
  and emits a single `preset_applied` analytics event in lieu of N
  `setting_changed`s. The cascade trigger fires from the generic
  TUNING_SPEC handler at line 381 by intercepting
  `spec.field === 'lighting_preset'`.

### `color_calibration_mode` (T-005-03)
- **Schema (Go)** — `settings.go:37` `ColorCalibrationMode string`,
  default `"none"` (line 59). Validated against
  `validColorCalibrationModes` (line 93) — `{none, from-reference-image}`.
  Plus `ReferenceImagePath string` (line 38) — a free-string tag
  pointing to the on-disk reference image. Both documented in
  `docs/knowledge/settings-schema.md:39-40`.
- **Schema (JS)** — `static/app.js:136` `color_calibration_mode: 'none'`
  in `makeDefaults()`. TUNING_SPEC entry at line 343.
- **UI** — `static/index.html:318` a second `<select>`
  `tuneColorCalibrationMode` with two hardcoded options. Plus
  `referenceImageRow` at line 326 — a setting row containing the
  in-panel "Upload reference image" button, hidden by default.
- **Application** — three call sites:
  1. `wireTuningUI()` at `static/app.js:409`: when the dropdown
     fires `input`, `syncReferenceImageRow()` toggles the upload
     row's visibility, then `applyColorCalibration(id)` re-evaluates
     the live scene.
  2. `selectFile()` at `static/app.js:3050`: after the per-asset
     load, if `mode === 'from-reference-image'` AND
     `file.has_reference`, call `loadReferenceEnvironment(id)`
     before populating the UI / loading the model. This is the
     "honor the saved mode on selection" path.
  3. `Reset to defaults` button at `static/app.js:430`: after
     resetting `currentSettings`, calls `applyColorCalibration` to
     tear down the live calibration if it was on.
- **`applyColorCalibration(id)`** at `static/app.js:3084` is the
  toggle: if `mode === 'from-reference-image' && file.has_reference`,
  call `loadReferenceEnvironment` (which sets `referencePalette` +
  `referenceEnvironment` and tints the live scene), reload the
  model preview. Otherwise, dispose `referenceEnvironment`, null
  `referencePalette`, restore `defaultEnvironment`, fall back to
  the active preset via `applyPresetToLiveScene`, reload the
  preview.
- **`syncReferenceImageRow()`** at `static/app.js:3072` — single
  read of `currentSettings.color_calibration_mode`. Called from
  `populateTuningUI` (line 362) and the calibration-mode change
  handler.

## How `referencePalette` and `referenceEnvironment` plug into bake/preview

- **State** — module-level globals at `static/app.js:33-34`:
  `referenceEnvironment` (PMREM env tex) and `referencePalette`
  (the `{bright, mid, dark}` extracted by `extractPalette`).
- **Bake side** — `getActiveBakePalette()` at line 1004 checks
  `referencePalette` FIRST, then falls through to the preset's
  `bake_config`, then to a hardcoded neutral. `createBakeEnvironment`
  at line 1162 has the matching priority for the env map.
  `setupBakeLights` reads the palette via `getActiveBakePalette`.
- **Preview side** — `getActivePreviewPalette()` at line 1024 has
  the same priority as the bake helper. `applyPresetToLiveScene`
  at line 1049 calls `getActivePreviewPalette` and then mutates
  the live three.js lights in place. `applyReferenceTint` at line
  2233 is the legacy direct-mutation path used after a successful
  upload — it walks the scene and tints ambient/hemi/directionals.
- **Upload flow** — `uploadReferenceImage(id, file)` at line 2009:
  POST the image, mark `file.has_reference = true`, persist
  `currentSettings.reference_image_path`, then — gated by
  `color_calibration_mode === 'from-reference-image'` — call
  `loadReferenceEnvironment(id)` and reload the preview.

## Touchpoints relevant to the migration

- **Go validator** — `validLightingPresets` is a closed map.
  Adding a new id requires adding the line and a regression test.
  `validColorCalibrationModes` at line 93 will be removable once
  the Go field is removed.
- **Go default** — `DefaultSettings` returns
  `LightingPreset: "default", ColorCalibrationMode: "none",
  ReferenceImagePath: ""`. Removing `ColorCalibrationMode` requires
  dropping the literal AND the `if s.ColorCalibrationMode == ""`
  forward-compat normalization at line 236.
- **Go diff helper** — `SettingsDifferFromDefaults` at line 163
  enumerates fields explicitly. Removing the field forces a
  compile-time visit; the matching test case
  `"color_calibration_mode"` at `settings_test.go:222` would also
  be removed.
- **Go tests** — `settings_test.go` references the field in
  `TestValidate_RejectsOutOfRange` (lines 77-78),
  `TestSettingsDifferFromDefaults` (line 222),
  `TestLoadSettings_NormalizesColorCalibrationMode` (lines 245-285),
  and the legacy fixture documents which omit the key. Each is a
  removal site or a migration-target site.
- **JS makeDefaults** — line 136 emits `color_calibration_mode`
  and `reference_image_path`. The latter stays (still useful as a
  tag); the former goes.
- **JS TUNING_SPEC** — line 343 entry would be removed; line 333
  `lighting_preset` entry stays (still the cascade).
- **JS handlers** — the `if (spec.field === 'color_calibration_mode')`
  branch at line 409 goes; its responsibilities (toggle the upload
  row, re-apply calibration) get folded into `applyLightingPreset`
  and `syncReferenceImageRow` reads the preset id instead.
- **HTML** — the entire `tuneColorCalibrationMode` setting row at
  index.html:318-324 is removed; `referenceImageRow` at 326-328
  stays but its visibility is now driven by the preset id.

## Constraints & assumptions

- **Single-user dev tool.** Per the ticket's First-Pass Scope and
  per CLAUDE.md, deleting the old field outright is acceptable.
  The forward-compat path I'll add (`color_calibration_mode:
  from-reference-image` → `lighting_preset: from-reference-image`)
  is best-effort — if it's missing, no production data is at risk.
- **The new preset is unusual.** Unlike the existing six, the
  `from-reference-image` preset has no intrinsic colors — they
  come from the user's uploaded image at runtime via
  `referencePalette`. Its `bake_config` is therefore a neutral
  fallback used only when the asset has no reference image yet.
  The existing reference-palette priority in `getActiveBakePalette`
  / `getActivePreviewPalette` already handles this — when a palette
  is loaded, it wins over the preset's colors.
- **`referencePalette` is global, not per-asset.** Switching assets
  triggers `selectFile` which already nulls it. Switching to the
  `from-reference-image` preset on an asset without a reference
  image leaves `referencePalette === null`, and the preset falls
  back to its neutral bake_config — same behavior the legacy
  `applyColorCalibration` had when `file.has_reference === false`.
- **`PRESET_FIELD_MAP` cascade.** The existing applyLightingPreset
  rewrites all six dependent intensity fields. For the
  `from-reference-image` preset I want the cascade to behave
  identically — the user should still see the intensity sliders
  jump to a sensible (neutral) baseline. So the new preset's
  `bake_config` will essentially mirror `default` numerically,
  with the runtime palette override doing the actual color work.
- **Analytics.** `preset_applied` already captures the cascade, so
  no schema bump is needed. The removed `setting_changed` for
  `color_calibration_mode` is a wash — its work is now folded
  into `preset_applied`.
