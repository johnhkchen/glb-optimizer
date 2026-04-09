---
id: T-010-02
story: S-010
title: combine-implementation
type: task
status: open
priority: critical
phase: done
depends_on: [T-010-01]
---

## Context

Implement the actual GLB merge: read up to three intermediate `.glb` files, reparent their meshes under the four named groups required by Pack v1, de-dupe textures by content hash, embed the metadata, write the output.

The trick is splitting `billboard_top` out of the side intermediate: per app.js:3865-3895 the side bake produces N "side variant" meshes plus exactly one mesh named `billboard_top`. The combine needs to recognize `billboard_top` and reparent it under `view_top` while everything else from that file goes under `view_side`.

The existing `CountTrianglesGLB` in `scene.go` parses the JSON chunk. Reuse that pattern; add a chunk writer.

## Acceptance Criteria

- `func CombinePack(side []byte, tilted []byte, volumetric []byte, meta PackMeta) ([]byte, error)`
  - `side` is required; `tilted` and `volumetric` may be `nil`
  - Returns the bytes of a valid Pack v1 GLB
- Mesh routing:
  - From `side`: mesh named exactly `billboard_top` → child of new `view_top` node (renamed to `view_top`); all other meshes → children of new `view_side` group (renamed `variant_0`, `variant_1`, ...)
  - From `tilted` (if present): all meshes → children of new `view_tilted` group (renamed `variant_0`, ...)
  - From `volumetric` (if present): all meshes → children of new `view_dome` group (renamed `slice_0`, `slice_1`, ... ordered by their original Y position bottom→top)
- Buffer merge: combine all three intermediates' bin chunks into one, rewriting accessor / bufferView byte offsets
- Texture dedup: hash each texture's image bytes (SHA256), share a single image when collisions exist
- Material independence: each mesh keeps its own material instance even if textures are shared (consumer needs per-variant opacity)
- `extras.plantastic` populated from `meta`; validated via `meta.Validate()` before write
- Returns error if final size > 5 * 1024 * 1024 bytes
- Round-trip test: a synthetic 3-intermediate fixture combines to a pack that re-parses cleanly and contains exactly the expected node names

## Out of Scope

- HTTP endpoint (T-010-03)
- Bake-time meta capture (T-011-02)
- Texture format conversion (PNG stays PNG)

## Implementation Notes

- The dome slice ordering matters: the consumer doesn't care about names but it does care that the child order is consistent so per-instance offsets line up. Sort by original min-Y of each slice's bbox.
- Do NOT mutate the input byte slices.
- For the unit test fixtures: build minimal valid GLBs in-memory using stringly-typed JSON + a tiny binary chunk. Don't depend on actual baked assets.
- Watch for: GLB chunk alignment (4-byte padding), JSON chunk requires trailing space padding, BIN chunk requires trailing zero padding.
