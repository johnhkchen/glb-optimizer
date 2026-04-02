"""
Blender headless script for high-quality LOD generation.

Usage:
    blender --background --python remesh_lod.py -- \
        --input /path/to/model.glb \
        --output /path/to/output.glb \
        --mode remesh_decimate \
        --voxel-size 0.05 \
        --decimate-ratio 0.5 \
        --planar-angle 5.0

Modes:
    decimate          - Just decimate (collapse), similar to gltfpack but better topology
    planar            - Planar dissolve only (great for architectural/hard-surface)
    remesh_decimate   - Voxel remesh then decimate (best for organic shapes)
    unsubdivide       - Reverse subdivision levels (for subdivided meshes)

All arguments after '--' are passed to this script.
"""

import bpy
import sys
import argparse
import math


def parse_args():
    # Everything after '--' in the blender command line
    argv = sys.argv[sys.argv.index('--') + 1:] if '--' in sys.argv else []
    parser = argparse.ArgumentParser(description='Blender LOD generator')
    parser.add_argument('--input', required=True, help='Input GLB file path')
    parser.add_argument('--output', required=True, help='Output GLB file path')
    parser.add_argument('--mode', default='remesh_decimate',
                        choices=['decimate', 'planar', 'remesh_decimate', 'unsubdivide'],
                        help='Processing mode')
    parser.add_argument('--voxel-size', type=float, default=0.0,
                        help='Voxel remesh size (0 = auto-calculate from model bounds)')
    parser.add_argument('--decimate-ratio', type=float, default=0.5,
                        help='Target ratio for decimate (0.0 to 1.0)')
    parser.add_argument('--planar-angle', type=float, default=5.0,
                        help='Angle threshold in degrees for planar dissolve')
    parser.add_argument('--target-tris', type=int, default=0,
                        help='Target triangle count (overrides decimate-ratio if set)')
    parser.add_argument('--iterations', type=int, default=1,
                        help='Un-subdivide iterations')
    return parser.parse_args(argv)


def clear_scene():
    """Remove all objects from the scene."""
    bpy.ops.object.select_all(action='SELECT')
    bpy.ops.object.delete()
    # Clear orphan data
    for block in bpy.data.meshes:
        if block.users == 0:
            bpy.data.meshes.remove(block)


def import_glb(filepath):
    """Import a GLB file."""
    bpy.ops.import_scene.gltf(filepath=filepath)


def get_mesh_objects():
    """Return all mesh objects in the scene."""
    return [obj for obj in bpy.context.scene.objects if obj.type == 'MESH']


def count_triangles():
    """Count total triangles across all mesh objects."""
    total = 0
    for obj in get_mesh_objects():
        # Ensure we're counting triangulated faces
        depsgraph = bpy.context.evaluated_depsgraph_get()
        obj_eval = obj.evaluated_get(depsgraph)
        mesh = obj_eval.to_mesh()
        mesh.calc_loop_triangles()
        total += len(mesh.loop_triangles)
        obj_eval.to_mesh_clear()
    return total


def get_model_bounds():
    """Get the bounding box dimensions of all mesh objects combined."""
    from mathutils import Vector
    min_co = Vector((float('inf'),) * 3)
    max_co = Vector((float('-inf'),) * 3)
    for obj in get_mesh_objects():
        for corner in obj.bound_box:
            world_co = obj.matrix_world @ Vector(corner)
            for i in range(3):
                min_co[i] = min(min_co[i], world_co[i])
                max_co[i] = max(max_co[i], world_co[i])
    dims = max_co - min_co
    return max(dims) if dims[0] != float('inf') else 1.0


def auto_voxel_size(target_ratio):
    """Calculate a voxel size based on model bounds and target detail level."""
    max_dim = get_model_bounds()
    # Higher ratio = more detail = smaller voxels
    # At ratio 0.5, voxel size is about 2% of max dimension
    # At ratio 0.1, voxel size is about 5% of max dimension
    base_size = max_dim * 0.02
    scale = 1.0 / max(target_ratio, 0.01)
    return base_size * math.sqrt(scale) * 0.3


def join_meshes():
    """Join all mesh objects into one for better remeshing."""
    meshes = get_mesh_objects()
    if len(meshes) <= 1:
        return meshes[0] if meshes else None

    # Select all meshes
    bpy.ops.object.select_all(action='DESELECT')
    for obj in meshes:
        obj.select_set(True)
    bpy.context.view_layer.objects.active = meshes[0]
    bpy.ops.object.join()
    return bpy.context.active_object


def apply_voxel_remesh(obj, voxel_size):
    """Apply voxel remesh modifier for even triangle distribution."""
    bpy.context.view_layer.objects.active = obj
    obj.select_set(True)

    mod = obj.modifiers.new(name='Remesh', type='REMESH')
    mod.mode = 'VOXEL'
    mod.voxel_size = voxel_size
    mod.use_smooth_shade = True

    bpy.ops.object.modifier_apply(modifier=mod.name)
    print(f"  Voxel remesh applied (size={voxel_size:.4f}), tris: {count_triangles()}")


