---
id: T-011-01
story: S-011
title: dist-output-path
type: task
status: open
priority: high
phase: done
depends_on: [T-010-02]
---

## Context

Combine output lands in `dist/plants/{species}.glb`. This directory is the **USB drop source** — a human carries it to the demo laptop and copies it into plantastic's `web/static/potree-viewer/assets/plants/`.

## Acceptance Criteria

- New constant `const DistPlantsDir = "dist/plants"`
- `func WritePack(species string, pack []byte) error` — writes to `DistPlantsDir/{species}.glb`, creating the directory if missing, atomic via temp-file + rename
- Existing intermediates in `outputs/` are NOT touched — combine reads them, writes pack elsewhere
- `dist/` is added to `.gitignore` if not already (verify before adding)
- A `just clean-packs` recipe removes everything in `dist/plants/` (does not touch `outputs/`)

## Out of Scope

- Subdirectory organization (all packs flat in `dist/plants/`)
- Manifest file in `dist/plants/` — the consumer generates its own from filesystem walk
- Symlinks or hardlinks from outputs to dist

## Notes

- The temp-file + rename pattern matters because a half-written `.glb` would fail validation on the dev laptop after a crashed combine. Atomicity prevents that.
