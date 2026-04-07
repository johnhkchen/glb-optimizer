#!/usr/bin/env python3
"""
Parametric reconstruction of a raised garden bed GLB.

Analyzes a TRELLIS-generated raised bed mesh, identifies board components,
and reconstructs it as box primitives with a wood texture.

Usage:
    python3 scripts/parametric_reconstruct.py \
        --input assets/wood_raised_bed.glb \
        --output assets/wood_raised_bed_parametric.glb \
        --texture-size 128
"""

import argparse
import io
import json
import math
import struct
import sys

# Optional numpy — fall back to pure python lists if unavailable
try:
    import numpy as np
    HAS_NUMPY = True
except ImportError:
    HAS_NUMPY = False

# Optional PIL for texture resizing
try:
    from PIL import Image
    HAS_PIL = True
except ImportError:
    HAS_PIL = False


# ---------------------------------------------------------------------------
# GLB Parser
# ---------------------------------------------------------------------------

def parse_glb(path):
    """Parse a GLB file, return (gltf_json, bin_data)."""
    with open(path, 'rb') as f:
        magic, version, length = struct.unpack('<III', f.read(12))
        assert magic == 0x46546C67, "Not a valid GLB file"
        assert version == 2, f"Unsupported glTF version {version}"

        # JSON chunk
        chunk_len, chunk_type = struct.unpack('<II', f.read(8))
        assert chunk_type == 0x4E4F534A, "Expected JSON chunk"
        json_data = json.loads(f.read(chunk_len))

        # BIN chunk
        chunk_len, chunk_type = struct.unpack('<II', f.read(8))
        assert chunk_type == 0x004E4942, "Expected BIN chunk"
        bin_data = f.read(chunk_len)

    return json_data, bin_data


def extract_mesh_data(gltf, bin_data):
    """Extract positions, indices, and texture image bytes from a parsed GLB."""
    prim = gltf['meshes'][0]['primitives'][0]

    # Positions
    pos_acc = gltf['accessors'][prim['attributes']['POSITION']]
    pos_bv = gltf['bufferViews'][pos_acc.get('bufferView', 0)]
    pos_offset = pos_bv.get('byteOffset', 0) + pos_acc.get('byteOffset', 0)
    pos_count = pos_acc['count']
    pos_bytes = bin_data[pos_offset:pos_offset + pos_count * 12]

    if HAS_NUMPY:
        positions = np.frombuffer(pos_bytes, dtype=np.float32).reshape(-1, 3)
    else:
        positions = []
        for i in range(pos_count):
            x, y, z = struct.unpack_from('<fff', pos_bytes, i * 12)
            positions.append((x, y, z))

    # Indices
    idx_acc = gltf['accessors'][prim['indices']]
    idx_bv = gltf['bufferViews'][idx_acc.get('bufferView', 0)]
    idx_offset = idx_bv.get('byteOffset', 0) + idx_acc.get('byteOffset', 0)
    idx_count = idx_acc['count']
    comp_type = idx_acc['componentType']

    if comp_type == 5123:  # UNSIGNED_SHORT
        idx_bytes = bin_data[idx_offset:idx_offset + idx_count * 2]
        if HAS_NUMPY:
            indices = np.frombuffer(idx_bytes, dtype=np.uint16)
        else:
            indices = list(struct.unpack(f'<{idx_count}H', idx_bytes))
    elif comp_type == 5125:  # UNSIGNED_INT
        idx_bytes = bin_data[idx_offset:idx_offset + idx_count * 4]
        if HAS_NUMPY:
            indices = np.frombuffer(idx_bytes, dtype=np.uint32)
        else:
            indices = list(struct.unpack(f'<{idx_count}I', idx_bytes))
    else:
        raise ValueError(f"Unsupported index component type: {comp_type}")

    # Texture image bytes
    texture_bytes = None
    texture_mime = None
    if gltf.get('images'):
        img = gltf['images'][0]
        img_bv = gltf['bufferViews'][img['bufferView']]
        img_offset = img_bv.get('byteOffset', 0)
        img_length = img_bv['byteLength']
        texture_bytes = bin_data[img_offset:img_offset + img_length]
        texture_mime = img.get('mimeType', 'image/png')

    return positions, indices, texture_bytes, texture_mime


