# T-014-02 Structure: Blender render_production.py Script

## 1. Deliverable

Single new file: `scripts/render_production.py` (~700 lines)

No existing files are modified in this ticket.

---

## 2. File Layout

```
scripts/render_production.py
├── Module docstring + usage examples
├── Imports (sys, os, argparse, math, json, struct, io)
├── bpy/bmesh/mathutils import guard
│
├── ─── Constants ───
│   ├── DEFAULT_RESOLUTION = 512
│   ├── BILLBOARD_ANGLES = 6
│   ├── TILTED_ELEVATION_RAD = math.pi / 6
│   ├── DOME_SEGMENTS = 6
│   ├── ALPHA_TEST_DEFAULT = 0.10
│   └── STRATEGY_TABLE (dict mapping category → slice_axis, dist_mode, layers)
│
├── ─── Argument Parsing ───
│   ├── parse_args()           → argparse.Namespace
│   └── load_config(path)      → dict (JSON file reader)
│   └── merge_config(args, config) → resolved params dict
│
├── ─── Scene Utilities ───
│   ├── clear_scene()          → None
│   ├── import_glb(path)       → list[bpy.types.Object]
│   ├── get_model_bounds(objects) → (center: Vector, size: Vector, min: Vector, max: Vector)
│   └── progress(msg)          → None (print to stderr)
│
├── ─── Lighting Setup ───
│   └── setup_bake_lighting(params) → None
│       Creates: 1 ambient (world shader), 2 sun lamps (key + fill),
│       hemisphere approximation via world gradient
│
├── ─── Renderer Setup ───
│   └── configure_eevee(resolution, transparent=True) → None
│       Sets: EEVEE engine, film_transparent, resolution, color management
│
├── ─── Camera Utilities ───
│   ├── create_ortho_camera(name) → bpy.types.Object
│   ├── position_side_camera(cam, center, size, angle_rad, elevation_rad=0)
│   │   → (half_w: float, half_h: float)
│   ├── position_topdown_camera(cam, center, size)
│   │   → (half_extent: float)
│   └── position_slice_camera(cam, center, size, floor_y, ceiling_y)
│       → (half_extent: float)
│
├── ─── Rendering ───
│   └── render_to_image(cam, resolution, name) → bpy.types.Image
│       Activates camera, renders, returns the Blender image datablock
│
├── ─── Quad Construction ───
│   ├── create_billboard_quad(name, width, height, image, bottom_pivot=True)
│   │   → bpy.types.Object
│   │   PlaneGeometry equivalent, UV-mapped, MeshBasicMaterial (emission shader)
│   ├── create_topdown_quad(name, size, image)
│   │   → bpy.types.Object
│   │   Flat on XZ plane (rotateX -PI/2)
│   └── create_dome_quad(name, size, dome_height, image, floor_y, segments=6)
│       → bpy.types.Object
│       PlaneGeometry with parabolic Y deformation
│
├── ─── Slice Boundary Computation ───
│   ├── compute_boundaries_equal_height(min_y, max_y, num_layers) → list[float]
│   ├── compute_boundaries_visual_density(objects, min_y, max_y, num_layers) → list[float]
│   ├── compute_boundaries_vertex_quantile(objects, min_y, max_y, num_layers) → list[float]
│   └── pick_adaptive_layer_count(size, base_layers) → int
│
├── ─── Axis Rotation ───
│   ├── resolve_slice_axis_rotation(objects, mode) → (rotation: Quaternion, inverse: Quaternion)
│   └── (identity returned for mode='y')
│
├── ─── Variant Renderers (Orchestration) ───
│   ├── render_side_billboard(objects, params) → list[bpy.types.Object]
│   │   Renders N side angles + 1 top-down, returns all quads
│   ├── render_tilted_billboard(objects, params) → list[bpy.types.Object]
│   │   Renders N tilted angles, returns all quads
│   └── render_volumetric(objects, params) → list[bpy.types.Object]
│       Computes boundaries, renders each slice, returns dome quads
│
├── ─── Export ───
│   └── export_quads_as_glb(quads, output_path, inverse_rotation=None) → None
│       Creates clean export scene, parents quads, exports as GLB
│
└── ─── Main ───
    └── main()
        1. Parse args + config
        2. Clear scene, import GLB
        3. Get bounds, resolve strategy
        4. Setup lighting + renderer
        5. Render side billboard → export {id}_billboard.glb
        6. Render tilted billboard → export {id}_billboard_tilted.glb
        7. Render volumetric → export {id}_volumetric.glb
        8. Print summary to stderr
```

---

## 3. Public Interface (CLI)

### 3.1 CLI Arguments

