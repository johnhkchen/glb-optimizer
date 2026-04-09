# Design — T-007-02: bake-preview-lighting-consistency

## Goal

Make the live three.js preview reflect the active lighting preset in
the same recognisable way the bake does, refresh the preview
in-place when the preset changes, and surface a "stale bake" hint so
the user knows the on-disk textures haven't been re-baked yet.

## Decisions

### D1: Reuse `preview_config`, do not invent a new schema

`static/presets/lighting.js` already exposes a `preview_config` field
that is currently a clone of `bake_config`. We will:

- Keep the structural identity (same keys, same shape) so callers can
  switch between the two by passing the field name.
- Hand-tune `preview_config` per preset where the live preview's
  busier 5-light topology demands it (intensity attenuation only —
  colors stay identical so the bake and preview look like the same
  preset).

**Rejected**: introducing a separate `live_intensity_factor` per
preset, or scaling at the application site. Too magical, drift-prone,
and obscures which numbers actually drive the scene.

### D2: Add `applyPresetToLiveScene()`, do NOT rebuild lights

The live scene's lights are added once in `initThreeJS()` and never
re-created. We will mirror the existing
`applyReferenceTint` / `resetSceneLights` pattern: traverse `scene`,
match by light type, mutate `.color` / `.intensity` /
`.groundColor` in place. Cheap, no GC churn, leaves OrbitControls /
helpers alone.

**Rejected**: tearing down and re-adding lights. Risk of subtle
ordering bugs (helpers, debug overlays), and the existing tint
helpers prove the in-place mutation pattern works.

### D3: Five live lights mapped to preview_config

The live `initThreeJS` creates one Ambient, three Directionals (key
@(5,10,7), back @(-5,5,-5), under-fill @(0,-3,5)), and one
Hemisphere. The mapping:

| Live light    | Color source           | Intensity source                  |
|---------------|------------------------|-----------------------------------|
| Ambient       | `hemisphere_sky`       | `ambient`                         |
| Hemisphere    | `hemisphere_sky` / `hemisphere_ground` | `hemisphere_intensity` |
| dirLight      | `key_color`            | `key_intensity`                   |
| dirLight2     | `hemisphere_sky`       | `key_intensity * 0.55` (soft rim) |
| dirLight3     | `fill_color`           | `fill_intensity`                  |

The "0.55" attenuation for the rim light is the only magic number;
it preserves the existing relative balance (1.5 vs 0.8 → ~0.53). We
identify dirLight2 vs dirLight by position — only dirLight2 has
negative x. dirLight3 is the only one with negative y.

**Rejected**: tagging lights with `userData.role` in `initThreeJS()`
and looking up by role. Cleaner in theory but requires touching
`initThreeJS` for a one-shot dispatch this ticket already needs to
visit anyway, and the position-based discriminator is trivially
testable.

### D4: Resolution helper mirrors `getActiveBakePalette`

Add `getActivePreviewPalette()` (and a small shared helper
`resolvePresetColors(cfg)` so both call sites stay one-line). Priority
order matches the bake:

1. `referencePalette` (explicit user calibration)
2. Active preset's `preview_config`
3. Neutral fallback

This preserves AC#5: "Reference image calibration mode still works
alongside presets — when active, it tints the current preset's
colors." Practically, when a reference palette is active, the live
preview uses the reference palette colors (same as today via
`applyReferenceTint`); we extend that path so the directional lights
also pick up the tint, closing a pre-existing gap.

### D5: Hook `applyPresetToLiveScene` into three call sites

The live preview lighting must refresh:

1. **In `applyLightingPreset`** — after the cascade, call
   `applyPresetToLiveScene()` so dropdown changes are immediate.
2. **In `selectFile` / `loadSettings`** — after settings load, apply
   the preset (so opening an asset that was saved with `dusk`
   immediately shows dusk lighting).
3. **In `applyColorCalibration` (when calibration is torn down)** —
   replace the `resetSceneLights()` call so we re-apply the active
   preset rather than snapping back to neutral white.

`resetSceneLights()` is *kept* as a safety net for the
"no asset loaded" case but its in-asset call sites move to
`applyPresetToLiveScene()`.

### D6: "Needs rebake" hint = client-only flag + DOM badge

A new module-level flag `bakeStale` plus a small DOM element
`#bakeStaleHint` near the regenerate button. State machine:

- `applyLightingPreset` → set `bakeStale = true`, show hint.
- A successful regenerate → set `bakeStale = false`, hide hint.
- `selectFile` → reset `bakeStale = false` for the new asset.

Hint text: "Bake out of date — regenerate to apply preset to assets."
The flag is **per-session, not persisted**. The AC explicitly says
"a small UI hint, since the existing baked textures still reflect the
old preset" — no on-disk staleness tracking required, and persisting
it would invite drift between settings file and the actual baked
textures on disk.

**Rejected**: storing a `last_baked_preset` field in
AssetSettings. More accurate (would survive page reload) but pushes
the schema; the AC says hint, not durable state.

### D7: `preview_config` tuning rule of thumb

Per AC: "visually similar … not pixel-identical". For most presets we
will keep `preview_config` ≡ `bake_config` and only tweak when the
five-light live setup blows out (e.g. `overcast` with high ambient
+ high hemi gets washed out under three directionals; we attenuate
ambient and hemi by ~0.6 in `preview_config` for that case).

The first-pass scope per the ticket: "Don't aim for perfect parity —
close enough that the user can tell which preset is active in both
views." So we will tune per preset only as needed and accept that
each preset's preview will look slightly hotter than its bake.

## Non-goals

- Restructuring `initThreeJS` to take a preset parameter at startup
  (the cascade dispatch handles initial state via the
  `selectFile`/`loadSettings` hook in D5).
- Tagging lights with `userData.role` (D3).
- Auto-rebake on preset change (out of scope per ticket).
- Live preview color application of `env_map_intensity` (flagged for
  follow-up in research; not in AC).
- Sky-as-background rendering (out of scope per ticket).
- Persisting `bakeStale` across reloads.

## Risks

1. **Preset application timing during init.** `selectFile` calls
   `loadSettings(...).then(populateTuningUI)`. If
   `applyPresetToLiveScene()` runs before three.js scene init
   completes, the traverse is a no-op. We will guard with
   `if (!scene) return` (the same guard `applyReferenceTint` and
   `resetSceneLights` already use implicitly via `scene.traverse`).
2. **`bakeStale` persistence after page reload.** The hint will be
   absent on reload even if the on-disk bake was generated under a
   different preset. Acceptable per D6; we'll mention it in
   `review.md` so a future ticket can opt to persist.
3. **Reference image + preset interaction.** Today
   `applyReferenceTint` only touches Ambient + Hemisphere. We will
   extend the preview-application path so the directionals also pick
   up the reference colors when calibration is active. This is a
   small enhancement that lives naturally inside
   `applyPresetToLiveScene` / `getActivePreviewPalette`. Worth
   calling out in review.
4. **Existing dirLight intensities are tuned for the legacy
   neutral-white scene.** For some presets the new mapping will look
   visibly different even when the colors are right (e.g. `dusk` at
   `key_intensity = 0.40` will look much darker than the legacy
   `1.5`). This is the desired behavior (the AC's whole point is
   "preview matches bake") but users with muscle memory may be
   surprised on first run. Mention in review.
