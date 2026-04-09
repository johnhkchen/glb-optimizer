# T-014-06 Progress: Validation Against Known-Good

## Completed Steps

### Step 1: inspect-intermediate.mjs ✓
Created `scripts/inspect-intermediate.mjs` — Node script using @gltf-transform/core.
- Loads any GLB intermediate and reports: mesh count, mesh names, vertex/index counts,
  texture dimensions, triangle areas, dominant normal axes
- Tested against all three reference intermediates:
  - Billboard: 7 meshes, all 512×512, side quads have dominant Z normal, top has Y
  - Tilted: 6 meshes, all 512×512, 42 KB
  - Volumetric: 5 meshes (49 vertices each = 7×7 dome grid), all 512×512

### Step 2: validate-blender-output.sh ✓
Created `scripts/validate-blender-output.sh` implementing checks 1-6:
- Argument parsing with --ref flag for reference override
- Prerequisite checks (node, node_modules, reference files, test files)
- Check 1: File size parity (0.5x–2x tolerance) — bash arithmetic
- Check 2: GLB structure via inspect-intermediate.mjs (mesh counts)
- Check 3: Texture dimensions from inspect output (all 512×512)
- Check 4: Quad geometry — zero-area check, normal orientation
- Check 5: Pack combine via `./glb-optimizer pack`
- Check 6: Pack verify via verify-pack.mjs
- Summary with pass/fail counts, exit 0 or 1

### Step 3: justfile recipe ✓
Added `just validate <id> [ref]` recipe.

### Step 4: Smoke Test ✓
Ran reference asset against itself. Results: 13/14 checks pass.

The one failure is check 6 (pack verify): `verify-pack.mjs` requires `view_top` as a
required group, but the browser-baked reference has unnamed meshes, so combine routes
all 7 billboard meshes into `view_side` and `view_top` is absent. This is a
pre-existing condition of browser-baked output (no mesh names → can't distinguish
top from side).

This is expected: the Blender pipeline produces named meshes (`billboard_0`..`billboard_5` +
`billboard_top`), so combine will correctly route them into `view_side` + `view_top`,
and check 6 should pass when run against Blender-produced output.

The pack-inspect tool itself reports `validation: OK` for the reference pack because
it uses a looser validation (view_top is optional in pack_inspect.go). The verifier
uses a stricter schema (view_top required). This discrepancy is documented as an open
concern in review.md.

### Step 5: Manual Testing Note
Checks 7-8 are manual per the ticket:
- Check 7: Copy pack to plantastic's `__fixtures__/packs/`, run `pnpm test:unit plant-pack-real`
- Check 8: Load pack in plantastic's scene, orbit camera, verify smooth crossfade

These require the plantastic repo + browser and are deferred to human execution.

## Deviations from Plan

1. **Tilted quad normal check relaxed**: The plan called for checking dominant normal
   axis on tilted quads. At 30° elevation, the dominant axis depends on the model's
   aspect ratio (could be Z or Y). Changed to only check for zero-area quads instead
   of enforcing a specific normal axis.

2. **Pack file detection**: Initially scanned all of dist/plants/ for pack files.
   Refined to extract the species name from `glb-optimizer pack` output and look up
   the specific file.
