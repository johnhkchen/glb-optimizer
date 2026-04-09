# T-012-05 Progress — Implement Phase

## Status: complete

All seven implementation steps from plan.md landed in one continuous
pass. Tests green, build clean, manual smoke against an empty workdir
and an orphan-populated workdir both produced the expected output.

## Step-by-step log

### Step 1 — clean_packs.go ✅

Created. Two functions: `IdentifyStalePacks` and `RemoveStalePacks`,
plus a private `statSize` helper. Algorithm: forward-map id → species
via `ResolveSpeciesIdentity`, build a live set, walk dist for `*.glb`,
flag any whose basename is not in the set. `os.IsNotExist` on distDir
is silently absorbed (returns empty list, nil error).

`go build ./...` clean.

### Step 2 — clean_packs_test.go ✅

Ten unit tests written. All pass on first run except
`TestRemoveStalePacks_MissingFileLogsContinues`, which had an inverted
assertion in the test itself (statErr != nil treated as a failure
when the file was correctly removed). Fixed to check
`!os.IsNotExist(statErr)`.

**Deviation from plan:** added `safeOpts(t)` helper that constructs a
`ResolverOptions` with `UploadManifestPath` pointing at a nonexistent
tempdir path. Without this, the resolver's tier-5 fallback would
silently read the developer's real `~/.glb-optimizer/uploads.jsonl`
and pollute test results with whatever entries happen to be there.
This is a test isolation fix, not a behaviour change.

### Step 3 — clean_cmd.go ✅

Created. `runCleanStalePacksCmd` mirrors `runPackAllCmd`'s structure:
parse flags, resolve workdir, scan files, build store, load mapping,
identify, remove. Returns 0 / 1 / 2 per the standard CLI convention.

### Step 4 — main.go subcommand wiring ✅

Added `case "clean-stale-packs":` after the `pack-inspect` case.
One-line addition matching the existing pattern.

### Step 5 — pack_cmd.go --clean flag ✅

Added `cleanFlag := fs.Bool("clean", false, ...)` to `runPackAllCmd`.
Refactored the post-summary failure detection from "return 1 on first
non-ok" to "compute anyFailed once, then optionally run cleanup, then
return 1 if anyFailed". The behaviour is identical for the
no-`--clean` case (still returns 1 on any failure); the `--clean`
branch only runs cleanup when `anyFailed == false`, otherwise prints
a "Skipped stale-pack cleanup" notice.

The cleanup output is printed *after* the pack summary table and
*before* the final exit, so the operator sees:

```
[pack table]
TOTAL: N packs, ...

Cleaned stale packs:
  - removed: foo.glb (1.2 MB)
TOTAL: 1 stale, 1 removed
```

### Step 6 — pack_cmd_test.go integration test ✅

Took the plan's fallback path: rather than spinning up a real
`runPackAllCmd` with synthetic GLB bytes (which would require a
working CombinePack pipeline and add ~150 LOC of fixture setup), I
added `TestStalePackCleanup_RoundTrip` to `clean_packs_test.go`. This
exercises the same identify-then-remove sequence at unit level
against a synthetic outputs/dist layout, asserts that a stale pack
is removed and a live pack survives.

Net new tests: 11 (the original 10 unit + 1 round-trip). All pass.

### Step 7 — justfile recipes ✅

Added two recipes after the existing `clean-packs` block:
- `clean-stale-packs` — dry-run, default
- `clean-stale-packs-apply` — actually deletes

Two recipes rather than `--` passthrough because the just dialect's
`--` handling is awkward and the explicit `-apply` suffix is a
useful speed bump.

## Verification results

```
$ go build ./...
(clean, no output)

$ go test ./... -count=1
ok  	glb-optimizer	3.068s
```

Manual smoke:
```
$ glb-optimizer clean-stale-packs --dir /tmp/glb-clean-smoke
No stale packs.
(exit 0)

$ touch /tmp/glb-clean-smoke/dist/plants/orphan_{a,b}.glb
$ glb-optimizer clean-stale-packs --dir /tmp/glb-clean-smoke
Stale packs (dry-run, would remove):
  - orphan_a.glb (0 B)
  - orphan_b.glb (0 B)
TOTAL: 2 stale, 0 removed (dry-run)

$ glb-optimizer clean-stale-packs --dir /tmp/glb-clean-smoke --apply
Removing stale packs:
  - removed: orphan_a.glb (0 B)
  - removed: orphan_b.glb (0 B)
TOTAL: 2 stale, 2 removed

$ ls /tmp/glb-clean-smoke/dist/plants/
(empty)
```

All three paths (no-stale, dry-run, apply) behave as designed.

## Deviations summary

| Deviation | Reason |
|-----------|--------|
| `safeOpts(t)` test helper added (not in plan) | Test isolation — prevent reading the dev's real uploads.jsonl |
| Round-trip test placed in clean_packs_test.go, not pack_cmd_test.go | Plan's documented fallback (avoiding heavy CombinePack fixture); same coverage at lower cost |
| Test fix in `MissingFileLogsContinues` | Inverted assertion in the test, not the implementation |

No design or AC deviations.
