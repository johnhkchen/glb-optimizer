# T-012-05 Design — Stale Pack Cleanup

## Decision summary

Add two pure Go functions in a new `clean_packs.go`, wire them through
a new `clean-stale-packs` subcommand and a `--clean` flag on `pack-all`,
and add one justfile recipe. Forward-mapping (id → resolved species)
plus set-difference is the algorithm. Dry-run is the default everywhere.

## Algorithm

```
live := {}
for id in discoverPackableIDs(outputsDir):
    ident, _, _ := ResolveSpeciesIdentity(id, outputsDir, store, opts)
    live[ident.Species] = struct{}{}

stale := []
for entry in readdir(distDir) where suffix == ".glb":
    species := strip ".glb"
    if species not in live:
        stale = append(stale, fullpath)

return stale (sorted)
```

This matches the AC ("OR by checking if any intermediate in outputsDir
resolves to that species") exactly. The dist walk is `*.glb`-filtered;
the live set is computed once per call and discarded after the
function returns. No cache, no persisted state.

## Function signatures

```go
// IdentifyStalePacks returns the absolute paths of *.glb files in
// distDir that have no live source intermediate in outputsDir.
//
// "Live source" means: there exists at least one id discoverable in
// outputsDir whose ResolveSpeciesIdentity output has the same species
// slug as the pack file's basename.
//
// store may be nil; opts is passed through to ResolveSpeciesIdentity
// so callers that loaded a --mapping JSON can keep their resolver
// inputs symmetric with pack-all.
//
// Errors: distDir or outputsDir cannot be read. A missing distDir is
// NOT an error — empty stale list, nil error. (A clean install with
// no packs ever built should be a no-op, not a hard fail.)
func IdentifyStalePacks(
    distDir, outputsDir string,
    store *FileStore,
    opts ResolverOptions,
) ([]string, error)

// RemoveStalePacks deletes (or, with dryRun=true, prints what it
// would delete) the supplied paths. It writes one human-readable
// line per file to w; the caller controls whether that is os.Stdout
// or a buffer for tests.
//
// Removal failures are logged to w but never abort the loop. The
// returned error is the join of every per-file failure (errors.Join)
// or nil if all removals succeeded. Dry-run never returns errors.
func RemoveStalePacks(
    w io.Writer,
    stale []string,
    dryRun bool,
) error
```

These mirror the ticket signatures with two minor refinements:

1. `IdentifyStalePacks` takes the resolver inputs (`store`, `opts`)
   directly instead of constructing them internally. This keeps the
   function pure and lets pack-all's `--clean` reuse the same
   ResolverOptions it just used for packing — no risk of the cleanup
   resolver returning a different live set than the pack resolver did
   five seconds earlier.
2. `RemoveStalePacks` takes an `io.Writer` so tests can assert on the
   output without intercepting stdout. The CLI passes `os.Stdout`.

## Why forward-mapping, not reverse

The ticket suggests two paths: (1) reverse-map filename → id via the
resolver, or (2) forward-map id → species and check membership.

Reverse mapping is impossible in general because the resolver is a
many-to-one function: hash fallback strips entropy, and a hand-edited
mapping file can collapse two ids onto the same species. There is no
inverse.

Forward mapping is O(N) with N = number of intermediates, and
membership-check is O(1). For the demo dataset (~30 species) this is
microseconds. Done.

## CLI surface

### Standalone

```
glb-optimizer clean-stale-packs [--dir PATH] [--apply] [--mapping FILE]
```

- `--dir` — workdir override, defaults to ~/.glb-optimizer (matches
  pack-all).
- `--apply` — actually delete. Without it, dry-run (the default).
- `--mapping` — same JSON file as `pack-all --mapping`, so the live
  set computed here matches what pack-all would write. Not required.

Exit code: 0 on success (including "no stale, nothing to do"), 1 on
any removal failure or directory-read error.

### pack-all flag

```
glb-optimizer pack-all [--dir PATH] [--mapping FILE] [--clean]
```

`--clean` triggers `IdentifyStalePacks` + `RemoveStalePacks(dryRun=false)`
**only if** the pack loop completed with `failCount == 0`. The cleanup
output is appended to the summary table (after the TOTAL line).

Why apply-mode under `--clean` and not dry-run-mode: the operator who
opts into `--clean` from `pack-all` is by definition asking for "give
me a clean dist/plants/ now." Dry-run there would be cargo-cult. The
standalone command remains dry-run-by-default for the audit use case.

### justfile

```makefile
# Show what would be removed (dry-run).
clean-stale-packs: build
    ./glb-optimizer clean-stale-packs

# Actually remove. Dangerous; no undo.
clean-stale-packs-apply: build
    ./glb-optimizer clean-stale-packs --apply
```

Two recipes rather than `--` passthrough because the `just` dialect's
`--` handling is verbose and brittle, and the explicit `-apply` suffix
is an extra speed bump for the operator's brain.

## Output format

### Standalone, dry-run

```
Stale packs (dry-run, would remove):
  - old_species_a.glb (1.2 MB)
  - old_species_b.glb (0.9 MB)
TOTAL: 2 stale, 0 removed (dry-run)
```

### Standalone, apply

```
Removing stale packs:
  - removed: old_species_a.glb (1.2 MB)
  - removed: old_species_b.glb (0.9 MB)
TOTAL: 2 stale, 2 removed
```

### pack-all --clean

The pack summary table prints first, then a blank line, then:

```
Cleaned stale packs:
  - removed: old_species_a.glb (1.2 MB)
  - removed: old_species_b.glb (0.9 MB)
```

If `--clean` was set but `failCount > 0`, print a one-liner instead:

```
Skipped stale-pack cleanup: pack-all had N failures.
```

### Empty case

```
No stale packs.
```

(One line, no table, no TOTAL.)

## Rejected alternatives

- **Reverse-resolver lookup.** Discussed above; impossible in general.
- **Mtime-based GC.** Out of scope per ticket; also fragile because
  re-bake without re-pack legitimately leaves an old pack file
  whose mtime is older than the new intermediate.
- **Move-to-trash.** Out of scope per ticket. Adds platform code
  (macOS Trash bindings, Linux gio trash) for negligible value
  versus the dry-run default.
- **Persistent stale-set cache.** Adds complexity, can drift from
  reality. Walking 30 files takes <1 ms.

## Risks and mitigations

| Risk | Mitigation |
|------|-----------|
| Operator runs `--apply` without dry-run first and loses a pack they wanted | Dry-run is default; apply requires explicit flag. Accepted per ticket. |
| `--clean` runs after partial failure and removes good packs | Gated on failCount==0. |
| Resolver returns different slug for same id under different opts (mapping diff between pack and clean) | Standalone CLI accepts the same `--mapping` flag, so the operator can pass the same JSON used at pack time. Pack-all `--clean` reuses its own opts. |
| Pack file basename collides with a species that no longer exists, but operator wants to keep it | Add to mapping or move it out of dist/plants/. Documented in ticket Notes. |
