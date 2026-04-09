# Research — T-007-01: lighting-preset-schema-and-set

## Ticket scope

Consolidate the granular lighting controls shipped in S-005 into named
presets ("midday-sun", "overcast", etc.). Each preset is a bundle that
rewrites the dependent AssetSettings fields to a known good starting
point. The user can still tune from there. The ticket also defines the
schema for `bake_config` / `preview_config` that T-007-02 will use to
make bake and live preview consistent.

Out of scope: bake/preview consistency wiring (T-007-02), reference image
as a preset (T-007-03), HDR envs, custom user presets, per-light editing.

## What exists today

### Settings layer (backend, settings.go)

- `AssetSettings` (`settings.go:22`) — 15 fields plus `SchemaVersion=1`.
  Lighting-relevant fields:
  - `BakeExposure` (`bake_exposure`, range [0,4], default 1.0) — already
    drives `offRenderer.toneMappingExposure` in app.js. This is the
    `tone_exposure` slot from the ticket; no new field needed.
  - `AmbientIntensity` (`ambient_intensity`, [0,4], 0.5)
  - `HemisphereIntensity` (`hemisphere_intensity`, [0,4], 1.0)
  - `KeyLightIntensity` (`key_light_intensity`, [0,8], 1.4)
  - `BottomFillIntensity` (`bottom_fill_intensity`, [0,4], 0.4)
  - `EnvMapIntensity` (`env_map_intensity`, [0,4], 1.2)
  - `LightingPreset` (`lighting_preset`, string enum, default `"default"`)
- `validLightingPresets` (`settings.go:70`) currently contains only
  `"default"`. The doc-comment explicitly says "S-007 will extend this".
- `Validate()` (`settings.go:92`) checks `LightingPreset` against the
  enum map and returns "is not a known preset" on miss.
- `SettingsDifferFromDefaults()` (`settings.go:155`) compares every
  field by hand — adding new fields requires visiting it. Existing
  intensity fields are already enumerated.

### Settings layer (frontend, static/app.js)

- `makeDefaults()` (`app.js:113`) — hand-mirrored copy of
  `DefaultSettings()`. Holds the same intensity defaults; the comment
  warns "keep in sync by hand".
- `applyDefaults()` (`app.js:134`) — wholesale `currentSettings = makeDefaults()`.
  Used by the reset button.
- `TUNING_SPEC` (`app.js:264`) — array of `{field, id, parse, fmt}` rows
  driving both `populateTuningUI` and `wireTuningUI`. The
  `lighting_preset` row at line 275 already exists with id
  `tuneLightingPreset`. Auto-instrumentation fires `setting_changed`
  on every change without per-field code.
- `populateTuningUI()` (`app.js:288`) reads `currentSettings[field]`,
  pushes to control. Handles checkbox vs other inputs.
- `wireTuningUI()` (`app.js:307`) installs `input` listeners that mutate
  `currentSettings[spec.field]`, debounce-save, and log analytics. Has
  a special branch for `color_calibration_mode` (`app.js:342`) that
  triggers extra side effects (`syncReferenceImageRow`,
  `applyColorCalibration`). Pattern: any preset-style field that
  cascades into other fields hooks here.

### Tuning panel HTML (static/index.html)

- `tuneLightingPreset` lives at `index.html:296`, currently a `<select>`
  with a single hardcoded `<option value="default">default</option>`.
  Needs to be populated from the preset registry.

### Bake-time light setup

- `setupBakeLights()` (`app.js:920`) — reads
  `referencePalette` (the calibrated palette from a reference image,
  if loaded) and uses bright/mid/dark colors for sky, fill, ground.
  Falls back to `0xffffff` / `0x444444` when no palette. Reads
  `currentSettings.ambient_intensity`, `hemisphere_intensity`,
  `key_light_intensity`, `bottom_fill_intensity` for the four light
  intensities.
- `createBakeEnvironment()` (`app.js:948`) — builds a PMREM env. With a
  reference palette: a vertical bright→mid→dark gradient canvas →
  PMREM. Without: `RoomEnvironment()` (neutral).
- `cloneModelForBake()` (`app.js:986`) sets
  `material.envMapIntensity = currentSettings.env_map_intensity`.
