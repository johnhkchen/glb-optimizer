# Progress — T-007-02

All six plan steps completed in order, no deviations.

## Steps

1. **Refactor bake palette resolution helper** — `e4ddbb0`
   - Added `resolvePresetColors(cfg)` and rewired `getActiveBakePalette`.
   - Return shape gained `key` / `fill` fields (existing bake call
     sites ignore them — additive only).
2. **Add live-scene preset helpers (dead code)** — `e306372`
   - Added `bakeStale` module flag.
   - Added `getActivePreviewPalette()`, `applyPresetToLiveScene()`,
     `setBakeStale()`.
3. **Tune preview_config per preset** — `aebc1ff`
   - Extended `makePreset` with optional `preview_overrides` shallow
     merge.
   - `overcast`: ambient 1.10→0.72, hemisphere_intensity 1.40→0.90
     in preview only.
   - `golden-hour`: key_intensity 1.60→1.36 in preview only.
   - `dusk`: key_intensity 0.40→0.55 in preview only.
4. **Add stale-bake hint element** — `336170b`
   - `<span id="bakeStaleHint">` inserted in `.toolbar-actions`.
   - `.bake-stale-hint` class added to `static/style.css`.
5. **Wire preset application into live preview lifecycle** — `b9a9140`
   - `applyLightingPreset` → `applyPresetToLiveScene()` +
     `setBakeStale(true)`.
   - `selectFile` callback → `applyPresetToLiveScene()` +
     `setBakeStale(false)`.
   - `applyColorCalibration` tear-down → `applyPresetToLiveScene()`
     replaces `resetSceneLights()`.
   - All four regenerate paths call `setBakeStale(false)` on success.
6. **Extend reference tint to all directionals** — `997a0ea`
   - `applyReferenceTint` now also tints the three DirectionalLights
     (key/back → sky, under-fill → mid).

## Build / test

After every step: `go build ./... && go test ./...` returned clean
(`ok glb-optimizer`). No JS test harness exists; relied on the
dev-time `lighting.js` assertion (which keys off `bake_config`, so
preview tweaks don't affect it) and on grep-checking call sites.

## Deviations

None.

## Known caveats (carry into review)

- Manual visual verification (the rose) was not performed by the
  agent — see review.md "Open concerns".
- One commit (`6dbffaf`) was created and immediately reset before
  step 1 because it accidentally swept up pre-existing untracked
  files via `git add -A`. Re-committed with `git add static/app.js`
  only. The mistaken commit never reached the remote.