```
Required:
  --source PATH          Source GLB file (TRELLIS model)
  --output-dir PATH      Output directory for intermediate GLBs
  --id STRING            Asset identifier (used in output filenames)

Optional (with defaults):
  --config PATH          JSON config file (overrides below defaults)
  --category STRING      Shape category (default: "unknown")
  --resolution INT       Render resolution in pixels (default: 512)
  --billboard-angles INT Number of side billboard angles (default: 6)
  --tilted-elevation FLOAT Tilted billboard elevation in degrees (default: 30)
  --volumetric-layers INT Base volumetric layer count (default: per STRATEGY_TABLE)
  --volumetric-resolution INT Volumetric render resolution (default: 512)
  --slice-distribution-mode STRING (default: per STRATEGY_TABLE)
  --slice-axis STRING    (default: per STRATEGY_TABLE)
  --dome-height-factor FLOAT (default: 0.5)
  --alpha-test FLOAT     (default: 0.10)
  --ground-align BOOL    (default: true)
  --bake-exposure FLOAT  (default: 1.0)
  --ambient-intensity FLOAT (default: 0.5)
  --hemisphere-intensity FLOAT (default: 1.0)
  --key-light-intensity FLOAT (default: 1.4)
  --bottom-fill-intensity FLOAT (default: 0.4)
  --env-map-intensity FLOAT (default: 1.2)
  --skip-billboard       Skip side billboard rendering
  --skip-tilted          Skip tilted billboard rendering
  --skip-volumetric      Skip volumetric rendering
```

### 3.2 JSON Config Schema

```json
{
  "source": "/path/to/model.glb",
  "output_dir": "/path/to/output/",
  "id": "asset_id",
  "category": "round-bush",
  "resolution": 512,
  "billboard_angles": 6,
  "tilted_elevation": 30,
  "volumetric_layers": 4,
  "volumetric_resolution": 512,
  "slice_distribution_mode": "visual-density",
  "slice_axis": "y",
  "dome_height_factor": 0.5,
  "alpha_test": 0.10,
  "ground_align": true,
  "bake_exposure": 1.0,
  "ambient_intensity": 0.5,
  "hemisphere_intensity": 1.0,
  "key_light_intensity": 1.4,
  "bottom_fill_intensity": 0.4,
  "env_map_intensity": 1.2
}
```

### 3.3 Exit Codes

| Code | Meaning |
|------|---------|
| 0 | Success — all requested GLBs produced |
| 1 | Error — import failure, render failure, or invalid arguments |
| 2 | Partial success — some variants failed (logged to stderr) |

---

## 4. Output Files

| File | Contents | Mesh Count |
|------|----------|------------|
| `{id}_billboard.glb` | N side quads + 1 top quad | N+1 (default: 7) |
| `{id}_billboard_tilted.glb` | N tilted quads | N (default: 6) |
| `{id}_volumetric.glb` | M dome-slice quads | M (adaptive) |

Each GLB contains:
- Named mesh objects (naming matches CombinePack expectations)
- Each mesh has a single material with embedded RGBA PNG texture
- Textures at the specified resolution (default 512x512)
- Quads sized to match the Three.js sizing formulas

---

## 5. Internal Module Boundaries

### Scene Layer (clear_scene, import_glb, get_model_bounds)

Handles Blender scene management. No rendering logic. Pure Blender API.

### Lighting Layer (setup_bake_lighting)

Creates and configures lights. Reads intensity parameters from the resolved config.
Stateless — called once per variant render cycle (or once if scene is reused).

### Camera Layer (create_ortho_camera, position_*)

Creates and positions orthographic cameras. Returns the sizing data needed for quad
construction. Pure geometry math + Blender camera API.

### Render Layer (render_to_image, configure_eevee)

Handles the actual EEVEE render call. Returns Blender image datablocks.

### Geometry Layer (create_*_quad, create_dome_quad)

Constructs quad meshes with UV mapping and materials. The dome quad includes the
parabolic deformation logic. Each function returns a Blender object ready for export.

### Boundary Layer (compute_boundaries_*, pick_adaptive_layer_count, resolve_slice_axis_rotation)

Pure math functions (except `resolve_slice_axis_rotation` which reads mesh data).
No Blender rendering API — just vertex data access and computation.

### Orchestration Layer (render_side_billboard, render_tilted_billboard, render_volumetric)

Composes the lower layers. Each function: set up camera → render → build quad → repeat.
Returns a list of quad objects for export.

### Export Layer (export_quads_as_glb)

Creates a clean scene with only the quad objects, applies any inverse rotation,
exports as GLB via `bpy.ops.export_scene.gltf()`.

---

## 6. Key Implementation Notes

### 6.1 Material Strategy

Billboard quads use `MeshBasicMaterial` in Three.js (unlit). In Blender, the closest
equivalent for GLB export is a Principled BSDF with:
- Base Color connected to the rendered texture image
- Metallic = 0, Roughness = 1, Specular = 0
- Alpha connected to the texture alpha channel
- Blend mode = ALPHA_CLIP or ALPHA_BLEND

When exported as GLB via Blender's glTF exporter, this produces a `pbrMetallicRoughness`
material with `baseColorTexture` — which is what CombinePack expects.

### 6.2 Texture Embedding

Rendered images are saved to Blender's image datablock (in-memory), then referenced
by the material. The glTF exporter embeds them as PNG in the GLB binary.

### 6.3 Ground Alignment for Volumetric

When `ground_align=True`, the export scene root is shifted by `-boundaries[0]` on Y.
This matches the Three.js `exportScene.position.y = -boundaries[0]`.

### 6.4 Axis Rotation for Volumetric

For `auto-horizontal` and `auto-thin` slice axes, the model is rotated before slicing
and the inverse rotation is applied to the export scene. Uses `mathutils.Quaternion`
with `rotation_difference()` between the picked axis and +Y.
