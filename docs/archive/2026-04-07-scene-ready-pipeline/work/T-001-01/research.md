# Research — T-001-01: Raised Bed Parametric Reconstruction

## Source Model Analysis

**File**: `assets/wood_raised_bed.glb` (1,257,868 bytes / ~1.2MB)

### GLB Structure
- **Format**: glTF 2.0 binary
- **Meshes**: 1 (`geometry_0`), 1 primitive
- **Nodes**: 2 (root `world` → child `geometry_0`)
- **Materials**: 1 (unnamed, single PBR material)
- **Textures**: 1 PNG image (1,000,304 bytes — 79.5% of file size)
- **Accessors**: 3 (POSITION, TEXCOORD_0, indices)
- **No normals stored** — only POSITION and TEXCOORD_0 attributes

### Geometry Stats
- **Vertices**: 6,571
- **Triangles**: 10,402
- **Bounding box**: [-0.500, -0.109, -0.324] to [0.501, 0.110, 0.324]
- **Dimensions (model units)**: 1.001 × 0.219 × 0.648 (X × Y × Z)
- **Axis-aligned faces**: 82.5% (8,583 of 10,402 triangles within 15° of XYZ axes)
  - X-normal: 2,923 (side faces)
  - Y-normal: 1,146 (top/bottom faces)
  - Z-normal: 4,514 (front/back faces)
- **Mean triangle area**: 0.000154 (very small — high tessellation of flat surfaces)

### Mesh Topology
- **Connected components**: 1 significant component (3,823 verts) + 2,748 isolated single vertices
- The isolated vertices are likely artifacts from TRELLIS generation (degenerate triangles)
- The mesh is a single watertight (or near-watertight) surface, NOT pre-segmented into boards
- No node hierarchy separating boards — everything is one mesh

### Texture
- Single 1MB PNG occupying 80% of file size
- UV range: [0,1] × [0,1] — full UV space utilized
- Wood grain texture baked by TRELLIS from the source image

### Model Orientation
- X-axis: length (long side of bed, ~1.0 units)
- Z-axis: width (short side of bed, ~0.65 units)
- Y-axis: height (~0.22 units)

## Existing Codebase

### Architecture
Go web server (`main.go`) with:
- **Upload/process pipeline**: Upload GLBs → process with gltfpack → download optimized
- **gltfpack integration** (`processor.go`): Builds CLI args for mesh simplification, compression, texture compression
- **Blender integration** (`blender.go`): Headless LOD generation via `remesh_lod.py`
- **LOD pipeline**: 4-level LOD generation (lod0-lod3) with progressive simplification
- **Web UI** (`static/app.js`): Upload, configure settings, preview, download

### Relevant Patterns
- Models stored in `~/.glb-optimizer/originals/` and `~/.glb-optimizer/outputs/`
- Processing is synchronous per-request (queue channel exists but unused)
- Blender script (`scripts/remesh_lod.py`) handles: decimate, planar dissolve, voxel remesh, unsubdivide
- All current optimization is mesh simplification — no parametric reconstruction exists

### What Does NOT Exist
- No mesh analysis / decomposition tooling
- No parametric geometry generation
- No box-primitive or CSG reconstruction
- No cut-list generation
- No GLB authoring from scratch (only gltfpack and Blender re-export)

## Key Constraints

1. **Single mesh problem**: The model is one fused mesh. Board identification requires spatial decomposition (plane clustering, OBB fitting), not simple component separation.

2. **Texture dominance**: The 1MB PNG is 80% of file size. Even reducing triangles to 300 won't help if the texture stays. Must either: (a) massively downsample the texture, (b) use a small tiled wood texture, or (c) use vertex colors.

3. **Target budget**: <50KB total, <300 triangles. A box is 12 triangles. Budget allows ~25 boxes. A raised bed typically has 12-16 boards + 4 posts = 16-20 boxes = 192-240 triangles. Feasible.

4. **No normals in source**: Only positions + UVs. Normals must be computed from face geometry during analysis.

5. **Model scale**: Dimensions are ~1.0 × 0.22 × 0.65 model units. Need to infer real-world scale to map to standard lumber sizes (2×6 = 1.5" × 5.5", 4×4 = 3.5" × 3.5").

## Physical Construction Pattern (Domain Knowledge)

A typical wooden raised garden bed:
- **Corner posts**: 4× vertical 4×4 posts at each corner
- **Side boards**: Horizontal boards (2×6 or 2×8) stacked 2-3 high on each side
- **Long sides**: 2 sides, each with 2-3 stacked boards
- **Short sides**: 2 sides, each with 2-3 stacked boards
- **Optional cap rail**: flat boards on top edges

Expected component count: 4 posts + 8-12 side boards = 12-16 pieces.

## Approach Space

Three viable decomposition strategies:
1. **Geometric analysis**: Cluster axis-aligned faces into planes, identify rectangular regions, fit OBBs
2. **Template-based**: Use domain knowledge of raised bed construction to define a parametric template, fit parameters to the mesh bounding regions
3. **Hybrid**: Use bounding box + axis-aligned plane detection to infer board positions, then snap to standard lumber sizes

The template-based approach is most reliable given the single-mesh topology and known object type.
