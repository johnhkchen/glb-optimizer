# Structure — T-006-01

## Files touched

- **Modify** `static/app.js` — single file, multiple sections.
- **No new files.** The framework lives in a new `// ── Scene
  Templates ──` section inside `app.js`, just above the existing
  `// ── Stress Test ──` section.
- **No HTML/CSS changes.** Picker UI is T-006-02.
- **No Go changes.** This ticket is JS-only.

## Sections in `static/app.js`

### New section: `// ── Scene Templates ──`

Inserted **immediately before** `// ── Stress Test ──` (currently
line ~2995). New code, top-to-bottom:

1. **InstanceSpec factory** — `makeInstanceSpec(position, rotationY, scale)`
   convenience constructor (saves repeating `{position, rotationY,
   scale}` literals).
2. **Scatter helpers**:
   - `scatterRandomly(boundsXZ, count, seed, minDistance)` →
     `Vector3[]`. Rejection sampling, ~30 attempts/instance budget.
   - `scatterInRow(start, end, count, jitter, seed)` →
     `Vector3[]`. Linear interpolation + per-axis jitter.
3. **Variation helpers**:
   - `applyVariation(specs, scaleRange, jitterAmount, seed)` →
     mutates and returns `specs`.
   - `applyOrientationRule(specs, rule, seed)` → mutates and
     returns `specs`. Implements `random-y`, `fixed`, `aligned-to-row`.
4. **Template registry**:
   - `const SCENE_TEMPLATES = { benchmark, 'debug-scatter' }`.
   - `let activeSceneTemplate = 'benchmark'`.
   - `function setSceneTemplate(id)` (also exposed as
     `window.setSceneTemplate` for the manual / future-UI hook).
   - `function getActiveSceneTemplate()`.
5. **Template definitions**:
   - `benchmark` — port of the inline grid math from `runStressTest`.
   - `debug-scatter` — 20 random scatter with ±30% scale.
6. **Bounds helper** — `boundsFromSpecs(specs)` → `{minX, maxX,
   minZ, maxZ, sizeX, sizeZ}` for camera framing fallback.

### Modified section: `// ── Stress Test ──`

#### `createInstancedFromModel(model, count, positions_or_specs, randomRotateY)`

Changed signature semantics: third arg may be `Vector3[]` (legacy)
or `InstanceSpec[]` (new). Detection: `_isSpecArray(arr)` checks
`arr.length === 0 || (arr[0] && arr[0].position && 'rotationY' in
arr[0])`. The fourth parameter (`randomRotateY`) is **only** honored
in the legacy path; spec arrays carry their own rotation per spec
and ignore it.

#### `createBillboardInstances(model, positions_or_specs)`

Same overload. The variant-bucketing (line ~3047) splits an
`InstanceSpec[]` into per-variant `InstanceSpec[]` arrays (not
`Vector3[]`), so each instance keeps its rotation+scale. The
top-quad path uses each spec's `rotationY` instead of the
hardcoded `seededRandom(i + 5555)`.

#### `createVolumetricInstances(model, positions_or_specs, hybridFade)`

Same overload. Per-instance `rotationY` and `scale` from the spec.

#### `runStressTest(count, useLods, quality)`

Replaced grid-building block with template invocation:

```js
const tpl = SCENE_TEMPLATES[activeSceneTemplate] || SCENE_TEMPLATES.benchmark;
const ctx = {
  bbox: originalModelBBox || modelBBox,
  shapeCategory: currentSettings?.shape_category,
  orientationRule: STRATEGY_TABLE[currentSettings?.shape_category]?.instance_orientation_rule || 'random-y',
  seed: 0,
};
const specs = tpl.generate(ctx, count);
const positions = specs.map(s => s.position); // legacy fallback for LOD path
```

Branch ladder unchanged structurally; each branch now passes
`specs` (new path) or `positions` (LOD path until T-006-02 migrates
it).

Camera framing now uses `boundsFromSpecs(specs)` to derive
`gridWidth` / `gridDepth` instead of computing them from the grid
math.

#### `shouldRandomRotateInstances()`