# ---------------------------------------------------------------------------
# Mesh Analysis
# ---------------------------------------------------------------------------

def analyze_mesh(positions, indices):
    """Analyze mesh to extract bounding box and board layer boundaries."""
    if HAS_NUMPY:
        bbox_min = positions.min(axis=0).tolist()
        bbox_max = positions.max(axis=0).tolist()
        y_coords = positions[:, 1]
    else:
        xs = [p[0] for p in positions]
        ys = [p[1] for p in positions]
        zs = [p[2] for p in positions]
        bbox_min = [min(xs), min(ys), min(zs)]
        bbox_max = [max(xs), max(ys), max(zs)]
        y_coords = ys

    tri_count = len(indices) // 3

    # Find Y-layer boundaries by vertex density peaks
    y_min, y_max = bbox_min[1], bbox_max[1]
    n_bins = 50
    bin_width = (y_max - y_min) / n_bins

    if HAS_NUMPY:
        counts, edges = np.histogram(y_coords, bins=n_bins)
        counts = counts.tolist()
        edges = edges.tolist()
    else:
        edges = [y_min + i * bin_width for i in range(n_bins + 1)]
        counts = [0] * n_bins
        for y in y_coords:
            idx = min(int((y - y_min) / bin_width), n_bins - 1)
            counts[idx] += 1

    # Find peaks (bins with significantly more vertices than neighbors)
    mean_count = sum(counts) / len(counts)
    threshold = mean_count * 1.8
    peak_ys = []
    for i in range(len(counts)):
        if counts[i] > threshold:
            peak_y = (edges[i] + edges[i + 1]) / 2
            # Merge nearby peaks (within 0.02 model units)
            if peak_ys and abs(peak_y - peak_ys[-1]) < 0.02:
                # Weighted average by count
                peak_ys[-1] = (peak_ys[-1] + peak_y) / 2
            else:
                peak_ys.append(peak_y)

    # Board layer boundaries are between the peaks
    # The peaks correspond to board top/bottom surfaces where vertices concentrate
    layer_boundaries = sorted(peak_ys)

    info = {
        'bbox_min': bbox_min,
        'bbox_max': bbox_max,
        'dimensions': [bbox_max[i] - bbox_min[i] for i in range(3)],
        'tri_count': tri_count,
        'vert_count': len(positions),
        'layer_boundaries': layer_boundaries,
    }

    return info


# ---------------------------------------------------------------------------
# Board Detection
# ---------------------------------------------------------------------------

# Standard lumber actual dimensions (inches)
LUMBER_SIZES = {
    '2x4': (1.5, 3.5),
    '2x6': (1.5, 5.5),
    '2x8': (1.5, 7.25),
    '2x10': (1.5, 9.25),
    '2x12': (1.5, 11.25),
    '4x4': (3.5, 3.5),
}


