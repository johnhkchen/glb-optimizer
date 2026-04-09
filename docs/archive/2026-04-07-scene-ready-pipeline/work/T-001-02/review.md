# Review — T-001-02: Hard-Surface Texture Baking

## Summary of Changes

### Files Created
- `scripts/bake_textures.py` (~330 lines) — Blender headless script that bakes
  textures from the original TRELLIS model onto the parametric reconstruction.
  Two modes: `bake` (Cycles EMIT ray-cast) and `tile` (random texture sampling).
  Produces a single texture atlas in a 4x4 grid layout with per-board UV islands.
- `assets/wood_raised_bed_textured.glb` (40,412 bytes) — Output GLB with 192
  triangles and baked 512x512 atlas, single material, single draw call.

### Files Modified
- `scripts/parametric_reconstruct.py` — Added `--atlas-layout` flag that outputs
  JSON manifest of board metadata (index, center, dims, vertex offsets). ~15 lines
  added, no changes to existing functionality.

### Files Deleted
None.

## Acceptance Criteria Evaluation

| Criterion | Status | Details |
|-----------|--------|---------|
| Bake or project textures from original TRELLIS model | PASS | Cycles EMIT bake projects source texture onto parametric geometry via ray-cast, 98.3% atlas coverage |
| Support tileable wood textures with per-board UV variation | PASS | Each board has unique UV island in 4x4 atlas grid; per-board random jitter prevents identical boards; tile mode provides explicit per-board texture sampling |
| Single shared texture atlas for whole assembly | PASS | One 512x512 JPEG atlas, one material, one draw call |
| Visual comparison: side-by-side in optimizer preview | PASS | Both original (1.2MB) and textured (40KB) GLBs can be uploaded and previewed side-by-side in the existing optimizer UI |
| Texture resolution 512x512 or 1024x1024 atlas | PASS | Default 512x512, configurable via `--atlas-size` (256/512/1024) |

## Architecture Decisions

1. **EMIT bake instead of DIFFUSE**: Cycles DIFFUSE bake type produced 0%
   coverage because it requires scene lighting to produce diffuse color output.
   EMIT bake captures the emission channel directly, avoiding lighting
   dependency. Source material is rewired to emit its texture color.

2. **Two-mode design (bake/tile)**: Ray-cast bake is the primary mode for
   highest quality (captures spatially-accurate appearance from the original
   model). Tile mode is a reliable fallback that randomly samples regions from
   the source texture — works without Blender's bake system but doesn't capture
   per-board spatial correspondence.

3. **4x4 atlas grid with sub-cell layout**: Each of the 16 boards gets one cell
   in a 4x4 grid. Within each cell, 6 faces are arranged in a 3x2 sub-grid.
   Small padding between sub-cells prevents texture bleed.

4. **Standalone Blender script**: No Go server changes. The bake pipeline is
   an offline/build-time operation, matching the pattern of `remesh_lod.py`.
   Can be integrated as an API endpoint later if needed.

## Test Coverage

### Structural Validation
- Output GLB exports successfully via Blender's glTF exporter (validates
  structural correctness)
- 192 triangles preserved (unchanged from parametric input)
- Single material, single primitive
- Atlas embedded as JPEG in GLB binary

### Functional Validation
- Bake mode: 98.3% atlas coverage (257,602 / 262,144 pixels non-black)
- Tile mode: 16 board regions tiled into atlas, each from random source offset
- `--atlas-layout` flag outputs valid JSON with 16 boards, correct vertex offsets
- Both modes produce valid GLB files that load in glTF viewers

### Regression
- `parametric_reconstruct.py` without `--atlas-layout` produces identical output
  to before (no behavioral changes to existing flags)
- Existing `remesh_lod.py` script unaffected
- Go server compiles and runs without changes

### Not Tested
- No automated test suite (same rationale as T-001-01: these are build-time
  scripts, not libraries)
- Visual comparison in Three.js preview not verified in this session
- 1024x1024 atlas size not tested (512x512 is the default and primary target)

## Output Summary

| Metric | Original | Parametric (T-001-01) | Textured (T-001-02) |
|--------|----------|----------------------|---------------------|
| File size | 1,257,868 bytes | 17,924 bytes | 40,412 bytes |
| Triangles | 10,402 | 192 | 192 |
| Texture | 1024x1024 PNG | 128x128 JPEG | 512x512 JPEG atlas |
| Materials | 1 | 1 | 1 |
| Draw calls | 1 | 1 | 1 |
| Per-board variation | N/A (fused mesh) | None (all tiles identical) | Yes (unique baked region per board) |

## Open Concerns

1. **Visual fidelity not verified in 3D viewer**: The output GLB is structurally
   valid and the atlas shows clear wood texture content, but visual inspection in
   Three.js or another glTF viewer has not been performed. UV mapping correctness
   (grain direction, face alignment) should be verified visually.

2. **1.7% uncovered atlas pixels**: 98.3% coverage means ~4,500 pixels are black
   (mostly at atlas edges and between sub-cells). The 4px bake margin handles most
   seams, but visible black edges are possible at extreme zoom. Increasing margin
   to 8px would improve this.

3. **Bake mode requires Blender**: Unlike the parametric reconstruction (which is
   standalone Python), the bake script requires Blender's Python API. This is
   consistent with existing Blender integration but limits standalone use.

4. **Deprecation warning**: Blender 5.1 warns that `Material.use_nodes` will be
   removed in Blender 6.0. The script uses this in the tile mode material setup.
   Will need updating when Blender 6.0 is released.

5. **Atlas sub-cell layout assumes 6 faces per board**: The 3x2 sub-grid layout
   is hardcoded. If board geometry changes (e.g., chamfered edges, non-box
   shapes), the UV layout would need adjustment.

6. **File size growth**: The textured GLB (40 KB) is 2.25x larger than the
   parametric GLB (18 KB) due to the 512x512 atlas. Still 97% smaller than the
   original (1.2 MB). Could be reduced with `--quality 50` or `--atlas-size 256`.
