---
id: T-010-03
story: S-010
title: pack-button-and-endpoint
type: task
status: open
priority: high
phase: done
depends_on: [T-010-02, T-011-02]
---

## Context

Wire the combine step into the existing production preview UI. After the user runs "Build hybrid impostor" (which produces the three intermediates), they should be able to click "Build Asset Pack" and get a pack file written to `dist/plants/{species}.glb`.

## Acceptance Criteria

- New handler in `handlers.go`: `handleBuildPack` registered at `POST /api/pack/:id`
  - Reads the three intermediates from `outputsDir` for the given asset id
  - Constructs `PackMeta` via `BuildPackMetaFromBake(id)` (T-011-02)
  - Calls `CombinePack(...)`
  - Writes result to `dist/plants/{species}.glb`
  - Returns JSON `{ "pack_path": "...", "size": N, "species": "..." }`
  - Returns 400 if required intermediates are missing, 500 on combine error, 413 if pack exceeds 5 MB
- New button in `static/index.html`: "Build Asset Pack" — visible only when the asset has both `has_billboard` and either `has_billboard_tilted` or `has_volumetric`
- Click handler in `static/app.js` calls the new endpoint, surfaces success/error in the existing toast/log area, fires a `pack_built` analytics event with `{ species, size, has_tilted, has_dome }`
- 400/413 errors render a clear message in the UI ("Pack exceeds 5 MB — reduce variant count or texture resolution and re-bake")

## Out of Scope

- Batch packing (T-010-04)
- Centralized publishing (deferred)
- Per-asset override UI for species id / common name (use defaults from T-011-02)

## Notes

- Reuse the existing analytics-event helper; do not roll a new one.
- The UI button should be near the existing "Build hybrid impostor" trigger, not in a new panel.
