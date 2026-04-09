---
id: T-010-04
story: S-010
title: justfile-pack-all
type: task
status: open
priority: medium
phase: done
depends_on: [T-010-03]
---

## Context

Demo prep: pack every asset in one command rather than clicking through the UI per asset. This is the recipe a human runs the morning of the demo to refresh `dist/plants/` before USB-copying it.

## Acceptance Criteria

- New justfile recipe: `just pack-all`
  - Walks `outputs/` for asset ids that have `{id}_billboard.glb` (the only required intermediate)
  - For each, invokes the combine step (via a small Go subcommand `glb-optimizer pack <id>` or by calling the HTTP endpoint against a locally running server — pick whichever is simpler)
  - Writes results to `dist/plants/{species}.glb`
  - Prints a summary table at the end: species, size, has_tilted, has_dome, status (ok / failed / oversize)
  - Exit code is non-zero if any pack failed or exceeded the cap
- The Go subcommand (if used) is `cmd/pack/main.go` or similar — does not require the HTTP server to be running
- A `just pack <id>` recipe that packs a single asset by id

## Out of Scope

- Watch mode
- Parallel packing (sequential is fine for the demo's volume)
- Cleaning stale packs in `dist/plants/` that no longer have intermediates (separate concern)

## Notes

- The Go subcommand approach is preferred over HTTP-against-self for the recipe — fewer moving parts in the justfile and easier to debug from a clean shell.
