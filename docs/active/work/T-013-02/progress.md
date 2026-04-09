# T-013-02 Progress: just bake recipe

## Steps Completed

### Step 1: Created bake_status.go ✓
- `discoverAllIDs` walks outputs dir, strips known suffixes (ordered longest-first to avoid partial matches), returns sorted unique IDs.
- `checkIntermediates` checks for billboard, billboard_tilted, and volumetric files.
- `fileExists` helper (unexported).

### Step 2: Implemented runBakeStatusCmd ✓
- Resolves workdir via existing `resolveWorkdir`.
- Scans existing files into FileStore for species resolution.
- Resolves species via `ResolveSpeciesIdentity` with fallback to truncated hash.
- Checks pack existence in dist/plants/{species}.glb.
- Prints tabwriter table: SPECIES, BILLBOARD, TILTED, DOME, PACK.
- Prints total summary line.

### Step 3: Registered bake-status in main.go ✓
- Added `case "bake-status":` to subcommand switch after `clean-stale-packs`.

### Step 4: Wrote bake_status_test.go ✓
- `TestDiscoverAllIDs` — 3 assets with mixed suffixes + 1 non-matching file.
- `TestDiscoverAllIDs_Empty` — empty dir returns empty slice.
- `TestCheckIntermediates` — selective intermediates (billboard + dome, no tilted).
- `TestCheckIntermediates_None` — nonexistent ID returns all false.

### Step 5: Replaced justfile bake recipe ✓
- Full-lifecycle bash script with `set -euo pipefail`.
- Server detection via `curl -sf /api/status`.
- Conditional start with `go run . --port $PORT &` + PID tracking.
- Poll loop (30 × 0.5s = 15s max) with process liveness check.
- `trap cleanup EXIT` for reliable server teardown.
- Passes `--port` flag to headless-bake.ts.

### Step 6: Added bake-status justfile recipe ✓
- `bake-status: build` → `./glb-optimizer bake-status`

### Step 7: Tests and verification ✓
- `go test ./...` — all tests pass.
- `just --list` — all recipes valid.
- `go build` compiles cleanly.

## Deviations from Plan

None. Implementation followed the plan exactly.
