# Structure — T-001-01: Raised Bed Parametric Reconstruction

## File Changes

### New Files

#### `scripts/parametric_reconstruct.py` (main entry point, ~400 lines)

Single-file script combining analysis, reconstruction, and GLB output. No external dependencies beyond numpy (already implied by the project's Blender usage). Falls back to pure Python if numpy unavailable.

**Module sections:**

1. **GLB Parser** — Read glTF 2.0 binary, extract positions + indices
2. **Mesh Analyzer** — Compute normals, detect axis-aligned boards via Y-layer slicing
3. **Board Detector** — Group geometry into rectangular slabs, snap to standard lumber
4. **GLB Builder** — Generate box primitives, UV-map with tiled texture, write GLB binary
5. **Cut List Formatter** — Print human-readable cut list to stdout
6. **CLI** — argparse entry point

**Public interface (CLI):**
```
python3 scripts/parametric_reconstruct.py \
    --input assets/wood_raised_bed.glb \
    --output assets/wood_raised_bed_parametric.glb \
    --texture-size 128
```

**Output:**
- Parametric GLB file at `--output` path
- Cut list printed to stdout
- JSON board manifest printed to stderr (for debugging/integration)

### Modified Files

None. This is a standalone script with no changes to the existing Go server.

### Deleted Files

None.

## Architecture

### Single-File Design Rationale

The analysis + reconstruction logic is tightly coupled (board detection feeds directly into box generation). Splitting into multiple files adds import complexity without benefit. A single well-sectioned script is easier to run standalone, embed in Blender, or invoke from the Go server.

### Data Flow

```
wood_raised_bed.glb
    │
    ▼
┌─────────────┐
│  GLB Parser  │  → positions[], indices[], texture_bytes
└─────┬───────┘
      │
      ▼
┌─────────────────┐
│  Mesh Analyzer   │  → face_normals[], axis_groups{}
└─────┬───────────┘
      │
      ▼
┌──────────────────┐
│  Board Detector   │  → boards[{center, dims, orientation, lumber_type}]
└─────┬────────────┘
      │
      ▼
┌───────────────────────┐
│  GLB Builder           │  → parametric.glb
│  (boxes + texture)     │
└─────┬─────────────────┘
      │
      ▼
┌──────────────────┐
│  Cut List Output  │  → stdout text
└──────────────────┘
```

### Board Detection Algorithm

1. **Y-axis layer detection**: Histogram vertex Y-coordinates. Peaks indicate board surfaces. Gaps between peaks = board boundaries. Expected: 2-3 horizontal board layers.

2. **Per-layer extent mapping**: For each Y-layer, find the X and Z extents of geometry. Boards on the ±X faces span the full Z range. Boards on the ±Z faces span partial X range (between posts).

3. **Corner post detection**: Vertical geometry at the 4 corners (±X, ±Z extremes) extending full Y height → 4×4 posts.

4. **Board extraction**: For each detected slab:
   - Position: center of the detected region
   - Dimensions: extent in each axis
   - Orientation: which axis is the board length
   - Lumber type: snap thickness/width to standard sizes

### GLB Builder Internals

**Box generation:**
- 8 vertices per box (shared corners, flat shading via face normals)
- Actually 24 vertices (4 per face × 6 faces) to support per-face UVs and normals
- 12 triangles per box (2 per face × 6 faces)
- UV mapping: tile texture along board length, scale by board dimensions

**Texture handling:**
- Extract the original PNG from the source GLB's binary chunk
- Decode with Python's built-in `struct` module (locate PNG in buffer)
- Resize to 128×128 using nearest-neighbor (pure Python) or PIL if available
- Re-encode as JPEG for compression
- Fallback: generate a solid brown color (128×128 single-color JPEG, ~1KB)

**GLB assembly:**
- Build JSON chunk: scene → node → mesh → accessor → bufferView → buffer
- Build binary chunk: vertex data (positions + normals + UVs) + index data + texture image
- Write 12-byte header + JSON chunk + BIN chunk

### Cut List Format

```
Cut List — Raised Bed Parametric Reconstruction
================================================
Qty  Lumber    Length     Description
---  ------    ------     -----------
 4   4×4       10.0 in   Corner posts
 4   2×6       48.0 in   Long side boards
 4   2×6       24.0 in   Short side boards
================================================
Total: 12 pieces
```

### Output Validation

The script prints a summary:
- Original: X triangles, Y bytes
- Parametric: X triangles, Y bytes
- Reduction: X% triangles, Y% file size
- Board count and cut list

## Boundaries

- Script does NOT modify any Go source code
- Script does NOT require Blender or gltfpack
- Script requires Python 3.8+ (f-strings, struct, json, argparse — all stdlib)
- numpy is optional (used for faster vertex math if available, pure-Python fallback)
- PIL/Pillow is optional (used for texture resizing if available, fallback to raw extraction)
