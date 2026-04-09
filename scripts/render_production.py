#!/usr/bin/env python3
"""
Production impostor renderer for glb-optimizer (Blender headless).

Renders four impostor variants for one GLB asset: side billboards, top-down billboard,
tilted billboards, and volumetric dome slices. Produces three intermediate GLB files
that feed into CombinePack.

Usage:
    blender -b --python scripts/render_production.py -- \
        --source ~/.glb-optimizer/outputs/{id}.glb \
        --output-dir ~/.glb-optimizer/outputs/ \
        --id {id} \
        --category round-bush

    # Or with JSON config:
    blender -b --python scripts/render_production.py -- --config params.json

Output files:
    {output-dir}/{id}_billboard.glb       — N side quads + billboard_top
    {output-dir}/{id}_billboard_tilted.glb — N tilted quads
    {output-dir}/{id}_volumetric.glb      — M dome-slice quads

Parameters reference: docs/knowledge/production-render-params.md
"""

import sys
import os
import argparse
import math
import json
import time

try:
    import bpy
    import bmesh
    from mathutils import Vector, Quaternion, Matrix
except ImportError:
    print("ERROR: This script must be run inside Blender.", file=sys.stderr)
    print("  blender -b --python scripts/render_production.py -- [args]", file=sys.stderr)
    sys.exit(1)


# ---------------------------------------------------------------------------
# Constants
# ---------------------------------------------------------------------------

DEFAULT_RESOLUTION = 512
BILLBOARD_ANGLES = 6
TILTED_ELEVATION_RAD = math.pi / 6  # 30 degrees
DOME_SEGMENTS = 6
PADDING_FACTOR = 0.55

# Strategy table — mirrors app.js STRATEGY_TABLE (L428-434)
STRATEGY_TABLE = {
    "round-bush":  {"slice_axis": "y",               "slice_distribution_mode": "visual-density", "volumetric_layers": 4},
    "directional": {"slice_axis": "auto-horizontal",  "slice_distribution_mode": "equal-height",   "volumetric_layers": 4},
    "tall-narrow": {"slice_axis": "y",               "slice_distribution_mode": "equal-height",   "volumetric_layers": 6},
    "planar":      {"slice_axis": "auto-thin",        "slice_distribution_mode": "equal-height",   "volumetric_layers": 3},
    "hard-surface": {"slice_axis": "n/a",             "slice_distribution_mode": "n/a",            "volumetric_layers": 0},
    "unknown":     {"slice_axis": "y",               "slice_distribution_mode": "visual-density", "volumetric_layers": 4},
}

DEFAULTS = {
    "category": "unknown",
    "resolution": 512,
    "billboard_angles": 6,
    "tilted_elevation": 30,
    "volumetric_layers": None,  # resolved from STRATEGY_TABLE
    "volumetric_resolution": 512,
    "slice_distribution_mode": None,  # resolved from STRATEGY_TABLE
    "slice_axis": None,  # resolved from STRATEGY_TABLE
    "dome_height_factor": 0.5,
    "alpha_test": 0.10,
    "ground_align": True,
    "bake_exposure": 1.0,
    "ambient_intensity": 0.5,
    "hemisphere_intensity": 1.0,
    "key_light_intensity": 1.4,
    "bottom_fill_intensity": 0.4,
    "env_map_intensity": 1.2,
}


# ---------------------------------------------------------------------------
# Argument Parsing
# ---------------------------------------------------------------------------

def parse_args():
    """Parse arguments after the '--' separator in Blender's argv."""
    argv = sys.argv
    if "--" in argv:
        argv = argv[argv.index("--") + 1:]
    else:
        argv = []

    p = argparse.ArgumentParser(description="Render production impostors for a GLB asset")
    p.add_argument("--source", help="Source GLB file path")
    p.add_argument("--output-dir", help="Output directory for intermediate GLBs")
    p.add_argument("--id", help="Asset identifier (used in output filenames)")
    p.add_argument("--config", help="JSON config file (all params; CLI args override)")

    p.add_argument("--category", default=None)
    p.add_argument("--resolution", type=int, default=None)
    p.add_argument("--billboard-angles", type=int, default=None)
    p.add_argument("--tilted-elevation", type=float, default=None, help="Degrees")
    p.add_argument("--volumetric-layers", type=int, default=None)
    p.add_argument("--volumetric-resolution", type=int, default=None)
    p.add_argument("--slice-distribution-mode", default=None)
    p.add_argument("--slice-axis", default=None)
    p.add_argument("--dome-height-factor", type=float, default=None)
    p.add_argument("--alpha-test", type=float, default=None)
    p.add_argument("--ground-align", type=str, default=None, help="true/false")
    p.add_argument("--bake-exposure", type=float, default=None)
    p.add_argument("--ambient-intensity", type=float, default=None)
    p.add_argument("--hemisphere-intensity", type=float, default=None)
    p.add_argument("--key-light-intensity", type=float, default=None)
    p.add_argument("--bottom-fill-intensity", type=float, default=None)
    p.add_argument("--env-map-intensity", type=float, default=None)

    p.add_argument("--skip-billboard", action="store_true")
    p.add_argument("--skip-tilted", action="store_true")
    p.add_argument("--skip-volumetric", action="store_true")

    return p.parse_args(argv)


