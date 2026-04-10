---
id: T-015-01
story: S-015
title: Fix dome/top-down camera position in render_production.py
type: task
status: open
priority: high
phase: plan
depends_on: []
---

## Context

The round-bush strategy's dome slices are rendered with the camera in the
wrong position. The dome slices should capture horizontal cross-sections
of the plant at different heights, viewed from above. Instead, they appear
to capture side views, making the plant look flat when viewed top-down.

## Investigation

1. Read `scripts/render_production.py` — find the dome/volumetric slice
   rendering section
2. Check camera position and orientation for dome renders:
   - Camera should be above the plant, looking down (-Y or -Z depending
     on Blender coordinate convention)
   - Each slice at a different height (Y offset in the plant's bbox)
3. Check the `STRATEGY_TABLE` for round-bush:
   - `slice_axis: "y"` — should slice along Y (height)
   - `slice_distribution_mode: "visual-density"` — more slices where
     there's more visual mass
   - `volumetric_layers: 4` — 4 dome slices
4. Compare with what plantastic's SpeciesInstancer expects for dome slices
5. Check if the issue is camera position, camera rotation, or slice plane
   orientation

## Likely fix

The dome slice camera is probably using the same camera rig as the side
billboards (orbiting around the plant) instead of a dedicated overhead rig.
The fix is likely changing the camera position for dome renders to be
directly above, looking straight down.

## Test

After fix:
1. Re-bake one species: `glb-optimizer prepare inbox/achillea_millefolium.glb -category round-bush -resolution 256`
2. Upload new pack to R2
3. Verify in plant-catalog billboard crossfade viewer — dome slices should
   show the canopy from above, not the side profile

## Acceptance Criteria

- Dome slices show the plant canopy from above
- Round-bush strategy produces convincing top-down view
- Side billboards remain correct (no regression)
