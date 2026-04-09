# Research — T-001-02: Hard-Surface Texture Baking

## Scope

Transfer appearance information (wood grain, color variation, weathering) from the
original TRELLIS model onto the parametric box-primitive reconstruction produced by
T-001-01. Output a single texture atlas for the whole assembly.

## What Exists Today

### Original TRELLIS Model (`assets/wood_raised_bed.glb`, 1.2 MB)
- 10,402 triangles, 6,571 vertices
- Contains a single embedded texture (PNG, ~1024x1024) mapped via TEXCOORD_0
- Material: PBR metallic-roughness with baseColorTexture
- Mesh is a single fused blob — no per-board separation

### Parametric Reconstruction (`assets/wood_raised_bed_parametric.glb`, 17.9 KB)
- 192 triangles (16 boxes x 12 tris), 384 vertices (16 boxes x 24 verts)
- Single mesh primitive, single material
- UVs: per-face tiling with factor `u_size * 4.0` — all boards tile the same
  texture identically. No per-board UV variation currently.
- Texture: 128x128 JPEG (3.1 KB) resized from original via PIL
- The resized texture is the *entire* original texture shrunk down — not a
  region-specific bake.

### Texture Pipeline in the Codebase
- **gltfpack**: handles texture compression (KTX2/WebP), quality, size limits.
  No baking capability.
- **Blender backend** (`scripts/remesh_lod.py`): decimation and remesh only.
  No baking workflow. Script is invoked headless via `blender -b --python`.
- **Frontend** (`static/app.js`): Three.js preview with GLTFLoader. No custom
  UV or texture manipulation. Billboard/volumetric renderers use OffscreenCanvas
  but only for 2D sprite capture, not UV-space baking.
- **Server** (`handlers.go`): File upload, process, download. No texture baking
  endpoint.

### parametric_reconstruct.py Texture Handling (lines 445–545)
- `prepare_texture()`: resizes original texture via PIL or falls back to
  procedural brown. Returns JPEG bytes.
- `_generate_wood_texture()`: PIL-based procedural grain using sin waves.
- `_minimal_jpeg()`: pure-Python 8x8 solid-color JPEG fallback.
- `write_glb()`: embeds single texture, sets sampler to REPEAT/LINEAR_MIPMAP_LINEAR.

### UV Layout in parametric_reconstruct.py (lines 326–416)
- `generate_box()` creates 24 vertices per box (4 per face, 6 faces).
- UVs tile proportionally: `u_tile = u_size * 4.0`, `v_tile = v_size * 4.0`.
- All boards share the same UV space — no per-board offset or rotation.
- Wood grain direction is implicit: UV U-axis follows board length on the
  visible (±Z, ±X) faces.

## Texture Sources Available

1. **Original TRELLIS texture**: embedded PNG in `wood_raised_bed.glb`. This is
   the primary appearance source. Contains wood grain, color variation, and some
   TRELLIS-generated weathering artifacts.

2. **Tileable wood textures**: not currently in the repo, but the acceptance
   criteria mention "tileable wood textures with per-board UV variation."
   Options: (a) generate from the original texture by extracting a tileable
   region, (b) use a procedural texture, (c) ship a stock tileable wood texture.

3. **Blender bake**: Blender can project textures from one mesh onto another
   using its bake system. Requires both meshes loaded, UV-unwrapped target,
   and ray-cast from target surface to source surface.

## Relevant External Tools

### Blender Bake (Cycles)
- `bpy.ops.object.bake(type='DIFFUSE')` — bakes diffuse color from one object
  to another via ray casting.
- Requires: source mesh (TRELLIS) + target mesh (parametric) in same scene,
  target selected with source as active, target must have UV map and image to
  bake to.
- Can handle the projection automatically — each target face ray-casts to the
  nearest source surface and samples its texture.
- Output: image file (PNG/JPEG) that maps to the target's UV layout.
- Execution: headless via `blender -b --python bake_script.py`.

### UV Projection Methods
- **Box projection**: Blender can auto-UV-unwrap boxes. Since all geometry is
  axis-aligned boxes, a simple box projection gives clean UVs.
- **Smart UV Project**: automatic unwrap that minimizes distortion. Overkill for
  boxes but available.
- **Manual/scripted**: the parametric script already generates UVs per-face.
  These can be adjusted for atlas packing.

## Constraints and Boundaries

- **File size budget**: parametric GLB is 17.9 KB currently. A 512x512 JPEG
  atlas at quality 75 is ~30-60 KB. A 1024x1024 is ~100-200 KB. Total output
  should stay reasonable for real-time use (under ~250 KB).
- **Draw calls**: acceptance criteria require a single shared atlas — this means
  one material, one texture, all boards packed into one UV space.
- **Per-board variation**: boards must not look identical. This requires either
  (a) baking unique texture regions per board from the original, or (b) UV
  offsets/rotations so each board samples a different part of the tileable texture.
- **Grain direction**: wood grain should run along board length. The current UV
  layout partially achieves this but all boards sample identically.
- **Integration**: the script should extend or complement `parametric_reconstruct.py`.
  No Go server changes required for the core baking — it's a build-time/offline
  operation.

## Key Observations

1. The original TRELLIS texture contains spatially-varying appearance. A true
   bake (ray-cast projection) would transfer this per-board. However, the
   TRELLIS mesh is a fused blob — the texture mapping may not cleanly correspond
   to individual boards.

2. A simpler approach: use a tileable wood texture and vary UVs per board
   (random offset + optional rotation). This gives visual variety without
   needing ray-cast projection. The original texture can be used as the tileable
   source by extracting a representative region.

3. Atlas packing for 16 boxes (6 visible faces each, but top/bottom faces are
   mostly hidden) can be done manually since all boards are axis-aligned
   rectangles. A grid layout in UV space is straightforward.

4. Blender is already available in the pipeline (detected at startup, headless
   execution proven). Adding a bake script follows the same pattern as
   `remesh_lod.py`.

5. The acceptance criteria call for "visual comparison: side-by-side of original
   vs reconstructed model in the optimizer's preview." The frontend already has
   version switching — adding a `parametric` version variant would enable this.

## Questions for Design Phase

- Bake from original mesh (ray-cast) vs. tileable texture with UV variation?
- Atlas resolution: 512x512 or 1024x1024?
- Should the bake script be standalone (like parametric_reconstruct.py) or
  integrated into the Go server as an API endpoint?
- How to handle top/bottom faces (mostly invisible) — skip or include in atlas?
