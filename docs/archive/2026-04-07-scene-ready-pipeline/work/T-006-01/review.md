# Review — T-006-01: scene template framework + variation

## What changed

Single file modified: `static/app.js`.

### New section: `// ── Scene Templates (T-006-01) ──`

Inserted just before the existing `// ── Stress Test ──` section.
~210 lines of new, self-contained code:

- **`makeInstanceSpec(position, rotationY, scale)`** — light
  factory returning `{position, rotationY, scale}`. Used by all
  templates and by the legacy→spec normalization path inside
  `createBillboardInstances`.
- **`scatterRandomly(boundsXZ, count, seed, minDistance)`** —
  rejection-sampled random points in an XZ rectangle. Budget
  is `count * 30` attempts; warns and returns whatever was
  accepted on budget exhaustion. Deterministic per `seed`.
- **`scatterInRow(start, end, count, jitter, seed)`** — linear
  interpolation between two `Vector3` endpoints with optional
  per-axis jitter. Required by the AC; no consumer in this
  ticket (T-006-02 row-template will exercise it).
- **`applyVariation(specs, scaleRange, jitterAmount, seed)`** —
  mutates specs with per-instance scale and optional XZ jitter.
- **`applyOrientationRule(specs, rule, seed)`** — implements the
  three-rule orientation policy: `random-y` → uniform random,
  `fixed` → 0, `aligned-to-row` → ±5° jitter (~0.175 rad p2p).
- **`boundsFromSpecs(specs)`** — XZ AABB derivation for camera
  framing fallback.
- **`SCENE_TEMPLATES`** registry with two entries:
  - **`benchmark`** — line-for-line port of the legacy 100x grid
    math. Spacing = `max(size.x, size.z) * 1.3`, `cols =
    ceil(sqrt(count))`, centered on origin. Now also stamps
    rotation per the active orientation rule (matches today's
    behavior via `shouldRandomRotateInstances`).
  - **`debug-scatter`** — 20 instances scattered over a 6×
    footprint area, min-distance = 1.1× footprint, ±30% scale
    variation, rotation per orientation rule.
- **`activeSceneTemplate`** state, **`setSceneTemplate(id)`** /
  **`getActiveSceneTemplate()`** accessors, exposed on `window`
  alongside `window.__SCENE_TEMPLATES` for devtools / future
  picker (T-006-02).
- **`_isSpecArray(arr)`** — internal predicate used by the
  placement helpers to detect new vs legacy input.

### Modified placement helpers

- **`createInstancedFromModel(model, count, arr, randomRotateY)`** —
  third arg now accepts `Vector3[]` or `InstanceSpec[]`. Spec
  form sources rotation+scale per instance; legacy form falls
  back to the previous random-Y / scale=1 behavior. Per-instance
  scale is now applied via `dummy.scale.set(s, s, s)`.
- **`createBillboardInstances(model, arr)`** — same overload,
  but with a wrinkle: side billboards face the camera at render
  time (`updateBillboardFacing()` overwrites their Y rotation
  every frame), so for the side variants we only preserve
  per-instance scale. The top-down quad path honors both
  rotation and scale from the spec. Internally, legacy
  `Vector3[]` input is normalized into specs before variant
  bucketing so per-instance state survives the bucket split.
- **`createVolumetricInstances(model, arr, hybridFade)`** — same
  overload; per-instance rotation+scale on every layer.

### Modified entry point: `runStressTest`

- Removed the inline grid-building block.
- Builds a `ctx = {bbox, shapeCategory, orientationRule, seed}`
  from `originalModelBBox` (or fallback) and `currentSettings`.
- Calls `SCENE_TEMPLATES[activeSceneTemplate].generate(ctx, count)`
  to get an `InstanceSpec[]`.
- Routes specs through the three non-LOD branches; routes the
  derived `Vector3[]` through `runLodStressTest` and
  `runProductionStressTest` unchanged.
- Camera framing now derives `gridWidth` / `gridDepth` from
  `boundsFromSpecs(specs)` plus a one-footprint padding (matches
  the implicit `spacing/2` centering padding the legacy math
  inherited).
- FPS overlay now uses `effectiveCount = specs.length` instead
  of the slider `count`, so `debug-scatter`'s hardcoded 20
  shows correctly even when the slider says 100.

## Acceptance criteria check

