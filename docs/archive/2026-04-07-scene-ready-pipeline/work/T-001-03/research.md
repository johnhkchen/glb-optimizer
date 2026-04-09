# Research: T-001-03 Rose Volumetric Distillation

## The Model

`assets/rose_julia_child.glb` — 7.8MB TRELLIS-generated Julia Child rose bush. Dense organic
foliage with many overlapping leaves, petals, and stems. This geometry is fundamentally
unsuitable for mesh simplification: decimation destroys thin features (leaves, petals) at the
ratios needed (99%+) for mobile scene rendering.

Target: <100KB output, 12-16 triangles (6-8 textured quads x 2 tris each).

## Existing Codebase Architecture

### Server (Go)

- `main.go` — HTTP server, route registration, startup. Embeds `static/*`. Working dirs:
  `~/.glb-optimizer/{originals,outputs}`.
- `handlers.go` — All API endpoints. Key patterns:
  - Upload/process/download for gltfpack pipeline
  - `handleGenerateLODs` / `handleGenerateBlenderLODs` — LOD generation endpoints
  - `handleUploadBillboard` — accepts a GLB blob via POST, saves as `{id}_billboard.glb`
  - `handlePreview` — serves files with version routing: `original|optimized|lod0-3|billboard`
  - `handleDeleteFile` — cleans up all variant files including `_billboard.glb`
- `models.go` — `FileRecord` struct (the canonical model), `FileStore` (in-memory thread-safe
  store), `LODLevel` struct. `FileRecord.HasBillboard` bool tracks billboard existence.
- `processor.go` — gltfpack CLI wrapper: `BuildCommand`, `RunGltfpack`.
- `blender.go` — Blender detection, `BlenderLODConfig`, `RunBlenderLOD`. Embeds
  `scripts/remesh_lod.py` and writes it to working dir at startup.

### Frontend (Three.js)

- `static/app.js` — Single-file SPA (~1100 lines). Key systems:
  - Billboard generation: `renderMultiAngleBillboardGLB()` renders model from 6 angles + 1
    top-down using offscreen `WebGLRenderer` with orthographic cameras, creates alpha-textured
    `PlaneGeometry` quads, exports via `GLTFExporter` as binary GLB.
  - Billboard instancing: `createBillboardInstances()` creates `InstancedMesh` for stress
    testing. Side quads face camera each frame (`updateBillboardFacing`), top quad swaps
    visibility based on camera elevation (`updateBillboardVisibility`).
  - Preview system: loads GLB via `GLTFLoader`, supports version switching, wireframe toggle,
    stats display (triangles, vertices, file size).
  - Stress test: places N instances in a grid with LOD distribution based on distance.

### Billboard Pattern (closest analog to volumetric distillation)

The existing billboard system (`renderMultiAngleBillboardGLB`):
1. Computes model bounding box
2. Creates offscreen WebGLRenderer (512x512, alpha=true)
3. Sets up orthographic camera sized to model bounds
4. Renders from N angles evenly spaced around Y axis
5. Renders one top-down view
6. Creates `PlaneGeometry` + `MeshBasicMaterial(map, transparent, alphaTest)` per view
7. Exports all quads as a single GLB via `GLTFExporter`
8. Uploads to server via `/api/upload-billboard/{id}`

Resolution: 512x512. Materials: `MeshBasicMaterial` with `alphaTest: 0.1`, `DoubleSide`.
Geometry origin shifted to bottom edge for ground placement.

## Volumetric Distillation vs Billboards

Billboards render the model from **outside looking in** — each quad shows the model's
silhouette from one viewing angle. The result is view-dependent: only one quad is "correct"
at any given camera angle.

Volumetric distillation renders **cross-sections through the model** — each quad captures
a slice at a specific depth. Stacked together, the slices reconstruct the volume. This is
view-independent and looks correct from any angle (similar to CT/MRI visualization).

### Slicing Strategies

1. **Vertical (Y-axis) slices**: Horizontal planes at different heights. Good for layered
   canopy structures. A rose bush is roughly hemispherical, so vertical slices would capture
   top-down "layers" — root/stem at bottom, dense foliage at top.

