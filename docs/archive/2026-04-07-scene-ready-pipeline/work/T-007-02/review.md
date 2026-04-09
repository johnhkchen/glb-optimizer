# Review — T-007-02: bake-preview-lighting-consistency

## What changed

### Files modified (4)

- `static/app.js`:
  - New module-level flag `bakeStale = false`.
  - New helper `resolvePresetColors(cfg)` (pure tuple→object).
  - `getActiveBakePalette()` refactored onto `resolvePresetColors`;
    return shape now also carries `key` / `fill` (additive).
  - New `getActivePreviewPalette()` mirroring the bake helper but
    reading `preview_config`.
  - New `applyPresetToLiveScene()` — walks `scene` and mutates the
    1 Ambient + 1 Hemisphere + 3 Directional lights from
    `initThreeJS()`. Discriminates the directionals by position
    (`y<0` = under-fill, `x<0` = back/rim, otherwise = main key).
  - New `setBakeStale(stale)` — toggles `bakeStale` and
    `#bakeStaleHint` visibility. Idempotent.
  - `applyLightingPreset()` calls `applyPresetToLiveScene()` and
    `setBakeStale(true)` after the cascade.
  - `selectFile()` callback calls `applyPresetToLiveScene()` and
    `setBakeStale(false)` after `populateTuningUI()`.
  - `applyColorCalibration()` tear-down branch now calls
    `applyPresetToLiveScene()` instead of `resetSceneLights()` so
    removing calibration falls back to the active preset, not
    raw white.
  - `generateBillboard`, `generateVolumetric`,
    `generateVolumetricLODs`, `generateProductionAsset`: each calls
    `setBakeStale(false)` immediately after `success = true`.
  - `applyReferenceTint()` extended to also tint the three
    DirectionalLights (key/rim → sky, under-fill → mid). Closes a
    pre-existing gap exposed by this ticket.

- `static/presets/lighting.js`:
  - `makePreset()` accepts an optional `preview_overrides` shallow
    merge applied on top of the bake_config clone.
  - `overcast.preview_config`: ambient 1.10→0.72,
    hemisphere_intensity 1.40→0.90.
  - `golden-hour.preview_config`: key_intensity 1.60→1.36.
  - `dusk.preview_config`: key_intensity 0.40→0.55.
  - All other presets unchanged (preview ≡ bake).
  - Color fields (sky/ground/key/fill/env_gradient) are identical
    between bake_config and preview_config for every preset — only
    intensities diverge — preserving the "same preset is
    recognisable in both surfaces" goal.

- `static/index.html`:
  - New `<span id="bakeStaleHint">` in `.toolbar-actions` between
    `generateProductionBtn` and `uploadReferenceBtn`. Starts hidden.

