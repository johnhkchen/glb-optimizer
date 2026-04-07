# Review — T-007-01: lighting-preset-schema-and-set

## What changed

### Files created (1)

- `static/presets/lighting.js` — preset registry. Defines 6 presets,
  `getLightingPreset(id)`, `listLightingPresets()`,
  `PRESET_FIELD_MAP`, and a dev-time default-drift assertion. ES
  module.

### Files modified (5)

- `static/app.js`:
  - Imports `LIGHTING_PRESETS`, `getLightingPreset`,
    `listLightingPresets`, `PRESET_FIELD_MAP` from the new module.
  - Adds `populateLightingPresetSelect()` and `applyLightingPreset()`
    after `applyDefaults()`.
  - Adds `getActiveBakePalette()` and `buildGradientEnvTexture()`
    near `setupBakeLights()`.
  - `setupBakeLights()` and the bake-light block inside
    `renderLayerTopDown()` now read from `getActiveBakePalette()`
    instead of branching on `referencePalette`.
  - `createBakeEnvironment()` now has three branches: reference
    palette → preset env_gradient (when non-default) → RoomEnvironment.
  - `wireTuningUI()` short-circuits the `lighting_preset` row to
    `applyLightingPreset(v)` instead of the per-field setter path.
  - Init block calls `populateLightingPresetSelect()` before
    `wireTuningUI()`.
- `static/index.html` — `#tuneLightingPreset` no longer carries a
  hardcoded `<option>`; it is populated at init.
- `settings.go` — `validLightingPresets` grown from `{default}` to
  the 6-id set; doc comment refreshed.
- `settings_test.go` — new `TestValidate_AcceptsAllPresets`
  table-driven test.
- `main.go` — `//go:embed static/*` → `//go:embed static` so the
  `static/presets/` subdirectory is embedded into the binary.
- `docs/knowledge/settings-schema.md` — `lighting_preset` row
  updated to enumerate the 6 valid values and describe the cascade.

### Files deleted

None.

## Commit history

```
be19570 Document lighting preset enum values (T-007-01)
16554c3 Extend validLightingPresets to 6 named presets (T-007-01)
5a6da3d Apply lighting preset colors to bake pipeline (T-007-01)
9be308c Wire lighting preset selection cascade (T-007-01)
0505293 Clear hardcoded lighting preset option (T-007-01)
a6890e5 Add lighting preset registry module (T-007-01)
```

Six atomic commits, each independently reverted-able. The order
follows plan.md exactly.

## Acceptance criteria check

- [x] `static/presets/lighting.js` exists with the documented schema.
- [x] Each preset has `id`, `name`, `description`, `bake_config`,
      `preview_config`.
- [x] `bake_config` exposes ambient, hemisphere_sky,
      hemisphere_ground, key_intensity, fill_intensity, env_gradient,
      tone_exposure (plus the additional intensity fields the bake
      already needs).
- [x] All 6 presets shipped: `default`, `midday-sun`, `overcast`,
      `golden-hour`, `dusk`, `indoor`.
- [x] `lighting_preset` is the discriminator; selecting a preset
      rewrites the dependent settings via `applyLightingPreset()`.
- [x] `getLightingPreset(id)` exported from the new module.
- [ ] **Manual verification not yet performed** — see "Open
      concerns" below. The pipeline should produce visibly warmer
      tones for `golden-hour`; this requires loading an asset and
      regenerating, which I can't do from the agent harness.

## Test coverage

- **Go**: `TestValidate_AcceptsAllPresets` covers all 6 ids
  positively. The pre-existing `TestValidate_RejectsOutOfRange`
  case for `"studio"` still covers the negative space. Roundtrip
  + migration tests are unchanged and still pass.
- **JS**: no JS test harness in this repo. The module-level dev
  assertion in `static/presets/lighting.js` verifies that the
  `default` preset's mapped intensities equal the constants the
  rest of app.js uses, logging a `console.warn` on drift. This is
  a smoke test, not a unit test — caveat: it duplicates the
  default constants instead of importing them (avoiding a cycle
  with app.js, which imports from this module). If `DefaultSettings`
  in Go and `makeDefaults` in app.js drift, only the JS-side
  drift is caught here.
- **Manual checklist** (for the human reviewer):
  1. Load any asset.
  2. Pick `golden-hour` from the preset dropdown — the six
     intensity sliders + bake exposure should snap to the preset
     values immediately, the dirty dot should appear, and the
     network tab should show one `preset_applied` event (NOT six
     `setting_changed` events).
  3. Click "Volumetric" to regenerate. The output should be
     visibly warmer/orange-tinted vs the `default` baseline.
  4. Pick `dusk`, regenerate — output should shift cool/blue.
  5. Pick `default`, regenerate — output should match the
     pre-ticket baseline (RoomEnvironment + neutral lights).
  6. Reload the page — picked preset should persist (settings file
     contains `"lighting_preset": "..."` and matching numbers).

## Open concerns

1. **Manual visual verification not performed.** The agent has no
   way to run a browser regenerate cycle; the "warmer tones" check
   in the AC is on the human reviewer. If the colors look wrong,
   tweaking is local to `static/presets/lighting.js` — no schema
   surface is affected.
2. **Default-drift assertion duplicates constants.** The
   module-level check in `lighting.js` hardcodes the expected
   `makeDefaults()` numbers instead of importing them, to avoid an
   import cycle with app.js. If a future ticket tunes
   `makeDefaults()` (and `DefaultSettings()` in Go) without
   updating both the assertion AND the `default` preset, the
   warning will fire — but you have to read the dev console to
   notice. Acceptable for now; a follow-up could move the canonical
   defaults into a third tiny module imported by both sides.
3. **Live preview lights are unchanged.** Per design, this ticket
   only wires presets into the bake pipeline. The live three.js
   `scene` lights are still set up in init code and not refreshed
   on preset change. T-007-02 owns this. Users will see the bake
   reflect the preset but the live preview still looks neutral
   until they regenerate.
4. **`preview_config` is currently a deep clone of `bake_config`
   per preset.** That's intentional (T-007-02 will start to
   diverge them), but anyone reading the registry should know the
   field is structurally present and unused for now.
5. **Reference image still wins over preset.** This is intended:
   explicit user calibration > preset starting point. T-007-03
   will let a preset *carry* a default reference image.
6. **Cascade emits `preset_applied`, not N `setting_changed`.**
   This is a design decision (D4) but T-003-04's fast-revert
   detector currently keys off `setting_changed`. If T-003-04
   wants to count preset picks as edits, it needs to subscribe to
   `preset_applied` too. Worth a heads-up to whoever owns the
   analytics dashboards.
7. **Profile + preset interaction unchanged.** A profile that
   carries `lighting_preset: "golden-hour"` along with the
   matching intensities will load and display correctly (the JSON
   PUT path doesn't trigger the cascade, but the values are
   already consistent in the profile body). A profile that
   carries `lighting_preset: "golden-hour"` with mismatched
   intensities will display the mismatched values — the cascade
   only fires on user dropdown selection. Documenting here so the
   future profile UX has eyes on it; not a bug today.

## Out of scope (for clarity)

- Live preview color application — T-007-02.
- Reference image as a preset attribute — T-007-03.
- Custom user-authored presets.
- HDR environment maps.
- Per-light editing inside a preset.

## Build / test status

- `go build ./...` — clean.
- `go test ./...` — passes (`ok glb-optimizer 0.377s`).
- No JS test harness exists; smoke check is the dev console
  assertion.