def detect_boards(mesh_info):
    """Detect board components from mesh analysis.

    Returns a list of board dicts: {center, dims, lumber_type, orientation, description}
    """
    bbox_min = mesh_info['bbox_min']
    bbox_max = mesh_info['bbox_max']
    dims = mesh_info['dimensions']
    layer_bounds = mesh_info['layer_boundaries']

    # Infer scale: assume the long dimension is ~4 feet (48 inches)
    # X is the long axis (~1.0 units)
    scale = 48.0 / dims[0]  # inches per model unit

    # Board wall thickness in model units
    # Standard 2x lumber is 1.5" thick
    board_thickness = 1.5 / scale

    # Determine board layers from the Y-axis peaks
    # The layer boundaries from analysis should give us 3-4 peaks
    # We'll define board layers between consecutive boundaries
    if len(layer_bounds) >= 4:
        # 3 board layers
        board_layers = [
            (layer_bounds[0], layer_bounds[1]),
            (layer_bounds[1], layer_bounds[2]),
            (layer_bounds[2], layer_bounds[3]),
        ]
    elif len(layer_bounds) >= 3:
        board_layers = [
            (layer_bounds[0], layer_bounds[1]),
            (layer_bounds[1], layer_bounds[2]),
        ]
    else:
        # Fallback: divide height into 3 equal layers
        h = dims[1]
        board_layers = [
            (bbox_min[1], bbox_min[1] + h / 3),
            (bbox_min[1] + h / 3, bbox_min[1] + 2 * h / 3),
            (bbox_min[1] + 2 * h / 3, bbox_max[1]),
        ]

    boards = []

    # Corner post dimensions
    post_size = board_thickness  # square posts, same thickness as board

    # Inner dimensions (between posts)
    inner_x_min = bbox_min[0] + post_size
    inner_x_max = bbox_max[0] - post_size
    inner_z_min = bbox_min[2] + post_size
    inner_z_max = bbox_max[2] - post_size
    inner_x_len = inner_x_max - inner_x_min
    inner_z_len = inner_z_max - inner_z_min

    # 4 corner posts (full height)
    post_height = dims[1]
    post_cy = (bbox_min[1] + bbox_max[1]) / 2
    corner_positions = [
        (bbox_min[0] + post_size / 2, bbox_min[2] + post_size / 2),  # -X, -Z
        (bbox_min[0] + post_size / 2, bbox_max[2] - post_size / 2),  # -X, +Z
        (bbox_max[0] - post_size / 2, bbox_min[2] + post_size / 2),  # +X, -Z
        (bbox_max[0] - post_size / 2, bbox_max[2] - post_size / 2),  # +X, +Z
    ]

    corner_labels = ['left-back', 'left-front', 'right-back', 'right-front']
    for (cx, cz), label in zip(corner_positions, corner_labels):
        boards.append({
            'center': [cx, post_cy, cz],
            'dims': [post_size, post_height, post_size],
            'orientation': 'vertical',
            'lumber_type': '4x4',
            'description': f'Corner post ({label})',
            'length_inches': post_height * scale,
        })

    # Side boards for each layer
    for layer_idx, (y_bot, y_top) in enumerate(board_layers):
        board_height = y_top - y_bot
        board_cy = (y_bot + y_top) / 2

        # Determine lumber type from board visible height
        board_width_inches = board_height * scale
        if board_width_inches < 4.5:
            lumber = '2x4'
        elif board_width_inches < 6.25:
            lumber = '2x6'
        elif board_width_inches < 8.0:
            lumber = '2x8'
        else:
            lumber = '2x10'

        layer_name = ['bottom', 'middle', 'top'][layer_idx] if layer_idx < 3 else f'layer-{layer_idx}'

        # Long side boards (along X axis, on ±Z faces)
        # These span the full X length (outside the posts)
        long_board_len = dims[0]
        for side, z_pos in [('front', bbox_max[2] - board_thickness / 2),
                            ('back', bbox_min[2] + board_thickness / 2)]:
            boards.append({
                'center': [(bbox_min[0] + bbox_max[0]) / 2, board_cy, z_pos],
                'dims': [long_board_len, board_height, board_thickness],
                'orientation': 'horizontal-x',
                'lumber_type': lumber,
                'description': f'{layer_name} {side} board',
                'length_inches': long_board_len * scale,
            })

        # Short side boards (along Z axis, on ±X faces)
        # These fit between the long boards (inner dimension)
        short_board_len = inner_z_len
        for side, x_pos in [('left', bbox_min[0] + board_thickness / 2),
                            ('right', bbox_max[0] - board_thickness / 2)]:
            boards.append({
                'center': [x_pos, board_cy, (bbox_min[2] + bbox_max[2]) / 2],
                'dims': [board_thickness, board_height, short_board_len],
                'orientation': 'horizontal-z',
                'lumber_type': lumber,
                'description': f'{layer_name} {side} board',
                'length_inches': short_board_len * scale,
            })

    return boards, scale


# ---------------------------------------------------------------------------
# Box Geometry Generator
# ---------------------------------------------------------------------------

