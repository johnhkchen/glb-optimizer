# Review — T-001-01: Raised Bed Parametric Reconstruction

## Summary of Changes

### Files Created
- `scripts/parametric_reconstruct.py` — Standalone Python script (~450 lines) that analyzes a TRELLIS-generated raised bed GLB, identifies board components via Y-axis vertex density analysis, and reconstructs the model as parametric box primitives with a wood texture.
- `assets/wood_raised_bed_parametric.glb` — Generated output (17,924 bytes, 192 triangles)

### Files Modified
None. The existing Go web server, Blender integration, and gltfpack tooling are untouched.

### Files Deleted
None.

## Acceptance Criteria Evaluation

| Criterion | Status | Details |
|-----------|--------|---------|
| Analyze GLB and identify board components | ✓ | 16 boards detected: 4 corner posts + 12 side boards (3 layers × 4 sides) |
| Generate replacement geometry from rectangular prisms | ✓ | Box primitives with proper normals and UVs, matching standard lumber sizes |
| Bake or apply wood textures | ✓ | Original texture extracted, resized to 128×128 JPEG (3.1KB), tiled across boards |
| Output GLB <50KB | ✓ | 17,924 bytes (64% under budget) |
| Output a cut list | ✓ | Printed to stdout: 4× 4×4 posts (10.5"), 6× 2×4 (28.1"), 6× 2×4 (48.0") |
| Triangle count <300 | ✓ | 192 triangles (36% under budget) |

## Architecture Decisions

1. **Single-file script**: All analysis, generation, and output in one file. No new dependencies added to the Go server. Easy to invoke standalone or integrate later.

2. **Template-guided detection**: Rather than general-purpose mesh decomposition (which would fail on the fused TRELLIS mesh), the script uses Y-axis vertex density peaks to find board layer boundaries, then constructs boards based on standard raised bed construction patterns.

3. **Graceful degradation**: The script has 3 fallback tiers:
   - numpy + PIL → full analysis + proper texture resize
   - numpy only → full analysis + procedural wood texture
   - stdlib only → pure Python analysis + minimal solid-color texture

4. **GLB writer from scratch**: No dependency on gltfpack or Blender for output. The script writes valid glTF 2.0 binary directly using struct packing.

## Test Coverage

- **Structural validation**: Output GLB verified for correct magic bytes, version, chunk alignment, accessor counts, buffer view layout, material setup, and sampler configuration.
- **Geometric validation**: Bounding box of output matches original model dimensions. Vertex and triangle counts confirmed.
- **Size validation**: 17,924 bytes < 50,000 target; 192 triangles < 300 target.
- **No automated test suite**: This is a single-use analysis script, not a library. Verification is through the output artifact and printed statistics. A future ticket could add pytest-based tests if the script is generalized.

## Open Concerns

1. **Visual fidelity not verified in 3D viewer**: The output GLB is structurally valid, but visual inspection in Three.js or a glTF viewer has not been performed in this session. The texture tiling, UV mapping, and board proportions should be checked visually.

2. **Lumber sizing approximation**: The scale inference (1.0 model unit = 48 inches) is an assumption. The TRELLIS model has no real-world scale metadata. The 3 board layers detected as 2×4 lumber (3.5" actual width) is plausible but may not match the original reference image. Boards could be 2×6 if the real bed is larger.

3. **Board overlap at corners**: The current construction has long side boards spanning the full bed length and short side boards fitting between posts. This matches standard butt-joint construction, but there may be slight visual overlap at corners depending on the board thickness. The overlap is within the original bounding box and should appear correct.

4. **Texture quality**: The 128×128 JPEG extracted from the 1024×1024 (or similar) original PNG loses significant detail. At the target file size this is unavoidable. Using `--texture-size 256` would improve quality at ~10KB additional cost (still well under 50KB budget).

5. **Single mesh limitation**: All 16 boards are packed into a single mesh primitive with shared material. This means the entire bed is one draw call (efficient), but individual boards cannot be selected or animated. For future interactivity (e.g., exploded view of cut list), boards would need to be separate nodes.

6. **No cap rail detected**: Some raised beds have a flat cap rail board along the top edges. The current analysis does not detect this, and the Y-layer approach would not distinguish a cap rail from a top side board. This is a minor omission — cap rails are optional in real construction.

## Performance

- Script execution: <1 second on the 1.2MB input
- Output generation is deterministic — same input always produces same output
- Memory usage: ~10MB peak (dominated by numpy array of 6,571 vertices)