def load_config(path):
    """Load JSON config file, converting kebab-case keys to snake_case."""
    with open(path, "r") as f:
        raw = json.load(f)
    out = {}
    for k, v in raw.items():
        out[k.replace("-", "_")] = v
    return out


def merge_config(args):
    """Merge defaults, strategy table, JSON config, and CLI args into one dict."""
    params = dict(DEFAULTS)

    # Load JSON config if provided
    if args.config:
        cfg = load_config(args.config)
        for k, v in cfg.items():
            if v is not None:
                params[k] = v

    # CLI args override (only non-None values)
    cli_map = {
        "source": args.source,
        "output_dir": getattr(args, "output_dir", None),
        "id": args.id,
        "category": args.category,
        "resolution": args.resolution,
        "billboard_angles": getattr(args, "billboard_angles", None),
        "tilted_elevation": getattr(args, "tilted_elevation", None),
        "volumetric_layers": getattr(args, "volumetric_layers", None),
        "volumetric_resolution": getattr(args, "volumetric_resolution", None),
        "slice_distribution_mode": getattr(args, "slice_distribution_mode", None),
        "slice_axis": getattr(args, "slice_axis", None),
        "dome_height_factor": getattr(args, "dome_height_factor", None),
        "alpha_test": getattr(args, "alpha_test", None),
        "ground_align": getattr(args, "ground_align", None),
        "bake_exposure": getattr(args, "bake_exposure", None),
        "ambient_intensity": getattr(args, "ambient_intensity", None),
        "hemisphere_intensity": getattr(args, "hemisphere_intensity", None),
        "key_light_intensity": getattr(args, "key_light_intensity", None),
        "bottom_fill_intensity": getattr(args, "bottom_fill_intensity", None),
        "env_map_intensity": getattr(args, "env_map_intensity", None),
    }
    for k, v in cli_map.items():
        if v is not None:
            params[k] = v

    # Handle ground_align string → bool
    if isinstance(params.get("ground_align"), str):
        params["ground_align"] = params["ground_align"].lower() in ("true", "1", "yes")

    # Resolve strategy table defaults for unset fields
    category = params.get("category", "unknown")
    strategy = STRATEGY_TABLE.get(category, STRATEGY_TABLE["unknown"])
    if params.get("volumetric_layers") is None:
        params["volumetric_layers"] = strategy["volumetric_layers"]
    if params.get("slice_distribution_mode") is None:
        params["slice_distribution_mode"] = strategy["slice_distribution_mode"]
    if params.get("slice_axis") is None:
        params["slice_axis"] = strategy["slice_axis"]

    # Convert tilted_elevation from degrees to radians for internal use
    params["tilted_elevation_rad"] = math.radians(params.get("tilted_elevation", 30))

    # Validate required fields
    for field in ("source", "output_dir", "id"):
        if not params.get(field):
            print(f"ERROR: --{field.replace('_', '-')} is required", file=sys.stderr)
            sys.exit(1)

    return params


# ---------------------------------------------------------------------------
# Utilities
# ---------------------------------------------------------------------------

def progress(msg):
    """Print a progress message to stderr."""
    print(f"[render_production] {msg}", file=sys.stderr)


def clear_scene():
    """Remove all objects, materials, and images from the scene."""
    bpy.ops.object.select_all(action='SELECT')
    bpy.ops.object.delete(use_global=False)
    for block in bpy.data.meshes:
        bpy.data.meshes.remove(block)
    for block in bpy.data.materials:
        bpy.data.materials.remove(block)
    for block in bpy.data.images:
        bpy.data.images.remove(block)
    for block in bpy.data.cameras:
        bpy.data.cameras.remove(block)
    for block in bpy.data.lights:
        bpy.data.lights.remove(block)


def import_glb(filepath):
    """Import a GLB file and return the imported mesh objects."""
    before = set(bpy.data.objects.keys())
    bpy.ops.import_scene.gltf(filepath=os.path.abspath(filepath))
    after = set(bpy.data.objects.keys())
    new_names = after - before
    objects = [bpy.data.objects[n] for n in new_names]
    meshes = [o for o in objects if o.type == 'MESH']
    if not meshes:
        print(f"ERROR: No mesh objects found in {filepath}", file=sys.stderr)
        sys.exit(1)
    progress(f"Imported {len(meshes)} mesh objects from {os.path.basename(filepath)}")
    return meshes


