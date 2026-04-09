# Review — T-005-03: color-calibration-mode

## What changed

### Files modified

| File | Change |
|---|---|
| `settings.go` | +`ColorCalibrationMode string` and +`ReferenceImagePath string` on `AssetSettings` (declaration order = JSON order, appended after `GroundAlign`). +`validColorCalibrationModes` map literal. `DefaultSettings()` initializes `"none"`/`""`. `Validate()` rejects unknown enum values. `LoadSettings` normalizes empty `color_calibration_mode` → `"none"` (forward compat for pre-T-005-03 docs). `SettingsDifferFromDefaults` extended with both new field comparisons. |
| `settings_test.go` | +2 cases on `TestValidate_RejectsOutOfRange` (`empty calibration mode`, `unknown calibration mode`). +2 cases on `TestSettingsDifferFromDefaults` `single_field_mutated` (`color_calibration_mode`, `reference_image_path`). +`TestLoadSettings_NormalizesColorCalibrationMode` exercising the legacy-load path. |
| `docs/knowledge/settings-schema.md` | +2 rows in the field table (`color_calibration_mode`, `reference_image_path`) and the JSON example gains a `"color_calibration_mode": "none"` line. (`reference_image_path` is `omitempty`, so it does not appear in the example.) |
| `static/index.html` | +Two `setting-row` blocks inside `#tuningSection`, just above the reset-button row: a `<select id="tuneColorCalibrationMode">` with two options (`none`, `from-reference-image`) and a conditional `<div id="referenceImageRow" style="display:none">` containing `<button id="tuneReferenceImageBtn">Upload reference image</button>`. |
| `static/app.js` | `makeDefaults()` mirrors the two new defaults. `TUNING_SPEC` enrolls `color_calibration_mode` (free analytics + dirty-dot tracking). `populateTuningUI` calls `syncReferenceImageRow()` after the walker. `wireTuningUI`: input-handler tail branches on `spec.field === 'color_calibration_mode'` to call `syncReferenceImageRow()` + `applyColorCalibration()`; in-panel button wired to `referenceFileInput.click()`; reset handler also calls `applyColorCalibration()`. New helpers `syncReferenceImageRow()` and `applyColorCalibration(id)` placed near `resetSceneLights`. `selectFile` reordered so `loadSettings` runs before the calibration decision and `loadReferenceEnvironment` is gated on `currentSettings.color_calibration_mode === 'from-reference-image' && file.has_reference`. `uploadReferenceImage` writes `currentSettings.reference_image_path = "outputs/{id}_reference{ext}"` + `saveSettings`, and the live-scene mutation is now gated on the mode. |

No new files (other than RDSPI artifacts under
`docs/active/work/T-005-03/`). No deletions. Two commits, both
leaving the tree buildable and tests green.

## Acceptance-criteria mapping

| AC bullet | Status | Notes |
|---|---|---|
| New settings field `color_calibration_mode` with `none` (default) and `from-reference-image` | ✅ | Enum validated; default is `none`; forward-compat normalization for pre-existing on-disk docs. |
| New settings field `reference_image_path` (string, optional) | ✅ | `omitempty`; populated client-side after a successful upload; treated as a tag, not dereferenced server-side. |
| Tuning panel adds a dropdown for `color_calibration_mode` | ✅ | `<select id="tuneColorCalibrationMode">` enrolled in `TUNING_SPEC`. |
| When set to `from-reference-image`, the panel shows the existing Reference Image upload control | ✅ | Conditional `#referenceImageRow` toggles via `syncReferenceImageRow()`; the in-panel button triggers the same hidden `#referenceFileInput` as the toolbar button — no DOM duplication. |
| When set to `none`, calibration is bypassed and bake uses neutral lighting | ✅ | Three gates: (1) `selectFile` will not call `loadReferenceEnvironment` unless mode is `from-reference-image`; (2) dropdown change handler tears down `referencePalette`/`referenceEnvironment` and resets to neutral via `applyColorCalibration`; (3) `uploadReferenceImage` no longer auto-applies calibration when mode is `none`. The bake side reads `referencePalette` directly, so once it is null the bake renderer falls back to neutral colors. |
| Mode persists per-asset via the settings system | ✅ | Same persistence channel as every other tuning field — debounced `PUT /api/settings/:id`. |
| Mode changes emit `setting_changed` analytics events | ✅ | Free via `wireTuningUI`'s auto-instrumentation; no per-control wiring needed. |
| Manual rose verification | ❌ deferred (operator) | Requires a browser session with a live asset. The wiring needed for the operator's clickpath is in place. |

## Test coverage

| Layer | What runs | Status |
|---|---|---|
| Go unit | `TestValidate_RejectsOutOfRange` (existing + 2 new cases) | ✅ green |
| Go unit | `TestSettingsDifferFromDefaults` (existing + 2 new mutations) | ✅ green |
| Go unit | `TestLoadSettings_NormalizesColorCalibrationMode` (new) | ✅ green |
| Go unit | All other settings tests (defaults, round-trip, migration paths, explicit-false ground_align) | ✅ green |
| Go build | `go build ./...` | ✅ clean |
| JS syntax | `node -c static/app.js` | ✅ clean |
| JS unit | None — project still has no JS test runner | ⚠️ untested |
| Manual integration | `curl` settings round-trip | ❌ deferred (operator) |
| Manual UI | dropdown render, in-panel upload row toggle, calibration apply/teardown, dirty dot, reset, end-to-end with rose | ❌ deferred (operator) |

