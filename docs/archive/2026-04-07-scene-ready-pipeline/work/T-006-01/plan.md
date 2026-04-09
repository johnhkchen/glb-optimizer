# Plan — T-006-01

## Step 1 — Add scatter / variation / orientation helpers

**Where:** new `// ── Scene Templates ──` section in `static/app.js`,
inserted immediately before `// ── Stress Test ──` (currently
line ~2995, just before `seededRandom`).

**Add:**

- `makeInstanceSpec(position, rotationY = 0, scale = 1)`
- `scatterRandomly(boundsXZ, count, seed, minDistance = 0)`
  - Rejection sampling, attempt budget = `count * 30`.
  - Min-distance check via squared-distance against accepted points.
  - On budget exhaustion, return what was accepted + `console.warn`.
  - All randomness via `seededRandom(seed * 1000 + attempt)`.
- `scatterInRow(start, end, count, jitter = 0, seed = 0)`
  - Linearly interpolates `count` points between `start` and `end`.
  - Per-axis X/Z jitter: `(seededRandom(seed*7 + i*2) - 0.5) * jitter * 2`.
- `applyVariation(specs, scaleRange, jitterAmount, seed = 0)`
  - For each spec: scale = lerp via `seededRandom(seed*7919 + i)`,
    position += jitter via `seededRandom(seed*7919 + i + 1)`.
  - Mutates and returns the array.
- `applyOrientationRule(specs, rule, seed = 0)`
  - `random-y` → `seededRandom(seed*131 + i) * 2π`
  - `fixed` → `0`
  - `aligned-to-row` → `(seededRandom(seed*131 + i) - 0.5) * 0.175` (~±5°)

**Verification:** code compiles (no JS build step — open the page,
check console for errors). Helpers are unused at this step; smoke
test by calling `scatterRandomly({minX:-1,maxX:1,minZ:-1,maxZ:1},
5, 0, 0.1)` from devtools and confirming 5 distinct Vector3 points.

**Commit:** `T-006-01: scene template scatter + variation helpers`

---

## Step 2 — Add SceneTemplate registry + benchmark template

**Add (in the same section):**

- `const SCENE_TEMPLATES = { benchmark: { id, name, generate } }`
- `let activeSceneTemplate = 'benchmark'`
- `function setSceneTemplate(id)` — validates id is in registry,
  warns and no-ops on unknown id.
- `function getActiveSceneTemplate()` — returns the id string.
- `function boundsFromSpecs(specs)` — returns
  `{minX, maxX, minZ, maxZ, sizeX, sizeZ}`; safe on empty array.
- `window.setSceneTemplate = setSceneTemplate` and friends.

**`benchmark.generate(ctx, count)`** — line-for-line port of the
existing inline grid math:

```js
const size = ctx.bbox.size;
const spacing = Math.max(size.x, size.z) * 1.3;
const cols = Math.ceil(Math.sqrt(count));
const rows = Math.ceil(count / cols);
const gridW = cols * spacing, gridD = rows * spacing;
const specs = [];
for (let i = 0; i < count; i++) {
  const r = Math.floor(i / cols), c = i % cols;
  specs.push(makeInstanceSpec(
    new THREE.Vector3(
      c * spacing - gridW/2 + spacing/2,
      0,
      r * spacing - gridD/2 + spacing/2,
    ),
  ));
}
applyOrientationRule(specs, ctx.orientationRule, ctx.seed);
return specs;
```

**Verification:** `window.__SCENE_TEMPLATES.benchmark.generate({
bbox: {size:{x:1,y:1,z:1}}, orientationRule:'random-y', seed:0
}, 4)` returns 4 `InstanceSpec` with the expected 2×2 layout.

**Commit:** `T-006-01: SceneTemplate registry + benchmark template`

---

## Step 3 — Add `debug-scatter` template

**Add:**

```js
SCENE_TEMPLATES['debug-scatter'] = {
  id: 'debug-scatter',
  name: 'Debug Scatter',
  generate(ctx, _count) {
    const size = ctx.bbox.size;
    const span = Math.max(size.x, size.z) * 6;
    const half = span / 2;
    const minDist = Math.max(size.x, size.z) * 1.1;
    const positions = scatterRandomly(
      { minX: -half, maxX: half, minZ: -half, maxZ: half },
      20, ctx.seed, minDist,
    );
    const specs = positions.map(p => makeInstanceSpec(p));
    applyVariation(specs, [0.7, 1.3], 0, ctx.seed);
    applyOrientationRule(specs, ctx.orientationRule, ctx.seed);
    return specs;
  },
};
```

**Verification:** `window.setSceneTemplate('debug-scatter')` then
`window.__SCENE_TEMPLATES['debug-scatter'].generate(ctx, 1)`
returns 20 specs with `scale ∈ [0.7, 1.3]` and rotation per the
ctx orientation rule.

**Commit:** `T-006-01: debug-scatter template`

---

## Step 4 — Wire `runStressTest` to active template

**Modifications:**

