#!/usr/bin/env python3
"""
Synthetic trellis asset generator for T-004-05.

Builds a horizontal trellis panel — 4 horizontal slats × 7 vertical
stiles, joined as a single triangulated mesh — and writes it as a GLB
to the path given on the command line. Geometry is wider than tall so
the classifier's PCA + horizontal-orientation disambiguation lands the
asset in the `directional` bucket (vs. `tall-narrow`). See
docs/active/work/T-004-05/design.md for the rationale.

Usage:
    python3 scripts/make_trellis_asset.py assets/trellis_synthetic.glb

Reproducible: no RNG, geometry is deterministic. Re-running overwrites
the output. The GLB-writer pattern is borrowed from
scripts/parametric_reconstruct.py::write_glb.
"""

import json
import os
import struct
import sys


# Panel dimensions in metres. Wider than tall so the principal PCA
# axis is horizontal — the disambiguation in classify_shape.py prefers
# `directional` over the colocated `tall-narrow` centroid in that case.
PANEL_WIDTH = 2.0
PANEL_HEIGHT = 0.8
PANEL_DEPTH = 0.04

# Slat / stile counts and thickness. Tuned so the per-axis vertex
# density is uniform enough that the hard-surface overlay
# (mean_peakiness threshold = 2.5) does not fire — see
# docs/active/work/T-004-05/design.md "Hard-surface overlay false
# positive" risk.
N_HORIZONTAL_SLATS = 12
N_VERTICAL_STILES = 16
SLAT_THICKNESS = 0.025  # vertical extent of each horizontal slat
STILE_THICKNESS = 0.025  # horizontal extent of each vertical stile


def _box(cx, cy, cz, sx, sy, sz):
    """Return (positions, normals, uvs, indices) for one axis-aligned
    box centred at (cx, cy, cz) with half-extents (sx/2, sy/2, sz/2).
    24 verts (4 per face × 6 faces), 12 triangles. UVs are face-local."""
    hx, hy, hz = sx / 2, sy / 2, sz / 2
    # Per-face: 4 corner positions + 4 normals + 4 uvs + 6 indices.
    faces = [
        # +X
        ([(cx + hx, cy - hy, cz - hz),
          (cx + hx, cy + hy, cz - hz),
          (cx + hx, cy + hy, cz + hz),
          (cx + hx, cy - hy, cz + hz)],
         (1.0, 0.0, 0.0)),
        # -X
        ([(cx - hx, cy - hy, cz + hz),
          (cx - hx, cy + hy, cz + hz),
          (cx - hx, cy + hy, cz - hz),
          (cx - hx, cy - hy, cz - hz)],
         (-1.0, 0.0, 0.0)),
        # +Y
        ([(cx - hx, cy + hy, cz - hz),
          (cx - hx, cy + hy, cz + hz),
          (cx + hx, cy + hy, cz + hz),
          (cx + hx, cy + hy, cz - hz)],
         (0.0, 1.0, 0.0)),
        # -Y
        ([(cx - hx, cy - hy, cz + hz),
          (cx - hx, cy - hy, cz - hz),
          (cx + hx, cy - hy, cz - hz),
          (cx + hx, cy - hy, cz + hz)],
         (0.0, -1.0, 0.0)),
        # +Z
        ([(cx - hx, cy - hy, cz + hz),
          (cx + hx, cy - hy, cz + hz),
          (cx + hx, cy + hy, cz + hz),
          (cx - hx, cy + hy, cz + hz)],
         (0.0, 0.0, 1.0)),
        # -Z
        ([(cx + hx, cy - hy, cz - hz),
          (cx - hx, cy - hy, cz - hz),
          (cx - hx, cy + hy, cz - hz),
          (cx + hx, cy + hy, cz - hz)],
         (0.0, 0.0, -1.0)),
    ]
    positions, normals, uvs, indices = [], [], [], []
    base_uvs = [(0.0, 0.0), (1.0, 0.0), (1.0, 1.0), (0.0, 1.0)]
    for verts, normal in faces:
        i0 = len(positions)
        positions.extend(verts)
        normals.extend([normal] * 4)
        uvs.extend(base_uvs)
        indices.extend([i0, i0 + 1, i0 + 2, i0, i0 + 2, i0 + 3])
    return positions, normals, uvs, indices


def build_trellis_mesh():
    positions, normals, uvs, indices = [], [], [], []

    # Horizontal slats span the full width, distributed equally along Y.
    if N_HORIZONTAL_SLATS == 1:
        ys = [0.0]
    else:
        step = (PANEL_HEIGHT - SLAT_THICKNESS) / (N_HORIZONTAL_SLATS - 1)
        ys = [-PANEL_HEIGHT / 2 + SLAT_THICKNESS / 2 + i * step
              for i in range(N_HORIZONTAL_SLATS)]
    for y in ys:
        p, n, u, idx = _box(0.0, y, 0.0,
                            PANEL_WIDTH, SLAT_THICKNESS, PANEL_DEPTH)
        i0 = len(positions)
        positions.extend(p)
        normals.extend(n)
        uvs.extend(u)
        indices.extend(j + i0 for j in idx)

    # Vertical stiles span the full height, distributed equally along X.
    if N_VERTICAL_STILES == 1:
        xs = [0.0]
    else:
        step = (PANEL_WIDTH - STILE_THICKNESS) / (N_VERTICAL_STILES - 1)
        xs = [-PANEL_WIDTH / 2 + STILE_THICKNESS / 2 + i * step
              for i in range(N_VERTICAL_STILES)]
    for x in xs:
        p, n, u, idx = _box(x, 0.0, 0.0,
                            STILE_THICKNESS, PANEL_HEIGHT, PANEL_DEPTH)
        i0 = len(positions)
        positions.extend(p)
        normals.extend(n)
        uvs.extend(u)
        indices.extend(j + i0 for j in idx)

    return positions, normals, uvs, indices


