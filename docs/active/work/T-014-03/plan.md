# T-014-03 Plan: API Build-Production Endpoint

## Implementation Steps

### Step 1: Add types and mutex to handlers.go

Add `blenderRenderMu`, `buildProductionConfig`, and `buildProductionResponse` types near the top of handlers.go (after existing type declarations).

**Files**: handlers.go
**Verify**: `go build` passes

### Step 2: Implement handleBuildProduction handler

Write the full handler function in handlers.go. Flow:
1. POST-only method check
2. Extract ID from URL path (`strings.TrimPrefix`)
3. Look up in FileStore, check status == "done"
4. Resolve category from query param or saved settings
5. Load AssetSettings + get ShapeStrategy
6. Build `buildProductionConfig` struct
7. Marshal to JSON, write temp file `{outputsDir}/{id}_render_config.json`
8. Acquire mutex, build `exec.CommandContext` with 300s timeout
9. Run Blender: `blender -b --python <script> -- --config <tmpfile>`
10. Release mutex, delete temp file (defer)
11. Check exit code: timeout vs non-zero vs success
12. Verify intermediate files: `{id}_billboard.glb`, `{id}_billboard_tilted.glb`, `{id}_volumetric.glb`
13. Update FileStore flags
14. Return JSON response with duration_ms

**Files**: handlers.go
**Verify**: `go build` passes

### Step 3: Wire route in main.go

1. Resolve `renderScriptPath` after Blender detection block
2. Register route: `mux.HandleFunc("/api/build-production/", handleBuildProduction(...))`

**Files**: main.go
**Verify**: `go build` passes, `go vet` clean

### Step 4: Write tests

Create `handlers_build_production_test.go` with:
- Method not allowed (GET → 405)
- Blender not available (→ 500)
- Asset not found (→ 404)
- Asset not optimized (→ 400)
- Config JSON generation (verify shape)
- hard-surface skips volumetric

Tests use the existing test patterns from `handlers_bake_complete_test.go` and `handlers_upload_test.go`.

**Files**: handlers_build_production_test.go (new)
**Verify**: `go test ./...` passes

### Step 5: Verify build and tests

Run full build + test suite to confirm no regressions.

**Verify**: `go build && go test ./...` all pass
