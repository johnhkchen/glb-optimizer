# T-014-01 Plan: Implementation Steps

## Step 1: Write Common Renderer Settings (Section 1)

Document the shared WebGLRenderer configuration used by all render functions:
- Output color space, tone mapping, tone mapping exposure
- Clear color (transparent alpha)
- Antialias, alpha, preserveDrawingBuffer flags

**Verification**: Each parameter has a line reference. Values match `renderBillboardAngle` L1400-1407.

## Step 2: Write Lighting Pipeline (Section 2)

Document the three-tier lighting resolution:
1. `createBakeEnvironment()` — PMREM env map with priority chain
2. `setupBakeLights()` — 4-light rig with configurable intensities
3. `cloneModelForBake()` — material cloning and envMapIntensity override

Include all 5 intensity defaults and the gradient env texture construction.

**Verification**: Cross-check against `makeDefaults()` L155-178 for default values.

## Step 3: Write Side Billboards Section (Section 3)

Document `renderMultiAngleBillboardGLB` and `renderBillboardAngle`:
- Camera: ortho sizing formula with halfW/halfH, distance, position orbit
- Resolution, angle count
- Quad geometry: PlaneGeometry with bottom-edge pivot
- Material settings, naming convention
- Include the billboard_top sub-section (from renderBillboardTopDown)

**Verification**: Confirm BILLBOARD_ANGLES=6, resolution=512 hardcoded at L1808.

## Step 4: Write Top-Down Billboard Section (Section 4)

Document `renderBillboardTopDown` as called from within renderMultiAngleBillboardGLB:
- Camera: ortho, straight down, sized by footprint
- Quad: flat on XZ plane, name=billboard_top
- Resolution inherited from parent call

**Verification**: Confirm ortho sizing uses max(halfW, halfD) at L1774-1776.

## Step 5: Write Tilted Billboards Section (Section 5)

Document `renderTiltedBillboardGLB` differences from side:
- 3 constants: TILTED_BILLBOARD_ANGLES, _ELEVATION_RAD, _RESOLUTION
- Camera elevation formula in renderBillboardAngle
- No billboard_top quad
- Separate upload endpoint

**Verification**: Confirm elevation=PI/6 (30deg), same renderBillboardAngle function.

## Step 6: Write Volumetric Dome Slices Section (Section 6)

The largest section. Document:
1. Slice axis rotation (`resolveSliceAxisRotation`) — 3 modes
2. Adaptive layer count (`pickAdaptiveLayerCount`) — aspect ratio thresholds
3. Boundary computation — 3 algorithms with pseudocode
4. Per-slice camera (`renderLayerTopDown`) — clipping plane, ortho sizing
5. Dome geometry (`createDomeGeometry`) — parabolic Y bulge formula
6. Naming convention: `vol_layer_{i}_h{baseMm}`
7. Ground alignment

**Verification**: Cross-check STRATEGY_TABLE for per-category overrides of layers,
slice_axis, distribution_mode.

## Step 7: Write STRATEGY_TABLE Reference (Section 7)

Reproduce the full table with all 6 categories and 4 fields.

**Verification**: Values match L428-434 exactly.

## Step 8: Write Volumetric LOD Chain (Section 8)

Document the 4-level LOD config table from VOLUMETRIC_LOD_CONFIGS.

**Verification**: Values match L2373-2378.

## Step 9: Write Validation Plan (Section 9)

Define comparison criteria for T-014-06:
- Structural: mesh names, quad counts, UV layouts
- Dimensional: texture resolution, quad world-space sizes
- Visual: texture diff between JS and Blender renders
- Size: expected file size ranges for known-good asset

**Verification**: Criteria are concrete and testable.

## Step 10: Assemble and Commit

Combine all sections into `docs/knowledge/production-render-params.md`.
Single atomic commit.

**Verification**: Document covers all 4 render types per acceptance criteria.
Every parameter has a line reference. Validation plan section exists.

## Testing Strategy

This ticket produces documentation, not code. Testing means:
1. **Completeness check**: Every function listed in the ticket deliverable section is covered
2. **Accuracy check**: Line references are correct (verified during writing)
3. **No code changes**: `go test` and existing tests unaffected
