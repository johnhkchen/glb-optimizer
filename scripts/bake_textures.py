#!/usr/bin/env python3
"""
Texture baking script for parametric geometry (Blender headless).

Projects textures from a source TRELLIS model onto a parametric reconstruction
via Blender's Cycles bake system, producing a single texture atlas.

Usage:
    blender -b --python scripts/bake_textures.py -- \
        --source assets/wood_raised_bed.glb \
        --target assets/wood_raised_bed_parametric.glb \
        --output assets/wood_raised_bed_textured.glb \
        --atlas-size 512 \
        --mode bake

Modes:
    bake  — Ray-cast diffuse bake from source mesh onto target (default)
    tile  — Sample tileable regions from source texture with per-board variation
"""

import sys
import os
import argparse
import math
import random

# Blender Python API — only available when run inside Blender
try:
    import bpy
    import bmesh
    from mathutils import Vector
    HAS_BPY = True
except ImportError:
    HAS_BPY = False
    print("ERROR: This script must be run inside Blender.", file=sys.stderr)
    print("  blender -b --python scripts/bake_textures.py -- [args]", file=sys.stderr)
    sys.exit(1)


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

    parser = argparse.ArgumentParser(description="Bake textures onto parametric geometry")
    parser.add_argument("--source", required=True, help="Source GLB (original TRELLIS model)")
    parser.add_argument("--target", required=True, help="Target GLB (parametric reconstruction)")
    parser.add_argument("--output", required=True, help="Output GLB path")
    parser.add_argument("--atlas-size", type=int, default=512, choices=[256, 512, 1024],
                        help="Atlas texture resolution (default: 512)")
    parser.add_argument("--mode", default="bake", choices=["bake", "tile"],
                        help="Bake mode: 'bake' (ray-cast) or 'tile' (tileable fallback)")
    parser.add_argument("--seed", type=int, default=42, help="Random seed for UV jitter")
    parser.add_argument("--quality", type=int, default=75, help="JPEG quality (1-100)")
    parser.add_argument("--margin", type=int, default=4, help="Bake margin in pixels")
    return parser.parse_args(argv)


# ---------------------------------------------------------------------------
# Scene Setup
# ---------------------------------------------------------------------------

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


def import_glb(filepath, name_prefix):
    """Import a GLB file and return the imported mesh objects."""
    before = set(bpy.data.objects.keys())
    bpy.ops.import_scene.gltf(filepath=filepath)
    after = set(bpy.data.objects.keys())
    new_names = after - before

    objects = []
    for name in new_names:
        obj = bpy.data.objects[name]
        obj.name = f"{name_prefix}_{name}"
        if obj.type == 'MESH':
            objects.append(obj)

    return objects


def join_objects(objects, name):
    """Join multiple mesh objects into one."""
    if not objects:
        return None
    if len(objects) == 1:
        objects[0].name = name
        return objects[0]

    bpy.ops.object.select_all(action='DESELECT')
    for obj in objects:
        obj.select_set(True)
    bpy.context.view_layer.objects.active = objects[0]
    bpy.ops.object.join()
    result = bpy.context.active_object
    result.name = name
    return result


def setup_scene(source_path, target_path):
    """Set up the Blender scene with source and target meshes."""
    clear_scene()

    # Set render engine to Cycles (required for baking)
    bpy.context.scene.render.engine = 'CYCLES'
    bpy.context.scene.cycles.device = 'CPU'
    bpy.context.scene.cycles.samples = 1  # Minimal for diffuse bake

    # Import meshes
    print(f"Importing source: {source_path}")
    source_objs = import_glb(os.path.abspath(source_path), "source")
    print(f"  Imported {len(source_objs)} source mesh objects")

    print(f"Importing target: {target_path}")
    target_objs = import_glb(os.path.abspath(target_path), "target")
    print(f"  Imported {len(target_objs)} target mesh objects")

    # Join into single objects
    source = join_objects(source_objs, "source")
    target = join_objects(target_objs, "target")

    if not source or not target:
        print("ERROR: Failed to import source or target mesh", file=sys.stderr)
        sys.exit(1)

    print(f"Source: {len(source.data.vertices)} verts, {len(source.data.polygons)} faces")
    print(f"Target: {len(target.data.vertices)} verts, {len(target.data.polygons)} faces")

    return source, target