def get_model_bounds(objects):
    """Compute combined world-space AABB of all mesh objects.

    Returns (center, size, bbox_min, bbox_max) as Vectors.
    """
    all_min = Vector((float('inf'), float('inf'), float('inf')))
    all_max = Vector((float('-inf'), float('-inf'), float('-inf')))

    for obj in objects:
        if obj.type != 'MESH':
            continue
        for corner in obj.bound_box:
            world_co = obj.matrix_world @ Vector(corner)
            for i in range(3):
                all_min[i] = min(all_min[i], world_co[i])
                all_max[i] = max(all_max[i], world_co[i])

    size = all_max - all_min
    center = (all_min + all_max) / 2.0
    return center, size, all_min, all_max


# ---------------------------------------------------------------------------
# Renderer & Lighting
# ---------------------------------------------------------------------------

def configure_renderer(resolution, params):
    """Configure the render engine with transparent background.

    Tries EEVEE first (fast). If EEVEE produces empty pixels in
    headless mode (no GPU context on macOS -b), falls back to Cycles
    CPU which works headlessly on all platforms.
    """
    scene = bpy.context.scene

    # Use Cycles in headless / background mode — EEVEE needs a GPU
    # context that isn't available when running `blender -b`.
    # Cycles CPU is ~5-10s per 512px frame, acceptable for batch.
    if bpy.app.background:
        scene.render.engine = 'CYCLES'
        scene.cycles.device = 'CPU'
        scene.cycles.samples = 64  # low sample count — billboard textures don't need noise-free GI
        scene.cycles.use_denoising = True
        print("[render_production] Using Cycles CPU (headless mode)")
    else:
        # Interactive / headed mode — prefer EEVEE for speed.
        for engine_name in ('BLENDER_EEVEE', 'BLENDER_EEVEE_NEXT'):
            try:
                scene.render.engine = engine_name
                break
            except TypeError:
                continue
        print(f"[render_production] Using {scene.render.engine}")

    scene.render.resolution_x = resolution
    scene.render.resolution_y = resolution
    scene.render.resolution_percentage = 100
    scene.render.film_transparent = True
    scene.render.image_settings.file_format = 'PNG'
    scene.render.image_settings.color_mode = 'RGBA'

    # Color management
    scene.view_settings.view_transform = 'Filmic'
    scene.view_settings.look = 'None'
    scene.view_settings.exposure = params.get("bake_exposure", 1.0)

    # EEVEE-specific
    if hasattr(scene, 'eevee'):
        if hasattr(scene.eevee, 'use_bloom'):
            scene.eevee.use_bloom = False


def setup_bake_lighting(params):
    """Set up lighting rig matching the Three.js bake pipeline.

    Creates:
    - World ambient (approximates AmbientLight + HemisphereLight)
    - Key sun lamp from above (directional top key)
    - Fill sun lamp from below (directional bottom fill)
    """
    # World ambient — approximates AmbientLight + HemisphereLight contribution
    world = bpy.data.worlds.get("World") or bpy.data.worlds.new("World")
    bpy.context.scene.world = world
    world.use_nodes = True
    nodes = world.node_tree.nodes
    links = world.node_tree.links
    nodes.clear()

    bg = nodes.new(type='ShaderNodeBackground')
    output = nodes.new(type='ShaderNodeOutputWorld')
    # Ambient contribution: blend of ambient and hemisphere intensities
    ambient = params.get("ambient_intensity", 0.5)
    hemisphere = params.get("hemisphere_intensity", 1.0)
    bg.inputs['Strength'].default_value = (ambient + hemisphere) * 0.5
    bg.inputs['Color'].default_value = (0.9, 0.9, 0.9, 1.0)
    links.new(bg.outputs['Background'], output.inputs['Surface'])

    # Key light — sun from above
    key_data = bpy.data.lights.new(name="KeyLight", type='SUN')
    key_data.energy = params.get("key_light_intensity", 1.4)
    key_obj = bpy.data.objects.new("KeyLight", key_data)
    bpy.context.collection.objects.link(key_obj)
    key_obj.location = (0, 10, 0)
    key_obj.rotation_euler = (math.radians(-90), 0, 0)  # Point downward

    # Fill light — sun from below
    fill_data = bpy.data.lights.new(name="FillLight", type='SUN')
    fill_data.energy = params.get("bottom_fill_intensity", 0.4)
    fill_obj = bpy.data.objects.new("FillLight", fill_data)
    bpy.context.collection.objects.link(fill_obj)
    fill_obj.location = (0, -10, 0)
    fill_obj.rotation_euler = (math.radians(90), 0, 0)  # Point upward


# ---------------------------------------------------------------------------
# Camera
# ---------------------------------------------------------------------------

def create_ortho_camera(name="RenderCam"):
    """Create an orthographic camera and add it to the scene."""
    cam_data = bpy.data.cameras.new(name)
    cam_data.type = 'ORTHO'
    cam_obj = bpy.data.objects.new(name, cam_data)
    bpy.context.collection.objects.link(cam_obj)
    return cam_obj