### Coverage gaps

- **No automated test exercises the JS gating logic in `selectFile`,
  the dropdown handler, or `uploadReferenceImage`.** Same constraint
  as every prior tuning ticket; verified by reading. If a JS test
  runner ever lands, the highest-leverage cases are: (a) selecting an
  asset with `has_reference: true` and mode `none` does NOT call
  `loadReferenceEnvironment`; (b) flipping the dropdown to
  `from-reference-image` while a reference is loaded calls
  `applyColorCalibration` and the live `referencePalette` becomes
  truthy; (c) flipping back to `none` clears it.
- **No automated test exercises the round-trip through the settings
  HTTP handler with the new field.** The handler is a thin
  decode/validate/save wrapper, and the unit tests cover all three of
  those steps directly. A focused `httptest.Server` test would be a
  cheap follow-up if the handler grows.

## Open concerns / TODOs

1. **Behavior change for existing assets with a reference image.**
   Before this ticket, any asset with `file.has_reference === true`
   auto-applied calibration on selection. After this ticket,
   calibration only applies when the per-asset settings file has
   `color_calibration_mode === 'from-reference-image'` and the
   default is `'none'`. In practice this is invisible because
   `scanExistingFiles` does not currently restore `HasReference` on
   server restart (a pre-existing gap), so the only way an asset has
   `has_reference: true` in memory is if the user uploaded the image
   *during the current session* — at which point they will see the
   new dropdown and pick a mode. If `scanExistingFiles` ever grows
   the `HasReference` restore, the conservative migration is: when
   detecting `_reference{.png|.jpg}` on disk and no settings file
   exists, write a settings file with mode `from-reference-image`
   and `reference_image_path` set. **Flag for the operator if this
   becomes a real regression.**

2. **Toolbar `#uploadReferenceBtn` is left in place.** Both the
   toolbar button and the in-panel button trigger the same hidden
   file input, so they remain consistent. S-007 will collapse the
   toolbar button into the lighting-preset framework. Removing it in
   this ticket would be churn for no benefit.

3. **Uploading a reference image while mode is `none` is allowed but
   does not visibly do anything.** The image is saved, and
   `reference_image_path` is persisted, so flipping the dropdown
   later picks it up immediately. The only path that reaches this
   case is the toolbar button (the in-panel button is hidden when
   mode is `none`). Documented; not a bug.

4. **`reference_image_path` is a free string, not enum-validated.**
   This is intentional — the field is a tag, not dereferenced
   server-side. The server constructs the on-disk path from `id` +
   `ReferenceExt` independently in `handleReferenceImage`. If a
   future ticket starts using the path field as a dereference target,
   it must add path-traversal validation. (Likely never — this is a
   single-user local tool.)

5. **`SettingsDifferFromDefaults` enumerates fields explicitly.**
   The new field comparisons were appended in the same chain. The
   `single_field_mutated` test gained a representative case for each
   new field; a future rename that misses this helper would at
   least fail the test if the renamed field happened to be covered.
   Same posture as T-005-02 — see that review for the broader
   discussion.

6. **`SchemaVersion` is not bumped.** Both new fields are additive
   with sensible defaults; the loader's normalization handles the
   missing-key case for the enum. Per the schema doc's "Adding fields
   without a version bump" guidance, this is the right call.

## Files for the reviewer to read first

1. `settings.go` — the two new fields, the enum table, the
   normalization line in `LoadSettings`, and the
   `SettingsDifferFromDefaults` extension. The deliberate choice to
   accept any string for `reference_image_path` is the only subtle
   bit.
2. `static/app.js` — `applyColorCalibration` and the three gate
   sites (`selectFile`, the dropdown change branch in `wireTuningUI`,
   and `uploadReferenceImage`). This is the only place where a
   future bug could go unnoticed (the project has no JS test runner).
3. `static/index.html` — confirm the two new rows live inside
   `#tuningSection` and use the reserved DOM ids.
4. `docs/knowledge/settings-schema.md` — confirm the field table and
   JSON example match the struct.
5. `settings_test.go` — confirm the legacy-load test mirrors the
   exact pre-T-005-03 doc shape.

## Out-of-scope items NOT touched (per ticket)

- Lighting presets enum (S-007) — `lighting_preset` and
  `validLightingPresets` are untouched.
- Improvements to `extractPalette` / `buildSyntheticEnvironment` /
  `applyReferenceTint` / `loadReferenceEnvironment` — calibration
  internals unchanged.
- New calibration sources (HDR images, color picker, etc.).
- Restoring `HasReference` in `scanExistingFiles` on startup
  (pre-existing gap).
- The toolbar `#uploadReferenceBtn` — left in place pending S-007.
