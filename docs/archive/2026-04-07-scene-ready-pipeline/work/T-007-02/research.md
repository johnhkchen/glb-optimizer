# Research — T-007-02: bake-preview-lighting-consistency

## Ticket recap

The bake (offscreen render) and the live three.js preview have
separate light setups. T-007-01 wired lighting presets into the bake
pipeline only (`setupBakeLights`, `renderLayerTopDown`,
`createBakeEnvironment`); the live preview's `initThreeJS()` still
hardcodes neutral white lights. This ticket collapses both surfaces
onto the same preset by also driving the main scene's lights from the
preset, refreshing them on preset change, and adding a "needs rebake"
hint so users know the on-disk baked textures still reflect the prior
preset.

## Files of interest

- `static/app.js` (3265 lines) — single-file frontend.
  - `initThreeJS()` @ 2225 — creates `scene`, sets up renderer,
    `defaultEnvironment` (RoomEnvironment PMREM), then hardcodes 1
    AmbientLight + 3 DirectionalLights + 1 HemisphereLight. None of
    these read from `currentSettings` or the active preset.
  - `applyLightingPreset(id)` @ 168 — T-007-01 cascade. Rewrites the
    six dependent intensity fields, calls `populateTuningUI()`, saves
    settings, emits `preset_applied`. Currently does NOT touch the
    live scene lights or post a "needs rebake" hint.
  - `getActiveBakePalette()` @ 980 — resolves
    referencePalette → preset bake_config → neutral fallback.
    Returns `{bright, mid, dark}` of `{r,g,b}` 0..1.
  - `setupBakeLights(offScene)` @ 1009 — adds Ambient + Hemisphere +
    top DirectionalLight (key) + bottom DirectionalLight (fill),
    intensities from `currentSettings`, colors from
    `getActiveBakePalette()`.
  - `renderLayerTopDown` @ 1283 — open-coded copy of the same setup
    for layer renders. Already palette-aware.
  - `renderBillboardTopDown` @ 1112 — calls `setupBakeLights`.
  - `createBakeEnvironment(renderer)` @ 1066 — referencePalette gradient
    → preset env_gradient (skipped for `default`) → RoomEnvironment.
  - `applyReferenceTint(palette)` @ 2130 — mutates the live scene's
    Ambient + Hemisphere lights when a reference image loads. Three
    DirectionalLights are NOT touched.
  - `resetSceneLights()` @ 2996 — resets Ambient + Hemisphere only
    back to white@0.4 / white+0x303040@0.5. Same blind spot as
    applyReferenceTint.
  - `selectFile(id)` @ ~2918 — calls `resetSceneLights()` after
    clearing the reference environment, before `loadSettings`.
  - `loadSettings` / `populateTuningUI` — no live-light call inside.
  - `currentSettings` — owns `lighting_preset` field plus the six
    intensity sliders the cascade rewrites.

- `static/presets/lighting.js` — preset registry. Each preset has
  `bake_config` and `preview_config`. **`preview_config` is currently
  a deep clone of `bake_config`** (T-007-01 punted divergence to this
  ticket). Both expose: `ambient`, `hemisphere_intensity`,
  `hemisphere_sky`, `hemisphere_ground`, `key_intensity`, `key_color`,
  `fill_intensity`, `fill_color`, `env_gradient`, `env_intensity`,
  `tone_exposure`. `PRESET_FIELD_MAP` maps preset keys to
  AssetSettings field names (uses `bake_config` as the source).

- `static/index.html` — `#tuneLightingPreset` `<select>` is populated
  at init by `populateLightingPresetSelect()`. No "needs rebake"
  element exists yet; we will add one near the regenerate button or
  inside the tuning panel.

- `settings.go` — `validLightingPresets` already accepts all 6 preset
  ids. The schema does not need touching for this ticket.

## Live-preview light topology (current)

`initThreeJS()` adds:
1. `AmbientLight(0xffffff, 0.4)`
2. `DirectionalLight(0xffffff, 1.5)` at `(5,10,7)` — "key"
3. `DirectionalLight(0xffffff, 0.8)` at `(-5,5,-5)` — back/rim
4. `DirectionalLight(0xffffff, 0.5)` at `(0,-3,5)` — fill from below
5. `HemisphereLight(0xffffff, 0x303040, 0.5)` — sky/ground

This is deliberately busier than the bake (which is rotationally
symmetric for billboards). The user navigates a perspective camera in
the live preview, so a single top-down key would look flat.

The bake topology is, by contrast:
1. `AmbientLight(sky, ambient_intensity)`
2. `HemisphereLight(sky, ground, hemisphere_intensity)`
3. `DirectionalLight(sky, key_light_intensity)` at `(0,10,0)` — pure
   top-down
4. `DirectionalLight(fill, bottom_fill_intensity)` at `(0,-10,0)`

The visual goal of "recognizably consistent" is therefore *color +
overall brightness*, not light placement.

## Constraints and assumptions

1. **Don't break reference-image calibration.** `applyReferenceTint`
   and `loadReferenceEnvironment` must still win over presets when a
   user has uploaded a calibration image. Current path:
   `referencePalette` is the highest-priority signal in
   `getActiveBakePalette`. The same priority must hold in the live
   preview.

2. **Don't reload the model on preset change.** Currently
   `applyLightingPreset` only updates `currentSettings` and the UI; no
   model reload. We must keep that — refreshing the live lights is
   in-place mutation only.

3. **`preview_config` exists but is unused.** The schema is in place;
   we can wire it without a registry refactor. Default preset already
   has neutral colors; non-default presets need slight tuning so the
   live preview's heavier light count doesn't look blown out.

4. **The "needs rebake" hint is purely informational** (per AC: "a
   small UI hint, since the existing baked textures still reflect the
   old preset"). No on-disk state, no auto-rebake. The hint should
   appear when a preset has been changed since the last successful
   regenerate, and clear after a regenerate completes.

5. **Three light types in the live scene need updating.**
   `applyReferenceTint` only touches Ambient + Hemisphere — the three
   DirectionalLights stay white. For full preset consistency we need
   to also drive the directional lights' colors and intensities from
   the preset (or at least the dominant key).

6. **Backend is out of scope.** No Go changes are required by the AC.
   Settings already persist `lighting_preset`; the rebake hint is
   client-side only.

## Open questions for design

- How aggressively should `preview_config` diverge from
  `bake_config`? The bake feeds two top-down passes (billboard +
  layer); the live preview has a richer 5-light setup. We can either
  (a) reuse bake_config values verbatim and accept brighter live
  output, or (b) attenuate intensities for the preview path.

- Where should the "needs rebake" indicator live? Candidates:
  next to the regenerate button (most discoverable), inside the
  tuning panel header (least intrusive), or as a small dot on the
  preset dropdown.

- How are the three preview directional lights mapped onto the
  preset's two colors (`key_color` and `fill_color`)? Simplest: dirLight
  (main key) ← key_color/key_intensity; dirLight2 (back) ←
  hemisphere_sky as a soft rim; dirLight3 (under-fill) ← fill_color.

- Does the live preview need its own env_map_intensity application?
  Materials in the live preview already auto-bind
  `scene.environment` (`defaultEnvironment` / `referenceEnvironment`),
  but `envMapIntensity` is only set during bake (in
  `cloneModelForBake`). For visual consistency between bake and
  preview we may want to apply the preset's `env_intensity` to live
  materials too. Likely out of scope for this ticket — flag for
  follow-up.
