# T-014-06 Review: Validation Against Known-Good

## Summary

Implemented the automated validation script (`scripts/validate-blender-output.sh`)
and its Node helper (`scripts/inspect-intermediate.mjs`) for validating Blender-produced
GLB intermediates against a known-good browser-baked reference. Added a `just validate`
recipe for easy invocation.

## Files Created

| File | Lines | Purpose |
|------|-------|---------|
| `scripts/inspect-intermediate.mjs` | ~160 | Node helper: loads GLB, reports mesh structure, textures, geometry |
| `scripts/validate-blender-output.sh` | ~250 | Bash orchestrator: runs 6 automated checks, prints pass/fail summary |

## Files Modified

| File | Change |
|------|--------|
| `justfile` | Added `validate` recipe with id + ref arguments |

## Work Artifacts

| File | Purpose |
|------|---------|
| `docs/active/work/T-014-06/research.md` | Codebase survey: reference asset, tooling, GLB structure |
| `docs/active/work/T-014-06/design.md` | 7 decisions: bash+node, single inspect script, runtime refs |
| `docs/active/work/T-014-06/structure.md` | File layout, check matrix, output format |
| `docs/active/work/T-014-06/plan.md` | 5 implementation steps with verification criteria |
| `docs/active/work/T-014-06/progress.md` | Implementation log with deviations |

## Test Coverage

### Automated Checks (1-6)
Smoke-tested against the reference asset (self-validation):
- Checks 1-5: All PASS (file sizes, GLB structure, textures, geometry, pack combine)
- Check 6: FAIL — expected for browser-baked reference (unnamed meshes → no view_top in pack)
- Check 6 should PASS for Blender-produced output (named meshes)

### Manual Checks (7-8)
Deferred to human execution per ticket scope:
- Check 7: plantastic cross-repo pack load test
- Check 8: Visual crossfade spot check with screenshot

### inspect-intermediate.mjs Verification
Tested against all three reference intermediates:
- Billboard: 7 meshes, 512×512 textures, correct normal orientations
- Tilted: 6 meshes, 512×512 textures, non-zero areas
- Volumetric: 5 meshes (49 vertices each), 512×512 textures, Y-dominant normals

## Open Concerns

1. **verify-pack.mjs vs pack-inspect.go discrepancy**: verify-pack.mjs requires
   `view_top` as a mandatory group, but pack-inspect.go treats it as optional and
   reports `validation: OK` when absent. One of them should be updated for consistency.
   Likely verify-pack.mjs should relax the requirement to match pack-inspect.go, since
   view_top is genuinely absent for models that don't produce a top-down billboard
   (e.g. hard-surface category).

2. **Browser-baked reference has unnamed meshes**: The reference asset
   `1e562361be18ea9606222f8dcf81849d` was baked via the browser UI, which produces
   unnamed meshes. This means combine puts all 7 billboard meshes under `view_side`
   and `view_top` is absent. If validation against this reference is needed after the
   Blender pipeline is live, a re-bake with named meshes should be performed.

3. **Tilted normal check is lenient**: Side quads and dome slices have strict normal
   axis checks (Z for vertical, Y for horizontal). Tilted quads only check for
   zero-area because at 30° elevation the dominant axis depends on model aspect ratio.
   A more precise check could compute the expected dominant axis from the elevation
   angle, but this was deferred as unnecessary complexity for v1.

4. **No test for the validation script itself**: The script is tested by running it
   against the reference asset, but there are no synthetic fixtures or unit tests for
   inspect-intermediate.mjs. Could add a test-validate.sh similar to test-verify-pack.sh.

5. **gltf-transform warning**: The inspect script logs `Missing optional extension,
   "KHR_materials_unlit"` to stderr when reading browser-baked GLBs. This is cosmetic
   (the extension is valid but not registered in gltf-transform) and does not affect
   results. The warning is suppressed in the bash script via `2>/dev/null`.

6. **dahlia_blush.glb not yet tested**: The ticket's acceptance criteria require
   running the validation on the dahlia_blush model specifically. This requires
   running `glb-optimizer prepare inbox/dahlia_blush.glb --category round-bush` first,
   which needs Blender available. The validation script is ready; the end-to-end test
   awaits Blender pipeline execution.