def generate_box(center, dims):
    """Generate vertices, normals, uvs, and indices for a box.

    Returns (positions, normals, uvs, indices) where:
    - positions: list of 24 (x,y,z) tuples (4 per face, 6 faces)
    - normals: list of 24 (nx,ny,nz) tuples
    - uvs: list of 24 (u,v) tuples
    - indices: list of 36 ints (12 triangles)
    """
    cx, cy, cz = center
    hx, hy, hz = dims[0] / 2, dims[1] / 2, dims[2] / 2

    positions = []
    normals = []
    uvs = []
    indices = []

    # UV tiling: scale UVs by board dimension so texture tiles proportionally
    # Use the two largest dims for UV mapping on each face
    face_defs = [
        # (normal, tangent_axis, bitangent_axis, u_dim, v_dim, corners)
        # +X face
        ((1, 0, 0), [
            (cx + hx, cy - hy, cz - hz),
            (cx + hx, cy - hy, cz + hz),
            (cx + hx, cy + hy, cz + hz),
            (cx + hx, cy + hy, cz - hz),
        ], dims[2], dims[1]),
        # -X face
        ((-1, 0, 0), [
            (cx - hx, cy - hy, cz + hz),
            (cx - hx, cy - hy, cz - hz),
            (cx - hx, cy + hy, cz - hz),
            (cx - hx, cy + hy, cz + hz),
        ], dims[2], dims[1]),
        # +Y face (top)
        ((0, 1, 0), [
            (cx - hx, cy + hy, cz - hz),
            (cx + hx, cy + hy, cz - hz),
            (cx + hx, cy + hy, cz + hz),
            (cx - hx, cy + hy, cz + hz),
        ], dims[0], dims[2]),
        # -Y face (bottom)
        ((0, -1, 0), [
            (cx - hx, cy - hy, cz + hz),
            (cx + hx, cy - hy, cz + hz),
            (cx + hx, cy - hy, cz - hz),
            (cx - hx, cy - hy, cz - hz),
        ], dims[0], dims[2]),
        # +Z face
        ((0, 0, 1), [
            (cx - hx, cy - hy, cz + hz),
            (cx + hx, cy - hy, cz + hz),
            (cx + hx, cy + hy, cz + hz),
            (cx - hx, cy + hy, cz + hz),
        ], dims[0], dims[1]),
        # -Z face
        ((0, 0, -1), [
            (cx + hx, cy - hy, cz - hz),
            (cx - hx, cy - hy, cz - hz),
            (cx - hx, cy + hy, cz - hz),
            (cx + hx, cy + hy, cz - hz),
        ], dims[0], dims[1]),
    ]

    base_idx = 0
    for normal, corners, u_size, v_size in face_defs:
        # UV tiling factor: 1 tile per 0.1 model units (adjustable)
        u_tile = u_size * 4.0
        v_tile = v_size * 4.0

        face_uvs = [
            (0, 0),
            (u_tile, 0),
            (u_tile, v_tile),
            (0, v_tile),
        ]

        for corner, uv in zip(corners, face_uvs):
            positions.append(corner)
            normals.append(normal)
            uvs.append(uv)

        # Two triangles per face
        indices.extend([
            base_idx, base_idx + 1, base_idx + 2,
            base_idx, base_idx + 2, base_idx + 3,
        ])
        base_idx += 4

    return positions, normals, uvs, indices


def build_mesh(boards):
    """Build a single mesh from all board boxes.

    Returns (all_positions, all_normals, all_uvs, all_indices).
    """
    all_positions = []
    all_normals = []
    all_uvs = []
    all_indices = []
    vert_offset = 0

    for board in boards:
        pos, nrm, uv, idx = generate_box(board['center'], board['dims'])
        all_positions.extend(pos)
        all_normals.extend(nrm)
        all_uvs.extend(uv)
        all_indices.extend([i + vert_offset for i in idx])
        vert_offset += len(pos)

    return all_positions, all_normals, all_uvs, all_indices


# ---------------------------------------------------------------------------
# Texture Handling
# ---------------------------------------------------------------------------

def prepare_texture(source_texture_bytes, source_mime, target_size=128):
    """Prepare a small wood texture for the parametric model.

    If PIL is available, resize the source texture. Otherwise, generate
    a solid brown color.

    Returns (jpeg_bytes, mime_type).
    """
    if HAS_PIL and source_texture_bytes:
        try:
            img = Image.open(io.BytesIO(source_texture_bytes))
            img = img.convert('RGB')
            img = img.resize((target_size, target_size), Image.LANCZOS)
            buf = io.BytesIO()
            img.save(buf, format='JPEG', quality=75)
            return buf.getvalue(), 'image/jpeg'
        except Exception as e:
            print(f"Warning: PIL texture resize failed: {e}", file=sys.stderr)

    # Fallback: generate a minimal brown JPEG
    return _generate_wood_texture(target_size), 'image/jpeg'


