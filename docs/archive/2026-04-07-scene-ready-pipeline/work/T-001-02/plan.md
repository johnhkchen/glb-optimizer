# Plan — T-001-02: Hard-Surface Texture Baking

## Step 1: Add `--atlas-layout` flag to parametric_reconstruct.py

**What**: Add a `--atlas-layout` CLI flag that prints a JSON manifest of board
metadata to stdout after mesh generation.

**Changes**:
- Add `--atlas-layout` argument to the argparse block (~3 lines)
- After `detect_boards()`, if `--atlas-layout` is set, print JSON with board
  index, description, center, dims, vertex_offset, vertex_count (~15 lines)

**Verification**: Run `python3 scripts/parametric_reconstruct.py --input
assets/wood_raised_bed.glb --output /dev/null --atlas-layout` and verify JSON
output contains 16 boards with correct metadata.

**Commit**: "Add --atlas-layout flag to parametric_reconstruct.py"

## Step 2: Create bake_textures.py — scene setup and argument parsing

**What**: Create the Blender bake script skeleton with argument parsing and
scene setup (import both GLBs).

**Changes**:
- Create `scripts/bake_textures.py`
- Implement argument parser (source, target, output, atlas-size, mode, seed)
- Implement `setup_scene()`: clear defaults, import source GLB, import target
  GLB, set Cycles engine
- Implement entrypoint that parses args and calls setup

**Verification**: Run `blender -b --python scripts/bake_textures.py --
--source assets/wood_raised_bed.glb --target assets/wood_raised_bed_parametric.glb
--output /tmp/test.glb --mode tile` and verify both meshes are imported
(print object names and vertex counts).

**Commit**: "Add bake_textures.py skeleton with scene setup"

## Step 3: Implement atlas UV layout

**What**: Add the `layout_atlas_uvs()` function that assigns each board a
unique UV island in a 4x4 grid.

**Changes**:
- Add `layout_atlas_uvs(obj, atlas_size, seed)` function
- Detect board boundaries in the mesh (every 24 vertices = 1 board, since
  parametric_reconstruct.py generates 24 verts per box)
- Assign each board's faces to a grid cell in UV space
- Apply per-board random UV jitter (seeded)
- Map the 4 visible faces into the grid cell (front/back face gets most area,
  top and ends get remaining space)

**Verification**: After running, inspect UV coordinates by printing min/max UV
per board. Verify 16 distinct UV islands, no overlaps, all within [0,1].

**Commit**: "Implement atlas UV layout for parametric mesh"

## Step 4: Implement Blender bake mode

**What**: Add the `bake_diffuse()` function that ray-casts diffuse color from
the source TRELLIS mesh onto the parametric mesh's atlas.

**Changes**:
- Add `bake_diffuse(source_obj, target_obj, image)` function
- Create material on target with image texture node for bake target
- Set up selected-to-active bake: select target, set source as active
- Configure bake params: type=DIFFUSE, pass_filter=COLOR, margin=4px,
  ray_distance=0.05 (small because meshes should nearly overlap)
- Handle Cycles device fallback (CPU if no GPU)
- Execute bake and save result

**Verification**: Run in bake mode, inspect output image file. Verify it's
512x512 with wood-colored content (not black/empty). Check that different board
regions show different texture content.

**Commit**: "Implement Blender diffuse bake from TRELLIS to parametric mesh"

## Step 5: Implement tileable fallback mode

**What**: Add `tile_from_source()` that extracts the original texture and
paints it into atlas slots with per-board variation.

**Changes**:
- Add `tile_from_source(source_obj, target_obj, image, seed)` function
- Extract baseColorTexture from source material
- For each board's UV island, sample a randomly-offset rectangular region
  of the source texture and copy pixels into the atlas at the island location
- Rotate sampling 90 degrees for post end-grain faces

**Verification**: Run with `--mode tile`, inspect output atlas image. Verify
16 distinct texture regions visible in the atlas, with visual variation between
boards.

**Commit**: "Implement tileable texture fallback for bake_textures.py"

## Step 6: Implement GLB export with embedded atlas

**What**: Export the parametric mesh with the baked/tiled atlas as a GLB file.

**Changes**:
- Remove source object from Blender scene
- Set target material to use baked atlas as baseColorTexture
- Configure PBR: metallic=0, roughness=0.9
- Export via `bpy.ops.export_scene.gltf(filepath=output, export_format='GLB')`
- Print summary stats (file size, triangle count, atlas size)

**Verification**: Open output GLB in a glTF viewer or the optimizer's preview.
Verify wood texture visible on all boards, per-board variation apparent,
single draw call.

**Commit**: "Add GLB export with baked atlas to bake_textures.py"

## Step 7: End-to-end test and output generation

**What**: Run the full pipeline and generate the final
`assets/wood_raised_bed_textured.glb`.

**Changes**:
- Run parametric_reconstruct.py (already produces parametric.glb)
- Run bake_textures.py with `--mode bake` (primary) or `--mode tile` (fallback)
- Verify output file size, triangle count, atlas content
- Generate the final asset file

**Verification**:
- Output GLB < 250 KB (target: 50-70 KB)
- 192 triangles (unchanged from parametric)
- Single material, single texture
- Visual inspection: boards show unique wood grain, grain runs along length
- Side-by-side comparison with original in optimizer preview

**Commit**: "Generate wood_raised_bed_textured.glb with baked atlas"

## Testing Strategy

**Structural tests** (automated, run after each step):
- GLB file validity: magic bytes, version, chunk alignment
- Accessor/buffer view consistency
- UV coordinates within [0,1] range for atlas mode
- Atlas image dimensions match `--atlas-size`

**Visual tests** (manual, after step 7):
- Upload both original and textured GLBs to the optimizer
- Compare side-by-side in the preview
- Verify wood grain direction, per-board variation, no seam artifacts
- Check at different zoom levels

**Regression tests**:
- Running parametric_reconstruct.py without new flags produces identical output
- Existing scripts/remesh_lod.py still works unchanged
- Go server starts and serves both files correctly

## Risk Mitigation

- **Bake fails (black texture)**: If ray-cast distance is wrong or meshes don't
  overlap, adjust `ray_distance` or fall back to `--mode tile`.
- **Blender not available**: Script prints clear error and exits. The tileable
  fallback could be extracted as a standalone PIL-based script if needed.
- **Atlas too large**: JPEG quality can be reduced (default 75, try 50). Or
  use 256x256 atlas for maximum compression.
- **UV seams visible**: Increase bake margin from 4px to 8px. Ensure sampler
  uses LINEAR filtering (already set in parametric_reconstruct.py).