def position_side_camera(cam_obj, center, size, angle_rad, elevation_rad=0):
    """Position camera for a side billboard render.

    Returns (half_w, half_h) for quad sizing.
    Matches app.js renderBillboardAngle() (L1392).
    """
    max_horiz = max(size.x, size.z)
    cos_e = math.cos(elevation_rad)
    sin_e = math.sin(elevation_rad)

    half_w = max_horiz * PADDING_FACTOR
    half_h = (size.y * cos_e + max_horiz * sin_e) * PADDING_FACTOR

    max_dim = max(size.x, size.y, size.z)
    dist = max_dim * 2

    cam_obj.location = Vector((
        center.x + math.sin(angle_rad) * dist * cos_e,
        center.y + dist * sin_e,
        center.z + math.cos(angle_rad) * dist * cos_e,
    ))

    # Look at center
    direction = center - cam_obj.location
    rot_quat = direction.to_track_quat('-Z', 'Y')
    cam_obj.rotation_euler = rot_quat.to_euler()

    # Set orthographic scale
    cam_data = cam_obj.data
    cam_data.ortho_scale = max(half_w, half_h) * 2
    cam_data.clip_start = 0.01
    cam_data.clip_end = max_dim * 10

    return half_w, half_h


def position_topdown_camera(cam_obj, center, size):
    """Position camera for a top-down billboard render.

    Returns half_extent for quad sizing.
    Matches app.js renderBillboardTopDown() (L1759).
    """
    half_w = size.x * PADDING_FACTOR
    half_d = size.z * PADDING_FACTOR
    half = max(half_w, half_d)

    cam_obj.location = Vector((center.x, center.y + size.y * 2, center.z))
    cam_obj.rotation_euler = (0, 0, 0)  # Will be overridden by track
    # Look straight down
    cam_obj.rotation_euler = (0, 0, 0)
    direction = Vector((center.x, center.y, center.z)) - cam_obj.location
    rot_quat = direction.to_track_quat('-Z', 'Y')
    cam_obj.rotation_euler = rot_quat.to_euler()

    cam_data = cam_obj.data
    cam_data.ortho_scale = half * 2
    cam_data.clip_start = 0.01
    cam_data.clip_end = size.y * 10

    return half


def position_slice_camera(cam_obj, center, size, floor_y, ceiling_y):
    """Position camera for a volumetric slice render (top-down with clipping).

    Returns half_extent for quad sizing.
    Matches app.js renderLayerTopDown() (L1956).
    """
    half_extent = max(size.x, size.z) * PADDING_FACTOR

    cam_height = ceiling_y + size.y * 2
    cam_obj.location = Vector((center.x, cam_height, center.z))
    # Look down at the floor
    direction = Vector((center.x, floor_y, center.z)) - cam_obj.location
    rot_quat = direction.to_track_quat('-Z', 'Y')
    cam_obj.rotation_euler = rot_quat.to_euler()

    cam_data = cam_obj.data
    cam_data.ortho_scale = half_extent * 2
    cam_data.clip_start = 0.01
    cam_data.clip_end = cam_height - floor_y + 0.01

    return half_extent


# ---------------------------------------------------------------------------
# Rendering
# ---------------------------------------------------------------------------

def render_to_image(cam_obj, resolution, image_name):
    """Render the current scene from the given camera and return the image.

    Renders to a temp PNG file and loads it back — more reliable across
    Blender versions than reading Render Result pixels directly (which
    broke in Blender 5.x where the pixels sequence can be empty).

    Returns a bpy.types.Image with the rendered result.
    """
    import tempfile

    scene = bpy.context.scene
    scene.camera = cam_obj
    scene.render.resolution_x = resolution
    scene.render.resolution_y = resolution

    # Render to a temp file
    tmp = tempfile.NamedTemporaryFile(suffix=".png", delete=False)
    tmp_path = tmp.name
    tmp.close()

    scene.render.filepath = tmp_path
    bpy.ops.render.render(write_still=True)

    # Load the rendered image back as a datablock
    img = bpy.data.images.load(tmp_path)
    img.name = image_name
    img.pack()

    # Clean up the temp file
    try:
        os.unlink(tmp_path)
    except OSError:
        pass

    return img


# ---------------------------------------------------------------------------
# Quad Construction
# ---------------------------------------------------------------------------

def create_material_for_image(name, image):
    """Create a Principled BSDF material with the image as base color.

    Exports as pbrMetallicRoughness with baseColorTexture in GLB.
    """
    mat = bpy.data.materials.new(name=name)
    mat.use_nodes = True
    mat.blend_method = 'CLIP'
    nodes = mat.node_tree.nodes
    links = mat.node_tree.links
    nodes.clear()

    output = nodes.new(type='ShaderNodeOutputMaterial')
    bsdf = nodes.new(type='ShaderNodeBsdfPrincipled')
    tex = nodes.new(type='ShaderNodeTexImage')
    tex.image = image

    links.new(tex.outputs['Color'], bsdf.inputs['Base Color'])
    links.new(tex.outputs['Alpha'], bsdf.inputs['Alpha'])
    links.new(bsdf.outputs['BSDF'], output.inputs['Surface'])

    # Make it as close to unlit/MeshBasicMaterial as possible
    bsdf.inputs['Metallic'].default_value = 0.0
    bsdf.inputs['Roughness'].default_value = 1.0
    if 'Specular IOR Level' in bsdf.inputs:
        bsdf.inputs['Specular IOR Level'].default_value = 0.0
    elif 'Specular' in bsdf.inputs:
        bsdf.inputs['Specular'].default_value = 0.0

    mat.use_backface_culling = False  # DoubleSide equivalent

    return mat