# ---------------------------------------------------------------------------
# Atlas UV Layout
# ---------------------------------------------------------------------------

def layout_atlas_uvs(target, atlas_size, seed):
    """Assign each board a unique UV island in a 4x4 atlas grid.

    The parametric mesh has 24 vertices per board (4 per face x 6 faces),
    so board boundaries are at every 24 vertices. Each board gets a cell
    in a 4x4 grid.
    """
    mesh = target.data
    verts_per_board = 24
    tris_per_board = 12
    total_verts = len(mesh.vertices)
    board_count = total_verts // verts_per_board

    if board_count == 0:
        print("ERROR: Target mesh has no boards (0 vertices)", file=sys.stderr)
        sys.exit(1)

    print(f"Atlas layout: {board_count} boards in {atlas_size}x{atlas_size} atlas")

    # Grid dimensions: ceil(sqrt(board_count)) x ceil(sqrt(board_count))
    grid_cols = math.ceil(math.sqrt(board_count))
    grid_rows = math.ceil(board_count / grid_cols)
    cell_u = 1.0 / grid_cols
    cell_v = 1.0 / grid_rows

    rng = random.Random(seed)

    # Ensure we have a UV layer
    if not mesh.uv_layers:
        mesh.uv_layers.new(name="atlas_uv")
    uv_layer = mesh.uv_layers.active

    # Each face loop gets UV coordinates
    # Faces are ordered: board 0 faces (12 tris = 6 quads with 2 tris each), board 1 faces, ...
    # Actually in the GLB, faces are stored as triangles. The parametric mesh has
    # 12 triangles per board (2 per face x 6 faces).
    #
    # Face ordering per board (from generate_box in parametric_reconstruct.py):
    # Face 0-1: +X face (2 triangles)
    # Face 2-3: -X face
    # Face 4-5: +Y face (top)
    # Face 6-7: -Y face (bottom)
    # Face 8-9: +Z face
    # Face 10-11: -Z face
    #
    # For atlas packing, we map each board's 6 quads into its grid cell.
    # Layout within cell:
    #   - Main visible faces (front/back = ±Z or ±X depending on orientation) get most area
    #   - Since all faces need to be UV-mapped for the bake, we use a simple
    #     sub-grid: 3 columns x 2 rows within each cell
    #     Row 0: [+X face] [+Y face] [-X face]
    #     Row 1: [+Z face] [-Y face] [-Z face]

    sub_cols = 3
    sub_rows = 2
    sub_u = cell_u / sub_cols
    sub_v = cell_v / sub_rows

    # Mapping from face-pair index (0-5) to sub-grid position
    face_to_subcell = [
        (0, 0),  # +X → col 0, row 0
        (2, 0),  # -X → col 2, row 0
        (1, 0),  # +Y → col 1, row 0
        (1, 1),  # -Y → col 1, row 1
        (0, 1),  # +Z → col 0, row 1
        (2, 1),  # -Z → col 2, row 1
    ]

    padding = 0.002  # Small padding between sub-cells to prevent bleed

    for board_idx in range(board_count):
        # Grid cell for this board
        col = board_idx % grid_cols
        row = board_idx // grid_cols
        cell_origin_u = col * cell_u
        cell_origin_v = row * cell_v

        # Per-board UV jitter
        jitter_u = rng.uniform(-0.003, 0.003)
        jitter_v = rng.uniform(-0.003, 0.003)

        # Process each face-pair (2 triangles = 1 quad)
        for face_pair_idx in range(6):
            sub_col, sub_row = face_to_subcell[face_pair_idx]

            # Sub-cell bounds in UV space
            su_min = cell_origin_u + sub_col * sub_u + padding + jitter_u
            sv_min = cell_origin_v + sub_row * sub_v + padding + jitter_v
            su_max = cell_origin_u + (sub_col + 1) * sub_u - padding + jitter_u
            sv_max = cell_origin_v + (sub_row + 1) * sub_v - padding + jitter_v

            # Each face-pair = 2 triangles = 6 loops
            # Triangle 1: verts 0,1,2 of quad → UVs: (min,min), (max,min), (max,max)
            # Triangle 2: verts 0,2,3 of quad → UVs: (min,min), (max,max), (min,max)
            tri_base = (board_idx * tris_per_board + face_pair_idx * 2)

            if tri_base + 1 >= len(mesh.polygons):
                break

            poly0 = mesh.polygons[tri_base]
            poly1 = mesh.polygons[tri_base + 1]

            # Triangle 1: loops[0]=BL, loops[1]=BR, loops[2]=TR
            loops0 = list(poly0.loop_indices)
            if len(loops0) >= 3:
                uv_layer.data[loops0[0]].uv = (su_min, sv_min)
                uv_layer.data[loops0[1]].uv = (su_max, sv_min)
                uv_layer.data[loops0[2]].uv = (su_max, sv_max)

            # Triangle 2: loops[0]=BL, loops[1]=TR, loops[2]=TL
            loops1 = list(poly1.loop_indices)
            if len(loops1) >= 3:
                uv_layer.data[loops1[0]].uv = (su_min, sv_min)
                uv_layer.data[loops1[1]].uv = (su_max, sv_max)
                uv_layer.data[loops1[2]].uv = (su_min, sv_max)

    print(f"  UV layout complete: {grid_cols}x{grid_rows} grid, {board_count} islands")
    return board_count