def apply_decimate(obj, ratio=0.5, target_tris=0):
    """Apply decimate modifier."""
    bpy.context.view_layer.objects.active = obj
    obj.select_set(True)

    if target_tris > 0:
        current = count_triangles()
        if current > 0:
            ratio = min(target_tris / current, 1.0)

    mod = obj.modifiers.new(name='Decimate', type='DECIMATE')
    mod.decimate_type = 'COLLAPSE'
    mod.ratio = max(ratio, 0.001)
    mod.use_collapse_triangulate = True

    bpy.ops.object.modifier_apply(modifier=mod.name)
    print(f"  Decimate applied (ratio={ratio:.4f}), tris: {count_triangles()}")


def apply_planar_dissolve(obj, angle_degrees=5.0):
    """Dissolve planar faces — removes unnecessary geometry on flat surfaces."""
    bpy.context.view_layer.objects.active = obj
    obj.select_set(True)

    bpy.ops.object.mode_set(mode='EDIT')
    bpy.ops.mesh.select_all(action='SELECT')
    bpy.ops.mesh.dissolve_limited(angle_limit=math.radians(angle_degrees))
    bpy.ops.mesh.select_all(action='SELECT')
    bpy.ops.mesh.select_all(action='SELECT')
    bpy.ops.mesh.quads_convert_to_tris()  # Re-triangulate for GLB export
    bpy.ops.object.mode_set(mode='OBJECT')

    print(f"  Planar dissolve applied (angle={angle_degrees}°), tris: {count_triangles()}")


def apply_unsubdivide(obj, iterations=1):
    """Reverse subdivision — perfect for meshes that were subdivided."""
    bpy.context.view_layer.objects.active = obj
    obj.select_set(True)

    bpy.ops.object.mode_set(mode='EDIT')
    bpy.ops.mesh.select_all(action='SELECT')
    bpy.ops.mesh.unsubdivide(iterations=iterations)
    bpy.ops.mesh.select_all(action='SELECT')
    bpy.ops.mesh.quads_to_tris()
    bpy.ops.object.mode_set(mode='OBJECT')

    print(f"  Unsubdivide applied ({iterations} iterations), tris: {count_triangles()}")


def apply_smooth_normals(obj):
    """Apply smooth shading for better visual quality at low poly."""
    bpy.context.view_layer.objects.active = obj
    obj.select_set(True)
    bpy.ops.object.shade_smooth()

    # Auto smooth normals for hard edges
    if hasattr(obj.data, 'use_auto_smooth'):
        obj.data.use_auto_smooth = True
        obj.data.auto_smooth_angle = math.radians(60)


def export_glb(filepath):
    """Export scene as GLB."""
    bpy.ops.export_scene.gltf(
        filepath=filepath,
        export_format='GLB',
        export_apply=True,
        export_texcoords=True,
        export_normals=True,
        export_materials='EXPORT',
        export_image_format='AUTO',
    )


def main():
    args = parse_args()

    print(f"\n{'='*60}")
    print(f"Blender LOD Generator")
    print(f"  Input:  {args.input}")
    print(f"  Output: {args.output}")
    print(f"  Mode:   {args.mode}")
    print(f"{'='*60}\n")

    clear_scene()
    import_glb(args.input)

    initial_tris = count_triangles()
    print(f"Imported: {initial_tris} triangles\n")

    obj = join_meshes()
    if obj is None:
        print("ERROR: No mesh objects found after import")
        sys.exit(1)

    if args.mode == 'remesh_decimate':
        # Step 1: Voxel remesh for even topology
        voxel_size = args.voxel_size if args.voxel_size > 0 else auto_voxel_size(args.decimate_ratio)
        print(f"Step 1: Voxel remesh (size={voxel_size:.4f})")
        apply_voxel_remesh(obj, voxel_size)

        # Step 2: Decimate to target
        print(f"Step 2: Decimate (ratio={args.decimate_ratio})")
        apply_decimate(obj, args.decimate_ratio, args.target_tris)

        # Step 3: Clean up flat areas
        print(f"Step 3: Planar dissolve (angle={args.planar_angle}°)")
        apply_planar_dissolve(obj, args.planar_angle)

    elif args.mode == 'decimate':
        print(f"Step 1: Decimate (ratio={args.decimate_ratio})")
        apply_decimate(obj, args.decimate_ratio, args.target_tris)

        print(f"Step 2: Planar dissolve (angle={args.planar_angle}°)")
        apply_planar_dissolve(obj, args.planar_angle)

    elif args.mode == 'planar':
        print(f"Step 1: Planar dissolve (angle={args.planar_angle}°)")
        apply_planar_dissolve(obj, args.planar_angle)

    elif args.mode == 'unsubdivide':
        print(f"Step 1: Unsubdivide ({args.iterations} iterations)")
        apply_unsubdivide(obj, args.iterations)

    # Smooth shading for visual quality
    apply_smooth_normals(obj)

    final_tris = count_triangles()
    reduction = (1 - final_tris / initial_tris) * 100 if initial_tris > 0 else 0
    print(f"\nResult: {initial_tris} -> {final_tris} triangles ({reduction:.1f}% reduction)")

    export_glb(args.output)
    print(f"Exported: {args.output}\n")


if __name__ == '__main__':
    main()
