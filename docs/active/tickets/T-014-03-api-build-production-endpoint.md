---
id: T-014-03
story: S-014
title: api-build-production-endpoint
type: task
status: open
priority: critical
phase: done
depends_on: [T-014-02]
---

## Context

Wire the Blender render script into the Go server as an API endpoint. The browser's "Build hybrid impostor" button (T-014-05) and the CLI `prepare` subcommand (T-014-04) both funnel through this endpoint.

## Endpoint

`POST /api/build-production/{id}?category={category}`

- `id` — asset hash (must exist in outputs/ with status=done, i.e., already optimized)
- `category` — shape category override (default: read from asset's saved settings). Valid: `round-bush`, `directional`, `tall-narrow`, `planar`, `hard-surface`.

### Behavior

1. Validate the asset exists and has been optimized (status=done)
2. Read the STRATEGY_TABLE entry for the category to get volumetric_layers, slice_axis, etc.
3. Read the asset's saved settings for billboard resolution, angle count, tilted elevation
4. Invoke Blender headlessly:
   ```
   blender -b --python scripts/render_production.py -- \
       --source {outputs}/{id}.glb --output-dir {outputs}/ --id {id} \
       --config <tmpfile with JSON params>
   ```
5. Wait for Blender to complete (timeout: 5 minutes per asset)
6. Verify the three intermediate files were written
7. Update the FileStore: `has_billboard=true`, `has_billboard_tilted=true`, `has_volumetric=true`
8. Return JSON `{ "id": "...", "billboard": true, "tilted": true, "volumetric": true, "duration_ms": N }`

### Error handling

- Blender not found → 500 with `"blender not installed"`
- Asset not optimized → 400 with `"asset must be optimized first (status=done)"`
- Blender exits non-zero → 500 with Blender's stderr
- Timeout → 500 with `"render timed out after 300s"`
- Intermediate files missing after Blender exits 0 → 500 with `"blender completed but intermediates missing"`

### Concurrency

- Only one Blender render at a time (Go mutex). Concurrent requests queue.
- The endpoint is async-friendly: returns immediately with `202 Accepted` + a job ID, caller polls `GET /api/build-production-status/{jobId}` for completion. OR: synchronous (blocks until done) for simplicity in v1. **Pick synchronous for v1** — the CLI and UI both wait anyway.

## Acceptance Criteria

- `curl -X POST localhost:8787/api/build-production/{id}?category=round-bush` runs Blender and returns the three intermediate flags
- The produced intermediates are identical in format to what the client-side JS produced (same GLB structure, same mesh naming)
- `glb-optimizer pack <id>` successfully combines the Blender-produced intermediates
- The endpoint is registered in `main.go` alongside the existing routes
- Server startup still logs "Found Blender: ..." — if Blender is missing, the endpoint returns 500 but the server still starts (billboard rendering is optional, not a hard dependency)

## Out of Scope

- Async job queue (v1 is synchronous)
- Progress reporting during the render
- GPU acceleration toggle
- The UI button update (T-014-05)
