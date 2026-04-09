# T-014-06 Research: Validation Against Known-Good

## Reference Asset

Asset `1e562361be18ea9606222f8dcf81849d` in `~/.glb-optimizer/outputs/`:

| File | Size | Purpose |
|------|------|---------|
| `_billboard.glb` | 1,852,800 bytes (1.8 MB) | 6 side quads + 1 top quad |
| `_billboard_tilted.glb` | 42,504 bytes (42 KB) | 6 tilted quads (no top) |
| `_volumetric.glb` | 704,212 bytes (704 KB) | Dome slices (visual-density, 5 layers) |
| `_bake.json` | 81 bytes | Bake stamp: `2026-04-08T21:17:16Z` |

Settings: `round-bush` category, `y` slice axis, `visual-density` distribution,
5 volumetric layers, 512px resolution, dome_height_factor=0.7, default lighting.

## Test Asset

`inbox/dahlia_blush.glb` — will be processed via `glb-optimizer prepare inbox/dahlia_blush.glb --category round-bush`.

The `prepare` subcommand (prepare_cmd.go) runs the full pipeline:
1. Copy + hash → originals/
2. Register in store + manifest
3. Optimize via gltfpack
4. Classify (or use --category override)
5. Generate LODs
6. Render via Blender (render_production.py)
7. Pack via RunPack()
8. Verify via InspectPack()

Prepare already includes internal verification — if any step fails, it exits 1
with structured error output. The validation script is an _additional_ external
check that compares the Blender output against the reference.

## Existing Tooling

### pack-inspect (pack_inspect.go)
- Works on **packs** (combined GLBs), not raw intermediates
- `--json` emits structured report with variant counts, metadata, size, sha256
- `--quiet` emits one-line `<sha256> <size> <OK|FAIL>`
- Intermediates fail inspection (no scene.extras.plantastic)

### verify-pack.mjs (scripts/)
- Node.js-based Pack v1 verifier using @gltf-transform/core
- Validates metadata schema, scene graph structure, mesh references
- Requires: `cd scripts && npm install` (gltf-transform already in package.json)

### GLB Intermediate Structure
Intermediates are raw GLBs exported by the bake pipeline. They are NOT packs:
- No `scene.extras.plantastic` metadata block
- Billboard GLB: 7 meshes (`billboard_0`..`billboard_5` + `billboard_top`)
- Tilted GLB: 6 meshes (`billboard_0`..`billboard_5`, no top)
- Volumetric GLB: N meshes named `vol_layer_{i}_h{mm}`
- All meshes use textured quads with transparent alpha

### Parsing Intermediates
Two options for parsing intermediate GLBs in a shell script:
1. **Node + gltf-transform**: Already available in scripts/node_modules. Can read
   any GLB, inspect scene graph, extract texture dimensions.
2. **Go binary**: pack_inspect.go has readGLB() but only exposes pack-level checks.
   Would need a new subcommand or direct GLB parsing in bash (impractical).

Node is the clear choice — gltf-transform is battle-tested and already a dependency.

## Relevant Patterns

### justfile recipes
- `just verify-pack <arg>` — wraps verify-pack.mjs, resolves species IDs
- `just pack <id>` — packs a single asset
- `just bake-install` / `just verify-pack-install` — install Node deps
- No existing `validate` recipe

### File Size Ranges (Ticket Check 1)
Reference sizes are the baseline. Test model is different (dahlia_blush vs
reference), so sizes won't match exactly. Ticket says 0.5x–2x range is acceptable.

| Intermediate | Reference Size | Lower (0.5x) | Upper (2x) |
|-------------|---------------|---------------|-------------|
| billboard | 1,852,800 | 926,400 | 3,705,600 |
| tilted | 42,504 | 21,252 | 85,008 |
| volumetric | 704,212 | 352,106 | 1,408,424 |

### Texture Dimensions (Ticket Check 3)
Per production-render-params.md: billboards render at 512×512. Volumetric slices
also at 512×512 (LOD0). All textures should be PNG-encoded inside the GLB.

### Quad Geometry (Ticket Check 4)
- Side quads: vertical PlaneGeometry with bottom-edge pivot (translate y+halfH)
- Top quad: rotated -PI/2 on X axis (lies flat on XZ plane)
- Dome slices: rotated -PI/2 with parabolic Y displacement

## Constraints

- Checks 1-6 must be automated in `scripts/validate-blender-output.sh`
- Checks 7-8 are manual (require plantastic repo + browser)
- The script should accept an asset ID as argument
- gltf-transform is the only GLB parsing library available in Node
- The validation should work for `round-bush` category only in v1
- Script must exit 0 on all checks passing, non-zero on any failure

## Open Questions

1. Should the script also run `prepare` or assume it was already run?
   → Ticket says "Run `glb-optimizer prepare ...` to produce Blender-baked
   intermediates" as a separate step. Script validates post-prepare output.

2. How to check texture dimensions from a GLB?
   → gltf-transform can extract image metadata (width, height) from textures.

3. How to verify quad geometry (normals, zero-area)?
   → gltf-transform provides accessor data for positions and normals.
   Can compute area from vertex positions and check normal direction.
