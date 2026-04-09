---
id: S-010
epic: E-002
title: pack-combine-step
type: story
status: open
priority: critical
tickets: [T-010-01, T-010-02, T-010-03, T-010-04, T-010-05]
---

## Goal

Implement the combine step that turns the three existing bake intermediates (`{id}_billboard.glb`, `{id}_billboard_tilted.glb`, `{id}_volumetric.glb`) into one Pack v1 file (`{species}.glb`) per the schema in E-002.

## Context

The runtime preview at app.js:3865-3895 already proves the four-variant scheme works. The intermediates exist on disk after a "Build hybrid impostor" run. What's missing is the merge: read three GLBs, split `billboard_top` out of the side file, reparent meshes under `view_side` / `view_top` / `view_tilted` / `view_dome` groups, merge binary chunks (textures, buffers), de-dupe textures by content hash, write `extras.plantastic`, save as one file under the 5 MB cap.

The Go side already has GLB chunk-parsing primitives in `scene.go:CountTrianglesGLB`. Reuse the chunk parser; build a chunk writer alongside it.

## Acceptance Criteria

- `pack.go::CombinePack(sideGlb, tiltedGlb, volumetricGlb, meta) → (packBytes, error)` produces a Pack v1 file matching the E-002 schema
- The `billboard_top` mesh in the side intermediate is correctly extracted and reparented under `view_top`; remaining side meshes go under `view_side`
- Optional intermediates (tilted, volumetric) can be `nil` — combine still produces a valid pack with only required variants
- Texture buffers are de-duplicated by SHA256 of their data
- Output file is < 5 MB or returns an error
- `extras.plantastic` is written exactly per the E-002 schema, validated against a schema test fixture
- A "Build Asset Pack" button in the production preview UI calls a new `POST /api/pack/:id` endpoint that runs the combine and saves to `dist/plants/{species}.glb`
- `just pack-all` recipe runs combine over every asset in the outputs dir that has the required intermediates
- Round-trip test: combine output loads in three.js GLTFLoader without errors, has the expected named children

## Non-Goals

- Re-running bakes (combine is downstream of bakes)
- Texture format conversion / compression
- Schema versioning beyond `format_version: 1`

## Dependencies

None — this story is the foundation. T-010-01 must ship before downstream tickets in either repo.
