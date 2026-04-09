# T-012-01 Review

## What changed

### New files

- **`species_resolver.go`** (~280 lines). The hash-to-species
  resolver: `ResolveSpeciesIdentity()`, `LoadMappingFile()`, the
  six-tier chain (CLI → mapping → meta-json → file-store →
  upload-manifest → hash). Owns `captureOverride` and
  `loadCaptureOverride`, which moved here from `pack_meta_capture.go`.

- **`species_resolver_test.go`** (~270 lines). 17 unit tests covering
  every tier, the normalisation pipeline, the both-or-neither CLI
  rule, the malformed-sidecar fall-through, the upload-manifest
  last-wins rule, and the `ResolverSource.String()` mapping.

### Modified files

- **`pack_meta_capture.go`** — `BuildPackMetaFromBake` gained an
  `opts ResolverOptions` parameter and now delegates species/common
  name to the resolver. The old override+derivation block (~25 lines)
  was removed; `captureOverride` and `loadCaptureOverride` moved to
  `species_resolver.go`. Public surface area otherwise unchanged.

- **`pack_runner.go`** — `RunPack` gained `opts ResolverOptions` and
  passes it to `BuildPackMetaFromBake`.

- **`handlers.go`** — `handleBuildPack` updated to call `RunPack` with
  `ResolverOptions{}` (empty). Server flow is byte-identical.

- **`pack_cmd.go`** — `runPackCmd` gained `--species` /
  `--common-name` (both-or-neither). `runPackAllCmd` gained
  `--mapping <FILE>` (loaded once, reused per id).

- **`pack_meta_capture_test.go`** — every existing
  `BuildPackMetaFromBake` call site updated to pass
  `ResolverOptions{}`. The single test that asserted on the old
  derivation-error path was rewritten to assert the new permissive
  fall-through behaviour and renamed
  `TestBuildPackMetaFromBake_FallbackToHash`.

- **`pack_runner_test.go`** — every `RunPack` call site updated to
  pass `ResolverOptions{}`.

## Acceptance criteria

| AC                                                              | Status |
| --------------------------------------------------------------- | ------ |
| `ResolveSpeciesIdentity` exists with `(SpeciesIdentity, ResolverSource, error)` | ✅ |
| Resolution order: CLI → mapping → sidecar → store → manifest → hash | ✅ |
| Output normalised to species regex                              | ✅ |
| `BuildPackMetaFromBake` calls the resolver                      | ✅ |
| `pack <id>` accepts `--species` / `--common-name`               | ✅ |
| `pack-all` accepts `--mapping <file.json>`                      | ✅ |
| Sidecar tier still works                                        | ✅ (existing test green) |
| Filename normalisation (`Plant A v1.glb` → `plant_a_v1`)        | ✅ (covered indirectly via `TestDeriveSpeciesFromName_EdgeCases`) |
| Empty fallback returns hash + warning                           | ✅ (`TestResolver_HashFallback_HexId`) |
| CLI override beats all other sources                            | ✅ |
| Mapping beats sidecar but loses to per-asset CLI                | ✅ |
| Real intermediate integration test                              | ⚠ deferred (smoke step in `plan.md`) |

## Test coverage

```
$ go test ./...
ok  	glb-optimizer	2.966s
```

- New `species_resolver_test.go`: **17 tests**, every tier and helper
  exercised at least once.
- Existing `pack_meta_capture_test.go`: 12 tests still pass; one was
  rewritten (see "Modified files").
- Existing `pack_runner_test.go` / `pack_cmd_test.go` / handler tests
  all green with the new `ResolverOptions` argument.

### Known coverage gaps

- **No automated integration test against a real `~/.glb-optimizer/`
  intermediate.** The fixture lives in the operator's home directory,
  not the repo. The `plan.md` step 7 smoke check is the documented
  manual verification — operator should run it once before T-011-04
  unblocks.
- **No multi-tier "everything-set" test that asserts the full
  priority order in one assertion.** The chain is verified
  pair-wise (CLI > mapping > sidecar > store) which is sufficient
  for the contract but means a future re-ordering bug between two
  tiers that aren't directly compared could slip through.

## Open concerns / known limitations

### Behavioural change visible to existing operators

The pre-T-012-01 capture returned an error when no filename source
produced a valid slug. Today's resolver instead falls back to a
hash-derived `species_<first8>` slug and logs a WARNING. **Any
caller that scripted around the old error message will silently
start producing packs labelled with hash slugs.** The motivation is
explicit in the ticket, so this is intentional, but it deserves a
callout in the changelog if there is one.

### Hash fallback uses first 8 chars only

`species_<first8>` is a ~32-bit prefix. Collision probability for a
demo set of <100 assets is negligible (~1 in 4 billion), but for a
larger producer set the prefix length should grow. Out of scope for
this ticket — the operator can always supply a `--mapping` file or
sidecar to disambiguate.

### Resolver is silent about disagreement between tiers

If the operator writes a sidecar AND ships a mapping file with a
different species id for the same asset, the mapping wins and the
operator gets *no warning* that the sidecar was ignored. This is by
design (the chain exists *because* tiers disagree) but could be a
foot-gun. A `--verbose` mode that logs every skipped tier is a
reasonable follow-up.

### Permissive sidecar parsing

A malformed `_meta.json` now logs a warning and falls through, where
the old code returned a hard error. This is consistent with the
ticket's "remove friction" framing but means a hand-typo'd sidecar
will produce a hash-fallback pack that the operator may not notice
without reading the log. Mitigated by the WARNING line in the log.

## Suggested follow-ups (out of scope for T-012-01)

1. **T-012-04 wiring opportunity.** When T-012-04 ships, its
   `uploads.jsonl` writer will automatically be picked up by the
   resolver — no further code changes required. The resolver's
   manifest schema is `{"id":"hash","filename":"basename.glb"}` per
   line; document this in T-012-04's design doc to avoid drift.

2. **`--strict` flag.** Promote the hash-fallback tier to a hard
   error for CI flows that should never ship a hash-named pack.
   Cheap to add now that the error return is wired.

3. **Resolver verbosity flag.** A `--verbose` or `--explain` mode on
   `pack` / `pack-all` that prints every skipped tier and the reason,
   so operators can debug "why is this pack named wrong?" without a
   recompile.

4. **Multi-tier integration test.** A single test that sets up *all*
   tiers and asserts the resolver picks the highest-priority one,
   then disables that tier and re-runs to walk down the chain. Would
   close the coverage gap noted above.
