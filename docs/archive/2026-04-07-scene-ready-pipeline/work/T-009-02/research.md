# Research — T-009-02 tilted-billboard-loader-and-instances

## Goal of this phase

Understand exactly how the existing horizontal billboard loader / instance
path works so the new tilted loader can mirror it without sharing more than
the ticket asks for. Also: confirm the runtime contract that T-009-01
actually shipped (which differs in one important way from the ticket's
optimistic assumption).

## What T-009-01 actually shipped

| Concern | What the T-009-02 ticket assumes | What T-009-01 actually shipped |
|---|---|---|
| Quad naming in the tilted GLB | `tilted_billboard_${i}` | **`billboard_${i}`** — same as the horizontal bake. The T-009-01 owner kept the legacy name and explicitly noted in `review.md` §"Open concerns" #4 that the runtime loader (i.e. this ticket) should "discriminate by file path, not by quad name." |
| Top quad | (not specified) | **No `billboard_top` quad.** Side variants only. |
| File on disk | `{outputs}/{id}_billboard_tilted.glb` | Same. |
| FileRecord flag | `has_billboard_tilted` (JSON) / `HasBillboardTilted` (Go) | Same. `omitempty`. Set both at upload time and by `scanExistingFiles` on startup. |
| Preview endpoint | `/api/preview/:id?version=billboard-tilted` | Same. |
| Devtools entry point | `await generateTiltedBillboard(selectedFileId)` | Same. `window.generateTiltedBillboard` and a `window.selectedFileId` getter are exposed in `static/app.js:1885-1891`. |

**Implication for this ticket:** the new `createTiltedBillboardInstances`
must NOT look for quads named `tilted_billboard_${i}`. It must look for
the same `billboard_${i}` family the horizontal loader uses, and rely on
the caller having loaded the correct file (the `?version=billboard-tilted`
URL) to know which set of quads it is dealing with. The loader is
file-discriminated, not name-discriminated.

## Existing horizontal billboard runtime path

All in `static/app.js`. Line numbers are current as of this commit.

### State (top of the billboard section, ~line 3787)

```js
let billboardInstances = []; // { mesh: InstancedMesh, positions: Vector3[] }
let billboardTopInstances = []; // { mesh: InstancedMesh }
```

These are module-level `let`s, reset by `clearStressInstances` (line ~3369)
back to `[]` on every preview swap.

### `createBillboardInstances(model, arr)` (line ~3794)

1. Walks `model.traverse`, collecting every mesh into `sideQuads`, except
   the one named `billboard_top` which it pulls aside as `topQuad`.
2. Normalises the second arg to an `InstanceSpec[]` (T-006-01: callers may
   still pass `Vector3[]`).
3. Buckets each spec into one of `numVariants = sideQuads.length` buckets,
   using `seededRandom(i + 9999)` so the assignment is stable per-index.
4. For each non-empty bucket, clones the variant's geometry and material,
   forces `depthWrite=true / alphaTest=0.5 / transparent=false`, builds an
   `InstancedMesh`, writes per-instance position+scale (rotation is left
   for `updateBillboardFacing` to overwrite each frame), pushes the mesh
   into `scene` and into `billboardInstances` as `{ mesh, positions }`.
5. If `topQuad` exists: builds a single horizontal `InstancedMesh` with
   per-instance Y rotation baked in (no per-frame update), pushes into
   `billboardTopInstances`.
6. Calls `updateBillboardFacing()` and `updateBillboardVisibility()` once
   so the first frame is correct before `requestAnimationFrame` ticks.
7. Returns the array of created `InstancedMesh`es so the caller (always
   `runStressTest` / `runProductionStressTest` / `runLodStressTest`) can
   `stressInstances.push(...result)` to track them for cleanup.

### `updateBillboardFacing()` (line ~3994)

```js
for (const { mesh, positions } of billboardInstances) {
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
```

Note: this re-writes scale to `(1,1,1)` every frame, dropping any
per-instance scale variation set at creation time. That is a pre-existing
quirk we should mirror exactly (not "fix") so the tilted path stays
behaviourally identical to the horizontal one.

### Animate loop hookup (line ~3338)

```js
if (stressActive && (billboardInstances.length > 0 || billboardTopInstances.length > 0)) {
    updateBillboardFacing();
    updateBillboardVisibility();
}
```

### `clearStressInstances` (line ~3369)

Resets `stressInstances`, `billboardInstances`, `billboardTopInstances`,
`volumetricInstances`, `volumetricHybridFade`, `stressActive`. Re-shows
`currentModel`. The new `tiltedBillboardInstances` array must be reset
here too.

## LOD toggle bar

`static/index.html:40-51`:

```html
<div class="lod-toggle" id="lodToggle" style="display:none">
    <button data-lod="lod0" disabled>LOD0</button>
    ...
    <button data-lod="billboard"  disabled>Camera-facing</button>
    <button data-lod="volumetric" disabled>Dome slices</button>
    <button data-lod="production" disabled>Hybrid</button>
    <button data-lod="vlod0" disabled>Vol high</button>
    ...
</div>
```

