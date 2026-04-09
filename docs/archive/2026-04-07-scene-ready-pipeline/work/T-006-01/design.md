# Design — T-006-01: scene template framework + variation

## Decision summary

Add a **section in `static/app.js`** (not a new module) that introduces:

1. A `SceneTemplate` registry — plain JS objects with
   `{id, name, generate(asset, count) → InstanceSpec[]}`.
2. Scatter helpers — `scatterRandomly`, `scatterInRow`,
   `applyVariation` — operating on `InstanceSpec[]`.
3. A new `runStressTest` orchestration that calls the active
   template's `generate(...)` and feeds the resulting `InstanceSpec[]`
   into the existing `createInstancedFromModel` /
   `createBillboardInstances` / `createVolumetricInstances` helpers,
   which gain a unified spec-aware code path.
4. Two registered templates:
   - `benchmark` — the existing 100x perfect grid (preserved verbatim).
   - `debug-scatter` — 20 instances scattered with size + position
     variation, used to validate the framework.

The active template defaults to `benchmark` so existing behavior is
unchanged when nothing else selects a template. `debug-scatter` is
selectable via a temporary `window.__sceneTemplate = 'debug-scatter'`
console hook (no UI per AC; UI lands in T-006-02).

## Why a section in `app.js` and not `scene_templates.js`