def write_glb(path, positions, normals, uvs, indices):
    """Hand-rolled GLB v2 writer. Mirrors
    scripts/parametric_reconstruct.py::write_glb but stripped of the
    texture branch (the trellis is solid-colored)."""
    vert_count = len(positions)
    idx_count = len(indices)

    pos_data = bytearray()
    nrm_data = bytearray()
    uv_data = bytearray()
    idx_data = bytearray()

    pos_min = [float("inf")] * 3
    pos_max = [float("-inf")] * 3
    for p in positions:
        pos_data += struct.pack("<fff", *p)
        for i in range(3):
            pos_min[i] = min(pos_min[i], p[i])
            pos_max[i] = max(pos_max[i], p[i])
    for n in normals:
        nrm_data += struct.pack("<fff", *n)
    for uv in uvs:
        uv_data += struct.pack("<ff", *uv)

    if max(indices) <= 65535:
        for i in indices:
            idx_data += struct.pack("<H", i)
        idx_component_type = 5123  # UNSIGNED_SHORT
    else:
        for i in indices:
            idx_data += struct.pack("<I", i)
        idx_component_type = 5125  # UNSIGNED_INT

    def pad4(buf):
        rem = len(buf) % 4
        if rem:
            buf += b"\x00" * (4 - rem)
        return buf

    pos_data = pad4(bytes(pos_data))
    nrm_data = pad4(bytes(nrm_data))
    uv_data = pad4(bytes(uv_data))
    idx_data = pad4(bytes(idx_data))

    offset = 0
    pos_off = offset; pos_len = vert_count * 12; offset += len(pos_data)
    nrm_off = offset; nrm_len = vert_count * 12; offset += len(nrm_data)
    uv_off  = offset; uv_len  = vert_count * 8;  offset += len(uv_data)
    idx_off = offset; idx_len = idx_count * (2 if idx_component_type == 5123 else 4)
    offset += len(idx_data)
    total_bin = len(pos_data) + len(nrm_data) + len(uv_data) + len(idx_data)

    gltf = {
        "asset": {"version": "2.0", "generator": "make_trellis_asset.py"},
        "scene": 0,
        "scenes": [{"nodes": [0]}],
        "nodes": [{"mesh": 0, "name": "trellis"}],
        "meshes": [{
            "name": "trellis",
            "primitives": [{
                "attributes": {"POSITION": 0, "NORMAL": 1, "TEXCOORD_0": 2},
                "indices": 3,
                "material": 0,
            }],
        }],
        "accessors": [
            {"bufferView": 0, "componentType": 5126, "count": vert_count,
             "type": "VEC3", "min": pos_min, "max": pos_max},
            {"bufferView": 1, "componentType": 5126, "count": vert_count, "type": "VEC3"},
            {"bufferView": 2, "componentType": 5126, "count": vert_count, "type": "VEC2"},
            {"bufferView": 3, "componentType": idx_component_type,
             "count": idx_count, "type": "SCALAR"},
        ],
        "bufferViews": [
            {"buffer": 0, "byteOffset": pos_off, "byteLength": pos_len, "target": 34962},
            {"buffer": 0, "byteOffset": nrm_off, "byteLength": nrm_len, "target": 34962},
            {"buffer": 0, "byteOffset": uv_off,  "byteLength": uv_len,  "target": 34962},
            {"buffer": 0, "byteOffset": idx_off, "byteLength": idx_len, "target": 34963},
        ],
        "buffers": [{"byteLength": total_bin}],
        "materials": [{
            "name": "wood",
            "pbrMetallicRoughness": {
                "metallicFactor": 0.0,
                "roughnessFactor": 0.9,
                "baseColorFactor": [0.545, 0.353, 0.169, 1.0],  # saddle brown
            },
        }],
    }

    json_bytes = json.dumps(gltf, separators=(",", ":")).encode("utf-8")
    pad = (4 - len(json_bytes) % 4) % 4
    json_bytes += b" " * pad

    bin_bytes = pos_data + nrm_data + uv_data + idx_data
    total_length = 12 + 8 + len(json_bytes) + 8 + len(bin_bytes)

    with open(path, "wb") as f:
        f.write(struct.pack("<III", 0x46546C67, 2, total_length))
        f.write(struct.pack("<II", len(json_bytes), 0x4E4F534A))
        f.write(json_bytes)
        f.write(struct.pack("<II", len(bin_bytes), 0x004E4942))
        f.write(bin_bytes)
    return total_length


def main(argv=None):
    argv = list(sys.argv[1:] if argv is None else argv)
    if not argv or argv[0] in ("-h", "--help"):
        print(__doc__)
        return 0
    out_path = argv[0]
    out_dir = os.path.dirname(out_path)
    if out_dir:
        os.makedirs(out_dir, exist_ok=True)
    positions, normals, uvs, indices = build_trellis_mesh()
    n = write_glb(out_path, positions, normals, uvs, indices)
    print(f"wrote {out_path} ({n} bytes, {len(positions)} verts, "
          f"{len(indices)//3} tris)", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