# ---------------------------------------------------------------------------
# Bake Mode: Ray-cast Diffuse
# ---------------------------------------------------------------------------

def get_source_texture(source):
    """Extract the source texture image from the source object's material."""
    if not source.data.materials:
        return None

    mat = source.data.materials[0]
    if not mat.use_nodes:
        return None

    for node in mat.node_tree.nodes:
        if node.type == 'TEX_IMAGE' and node.image:
            return node.image

    return None


def setup_source_emission(source):
    """Rewire source material to emit its texture color (needed for EMIT bake)."""
    if not source.data.materials:
        return

    mat = source.data.materials[0]
    if not mat.node_tree:
        return

    nodes = mat.node_tree.nodes
    links = mat.node_tree.links

    # Find the texture image node
    tex_node = None
    for node in nodes:
        if node.type == 'TEX_IMAGE' and node.image:
            tex_node = node
            break

    if not tex_node:
        print("  WARNING: No texture found on source material", file=sys.stderr)
        return

    # Create emission shader and connect texture to it
    emit_node = nodes.new('ShaderNodeEmission')
    emit_node.inputs['Strength'].default_value = 1.0
    links.new(tex_node.outputs['Color'], emit_node.inputs['Color'])

    # Find output node and connect emission
    out_node = None
    for node in nodes:
        if node.type == 'OUTPUT_MATERIAL':
            out_node = node
            break

    if out_node:
        links.new(emit_node.outputs['Emission'], out_node.inputs['Surface'])

    print("  Source material rewired to emission for baking")


