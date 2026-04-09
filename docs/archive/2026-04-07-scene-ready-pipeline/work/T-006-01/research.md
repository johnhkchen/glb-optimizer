# Research — T-006-01: scene template framework + variation

## Ticket goal in one line

Replace the hardcoded "perfect grid of identical clones" stress test
with a pluggable scene-template framework: deterministic scattered
placements with per-instance scale, position, and rotation variation,
honoring per-asset orientation rules from S-004 / T-004-05.

## Where the current stress test lives

All stress-test logic is in `static/app.js`. There is no dedicated
module — it lives in a "Stress Test" section starting at line ~2995.

Key state (top of file):

- `static/app.js:23` `let stressInstances = []` — flat list of `THREE.InstancedMesh`
  objects added to the live scene; cleared by `clearStressInstances()`.
- `static/app.js:24` `let stressActive = false` — gates billboard / volumetric
  facing+visibility updates inside the render loop.
- `static/app.js:29` `let originalModelBBox = null` — bbox of the originally
  loaded asset, captured once so spacing stays stable when previewing
  alternate versions (billboard, volumetric, LODs).
- `static/app.js:3030` `billboardInstances`, `static/app.js:3031` `billboardTopInstances`,
  `static/app.js:3118` `volumetricInstances`, `static/app.js:3119` `volumetricHybridFade`
  — module globals consumed by the per-frame facing/fade routines.

## Entry point: `runStressTest`

`static/app.js:3255` `runStressTest(count, useLods, quality)` is the single
entry point invoked by the toolbar `Run` button (`stressBtn` handler at
`static/app.js:3818`).

Steps it performs today:

1. `clearStressInstances()`, `stressActive = true`, hide `currentModel`.
2. Compute `spacing = max(size.x, size.z) * 1.3` from `originalModelBBox`.
3. Build a perfect square `cols × rows` grid of `THREE.Vector3` positions
   centered on the origin (Y=0).
4. Branch on `previewVersion` (`billboard` / `volumetric` / `production`
   / regular mesh) and call one of:
   - `createBillboardInstances(model, positions)`
   - `createVolumetricInstances(model, positions, hybridFade)`
   - `runProductionStressTest(positions)` (loads two GLBs in parallel)
   - `createInstancedFromModel(model, count, positions, randomRotateY)`
5. If `useLods` is set, defer to `runLodStressTest(...)` which buckets
   positions by distance-from-center and loads each LOD GLB on demand.
6. Pull camera back to frame the grid; update FPS / triangle / memory
   overlays.

The grid construction is **inlined** at `static/app.js:3270-3280` — there is
no scatter helper, no per-instance variation, no seed parameter. The
only "variation" is the optional Y-rotation in
`createInstancedFromModel`, gated by `randomRotateY`.

## Determinism primitive

`static/app.js:2997` `seededRandom(i)` — pure `sin(i*127.1+311.7)*43758.5453`
hash, returning `[0,1)`. Already used by:

- `createInstancedFromModel` (line 3017) — Y rotation per instance.
- `createBillboardInstances` (line 3049, 3096) — variant pick + top-quad
  Y rotation, with `+9999` / `+5555` salts to decorrelate streams.
- `createVolumetricInstances` (line 3165) — Y rotation per layer instance.

The salt convention (`i + 9999`, `i + 5555`, `i + 3333`) is the de-facto
"give me a fresh deterministic stream from the same index" pattern.
Any new helper should follow it. There is **no** seed *parameter* — the
seed is implicitly `0`. Reseeding requires offsetting `i`.

## How orientation rules already flow in

S-004 / T-004-05 already wired per-shape orientation rules:

- `static/app.js:349` `STRATEGY_TABLE` — JS mirror of
  `strategy.go`'s `shapeStrategyTable`. Each shape category has an
  `instance_orientation_rule` of `'random-y' | 'fixed' | 'aligned-to-row'`.
