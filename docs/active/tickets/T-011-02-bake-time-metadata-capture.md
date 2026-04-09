---
id: T-011-02
story: S-011
title: bake-time-metadata-capture
type: task
status: open
priority: high
phase: done
depends_on: [T-010-01]
---

## Context

Combine needs a `PackMeta` to embed. Most fields can be derived from the bake-time state: the asset's source mesh (for footprint dims), the current settings (for fade thresholds), the asset id (for species name).

This ticket is the bridge: a function that reads the bake state at the moment of "Build Asset Pack" and assembles a fully-populated `PackMeta`.

## Acceptance Criteria

- New function `BuildPackMetaFromBake(id string) (PackMeta, error)` in a new `pack_meta_capture.go` (or in `pack.go`)
  - Reads the original (un-decimated) source mesh GLB for asset `id`
  - Computes footprint:
    - `height_m` = `max_y - min_y` of the world AABB
    - `canopy_radius_m` = `max(width_x, depth_z) / 2`
  - Reads `settings.go` current values for `tilted_fade_low_start`, `tilted_fade_low_end`, `tilted_fade_high_start`
  - Determines species id:
    - Read from a per-asset config file if present (`outputs/{id}_meta.json`)
    - Else: derive from source filename (lowercase, non-alphanum → `_`, strip leading digits)
  - Determines common_name:
    - Read from per-asset config if present
    - Else: title-case the species id with underscores → spaces
  - Sets `bake_id` to current UTC time in RFC3339
  - Sets `format_version` to `PackFormatVersion`
  - Returns the assembled `PackMeta`, validated
- Unit test: synthetic bake state produces correct meta; invalid species id raises error
- Integration test: a real bake intermediate produces a meta whose footprint values are within 5% of expected for at least one fixture asset

## Out of Scope

- UI for editing meta before pack (a future override UI; not for demo)
- Reading scientific name from EXIF or other embedded sources
- Per-asset config file format spec — `outputs/{id}_meta.json` is just an opaque JSON object with `species` / `common_name` keys for now

## Notes

- If no source mesh is available (asset deleted from outputs/), return a clear error — combine should fail loudly, not silently use defaults.
- The `_meta.json` per-asset config is intentionally minimal. It exists as an escape hatch for cases where filename-derived species ids aren't right (e.g., `sample_2026-04-08T010040.068` should map to `dahlia_blush`).
