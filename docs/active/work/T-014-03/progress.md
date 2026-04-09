# T-014-03 Progress: API Build-Production Endpoint

## Step 1: Add types and mutex — DONE

Added to handlers.go:
- `blenderRenderMu sync.Mutex` (package-level)
- `blenderRenderTimeout = 300 * time.Second`
- `buildProductionConfig` struct (JSON config shape for render_production.py)
- Added imports: `context`, `os/exec`, `sync`

## Step 2: Implement handleBuildProduction — DONE

Handler function added to handlers.go (~130 lines). Full flow:
1. POST-only, Blender availability check, render script existence check
2. Asset ID extraction and FileStore lookup (404 if missing)
3. Status validation (400 if not done)
4. Category resolution: query param > saved settings > "unknown"
5. Strategy lookup via `getStrategyForCategory()`
6. Config struct built merging AssetSettings + ShapeStrategy
7. Temp JSON config written, deferred cleanup
8. Mutex acquired, `exec.CommandContext` with 300s timeout
9. Timeout detection (context.DeadlineExceeded) vs non-zero exit
10. Intermediate file verification (billboard, tilted, volumetric)
11. FileStore update and JSON response

Reused existing `fileExists` from bake_status.go (removed duplicate).

## Step 3: Wire route in main.go — DONE

- Resolved `renderScriptPath` after Blender detection block (tries CWD, falls back to executable dir)
- Registered: `mux.HandleFunc("/api/build-production/", handleBuildProduction(...))`

## Step 4: Write tests — DONE

Created `handlers_build_production_test.go` with 6 test cases:
- TestBuildProduction_MethodNotAllowed — PASS
- TestBuildProduction_BlenderNotAvailable — PASS
- TestBuildProduction_AssetNotFound — PASS
- TestBuildProduction_AssetNotOptimized — PASS
- TestBuildProduction_ConfigGeneration — PASS
- TestBuildProduction_HardSurfaceSkipsVolumetric — PASS

## Step 5: Verify build and tests — DONE

`go build ./...` clean. `go test ./...` all pass (3.2s).

## Deviations from Plan

None. Implementation followed the plan exactly.
