---
id: T-010-05
story: S-010
title: pack-size-cap
type: task
status: open
priority: medium
phase: done
depends_on: [T-010-02]
---

## Context

5 MB hard cap per pack. Forces texture discipline at bake time and keeps the total demo asset payload bounded. The cap is enforced inside `CombinePack` (T-010-02) but this ticket is about the error-message ergonomics — when a bake produces a pack that's too big, the user needs to know *why* and what to do about it.

## Acceptance Criteria

- `CombinePack` returns a structured error type `*PackOversizeError` (not a plain `errors.New`) with fields:
  - `Species string`
  - `ActualBytes int64`
  - `LimitBytes int64`
  - `Breakdown` — counts of textures, total texture bytes, mesh bytes, JSON bytes
- `Error()` formats as:
  ```
  pack "achillea_millefolium" exceeds 5 MB limit (actual: 6.2 MB)
    textures:    18 × avg 320 KB = 5.7 MB
    meshes:      147 KB
    metadata:    2 KB
  hint: reduce billboard texture resolution or variant count and re-bake
  ```
- The HTTP endpoint (T-010-03) returns this message verbatim in its 413 response
- Unit test: synthetic combine that exceeds the cap returns a `*PackOversizeError` with correct breakdown

## Out of Scope

- Auto-shrinking textures (out of E-002 scope; that's a bake-side concern)
- Soft warnings under the limit
- Configurable limit per environment

## Notes

- The cap is intentionally hard. If a species genuinely needs more than 5 MB to look good, that's a signal to revisit the bake — not to relax the limit.
