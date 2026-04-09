---
id: T-014-02
story: S-014
title: blender-render-production-script
type: task
status: open
priority: critical
phase: done
depends_on: [T-014-01]
---

## Context

Write `scripts/render_production.py` — a Blender Python script that renders the four impostor variants for one asset using the exact parameters documented in T-014-01. Runs headlessly via `blender -b --python`. Produces the same three intermediate GLB files the client-side JS currently creates.

## Usage

```sh
blender -b --python scripts/render_production.py -- \
    --source ~/.glb-optimizer/outputs/{id}.glb \
    --output-dir ~/.glb-optimizer/outputs/ \
    --id {id} \
    --category round-bush \
    --resolution 512 \
    --billboard-angles 6 \
    --tilted-elevation 30
```

Alternatively, all parameters can be read from a JSON config file (`--config params.json`) so the Go server can pass the full parameter set without a long arg list.

## Output files

- `{output-dir}/{id}_billboard.glb` — N side-variant quads named `billboard_0` ... `billboard_{N-1}` + one `billboard_top` quad. Materials: each quad has its own `baseColorTexture` (RGBA PNG embedded in the GLB).
- `{output-dir}/{id}_billboard_tilted.glb` — N tilted-variant quads. No top quad.
- `{output-dir}/{id}_volumetric.glb` — M dome-slice quads, ordered bottom→top, each a horizontal quad at the appropriate Y.

## Rendering approach

For each variant:
1. Import the source GLB via `bpy.ops.import_scene.gltf()`
2. Set up a simple studio lighting rig (hemisphere + directional, matching the client-side three.js scene setup as closely as practical)
3. Position an orthographic camera at the specified angle/elevation/distance
4. Set render resolution and transparent background (`film_transparent = True`)
5. Render to an in-memory image via Cycles or EEVEE
6. Create a `PlaneGeometry`-equivalent mesh in Blender, UV-mapped to the rendered image
7. Size the quad to match the model's silhouette from that angle (use the rendered image's alpha channel bounding box, or the model's projected bbox)
8. Export the quad mesh + texture as a minimal GLB

For volumetric slices:
1. Set clipping planes at each slice height (per the `slice_distribution_mode` from STRATEGY_TABLE)
2. Render from above through each clipping window
3. Create horizontal quads at each height

## Acceptance Criteria

- Produces three valid GLB files with the correct naming convention
- Each GLB contains the expected number of named mesh children (N side quads + 1 top for billboard, N for tilted, M for volumetric)
- Each mesh has a `baseColorTexture` with RGBA data at the specified resolution
- The existing combine step (`CombinePack`) successfully processes the Blender-produced intermediates — test by running `glb-optimizer pack <id>` after the render
- Running `node scripts/verify-pack.mjs` (T-012-03) on the produced pack passes validation
- Smoke test: render `inbox/dahlia_blush.glb`, compare file sizes against the known-good `1e562361...` intermediates (within 2x — exact match not required, but order-of-magnitude parity expected)

## Out of Scope

- Matching the three.js output pixel-for-pixel (different renderer, different AA, different tonemapping — visual parity is the goal, not binary identity)
- GPU acceleration in Blender (CPU rendering is fine for the initial version; GPU can be enabled later via `--gpu` flag)
- Batch processing (T-014-04 wraps this in a loop)
- EEVEE vs Cycles decision (pick whichever is faster for transparent-background renders; EEVEE is ~10x faster and sufficient for billboard textures)

## Notes

- Use **EEVEE** for speed. Billboard textures don't need ray-traced lighting — they need clean silhouettes with correct color. EEVEE renders a 512px frame in ~1 second; Cycles takes 10-30 seconds.
- The quad sizing matters: if the side billboard quad is 1m × 0.6m but the three.js version was 1m × 1m, the instancer's scale math breaks. Use the same sizing logic the three.js code uses (documented in T-014-01).
- Blender's `film_transparent` gives RGBA output with premultiplied alpha. The three.js offscreen renderer does the same. They should be compatible.
- Test early with the `dahlia_blush.glb` model in `inbox/`. If Blender can't import the TRELLIS GLB, that's a blocker to surface immediately.
