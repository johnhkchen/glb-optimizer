# T-014-04 Progress: CLI Prepare Subcommand

## Files Created
- `prepare_cmd.go` (~370 lines) — `runPrepareCmd`, `runPrepareAllCmd`, `runPrepare`, `hashFile`, `speciesFromFilename`, `formatSize`, `printPrepareSummary`, `printPrepareJSON`
- `prepare_cmd_test.go` (~190 lines) — 12 tests covering helpers, output formatting, error paths, CLI arg validation

## Files Modified
- `main.go` — added `"prepare"` and `"prepare-all"` cases to subcommand dispatch switch

## Implementation Notes

### Plan Deviations: None
All 9 plan steps completed as specified.

### Key Implementation Details
1. **hashFile** uses SHA-256 truncated to 16 bytes (32 hex chars) matching `generateID()` length
2. **speciesFromFilename** sanitizes filenames: lowercase, replace spaces/hyphens with underscores, strip non-alphanumeric, truncate to 64 chars
3. **runPrepare** executes all 8 pipeline steps with fail-fast error propagation and step identification
4. **Idempotency**: Steps 1, 3, 5 skip if output files already exist
5. **Blender config** reuses `buildProductionConfig` struct from handlers.go
6. **Pack step** delegates to existing `RunPack()` with `CLISpecies`/`CLICommonName` from filename
7. **Verify step** calls existing `InspectPack()` — success means valid pack structure

### Build & Test Status
- `go build ./...` — clean
- `go vet ./...` — clean
- `go test ./...` — all pass (12 new + all existing)
