---
id: S-012
epic: E-002
title: integration-support-tooling
type: story
status: open
priority: high
tickets: [T-012-01, T-012-02, T-012-03, T-012-04, T-012-05]
---

## Goal

Build the support tooling that makes T-011-04 (the integration handshake) cheap to execute and re-execute, and that hardens the producer's pack output workflow for demo day. The producer is currently stalled waiting on the consumer side to catch up; this story is the productive use of that waiting time.

## Context

When T-011-04 hit `phase: research`, three friction points became visible that the producer agent (or a human operator) would otherwise have to grind through manually every time a pack is regenerated:

1. **Hash-to-species mapping is manual.** Source GLBs in `~/.glb-optimizer/outputs/` are stored under content-hash filenames (`0b5820c3aaf51ee5cff6373ef9565935.glb`). T-011-02's metadata capture supports a `_meta.json` sidecar that overrides the species id, but those sidecars don't exist on disk and have to be hand-authored. The original upload filename (`achillea_millefolium.glb`) is sitting in `~/.glb-optimizer/originals/` but nothing currently links the two.

2. **No tooling to inspect a pack.** Once `glb-optimizer pack-all` produces `dist/plants/{species}.glb`, the only way to verify what it actually contains is to load it in three.js or write a one-off Go test. For the handshake's Phase 4 ("verify the pack is correct before handoff"), the agent needs a fast `glb-optimizer pack-inspect` CLI that prints the metadata block, variant counts, byte sizes, and sha256 in one shot.

3. **No way to verify a pack is consumer-compatible without standing up plantastic.** The consumer-side loader test in plantastic T-080-01 is mocked. The handshake's Phase 4 needs a real-pack validator that can run on the producer's host without the entire SvelteKit dev environment. A small node script using `@gltf-transform/core` can validate the Pack v1 schema against a `.glb` file in isolation and give a green/red answer in under a second.

Three additional polish items came out of T-010-04's review (open concerns and follow-ups):

4. **Filename loss across server restarts.** When the glb-optimizer Go server restarts, it forgets which upload filename mapped to which content hash, so re-bakes lose human-readable provenance. This is the upstream root cause of friction #1 — fix it once and the manual mapping problem mostly disappears.

5. **No cleanup of stale dist/plants files.** When pack-all runs over an outputs dir whose contents have changed, old packs for assets that are no longer present linger in `dist/plants/` and could get USB-copied to plantastic by mistake. A `just clean-stale-packs` recipe + a `--clean` flag on `pack-all` closes the gap.

## Relationship to T-011-04

This story does NOT block T-011-04. The producer agent in T-011-04 research phase can pick up tickets here as parallel work, OR it can grind through the manual workflow without this tooling. The story's purpose is to make the handshake **cheaper and more reliable** — both for the first execution (today) and for every re-execution that follows (demo morning, post-bake-tweak, post-species-add).

That said: if T-012-01 (hash-to-species resolver) lands before T-011-04 enters its `implement` phase, the producer agent will save manual work. Same for T-012-02 (pack-inspect) for Phase 1 verification and T-012-03 (standalone verifier) for Phase 4 verification. **The recommended order**: T-012-01 → T-012-02 → T-012-03 → T-012-04 → T-012-05, with the first three being the highest-leverage.

## Acceptance Criteria

- Producer agent can execute T-011-04's full handshake protocol on a fresh checkout WITHOUT manually authoring `_meta.json` sidecars (T-012-01)
- Producer agent can verify a pack file end-to-end without launching three.js or starting the web server (T-012-02 + T-012-03)
- Re-baking and re-packing produces a consistent `dist/plants/` directory with no stale entries (T-012-05)
- Server restarts preserve the human-readable filename associated with each content hash (T-012-04)
- Each ticket ships with its own tests; story-level integration test demonstrates the full chain: upload source → bake intermediates → pack-all → pack-inspect → verify-pack → handoff-ready file

## Non-Goals

- Automating the cross-repo handshake itself (T-011-04 stays human-in-the-loop, this story just makes it faster)
- Adding new bake variants or modifying the impostor scheme
- Changing the Pack v1 schema (E-002 §"Pack Format v1" is frozen)
- A full asset-management UI (the demo doesn't need it)
- Backfilling provenance for already-existing intermediates (T-012-04 only fixes the going-forward path; existing hashes still need T-012-01's resolver)

## Dependencies

- E-002 (Pack v1 spec, combine, capture, CLI all done)
- No dependency on T-011-04 — this story runs in parallel with the handshake
