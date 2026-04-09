---
id: T-012-03
story: S-012
title: standalone-pack-verifier
type: task
status: open
priority: high
phase: done
depends_on: [T-010-01]
---

## Context

The handshake's Phase 4 says "verify the pack loads in plantastic's loader test." Plantastic's `plant-pack.test.ts` is currently 100% mocked GLTFLoader (per its T-080-01 review) — there is no real-binary-pack test, and the consumer's review explicitly says "the first real-pack integration test happens at the cross-repo handshake milestone." That test still needs to be written.

The producer-side agent could write it inside plantastic's repo via cross-repo file access, but that couples two repos' test infrastructure together and introduces a dependency on plantastic's vitest + three.js + jsdom stack just to verify Pack v1 conformance.

A simpler alternative: ship a **standalone node script in glb-optimizer's repo** that uses `@gltf-transform/core` (a node-friendly glTF parser with no DOM dependency) to load a `.glb`, validate it against Pack v1, and exit 0/1. The producer agent runs this script as Phase 4 verification. Plantastic's own real-pack test can be added later as part of the consumer-side workflow without blocking the demo.

The script lives in glb-optimizer's repo because the producer is the source of truth for what Pack v1 means. Plantastic can later import the same JSON schema if it wants.

## Acceptance Criteria

### Verifier script

- New file: `scripts/verify-pack.mjs` (node ESM, no TypeScript needed for this small script)
- Usage: `node scripts/verify-pack.mjs <path-to-pack.glb>`
- Behavior:
  1. Parses the `.glb` using `@gltf-transform/core`
  2. Reads `scene.extras.plantastic`
  3. Validates the metadata against the v1 schema (same rules as `PackMeta.Validate()`)
  4. Walks the scene graph and checks:
     - `view_side` group exists with ≥1 child mesh
     - `view_top` mesh exists
     - If `view_tilted` exists, it's a group with ≥1 child mesh
     - If `view_dome` exists, it's a group with ≥1 child mesh
     - All meshes referenced have valid material + texture references
  5. Prints a one-line PASS or a multi-line FAIL with reasons
  6. Exits 0 on PASS, 1 on FAIL
- Tolerates missing optional variants (`view_tilted`, `view_dome`) — they're not required by Pack v1
- Handles malformed `.glb` files cleanly (truncated, wrong magic, invalid JSON chunk) — never panics, always exits 1 with a reason

### Schema source-of-truth

- The validation rules in `verify-pack.mjs` MUST match the rules in Go's `PackMeta.Validate()` exactly
- Document any divergence as a deliberate skip (e.g., "node script doesn't check fade ordering — that's the Go side's job")
- Add a comment in `verify-pack.mjs` pointing at `pack_meta.go` as the canonical source

### justfile recipe

- New recipe: `just verify-pack <species_or_path>` — wraps the node script with the same arg resolution as `pack-inspect` (species id → `~/.glb-optimizer/dist/plants/{id}.glb`)
- Script can also be invoked directly via `node scripts/verify-pack.mjs <path>`

### Tests

- Test fixture: a known-valid synthetic pack file in `scripts/fixtures/valid-pack.glb` (small, committed)
- Test fixture: a known-broken pack with missing `view_top` in `scripts/fixtures/broken-pack-no-top.glb`
- Shell test: `node scripts/verify-pack.mjs scripts/fixtures/valid-pack.glb` exits 0
- Shell test: `node scripts/verify-pack.mjs scripts/fixtures/broken-pack-no-top.glb` exits 1 and mentions `view_top`
- Tests run in the existing test pipeline (`just check` or equivalent)

### Dependencies

- Add `@gltf-transform/core` to a new minimal `package.json` in `scripts/` (or in repo root if there isn't one yet) — keep it small, don't pull in @gltf-transform/extensions unless needed
- Document the install step in the script's header comment

## Out of Scope

- Validating the actual *content* of variants (e.g., are the textures sensible? does the geometry look like a plant?) — schema validation only
- Running validation across all packs in `dist/plants/` (the operator can shell-loop)
- Wiring this into CI (manual invocation is fine for v1)
- Sharing the schema between Go and node via code generation (deliberate duplication is fine; the schema is small and stable)

## Notes

- This script is the producer's "did I make a pack the consumer can actually load?" answer. Run it BEFORE physically copying any pack to plantastic's asset directory.
- The schema-duplication tradeoff is deliberate. A code-generated schema would be cleaner but adds toolchain weight; manual sync is fine for a 1-page schema spec that's already frozen.
- Keep the script short (target ≤200 LOC) and readable — an operator should be able to read it during the demo morning if something goes weird.
