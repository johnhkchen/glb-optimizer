# Design — T-001-01: Raised Bed Parametric Reconstruction

## Problem Summary

Reconstruct a 1.2MB TRELLIS-generated raised bed mesh (10,402 triangles, single fused mesh) as parametric box primitives with wood texture. Target: <50KB, <300 triangles, plus a cut list.

## Options Evaluated

### Option A: Geometric Decomposition (Plane Clustering + OBB Fitting)

Cluster triangles by normal direction → group coplanar faces → identify rectangular regions → fit oriented bounding boxes.

**Pros**: General-purpose, works on any boxy mesh, no domain assumptions.
**Cons**: Complex (RANSAC/region-growing needed), fragile on TRELLIS meshes (noisy normals, degenerate tris, 2748 isolated verts), requires scipy/sklearn or equivalent. Over-engineered for a single known object type.

**Verdict**: Rejected. Too complex for a single raised bed, and the fused mesh topology makes clean separation unreliable.

### Option B: Template-Based Parametric Fitting

Define a "raised bed" template: 4 corner posts + N side boards per face. Extract parameters (overall width, length, height, board count, board thickness) from the mesh bounding box and axis-aligned surface analysis.

**Pros**: Simple, reliable, produces clean standard-lumber output. Domain knowledge makes this robust.
**Cons**: Only works for raised beds (not general). Requires manual template definition.

**Verdict**: Rejected as standalone. Too rigid — doesn't adapt to actual board positions in the mesh.

### Option C: Hybrid — Axis-Aligned Slab Detection with Template Guidance (CHOSEN)

1. **Slice the mesh** along each axis to find material boundaries (where geometry exists vs. gaps)
2. **Identify boards** by finding rectangular regions in the occupancy grid
3. **Snap to standard lumber** using domain knowledge as constraints
4. **Generate box primitives** at detected positions

Specifically:
- Sample the mesh along the Y-axis (height) to find horizontal board layers
- For each layer, determine which sides have boards (±X faces, ±Z faces)
- Detect corner posts from vertical geometry extending the full height
- Fit standard lumber dimensions to the detected slabs

**Pros**: Uses actual geometry data but guided by construction knowledge. Handles variation in TRELLIS output. Straightforward implementation.
**Cons**: Still somewhat specific to box-like construction. Fine for this ticket's scope.

## Chosen Approach: Details

### Step 1 — Mesh Analysis Script (Python)

A standalone Python script (`scripts/analyze_bed.py`) that:
1. Parses the GLB binary (no dependency on gltfpack/Blender)
2. Extracts vertex positions and triangle indices
3. Computes face normals
4. Performs axis-aligned occupancy analysis:
   - Ray-cast or voxelize along Y to find board layers
   - Identify extent of each board (X/Z ranges per Y-layer)
5. Outputs a JSON board manifest: list of `{position, dimensions, orientation, lumber_type}`

### Step 2 — Parametric GLB Generator (Python)

A script (`scripts/build_parametric_bed.py`) that:
1. Reads the board manifest JSON
2. Generates box geometry (8 verts, 12 tris per box)
3. Assigns UVs that tile a small wood texture across each face
4. Packs a small wood grain texture (JPEG, ~10-20KB target)
5. Writes a valid glTF 2.0 binary (GLB) file

### Step 3 — Integration

A single entry point (`scripts/parametric_reconstruct.py`) that:
1. Runs analysis on input GLB
2. Generates the parametric GLB
3. Prints the cut list to stdout
4. Can be invoked standalone or integrated into the Go server later

### Texture Strategy

The original 1MB PNG is 80% of file size. Options:
- **Downsample original**: Resize to 128×128 or 256×256 JPEG → ~5-15KB. Loses detail but preserves original look.
- **Tiled procedural**: Use a generic wood grain texture, tile it per-face. Clean look, tiny file.
- **Extract from original**: Crop a representative wood patch from the original texture, use as tile.

**Decision**: Extract a representative wood patch from the original PNG, resize to 128×128, save as JPEG (~5KB). This preserves the original wood color/grain while meeting size targets. UV mapping will tile this across board faces with grain running along the board length.

### Lumber Size Mapping

Model dimensions: 1.001 × 0.219 × 0.648 units.

Assuming the model represents a real raised bed approximately 4ft × 2ft × 10in:
- Scale factor: ~1.0 unit ≈ 48 inches (4 feet)
- Board thickness (Y dimension between layers): ~0.035 units ≈ 1.68" → standard 2× lumber (actual 1.5")
- Board width (visible face height per board): ~0.11 units ≈ 5.28" → 2×6 (actual 5.5") or 2×8 (actual 7.25")
- Post cross-section: square at corners → 4×4 (actual 3.5" × 3.5")

These will be snapped to nearest standard sizes in the cut list.

### File Size Budget

Target: <50KB total GLB.
- JSON chunk: ~2KB (scene graph, materials, accessors)
- Geometry: 16 boards × (8 verts × 12 bytes + 36 indices × 2 bytes) = ~2.8KB
- Texture: 128×128 JPEG ≈ 5-10KB
- Padding/overhead: ~2KB
- **Estimated total: ~12-17KB** — well under 50KB budget

### Triangle Budget

Target: <300 triangles.
- 16 boards × 12 triangles = 192 triangles
- Even 25 boards = 300 triangles exactly
- Comfortable within budget

## Risks

1. **Board detection accuracy**: If the TRELLIS mesh has significant noise or curved surfaces, slab detection may misidentify boundaries. Mitigation: visual validation, manual parameter override.

2. **Scale inference**: Without metadata about real-world scale, lumber size mapping is approximate. Mitigation: output both model-unit and inferred real-world dimensions.

3. **Texture extraction**: The original PNG may not have a clean tileable region. Mitigation: fallback to a solid wood color or a bundled generic wood texture.

## Non-Goals

- General-purpose mesh decomposition for arbitrary objects
- Integration into the Go web server (future ticket scope)
- Animation or rigging support
- PBR material channels beyond base color