Kept as-is for any external callers; internally `runStressTest` now
gets rotation from the template, not from this helper. (No external
callers found in repo, but the function is small and a name like
"should..." reads as part of the public preview API surface.)

## Public surface added to `window`

- `window.setSceneTemplate(id)` — selects active template by id.
- `window.getActiveSceneTemplate()` — returns the active id.
- `window.__SCENE_TEMPLATES` — read-only registry, for inspection
  in devtools.

These are explicit globals so they survive minification and provide
a stable handle for T-006-02's UI to wire into.

## Ordering of changes (commits)

1. **Add scatter / variation / orientation helpers** (pure
   functions, no consumers yet). Self-contained, easy to review.
2. **Add SceneTemplate registry + benchmark template** (still
   unused — `runStressTest` unchanged).
3. **Add `debug-scatter` template** (still unused).
4. **Wire `runStressTest` to call the active template**, plumb
   `InstanceSpec[]` through the three placement helpers via
   overload, replace inline grid math with `tpl.generate(...)`.
   This is the only commit that can regress existing behavior;
   keeping it isolated makes bisect cheap.
5. **Manual verification + cleanups** (camera framing helper, JSDoc
   comments on the public surface).

Each commit leaves the app in a working state. Commit 4 is the
behavioral change; commits 1-3 are dead code until then.

## Testing strategy

- **No new automated tests.** This codebase has Go tests
  (`*_test.go`) but no JS test infrastructure. Adding Jest / Vitest
  is out of scope and would dwarf the ticket.
- **Manual verification gate** (matches the AC's "manual
  verification" line):
  1. Load `assets/rose/rose.glb` (or whatever the rose path is).
     Confirm `currentSettings.shape_category === 'round-bush'`.
  2. In devtools: `window.setSceneTemplate('debug-scatter')`.
  3. Click Run with count slider ≥ 2. Verify 20 scattered roses
     with visible size variation and per-instance Y rotation.
  4. `window.setSceneTemplate('benchmark')`, Run again — confirm
     the original 100x grid is byte-for-byte the same.
  5. Switch to a `directional` or `hard-surface` asset, run
     `debug-scatter` — instances should all face the same way.
  6. Run an LOD stress test — confirm the LOD path still works
     (it uses the legacy `Vector3[]` path).
- **Tri-state orientation smoke check** for `aligned-to-row`: needs
  a `planar` asset; if none is available, log the limitation in
  `review.md` and rely on T-006-02's row-template ticket to exercise
  the path.

## Public interface diff (textual)

```
+ function makeInstanceSpec(position, rotationY, scale)
+ function scatterRandomly(boundsXZ, count, seed, minDistance)
+ function scatterInRow(start, end, count, jitter, seed)
+ function applyVariation(specs, scaleRange, jitterAmount, seed)
+ function applyOrientationRule(specs, rule, seed)
+ function boundsFromSpecs(specs)
+ const SCENE_TEMPLATES = { benchmark, 'debug-scatter' }
+ let activeSceneTemplate = 'benchmark'
+ function setSceneTemplate(id)
+ function getActiveSceneTemplate()
+ window.setSceneTemplate / getActiveSceneTemplate / __SCENE_TEMPLATES

~ createInstancedFromModel: 3rd arg accepts InstanceSpec[] or Vector3[]
~ createBillboardInstances:  2nd arg accepts InstanceSpec[] or Vector3[]
~ createVolumetricInstances: 2nd arg accepts InstanceSpec[] or Vector3[]
~ runStressTest: builds positions via active template, not inline grid
```

No deletions. `shouldRandomRotateInstances` retained as a thin
public-shaped wrapper.

## Constraints honored

- `originalModelBBox` is the spacing source — every template reads
  it via `ctx.bbox`, never recomputes from `currentModel`.
- `frustumCulled = false` retained on every InstancedMesh.
- `seededRandom` salting convention reused (`seed * <prime> + i`)
  for every new random stream.
- `runLodStressTest` left untouched on this ticket — it still
  receives `Vector3[]`. Listed as known limitation.