def create_billboard_quad(name, width, height, image, bottom_pivot=True):
    """Create a billboard quad mesh with the rendered image as texture.

    If bottom_pivot=True, the quad's origin is at its bottom edge (y=0),
    matching Three.js PlaneGeometry + translate(0, height/2, 0).
    """
    bpy.ops.mesh.primitive_plane_add(size=1)
    obj = bpy.context.active_object
    obj.name = name

    # Scale to match target dimensions
    obj.scale = (width, height, 1)
    bpy.ops.object.transform_apply(scale=True)

    if bottom_pivot:
        # Shift geometry up so bottom edge is at y=0
        mesh = obj.data
        for v in mesh.vertices:
            v.co.y += 0.5  # Plane is -0.5 to 0.5; shift to 0 to 1
        # The plane is created in XY by default — vertices are in XY plane
        # After scaling, half_height offset = height/2 is already handled by
        # the 0→1 shift on the unit plane

    # Apply material
    mat = create_material_for_image(f"mat_{name}", image)
    obj.data.materials.clear()
    obj.data.materials.append(mat)

    return obj


def create_topdown_quad(name, size, image):
    """Create a top-down quad lying flat on the XZ plane.

    Matches Three.js: PlaneGeometry + rotateX(-PI/2).
    """
    bpy.ops.mesh.primitive_plane_add(size=1)
    obj = bpy.context.active_object
    obj.name = name

    obj.scale = (size, size, 1)
    bpy.ops.object.transform_apply(scale=True)

    # Rotate to lie flat on XZ (Blender's plane is in XY by default)
    obj.rotation_euler = (math.radians(-90), 0, 0)
    bpy.ops.object.transform_apply(rotation=True)

    mat = create_material_for_image(f"mat_{name}", image)
    obj.data.materials.clear()
    obj.data.materials.append(mat)

    return obj


def create_dome_quad(name, size, dome_height, image, floor_y, segments=DOME_SEGMENTS):
    """Create a dome-geometry quad with parabolic Y deformation.

    Matches app.js createDomeGeometry() (L2037):
        PlaneGeometry(size, size, segments, segments)
        rotateX(-PI/2)
        For each vertex: y = (1 - dist^2) * domeHeight
    """
    # Create subdivided plane
    bpy.ops.mesh.primitive_plane_add(size=1)
    obj = bpy.context.active_object
    obj.name = name

    # Subdivide to get segments x segments grid
    bpy.context.view_layer.objects.active = obj
    bpy.ops.object.mode_set(mode='EDIT')
    bm = bmesh.from_edit_mesh(obj.data)
    bmesh.ops.subdivide_edges(bm, edges=bm.edges[:], cuts=segments - 1, use_grid_fill=True)
    bmesh.update_edit_mesh(obj.data)
    bpy.ops.object.mode_set(mode='OBJECT')

    # Scale to target size
    obj.scale = (size, size, 1)
    bpy.ops.object.transform_apply(scale=True)

    # Rotate to XZ plane (Blender plane is in XY)
    obj.rotation_euler = (math.radians(-90), 0, 0)
    bpy.ops.object.transform_apply(rotation=True)

    # Apply parabolic dome deformation
    half = size / 2.0
    mesh = obj.data
    for v in mesh.vertices:
        dist = min(1.0, math.sqrt(v.co.x ** 2 + v.co.z ** 2) / half) if half > 0 else 0
        v.co.y = (1.0 - dist * dist) * dome_height

    mesh.update()
    # calc_normals() removed in Blender 5.x — normals are auto-computed
    if hasattr(mesh, 'calc_normals'):
        mesh.calc_normals()

    # Position at slice floor height
    obj.location.y = floor_y

    # Apply material
    mat = create_material_for_image(f"mat_{name}", image)
    obj.data.materials.clear()
    obj.data.materials.append(mat)

    return obj


# ---------------------------------------------------------------------------
# Slice Boundary Computation
# ---------------------------------------------------------------------------

def compute_boundaries_equal_height(min_y, max_y, num_layers):
    """Linear interpolation of bounding box. Matches app.js L2052."""
    boundaries = []
    for i in range(num_layers + 1):
        boundaries.append(min_y + (i / num_layers) * (max_y - min_y))
    return boundaries


