# Structure — T-001-02: Hard-Surface Texture Baking

## Files Created

### `scripts/bake_textures.py` (~300 lines)
Blender headless script for texture baking. Invoked via:
```
blender -b --python scripts/bake_textures.py -- \
  --source assets/wood_raised_bed.glb \
  --target assets/wood_raised_bed_parametric.glb \
  --output assets/wood_raised_bed_textured.glb \
  --atlas-size 512 \
  --mode bake
```

**Modules within the script:**

1. **Argument Parser** (~20 lines)
   - `--source`: original TRELLIS GLB path
   - `--target`: parametric GLB path
   - `--output`: output GLB path (with baked atlas)
   - `--atlas-size`: 512 or 1024 (default 512)
   - `--mode`: `bake` (ray-cast from source) or `tile` (tileable fallback)
   - `--seed`: random seed for UV jitter (default 42)

2. **Scene Setup** (~40 lines)
   - Clear default scene
   - Import source GLB (`bpy.ops.import_scene.gltf`)
   - Import target GLB
   - Position both at origin (they should already overlap spatially)
   - Set render engine to Cycles (required for bake)

3. **Atlas UV Layout** (~80 lines)
   - `layout_atlas_uvs(target_obj, board_count, atlas_size, seed)`
   - Iterate over target mesh faces in board-order (each board = 12 tris = 6 quads)
   - Assign each board a slot in a 4x4 grid within UV space [0,1]
   - Each slot: `(col/4, row/4)` to `((col+1)/4, (row+1)/4)`
   - Map the board's 6 face-quads into the slot:
     - Front/back face (largest): occupies ~60% of slot area
     - Top face: narrow strip at top of slot
     - Two end faces: small squares at sides
   - Apply per-board UV jitter: shift UV island by `random.uniform(-0.02, 0.02)`
     in both U and V, seeded by board index.
   - Create new UV layer `atlas_uv` on the target mesh

4. **Bake Execution** (~50 lines)
   - `bake_diffuse(source_obj, target_obj, atlas_image)`
   - Create image texture node on target material, assign atlas image
   - Select target, set source as active (`selected_to_active = True`)
   - Configure bake settings: type=DIFFUSE, pass_filter=COLOR only,
     margin=4px (bleed to prevent seams), ray distance=0.05
   - Execute: `bpy.ops.object.bake(type='DIFFUSE')`
   - Save baked image to temp path

5. **Tileable Fallback** (~40 lines)
   - `tile_from_source(source_texture_path, target_obj, atlas_image, seed)`
   - Extract the original texture from the source GLB
   - For each board's UV island, sample a randomly-offset region of the
     source texture and paint it into the atlas
   - Rotates sampling 90 degrees for end-grain faces
   - Used when `--mode tile` or when bake fails

6. **GLB Export** (~30 lines)
   - Remove source object from scene (keep only target with baked material)
   - Update target material: set baseColorTexture to baked atlas
   - Set metallic=0, roughness=0.9 (wood PBR values)
   - Export as GLB: `bpy.ops.export_scene.gltf(filepath=output, export_format='GLB')`
   - Compress atlas to JPEG in-GLB (Blender glTF exporter supports this)

7. **Entrypoint** (~40 lines)
   - Parse args from `sys.argv` (after `--` separator)
   - Run scene setup, UV layout, bake/tile, export
   - Print summary stats (atlas size, output file size, board count)

## Files Modified

### `scripts/parametric_reconstruct.py`
**Changes**: Add `--atlas-layout` flag and JSON output of board UV metadata.

- Add CLI flag: `--atlas-layout` (store_true)
- When set, after building the mesh, output a JSON manifest to stdout with:
  ```json
  {
    "boards": [
      {"index": 0, "description": "Corner post (left-back)", "center": [...], "dims": [...], "vertex_offset": 0, "vertex_count": 24},
      ...
    ],
    "total_vertices": 384,
    "total_triangles": 192
  }
  ```
- This metadata helps `bake_textures.py` identify which faces belong to which
  board without re-parsing the geometry.
- No changes to existing functionality — `--atlas-layout` is additive.

## Files Unchanged

- `main.go` — no new routes needed
- `handlers.go` — baking is offline, not an API endpoint
- `models.go` — no new data structures
- `processor.go` — gltfpack not involved
- `blender.go` — bake script invoked manually, not through the Go server
- `static/index.html` — no frontend changes (both files can be uploaded and
  previewed separately using existing UI)
- `static/app.js` — no changes
- `static/style.css` — no changes
- `scripts/remesh_lod.py` — unrelated

## Output Artifacts

### `assets/wood_raised_bed_textured.glb`
- Parametric box geometry (192 tris) with baked 512x512 atlas
- Single material, single texture, single draw call
- Expected size: 50-70 KB (geometry ~18 KB + JPEG atlas ~30-50 KB)
- UV layout: 4x4 grid, one slot per board, per-board jitter applied

## Architecture Boundaries

- `bake_textures.py` depends on Blender's Python API (`bpy`) — it is a Blender
  script, not a standalone Python script. Cannot run without Blender.
- `parametric_reconstruct.py` remains standalone (no `bpy` dependency). The
  `--atlas-layout` flag just adds JSON output, no new imports.
- The bake pipeline is: `parametric_reconstruct.py` (generate geometry) →
  `bake_textures.py` (bake appearance) → final GLB.
- These are sequential build steps, not coupled at the module level.

## Data Flow

```
wood_raised_bed.glb (original TRELLIS)
         |
         v
parametric_reconstruct.py --input ... --output ... --atlas-layout
         |
         +--> wood_raised_bed_parametric.glb (box geometry, tiled UVs)
         +--> board manifest JSON (stdout, when --atlas-layout)
         |
         v
bake_textures.py --source original.glb --target parametric.glb --output textured.glb
         |
         +--> Blender scene: loads both GLBs
         +--> Rewrites target UVs into 4x4 atlas grid
         +--> Bakes diffuse from source → target (or tiles as fallback)
         +--> Exports target with embedded baked atlas
         |
         v
wood_raised_bed_textured.glb (final output: parametric geometry + baked atlas)
```

## Component Interactions

- `bake_textures.py` re-UVs the target mesh internally (doesn't rely on the
  tiling UVs from `parametric_reconstruct.py`). The atlas UV layout is computed
  fresh based on face topology.
- The `--atlas-layout` metadata from `parametric_reconstruct.py` is optional
  context for debugging/verification, not a hard dependency.
- Both scripts can be run independently: `parametric_reconstruct.py` works as
  before, `bake_textures.py` works with any source/target GLB pair.

## Ordering

1. `parametric_reconstruct.py` must run first (produces target geometry).
2. `bake_textures.py` runs second (requires both source and target GLBs).
3. No circular dependencies.
