# T-013-02 Plan: just bake recipe

## Implementation Steps

### Step 1: Create bake_status.go with ID discovery and intermediate checks

Write `discoverAllIDs` and `checkIntermediates` functions. `discoverAllIDs` walks the outputs directory, strips known suffixes to extract unique content-hash prefixes. `checkIntermediates` checks for `_billboard.glb`, `_billboard_tilted.glb`, and `_volumetric.glb`.

### Step 2: Implement runBakeStatusCmd

Wire up the full subcommand: resolve workdir, discover IDs, resolve species names (reuse existing resolver pattern from pack_cmd.go), check for pack existence in dist/plants/, print tabwriter table.

### Step 3: Register bake-status in main.go

Add `case "bake-status":` to the subcommand switch in main.go.

### Step 4: Write bake_status_test.go

Tests for `discoverAllIDs` (correct ID extraction from mixed files) and `checkIntermediates` (selective intermediate detection).

### Step 5: Replace justfile `bake` recipe

Replace the thin T-013-01 `bake` recipe with the full-lifecycle version:
- Check if server is running via `curl /api/status`
- If not, start with `go run . --port 8787 &` and poll for readiness
- Run headless-bake.ts via the existing scripts path
- Clean up server if we started it (trap-based)

### Step 6: Add justfile `bake-status` recipe

Add thin wrapper: `bake-status: build` → `./glb-optimizer bake-status`.

### Step 7: Run tests and verify

Run `go test ./...` to verify Go tests pass. Manually verify the justfile recipe syntax is valid with `just --list`.

## Commit Strategy

Single commit for all changes — the justfile recipe and Go subcommand are tightly coupled and not useful independently.

## Risk Mitigations

- Server lifecycle shell code is the most fragile part. Keep it simple: one background process, one PID, one trap.
- `discoverAllIDs` must handle the suffix-stripping correctly — the `_billboard_tilted.glb` suffix must be stripped before `_billboard.glb` to avoid partial matches. Test covers this.