- `static/style.css`:
  - New `.bake-stale-hint` rule (color #f5b942, 12px, padded).

### Files created / deleted

None.

## Commit history

```
997a0ea Extend reference image tint to directional lights (T-007-02)
b9a9140 Wire preset application into live preview (T-007-02)
336170b Add stale-bake hint element (T-007-02)
aebc1ff Tune preview_config per preset (T-007-02)
e306372 Add live-scene preset helpers (T-007-02)
e4ddbb0 Refactor bake palette resolution helper (T-007-02)
```

Six atomic commits matching plan.md exactly. Each is independently
revertable. Reverting just `b9a9140` (the wiring) leaves the helpers
in place as dead code and reverts behavior to the pre-ticket
neutral live preview, which is the cleanest rollback boundary if
the visual tuning needs more iteration.

> Note: an earlier accidental commit (`6dbffaf`) was reset before
> step 1 because `git add -A` swept up many pre-existing untracked
> files. The replacement commit `e4ddbb0` contains only
> `static/app.js`. Local-only — never reached the remote.

## Acceptance criteria check

- [x] Main scene's lights read intensities/colors from
      `preview_config`. Implemented in `applyPresetToLiveScene()`,
      called from `applyLightingPreset` (preset change),
      `selectFile` (asset open), and `applyColorCalibration`
      (calibration tear-down).
- [x] Bake's `setupBakeLights` reads from `bake_config`. Already
      true after T-007-01 via `getActiveBakePalette()`; refactored
      onto `resolvePresetColors` in step 1.
- [x] Switching the `lighting_preset` setting immediately re-applies
      lighting to the main scene. (`applyLightingPreset` →
      `applyPresetToLiveScene`.)
- [x] Switching the `lighting_preset` setting marks the asset as
      "needs rebake" via the `#bakeStaleHint` element.
      `setBakeStale(true)` toggles the warning; the four regenerate
      paths clear it on success.
- [x] `bake_config` and `preview_config` produce *visually similar*
      lighting on the same model — colors are identical, intensities
      diverge only where the live scene's busier 5-light topology
      demanded compensation (overcast, golden-hour, dusk). Default,
      midday-sun, indoor: preview ≡ bake.
- [x] Reference image calibration mode still works alongside
      presets. Priority order is preserved
      (`getActivePreviewPalette` checks `referencePalette` first);
      bonus: `applyReferenceTint` now also covers the three
      DirectionalLights, which it did not before.
- [ ] **Manual verification not yet performed.** Per the AC: "with
      the rose loaded, switch presets and watch the live preview
      update; regenerate production asset and confirm the bake
      matches." See "Open concerns" below.

## Test coverage

- **Go**: untouched. `go test ./...` clean after every step
  (`ok glb-optimizer`). No new Go code.
- **JS**: no harness exists. Verifications relied on:
  - Reading `applyPresetToLiveScene` against the directional
    positions hardcoded in `initThreeJS()` to confirm the
    discriminator (`y<0` = dirLight3, `x<0` = dirLight2, else =
    dirLight) is unambiguous given:
    - dirLight @ (5, 10, 7) — `x>0`, `y>0`
    - dirLight2 @ (-5, 5, -5) — `x<0`
    - dirLight3 @ (0, -3, 5) — `y<0`
  - Re-running the existing dev-time assertion in `lighting.js`,
    which keys off `bake_config` (untouched). Preview tweaks do
    not affect it.
  - Grep verification that all four regenerate paths now contain
    `setBakeStale(false)` adjacent to `success = true`.
- **Manual checklist** (for the human reviewer):
  1. Load any asset (the rose if available).
  2. Pick `golden-hour` from the preset dropdown — the live preview
     should immediately turn warmer/orange-tinted, the
     `Bake out of date` hint should appear in the toolbar.
  3. Pick `dusk` — live preview should shift cool/blue, hint
     stays.
  4. Pick `overcast` — ambient brightness should drop relative to
     the bake (per the preview attenuation in step 3), so the
     model doesn't look blown out.
  5. Click "Production Asset" → wait for completion → hint should
     disappear.
  6. Pick `default` — live preview returns to the pre-ticket
     neutral look, hint reappears.
  7. Reload the page — hint should NOT persist (state is in-memory
     only per design D6).
  8. Upload a reference image while a preset other than `default`
     is active → confirm directional lights also adopt the
     calibrated tint (this was previously a gap).
  9. Switch to a fresh asset → hint should clear.

## Open concerns

1. **Manual visual verification not performed.** The agent has no
   browser; the "switch presets and watch the preview" check in
   the AC is on the human reviewer. If a preview attenuation looks
   wrong, the only file to tweak is
   `static/presets/lighting.js` (the `preview_overrides` block on
   the offending preset). No schema or wiring changes required.

2. **Preset application is silent on first init when no asset is
   selected.** `applyPresetToLiveScene` early-returns when
   `currentSettings` is null. The very first `selectFile` triggers
   it. This matches existing reference-tint behavior; flagging
   only because users clicking around the file list before
   selecting anything will see the legacy hardcoded
   `initThreeJS()` lights for ~a frame. Not worth fixing.

3. **`bakeStale` is in-memory only (per design D6).** Reloading
   the page after picking a preset will *not* show the hint, even
   if the on-disk bake still reflects the previous preset. The AC
   only asks for "a small UI hint", and persisting it would
   require either tracking `last_baked_preset` in `AssetSettings`
   (schema bump) or rescanning baked-texture metadata. Either is
   a follow-up if user feedback warrants it.

4. **Live preview env_map_intensity is not driven by the preset.**
   Materials in the live preview pick up `scene.environment`
   (`defaultEnvironment` or `referenceEnvironment`) but
   `envMapIntensity` is only set in `cloneModelForBake`. For full
   preset consistency a future ticket could mutate the live
   model's materials' `envMapIntensity` from `preview_config.env_intensity`.
   Out of scope for this AC ("intensities/colors", not "envMap").

5. **Directional-light role is discriminated by position.** If a
   future change to `initThreeJS` moves the lights, the
   discriminator in `applyPresetToLiveScene` and the (also
   position-based) extension in `applyReferenceTint` will silently
   misroute. Worth a comment in `initThreeJS` if anyone touches
   it. Acceptable for now — it's a 6-line block of code five
   functions away.

6. **Preview attenuations were chosen analytically, not
   empirically.** I picked `0.65×` for overcast diffuse, `0.85×`
   for golden-hour key, and `0.55→0.55` for dusk key based on the
   five-light topology vs the four-light bake topology, not by
   loading the rose and eyeballing. Likely close enough per the
   first-pass scope; reviewer should sanity-check on the actual
   model.

7. **`applyReferenceTint` directional extension is technically a
   pre-existing-bug fix bundled into this ticket.** It's the
   minimum necessary to keep AC#5 honest ("when active, it tints
   the current preset's colors") because otherwise the tinted
   ambient + hemisphere would coexist with white directionals.
   Calling out so it's not surprising in the diff.

## Out of scope (for clarity)

- Reference image as a preset (T-007-03).
- Auto-rebake on preset change.
- Real-time lighting in live preview during bake.
- Sky-as-background rendering.
- Live `envMapIntensity` application (see concern 4).
- Persisting `bakeStale` across reloads (see concern 3).

## Build / test status

- `go build ./...` — clean.
- `go test ./...` — passes (`ok glb-optimizer`).
- No JS test harness exists in the repo.
