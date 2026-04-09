# Research — T-009-03

Three-stage crossfade and production-bundle integration. T-009-01 baked the
tilted billboard and exposed it via `/api/upload-billboard-tilted/:id`.
T-009-02 wired runtime instancing (`createTiltedBillboardInstances`,
`updateTiltedBillboardFacing`, `tiltedBillboardInstances[]`) and added a
"Tilted" preview button. This ticket is the payoff: 3-state visibility
crossfade + bundling the tilted bake into `generateProductionAsset` /
`prepareForScene` / `runProductionStressTest`.

## Existing 2-state crossfade

`static/app.js:4024 updateBillboardVisibility()` — runs in `animate()`
when `stressActive && billboardInstances.length > 0`. Reads camera dir,
computes `lookDownAmount = abs(camDir.dot(0,-1,0))`, smoothsteps in band
0.55–0.75 between `billboardInstances` (side, fade out) and
`billboardTopInstances` (top, fade in). Sets each mesh's
`material.opacity` and toggles `mesh.visible` below 0.01.

`static/app.js:3984 updateVolumetricVisibility()` — same pattern,
gated on `volumetricHybridFade`. Fade band 0.55–0.85, fades the dome
slices in as `lookDownAmount` increases. Used only by
`runProductionStressTest`, which calls `createVolumetricInstances(..., true)`
to set `volumetricHybridFade = true`.

These two crossfades are independent in implementation but their bands
overlap (0.55–0.75 for side→top, 0.55–0.85 for dome). Side and top
horizontal billboards are the only 2-element fade today; the dome is
toggled in vs the *combined* horizontal pair.

## Tilted runtime surface area (T-009-02)

`static/app.js:3801 tiltedBillboardInstances = []` (module state, reset
in `clearStressInstances`).
`static/app.js:4061 createTiltedBillboardInstances(model, arr)` — mirrors
`createBillboardInstances` minus the `billboard_top` carve-out. Every
mesh in the tilted bake is a side variant. Uses seed `+7777` so the
variant assignment doesn't collapse onto the horizontal one.
`static/app.js:4134 updateTiltedBillboardFacing()` — Y-billboarded
matrix update each frame, mirror of `updateBillboardFacing`.
`animate()` already calls `updateTiltedBillboardFacing()` whenever
`tiltedBillboardInstances.length > 0`, but no visibility function is
attached — the tilted instances are currently always visible when
present, which is fine for the standalone "Tilted" preview button but
must be replaced with the new 3-state visibility for the Production
hybrid.

## Production bundle pipeline

`static/app.js:2389 generateProductionAsset(id)` runs two bakes
sequentially: `renderMultiAngleBillboardGLB → /api/upload-billboard/:id`
then `renderHorizontalLayerGLB → /api/upload-volumetric/:id`. Marks
the file `has_billboard` / `has_volumetric` in the local store and
emits a single `regenerate` analytics event with
`trigger: 'production'`. The standalone tilted bake helper
`generateTiltedBillboard(id)` lives at `app.js:1330` and is currently
only callable from devtools (`window.generateTiltedBillboard`). Uses
the same shape — `renderTiltedBillboardGLB → /api/upload-billboard-tilted/:id`,
emits `regenerate` with `trigger: 'billboard_tilted'`.

`static/app.js:2472 prepareForScene(id)` orchestrates the four stages
defined in `PREPARE_STAGES = [{gltfpack},{classify},{lods},{production}]`.
Each stage is rendered as an `<li>` in `prepareStages` via
`setPrepareStages` / `markPrepareStage` (status: pending|running|ok|error).
Stage 4 ("production") just calls `generateProductionAsset(id)` and
checks `after.has_billboard && after.has_volumetric` afterward.

`static/app.js:4231 runProductionStressTest(positions)` loads
`?version=billboard` and `?version=volumetric` in parallel, then calls
`createBillboardInstances` and `createVolumetricInstances(model, positions, true)`
to set `volumetricHybridFade = true`. Triggered from the Production
hybrid LOD button when `previewVersion === 'production'`
(`app.js:4197`).

## Backend surface (T-009-01)