def bake_diffuse(source, target, atlas_size, margin, output_path):
    """Bake texture from source mesh onto target mesh via EMIT ray-cast."""
    print(f"Baking texture: {atlas_size}x{atlas_size}, margin={margin}px")

    # Rewire source material to emission (Cycles EMIT bake captures this)
    setup_source_emission(source)

    # Create atlas image
    atlas_name = "baked_atlas"
    atlas = bpy.data.images.new(atlas_name, atlas_size, atlas_size, alpha=False)
    atlas.colorspace_settings.name = 'sRGB'

    # Set up target material with image texture node for bake target
    if target.data.materials:
        target_mat = target.data.materials[0]
    else:
        target_mat = bpy.data.materials.new(name="baked_wood")
        target.data.materials.append(target_mat)

    target_mat.use_nodes = True
    nodes = target_mat.node_tree.nodes
    links = target_mat.node_tree.links

    # Clear existing nodes
    for node in nodes:
        nodes.remove(node)

    # Create shader nodes — atlas not connected to BSDF during bake
    output_node = nodes.new('ShaderNodeOutputMaterial')
    bsdf_node = nodes.new('ShaderNodeBsdfPrincipled')
    tex_node = nodes.new('ShaderNodeTexImage')

    tex_node.image = atlas
    tex_node.name = "bake_target"

    links.new(bsdf_node.outputs['BSDF'], output_node.inputs['Surface'])

    bsdf_node.inputs['Metallic'].default_value = 0.0
    bsdf_node.inputs['Roughness'].default_value = 0.9

    # Make the texture node active (Blender bakes to the active image texture node)
    nodes.active = tex_node

    # Configure bake settings
    bpy.context.scene.render.bake.use_selected_to_active = True
    bpy.context.scene.render.bake.cage_extrusion = 1.0
    bpy.context.scene.render.bake.max_ray_distance = 0.0  # unlimited
    bpy.context.scene.render.bake.margin = margin
    bpy.context.scene.render.bake.margin_type = 'EXTEND'

    # Select source, set target as active
    bpy.ops.object.select_all(action='DESELECT')
    source.select_set(True)
    target.select_set(True)
    bpy.context.view_layer.objects.active = target

    # Execute EMIT bake (captures emission output = source texture color)
    print("  Executing Cycles EMIT bake...")
    try:
        bpy.ops.object.bake(type='EMIT')
        print("  Bake complete")
    except RuntimeError as e:
        print(f"  WARNING: Bake failed: {e}", file=sys.stderr)
        print("  Falling back to solid color fill", file=sys.stderr)
        fill_solid_color(atlas, (0.545, 0.353, 0.169, 1.0))

    # Check coverage
    pixels = list(atlas.pixels)
    total_pixels = len(pixels) // 4
    nonzero = sum(1 for i in range(0, len(pixels), 4)
                  if pixels[i] > 0.01 or pixels[i+1] > 0.01 or pixels[i+2] > 0.01)
    coverage = nonzero / total_pixels * 100
    print(f"  Atlas coverage: {coverage:.1f}% ({nonzero}/{total_pixels} pixels)")

    if coverage < 10.0:
        print("  WARNING: Very low coverage — bake may have failed", file=sys.stderr)
        print("  Consider using --mode tile as fallback", file=sys.stderr)

    # Connect baked atlas to BSDF (safe after bake is done)
    links.new(tex_node.outputs['Color'], bsdf_node.inputs['Base Color'])

    return atlas


def fill_solid_color(image, color):
    """Fill an image with a solid color (fallback)."""
    pixels = list(image.pixels)
    for i in range(0, len(pixels), 4):
        pixels[i] = color[0]
        pixels[i + 1] = color[1]
        pixels[i + 2] = color[2]
        pixels[i + 3] = color[3]
    image.pixels = pixels


# ---------------------------------------------------------------------------
# Tile Mode: Tileable Texture Sampling
# ---------------------------------------------------------------------------

def tile_from_source(source, target, atlas_size, seed):
    """Create atlas by sampling tileable regions from the source texture."""
    print(f"Tiling from source texture: {atlas_size}x{atlas_size}")

    source_img = get_source_texture(source)

    # Create atlas image
    atlas_name = "tiled_atlas"
    atlas = bpy.data.images.new(atlas_name, atlas_size, atlas_size, alpha=False)

    if source_img is None:
        print("  WARNING: No source texture found, using solid color", file=sys.stderr)
        fill_solid_color(atlas, (0.545, 0.353, 0.169, 1.0))
        setup_target_material(target, atlas)
        return atlas

    # Get source image pixels
    src_w, src_h = source_img.size
    src_pixels = list(source_img.pixels)  # flat RGBA array
    print(f"  Source texture: {src_w}x{src_h}")

    # Create atlas pixels
    atlas_pixels = [0.0] * (atlas_size * atlas_size * 4)

    rng = random.Random(seed)

    # Determine board count and grid layout
    mesh = target.data
    verts_per_board = 24
    board_count = len(mesh.vertices) // verts_per_board
    grid_cols = math.ceil(math.sqrt(board_count))
    grid_rows = math.ceil(board_count / grid_cols)
    cell_w = atlas_size // grid_cols
    cell_h = atlas_size // grid_rows

    for board_idx in range(board_count):
        col = board_idx % grid_cols
        row = board_idx // grid_cols

        # Random offset into source texture for this board
        offset_x = rng.randint(0, max(0, src_w - cell_w))
        offset_y = rng.randint(0, max(0, src_h - cell_h))

        # Copy pixels from source region to atlas cell
        cell_x0 = col * cell_w
        cell_y0 = row * cell_h

        for cy in range(cell_h):
            for cx in range(cell_w):
                # Source coordinates (with wrapping)
                sx = (offset_x + cx) % src_w
                sy = (offset_y + cy) % src_h
                src_idx = (sy * src_w + sx) * 4

                # Atlas coordinates
                ax = cell_x0 + cx
                ay = cell_y0 + cy
                if ax >= atlas_size or ay >= atlas_size:
                    continue
                atlas_idx = (ay * atlas_size + ax) * 4

                atlas_pixels[atlas_idx] = src_pixels[src_idx]
                atlas_pixels[atlas_idx + 1] = src_pixels[src_idx + 1]
                atlas_pixels[atlas_idx + 2] = src_pixels[src_idx + 2]
                atlas_pixels[atlas_idx + 3] = 1.0

    atlas.pixels = atlas_pixels
    print(f"  Tiled {board_count} board regions into atlas")

    # Set up material on target
    setup_target_material(target, atlas)

    return atlas