def compute_boundaries_visual_density(objects, min_y, max_y, num_layers):
    """Trunk-filtered, radially-weighted quantile. Matches app.js L2073."""
    # Collect all vertex Y positions with world transform
    vertices = []
    max_radius = 0.0
    center_xz = Vector((0, 0))

    # First pass: find center and max radius
    all_positions = []
    for obj in objects:
        if obj.type != 'MESH':
            continue
        mesh = obj.data
        for v in mesh.vertices:
            world_co = obj.matrix_world @ v.co
            all_positions.append(world_co)
            r = math.sqrt(world_co.x ** 2 + world_co.z ** 2)
            max_radius = max(max_radius, r)

    if max_radius == 0:
        return compute_boundaries_equal_height(min_y, max_y, num_layers)

    # Filter: discard vertices below trunk height
    trunk_y = min_y + 0.10 * (max_y - min_y)

    weighted_ys = []
    for co in all_positions:
        if co.y < trunk_y:
            continue
        r = math.sqrt(co.x ** 2 + co.z ** 2)
        weight = max(0.05, min(1.0, r / max_radius))
        weighted_ys.append((co.y, weight))

    if not weighted_ys:
        return compute_boundaries_equal_height(min_y, max_y, num_layers)

    # Sort by Y
    weighted_ys.sort(key=lambda x: x[0])

    # Compute weighted cumulative distribution
    total_weight = sum(w for _, w in weighted_ys)
    cumulative = 0.0
    boundaries = [min_y]

    target_idx = 1
    for y, w in weighted_ys:
        cumulative += w
        frac = cumulative / total_weight
        while target_idx < num_layers and frac >= target_idx / num_layers:
            boundaries.append(y)
            target_idx += 1

    # Fill any remaining boundaries
    while len(boundaries) <= num_layers:
        boundaries.append(max_y)

    boundaries[0] = min_y
    boundaries[num_layers] = max_y
    return boundaries


def compute_boundaries_vertex_quantile(objects, min_y, max_y, num_layers):
    """Unweighted vertex Y quantile (legacy fallback). Matches app.js L2151."""
    ys = []
    for obj in objects:
        if obj.type != 'MESH':
            continue
        mesh = obj.data
        for v in mesh.vertices:
            world_co = obj.matrix_world @ v.co
            ys.append(world_co.y)

    if not ys:
        return compute_boundaries_equal_height(min_y, max_y, num_layers)

    ys.sort()
    count = len(ys)
    boundaries = []
    for i in range(num_layers + 1):
        idx = min(int(i / num_layers * count), count - 1)
        boundaries.append(ys[idx])

    boundaries[0] = min_y
    boundaries[num_layers] = max_y
    return boundaries


def pick_adaptive_layer_count(size, base_layers):
    """Adjust layer count by aspect ratio. Matches app.js L2189."""
    height_to_width = size.y / max(size.x, size.z) if max(size.x, size.z) > 0 else 1
    if height_to_width > 2.5:
        return base_layers + 2
    if height_to_width > 1.5:
        return base_layers + 1
    return base_layers


# ---------------------------------------------------------------------------
# Axis Rotation
# ---------------------------------------------------------------------------

def resolve_slice_axis_rotation(objects, mode):
    """Resolve slice axis rotation. Matches app.js resolveSliceAxisRotation() (L2211).

    Returns (rotation_quat, inverse_quat). Identity for mode='y'.
    """
    if mode == 'y':
        return Quaternion(), Quaternion()

    _, size, _, _ = get_model_bounds(objects)
    y_axis = Vector((0, 1, 0))

    if mode == 'auto-horizontal':
        # Pick the longer of X/Z
        if size.x >= size.z:
            picked = Vector((1, 0, 0))
        else:
            picked = Vector((0, 0, 1))
    elif mode == 'auto-thin':
        # Pick the shortest of X/Y/Z
        dims = [(size.x, Vector((1, 0, 0))),
                (size.y, Vector((0, 1, 0))),
                (size.z, Vector((0, 0, 1)))]
        dims.sort(key=lambda x: x[0])
        picked = dims[0][1]
    else:
        return Quaternion(), Quaternion()

    rotation = picked.rotation_difference(y_axis)
    inverse = rotation.conjugated()
    return rotation, inverse


# ---------------------------------------------------------------------------
# Variant Renderers
# ---------------------------------------------------------------------------

def render_side_billboard(model_objects, params, cam_obj):
    """Render side billboard angles + top-down. Returns list of quad objects."""
    center, size, bbox_min, bbox_max = get_model_bounds(model_objects)
    num_angles = params.get("billboard_angles", BILLBOARD_ANGLES)
    resolution = params.get("resolution", DEFAULT_RESOLUTION)
    quads = []

    for i in range(num_angles):
        angle_rad = (i / num_angles) * 2 * math.pi
        half_w, half_h = position_side_camera(cam_obj, center, size, angle_rad)

        progress(f"  Rendering side billboard {i}/{num_angles} (angle={math.degrees(angle_rad):.0f}deg)")
        img = render_to_image(cam_obj, resolution, f"billboard_{i}")

        quad_w = half_w * 2
        quad_h = half_h * 2
        quad = create_billboard_quad(f"billboard_{i}", quad_w, quad_h, img, bottom_pivot=True)
        quads.append(quad)

    # Top-down billboard
    progress("  Rendering top-down billboard")
    half_extent = position_topdown_camera(cam_obj, center, size)
    img = render_to_image(cam_obj, resolution, "billboard_top")
    quad_size = half_extent * 2
    top_quad = create_topdown_quad("billboard_top", quad_size, img)
    quads.append(top_quad)

    return quads


