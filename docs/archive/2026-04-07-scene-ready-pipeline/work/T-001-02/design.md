# Design — T-001-02: Hard-Surface Texture Baking

## Decision Summary

Use a **two-stage approach**: (1) Blender ray-cast bake to project the original
TRELLIS texture onto the parametric geometry's UV atlas, (2) per-board UV
variation with seeded random offsets to prevent identical boards. Output a single
512x512 atlas. Standalone Python/Blender script, no Go server changes.

## Options Evaluated

### Option A: Tileable Texture with UV Variation Only
- Extract a tileable region from the original TRELLIS texture.
- Modify `generate_box()` to add per-board UV offsets (random shift + optional
  90-degree rotation for end-grain).
- All boards sample from the same tileable texture but at different offsets.

**Pros**: Simple, no Blender dependency for baking, small texture size.
**Cons**: Loses the spatial color variation from the original model. All boards
share the same base appearance — variation is limited to offset. Doesn't fulfill
"bake or project textures from the original TRELLIS model" criterion.

### Option B: Blender Ray-Cast Bake (Chosen)
- Load both original TRELLIS mesh and parametric mesh in Blender.
- Set up UV atlas on parametric mesh with per-board island packing.
- Bake diffuse color from TRELLIS → parametric via Cycles ray-cast.
- Each board gets its own UV island in the atlas, capturing the unique
  appearance of that region of the original model.
- Add small per-board UV jitter in the parametric script to ensure boards
  aren't pixel-identical even if they overlap the same source region.

**Pros**: Transfers actual appearance from the original model. Each board gets
unique texture content. Fulfills "bake or project" criterion directly.
**Cons**: Requires Blender (already available in pipeline). Slower than Option A
(but this is a build-time operation, not real-time). Bake quality depends on
ray-cast accuracy between the fused TRELLIS mesh and the box primitives.

### Option C: Three.js Canvas Bake (Frontend)
- Render the original model from 6 orthographic views in the browser.
- Map rendered pixels back to parametric UV space.
- Export baked texture via canvas.toBlob().

**Pros**: No external tool dependency. Could be interactive.
**Cons**: Complex to implement correctly. Orthographic projection doesn't handle
occluded faces. Quality limited by canvas resolution. Doesn't leverage existing
Blender infrastructure. Not suitable for offline/build-time use.

## Why Option B

1. **Fulfills acceptance criteria directly**: "Bake or project textures from the
   original TRELLIS model onto the parametric reconstruction."

2. **Proven infrastructure**: Blender headless execution is already implemented
   (`blender.go`, `remesh_lod.py`). Adding a bake script follows the same
   pattern.

3. **Per-board uniqueness**: Ray-cast bake naturally gives each board unique
   texture content based on its spatial position relative to the original model.
   Boards on different sides of the raised bed will have different wood grain
   patterns.

4. **Atlas output**: Blender's bake system outputs to a single image mapped to
   the target mesh's UVs — directly produces the single shared atlas required.

5. **Quality control**: 512x512 is sufficient for the use case. At ~30-50 KB
   JPEG, the total GLB stays under 70 KB (well within budget).

## Design Details

### UV Atlas Layout

The parametric mesh has 16 boards (4 posts + 12 side boards). Each board has 6
faces, but only 4 are typically visible (top and bottom faces are largely hidden
by adjacent boards or the ground). Strategy:

- Pack each board's 4 visible faces (±X or ±Z long face, ±Y top, and 2 end
  faces) as a rectangular UV island.
- Posts get square islands. Side boards get elongated rectangular islands.
- Use a simple grid packing: 4 columns × 4 rows = 16 slots in the atlas.
- Each slot contains the unwrapped faces of one board.
- Total UV utilization: ~70-80% (acceptable for 512x512).

Actually, for simplicity and atlas efficiency, we can take a simpler approach:
each board maps its largest visible face to a unique rectangular region of the
atlas. The 4 visible faces of each board all sample from the same atlas region
but with face-appropriate UV mapping. This means we need 16 rectangular regions
in the atlas (one per board), and we let Blender bake the diffuse appearance
into each region.

### Bake Script Flow

```
1. Import original TRELLIS GLB
2. Import parametric GLB
3. Create atlas image (512x512)
4. For each board in parametric mesh:
   a. Compute UV island bounds in atlas grid (4x4 layout)
   b. Assign UVs to the board's faces mapping to that island
5. Select parametric mesh, set TRELLIS as bake source
6. Bake type=DIFFUSE with "Selected to Active" enabled
7. Save atlas as JPEG
8. Re-export parametric GLB with baked atlas
```

### Per-Board UV Variation

Even after baking, boards at symmetric positions may look similar (the TRELLIS
model is symmetric). To add variation:

- Apply a small random UV offset (0-0.1 in atlas-normalized space) per board
  before baking. This shifts each board's sampling region slightly.
- Use a seeded random based on board index for reproducibility.
- End-grain faces (board ends) can optionally have UVs rotated 90 degrees.

### Atlas Resolution Choice: 512x512

- 512x512 JPEG at quality 75: ~30-50 KB
- Total GLB with atlas: ~50-70 KB (geometry ~18 KB + atlas ~30-50 KB)
- 1024x1024 would be ~100-200 KB — overkill for a garden bed at typical
  viewing distances. The acceptance criteria say "512x512 or 1024x1024."
- We'll default to 512, with a `--atlas-size` CLI flag to allow 1024.

### Integration Points

- **New script**: `scripts/bake_textures.py` — Blender headless script.
  Invoked similarly to `remesh_lod.py`.
- **Modified script**: `scripts/parametric_reconstruct.py` — update
  `generate_box()` to support atlas UV mapping (per-board UV islands instead of
  tiling). Add `--atlas-layout` flag that outputs UV island metadata as JSON.
- **Frontend**: Add "Parametric" as a preview version in the version switcher.
  No baking in the browser — just display the pre-baked result.
- **Go server**: No changes for the core bake. Optionally add a
  `POST /api/bake-textures/:id` endpoint later, but not required for this
  ticket (baking is an offline build step).

### Visual Comparison

The acceptance criteria require "side-by-side of original vs reconstructed model
in the optimizer's preview." Implementation:

- Upload both `wood_raised_bed.glb` and `wood_raised_bed_parametric.glb` as
  separate files in the optimizer.
- Preview each one using the existing version switcher.
- No custom split-screen needed — the existing file list + preview handles this.

### Fallback Strategy

If Blender bake produces poor results (e.g., ray-cast misses due to geometry
mismatch between fused TRELLIS mesh and box primitives):

- Fall back to Option A: extract a tileable wood region from the original
  texture and apply with per-board UV offsets.
- The atlas UV layout code is still useful — just fill with tileable texture
  sampling instead of ray-cast bake.
- This fallback can be implemented as a `--mode` flag: `bake` vs `tile`.

## Rejected Alternatives

- **Substance Painter / external DCC tools**: not automatable, not in the
  existing toolchain.
- **Normal map / roughness baking**: overkill for this use case. Base color
  only is sufficient for the wood appearance.
- **Per-board separate textures**: violates single-atlas requirement and
  increases draw calls.
- **Procedural wood in shader**: would require custom shader code in Three.js,
  not portable to other renderers, defeats the purpose of texture baking.
