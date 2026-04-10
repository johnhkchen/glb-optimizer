---
id: S-015
title: Fix top-down billboard render for round-bush strategy
status: open
tickets: [T-015-01]
---

## Problem

The round-bush rendering strategy produces a top-down billboard (view_dome
slices) that looks like the plant is flat on the ground instead of viewed
from above. The Blender camera for the top-down pass is positioned to the
side instead of above, so the dome slices capture a side profile instead
of an overhead canopy view.

## Impact

The billboard crossfade viewer in plant-catalog shows incorrect dome
slices — when the user orbits to a top-down view, they see a flat side
view instead of the canopy from above. This breaks the core value
proposition of the hybrid impostor system.

## Where to look

- `scripts/render_production.py` — the Blender headless renderer that
  positions cameras for each billboard variant
- The dome slice rendering section — camera should be positioned above
  the plant looking down, not from the side
- Compare with the side billboard camera positions (those work correctly)
- The `STRATEGY_TABLE` round-bush entry may have incorrect `slice_axis`
  or `slice_distribution_mode` settings