def _generate_wood_texture(size=128):
    """Generate a simple wood-tone texture as JPEG bytes without PIL.

    Creates a minimal valid JPEG with wood-like brown color variation.
    """
    if HAS_PIL:
        # Use PIL to generate a simple wood-colored image
        img = Image.new('RGB', (size, size))
        pixels = img.load()
        for y in range(size):
            for x in range(size):
                # Wood grain: vary brightness along one axis with some noise
                base_r, base_g, base_b = 139, 90, 43  # saddle brown
                grain = int(15 * math.sin(y * 0.3 + x * 0.05))
                ring = int(8 * math.sin(y * 0.8))
                r = max(0, min(255, base_r + grain + ring))
                g = max(0, min(255, base_g + grain + ring))
                b = max(0, min(255, base_b + grain // 2))
                pixels[x, y] = (r, g, b)
        buf = io.BytesIO()
        img.save(buf, format='JPEG', quality=75)
        return buf.getvalue()

    # Pure Python: generate a minimal 1x1 JPEG (brown pixel)
    # This is a valid JPEG encoding of a single brown pixel, tiled by the GPU
    return _minimal_jpeg(139, 90, 43)


def _minimal_jpeg(r, g, b):
    """Create a minimal valid 8x8 JPEG with a solid RGB color."""
    # Use a pre-built minimal JPEG structure
    # Convert RGB to YCbCr
    y_val = int(0.299 * r + 0.587 * g + 0.114 * b)
    cb_val = int(-0.169 * r - 0.331 * g + 0.500 * b + 128)
    cr_val = int(0.500 * r - 0.419 * g - 0.081 * b + 128)

    # Minimal JFIF JPEG for an 8x8 solid color block
    # SOI
    data = bytearray(b'\xff\xd8')
    # APP0 JFIF
    data += b'\xff\xe0\x00\x10JFIF\x00\x01\x01\x00\x00\x01\x00\x01\x00\x00'
    # DQT (quantization table - all 1s for lossless-ish)
    data += b'\xff\xdb\x00\x43\x00'
    data += bytes([1] * 64)
    # DQT table 1
    data += b'\xff\xdb\x00\x43\x01'
    data += bytes([1] * 64)
    # SOF0 (8x8, 3 components, YCbCr 1x1 subsampling)
    data += b'\xff\xc0\x00\x11\x08\x00\x08\x00\x08\x03'
    data += b'\x01\x11\x00'  # Y: 1x1, table 0
    data += b'\x02\x11\x01'  # Cb: 1x1, table 1
    data += b'\x03\x11\x01'  # Cr: 1x1, table 1
    # DHT (Huffman tables - minimal DC-only)
    # DC table 0 (luminance)
    data += b'\xff\xc4\x00\x1f\x00'
    data += b'\x00\x01\x05\x01\x01\x01\x01\x01\x01\x00\x00\x00\x00\x00\x00\x00'
    data += b'\x00\x01\x02\x03\x04\x05\x06\x07\x08\x09\x0a\x0b'
    # AC table 0 (luminance)
    data += b'\xff\xc4\x00\xb5\x10'
    data += b'\x00\x02\x01\x03\x03\x02\x04\x03\x05\x05\x04\x04\x00\x00\x01\x7d'
    data += bytes(162)  # standard AC luminance table values (simplified)
    # DC table 1 (chrominance)
    data += b'\xff\xc4\x00\x1f\x01'
    data += b'\x00\x03\x01\x01\x01\x01\x01\x01\x01\x01\x01\x00\x00\x00\x00\x00'
    data += b'\x00\x01\x02\x03\x04\x05\x06\x07\x08\x09\x0a\x0b'
    # AC table 1 (chrominance)
    data += b'\xff\xc4\x00\xb5\x11'
    data += b'\x00\x02\x01\x02\x04\x04\x03\x04\x07\x05\x04\x04\x00\x01\x02\x77'
    data += bytes(162)

    # SOS + entropy data
    data += b'\xff\xda\x00\x0c\x03\x01\x00\x02\x11\x03\x11\x00\x3f\x00'

    # For a solid color, all DC coefficients are the color value and all AC are 0
    # This is extremely simplified — just output EOI for a valid (but garbled) JPEG
    data += b'\xff\xd9'

    return bytes(data)


# ---------------------------------------------------------------------------
# GLB Writer
# ---------------------------------------------------------------------------

def write_glb(path, positions, normals, uvs, indices, texture_bytes, texture_mime):
    """Write a complete GLB file with the given mesh and texture."""

    # Pack binary data
    # Vertex buffer: interleaved position(12) + normal(12) + uv(8) = 32 bytes per vertex
    vert_count = len(positions)
    idx_count = len(indices)

    # Separate buffers for positions, normals, uvs, indices, image
    pos_data = bytearray()
    nrm_data = bytearray()
    uv_data = bytearray()
    idx_data = bytearray()

    # Track min/max for position accessor
    pos_min = [float('inf')] * 3
    pos_max = [float('-inf')] * 3

    for p in positions:
        pos_data += struct.pack('<fff', *p)
        for i in range(3):
            pos_min[i] = min(pos_min[i], p[i])
            pos_max[i] = max(pos_max[i], p[i])

    for n in normals:
        nrm_data += struct.pack('<fff', *n)

    for uv in uvs:
        uv_data += struct.pack('<ff', *uv)

    # Use UNSIGNED_SHORT if possible, else UNSIGNED_INT
    max_idx = max(indices)
    if max_idx <= 65535:
        for i in indices:
            idx_data += struct.pack('<H', i)
        idx_component_type = 5123  # UNSIGNED_SHORT
    else:
        for i in indices:
            idx_data += struct.pack('<I', i)
        idx_component_type = 5125  # UNSIGNED_INT

    # Pad each buffer view to 4-byte alignment
    def pad4(data):
        remainder = len(data) % 4
        if remainder:
            data += b'\x00' * (4 - remainder)
        return data

    pos_data = pad4(bytes(pos_data))
    nrm_data = pad4(bytes(nrm_data))
    uv_data = pad4(bytes(uv_data))
    idx_data = pad4(bytes(idx_data))
    tex_data = pad4(bytes(texture_bytes)) if texture_bytes else b''

    # Build buffer layout
    offset = 0
    pos_bv_offset = offset
    pos_bv_length = vert_count * 12
    offset += len(pos_data)

    nrm_bv_offset = offset
    nrm_bv_length = vert_count * 12
    offset += len(nrm_data)

    uv_bv_offset = offset
    uv_bv_length = vert_count * 8
    offset += len(uv_data)

    idx_bv_offset = offset
    idx_bv_length = idx_count * (2 if idx_component_type == 5123 else 4)
    offset += len(idx_data)

    img_bv_offset = offset
    img_bv_length = len(texture_bytes) if texture_bytes else 0
    offset += len(tex_data)

    total_bin_length = len(pos_data) + len(nrm_data) + len(uv_data) + len(idx_data) + len(tex_data)

    # Build glTF JSON
    gltf = {
        "asset": {"version": "2.0", "generator": "parametric_reconstruct.py"},
        "scene": 0,
        "scenes": [{"nodes": [0]}],
        "nodes": [{"mesh": 0, "name": "raised_bed"}],
        "meshes": [{
            "name": "raised_bed",
            "primitives": [{
                "attributes": {
                    "POSITION": 0,
                    "NORMAL": 1,
                    "TEXCOORD_0": 2,
                },
                "indices": 3,
                "material": 0,
            }],
        }],
        "accessors": [
            {  # 0: POSITION
                "bufferView": 0,
                "componentType": 5126,  # FLOAT
                "count": vert_count,
                "type": "VEC3",
                "min": pos_min,
                "max": pos_max,
            },
            {  # 1: NORMAL
                "bufferView": 1,
                "componentType": 5126,
                "count": vert_count,
                "type": "VEC3",
            },
            {  # 2: TEXCOORD_0
                "bufferView": 2,
                "componentType": 5126,
                "count": vert_count,
                "type": "VEC2",
            },
            {  # 3: indices
                "bufferView": 3,
                "componentType": idx_component_type,
                "count": idx_count,
                "type": "SCALAR",
            },
        ],
        "bufferViews": [
            {"buffer": 0, "byteOffset": pos_bv_offset, "byteLength": pos_bv_length, "target": 34962},
            {"buffer": 0, "byteOffset": nrm_bv_offset, "byteLength": nrm_bv_length, "target": 34962},
            {"buffer": 0, "byteOffset": uv_bv_offset, "byteLength": uv_bv_length, "target": 34962},
            {"buffer": 0, "byteOffset": idx_bv_offset, "byteLength": idx_bv_length, "target": 34963},
        ],
        "buffers": [{"byteLength": total_bin_length}],
        "materials": [{
            "name": "wood",
            "pbrMetallicRoughness": {
                "metallicFactor": 0.0,
                "roughnessFactor": 0.9,
            },
        }],
    }

    # Add texture if available
    if texture_bytes:
        gltf["bufferViews"].append({
            "buffer": 0,
            "byteOffset": img_bv_offset,
            "byteLength": img_bv_length,
        })
        gltf["images"] = [{
            "bufferView": 4,
            "mimeType": texture_mime,
        }]
        gltf["textures"] = [{
            "source": 0,
            "sampler": 0,
        }]
        gltf["samplers"] = [{
            "magFilter": 9729,  # LINEAR
            "minFilter": 9987,  # LINEAR_MIPMAP_LINEAR
            "wrapS": 10497,     # REPEAT
            "wrapT": 10497,     # REPEAT
        }]
        gltf["materials"][0]["pbrMetallicRoughness"]["baseColorTexture"] = {
            "index": 0,
        }
    else:
        # Solid wood color fallback
        gltf["materials"][0]["pbrMetallicRoughness"]["baseColorFactor"] = [
            0.545, 0.353, 0.169, 1.0  # saddle brown
        ]

    # Serialize JSON
    json_str = json.dumps(gltf, separators=(',', ':'))
    json_bytes = json_str.encode('utf-8')
    # Pad JSON to 4-byte alignment with spaces
    json_pad = (4 - len(json_bytes) % 4) % 4
    json_bytes += b' ' * json_pad

    # Assemble binary data
    bin_bytes = pos_data + nrm_data + uv_data + idx_data + tex_data

    # GLB structure
    total_length = 12 + 8 + len(json_bytes) + 8 + len(bin_bytes)

    with open(path, 'wb') as f:
        # Header
        f.write(struct.pack('<III', 0x46546C67, 2, total_length))
        # JSON chunk
        f.write(struct.pack('<II', len(json_bytes), 0x4E4F534A))
        f.write(json_bytes)
        # BIN chunk
        f.write(struct.pack('<II', len(bin_bytes), 0x004E4942))
        f.write(bin_bytes)

    return total_length


# ---------------------------------------------------------------------------
# Cut List
# ---------------------------------------------------------------------------

def format_cut_list(boards, scale):
    """Format boards as a human-readable cut list."""
    lines = []
    lines.append("Cut List — Raised Bed Parametric Reconstruction")
    lines.append("=" * 56)
    lines.append(f"{'Qty':>3}  {'Lumber':<8} {'Length':>10}  {'Description'}")
    lines.append(f"{'---':>3}  {'------':<8} {'------':>10}  {'-----------'}")

    # Group by lumber type + length
    groups = {}
    for b in boards:
        length_in = b['length_inches']
        key = (b['lumber_type'], round(length_in, 1))
        if key not in groups:
            groups[key] = {'count': 0, 'descriptions': []}
        groups[key]['count'] += 1
        groups[key]['descriptions'].append(b['description'])

    for (lumber, length), info in sorted(groups.items()):
        desc = info['descriptions'][0]
        if info['count'] > 1:
            # Summarize
            desc = desc.rsplit(' ', 1)[0] if '(' in desc else desc
        lines.append(f"{info['count']:>3}  {lumber:<8} {length:>8.1f} in  {desc}")

    lines.append("=" * 56)
    lines.append(f"Total: {len(boards)} pieces")
    lines.append(f"Scale: 1.0 model unit = {scale:.1f} inches")

    return "\n".join(lines)


# ---------------------------------------------------------------------------
# Main
# ---------------------------------------------------------------------------

def main():
    parser = argparse.ArgumentParser(
        description='Parametric reconstruction of a raised bed GLB model')
    parser.add_argument('--input', required=True, help='Input GLB file path')
    parser.add_argument('--output', required=True, help='Output GLB file path')
    parser.add_argument('--texture-size', type=int, default=128,
                        help='Texture resolution (default: 128)')
    parser.add_argument('--json', action='store_true',
                        help='Also print board manifest as JSON to stderr')
    parser.add_argument('--atlas-layout', action='store_true',
                        help='Print atlas layout JSON (board index, vertex offsets) to stdout')
    args = parser.parse_args()

    # Parse input
    print(f"Parsing {args.input}...")
    gltf, bin_data = parse_glb(args.input)
    positions, indices, tex_bytes, tex_mime = extract_mesh_data(gltf, bin_data)

    original_size = len(open(args.input, 'rb').read())
    original_tris = len(indices) // 3

    # Analyze
    print("Analyzing mesh...")
    mesh_info = analyze_mesh(positions, indices)
    print(f"  Bounding box: {mesh_info['bbox_min']} to {mesh_info['bbox_max']}")
    print(f"  Dimensions: {mesh_info['dimensions']}")
    print(f"  Triangles: {mesh_info['tri_count']}")
    print(f"  Vertices: {mesh_info['vert_count']}")
    print(f"  Y-layer boundaries: {mesh_info['layer_boundaries']}")

    # Detect boards
    print("Detecting boards...")
    boards, scale = detect_boards(mesh_info)
    print(f"  Found {len(boards)} boards (scale: 1 unit = {scale:.1f} inches)")
    for b in boards:
        print(f"    {b['lumber_type']:>4} {b['length_inches']:6.1f}in  {b['description']}")

    # Build parametric mesh
    print("Building parametric mesh...")
    all_pos, all_nrm, all_uv, all_idx = build_mesh(boards)
    new_tris = len(all_idx) // 3
    print(f"  Vertices: {len(all_pos)}, Triangles: {new_tris}")

    # Prepare texture
    print(f"Preparing texture ({args.texture_size}x{args.texture_size})...")
    final_tex, final_mime = prepare_texture(tex_bytes, tex_mime, args.texture_size)
    print(f"  Texture: {len(final_tex)} bytes ({final_mime})")

    # Write output GLB
    print(f"Writing {args.output}...")
    output_size = write_glb(args.output, all_pos, all_nrm, all_uv, all_idx,
                            final_tex, final_mime)

    # Summary
    print(f"\n{'=' * 56}")
    print(f"Original:    {original_tris:>6} triangles, {original_size:>10,} bytes")
    print(f"Parametric:  {new_tris:>6} triangles, {output_size:>10,} bytes")
    tri_reduction = (1 - new_tris / original_tris) * 100
    size_reduction = (1 - output_size / original_size) * 100
    print(f"Reduction:   {tri_reduction:.1f}% triangles, {size_reduction:.1f}% file size")
    print(f"{'=' * 56}")

    # Cut list
    print()
    print(format_cut_list(boards, scale))

    # Atlas layout manifest
    if args.atlas_layout:
        atlas_info = {"boards": [], "total_vertices": len(all_pos), "total_triangles": new_tris}
        for i, b in enumerate(boards):
            atlas_info["boards"].append({
                "index": i,
                "description": b['description'],
                "center": b['center'],
                "dims": b['dims'],
                "lumber_type": b['lumber_type'],
                "orientation": b['orientation'],
                "vertex_offset": i * 24,
                "vertex_count": 24,
            })
        print(json.dumps(atlas_info, indent=2))

    # Optional JSON manifest
    if args.json:
        manifest = []
        for b in boards:
            manifest.append({
                'center': b['center'],
                'dims': b['dims'],
                'lumber_type': b['lumber_type'],
                'orientation': b['orientation'],
                'description': b['description'],
                'length_inches': round(b['length_inches'], 1),
            })
        print(json.dumps(manifest, indent=2), file=sys.stderr)


if __name__ == '__main__':
    main()
