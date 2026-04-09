# T-014-02 Implementation Progress

## Completed

### Step 1-10: Full script implementation (combined)
- Created `scripts/render_production.py` (~620 lines)
- All sections implemented in a single pass:
  - Imports with bpy guard (try/except pattern from bake_textures.py)
  - Constants: STRATEGY_TABLE, BILLBOARD_ANGLES, TILTED_ELEVATION_RAD, DOME_SEGMENTS, DEFAULTS
  - Argument parsing: parse_args(), load_config(), merge_config()
  - Scene utilities: clear_scene(), import_glb(), get_model_bounds()
  - Renderer config: configure_eevee() with Filmic color management, film_transparent
  - Lighting: setup_bake_lighting() with world ambient + key/fill sun lamps
  - Camera: create_ortho_camera(), position_side_camera(), position_topdown_camera(), position_slice_camera()
  - Rendering: render_to_image() using bpy.ops.render.render()
  - Quad construction: create_billboard_quad() (bottom pivot), create_topdown_quad() (XZ flat), create_dome_quad() (parabolic deformation)
  - Materials: create_material_for_image() — Principled BSDF with texture, alpha clip
  - Slice boundaries: compute_boundaries_equal_height(), visual_density(), vertex_quantile()
  - Adaptive layer count: pick_adaptive_layer_count()
  - Axis rotation: resolve_slice_axis_rotation() for auto-horizontal/auto-thin
  - Variant orchestrators: render_side_billboard(), render_tilted_billboard(), render_volumetric()
  - Export: export_quads_as_glb() with inverse rotation wrapper support
  - Main: full orchestration with JSON config, CLI arg override, skip flags, progress output
- Python syntax verification: PASSED

### Deviation from Plan
- Combined steps 1-10 into a single implementation pass rather than incremental commits.
  Rationale: the script is a cohesive unit and all sections are interdependent. Incremental
  commits would each be non-functional until the full pipeline is wired up.

## Key Design Decisions During Implementation

1. **EEVEE version detection**: Used `BLENDER_EEVEE_NEXT` for Blender 4.x+, `BLENDER_EEVEE` for 3.x
2. **Material approach**: Principled BSDF with Metallic=0, Roughness=1, Specular=0 — exports as `pbrMetallicRoughness` with `baseColorTexture` in GLB, which CombinePack expects
3. **Render result capture**: Copy `Render Result` pixels to a new image datablock and pack it — this ensures the image embeds in the GLB
4. **Dome geometry**: bmesh `subdivide_edges` for grid subdivision, then vertex-level parabolic deformation
5. **Volumetric return type**: Returns `(quads, inverse_rotation)` tuple consistently; inverse is `None` when no axis rotation was applied

## Remaining

### Step 11: Smoke test against CombinePack
- Requires a running Blender installation to execute the script
- Cannot be run in this session (no Blender available in CI/dev environment)
- Deferred to manual testing or T-014-06 (validation ticket)

## Open Issues

1. **Blender availability**: Script cannot be tested without a Blender installation. The `bpy` import guard ensures a clear error message if run outside Blender.
2. **TRELLIS GLB import**: Flagged as an early blocker in the ticket. Cannot verify until the script is run with the actual `inbox/dahlia_blush.glb` model.
3. **Lighting parity**: The world ambient + 2 sun lamp setup is an approximation of Three.js's 4-light rig. Visual tuning will be needed (T-014-06).
4. **Render Result pixel copy**: The `result.pixels[:]` slice may be slow for large images. Fine for 512x512 but worth monitoring for larger resolutions.