1. **Helper overload pattern.** Add a private
   `_isSpecArray(arr)` predicate. In each of:
   - `createInstancedFromModel(model, count, arr, randomRotateY)`
   - `createBillboardInstances(model, arr)`
   - `createVolumetricInstances(model, arr, hybridFade)`

   At the top:

   ```js
   const isSpec = _isSpecArray(arr);
   const getPos = i => isSpec ? arr[i].position : arr[i];
   const getRotY = i => isSpec ? arr[i].rotationY :
                        (randomRotateY ? seededRandom(i) * Math.PI * 2 : 0);
   const getScale = i => isSpec ? arr[i].scale : 1;
   const n = arr.length;
   ```

   Replace inline `dummy.position.copy(positions[i])` /
   `dummy.rotation.set(...)` / `dummy.scale.set(1,1,1)` with the
   getter calls.

   For `createBillboardInstances`'s variant bucketing, change
   `variantPositions[v].push(positions[i])` to push the full spec
   (or wrap a Vector3 into a synthetic spec) so per-instance
   rotation+scale survives the bucket split.

2. **`runStressTest` rewrite (the only behavioral commit).** Replace
   the grid-building block (lines ~3261-3280) with:

   ```js
   const tpl = SCENE_TEMPLATES[activeSceneTemplate] || SCENE_TEMPLATES.benchmark;
   const ctx = {
     bbox: originalModelBBox || modelBBox || { size: new THREE.Box3().setFromObject(currentModel).getSize(new THREE.Vector3()) },
     shapeCategory: currentSettings?.shape_category,
     orientationRule: (STRATEGY_TABLE[currentSettings?.shape_category] || {}).instance_orientation_rule || 'random-y',
     seed: 0,
   };
   const specs = tpl.generate(ctx, count);
   const positions = specs.map(s => s.position);
   const bounds = boundsFromSpecs(specs);
   const gridWidth = bounds.sizeX;
   const gridDepth = bounds.sizeZ;
   ```

   Branch ladder: pass `specs` to the three non-LOD helpers; pass
   `positions` to `runLodStressTest` and `runProductionStressTest`
   (both unchanged on this ticket — they own their own rotation
   logic, and changing them is T-006-02 territory).

3. **Camera framing:** keep the existing
   `camDist = Math.max(gridWidth, gridDepth) * 1.2` line — `gridWidth`
   / `gridDepth` now come from `boundsFromSpecs` instead of the
   removed grid math, so the math is unchanged.

**Verification (manual, blocks the commit):**

1. Reload page. Load any asset. Click Run at default count → must
   look identical to before this ticket (benchmark template).
2. Devtools: `setSceneTemplate('debug-scatter'); ` then click Run
   with count ≥ 2. Expect 20 scattered instances with size
   variation and orientation per the asset's shape category.
3. Toggle LOD checkbox + click Run. Expect the LOD path to still
   work (it uses `positions`, not `specs`).
4. `setSceneTemplate('benchmark')` + Run again. Expect the
   original 100x grid layout.

**Commit:** `T-006-01: route runStressTest through scene template registry`

---

## Step 5 — Cleanup pass

- Add JSDoc-style comments on the public surface
  (`SceneTemplate`, `InstanceSpec`, `setSceneTemplate`).
- Add a one-line console.info on app boot listing available
  templates so future users can discover them without grepping.
- Audit `shouldRandomRotateInstances` call sites — the function is
  no longer needed inside `runStressTest`. If no other call sites
  exist, leave it alone (zero-risk dead code) and document in
  `review.md`. Do not delete on this ticket.

**Verification:** smoke test all three placement modes
(billboard / volumetric / regular) on benchmark + debug-scatter,
plus the LOD path on benchmark. No console errors.

**Commit:** `T-006-01: scene template framework cleanup + docs`

---

## Testing strategy summary

| What | How | When |
|---|---|---|
| Helpers (scatter / variation / orientation) | Devtools console smoke test after Step 1 | Step 1 |
| Benchmark template parity | Visual diff vs. pre-ticket grid | Step 4 |
| Debug-scatter template | Manual: rose + count≥2 → 20 scattered with scale variation | Step 4 |
| LOD path regression | Toggle LOD checkbox, click Run | Step 4 |
| Orientation rule application | Switch to `directional` asset → `debug-scatter` → all instances same Y | Step 4 |
| Production hybrid path | Click Run on a file with billboard+volumetric, default benchmark | Step 5 |

No automated JS tests on this ticket — see structure.md.

## Risks & mitigations

- **Helper overload introduces a runtime branch on every instance
  matrix write.** Negligible at <10k instances; if profiling shows
  it matters, hoist the branch out of the loop in a follow-up.
- **`runProductionStressTest` and `runLodStressTest` don't get
  variation.** Documented limitation; T-006-02 will revisit.
- **`debug-scatter`'s 20-instance hardcoding** ignores the count
  slider. Intentional per the AC ("scatters 20 instances"); the
  benchmark template still honors the slider so the slider isn't
  dead UI.