2. **Radial slices**: Vertical planes through the center at different angles (like pie slices).
   Each plane captures a cross-section of the full height. This is what the existing billboard
   system approximates but with depth information.

3. **Hybrid**: A few radial slices for the main structure + a horizontal canopy cap. Best
   visual coverage with fewest quads.

For a rose bush (roughly cylindrical/spherical mass of foliage on a short stem), **radial
slicing** is the primary strategy: 4-6 vertical planes through the center at evenly spaced
angles, plus 1-2 horizontal planes for the canopy top.

### Depth-Aware Rendering

The key technical challenge: each slice must capture only the geometry at that depth, not
the full model. Approaches:

- **Clipping planes**: Use orthographic camera with near/far planes set to a thin slab around
  the slice position. Render only geometry within that depth range.
- **Depth peeling**: Render multiple passes, each peeling the nearest layer. Complex, overkill
  for offline baking.
- **Section rendering**: For radial slices, render from the slice plane's viewpoint with a
  very narrow orthographic frustum, capturing geometry on both sides of the plane.

The clipping-plane approach is simplest and maps directly to Three.js's `camera.near/far`.
For radial slices through the center, we render with the camera at the slice angle, with
near=0 (center) and far covering the model's radius — this captures one "half" of the model.
The quad is then placed at the slice position in 3D space.

However, for volumetric reconstruction the goal is NOT to capture a thin slice but rather
to capture the visual appearance from that viewing angle, similar to billboard but with
the quads arranged to intersect through the model's volume rather than sitting outside it.
This is closer to "billboard clouds" or "cross-tree" rendering used in game vegetation.

### Cross-Tree Pattern (Industry Standard for Vegetation)

The standard game-industry approach for vegetation imposters:
- 2-3 vertical planes intersecting at the center of the plant
- Each plane textured with an orthographic render from that angle
- Planes rotated 60-90 degrees apart
- Optional horizontal plane for canopy (top-down view)

This is essentially what billboards do but with planes **intersecting through the model**
rather than surrounding it. The visual effect: from any angle, at least one plane shows
a reasonable approximation of the plant.

This is the most practical interpretation of "volumetric distillation" for the rose bush.

## File/Endpoint Integration Points

- New output variant: `{id}_volumetric.glb` (parallel to `_billboard.glb`)
- Preview routing: add `volumetric` case to `handlePreview` switch
- Delete cleanup: add `_volumetric.glb` removal to `handleDeleteFile`
- Model tracking: add `HasVolumetric bool` to `FileRecord`
- Frontend: new generation function, new toolbar button, new LOD toggle option
- Upload endpoint: can reuse `/api/upload-billboard/` pattern or create dedicated endpoint

## Texture Budget Analysis

Target: <100KB total GLB with embedded textures.
- GLB overhead (header, JSON, buffers): ~2-5KB
- Per quad geometry (PlaneGeometry, 4 verts, 2 tris): ~200 bytes
- 8 quads geometry: ~1.6KB
- Remaining for textures: ~93KB
- Per texture at 256x256 PNG with alpha: ~20-40KB (depends on content)
- At 256x256: can fit ~3-4 textures in budget
- At 128x128: can fit ~8-12 textures easily
- At 256x256 with JPEG+separate alpha: ~10-15KB each, fits 6-8

Practical resolution: 256x256 per slice with PNG (alpha needed). With 6 slices + 1 top,
that's ~7 textures. May need 128x128 or shared atlas to hit <100KB.

## Assumptions and Constraints

1. The rose model is available in `assets/` but the optimizer works from uploaded files in
   the working directory — generation happens client-side in the browser like billboards.
2. Three.js 0.160.0 is pinned via importmap (no bundler).
3. GLTFExporter produces binary GLB — textures are embedded as PNG by default.
4. The alpha blending + depth sorting concern from acceptance criteria is handled by Three.js's
   built-in transparency sorting, but intersecting transparent planes will have artifacts.
   `alphaTest` (cutout) avoids sorting issues at the cost of hard edges.
5. No Blender dependency for this feature — it's entirely client-side rendering like billboards.
