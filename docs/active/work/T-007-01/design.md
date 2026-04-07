# Design — T-007-01: lighting-preset-schema-and-set

## Goal recap

A preset is a named, opinionated bundle of light intensities, light
colors, env-gradient stops, and tone exposure. Picking it overwrites
the dependent AssetSettings fields. The user can still tune from
there. The preset definitions need to be reachable from the bake path
(so colors actually change the rendered output) and from the tuning
panel (so picking a preset updates the sliders).

## Decisions

### D1. Preset registry lives in `static/presets/lighting.js`

- Plain global script, loaded via `<script src="/static/presets/lighting.js">`
  in `index.html` BEFORE `app.js`. No build step. Defines a single
  global `window.LIGHTING_PRESETS` (object keyed by id) and
  `window.getLightingPreset(id)`.
- Why a global vs ES module: `app.js` is a non-module global script
  today (no `import`/`export`). Switching to modules is out of scope —
  it would touch every browser load and is a separate refactor.
- Why a separate file vs inline in app.js: data is independently
  reviewable, the ticket explicitly names this path, and T-007-02 /
  T-007-03 will both add fields here.

**Rejected**: a Go-side preset registry exposed via JSON endpoint. The
backend already validates the enum string; it doesn't need the bundle
contents. Adding an endpoint adds wire latency at preset selection
time for no benefit. The preset numbers are not user-editable and
don't need to be hot-swapped without a deploy.

### D2. Schema shape (per preset)

```js
{
  id: 'midday-sun',
  name: 'Midday Sun',
  description: 'Bright neutral white sun, strong key light',
  bake_config: {
    ambient: 0.45,
    hemisphere_sky:    [0.95, 0.97, 1.00],   // RGB 0..1 tints
    hemisphere_ground: [0.30, 0.27, 0.22],
    hemisphere_intensity: 1.10,
    key_intensity:  1.80,
    key_color:      [1.00, 0.99, 0.95],
    fill_intensity: 0.35,
    fill_color:     [0.85, 0.88, 1.00],
    env_gradient:   [[1,1,1],[0.92,0.94,1],[0.55,0.55,0.6]],
    env_intensity:  1.20,
    tone_exposure:  1.00,
  },
  preview_config: { /* same shape */ }
}
```

- `bake_config` and `preview_config` have IDENTICAL shape so T-007-02
  can wire one or both into the live scene without further schema
  changes. For this ticket the two are seeded equal for every preset
  (single source of truth via a small helper inside the registry
  module).
- The numeric/array shape is JSON-serializable but kept as JS objects
  (not JSON files) so future presets can include comments and small
  helpers.
- Colors are RGB triples in linear-friendly 0..1 space — that matches
  what `THREE.Color({r,g,b})` already takes throughout app.js.
- `env_gradient` is an array of three RGB triples (top, mid, bottom),
  matching the existing `buildSyntheticEnvironment()` shape.

**Rejected**: hex string colors. They would need parsing on every
read; the rest of app.js works in `{r,g,b}` already. Sticking with
the in-house convention.

**Rejected**: an extra `key_position` light direction per preset. The
bake intentionally uses a top-down key for rotational symmetry; per
the ticket's First-Pass Scope, "don't agonize". Ship without it.

### D3. Settings field mapping (which AssetSettings fields the preset rewrites)

The preset's `bake_config` maps onto these existing AssetSettings
fields when applied:

| bake_config         | AssetSettings              |
|---------------------|----------------------------|
| ambient             | ambient_intensity          |
| hemisphere_intensity| hemisphere_intensity       |
| key_intensity       | key_light_intensity        |
| fill_intensity      | bottom_fill_intensity      |
| env_intensity       | env_map_intensity          |
| tone_exposure       | bake_exposure              |

The color/gradient fields (`hemisphere_sky`, `key_color`, `fill_color`,
`env_gradient`) do NOT have AssetSettings counterparts — they live only
in the preset registry and are pulled into the bake at render time
(see D5). This intentionally keeps the on-disk schema unchanged.

### D4. Cascade application — through the same hook the slider listeners use

When the user picks a preset in `tuneLightingPreset`, app.js needs to:

1. Update `currentSettings.lighting_preset = newId`.
2. For each mapped AssetSettings field, set `currentSettings[field]`
   to the preset's value AND update the corresponding control's value
   AND fire the same `setting_changed` analytics event the slider
   listener would have fired.
3. Debounce-save once.