def render_tilted_billboard(model_objects, params, cam_obj):
    """Render tilted billboard angles. Returns list of quad objects."""
    center, size, bbox_min, bbox_max = get_model_bounds(model_objects)
    num_angles = params.get("billboard_angles", BILLBOARD_ANGLES)
    resolution = params.get("resolution", DEFAULT_RESOLUTION)
    elevation_rad = params.get("tilted_elevation_rad", TILTED_ELEVATION_RAD)
    quads = []

    for i in range(num_angles):
        angle_rad = (i / num_angles) * 2 * math.pi
        half_w, half_h = position_side_camera(cam_obj, center, size, angle_rad, elevation_rad)

        progress(f"  Rendering tilted billboard {i}/{num_angles} (angle={math.degrees(angle_rad):.0f}deg)")
        img = render_to_image(cam_obj, resolution, f"tilted_{i}")

        quad_w = half_w * 2
        quad_h = half_h * 2
        quad = create_billboard_quad(f"billboard_{i}", quad_w, quad_h, img, bottom_pivot=True)
        quads.append(quad)

    return quads


def render_volumetric(model_objects, params, cam_obj):
    """Render volumetric dome slices. Returns list of quad objects."""
    center, size, bbox_min, bbox_max = get_model_bounds(model_objects)
    resolution = params.get("volumetric_resolution", DEFAULT_RESOLUTION)
    base_layers = params.get("volumetric_layers", 4)
    dist_mode = params.get("slice_distribution_mode", "visual-density")
    slice_axis = params.get("slice_axis", "y")
    dome_height_factor = params.get("dome_height_factor", 0.5)
    ground_align = params.get("ground_align", True)

    # Handle hard-surface (no volumetric)
    if base_layers == 0:
        progress("  Skipping volumetric — hard-surface category (0 layers)")
        return []

    # Axis rotation
    rotation, inverse = resolve_slice_axis_rotation(model_objects, slice_axis)
    has_rotation = rotation.angle > 0.001

    if has_rotation:
        # Apply rotation to model objects
        for obj in model_objects:
            obj.rotation_mode = 'QUATERNION'
            obj.rotation_quaternion = rotation @ obj.rotation_quaternion
            bpy.context.view_layer.update()
        # Recompute bounds after rotation
        center, size, bbox_min, bbox_max = get_model_bounds(model_objects)

    # Adaptive layer count
    num_layers = pick_adaptive_layer_count(size, base_layers)
    progress(f"  Volumetric: {num_layers} layers (base={base_layers}), mode={dist_mode}")

    # Compute boundaries
    min_y = bbox_min.y
    max_y = bbox_max.y

    if dist_mode == "equal-height":
        boundaries = compute_boundaries_equal_height(min_y, max_y, num_layers)
    elif dist_mode == "visual-density":
        boundaries = compute_boundaries_visual_density(model_objects, min_y, max_y, num_layers)
    elif dist_mode == "vertex-quantile":
        boundaries = compute_boundaries_vertex_quantile(model_objects, min_y, max_y, num_layers)
    else:
        progress(f"  WARNING: unknown slice_distribution_mode '{dist_mode}', falling back to equal-height")
        boundaries = compute_boundaries_equal_height(min_y, max_y, num_layers)

    quads = []
    for i in range(num_layers):
        floor_y = boundaries[i]
        ceiling_y = boundaries[i + 1]
        layer_thickness = max(ceiling_y - floor_y, 0.001)
        dome_height = layer_thickness * dome_height_factor

        half_extent = position_slice_camera(cam_obj, center, size, floor_y, ceiling_y)

        base_mm = round(floor_y * 1000)
        mesh_name = f"vol_layer_{i}_h{base_mm}"

        progress(f"  Rendering volumetric slice {i}/{num_layers} (floor={floor_y:.3f}, ceil={ceiling_y:.3f})")
        img = render_to_image(cam_obj, resolution, mesh_name)

        quad_size = half_extent * 2
        quad = create_dome_quad(mesh_name, quad_size, dome_height, img, floor_y, DOME_SEGMENTS)
        quads.append(quad)

    # Ground alignment
    if ground_align and boundaries:
        ground_offset = -boundaries[0]
        for q in quads:
            q.location.y += ground_offset

    # Undo axis rotation on model objects
    if has_rotation:
        for obj in model_objects:
            obj.rotation_quaternion = inverse @ obj.rotation_quaternion
            bpy.context.view_layer.update()

    return quads, (inverse if has_rotation else None)


