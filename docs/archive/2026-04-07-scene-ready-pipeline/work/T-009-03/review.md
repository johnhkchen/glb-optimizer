# Review — T-009-03

Three-stage crossfade and production-bundle integration. T-009-01 baked
the tilted billboard, T-009-02 wired runtime instancing, and this
ticket bundles the tilted bake into the production pipeline and
replaces the existing 2-state crossfade with a 3-state unified
visibility function.

## Files changed

| File | Change |
|---|---|
| `settings.go` | Added `TiltedFadeLowStart` / `TiltedFadeLowEnd` / `TiltedFadeHighStart` to `AssetSettings` (omitempty), defaults 0.30 / 0.55 / 0.75 in `DefaultSettings()`, three [0,1] `checkRange` calls in `Validate()`. |
| `settings_test.go` | New `TestDefaultSettings_TiltedFadeFields` asserting the three defaults; three new rows in the `TestValidate_RejectsOutOfRange` table covering negative-low and >1 cases. |
| `static/app.js` | (1) Three keys in `makeDefaults()`. (2) New `normalizeTiltedFadeFields()` called from `loadSettings` to backfill legacy on-disk JSON. (3) Three new `TUNING_SPEC` entries — auto-instrumented for `setting_changed` analytics. (4) New `let productionHybridFade = false;` module flag, reset in `clearStressInstances`. (5) New `updateHybridVisibility()` + helper `applyOpacityToMeshes()`. (6) `animate()` dispatches between the unified pass and the legacy 2-state functions based on `productionHybridFade`. (7) `generateProductionAsset(id, onSubstage)` gained an optional progress callback and now bakes the tilted billboard between the horizontal and volumetric passes. (8) `prepareForScene` stage 4 passes a callback that updates the running label per substage; success check requires all three flags. (9) `runProductionStressTest` gates on all three flags, loads three GLBs in parallel, instantiates all three layers, and sets `productionHybridFade = true`. |
| `static/index.html` | Three new `<div class="setting-row">` slider blocks inserted after the `tuneAlphaTest` row (`tuneTiltedFadeLowStart`, `tuneTiltedFadeLowEnd`, `tuneTiltedFadeHighStart`). |
| `static/help_text.js` | Three new tooltip strings explaining the bands. |

No backend route changes — `handlers.go`, `main.go`, `models.go`,
`analytics.go`, and `docs/knowledge/analytics-schema.md` are
untouched.

## Acceptance criteria check

- [x] **Three-stage crossfade math.** `updateHybridVisibility()`
  computes `lookDownAmount = abs(camDir.dot(0,-1,0))` once and applies
  four opacities (billboard side, billboard top, tilted, volumetric)
  via two smoothstep bands. Source-of-truth thresholds are
  `currentSettings.tilted_fade_low_start / low_end / high_start` plus
  the fixed `highEnd = 1.0`. Math derivation is in
  `docs/active/work/T-009-03/design.md`. The legacy
  `updateBillboardVisibility` and `updateVolumetricVisibility`
  functions are intact for the standalone preview modes — no
  regression risk to non-hybrid paths.
- [x] **Setting fields.** `tilted_fade_low_start = 0.30`,
  `tilted_fade_low_end = 0.55`, `tilted_fade_high_start = 0.75`.
  Existing dome `fadeStart`/`fadeEnd` (0.55/0.85) are *not* edited
  in `updateVolumetricVisibility` because that function is now only
  used for the standalone Volumetric preview, where the original
  behavior is correct. The hybrid path's dome opacity is computed
  from the same `tilted_fade_high_start`/1.0 band requested in the
  ticket.
- [x] **Production bundle integration.** `generateProductionAsset`
  bakes billboard → tilted billboard → volumetric in sequence.
  `prepareForScene` stage 4 success requires
  `has_billboard && has_billboard_tilted && has_volumetric`.
  `runProductionStressTest` loads all three GLBs in parallel and
  instantiates all three at the same positions, with
  `productionHybridFade = true`.
- [x] **prepareForScene per-stage progress UI.** The "Production
  asset" row's running label updates between sub-bakes
  (`production — horizontal bake…` → `tilted bake…` → `volumetric
  bake…` → final ✓). Implemented as a label update rather than
  substages — see design.md Option C rejection rationale.
- [x] **Settings + UI sliders.** Three new range inputs in the
  tuning panel, range 0.0–1.0, step 0.01. Wired through `TUNING_SPEC`
  so populate / save / dirty-dot / `setting_changed` analytics all
  work for free.
