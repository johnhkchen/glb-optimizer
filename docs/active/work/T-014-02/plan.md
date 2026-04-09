# T-014-02 Plan: Blender render_production.py Script

## Step 1: Scaffold — Imports, Constants, Arg Parsing

Write the file header, all imports (with bpy guard), constants (BILLBOARD_ANGLES,
TILTED_ELEVATION_RAD, DOME_SEGMENTS, STRATEGY_TABLE, defaults), `parse_args()`,
`load_config()`, `merge_config()`, and the `progress()` utility.

**Verify:** `python3 scripts/render_production.py --help` prints usage (outside
Blender, the bpy import guard fires but argparse section can be tested separately).

**Commit:** "T-014-02: scaffold render_production.py with arg parsing and constants"

---

## Step 2: Scene Utilities — clear_scene, import_glb, get_model_bounds

Implement `clear_scene()` (matching existing scripts), `import_glb(path)` that calls
`bpy.ops.import_scene.gltf()` and returns the imported objects, and `get_model_bounds()`
that computes the combined world-space AABB of all mesh objects.

**Verify:** Can be tested by running the script with a real GLB in Blender:
`blender -b --python scripts/render_production.py -- --source inbox/dahlia_blush.glb --output-dir /tmp --id test`
(will fail at render step but import + bounds should print to stderr)

**Commit:** "T-014-02: scene utilities — import, bounds, clear"

---

## Step 3: Lighting + Renderer Configuration

Implement `configure_eevee(resolution, transparent)` — sets render engine to EEVEE,
enables film_transparent, sets resolution, configures color management (Filmic view
transform, exposure from params).

Implement `setup_bake_lighting(params)` — creates world ambient, two sun lamps
(key at (0,10,0), fill at (0,-10,0)), sets intensities from params.

**Verify:** Manual visual check by rendering a test frame.

**Commit:** "T-014-02: EEVEE renderer and lighting setup"

---

## Step 4: Camera Utilities

Implement `create_ortho_camera(name)`, `position_side_camera()`, `position_topdown_camera()`,
and `position_slice_camera()`. Each function sizes the orthographic frustum using the
exact formulas from `production-render-params.md`:

- Side: `halfW = max(size.x, size.z) * 0.55`, `halfH = size.y * 0.55`
- Tilted: incorporates `cos(elevation)` and `sin(elevation)` terms
- Top-down: `half = max(size.x * 0.55, size.z * 0.55)`
- Slice: `halfExtent = max(size.x, size.z) * 0.55`, clip to floor/ceiling

**Verify:** Print camera parameters for the test model, compare against expected
values from the JS pipeline.

**Commit:** "T-014-02: orthographic camera positioning utilities"

---

## Step 5: Render-to-Image + Quad Construction

Implement `render_to_image(cam, resolution, name)` — sets active camera, calls
`bpy.ops.render.render()`, returns the rendered image datablock.

Implement `create_billboard_quad(name, width, height, image, bottom_pivot=True)` —
creates a plane mesh, UV-maps it, creates a Principled BSDF material with the
rendered image as base color texture, sets alpha blend mode.

Implement `create_topdown_quad(name, size, image)` — same but rotated flat on XZ.

**Verify:** Render one side billboard angle for the test model, export just that
quad as GLB, open in glTF viewer to check texture and sizing.

**Commit:** "T-014-02: render-to-image and billboard quad construction"

---

## Step 6: Side Billboard Orchestration + Export

Implement `render_side_billboard(objects, params)` — loops over N angles, renders
each, builds quads named `billboard_0`..`billboard_{N-1}`, adds top-down as
`billboard_top`. Returns list of quad objects.

Implement `export_quads_as_glb(quads, output_path)` — creates a clean scene, links
quads, exports via `bpy.ops.export_scene.gltf(export_format='GLB')`.

Wire into `main()` to produce `{id}_billboard.glb`.

**Verify:** Run full side billboard render on `dahlia_blush.glb`. Check:
- Output file exists and is valid GLB
- Contains 7 named meshes
- Each mesh has a baseColorTexture
- File size is reasonable (expected: a few hundred KB to ~2MB)

**Commit:** "T-014-02: side billboard rendering and GLB export"

