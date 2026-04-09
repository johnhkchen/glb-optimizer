# Plan — T-007-01: lighting-preset-schema-and-set

## Step 1 — Create the preset registry module

**File**: `static/presets/lighting.js` (NEW)

**What**: Add `LIGHTING_PRESETS`, `getLightingPreset`,
`listLightingPresets` on `window`. Six preset literals in declaration
order: `default`, `midday-sun`, `overcast`, `golden-hour`, `dusk`,
`indoor`. Use the `makePreset` helper to seed `preview_config` from a
deep clone of `bake_config`. Add the `default`-matches-app-defaults
dev assertion (console.warn on drift).

**Verify**: Open the page in a browser dev console after step 2:
- `window.LIGHTING_PRESETS` exists, has 6 keys
- `window.getLightingPreset('golden-hour').bake_config.tone_exposure === 1.10`
- `window.getLightingPreset('does-not-exist') === undefined`
- No console.warn from the default-drift assertion

**Commit**: `Add lighting preset registry module (T-007-01)`

## Step 2 — Wire the script tag and clear the hardcoded option

**Files**: `static/index.html`

**What**:
1. Insert `<script src="/static/presets/lighting.js"></script>` BEFORE
   the `<script src="/static/app.js"></script>` line.
2. Inside `#tuneLightingPreset` (line 296), remove the inline
   `<option>` so the select starts empty.

**Verify**: Page loads without console errors. The lighting-preset
select is empty (will be populated in step 3). The other tuning
controls still work.

**Commit**: `Load lighting preset registry in index.html (T-007-01)`

## Step 3 — Populate dropdown and apply cascade

**Files**: `static/app.js`

**What**:
1. Add `populateLightingPresetSelect()` after `makeDefaults()`. Walks
   `window.listLightingPresets()` and appends `<option>` rows. Idempotent.
2. Call it once from the same init block that calls `wireTuningUI()`.
3. Add `applyLightingPreset(id)`:
   - Look up via `getLightingPreset(id) || getLightingPreset('default')`.
   - Build a `changed` map by diffing the preset's mapped intensities
     against `currentSettings`.
   - Mutate `currentSettings` for each mapped field (ambient_intensity,
     hemisphere_intensity, key_light_intensity, bottom_fill_intensity,
     env_map_intensity, bake_exposure).
   - Set `currentSettings.lighting_preset = id`.
   - Call `populateTuningUI()` to refresh sliders + dirty dot.
   - Call `saveSettings(selectedFileId)`.
   - Emit `logEvent('preset_applied', {from, to, changed}, selectedFileId)`.
