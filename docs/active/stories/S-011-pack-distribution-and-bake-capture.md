---
id: S-011
epic: E-002
title: pack-distribution-and-bake-capture
type: story
status: open
priority: critical
tickets: [T-011-01, T-011-02, T-011-03, T-011-04]
depends_on: [S-010]
---

## Goal

Get pack files out of glb-optimizer in a transferable form, with the bake-time settings (fade thresholds, footprint, species id) accurately captured into the pack metadata so the consumer doesn't need to know anything about the bake.

## Context

The combine step (S-010) needs metadata to write into `extras.plantastic`. That metadata has to come from somewhere — the bake driver knows the current `tilted_fade_*` settings and the source mesh, so it can capture footprint dims and fade thresholds at the moment the user clicks "Build hybrid impostor" or "Build Asset Pack."

The output landing zone is `dist/plants/{species}.glb`. Glb-optimizer has zero knowledge of plantastic's filesystem layout — distribution is **manual USB drop** for the demo. A future ticket may add a publish CLI; that's not in this epic.

## Acceptance Criteria

- Combine writes packs to `dist/plants/{species}.glb` (created if missing)
- The bake driver captures `currentSettings.tilted_fade_low_start/low_end/high_start` at the moment of bake and includes them in the meta passed to combine
- Footprint (`canopy_radius_m`, `height_m`) is computed from the source mesh's axis-aligned bounding box with sensible axis choices (height = max_y − min_y, canopy_radius = max(width_x, depth_z) / 2)
- `species` id is captured from the source filename or a per-asset config field; defaults to `filepath.Base(input)` minus `.glb`, lowercased, non-alphanum → underscore
- `common_name` is read from a per-asset config field if present; falls back to the species id with underscores → spaces and title case
- `bake_id` is set to the combine timestamp in ISO 8601 UTC
- Existing intermediates in outputs/ are not modified — combine only reads them

## Non-Goals

- HTTPS publish / asset server (E-003 or later)
- Per-species manual override UI for footprint values (compute from bbox, that's it)
- Bake-time validation that pack will fit under 5 MB (size check happens in combine — if it fails, the user re-bakes with smaller textures or fewer variants)

## Dependencies

S-010 — combine step exists and can accept metadata.