def setup_target_material(target, atlas):
    """Set up the target object's material to use the atlas texture."""
    if target.data.materials:
        target_mat = target.data.materials[0]
    else:
        target_mat = bpy.data.materials.new(name="baked_wood")
        target.data.materials.append(target_mat)

    target_mat.use_nodes = True
    nodes = target_mat.node_tree.nodes
    links = target_mat.node_tree.links

    for node in nodes:
        nodes.remove(node)

    output_node = nodes.new('ShaderNodeOutputMaterial')
    bsdf_node = nodes.new('ShaderNodeBsdfPrincipled')
    tex_node = nodes.new('ShaderNodeTexImage')

    tex_node.image = atlas
    tex_node.name = "bake_target"

    links.new(tex_node.outputs['Color'], bsdf_node.inputs['Base Color'])
    links.new(bsdf_node.outputs['BSDF'], output_node.inputs['Surface'])

    bsdf_node.inputs['Metallic'].default_value = 0.0
    bsdf_node.inputs['Roughness'].default_value = 0.9

    nodes.active = tex_node


# ---------------------------------------------------------------------------
# GLB Export
# ---------------------------------------------------------------------------

def export_glb(target, source, output_path, atlas, quality):
    """Export the target mesh with baked atlas as a GLB file."""
    print(f"Exporting to {output_path}")

    # Remove source object from scene
    bpy.ops.object.select_all(action='DESELECT')
    source.select_set(True)
    bpy.ops.object.delete(use_global=False)

    # Select and activate target
    target.select_set(True)
    bpy.context.view_layer.objects.active = target

    # Pack atlas image into the blend file so it embeds in GLB
    atlas.pack()

    # Export as GLB
    bpy.ops.export_scene.gltf(
        filepath=os.path.abspath(output_path),
        export_format='GLB',
        use_selection=True,
        export_apply=True,
        export_image_format='JPEG',
        export_jpeg_quality=quality,
        export_materials='EXPORT',
    )

    output_size = os.path.getsize(output_path)
    print(f"  Output: {output_size:,} bytes")
    return output_size


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    args = parse_args()

    print("=" * 56)
    print("Texture Baking — T-001-02")
    print(f"  Source: {args.source}")
    print(f"  Target: {args.target}")
    print(f"  Output: {args.output}")
    print(f"  Mode:   {args.mode}")
    print(f"  Atlas:  {args.atlas_size}x{args.atlas_size}")
    print(f"  Seed:   {args.seed}")
    print("=" * 56)

    # Set up scene
    source, target = setup_scene(args.source, args.target)

    # Layout atlas UVs on target
    board_count = layout_atlas_uvs(target, args.atlas_size, args.seed)

    # Bake or tile
    if args.mode == "bake":
        atlas = bake_diffuse(source, target, args.atlas_size, args.margin, args.output)
    else:
        atlas = tile_from_source(source, target, args.atlas_size, args.seed)

    # Export
    output_size = export_glb(target, source, args.output, atlas, args.quality)

    # Summary
    print()
    print("=" * 56)
    print(f"Boards:     {board_count}")
    print(f"Triangles:  {board_count * 12}")
    print(f"Atlas:      {args.atlas_size}x{args.atlas_size} ({args.mode})")
    print(f"Output:     {output_size:,} bytes")
    print("=" * 56)


if __name__ == '__main__':
    main()