- `handlers.go:466 handleUploadBillboardTilted` mirrors
  `handleUploadBillboard`, writes `{outputs}/{id}_billboard_tilted.glb`,
  flips `record.HasBillboardTilted`.
- `handlers.go:335` serves `?version=billboard-tilted` from the same
  preview route.
- `handlers.go:657` deletes the tilted GLB on file delete.
- `main.go:212` `scanExistingFiles` detects the tilted GLB on startup.
- `models.go:56 HasBillboardTilted bool` field on `FileRecord`.
- `handlers_billboard_test.go` exists with happy-path + 404 + wrong-method
  coverage for the tilted upload endpoint.

No backend changes are required for T-009-03 — the tilted endpoint and
flag already work. All work is in `static/app.js` + `static/index.html`
+ `settings.go` (3 new tunable fields) + JS `makeDefaults()` + tests.

## Settings infrastructure

`settings.go:23 AssetSettings` struct, declaration order = JSON order.
Adding new fields requires:
- struct field + json tag,
- `DefaultSettings()` entry,
- `Validate()` `checkRange` call,
- `static/app.js:142 makeDefaults()` mirror (kept in sync by hand per
  the comment),
- `TUNING_SPEC` entry in `app.js:687` for auto-instrumented sliders +
  populate/wire/save/dirty/setting_changed analytics for free,
- HTML `setting-row` with matching `id` (`tuneFoo`) and value display
  (`tuneFooValue`),
- one-line `help_text.js` entry (optional but consistent),
- `settings_test.go` defaults assertion.

`settings.go:88 validResolutions`, `validLightingPresets`, etc. already
guard the discrete fields. New continuous fields use `checkRange`.

## Crossfade math constraints

The ticket spec table mandates:

| `lookDownAmount` | Horizontal | Tilted | Dome |
|---|---|---|---|
| 0.00–0.30 | 1.0 | 0.0 | 0.0 |
| 0.30–0.55 | fade out | fade in | 0.0 |
| 0.55–0.75 | 0.0 | 1.0 | 0.0 |
| 0.75–1.00 | 0.0 | fade out | fade in |

Two non-overlapping smoothstep bands:
- low band: `tilted_fade_low_start` (0.30) → `tilted_fade_low_end` (0.55)
  drives horizontal→tilted handoff.
- high band: `tilted_fade_high_start` (0.75) → `1.0` drives
  tilted→dome handoff.

All three opacities derive from the same `lookDownAmount` reading; one
visibility function can compute and apply all three to keep the math
adjacent and symmetric. The existing dome `fadeStart=0.55, fadeEnd=0.85`
must shift to `0.75 → 1.0` to align with `tilted_fade_high_start`.

## Stress test layering for "production" mode

`runProductionStressTest` currently fetches 2 GLBs in parallel and
makes 2 instance arrays. For T-009-03 it must fetch 3 in parallel and
make 3. The instance helpers are already separate:
- `createBillboardInstances(scene, positions)` — horizontal side/top
- `createTiltedBillboardInstances(scene, positions)` — tilted side
- `createVolumetricInstances(scene, positions, hybridFade=true)` — dome

Each pushes into its own module-level array (`billboardInstances`,
`tiltedBillboardInstances`, `volumetricInstances`) and `clearStressInstances`
already resets all three.

## Open assumptions

1. The new visibility function should *replace* the gating in
   `animate()` so that during a Production stress test the tilted
   instances follow the 3-state crossfade, but during the standalone
   "Tilted" preview (no horizontal/dome present) they remain fully
   visible. Easiest gate: a `tiltedHybridFade` boolean mirroring
   `volumetricHybridFade`, set true only by `runProductionStressTest`.
2. The bundle-size budget (<500 KB) is verified manually post-implement.
3. Default thresholds (0.30/0.55/0.75) come straight from the ticket;
   tuning by eye against the rose is a manual step in Verification.
4. `prepareForScene`'s "production" stage stays a single line item;
   the spec allows substages but the simpler path is to update the
   running label (`markPrepareStage('production', 'running', 'tilted bake…')`)
   as each of the three bakes runs.