- [x] **`setting_changed` analytics.** Auto-instrumented by
  `wireTuningUI`. No new event types added.

## Test coverage

| Layer | Coverage | Gap |
|---|---|---|
| Go `AssetSettings` defaults | `TestDefaultSettings_TiltedFadeFields` asserts 0.30 / 0.55 / 0.75 exactly. | None. |
| Go `Validate()` range checks | Three rows added to `TestValidate_RejectsOutOfRange` (negative low_start, >1 low_end, >1 high_start). | The Validate() doesn't enforce ordering (`low_start < low_end < high_start`) — intentional, matches the existing `alpha_test` style of range-only validation, and the JS smoothstep clamps so degenerate orderings produce a hard cut rather than NaN. |
| Go settings migration | Pre-existing `TestLoadSettings_MigratesOldFile` continues to pass with no edits — proves legacy on-disk JSON without the new keys still loads cleanly via `omitempty`. | None. |
| Go handlers | No backend route changes; `handlers_billboard_test.go` from T-009-01 already covers the tilted upload endpoint. | None. |
| JS visibility math | `node --check static/app.js` exits 0. | No JS unit-test runner in this repo (confirmed in T-009-01 / T-009-02 reviews). The crossfade smoothstep formulas can only be exercised manually in the browser. |
| JS bundle integration | `node --check` syntax pass. | The triple-bake path through `generateProductionAsset` has not been exercised end-to-end without a manual run on a real asset. |
| Bundle-size budget (<500 KB on rose) | None automated. | Manual verification per ticket. |

## Open concerns

1. **Manual verification on the rose is the gating step.** All
   automated checks are green, but the user-visible payoff
   (a noticeably smoother transition through ~45°) and the bundle
   size budget can only be confirmed by running `Prepare for scene`
   on the rose, opening the Production preview, running the count=20
   stress test, and slowly orbiting from horizontal to overhead. If
   the defaults look off, drag the three new sliders by eye and
   update the `DefaultSettings()` / `makeDefaults()` / HTML
   `value="..."` defaults to match.

2. **`prepareForScene` analytics payload is unchanged.** The
   `prepare_for_scene` event still records `stages_run: ['gltfpack',
   'classify', 'lods', 'production']` — no sub-stage granularity is
   captured. If we ever need to attribute prepare-time spend across
   the three sub-bakes, this needs design work; deliberately out of
   scope for this ticket.

3. **`updateBillboardVisibility` still uses the hard-coded
   `fadeStart=0.55, fadeEnd=0.75`** because it serves the standalone
   Billboard preview button (not Production). Same for
   `updateVolumetricVisibility` (`0.55, 0.85`). The ticket text
   suggested editing these to align with the new tilted bands; we
   intentionally did not, because doing so would change behavior of
   the standalone preview modes that don't have a tilted layer to
   transition through. The new tunable thresholds drive only the
   unified Production hybrid pass — that is the surface the user
   actually ships with.

4. **`generateProductionAsset` still emits a single `regenerate`
   event with `trigger: 'production'`** even though it now does three
   bakes. We did *not* fan out per-substage `regenerate` events
   (which would also have meant emitting a `billboard_tilted` event
   alongside the `production` one) — keeping a single trigger
   per user action matches the existing `production` semantics and
   avoids double-counting in analytics joins. Devtools-only callers
   that want a standalone tilted-only event continue to use
   `generateTiltedBillboard`, which still emits its own
   `regenerate{trigger:'billboard_tilted'}`.

5. **Tilted bake uses the same `currentSettings.alpha_test`** as the
   horizontal bake, inherited from the existing
   `renderTiltedBillboardGLB` implementation. If that field is ever
   split per-impostor-type, this code path needs a paired update.

## Handoff notes for the reviewer

- Read the diff in this order to follow the data flow:
  `settings.go` → `static/app.js` (makeDefaults / TUNING_SPEC /
  loadSettings) → `static/index.html` → `static/app.js`
  (`updateHybridVisibility` + `animate()` dispatch) →
  `generateProductionAsset` → `prepareForScene` →
  `runProductionStressTest`.
- The math derivation, including the choice to drive the side/top
  split off `sLow` rather than the legacy 0.55–0.75 band, is in
  `design.md`. Verify the four columns of the spec table by
  substituting `lookDown = 0.0, 0.30, 0.42, 0.55, 0.65, 0.75, 0.88,
  1.0` into the formulas if you want to double-check on paper.
- Step 6 of `plan.md` is the manual checklist for the rose — please
  run it before merging if possible.
