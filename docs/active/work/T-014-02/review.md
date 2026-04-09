# T-014-02 Review: Blender render_production.py Script

## 1. Summary of Changes

### Files Created
| File | Lines | Purpose |
|------|-------|---------|
| `scripts/render_production.py` | 1026 | Headless Blender Python script for production impostor rendering |

### Files Modified
None.

### Files Deleted
None.

---

## 2. What the Script Does

`render_production.py` is a Blender headless script that renders four impostor variant
types for one GLB asset, producing three intermediate GLB files:

1. **Side billboard GLB** (`{id}_billboard.glb`): 6 side-angle quads + 1 top-down quad (7 total)
2. **Tilted billboard GLB** (`{id}_billboard_tilted.glb`): 6 tilted-angle quads (30deg elevation)
3. **Volumetric GLB** (`{id}_volumetric.glb`): M dome-slice quads with parabolic Y deformation

The script accepts CLI arguments or a JSON config file, making it suitable for both
manual use and Go server integration (T-014-03/T-014-04).

---

## 3. Architecture

28 functions organized in 8 logical layers:

| Layer | Functions | Responsibility |
|-------|-----------|----------------|
| Arg parsing | parse_args, load_config, merge_config | CLI + JSON config, strategy table resolution |
| Scene utils | clear_scene, import_glb, get_model_bounds | Blender scene lifecycle |
| Renderer | configure_eevee | EEVEE setup, color management, transparency |
| Lighting | setup_bake_lighting | World ambient + key/fill sun lamps |
| Camera | create_ortho_camera, position_side/topdown/slice_camera | Orthographic frustum sizing |
| Rendering | render_to_image | Render dispatch + image capture |
| Geometry | create_billboard/topdown/dome_quad, create_material_for_image | Quad mesh + material construction |
| Boundaries | compute_boundaries_* (3 modes), pick_adaptive_layer_count, resolve_slice_axis_rotation | Volumetric slice computation |
| Orchestration | render_side/tilted_billboard, render_volumetric | Per-variant render loops |
| Export | export_quads_as_glb | GLB output with optional rotation wrapper |
| Main | main | CLI entry point, full pipeline |

---

## 4. Key Formulas — Parity with Three.js

All camera/quad sizing formulas were translated directly from the production-render-params
reference document (T-014-01):

| Formula | Source | Implementation |
|---------|--------|----------------|
| `halfW = max(size.x, size.z) * 0.55` | app.js L1412 | `position_side_camera()` |
| `halfH = size.y * 0.55` (side) | app.js L1411 | `position_side_camera()` |
| `halfH = (size.y*cosE + maxH*sinE) * 0.55` (tilted) | app.js L1411 | `position_side_camera(elevation_rad=...)` |
| `half = max(size.x*0.55, size.z*0.55)` (top) | app.js L1774-1776 | `position_topdown_camera()` |
| `halfExtent = max(size.x, size.z) * 0.55` (slice) | app.js L1961 | `position_slice_camera()` |
| `y = (1 - dist^2) * domeHeight` | app.js L2037 | `create_dome_quad()` |
| `heightToWidth > 2.5: +2, >1.5: +1` | app.js L2189 | `pick_adaptive_layer_count()` |
| Trunk filter at 10%, radial weight | app.js L2073 | `compute_boundaries_visual_density()` |

---

## 5. Mesh Naming — CombinePack Compatibility

| Variant | Mesh Names | CombinePack Route |
|---------|------------|-------------------|
| Side | `billboard_0`..`billboard_5`, `billboard_top` | `routeSideMeshes()` → `view_side` + `view_top` |
| Tilted | `billboard_0`..`billboard_5` | `routeTiltedMeshes()` → `view_tilted` |
| Volumetric | `vol_layer_{i}_h{mm}` | `routeVolumetricMeshes()` — Y-sorted, renamed to `slice_N` |

CombinePack routes side meshes by checking for `billboard_top` by exact name; all
other meshes go to `view_side`. Tilted meshes are taken as-is. Volumetric meshes are
sorted by their POSITION accessor min-Y, so the original naming is informational only.

