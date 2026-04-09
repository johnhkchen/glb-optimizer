---
id: T-014-01
story: S-014
title: extract-rendering-parameters
type: task
status: open
priority: critical
phase: done
depends_on: []
---

## Context

The Production variant in `static/app.js` renders four types of impostor artifacts. Each render function has hardcoded camera parameters, resolution, angle counts, and geometry construction logic. Before writing the Blender script, we need an authoritative document of every parameter the client-side code uses.

## Deliverable

A new file `docs/knowledge/production-render-params.md` documenting every rendering parameter extracted from `app.js`:

### Side billboards (`renderBillboardGLB`)
- Number of azimuth angles (N)
- Camera distance from model center (how computed — bbox diagonal? fixed?)
- Camera FOV or orthographic size
- Camera elevation (0° = horizontal)
- Render resolution (width × height pixels)
- Background: transparent RGBA
- Output quad: dimensions, pivot point (bottom-center? geometric center?), UV mapping
- Naming convention: `billboard_0`, `billboard_1`, ..., `billboard_top`

### Top-down billboard (`renderBillboardTopDown`)
- Camera position (directly above model center)
- Orthographic or perspective? Size/FOV
- Render resolution
- Output quad: horizontal, dimensions, name (`billboard_top`)

### Tilted billboards (`renderTiltedBillboardGLB`)
- Number of azimuth angles
- Camera elevation angle (the ~30-35° value from S-009)
- Camera distance
- Same resolution as side? Different?
- Output quads: no `billboard_top` (confirmed in T-009-01 design docs)
- Naming convention: same `billboard_N` pattern but in a separate file

### Volumetric dome slices
- Number of slices (from `STRATEGY_TABLE[category].volumetric_layers`)
- Slice axis (from `STRATEGY_TABLE[category].slice_axis`)
- Slice distribution mode (`visual-density` vs `equal-height`)
- Per-slice: camera position, clipping planes, render resolution
- Per-slice: output quad dimensions, Y position, orientation

### Shared
- `STRATEGY_TABLE` entries per shape category (round-bush, directional, tall-narrow, planar, hard-surface)
- `TILTED_BILLBOARD_ANGLES` constant
- `TILTED_BILLBOARD_RESOLUTION` constant
- Billboard resolution constant(s)
- Any material overrides (env map, lighting setup for the offscreen render)

## Acceptance Criteria

- Document covers all four render types with numeric values extracted from the JS source
- Each parameter has a line reference to `app.js` (line number + function name)
- Document includes a "validation plan" section: how to compare Blender output against the known-good `1e562361...` intermediates (file size ranges, quad counts, texture dimensions)
- Reviewed by reading the actual JS code, NOT by guessing from docs or comments

## Out of Scope

- Writing the Blender script (T-014-02)
- Modifying app.js
- Optimizing or changing any parameter values
