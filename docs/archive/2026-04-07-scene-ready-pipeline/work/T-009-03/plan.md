# Plan — T-009-03

Six committable steps. Each ends in a green build (`go build ./...`,
`go test ./...`, `node --check static/app.js`). Manual verification
runs at the very end.

## Step 1 — backend settings fields

**Files:** `settings.go`, `settings_test.go`.

- Add `TiltedFadeLowStart`, `TiltedFadeLowEnd`, `TiltedFadeHighStart`
  to `AssetSettings`, in that order, after `AlphaTest`. JSON tags
  `tilted_fade_low_start` / `tilted_fade_low_end` /
  `tilted_fade_high_start`, all with `,omitempty`.
- Add three lines to `DefaultSettings()`: 0.30, 0.55, 0.75.
- Add three `checkRange` calls in `Validate()` after the existing
  `alpha_test` check, range [0, 1].
- Add `TestDefaultSettings_TiltedFadeFields` asserting the three
  defaults.
- Extend `TestValidate_RejectsOutOfRange` table with one negative-low
  and one >1 case (e.g. `s.TiltedFadeLowStart = -0.1`,
  `s.TiltedFadeHighStart = 1.5`).

**Verify:**
- `go test ./...` passes.
- The pre-existing `TestLoadSettings_MigratesOldFile` still passes
  unchanged (legacy JSON has zero for the new fields, but `omitempty`
  + zero-tolerant validation accepts them).

**Commit:** `T-009-03: settings fields for tilted fade thresholds`

## Step 2 — JS settings data plumbing

**Files:** `static/app.js`, `static/index.html`, `static/help_text.js`.

- Add three keys to `makeDefaults()` (mirror Go defaults exactly).
- Add zero-normalization at the settings-load site so legacy
  on-disk JSON gets sane values.
- Add three entries to `TUNING_SPEC` (`parseFloat`, `toFixed(2)`).
- Add three `<div class="setting-row">` blocks in `index.html`
  immediately after the `tuneAlphaTest` row.
- Add three help-text strings.

**Verify:**
- `node --check static/app.js` exits 0.
- Manual sanity (post-implement): load any asset, the three sliders
  render and read back from `currentSettings`.

**Commit:** `T-009-03: tuning panel sliders + JS defaults for tilted fade`

## Step 3 — unified visibility function

**Files:** `static/app.js`.

- Declare `let productionHybridFade = false;` next to
  `volumetricHybridFade`.
- Reset to `false` in `clearStressInstances`.
- Add `updateHybridVisibility()` adjacent to the existing visibility
  functions, implementing the four-opacity smoothstep math from
  `design.md`.
- Replace the visibility/facing dispatch in `animate()` with the
  if/else block from `structure.md` step 10.

**Verify:**
- `node --check static/app.js` exits 0.
- Reading the diff, the legacy `updateBillboardVisibility` and
  `updateVolumetricVisibility` are unchanged in body, only their
  call site in `animate()` moves into the non-hybrid branch.

**Commit:** `T-009-03: three-state hybrid visibility function`

## Step 4 — generateProductionAsset triple bake

**Files:** `static/app.js`.

- Refactor `generateProductionAsset(id)` →
  `generateProductionAsset(id, onSubstage = () => {})`.
- Insert `onSubstage('horizontal')` before the billboard upload,
  `onSubstage('tilted')` after, `onSubstage('volumetric')` after the
  tilted upload, in line with the three sub-bakes.
- Add the tilted bake block (renderTiltedBillboardGLB +
  upload-billboard-tilted POST + `store_update(id, f =>
  f.has_billboard_tilted = true)`) between the existing two.
- The success boolean and finally-block analytics are unchanged.

**Verify:**
- `node --check static/app.js` exits 0.
- Direct path: clicking "Build hybrid impostor" (existing button)
  must still work without passing a callback (default no-op).

**Commit:** `T-009-03: bundle tilted bake into generateProductionAsset`

## Step 5 — prepareForScene + runProductionStressTest wiring

**Files:** `static/app.js`.

- Stage 4 of `prepareForScene` calls
  `generateProductionAsset(id, substage => markPrepareStage('production', 'running', `${substage} bake…`))`.
- Failure check in stage 4 expanded to require all three flags.
- `runProductionStressTest`: gate on three flags, fetch three GLBs in
  parallel via `Promise.all`, instantiate three layers, set
  `productionHybridFade = true` after.

**Verify:**
- `node --check static/app.js` exits 0.
- Reading the diff, no new analytics events; existing
  `prepare_for_scene` event payload is unchanged in shape.

**Commit:** `T-009-03: prepareForScene + production stress test load all three`

## Step 6 — manual verification on the rose

**Files:** none (no commit).

Per ticket Verification:
1. Load the rose asset.
2. Click "Prepare for scene". Watch the production stage cycle
   through `horizontal bake…` / `tilted bake…` / `volumetric bake…`
   in the per-stage UI.
3. Open the Production preview, run a count=20 stress test.
4. Slowly orbit from horizontal to overhead. The transition through
   the ~45° band should be smoother than today's 2-state crossfade.
   No "tipping over" billboards. No "paper-thin slices" hard cut.
5. Open devtools console and read
   `(await fetch('/api/files')).then(r=>r.json())` to confirm the
   rose's record has all three flags. Note the bundle delta.
6. If the defaults look off, drag the three new sliders by eye until
   smooth, then update the defaults in `settings.go`,
   `static/app.js`, and `static/index.html` (one extra commit).

**No commit unless defaults change.**

## Test strategy summary

| Layer | Test |
|---|---|
| Go settings struct | `TestDefaultSettings_TiltedFadeFields`, `TestValidate_RejectsOutOfRange` table extension. |
| Go migration | Pre-existing `TestLoadSettings_MigratesOldFile` continues to pass — proves legacy JSON stays valid. |
| Go handlers | No changes; existing `handlers_billboard_test.go` covers the tilted upload route. |
| JS visibility math | Cannot unit-test (no JS test runner in repo, confirmed in T-009-01/02 reviews). Coverage is `node --check` for syntax + manual verification per ticket. |
| Bundle integration | Manual: `prepareForScene` + Production preview stress test. |

## Rollback

Each step is an isolated commit. Rolling back step 4 + 5 alone leaves
the unified visibility function and tunable settings present but
inert (no caller sets `productionHybridFade=true`). Rolling back
steps 3–5 leaves only the new settings fields, harmless on disk.
