# T-012-05 Plan — Implementation Sequence

Each step is a self-contained, commit-able unit. Steps 1–2 are
usable as a Go library even before the CLI lands.

## Step 1 — clean_packs.go

Create the new file with `IdentifyStalePacks` and `RemoveStalePacks`
exactly as specified in structure.md. Imports: `errors`, `fmt`, `io`,
`os`, `path/filepath`, `sort`, `strings`. Reuses `discoverPackableIDs`
(pack_cmd.go), `ResolveSpeciesIdentity` (species_resolver.go), and
`humanBytes` (combine.go).

**Verify:** `go build ./...` passes.

## Step 2 — clean_packs_test.go

Write all ten unit tests from structure.md. Pattern: each test calls
`t.TempDir()`, drops synthetic files via `os.WriteFile`, calls the
function, asserts on returned slice / files-on-disk / output buffer.
No FileStore needed for the synthetic-id tests because the resolver
hash-fallback tier produces a deterministic slug for any non-hex id
(see `species_resolver.go:hashFallbackIdentity`). For ids that look
like hex hashes, set up a minimal FileStore record so the file-store
tier wins.

Specifically: use ids like `"my_test_species"`, `"another_one"`. These
are non-hex, so the resolver derives the slug from the id itself
(`deriveSpeciesFromName` returns `"my_test_species"`). Match dist
filename `my_test_species.glb` and the test passes.

**Verify:** `go test -run TestIdentifyStalePacks -v ./...` and
`go test -run TestRemoveStalePacks -v ./...` both pass.

## Step 3 — clean_cmd.go

Create the new file with `runCleanStalePacksCmd` from structure.md.
Imports: `flag`, `fmt`, `os`, `path/filepath`. Reuses `resolveWorkdir`,
`scanExistingFiles`, `LoadMappingFile`, `IdentifyStalePacks`,
`RemoveStalePacks`.

**Verify:** `go build ./...` passes.

## Step 4 — main.go subcommand wiring

Add one case to the existing switch in `main()`:

```go
case "clean-stale-packs":
    os.Exit(runCleanStalePacksCmd(os.Args[2:]))
```

Inserted after the `pack-inspect` case, before the closing brace.

**Verify:** `go build ./...` passes; `./glb-optimizer clean-stale-packs --help`
prints flags (or at least exits non-zero with usage; flag.ContinueOnError
default behaviour).

## Step 5 — pack_cmd.go --clean flag

Three edits to `runPackAllCmd`:

1. After the existing `mappingFlag` declaration:
   ```go
   cleanFlag := fs.Bool("clean", false, "Remove stale packs after a successful pack-all")
   ```

2. After `printPackSummary(os.Stdout, results)` and the existing
   failure-detection loop, refactor:
   ```go
   anyFailed := false
   for _, r := range results {
       if r.Status != "ok" {
           anyFailed = true
           break
       }
   }
   ```

3. Then:
   ```go
   if *cleanFlag {
       fmt.Fprintln(os.Stdout, "")
       if anyFailed {
           fmt.Fprintln(os.Stdout, "Skipped stale-pack cleanup: pack-all had failures.")
       } else {
           fmt.Fprintln(os.Stdout, "Cleaned stale packs:")
           stale, err := IdentifyStalePacks(distDir, outputsDir, store, opts)
           if err != nil {
               fmt.Fprintf(os.Stderr, "cleanup: %v\n", err)
               return 1
           }
           if err := RemoveStalePacks(os.Stdout, stale, false); err != nil {
               fmt.Fprintf(os.Stderr, "cleanup: %v\n", err)
               return 1
           }
       }
   }
   if anyFailed {
       return 1
   }
   return 0
   ```

The original loop returned 1 immediately on first failure. The
refactor preserves that behaviour (anyFailed → return 1) while
allowing cleanup to run between summary print and exit on the
all-ok path.

**Verify:** `go build ./...` and existing pack-all tests still pass:
`go test -run TestRunPackAll -v ./...` (or whatever the existing
naming is).

## Step 6 — pack_cmd_test.go integration test

Adding a real `runPackAllCmd` integration test requires a real
billboard intermediate that combines successfully. The existing
`pack_runner_test.go` does this via `setupPackEnv`. Look up that
helper and reuse it if possible.

**Fallback if integration is too heavy:** add a unit test that
calls `IdentifyStalePacks` after constructing a synthetic outputs
dir + dist dir, then calls `RemoveStalePacks(apply=true)`, then
asserts on file presence. This still validates the round-trip.

Test name: `TestRunPackAll_CleanFlag_RemovesStalePack` if integration,
or `TestStalePackCleanup_RoundTrip` if unit-level.

**Verify:** `go test ./... -count=1` passes.

## Step 7 — justfile recipes

Add the two new recipes after the existing `clean-packs` block:

```makefile
# Show stale packs in dist/plants/ that no longer have a source
# intermediate (dry-run; nothing is deleted).
clean-stale-packs: build
    ./glb-optimizer clean-stale-packs

# Actually delete the stale packs identified above. No undo.
clean-stale-packs-apply: build
    ./glb-optimizer clean-stale-packs --apply
```

**Verify:** `just --list | grep clean` shows both new recipes.

## Step 8 — final go build / go test

```bash
go build ./...
go test ./... -count=1
```

Both must pass with no warnings. If any preexisting test breaks
because of the pack-all refactor, that is a regression and must be
fixed before review.

## Testing strategy summary

| Layer | Tests | Style |
|-------|-------|-------|
| `IdentifyStalePacks` | 6 unit tests | TempDir + synthetic files |
| `RemoveStalePacks` | 4 unit tests | TempDir + io.Writer buffer |
| `runCleanStalePacksCmd` | 0 (covered by units) | — |
| `runPackAllCmd --clean` | 1 integration or unit | TempDir round-trip |
| justfile | 0 | manual smoke |

Total new test functions: ~11. Existing tests: must remain green.

## Verification checklist (pre-review)

- [ ] `go build ./...` clean
- [ ] `go test ./... -count=1` clean
- [ ] `./glb-optimizer clean-stale-packs --dir /tmp/empty` exits 0,
      prints "No stale packs." (manual smoke against an empty workdir)
- [ ] `just clean-stale-packs` and `just clean-stale-packs-apply`
      both visible in `just --list`
- [ ] Ticket ACs reviewed line-by-line in review.md
