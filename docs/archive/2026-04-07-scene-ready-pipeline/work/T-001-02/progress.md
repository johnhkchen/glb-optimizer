# Progress — T-001-02: Hard-Surface Texture Baking

## Status: Complete

## Completed
- Step 1: Added --atlas-layout flag to parametric_reconstruct.py
- Step 2: Created bake_textures.py with scene setup and argument parsing
- Step 3: Implemented atlas UV layout (4x4 grid, per-board islands)
- Step 4: Implemented Blender EMIT bake mode (ray-cast from source to target)
  - Deviation: used EMIT bake type instead of DIFFUSE — Cycles DIFFUSE bake
    produced 0% coverage due to lighting dependency. EMIT with emission-rewired
    source material captures texture color directly. Coverage: 98.3%.
- Step 5: Implemented tileable fallback mode (per-board random sampling)
- Step 6: Implemented GLB export with embedded JPEG atlas
- Step 7: End-to-end test and output generation
  - Bake mode: 40,412 bytes, 192 triangles, 512x512 atlas, 98.3% coverage
  - Tile mode: 31,556 bytes, 192 triangles, 512x512 atlas

## Deviations from Plan
- Bake type changed from DIFFUSE to EMIT to avoid Cycles lighting issues
- Source material rewired to Emission shader to make texture visible in EMIT bake
- Cage extrusion increased from 0.1 to 1.0 for reliable ray-cast coverage
- Debug atlas PNG save removed from production code
