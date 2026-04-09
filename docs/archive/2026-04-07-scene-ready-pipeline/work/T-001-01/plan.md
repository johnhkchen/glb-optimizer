# Plan — T-001-01: Raised Bed Parametric Reconstruction

## Implementation Steps

### Step 1: GLB Parser + Mesh Analyzer

Write the GLB binary parser and mesh analysis functions in `scripts/parametric_reconstruct.py`.

**Substeps:**
1. Parse GLB header (magic, version, length)
2. Read JSON chunk → extract accessor/bufferView metadata
3. Read BIN chunk → extract vertex positions, indices, texture image bytes
4. Compute per-face normals from triangle positions
5. Classify faces by dominant axis (±X, ±Y, ±Z)

**Verification:** Run on `assets/wood_raised_bed.glb`, confirm 10,402 triangles, correct bounding box, ~82% axis-aligned faces.

### Step 2: Board Detection

Implement Y-layer slicing and board identification.

**Substeps:**
1. Histogram vertex Y-coordinates to find board layer boundaries
2. For each layer, compute X/Z extents of axis-aligned faces
3. Identify corner posts (geometry at all 4 X/Z extremes spanning full height)
4. Identify side boards (horizontal slabs on ±X and ±Z faces)
5. Compute center, dimensions, and orientation for each board
6. Snap dimensions to standard lumber sizes (2×4, 2×6, 2×8, 4×4)
7. Output board manifest as list of dicts

**Verification:** Expected 12-20 boards. Check that total board volume approximately matches mesh bounding box geometry. Print board list for manual inspection.

### Step 3: Box Geometry Generator

Build the parametric mesh from detected boards.

**Substeps:**
1. For each board, generate 24 vertices (4 per face × 6 faces, with normals and UVs)
2. Generate 36 indices per box (12 triangles)
3. UV-map: grain runs along board length, texture tiles proportionally to board dimensions
4. Accumulate all vertices/indices into a single mesh with shared buffer
5. Compute total triangle count, verify <300

**Verification:** Triangle count = boards × 12. All vertices within expected bounding box.

### Step 4: Texture Handling

Extract or generate a wood texture for the parametric model.

**Substeps:**
1. Extract the PNG image bytes from the source GLB's binary chunk
2. If PIL available: resize to 128×128, save as JPEG quality 70
3. If PIL unavailable: generate a minimal solid-color brown JPEG (stdlib only)
4. Store the texture bytes for embedding in output GLB

**Verification:** Texture is <15KB. Output is valid JPEG/PNG.

### Step 5: GLB Writer

Assemble and write the output GLB file.

**Substeps:**
1. Build the glTF JSON structure: scene, nodes, mesh, material, texture, image, accessors, bufferViews, buffer
2. Pack binary data: vertex buffer (positions + normals + UVs), index buffer, image bytes
3. Pad chunks to 4-byte alignment per glTF spec
4. Write GLB header + JSON chunk + BIN chunk
5. Verify output file size <50KB

**Verification:** Open output in a glTF validator or 3D viewer. File size <50KB. Valid GLB structure.

### Step 6: Cut List + CLI

Add the cut list formatter and CLI argument parsing.

**Substeps:**
1. Format board manifest as a human-readable cut list table
2. Include lumber type, dimensions in inches, quantities grouped by type
3. Wire up argparse: `--input`, `--output`, `--texture-size`
4. Print summary stats (triangle reduction, size reduction)

**Verification:** Run end-to-end: `python3 scripts/parametric_reconstruct.py --input assets/wood_raised_bed.glb --output assets/wood_raised_bed_parametric.glb`. Confirm output GLB exists, <50KB, cut list printed.

### Step 7: Validation + Edge Cases

Test and harden.

**Substeps:**
1. Verify output GLB loads in Three.js / glTF viewer
2. Confirm triangle count <300
3. Confirm file size <50KB
4. Verify cut list is reasonable (standard lumber sizes, correct quantities)
5. Test with PIL available and without (fallback texture path)

**Verification:** All acceptance criteria met.

## Testing Strategy

- **Unit-level**: Each function tested by running the script on the known input and checking intermediate outputs via print statements / stderr JSON
- **Integration**: End-to-end run produces valid GLB meeting size/triangle targets
- **Visual**: Output GLB loaded in a 3D viewer should be recognizable as the same raised bed
- **No automated test suite**: This is a standalone analysis script, not library code. Verification is via the output artifacts and printed statistics.

## Commit Plan

1. **Commit 1**: Complete `scripts/parametric_reconstruct.py` with all functionality
2. **Commit 2**: Generated output `assets/wood_raised_bed_parametric.glb` (if we track generated assets)

Single commit is likely sufficient since this is one self-contained script.
