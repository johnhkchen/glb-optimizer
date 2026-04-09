# T-014-06 Design: Validation Against Known-Good

## Decision 1: Script Language

**Options:**
- (A) Pure bash + jq — can check file sizes but can't parse GLB internals
- (B) Bash wrapper calling Node helper — bash for orchestration, Node for GLB parsing
- (C) Pure Node script — single language, full GLB access

**Decision: (B) Bash wrapper + Node helper**

Why: The ticket explicitly specifies `scripts/validate-blender-output.sh` (a shell script).
Checks 1 (file size) and 5-6 (pack/verify) are naturally shell commands. Checks 2-4
(GLB structure, texture dimensions, quad geometry) require GLB parsing — delegate those
to a small Node script that outputs JSON. This keeps the orchestration readable while
leveraging gltf-transform for heavy lifting.

## Decision 2: Node Helper Scope

**Options:**
- (A) One monolithic Node script that does everything
- (B) One Node script per check (inspect-intermediate.mjs, check-textures.mjs, etc.)
- (C) One Node script that inspects an intermediate GLB and returns structured JSON

**Decision: (C) Single inspect-intermediate.mjs**

Why: All three GLB-level checks (structure, textures, geometry) require loading and
walking the same GLB. Loading it once and emitting a single JSON report with all
findings is more efficient and simpler than three separate scripts. The bash wrapper
then validates the JSON fields.

Output schema:
```json
{
  "path": "...",
  "meshes": [
    {
      "name": "billboard_0",
      "vertexCount": 4,
      "textureWidth": 512,
      "textureHeight": 512,
      "normalY": 0.0,
      "area": 1.234
    }
  ],
  "meshCount": 7
}
```

## Decision 3: Reference Sizes — Hardcoded vs Measured

**Options:**
- (A) Hardcode reference sizes in the script
- (B) Read reference sizes at runtime from the reference asset directory
- (C) Accept reference ID as a flag and measure

**Decision: (B) Read at runtime**

Why: The reference asset is at a known path (`~/.glb-optimizer/outputs/{REF_ID}_*`).
Reading sizes at runtime means the script doesn't break if the reference is re-baked
with slightly different parameters. The default reference ID is hardcoded but can be
overridden with a flag.

## Decision 4: Geometry Validation Depth

**Options:**
- (A) Just check mesh count and names (fast, surface-level)
- (B) Also check vertex count per quad (should be 4 for PlaneGeometry)
- (C) Full check: vertex count, normal direction, area > 0, texture dimensions

**Decision: (C) Full check**

Why: The ticket explicitly calls out "No flipped normals, no zero-area quads" and
"each quad's baseColorTexture is at the expected resolution." Skipping these would
leave the acceptance criteria unmet.

Geometry checks:
- Side quads (`billboard_0`..`billboard_5`): normal Z should be non-zero (vertical plane, camera-facing)
- Top quad (`billboard_top`): dominant normal should point along +Y (horizontal, rotated -PI/2)
- Dome slices (`vol_layer_*`): dominant normal should point approximately +Y (mostly horizontal)
- All quads: area > 0 (no degenerate geometry)
- All textures: 512×512 for standard resolution

## Decision 5: Pack Combine Check

**Options:**
- (A) Run `glb-optimizer pack <test_id>` from the script
- (B) Assume prepare already packed and just check the pack exists
- (C) Always re-pack to ensure a clean combine from the Blender intermediates

**Decision: (A) Run pack from the script**

Why: The validation script should be self-contained. Running `glb-optimizer pack` on
the test ID confirms that the Blender-produced intermediates are compatible with the
pack pipeline. The `prepare` command already packs, but an independent re-pack after
any potential intermediate modifications is more trustworthy.

## Decision 6: verify-pack.mjs Integration

**Decision:** Run `node scripts/verify-pack.mjs <pack_path>` and check exit code.

The verifier already validates Pack v1 schema, scene graph structure, and mesh references.
No need to duplicate its checks. Just invoke it and report pass/fail.

## Decision 7: Exit Code Semantics

**Decision:**
- Exit 0: All 6 automated checks pass
- Exit 1: One or more checks failed (details printed to stderr)
- Exit 2: Usage error (missing arguments, missing dependencies)

Each check prints a `[PASS]` or `[FAIL]` line to stdout. A final summary counts
passes and failures. This matches the verify-pack.mjs pattern of printing structured
results.

## Rejected Approaches

1. **Python script**: Would need a GLB parsing library not currently in the project.
   Node + gltf-transform is already available.

2. **Go subcommand**: Would require modifying the Go binary for a one-off validation
   tool. The shell script + Node helper approach keeps validation decoupled from the
   core binary.

3. **Pixel-diff comparison**: Explicitly out of scope per ticket. Different renderers
   produce different results — structural + dimensional parity is the goal.