The bar is shown/hidden in `updatePreviewButtons` (`app.js:~4464`):

```js
lodToggle.style.display = hasLods || hasVlods ||
    (file && (file.has_billboard || file.has_volumetric)) ? 'flex' : 'none';
```

Each button is enabled/disabled by a per-`data-lod` switch in the same
function (`app.js:~4470`). The click handler at `~4543` reads
`btn.dataset.lod`, sets `previewVersion`, picks a `fileSize` estimate,
maps `'production' → 'volumetric'` for the actual model URL, and calls
`loadModel(\`/api/preview/${selectedFileId}?version=${previewVer}&t=...\`, fileSize)`.

For the new "Tilted" button, the URL path needs to use
`?version=billboard-tilted`, and (since this ticket is plumbing only,
not the crossfade) selecting it should just load the tilted GLB into
the preview viewport — no instancing magic at all in `loadModel`.

## Where instances get created on the preview path

This is the part that surprised me on first reading and is worth being
explicit about:

The horizontal "Camera-facing" button does NOT create instances when
clicked. It just calls `loadModel(...)` with the billboard GLB and the
viewport shows the exported quads in their export-time positions
(see `renderMultiAngleBillboardGLB` line ~1774, which sets
`quad.position.set(i * quadSize * 1.2, 0, 0)` so the variants line up
side-by-side for visual inspection).

`createBillboardInstances` is only called from the **stress test**
path (`runStressTest` line ~4078, `runProductionStressTest` line ~4141,
`runLodStressTest` line ~4237). The stress test fires from the
"Run scene" button in the right panel, not from the LOD toggle.

The ticket's third bullet — *"Selecting the 'Tilted' preview loads
`/api/preview/:id?version=billboard-tilted` and creates instances at
the asset's position"* — therefore needs interpretation. Two readings:

1. **Literal:** the click should both swap the previewed model AND
   place a single instance at the asset's origin via the new
   `createTiltedBillboardInstances`. This is unlike how the horizontal
   button works.
2. **By analogy with the horizontal button:** just swap the previewed
   model. `createTiltedBillboardInstances` exists for the stress-test
   path (T-009-03) but is not invoked from the LOD toggle in this
   ticket.

Reading #2 is what every other LOD toggle button in the bar does. The
ticket says "no crossfade yet — that's T-009-03," and the "creates
instances at the asset's position" phrasing is most naturally read as
*"the previewed file is the tilted bake, displayed in its export
positions"*, i.e. exactly what `loadModel` already does for the
horizontal billboard.

We will go with reading #2 in design.md and confirm by inspection that
the loader function is still wired into the stress-test path so T-009-03
has a hook to plug into. This matches the ticket's "First-Pass Scope"
note: *"This is plumbing. Mirror the existing billboard loader code
path; do not try to share more than necessary between the two."*

## Backend touchpoints (read-only, all already shipped in T-009-01)

- `models.go:56` — `HasBillboardTilted bool` on `FileRecord`.
- `handlers.go:334` — `case "billboard-tilted":` arm in `handlePreview`.
- `handlers.go:466` — `handleUploadBillboardTilted` mirroring the
  existing horizontal upload handler.
- `main.go:122` — route registration.
- `main.go:212` — `scanExistingFiles` startup detection.

Nothing in the Go tree needs to change for this ticket. The flag
`has_billboard_tilted` already arrives on the `FileRecord` JSON the
frontend `refreshFiles` consumes.

## Constraints and assumptions

- **No top quad in the tilted GLB.** The new loader must tolerate the
  absence of a `billboard_top` mesh. Treat all meshes in the tilted file
  as side variants.
- **Camera-facing in yaw only.** Same `Math.atan2(camPos.x - pos.x,
  camPos.z - pos.z)` rotation as horizontal. The tilt is baked into the
  texture, not the runtime transform.
- **Stable variant assignment.** Use a different seed offset from the
  horizontal loader (`+ 9999`) so the two loaders don't accidentally
  produce identical variant patterns when both run on the same instance
  set in T-009-03 — pick `+ 7777` (arbitrary, just distinct).
- **Cleanup parity.** `clearStressInstances` must reset
  `tiltedBillboardInstances = []` or the next preview swap will hold
  references and leak GPU memory.
- **No new Go code.** All backend support is already in place.

## Open questions for design

1. Button label — ticket says "Tilted" or similar, name TBD. The other
   buttons in the bar use nouny phrases ("Camera-facing", "Dome slices",
   "Hybrid"). "Tilted" alone is fine and consistent enough; "Tilted
   billboards" is more verbose but matches the existing naming. Decide
   in design.md.
2. Whether `updateTiltedBillboardFacing` should be a separate function
   or share `updateBillboardFacing` via parameterisation. The ticket
   says separate; honour that and keep the two paths cleanly forked.
3. Whether to also add a `lodToggle.style.display` clause for
   `has_billboard_tilted` in `updatePreviewButtons`. Yes — otherwise an
   asset that has only a tilted bake (no horizontal billboard, no
   volumetric, no LODs) would never show the toggle bar at all.
