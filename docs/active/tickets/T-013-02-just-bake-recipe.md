---
id: T-013-02
story: S-013
title: just-bake-recipe
type: task
status: open
priority: critical
phase: done
depends_on: [T-013-01]
---

## Context

Wrap T-013-01's Playwright script into a single `just bake <source.glb>` recipe that an operator (or agent) can call without knowing the Playwright invocation details.

## Acceptance Criteria

- New justfile recipe: `just bake path/to/source.glb`
  - Starts the Go server if not already running (check port; `go run . &` if needed)
  - Runs the Playwright headless bake script
  - Runs `just pack <id>` after intermediates are confirmed
  - Prints final pack path + size
  - Kills the Go server only if it started one (don't kill a pre-existing server)
- New justfile recipe: `just bake-status` — prints a table of all assets in `outputs/` with their intermediate completeness (has_billboard / has_billboard_tilted / has_volumetric / has_pack)
- The recipe works from a clean shell with no prior state

## Out of Scope

- Batch baking (T-013-03)
- Species-id override at bake time (use the resolver from T-012-01)
- Modifying the Go server
