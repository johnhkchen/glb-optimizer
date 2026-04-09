---
id: T-013-03
story: S-013
title: batch-bake
type: task
status: open
priority: high
phase: done
depends_on: [T-013-02]
---

## Context

Bake multiple source GLBs in sequence. The operator drops N `.glb` files into an `inbox/` directory; the script processes each one and writes packs to `dist/plants/`.

## Acceptance Criteria

- New justfile recipe: `just bake-all [inbox-dir]`
  - Default inbox: `inbox/` at the repo root (created if missing; already contains `dahlia_blush.glb` as a reference model)
  - For each `.glb` in the inbox:
    1. Upload to the Go server
    2. Run the headless bake (T-013-01)
    3. Run pack (T-013-02)
  - Prints a summary table at the end: filename → species → pack size → status
  - Exit non-zero if any asset failed; continue processing the rest
  - Moves successfully-baked source files to `inbox/done/` so re-running skips them
- The recipe reuses a single Go server + browser instance across all assets (don't restart between each)
- Total time per asset: ~2-3 minutes on the Mac mini (billboard + tilted + volumetric render + pack combine)

## Out of Scope

- Parallel baking (sequential is fine — the browser can only render one asset at a time)
- Automatic species-id detection from filename (uses the existing resolver chain from T-012-01)
- Pushing packs to plantastic (manual USB/copy step)