---

## Step 7: Tilted Billboard

Implement `render_tilted_billboard(objects, params)` — same loop as side but with
elevation parameter. No top-down quad. Naming: `billboard_0`..`billboard_{N-1}`.

Wire into `main()` to produce `{id}_billboard_tilted.glb`.

**Verify:** Run tilted render. Check 6 meshes, correct naming, reasonable file size.

**Commit:** "T-014-02: tilted billboard rendering"

---

## Step 8: Slice Boundary Computation

Implement the three boundary algorithms:
- `compute_boundaries_equal_height(min_y, max_y, num_layers)` — linear interpolation
- `compute_boundaries_visual_density(objects, min_y, max_y, num_layers)` — trunk-filter,
  radial weight, weighted quantile
- `compute_boundaries_vertex_quantile(objects, min_y, max_y, num_layers)` — sorted Y percentiles

Implement `pick_adaptive_layer_count(size, base_layers)` — aspect ratio check.

Implement `resolve_slice_axis_rotation(objects, mode)` — returns rotation quaternion pair.

**Verify:** Print computed boundaries for the test model with each algorithm. Compare
layer count against expected (4 for round-bush default).

**Commit:** "T-014-02: slice boundary computation (equal-height, visual-density, vertex-quantile)"

---

## Step 9: Dome Geometry + Volumetric Rendering

Implement `create_dome_quad(name, size, dome_height, image, floor_y, segments)` —
creates subdivided plane, applies parabolic Y deformation, UV-maps, applies material.

Implement `render_volumetric(objects, params)` — resolves axis rotation, computes
boundaries, renders each slice top-down with clipping, builds dome quads named
`vol_layer_{i}_h{baseMm}`, applies ground alignment.

Wire into `main()` to produce `{id}_volumetric.glb`.

**Verify:** Run volumetric render. Check:
- Correct number of dome slice meshes (adaptive count)
- Naming pattern matches `vol_layer_*` regex
- Dome geometry has parabolic vertex heights
- Ground alignment shifts Y correctly

**Commit:** "T-014-02: volumetric dome slice rendering"

---

## Step 10: Main Orchestration + JSON Config

Wire up `main()` to run all three variant renderers in sequence. Add `--config`
JSON loading with CLI override support. Add `--skip-*` flags. Add progress output.
Add error handling with appropriate exit codes.

**Verify:** Full end-to-end run:
```
blender -b --python scripts/render_production.py -- \
    --source inbox/dahlia_blush.glb \
    --output-dir /tmp/test-render \
    --id dahlia_blush \
    --category round-bush
```

Check all three output GLBs exist, have correct mesh counts and naming.

**Commit:** "T-014-02: main orchestration with JSON config support"

---

## Step 11: Smoke Test Against CombinePack

Run the produced GLBs through the Go pipeline:
1. Copy output GLBs to the expected location
2. Run `glb-optimizer pack <id>` (if the CLI supports it)
3. Or manually test via the API

If CombinePack rejects the files, debug mesh naming or structure issues.

**Verify:** CombinePack successfully processes all three intermediates into a pack.

**Commit:** (fix commits only if issues found)

---

## Testing Strategy

### Manual Testing (Primary)

This is a Blender script — the primary testing mode is running it with real GLB
models and verifying the output visually and structurally.

Test model: `inbox/dahlia_blush.glb`
Reference output: known-good `1e562361...` intermediates (file size comparison)

### Structural Validation

After rendering, inspect output GLBs:
- Valid GLB format (can be loaded by any glTF viewer)
- Correct mesh count and naming
- Each mesh has a baseColorTexture with RGBA data at specified resolution
- Quad dimensions match expected values (from bounding box + 0.55 factor)

### Integration Testing

- `CombinePack` accepts the Blender-produced intermediates
- `verify-pack.mjs` passes on the resulting pack
- File sizes within 2x of known-good reference

### No Unit Tests for This Ticket

Blender Python scripts cannot be unit-tested outside Blender (they depend on `bpy`).
The existing scripts (`bake_textures.py`, `remesh_lod.py`) also have no unit tests.
`classify_shape.py` has tests but it's a standalone script (no Blender dependency).