# ---------------------------------------------------------------------------
# Export
# ---------------------------------------------------------------------------

def export_quads_as_glb(quads, output_path, inverse_rotation=None):
    """Export quad objects as a GLB file.

    Creates a fresh scene with only the quad objects, optionally applies
    inverse rotation wrapper, and exports.
    """
    # Deselect everything
    bpy.ops.object.select_all(action='DESELECT')

    # Select only the quads
    for q in quads:
        q.select_set(True)

    # If inverse rotation needed, create a parent empty and rotate it
    if inverse_rotation is not None and inverse_rotation.angle > 0.001:
        bpy.ops.object.empty_add(type='PLAIN_AXES')
        root = bpy.context.active_object
        root.name = "vol_root"
        root.rotation_mode = 'QUATERNION'
        root.rotation_quaternion = inverse_rotation
        for q in quads:
            q.parent = root
        root.select_set(True)

    bpy.ops.export_scene.gltf(
        filepath=os.path.abspath(output_path),
        export_format='GLB',
        use_selection=True,
        export_apply=True,
        export_materials='EXPORT',
        export_image_format='AUTO',
        export_texcoords=True,
        export_normals=True,
    )

    output_size = os.path.getsize(output_path)
    progress(f"  Exported: {output_path} ({output_size:,} bytes)")

    # Cleanup: remove quads and root from the scene
    bpy.ops.object.select_all(action='DESELECT')
    for q in quads:
        q.select_set(True)
    if inverse_rotation is not None and inverse_rotation.angle > 0.001:
        root = bpy.data.objects.get("vol_root")
        if root:
            root.select_set(True)
    bpy.ops.object.delete(use_global=False)

    # Clean up orphaned materials and images from this export
    for mat in bpy.data.materials:
        if mat.name.startswith("mat_billboard_") or mat.name.startswith("mat_vol_"):
            if mat.users == 0:
                bpy.data.materials.remove(mat)


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    start_time = time.time()

    args = parse_args()
    params = merge_config(args)

    source = params["source"]
    output_dir = params["output_dir"]
    asset_id = params["id"]
    category = params.get("category", "unknown")

    progress(f"Starting production render for '{asset_id}'")
    progress(f"  Source: {source}")
    progress(f"  Output dir: {output_dir}")
    progress(f"  Category: {category}")
    progress(f"  Resolution: {params.get('resolution', 512)}")

    os.makedirs(output_dir, exist_ok=True)

    # Clear and import
    clear_scene()
    model_objects = import_glb(source)

    center, size, bbox_min, bbox_max = get_model_bounds(model_objects)
    progress(f"  Model bounds: size=({size.x:.3f}, {size.y:.3f}, {size.z:.3f})")
    progress(f"  Model center: ({center.x:.3f}, {center.y:.3f}, {center.z:.3f})")

    # Setup renderer and lighting
    configure_renderer(params.get("resolution", DEFAULT_RESOLUTION), params)
    setup_bake_lighting(params)

    # Create shared camera
    cam = create_ortho_camera("ProductionCam")

    produced = []

    # --- Side billboard ---
    if not args.skip_billboard:
        progress("Rendering side billboards...")
        quads = render_side_billboard(model_objects, params, cam)
        out_path = os.path.join(output_dir, f"{asset_id}_billboard.glb")
        export_quads_as_glb(quads, out_path)
        produced.append(("billboard", out_path))
    else:
        progress("Skipping side billboard (--skip-billboard)")

    # --- Tilted billboard ---
    if not args.skip_tilted:
        progress("Rendering tilted billboards...")
        quads = render_tilted_billboard(model_objects, params, cam)
        out_path = os.path.join(output_dir, f"{asset_id}_billboard_tilted.glb")
        export_quads_as_glb(quads, out_path)
        produced.append(("tilted", out_path))
    else:
        progress("Skipping tilted billboard (--skip-tilted)")

    # --- Volumetric ---
    if not args.skip_volumetric:
        vol_layers = params.get("volumetric_layers", 4)
        if vol_layers > 0:
            progress("Rendering volumetric dome slices...")
            result = render_volumetric(model_objects, params, cam)
            if isinstance(result, tuple):
                quads, inv_rot = result
            else:
                quads, inv_rot = result, None
            if quads:
                out_path = os.path.join(output_dir, f"{asset_id}_volumetric.glb")
                export_quads_as_glb(quads, out_path, inverse_rotation=inv_rot)
                produced.append(("volumetric", out_path))
        else:
            progress("Skipping volumetric — 0 layers configured (hard-surface)")
    else:
        progress("Skipping volumetric (--skip-volumetric)")

    elapsed = time.time() - start_time
    progress(f"Done. Produced {len(produced)} GLB file(s) in {elapsed:.1f}s:")
    for name, path in produced:
        progress(f"  {name}: {path}")


if __name__ == "__main__":
    main()