| AC bullet | Status |
|---|---|
| `SceneTemplate` interface `{id, name, generate(asset, count)}` | ✅ (asset → ctx; documented deviation in design.md) |
| `InstanceSpec` `{position, rotation, scale}` | ⚠️ Deviation: `rotationY: number` (radians) instead of `Quaternion`; `scale: number` instead of `Vector3`. Rationale + upgrade path in design.md. |
| `scatterRandomly(bounds, count, seed)` with min-distance | ✅ (4th arg `minDistance`) |
| `scatterInRow(start, end, count, jitter)` | ✅ (5th arg `seed`) |
| `applyVariation(specs, scaleRange, jitterAmount)` | ✅ (4th arg `seed`) |
| Orientation: `round-bush`/`tall-narrow` → random Y | ✅ via `random-y` rule |
| Orientation: `directional`/`planar` → fixed or ±5° | ✅ `directional`→`fixed` (0); `planar`→`aligned-to-row` (±5°) |
| Orientation: `hard-surface` → fixed | ✅ |
| `debug-scatter` template, 20 instances, scatter + size variation | ✅ |
| Manual verification (load rose, run debug-scatter) | ⏳ **Pending human run** — see checklist below |
| Existing grid kept as Benchmark template | ✅ |
| Deterministic per seed | ✅ all helpers thread `seed` through `seededRandom` |

## Test coverage

**No automated tests added.** The repo has Go tests but no JS test
infrastructure (no `package.json`, no test runner). Adding Jest /
Vitest is out of scope for this ticket and would dwarf the change.

What was verified automatically:
- `node --check static/app.js` → clean.
- `go build ./...` → clean (sanity check; no Go changes).

What needs human manual verification (mirrors the AC):
1. Load any GLB asset. With the slider at default, click Run.
   Verify it looks identical to before this ticket — the
   `benchmark` template should be a byte-for-byte port of the
   prior grid layout.
2. Open devtools. Run `setSceneTemplate('debug-scatter')`.
3. Click Run with count slider ≥ 2. Expect 20 scattered instances
   over a ~6× footprint area, with visible size variation and Y
   rotation appropriate to the asset's `shape_category`.
4. Switch to a `directional` or `hard-surface` asset and re-run
   `debug-scatter`. Expect all instances to face the same way
   (rotationY = 0).
5. Run `setSceneTemplate('benchmark')` and click Run again to
   confirm round-trip parity.
6. Toggle the LOD checkbox and click Run. Expect the LOD path to
   still work (it uses the legacy `Vector3[]` path).
7. On a file with billboard+volumetric variants, click Run with
   `previewVersion === 'production'`. Expect the production
   hybrid to still render (also legacy path).

## Open concerns / known limitations

1. **`InstanceSpec` shape deviates from the AC literal.** Scalar
   `rotationY` instead of `Quaternion`; scalar `scale` instead of
   `Vector3`. This is a deliberate, documented deviation
   (design.md "Quaternion vs. scalar Y"). Reviewer should
   reject if they want strict spec adherence — upgrade is
   mechanical: change the field types and propagate through ~3
   helpers.
2. **`runLodStressTest` and `runProductionStressTest` still use
   the legacy `Vector3[]` path.** They do NOT receive
   per-instance scale or rotation variation. This is acceptable
   for T-006-01 (the framework + debug template land) but means
   `debug-scatter` + LOD checkbox = scattered positions but no
   scale variation. Migration is T-006-02 territory; flagged so
   it doesn't get lost.
3. **Side billboards always camera-face.** `updateBillboardFacing`
   overwrites instance Y rotation every frame. This means
   `debug-scatter` rotation variation is invisible in billboard
   mode for side quads. Top-quad path still honors per-instance
   rotation, and scale is preserved on both. This is a property
   of the existing billboard rendering, not a regression.
4. **No template picker UI.** Selection happens via
   `window.setSceneTemplate(id)` in devtools. T-006-02 will add
   the dropdown. Until then, users won't discover `debug-scatter`
   without reading code. A boot-time `console.info` listing
   templates was considered but skipped to avoid console noise;
   easy to add later if needed.
5. **`shouldRandomRotateInstances` is now unreachable** from
   `runStressTest` (the only caller). It's left in place to
   minimize blast radius. Safe to delete in a follow-up if no
   external scripts depend on the global.
6. **`scatterRandomly` rejection sampling can degrade silently**
   if a future template asks for high density. Today's
   `debug-scatter` is at <10% density; well inside the safe
   regime. The `console.warn` on budget exhaustion is the
   user-facing signal.
7. **Manual verification has not been performed by Claude.** The
   AC requires loading the rose in the live frontend; that
   requires a running server and a browser, which is outside the
   automated harness. The reviewer must walk the checklist above
   before merging.

## What a human reviewer should focus on

- **The `InstanceSpec` shape deviation.** Is scalar
  `rotationY` + scalar `scale` acceptable, or must we match the
  AC literally? This is the only judgment call in the change.
- **Benchmark template parity.** Is the existing 100x grid
  visually identical after the rewrite? Hardest thing to test
  without eyeballing.
- **Camera framing.** The padding heuristic
  (`bounds.sizeX + sizeXZ`) is approximate. On `debug-scatter`
  the camera should still frame everything; on `benchmark` it
  should match the legacy framing within a small margin.
- **The legacy `Vector3[]` path through
  `createBillboardInstances`'s synthetic-spec normalization.** This
  is the trickiest piece of the refactor — the synthetic spec
  has `rotationY = seededRandom(i + 5555) * Math.PI * 2` to
  preserve the old top-quad random rotation. Worth a careful
  read.