Implementation: a single `applyLightingPreset(id)` helper called from
the existing `lighting_preset` branch in `wireTuningUI()`. Reuses
`populateTuningUI()` to refresh the controls (it already walks
TUNING_SPEC and writes both `.value` and `Value` text spans), and
emits one `preset_applied` analytics event capturing `{from, to}`
plus the cascade — instead of N separate `setting_changed` events
for the secondary fields. Reasoning: a preset cascade is a single
user intent, not N tunings; T-003-04 fast-revert detection should not
treat it as 6 rapid edits. Document this choice in the analytics
schema doc as a follow-up.

**Rejected**: fire a `setting_changed` per cascaded field. That bloats
the analytics stream and miscounts edit velocity. A single
`preset_applied` event with the diff is more honest.

### D5. Bake-time color application

- Add `getActiveBakePalette()` in app.js that returns:
  1. `referencePalette` if loaded (highest priority — explicit user
     calibration always wins);
  2. else the active preset's `bake_config` colors converted to the
     `{bright, mid, dark}` shape `setupBakeLights` already uses
     (`bright = hemisphere_sky`, `mid = key_color` or `fill_color`,
     `dark = hemisphere_ground`);
  3. else neutral white/dark (the current default fallback).
- `setupBakeLights()` and the inline duplicate inside
  `renderLayerTopDown()` both call `getActiveBakePalette()` instead
  of branching on `referencePalette` directly. Tightens the existing
  duplication; still local to the bake pipeline.
- `createBakeEnvironment()` similarly: when no reference palette but a
  non-default preset is active, build the gradient from the preset's
  `env_gradient` instead of using `RoomEnvironment()`. Same code path
  as the reference-palette branch — only the source colors differ.

This is the minimum change that makes "pick golden-hour, regenerate,
see warmer tones" actually work.

### D6. Validation surface in Go

- Extend `validLightingPresets` (`settings.go:70`) to all 6 ids.
- No new field; `Validate()` body unchanged.
- Existing test `TestValidate_RejectsOutOfRange` uses `"studio"` as
  the unknown-preset case — still unknown, still passes.
- Add a new test row asserting each of the 6 ids is accepted.

**Rejected**: building a Go struct mirror of the JS preset registry
for backend-side bake_config knowledge. The Go code does not bake;
nothing on the backend needs the numbers. Mirroring would be dead
weight that drifts.

### D7. The `default` preset matches `DefaultSettings()` exactly

The `default` preset's `bake_config` numbers are restated as literals
(not pulled at runtime from `makeDefaults()`) — they belong in the
registry as the authoritative starting point. A unit-style sanity
check (a small JS-side assertion at module load, dev console only)
verifies they match `makeDefaults()` to catch drift; alternative is
a Go test that loads the JS file and parses, which is more fragile.
Going with the runtime assertion: cheap, runs every page load in
dev, and breaks loudly if someone updates `DefaultSettings()` without
syncing.

## Initial preset values

Numbers below are starting points — "don't agonize". Each preset is
distinct enough to be visibly different in the bake.

- **default** — current neutral baseline. Matches `DefaultSettings()`.
  Sky/ground white-ish, intensities at the documented defaults.
- **midday-sun** — bright, neutral-warm. Stronger key, lower ambient.
  Sky 0.95/0.97/1.00, ground 0.30/0.27/0.22, key 1.80, ambient 0.45,
  exposure 1.00.
- **overcast** — soft, low-contrast, slightly cool. Sky 0.90/0.93/0.98,
  ground 0.55/0.58/0.62, key 0.60, ambient 1.10, hemisphere 1.40,
  exposure 1.00.
- **golden-hour** — warm key, low ambient, dramatic. Sky 1.00/0.85/0.55,
  ground 0.20/0.13/0.08, key 1.60, key_color 1.00/0.78/0.45, ambient
  0.30, exposure 1.10. Visible warmth requirement is satisfied here.
- **dusk** — cool blue ambient, low key. Sky 0.55/0.65/1.00, ground
  0.10/0.12/0.20, key 0.40, key_color 0.65/0.75/1.00, ambient 0.55,
  exposure 0.85.
- **indoor** — soft fill, no strong key. Sky 0.95/0.93/0.88, ground
  0.45/0.42/0.38, key 0.50, fill 0.80, ambient 0.85, exposure 0.95.

## Out of design

- Live preview color application — T-007-02.
- Profiles + preset interaction (does loading a profile reset the
  preset?) — orthogonal; profiles already overwrite individual fields.
- Reference image as a preset attribute — T-007-03.