The AC explicitly allows either ("New JS module `scene_templates.js`
*or* section in `app.js`"). The section approach wins on the
following grounds, all rooted in the research:

- **No bundler.** `index.html` loads `app.js` as a plain
  `<script type="module">` (or classic script — see
  `static/index.html`). A new file means another `<script>` tag,
  another import path, and either ES module imports or polluting
  `window`. The existing codebase is one big `app.js`; following
  that convention avoids the "first new file" tax.
- **Tight coupling to `app.js` globals.** Templates need
  `originalModelBBox`, `currentSettings.shape_category`,
  `STRATEGY_TABLE`, `seededRandom`, and the
  `createX...Instances` helpers. Splitting the file forces all of
  these into exports / a shared module — a refactor far larger than
  this ticket warrants.
- **First-pass scope.** The ticket's first-pass scope says "get the
  framework right; templates and UI come next." A section is
  cheaper to iterate on; T-006-02 can extract a module if the
  surface area justifies it.

A new module was rejected because it would force either a global
namespace dance or a much wider refactor of `app.js`'s 3,800-line
single-file architecture, with no offsetting benefit at this stage.

## InstanceSpec shape

```js
// InstanceSpec — one entry per instance the template wants placed.
// position: THREE.Vector3 (world-space, Y is up)
// rotationY: number (radians; full quaternion is overkill — every
//   existing helper only writes Y rotation, and our orientation
//   rules are all Y-axis. Using a scalar keeps the spec light and
//   matches the existing dummy.rotation.set(0, y, 0) pattern.)
// scale: number (uniform; per-axis scale would force a wider rewrite
//   of helpers that today call dummy.scale.set(1,1,1).)
{ position, rotationY, scale }
```

**Quaternion vs. scalar Y.** The AC literally says
`rotation: Quaternion`. I am deviating: scalar `rotationY` matches
how every existing helper writes its instance matrices
(`dummy.rotation.set(0, y, 0)`), avoids dragging in
`THREE.Quaternion` allocations on the hot path, and is trivially
upgradable later (a future template that needs full 3-axis rotation
can switch to a quaternion field — converting scalar→quat is
mechanical). I will document the deviation in `review.md` so the
reviewer can flag it if they disagree.

**Uniform vs. per-axis scale.** Same reasoning. Plants don't need
non-uniform scale; if we ever do, the field becomes a Vector3 and
the helpers update once.

## SceneTemplate interface

```js
// id:    stable string used by future picker UI / persistence
// name:  human-readable label for the picker
// generate(ctx, count) → InstanceSpec[]
//
// ctx is a small bag of inputs the template needs:
//   { bbox, shapeCategory, orientationRule, seed }
// — bbox is originalModelBBox (or modelBBox fallback)
// — shapeCategory is currentSettings.shape_category
// — orientationRule is STRATEGY_TABLE[cat].instance_orientation_rule
// — seed is an integer; deterministic per template invocation
//
// `count` is the user-requested instance count from the slider.
// Templates may honor or override it (debug-scatter ignores count
// and produces its own fixed 20-instance scatter, since the AC says
// "scatters 20 instances").
```

Passing a `ctx` object instead of a raw `asset` reference (as the AC
literally says) keeps templates pure and testable: they don't
traverse `THREE.Object3D` graphs, they only consume metadata. The
helpers that *do* need the asset (`createInstancedFromModel` etc.)
still receive `currentModel` directly from `runStressTest`.

## Scatter helpers

### `scatterRandomly(boundsXZ, count, seed, minDistance)`

- `boundsXZ`: `{minX, maxX, minZ, maxZ}` rectangle.
- Returns `Vector3[]` (Y=0; templates lift to InstanceSpec).
- **Min-distance constraint:** rejection sampling with a per-call
  budget of `count * 30` attempts. If we exhaust the budget, we
  return whatever we have and log a console.warn — degrading
  gracefully is more useful than throwing in a debug-scatter call.
- **Determinism:** uses `seededRandom(seed * 1000 + i)` so seed `0`
  gives the same scatter every run, and bumping seed reshuffles.

Rejected: full Poisson-disk sampling (Bridson). Overkill for the
debug template, more code, no visible quality difference at 20
instances. Rejection sampling is well-known to degrade past ~70%
density; we will be at <10%.

### `scatterInRow(start, end, count, jitter, seed)`

- `start`, `end`: `Vector3`.
- Linearly interpolates `count` points; adds per-axis jitter
  (`±jitter * spacing`) using two seeded random streams.
- Returns `Vector3[]`. Used by future row-planted templates
  (T-006-02); included now because the AC requires it and because
  exercising the helper from `debug-scatter`'s tests catches drift.

### `applyVariation(specs, scaleRange, jitterAmount, seed)`

- `scaleRange`: `[min, max]`. Each spec's scale becomes
  `lerp(min, max, seededRandom(seed*7919 + i))`.
- `jitterAmount`: number. Adds X/Z jitter to each spec's position
  (Y untouched — instances stay on the ground plane).
- Mutates and returns the array (saves an allocation; templates
  always own the array they pass in).

## Orientation rule application

A small helper, `applyOrientationRule(specs, rule, seed)`, sets
each spec's `rotationY` based on the per-shape rule:

- `'random-y'` → `seededRandom(seed*131 + i) * 2π`
- `'fixed'` → `0`
- `'aligned-to-row'` → `0` plus ±5° (≈0.087 rad) jitter via
  `(seededRandom(seed*131 + i) - 0.5) * 0.175`. The AC explicitly
  calls out the ±5° jitter for `directional` / `planar`.

Today's `shouldRandomRotateInstances()` becomes a thin wrapper over
this (kept for back-compat with any other call sites — there is
exactly one: `runStressTest`).

## Helper changes: spec-aware code path

The three placement helpers gain an `applySpecsToInstanceMatrices`
internal: given `InstanceSpec[]`, fill `dummy.position`,
`dummy.rotation.set(0, spec.rotationY, 0)`,
`dummy.scale.setScalar(spec.scale)`. Each helper currently iterates
positions and uses `dummy.rotation.set(0, randomY ? ... : 0, 0)` /
`dummy.scale.set(1,1,1)`. The minimal change is:

- Add an overload: helpers accept either `Vector3[]` (legacy code
  path; existing call sites in `runLodStressTest` still pass these)
  *or* `InstanceSpec[]`. Detection is `arr[0]?.position instanceof
  THREE.Vector3`.
- This avoids touching `runLodStressTest`'s distance-bucket logic
  (which still emits raw positions per LOD bucket) on this ticket.
  T-006-02 / T-006-03 can migrate LOD pathways later.

Rejected alternative: make every helper take only `InstanceSpec[]`
and rewrite `runLodStressTest`. Larger blast radius, more risk of
breaking the LOD path which is exercised by the production hybrid
flow today.

## Template registry + selection

```js
const SCENE_TEMPLATES = {
  benchmark:     { id, name, generate },
  'debug-scatter': { id, name, generate },
};
let activeSceneTemplate = 'benchmark';
```

`runStressTest` looks up `SCENE_TEMPLATES[activeSceneTemplate]`,
calls `generate(ctx, count)`, then routes the resulting specs into
the same branch ladder as today (billboard / volumetric / production
/ regular / LOD).

Selection is a temporary `window.setSceneTemplate(id)` (and
mirrored `window.__sceneTemplate` getter) for now. T-006-02 will
replace this with a real picker. The hook is documented in
`review.md`'s "Open Concerns" so it doesn't get forgotten.

## What `benchmark` does

`benchmark.generate(ctx, count)` reproduces the existing grid
exactly: `cols = ceil(sqrt(count))`, `spacing = max(size.x,
size.z) * 1.3`, centered on origin, every spec with `scale=1` and
`rotationY` derived from the orientation rule (matching today's
behavior). This is a near-line-for-line port of `static/app.js:3270-3280`,
just expressed as a template instead of inlined.

## What `debug-scatter` does

```js
generate(ctx, _count) {
  const size = ctx.bbox.size;
  const span = Math.max(size.x, size.z) * 6; // ~6× model footprint
  const half = span / 2;
  const positions = scatterRandomly(
    { minX: -half, maxX: half, minZ: -half, maxZ: half },
    20, ctx.seed, Math.max(size.x, size.z) * 1.1, // min-distance
  );
  const specs = positions.map(p => ({
    position: p, rotationY: 0, scale: 1,
  }));
  applyVariation(specs, [0.7, 1.3], 0, ctx.seed); // scale only
  applyOrientationRule(specs, ctx.orientationRule, ctx.seed);
  return specs;
}
```

Twenty instances, ±30% scale variation, rotation per the active
shape's orientation rule. No additional jitter (scatter already
randomized positions). Manual verification path: load the rose,
set `window.setSceneTemplate('debug-scatter')`, click Run with
`count >= 2` — see scattered bushes with size variation and random
Y rotations.

## Camera framing

Today's `camDist = max(gridWidth, gridDepth) * 1.2` only works when
the template lays out a known rectangle. The new contract: each
template can return `{specs, bounds}` where `bounds` is an
optional `{minX, maxX, minZ, maxZ}`. If omitted, `runStressTest`
falls back to computing AABB from the specs themselves
(`min/max` over `spec.position.x/z` plus a `size` margin).

## Risks / things this design intentionally does not solve

- **No quaternion support.** Scalar `rotationY` only. Documented
  deviation from AC.
- **No multi-asset scenes.** Out of scope per ticket.
- **No persistence of template selection.** Lives only in
  `window.__sceneTemplate` until T-006-02.
- **`runLodStressTest` still uses raw `Vector3[]` positions** —
  scale variation does not propagate to the LOD path on this
  ticket. Documented as a known limitation in `review.md`.
- **Min-distance rejection sampling** can degrade if a future
  template asks for high density; today's debug template is well
  inside the safe regime.