4. In `wireTuningUI()`, add a `lighting_preset` branch in the input
   listener that calls `applyLightingPreset(v)` and SHORT-CIRCUITS the
   default per-field analytics path (so we don't double-fire). Place
   the new branch alongside the existing `color_calibration_mode`
   branch.

**Verify**:
- Pick `golden-hour` from the dropdown. The 6 intensity sliders + tone
  exposure update to the preset values immediately.
- Reload — the picked preset persists (settings file has
  `"lighting_preset": "golden-hour"` and the matching numbers).
- Click reset → preset goes back to `default`, sliders restore.
- Pick `default` after picking `golden-hour` → all sliders go back to
  `makeDefaults()` values (no drift).
- Network tab: one `preset_applied` analytics event per pick, NOT six
  `setting_changed` events.

**Commit**: `Wire lighting preset selection cascade (T-007-01)`

## Step 4 — Apply preset colors to the bake

**Files**: `static/app.js`

**What**:
1. Add `getActiveBakePalette()` near `setupBakeLights()`:
   ```js
   function getActiveBakePalette() {
     if (referencePalette) return referencePalette;
     const preset = getLightingPreset(currentSettings && currentSettings.lighting_preset)
                 || getLightingPreset('default');
     const bc = preset.bake_config;
     const t = ([r,g,b]) => ({r, g, b});
     return {
       bright: t(bc.hemisphere_sky),
       mid:    t(bc.key_color),
       dark:   t(bc.hemisphere_ground),
     };
   }
   ```
2. Replace the `referencePalette ? ... : ...` ternaries in
   `setupBakeLights()` (lines ~921–929) with three lines pulling from
   `getActiveBakePalette()`.
3. Same swap inside `renderLayerTopDown()` (lines ~1181–1189).
4. In `createBakeEnvironment()`:
   - Extract a small `buildGradientEnvTexture(stops, pmrem)` helper from
     the existing reference-palette branch (lines ~955–971). `stops` is
     an array of three RGB triples top→bottom.
   - When `referencePalette` is set: call helper with the palette.
   - Else when active preset is non-default and has an
     `env_gradient`: call helper with the preset stops.
   - Else: keep the existing `RoomEnvironment()` neutral fallback for
     `default`.

**Verify**:
- Pick `golden-hour`, click Regenerate (billboards). Open the
  generated GLB output — visibly warmer/orange tint vs `default`.
- Pick `dusk`, regenerate → cool blue tint.
- Pick `default`, regenerate → output is unchanged from before this
  ticket (RoomEnvironment + neutral lights). This is the regression
  guardrail.
- With a reference image loaded, the palette still wins regardless of
  the picked preset (existing behavior preserved).

**Commit**: `Apply lighting preset colors to bake pipeline (T-007-01)`

## Step 5 — Extend backend enum + tests

**Files**: `settings.go`, `settings_test.go`

**What**:
1. Update `validLightingPresets` to the 6-id map. Drop the
   "S-007 will extend this" doc comment.
2. Add `TestValidate_AcceptsAllPresets`:
   ```go
   func TestValidate_AcceptsAllPresets(t *testing.T) {
     for _, id := range []string{
       "default", "midday-sun", "overcast",
       "golden-hour", "dusk", "indoor",
     } {
       s := DefaultSettings()
       s.LightingPreset = id
       if err := s.Validate(); err != nil {
         t.Errorf("preset %q rejected: %v", id, err)
       }
     }
   }
   ```
3. Run `go test ./...` — both new and existing tests pass. The
   `"studio"` rejection test still passes (still not a known preset).

**Commit**: `Extend validLightingPresets to 6 named presets (T-007-01)`

## Step 6 — Schema documentation

**Files**: `docs/knowledge/settings-schema.md`

**What**: Update the `lighting_preset` row to enumerate the 6 valid
values. No migration row — schema_version unchanged, default
`"default"` is still valid.

**Verify**: Doc renders, lists six values.

**Commit**: `Document lighting preset enum values (T-007-01)`

## Testing strategy

- **Unit (Go)**: `TestValidate_AcceptsAllPresets` is the only new
  test. The existing `TestValidate_RejectsOutOfRange` and the
  default-validity test still cover the negative space.
- **Unit (JS)**: there is no JS test harness in this repo. The
  module-level dev assertion in `static/presets/lighting.js` provides
  the equivalent of a smoke test by failing loudly in the browser
  console if `default` drifts from `makeDefaults()`.
- **Manual**: each step's Verify section is the integration test.
  The decisive manual checks live in step 3 (cascade) and step 4
  (visible warmer tones for `golden-hour`).
- **Regression**: the `default` preset must produce identical bake
  output to before this ticket. Step 4's verify section calls this
  out explicitly.

## Ordering and atomicity

Steps 1, 2, 3, 4, 5, 6 each commit atomically.

- Step 1 is harmless on its own (file unused).
- Step 2 references the file from step 1; do not reorder.
- Step 3 makes the dropdown functional but does not yet change bake
  output. Page is still fully functional after step 3 alone.
- Step 4 turns on the visible bake difference. Independent of step 5.
- Step 5 (Go enum) can land before or after step 4; doing it last
  keeps the backend strict during dev (only `default` accepted) until
  the frontend is ready, but in practice they're decoupled enough
  that the order doesn't matter.
- Step 6 is doc-only.

## Risks and mitigations

- **Risk**: `populateTuningUI()` triggers `input` listeners
  recursively when applying the cascade.
  **Mitigation**: `populateTuningUI()` writes `el.value =` directly,
  which does NOT fire `input` events in any browser. Verified by
  reading lines 288–305 of app.js — only listener installation lives
  in `wireTuningUI`. Safe.
- **Risk**: A user with a settings file written before this ticket
  has `lighting_preset: "default"` but their other intensities have
  drifted. Picking `default` would clobber their tuning.
  **Mitigation**: This is the documented contract — picking a preset
  overwrites dependent settings. The user can undo via the reset
  button or by re-tuning. The `default` preset's intensities equal
  `makeDefaults()`, so the worst case is "settings reset to defaults".
- **Risk**: `getLightingPreset` returns undefined if the registry
  failed to load (network error, 404).
  **Mitigation**: All call sites use `getLightingPreset(id) ||
  getLightingPreset('default')`. The bake palette fallback path
  ultimately returns neutral white — the page never crashes.
