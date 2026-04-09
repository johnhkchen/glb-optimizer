---
id: T-014-06
story: S-014
title: validation-against-known-good
type: task
status: open
priority: high
phase: done
depends_on: [T-014-04]
---

## Context

The Blender-produced intermediates must be visually compatible with the client-side JS output. The crossfade parameters in plantastic's SpeciesInstancer were calibrated against the client-side output. If Blender renders produce quads with different sizes, different aspect ratios, or significantly different alpha coverage, the crossfade could break.

This ticket validates the Blender pipeline against the known-good manual bake.

## Validation procedure

### Reference asset

Asset `1e562361be18ea9606222f8dcf81849d` in `~/.glb-optimizer/outputs/` — manually baked via the browser UI. Has:
- `_billboard.glb` (1.8 MB)
- `_billboard_tilted.glb` (42 KB)
- `_volumetric.glb` (704 KB)

### Test asset

Run `glb-optimizer prepare inbox/dahlia_blush.glb --category round-bush` to produce Blender-baked intermediates for a different model.

### Checks

1. **File size parity**: Blender intermediates within 0.5x–2x of the reference's sizes (different model, so exact match isn't expected, but order of magnitude should be similar)
2. **GLB structure**: `pack-inspect` on the Blender pack shows the same variant counts (N side quads, 1 top, N tilted, M dome slices) as the reference
3. **Texture dimensions**: each quad's baseColorTexture is at the expected resolution (512×512 or whatever T-014-01 documented)
4. **Quad geometry**: side quads are vertical, top quad is horizontal, dome slices are horizontal at stacked heights. No flipped normals, no zero-area quads.
5. **Pack combine**: `glb-optimizer pack <id>` succeeds on the Blender output
6. **Pack verify**: `node scripts/verify-pack.mjs <pack>` passes
7. **Cross-repo load**: copy the pack to plantastic's `__fixtures__/packs/`, run `pnpm test:unit plant-pack-real` — passes
8. **Visual spot check**: load the pack in plantastic's scene, orbit the camera, verify the crossfade transitions look smooth (no sudden pop, no blank angles, no misaligned quads)

### Automated

Write a script `scripts/validate-blender-output.sh` that runs checks 1-6 automatically. Check 7-8 are manual (require the other repo + a browser).

## Acceptance Criteria

- Validation script passes for the dahlia_blush model
- plantastic's real-pack test passes with the Blender-produced pack
- Visual spot check confirms smooth crossfade (documented with a screenshot in `progress.md`)
- Any parameter adjustments needed are fed back to T-014-01's doc and T-014-02's script

## Out of Scope

- Pixel-diff comparison between Blender and three.js output (different renderers, different tonemapping — visual parity, not binary identity)
- Performance benchmarking of Blender render time
- Testing every shape category (round-bush is sufficient for v1; others are a follow-up)
