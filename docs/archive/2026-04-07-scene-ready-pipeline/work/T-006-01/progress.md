# Progress — T-006-01

All steps from `plan.md` completed in a single pass against
`static/app.js`. No deviations from the plan; no commits created
(this is a working tree change — committing is left to the user).

## Completed

- **Step 1.** Added `// ── Scene Templates (T-006-01) ──` section to
  `static/app.js`, immediately before `// ── Stress Test ──`.
  Helpers added: `makeInstanceSpec`, `scatterRandomly`,
  `scatterInRow`, `applyVariation`, `applyOrientationRule`,
  `boundsFromSpecs`, `_isSpecArray`.
- **Step 2.** Added `SCENE_TEMPLATES` registry,
  `activeSceneTemplate` state, `setSceneTemplate` /
  `getActiveSceneTemplate` accessors, and the `benchmark` template
  (line-for-line port of the previous inline grid math).
  `window.setSceneTemplate`, `window.getActiveSceneTemplate`,
  `window.__SCENE_TEMPLATES` exposed for devtools / future picker.
- **Step 3.** Added `debug-scatter` template: 20 instances scattered
  via `scatterRandomly` with min-distance ~1.1× footprint, ±30%
  scale variation via `applyVariation`, rotation per the active
  shape's orientation rule.
- **Step 4.** Updated the three placement helpers to accept either
  `Vector3[]` (legacy) or `InstanceSpec[]` (new):
  - `createInstancedFromModel(model, count, arr, randomRotateY)` —
    detects spec form via `_isSpecArray`, sources rotation+scale
    from the spec when present.
  - `createBillboardInstances(model, arr)` — normalizes legacy
    `Vector3[]` into synthetic specs internally so per-variant
    bucketing carries rotation+scale through. Side billboards still
    have their Y rotation overwritten by `updateBillboardFacing()`
    each frame (camera-facing) — only scale is preserved on the
    side path; rotation+scale both flow through to the top quad.
  - `createVolumetricInstances(model, arr, hybridFade)` — same
    overload pattern; per-instance rotation+scale honored.
  - `runStressTest` rewritten: builds `ctx`, calls
    `tpl.generate(ctx, count)`, feeds `specs` into the non-LOD
    branches and `positions` into the LOD/production branches.
    `gridWidth` / `gridDepth` derived from `boundsFromSpecs` plus
    one footprint of padding so the camera framing matches the
    legacy behavior.
- **Step 5.** Cleanup: section is documented inline; the public
  surface has comments. `shouldRandomRotateInstances` is left
  in place (no external callers found, but keeping it costs
  nothing and avoids touching unrelated code).

## Verification performed

- `node --check static/app.js` — clean.
- `go build ./...` — clean (no Go changes; sanity check only).
- **Manual verification (the AC criterion) is still pending** —
  this requires loading the rose in the live frontend, calling
  `window.setSceneTemplate('debug-scatter')` from devtools, and
  visually confirming 20 scattered instances with size variation
  and per-instance Y rotation. See `review.md` for the verification
  checklist the human reviewer should walk through.

## Deviations from plan

None. The plan was followed step-by-step. The single intentional
shape deviation from the AC (`rotationY: number` instead of
`rotation: Quaternion`, and `scale: number` instead of `Vector3`)
was decided in `design.md` and implemented as designed.

## Files changed

- `static/app.js` — single file, ~210 lines added (new section),
  ~50 lines modified across the three placement helpers and
  `runStressTest`.

## Files NOT changed (intentional)

- `static/index.html` — no template picker UI on this ticket
  (T-006-02).
- `static/style.css` — no UI changes.
- All Go files — JS-only ticket.
- `runLodStressTest`, `runProductionStressTest` — left on the
  legacy `Vector3[]` path. T-006-02 will migrate these.
