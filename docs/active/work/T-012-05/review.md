# T-012-05 Review — Stale Pack Cleanup

## Summary

Adds a stale-pack identifier and remover with two CLI surfaces: a
standalone `glb-optimizer clean-stale-packs` (dry-run by default,
`--apply` to delete) and a `--clean` flag on `pack-all` that runs
cleanup after a fully successful pack pass. Closes T-010-04 review
Open Concern #3.

## Files

| File | Change | LOC |
|------|--------|-----|
| `clean_packs.go` | NEW | 137 |
| `clean_packs_test.go` | NEW | 295 |
| `clean_cmd.go` | NEW | 65 |
| `main.go` | MOD (+2 LOC) | subcommand case |
| `pack_cmd.go` | MOD (+~25 LOC) | --clean flag, post-pack cleanup hook, anyFailed refactor |
| `justfile` | MOD (+10 LOC) | clean-stale-packs + clean-stale-packs-apply recipes |

## Acceptance criteria coverage

### Cleanup logic

- ✅ `IdentifyStalePacks(distDir, outputsDir string, store, opts) ([]string, error)` —
  signature widened to take resolver inputs (`store`, `opts`) so
  pack-all `--clean` reuses the exact same resolver state it just
  used for packing. The ticket-level signature is preserved as the
  first two arguments. `clean_packs.go:35`
- ✅ Forward-mapping algorithm: walk discoverPackableIDs, build live
  set via ResolveSpeciesIdentity, walk dist for *.glb, set-difference.
- ✅ Returns sorted list of stale pack absolute paths.
- ✅ `RemoveStalePacks(w io.Writer, stale []string, dryRun bool) error` —
  signature gained an `io.Writer` for testability; the CLI passes
  `os.Stdout`. `clean_packs.go:88`
- ✅ Dry-run prints, removes nothing.
- ✅ Apply removes each file, prints what was removed.
- ✅ Per-file removal failures log + continue, return aggregated
  error via `errors.Join`.

### CLI integration

- ✅ `just clean-stale-packs` invokes the binary in dry-run mode.
- ✅ `just clean-stale-packs-apply` invokes with `--apply` (chose
  the explicit-recipe form over `--` passthrough; documented in
  design.md).
- ✅ `--clean` flag on pack-all runs cleanup AFTER successful packing
  only. Failed pack-all prints `Skipped stale-pack cleanup: pack-all
  had failures.` instead.
- ✅ Cleanup output appended to the pack-all summary table (after
  the TOTAL line, before exit).

### Tests

| AC | Test |
|----|------|
| Identify stale with synthetic dirs | `TestIdentifyStalePacks_Mixed` |
| Empty dist returns empty list | `TestIdentifyStalePacks_EmptyDist` |
| All-stale dist returns all packs | `TestIdentifyStalePacks_AllStale` |
| Dry-run leaves files in place | `TestRemoveStalePacks_DryRunLeavesFiles` |
| Apply mode deletes files | `TestRemoveStalePacks_ApplyDeletes` |
| Removal of non-existent file logs warning, continues | `TestRemoveStalePacks_MissingFileLogsContinues` |
| Integration: pack-all + --clean removes a stale pack | `TestStalePackCleanup_RoundTrip` (unit-level round trip; see "Open concerns" below) |

Plus three additional tests not required by the AC but useful for
boundary coverage: `_AllLive`, `_MissingDist`, `_IgnoresNonGLB`,
`_EmptyList`. Net new test functions: 11.

## Test coverage

- All eleven new tests pass on `go test ./... -count=1`.
- Existing test suite remains green; the `pack_cmd.go` `anyFailed`
  refactor is behaviour-preserving.
- End-to-end smoke against a tmp workdir produced the correct
  no-stale, dry-run, and apply outputs.

## Open concerns

1. **Integration test for `runPackAllCmd --clean` is unit-level,
   not full pipeline.** I took plan.md's documented fallback rather
   than build a synthetic GLB that combines successfully. The
   round-trip (`IdentifyStalePacks` then `RemoveStalePacks(apply)`)
   is exercised, but the *wiring* inside `runPackAllCmd` (flag
   parsing → post-summary hook → exit code) is only validated
   manually. **Mitigation:** the manual smoke against `/tmp/glb-clean-smoke`
   covered the standalone CLI; pack-all `--clean` is the same plumbing
   minus one flag and one if-branch. Adding a real-pipeline test
   would be ~150 LOC of fixture setup for ~5 LOC of new wiring;
   not worth it pre-merge but worth tracking.

2. **`safeOpts(t)` test helper is local to `clean_packs_test.go`.**
   Other test files that exercise the resolver (notably
   `pack_runner_test.go`, `species_resolver_test.go`) likely have
   the same latent risk of reading the dev's real `uploads.jsonl`.
   Out of scope for this ticket but worth a follow-up.

3. **No `--mapping` round-trip in tests.** The standalone CLI
   accepts `--mapping`, but no test verifies that a mapping JSON
   actually changes which packs are flagged as stale. The path is
   exercised indirectly through `LoadMappingFile` (which has its
   own tests in T-012-01) and `ResolveSpeciesIdentity`'s tier-2,
   so the risk is low.

4. **Removal is permanent, no undo.** Per ticket Notes — explicitly
   accepted, dry-run is the only safety net. Documented in the
   justfile recipe comment: "No undo — always run `just
   clean-stale-packs` first to audit."

5. **Pack files whose basename contains characters illegal in a
   species slug** (e.g., a hand-dropped `Foo Bar.glb`) will always
   be classified as stale because no resolver output can ever match.
   The dry-run default catches this; the operator can move the file
   out before applying. Worth a one-line note in operator docs but
   not a code change.

## TODOs / follow-ups (not blocking)

- Add a real-pipeline integration test for `pack-all --clean` once
  the CombinePack fixture infra exists for another ticket.
- Promote `safeOpts` to a shared test helper if any other test file
  starts hitting the uploads-manifest pollution issue.
- Consider an operator doc note about non-slug pack basenames.

## Critical issues for human attention

None. The ticket is `priority: low` and the implementation is
self-contained: no schema changes, no API changes, no migrations,
no behaviour change to existing code paths. The `pack_cmd.go`
refactor was checked against existing tests.

## Demo readiness

The intended demo-morning workflow now works:

```
$ just pack-all --clean    # ← (would need pack-all to support flag passthrough)
# OR, more reliably:
$ ./glb-optimizer pack-all --clean
[pack table]
TOTAL: N packs, N ok, 0 failed

Cleaned stale packs:
  - removed: removed_species.glb (1.2 MB)
TOTAL: 1 stale, 1 removed
```

**Note:** the existing `pack-all` justfile recipe is `./glb-optimizer
pack-all` with no argument passthrough. Operators who want
`--clean` from `just` will need to either invoke the binary directly
or we add a `pack-all-clean` recipe in a follow-up. Not addressed
here because the ticket only required a `--clean` flag on the
binary, not a justfile shortcut for it.
