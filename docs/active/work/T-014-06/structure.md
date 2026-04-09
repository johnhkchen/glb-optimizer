# T-014-06 Structure: Validation Against Known-Good

## New Files

### scripts/validate-blender-output.sh
Bash orchestration script. Runs checks 1-6 from the ticket.

```
Usage: scripts/validate-blender-output.sh <test_asset_id> [--ref <reference_id>]

Default reference: 1e562361be18ea9606222f8dcf81849d
```

Flow:
1. Resolve paths: outputs dir, test intermediates, reference intermediates
2. Check prerequisites: Node installed, glb-optimizer binary built, reference files exist
3. Run check 1: File size parity (bash arithmetic)
4. Run check 2: GLB structure via inspect-intermediate.mjs on test billboard/tilted/volumetric
5. Run check 3: Texture dimensions (from inspect-intermediate.mjs output)
6. Run check 4: Quad geometry (from inspect-intermediate.mjs output)
7. Run check 5: Pack combine via `./glb-optimizer pack <id>`
8. Run check 6: Pack verify via `node scripts/verify-pack.mjs <pack>`
9. Print summary, exit 0 or 1

### scripts/inspect-intermediate.mjs
Node helper that loads a GLB intermediate and emits a JSON report.

```
Usage: node scripts/inspect-intermediate.mjs <path.glb>
```

Output JSON:
```json
{
  "path": "/path/to/file.glb",
  "size_bytes": 1852800,
  "mesh_count": 7,
  "meshes": [
    {
      "name": "billboard_0",
      "vertex_count": 4,
      "index_count": 6,
      "has_texture": true,
      "texture_width": 512,
      "texture_height": 512,
      "area": 1.234,
      "dominant_normal_axis": "z",
      "min_y": 0.0
    }
  ]
}
```

Fields per mesh:
- `name`: mesh name from GLB
- `vertex_count`: number of vertices (expect 4 for a quad, more for dome geometry)
- `index_count`: number of indices (expect 6 for a quad = 2 triangles)
- `has_texture`: whether a baseColorTexture exists
- `texture_width`, `texture_height`: texture dimensions (0 if no texture)
- `area`: total triangle area computed from vertex positions
- `dominant_normal_axis`: "x", "y", or "z" — which axis has the largest average normal component
- `min_y`: minimum Y coordinate of vertices (for ground alignment check)

## Modified Files

### justfile
Add recipe:
```
# Validate Blender-produced intermediates against a known-good reference.
validate id ref="1e562361be18ea9606222f8dcf81849d": build
    bash scripts/validate-blender-output.sh {{id}} --ref {{ref}}
```

## File Dependencies

```
scripts/validate-blender-output.sh
  ├── reads: ~/.glb-optimizer/outputs/{ref}_billboard.glb (reference)
  ├── reads: ~/.glb-optimizer/outputs/{ref}_billboard_tilted.glb
  ├── reads: ~/.glb-optimizer/outputs/{ref}_volumetric.glb
  ├── reads: ~/.glb-optimizer/outputs/{id}_billboard.glb (test)
  ├── reads: ~/.glb-optimizer/outputs/{id}_billboard_tilted.glb
  ├── reads: ~/.glb-optimizer/outputs/{id}_volumetric.glb
  ├── calls: node scripts/inspect-intermediate.mjs (3x for test intermediates)
  ├── calls: ./glb-optimizer pack <id>
  └── calls: node scripts/verify-pack.mjs <pack_path>
scripts/inspect-intermediate.mjs
  └── imports: @gltf-transform/core (already in scripts/package.json)
```

## Validation Check Matrix

| # | Check | Method | Pass Criteria |
|---|-------|--------|---------------|
| 1 | File size parity | bash stat + arithmetic | Each intermediate within 0.5x–2x of reference |
| 2 | GLB structure | inspect-intermediate.mjs mesh_count + names | Billboard: 7 meshes (billboard_0-5 + billboard_top). Tilted: 6 meshes (billboard_0-5). Volumetric: ≥1 mesh named vol_layer_* |
| 3 | Texture dimensions | inspect-intermediate.mjs texture_width/height | All textures 512×512 |
| 4 | Quad geometry | inspect-intermediate.mjs area + dominant_normal | Side quads: dominant normal z, area > 0. Top: dominant normal y. Dome: dominant normal y. No zero-area. |
| 5 | Pack combine | glb-optimizer pack exit code | Exit 0 |
| 6 | Pack verify | verify-pack.mjs exit code | Exit 0, prints "PASS" |

## Output Format

```
=== Blender Output Validation ===
Test asset:      <id>
Reference asset: <ref_id>

[1/6] File size parity
  billboard:  1,234,567 bytes (ref: 1,852,800) ... PASS
  tilted:     38,000 bytes (ref: 42,504) ... PASS
  volumetric: 600,000 bytes (ref: 704,212) ... PASS

[2/6] GLB structure
  billboard:  7 meshes (billboard_0..5 + billboard_top) ... PASS
  tilted:     6 meshes (billboard_0..5) ... PASS
  volumetric: 5 meshes (vol_layer_*) ... PASS

[3/6] Texture dimensions
  billboard:  all 512x512 ... PASS
  tilted:     all 512x512 ... PASS
  volumetric: all 512x512 ... PASS

[4/6] Quad geometry
  billboard:  no zero-area, normals correct ... PASS
  tilted:     no zero-area, normals correct ... PASS
  volumetric: no zero-area, normals correct ... PASS

[5/6] Pack combine
  glb-optimizer pack <id> ... PASS

[6/6] Pack verify
  verify-pack.mjs ... PASS

Result: 6/6 checks passed
```
