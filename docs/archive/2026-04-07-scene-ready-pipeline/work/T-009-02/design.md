# Design — T-009-02 tilted-billboard-loader-and-instances

## Decision summary

1. **New module-level state `tiltedBillboardInstances = []`** mirroring
   `billboardInstances`.
2. **New `createTiltedBillboardInstances(model, arr)`** that mirrors
   `createBillboardInstances` but treats every mesh in the model as a
   side variant (no `billboard_top` carve-out, since the tilted bake
   has no top quad).
3. **New `updateTiltedBillboardFacing()`** that mirrors
   `updateBillboardFacing` and writes per-frame yaw to every instance
   in `tiltedBillboardInstances`. No shared helper.
4. **Animate-loop hook**: extend the existing guard to also call
   `updateTiltedBillboardFacing()` when `tiltedBillboardInstances.length > 0`.
5. **`clearStressInstances` reset**: add `tiltedBillboardInstances = []`
   to the existing reset block.
6. **New LOD toggle button** `data-lod="billboard-tilted"` labelled
   **"Tilted"**, sandwiched between `data-lod="billboard"` and
   `data-lod="volumetric"`. Enabled when `file.has_billboard_tilted` is
   truthy. Click handler reuses the existing `lodToggle` listener loop;
   no new event wiring needed.
7. **`updatePreviewButtons` updates**: add `has_billboard_tilted` to the
   `lodToggle.style.display` predicate, and add a
   `lod === 'billboard-tilted'` arm to the per-button `disabled` switch.
8. **No call to `createTiltedBillboardInstances` from the LOD click
   handler.** Selecting the button just swaps the previewed model via
   `loadModel(/api/preview/.../?version=billboard-tilted)`. The new
   instance helpers exist for T-009-03 to wire into the stress-test
   path; this ticket leaves them dead-code-but-callable.

## Options considered

### Option A — Mirror exactly, no sharing (chosen)

Two parallel functions, two parallel state arrays, two parallel
animate-loop calls. Trivial to read; trivial to delete one of them
later if the tilted bake supersedes the horizontal one (or vice
versa).

**Pros**
- Zero risk of regressing the horizontal path. Every existing call
  site keeps its existing behaviour bit-for-bit.
- Matches the ticket's "First-Pass Scope" instruction explicitly.
- T-009-03 (the crossfade) will need both paths to coexist briefly
  anyway, so factoring them into one helper now would just be undone
  in two tickets' time.

**Cons**
- Code duplication (~30 lines of `createBillboardInstances` body, ~10
  lines of `updateBillboardFacing` body). Mitigated by both being
  short, mechanical, and unlikely to drift in the lifetime of this
  feature.

### Option B — Parameterise the existing functions

Add an `opts = { state, seedOffset, hasTopQuad }` argument to
`createBillboardInstances` and `updateBillboardFacing`.

**Pros**
- No duplication.

**Cons**
- Every existing call site has to be touched (4 in `runStressTest`,
  `runProductionStressTest`, `runLodStressTest`, plus the animate
  loop). Each touch is a chance to break the horizontal path.
- The "T-009-03 will redo this" argument applies again. The shared
  signature would have to grow further to support the three-way
  crossfade, at which point we will probably want to refactor
  anyway.
- Violates the ticket's explicit "do not try to share more than
  necessary between the two" instruction.

**Rejected.**

### Option C — Generic "billboard family" registry

Replace the per-flavour state arrays with a single
`Map<flavour, BillboardSet[]>` and iterate generically in the
animate loop and in `clearStressInstances`.

**Pros**
- Scales to N billboard types.

**Cons**
- Big-bang refactor; out of scope; would require touching the
  horizontal and volumetric paths for the sake of a feature whose
  long-term shape is still unsettled.

**Rejected.**

## Selected approach in detail

### State and helpers (additions only, no modifications to existing
billboard code)

```js
// Tilted-camera billboard instances (T-009-02). Side-only — the
// tilted bake has no `billboard_top` quad. Camera-facing in yaw,
// same as `billboardInstances`; the tilt is baked into the texture.
let tiltedBillboardInstances = []; // { mesh: InstancedMesh, positions: Vector3[] }

function createTiltedBillboardInstances(model, arr) {
    // Tilted bake from T-009-01 contains only side quads (no
    // `billboard_top`). All meshes in the model are side variants.
    const sideQuads = [];
    model.traverse((child) => { if (child.isMesh) sideQuads.push(child); });
    if (sideQuads.length === 0) return [];

    const isSpec = _isSpecArray(arr);
    const specs = isSpec
        ? arr
        : arr.map((p, i) => makeInstanceSpec(
            p, seededRandom(i + 5555) * Math.PI * 2, 1));

    const numVariants = sideQuads.length;
    const variantSpecs = Array.from({ length: numVariants }, () => []);
    for (let i = 0; i < specs.length; i++) {
        // Distinct seed offset from the horizontal loader (+9999) so
        // T-009-03 can run both at the same positions without their
        // variant assignments colliding into a visible pattern.
        const variant = Math.floor(seededRandom(i + 7777) * numVariants);
        variantSpecs[variant].push(specs[i]);
    }

    const created = [];
    const dummy = new THREE.Object3D();

    for (let v = 0; v < numVariants; v++) {
        const bucket = variantSpecs[v];
        if (bucket.length === 0) continue;

        const quad = sideQuads[v];
        const geom = quad.geometry.clone();
        const mat = quad.material.clone();
        mat.depthWrite = true;
        mat.alphaTest = 0.5;
        mat.transparent = false;
        const instancedMesh = new THREE.InstancedMesh(geom, mat, bucket.length);
        instancedMesh.frustumCulled = false;

        for (let i = 0; i < bucket.length; i++) {
            dummy.position.copy(bucket[i].position);
            const s = bucket[i].scale;
            dummy.scale.set(s, s, s);
            dummy.updateMatrix();
            instancedMesh.setMatrixAt(i, dummy.matrix);
        }
        instancedMesh.instanceMatrix.needsUpdate = true;

        scene.add(instancedMesh);
        created.push(instancedMesh);
        tiltedBillboardInstances.push({
            mesh: instancedMesh,
            positions: bucket.map(s => s.position),
        });
    }

    updateTiltedBillboardFacing();
    return created;
}

function updateTiltedBillboardFacing() {
    if (tiltedBillboardInstances.length === 0) return;
    const camPos = camera.position;
    const dummy = new THREE.Object3D();

    for (const { mesh, positions } of tiltedBillboardInstances) {
        for (let i = 0; i < positions.length; i++) {
            const pos = positions[i];
            dummy.position.copy(pos);
            dummy.rotation.set(0, Math.atan2(camPos.x - pos.x, camPos.z - pos.z), 0);
            dummy.scale.set(1, 1, 1);
            dummy.updateMatrix();
            mesh.setMatrixAt(i, dummy.matrix);
        }
        mesh.instanceMatrix.needsUpdate = true;
    }
}
```

