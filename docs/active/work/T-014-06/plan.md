# T-014-06 Plan: Validation Against Known-Good

## Step 1: Create inspect-intermediate.mjs

Write `scripts/inspect-intermediate.mjs` — a Node script using gltf-transform that:
- Loads a GLB file
- Walks all meshes, extracting: name, vertex count, index count, texture dimensions,
  computed triangle area, dominant normal axis
- Outputs a single JSON object to stdout
- Exits 0 on success, 1 on parse failure

**Verification:** Run against the reference billboard GLB and check output JSON is
well-formed with 7 meshes.

## Step 2: Create validate-blender-output.sh

Write `scripts/validate-blender-output.sh` implementing all 6 automated checks:
- Parse arguments: `<test_id> [--ref <ref_id>]`
- Check prerequisites (node, glb-optimizer binary, reference files)
- Check 1: Compare file sizes with 0.5x–2x tolerance
- Check 2: Run inspect-intermediate.mjs on each test intermediate, verify mesh counts and names
- Check 3: Verify all textures are 512×512 from the inspect output
- Check 4: Verify no zero-area quads and correct normal orientations
- Check 5: Run `./glb-optimizer pack <test_id>` and check exit code
- Check 6: Run `node scripts/verify-pack.mjs <pack_path>` and check exit code
- Print summary, exit 0 if all pass

**Verification:** Run against the reference asset ID itself (self-validation) — all
checks should pass since the reference is known-good.

## Step 3: Add justfile recipe

Add `validate` recipe to justfile:
```
validate id ref="1e562361be18ea9606222f8dcf81849d": build
    bash scripts/validate-blender-output.sh {{id}} --ref {{ref}}
```

**Verification:** `just validate 1e562361be18ea9606222f8dcf81849d` runs without error.

## Step 4: Test with Reference Asset

Run the validation script against the reference asset to confirm all 6 checks pass
when comparing an asset against itself. This is a smoke test — the reference should
always validate against itself.

**Verification:** Exit code 0, all 6 checks show PASS.

## Step 5: Manual Testing Note

Checks 7-8 (cross-repo plantastic load + visual spot check) are manual per the ticket.
Document in progress.md that these are deferred to human execution. The automated
script covers checks 1-6 only.

## Risk Mitigations

1. **gltf-transform API surface**: The library's Node API is well-documented. Key
   methods: `NodeIO.read()`, `Root.listMeshes()`, `Mesh.listPrimitives()`,
   `Primitive.getAttribute('POSITION')`, `Material.getBaseColorTexture()`,
   `Texture.getSize()`. If `getSize()` is unavailable, fall back to
   `Texture.getImage()` and parse PNG header for dimensions.

2. **Normal computation**: For quads (4 vertices, 2 triangles), the dominant normal
   is computed from the cross product of triangle edges. For dome geometry (6×6 grid),
   average all face normals.

3. **Pack combine may fail for test asset**: If the test asset hasn't been fully
   prepared (no originals, no settings), the pack step will fail. The script should
   detect this and report it as a prerequisite failure, not a validation failure.

4. **Missing npm dependencies**: The script should check that `scripts/node_modules`
   exists and suggest `just verify-pack-install` if not.