- `renderLayerTopDown()` (`app.js:1147`) — the volumetric path. Uses
  the same palette/white fallback pattern locally (duplicated, not
  shared) at lines 1181–1196.
- The bake renderer's `toneMappingExposure` is set from
  `currentSettings.bake_exposure` at lines 1014 and 1158. This is the
  tone-exposure hook the preset's `tone_exposure` should map onto.

### Live preview light setup

- The live `scene` constructed during init has fixed ambient +
  hemisphere lights wired once at startup (not searched in detail).
- `applyReferenceTint()` (`app.js:2027`) walks the live scene and
  rewrites the ambient + hemisphere colors when a reference palette
  is loaded. This is the only place live-scene lights are mutated
  after init. Per the ticket, T-007-02 owns wiring `preview_config`
  into the live scene; this ticket just defines the schema field.

### Reference palette interaction

- When a reference image is loaded, `referencePalette` becomes truthy
  and overrides `bake_config` colors at bake time. Presets do not
  conflict — they share the intensity sliders but the palette wins for
  colors. T-007-03 will eventually let a preset declare a default
  reference image; out of scope here.

### Static asset directory

- `static/` is flat: `app.js`, `index.html`, `style.css`. There is no
  `static/presets/` subdirectory yet. The ticket explicitly names
  `static/presets/lighting.js` as the schema location.
- `app.js` is a single ~2400-line module loaded as a regular `<script>`
  in `index.html` (no ES module imports today). Verified by grepping
  for `import ` — only THREE imports inside. The new preset module
  must therefore be either: (a) a plain `<script>` that defines a
  global, or (b) the project converts to modules. (a) is the
  smaller-blast-radius option.

### Schema documentation

- `docs/knowledge/settings-schema.md` is the source of truth table for
  the on-disk fields and migration policy. Adding presets to the enum
  needs a row update there; no new field is added on disk.

## Constraints and assumptions

1. **Schema version stays at 1.** No new persisted field — the existing
   `lighting_preset` string is repurposed from a single-value enum to
   a 6-value enum. Old files with `lighting_preset: "default"` are
   already valid.
2. **Validate() must reject unknown presets** so `validLightingPresets`
   needs all 6 ids. There is an existing test
   (`settings_test.go:74`) that asserts `"studio"` is rejected — that
   test still passes after adding the new ids.
3. **Frontend default mirror.** `makeDefaults()` in app.js still
   defaults `lighting_preset: 'default'`; when picking the `default`
   preset its bake_config must match the current hardcoded defaults
   exactly so applying it is a no-op for unmodified assets.
4. **No build step.** Anything new under `static/` is served as-is by
   the Go file server; no bundler, no TypeScript. The preset module
   must be plain ES5/ES2015 JS that runs in the browser and is
   reachable from `app.js` without `import`.
5. **Visible bake change required.** AC says "regenerate the bake, see
   warmer tones" for `golden-hour`. Just rewriting intensities won't
   change tones — the bake light setup uses pure white when no
   reference palette. The preset's color values must reach
   `setupBakeLights()` / `createBakeEnvironment()` for the warmer-tones
   verification to pass. Cleanest seam: a `getActiveBakePalette()`
   helper that returns the reference palette when present, else the
   active preset's bake_config colors, else neutral white.
6. **Preview wiring is T-007-02.** This ticket includes
   `preview_config` in the schema and exports it via `getPreset(id)`,
   but does NOT touch the live `scene` lights. Document this in the
   preset module so a future reader knows the field is intentionally
   unused here.
7. **Analytics already covered.** `setting_changed` auto-fires for
   `lighting_preset` via the existing TUNING_SPEC walker. Picking a
   preset that cascades into ambient/hemisphere/key/etc. should also
   fire those secondary `setting_changed` events so the analytics
   pipeline sees the cascade — that means routing the cascade through
   the same path the user input listener uses, not by direct
   assignment.

## Open questions for Design

- One file vs. one-file-per-preset under `static/presets/`?
- Should `getPreset` live in JS only, or also in Go (for backend
  validation beyond enum membership)?
- How exactly to apply the preset cascade so secondary
  `setting_changed` events fire correctly?
- For `default`, do `bake_config` numbers come from `makeDefaults()` at
  runtime (DRY) or are they re-stated as literals (clarity)?