### Animate-loop integration

Existing block at `app.js:~3338`:

```js
if (stressActive && (billboardInstances.length > 0 || billboardTopInstances.length > 0)) {
    updateBillboardFacing();
    updateBillboardVisibility();
}
```

becomes:

```js
if (stressActive && (billboardInstances.length > 0 || billboardTopInstances.length > 0)) {
    updateBillboardFacing();
    updateBillboardVisibility();
}
if (stressActive && tiltedBillboardInstances.length > 0) {
    updateTiltedBillboardFacing();
}
```

Two separate guards instead of widening the existing one — keeps the
horizontal `updateBillboardVisibility` from being called when only
tilted instances are active (the visibility helper assumes a
horizontal+top crossfade pair, which the tilted path does not have).

### `clearStressInstances` reset

Add `tiltedBillboardInstances = [];` next to the existing
`billboardInstances = []` line at `app.js:~3375`.

### `updatePreviewButtons`

Two changes at `app.js:~4464` and `~4470`:

```js
// (1) Visibility predicate — add the tilted flag.
lodToggle.style.display =
    hasLods || hasVlods ||
    (file && (file.has_billboard || file.has_billboard_tilted || file.has_volumetric))
    ? 'flex' : 'none';

// (2) Per-button disabled switch — add a tilted arm.
} else if (lod === 'billboard-tilted') {
    btn.disabled = !(file && file.has_billboard_tilted);
} else if (lod === 'volumetric') {
    ...
```

### `index.html`

Insert one line after the existing `data-lod="billboard"` button at
`static/index.html:45`:

```html
<button data-lod="billboard"        disabled>Camera-facing</button>
<button data-lod="billboard-tilted" disabled>Tilted</button>
<button data-lod="volumetric"       disabled>Dome slices</button>
```

### LOD click handler — does it need a special case?

The existing handler at `app.js:~4543`:

```js
let fileSize = 0;
if (version === 'billboard' || version === 'volumetric' || version === 'production') {
    fileSize = 50000;
} else if (version.startsWith('vlod')) { ... }
else { ... /* lod0..lod3 */ }
const previewVer = version === 'production' ? 'volumetric' : version;
loadModel(`/api/preview/${selectedFileId}?version=${previewVer}&t=${Date.now()}`, fileSize);
```

For `version === 'billboard-tilted'`:

- `fileSize` falls into the first branch — extend the predicate to
  include `'billboard-tilted'` so it picks the 50000 estimate instead
  of crashing in the `vlod`/`lod` integer parse.
- `previewVer` mapping: pass `billboard-tilted` straight through
  (the backend's `handlePreview` already handles it). No need to
  add the production-style alias.

```js
if (version === 'billboard' || version === 'billboard-tilted'
    || version === 'volumetric' || version === 'production') {
    fileSize = 50000;
}
```

## Out of scope (deferred to T-009-03)

- The three-stage crossfade between horizontal billboards, tilted
  billboards, and dome slices.
- Wiring `createTiltedBillboardInstances` into `runStressTest` /
  `runProductionStressTest`.
- "Prepare for scene" baking the tilted variant.
- Any settings UI for the tilt elevation.

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| Tilted bake from T-009-01 actually has a `billboard_top` quad I missed | Re-checked `renderTiltedBillboardGLB` at `app.js:1846-1880` — only `billboard_${i}` side quads, no top. Confirmed. |
| Animate loop runs `updateBillboardVisibility` against tilted instances and breaks them | The new animate-loop block keeps the visibility call gated on `billboardInstances` only. Tilted has its own gate. |
| `clearStressInstances` is missed | Verified by reading the ticket line ("clearStressInstances resets tiltedBillboardInstances = []") — explicit acceptance criterion. |
| The existing `updatePreviewButtons` `disabled` loop doesn't recognise the new `data-lod` and silently leaves it enabled | Adding the explicit `else if (lod === 'billboard-tilted')` arm prevents fallthrough into the `lod.startsWith('lod')` integer parse, which would otherwise produce `NaN` and a confusing disabled state. |