- `static/app.js:365` `shouldRandomRotateInstances()` reads
  `currentSettings.shape_category`, looks up the rule, and returns
  `false` for `'fixed'` and `'aligned-to-row'`. This is the boolean
  fed into `createInstancedFromModel(..., randomRotateY)` and
  `runLodStressTest`.

The acceptance criteria split this binary into three behaviors:

| `shape_category` | rule today | new template behavior |
|---|---|---|
| `round-bush`, `tall-narrow`, `unknown` | `random-y` | random Y per instance |
| `directional`, `planar` | `fixed` / `aligned-to-row` | fixed (or ±5° jitter) |
| `hard-surface` | `fixed` | fixed |

So today's binary `randomRotateY` is sufficient for the AC's two
buckets — but the AC carves out `planar` (`aligned-to-row`) for ±5°
jitter, which the current code does not express. The new framework
needs a tri-state, not a boolean.

`currentSettings.shape_category` is populated when a file is loaded
(see `static/app.js:674` and the classification flow around `static/app.js:761`).
The framework can read it the same way `shouldRandomRotateInstances`
does today.

## UI surface that exists

`static/index.html:65-74` defines `.stress-controls`:

- `#stressCount` — range 1..100 (count)
- `#stressUseLods` — checkbox
- `#lodQuality` — range 0..100, hidden until LOD is on
- `#stressBtn` — Run button

There is **no** template picker today. AC explicitly says template
picking UI is out of scope (T-006-02), but the framework needs a way
to select the active template programmatically. A module-level
`activeTemplate` variable plus a `runStressTest` parameter is the
minimum surface area.

## Constraints inherited from existing code

1. **Spacing must come from `originalModelBBox`**, not the currently
   displayed model. Otherwise switching between regular / billboard
   / volumetric versions changes layout under the user's feet.
2. **InstancedMesh ignores source-mesh transforms** —
   `createInstancedFromModel` (3007-3013) computes a `modelInverse`
   and bakes per-mesh local transforms into each instance matrix.
   Any new helper that builds matrices itself must follow the same
   pattern, OR continue to feed `positions[]` to existing helpers
   and let them keep doing this work.
3. **Per-instance scale is currently `(1,1,1)`** in all three
   `createX...Instances` helpers. Adding scale variation means either
   (a) extending those helpers to accept `InstanceSpec[]` instead of
   `Vector3[]`, or (b) baking scale into a `Matrix4` upstream and
   passing matrices in. Option (a) is a wider change but keeps the
   helpers symmetrical; option (b) is local but duplicates the
   `modelInverse` logic.
4. **`runLodStressTest` distance-buckets positions** by
   `positions[i].length()`. Scattered placements still produce a
   meaningful distance metric, so the existing LOD logic continues
   to work without modification.
5. **Frustum culling is disabled** on every InstancedMesh
   (`frustumCulled = false`). Scattered bounds may grow larger than
   the current grid; the camera-pullback math
   (`camDist = max(gridWidth, gridDepth) * 1.2`) needs to be replaced
   with a bounds-derived value the template can supply.

## What does NOT exist today

- No `SceneTemplate` abstraction; no registry of templates.
- No scatter helpers (`scatterRandomly`, `scatterInRow`).
- No `applyVariation` — scale is always 1, position is exact, rotation
  is binary (random-y or zero).
- No min-distance / Poisson-disk style rejection sampling.
- No "Benchmark" label on the existing grid behavior. The grid IS the
  stress test today, with no name.

## Files in scope

- `static/app.js` — primary surface; "Stress Test" section
  (lines ~2995-3470) plus the toolbar handler block (~3795-3830)
  and module-level state (lines 23-30).
- `static/index.html:65-74` — `.stress-controls`. Untouched by this
  ticket per AC; UI lands in T-006-02.
- `static/style.css` — untouched.

## Out-of-scope but adjacent

- `T-006-02` will add the template picker UI and additional templates.
- `T-006-03` will add the ground plane.
- This ticket explicitly keeps the existing 100x grid behavior alive,
  rebadged as a "Benchmark" template, so T-006-02 can build on top
  without regressing the FPS-measurement workflow.
