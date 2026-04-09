# T-013-02 Structure: just bake recipe

## Files to Modify

### justfile
- **Replace** existing `bake source:` recipe (line 54-55) with full-lifecycle version
- **Add** `bake-status: build` recipe after bake-related recipes

### main.go
- **Add** `case "bake-status":` to the subcommand dispatch switch (after `clean-stale-packs`)

## Files to Create

### bake_status.go
New Go file implementing the `bake-status` subcommand. Functions:

- `runBakeStatusCmd(args []string) int` — entry point, resolves workdir, calls discoverAllIDs, resolves species, prints table
- `discoverAllIDs(outputsDir string) []string` — walks outputs dir, extracts unique content-hash prefixes
- `checkIntermediates(outputsDir, id string) (billboard, tilted, dome bool)` — checks file existence for three intermediates

### bake_status_test.go
Tests for the bake-status subcommand:

- `TestDiscoverAllIDs` — temp dir with mixed files, verify correct ID extraction
- `TestCheckIntermediates` — temp dir with selective intermediates, verify bool detection
- `TestRunBakeStatusCmd` — integration test with temp workdir

## Files NOT Modified

- `headless-bake.ts` — no changes needed; already handles bake + pack
- `bake-debug` recipe — kept as-is
- `bake-install` recipe — kept as-is
- `pack_cmd.go` — bake-status is a separate subcommand, doesn't modify pack logic
- No Go server routes touched (out of scope per ticket)

## Dependency Graph

```
justfile changes ─── no dependencies on Go changes (recipe just calls ./glb-optimizer)
main.go ─── depends on bake_status.go existing
bake_status.go ─── uses resolveWorkdir from pack_cmd.go
                    uses FileStore + ResolverOptions from existing code
bake_status_test.go ─── depends on bake_status.go
```

## Function Signatures

```go
// bake_status.go
func runBakeStatusCmd(args []string) int
func discoverAllIDs(outputsDir string) ([]string, error)  
func checkIntermediates(outputsDir, id string) (billboard, tilted, dome bool)
```