---

## 6. Test Coverage

### Automated Tests
- **Python syntax check**: PASSED (ast.parse)
- **No unit tests**: Blender Python scripts cannot be unit-tested outside Blender.
  This is consistent with the existing scripts (bake_textures.py, remesh_lod.py have
  no unit tests either).

### Manual Testing Required
1. **Import test**: `blender -b --python scripts/render_production.py -- --source inbox/dahlia_blush.glb --output-dir /tmp/test --id test --category round-bush`
   - Verify TRELLIS GLB imports successfully
   - Verify three output GLBs are produced

2. **Structural validation**: Open each output GLB in a glTF viewer
   - Billboard: 7 meshes, correct naming, 512x512 textures
   - Tilted: 6 meshes, correct naming
   - Volumetric: N dome-slice meshes with dome geometry

3. **CombinePack integration**: Run `glb-optimizer pack <id>` on the output
   - CombinePack should successfully merge all three intermediates

4. **verify-pack.mjs**: Run on the resulting pack for structural validation

5. **File size comparison**: Compare against known-good `1e562361...` intermediates (within 2x)

---

## 7. Open Concerns

### 7.1 TRELLIS GLB Import (BLOCKER — untested)
The ticket flags this as an early blocker. If Blender cannot import TRELLIS-format GLBs,
the entire approach fails. Must be tested with `inbox/dahlia_blush.glb` before proceeding
to downstream tickets (T-014-03, T-014-04).

### 7.2 Lighting Visual Parity (LOW — tuning needed)
The Blender lighting rig (world ambient + 2 sun lamps) is an approximation of the
Three.js 4-light setup (ambient + hemisphere + key directional + fill directional).
Visual output will differ. T-014-06 (validation) should compare textures and tune
the lighting intensities if needed.

### 7.3 Render Result Pixel Copy (LOW — performance)
`render_to_image()` copies pixels from `Render Result` to a new image via
`img.pixels.foreach_set(result.pixels[:])`. The `result.pixels[:]` creates a full copy
of the pixel buffer. For 512x512 RGBA this is ~1MB — fast. For 2048x2048 it would be
~16MB, which is still manageable but slower.

### 7.4 EEVEE Version Detection (LOW — compatibility)
The script uses `BLENDER_EEVEE_NEXT` for Blender 4.x+ and `BLENDER_EEVEE` for 3.x.
This should be tested on the specific Blender version installed on the target machine.

### 7.5 Material Blend Mode (MEDIUM — alpha handling)
The script uses `blend_method = 'CLIP'` (alpha clip) on the material. The glTF exporter
should translate this to `alphaMode: "MASK"` in the GLB. If CombinePack expects
`alphaMode: "BLEND"` instead, the material setup may need adjustment.

### 7.6 Dome Geometry Subdivision (LOW — precision)
The `bmesh.ops.subdivide_edges()` subdivision may produce a slightly different vertex
layout than Three.js `PlaneGeometry(size, size, segments, segments)`. The parabolic
deformation should still be correct since it operates on vertex positions, but the
exact triangle topology may differ.

---

## 8. Relationship to Other Tickets

| Ticket | Relationship | Status |
|--------|-------------|--------|
| T-014-01 | Dependency (params doc) | Done |
| T-014-02 | **This ticket** | Implementing |
| T-014-03 | Downstream (Go API endpoint) | Will call this script |
| T-014-04 | Downstream (CLI subcommand) | Will wrap this script |
| T-014-05 | Downstream (UI button) | Delegates to T-014-03 API |
| T-014-06 | Downstream (validation) | Will compare Blender vs JS output |

---

## 9. TODOs for Follow-Up

- [ ] Test TRELLIS GLB import in Blender (blocker)
- [ ] Manual render test with `dahlia_blush.glb`
- [ ] CombinePack integration test
- [ ] Lighting intensity tuning (T-014-06)
- [ ] Consider `//go:embed` of this script in a future ticket (like remesh_lod.py)
