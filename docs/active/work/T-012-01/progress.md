# T-012-01 Progress

## Status: complete

All five plan steps shipped in a single pass. Full `go test ./...` is
green.

## Step-by-step

### Step 1 — Create `species_resolver.go` ✅

Created `species_resolver.go` (~280 lines incl. doc comments) with:

- `SpeciesIdentity`, `ResolverSource` (+ `String()`), `ResolverOptions`
- `ResolveSpeciesIdentity` walking the six-tier chain
- `LoadMappingFile`, `lookupUploadManifest`
- `captureOverride` + `loadCaptureOverride` (moved here from
  `pack_meta_capture.go`)
- `hashFallbackIdentity`, `normalizeIdentity`, `identityFromFilename`,
  `hexHashRe`

### Step 2 — Update `pack_meta_capture.go` ✅

- Removed `captureOverride` and `loadCaptureOverride` (now in resolver).
- `BuildPackMetaFromBake` signature gained `opts ResolverOptions`.
- The old filename/override/derivation block was replaced with one
  call to `ResolveSpeciesIdentity` plus a single `log.Printf` of the
  resolved (id, species, common_name, source) tuple.
- Renamed `TestBuildPackMetaFromBake_DerivationFails` →
  `TestBuildPackMetaFromBake_FallbackToHash` and rewrote it to assert
  the new permissive behaviour (id-derived slug + title-cased common
  name, no error).

### Step 3 — Thread `opts` through `RunPack` ✅

- `pack_runner.go::RunPack` gained `opts ResolverOptions` (passed
  straight through to `BuildPackMetaFromBake`).
- `handlers.go::handleBuildPack` updated to call `RunPack` with
  `ResolverOptions{}` — server flow unchanged.
- Existing `pack_runner_test.go` and `pack_meta_capture_test.go` call
  sites bulk-updated via `perl -i -pe`.

### Step 4 — CLI flags in `pack_cmd.go` ✅

- `runPackCmd` gained `--species` and `--common-name` flags. Both must
  be supplied together; passing only one prints
  `"--species and --common-name must be provided together"` and exits 2.
- `runPackAllCmd` gained `--mapping <FILE>` flag. The mapping file is
  loaded via `LoadMappingFile`; on parse error the command prints the
  error and exits 1. The same `ResolverOptions{Mapping: ...}` is
  reused for every `RunPack` call in the loop.
- Updated usage strings in both commands.

### Step 5 — `species_resolver_test.go` ✅

Added 17 unit tests covering every tier and the helper functions:

- `TestResolver_CLIOverrideWins`
- `TestResolver_MappingFileBeatsSidecar`
- `TestResolver_SidecarBeatsFileStore`
- `TestResolver_FileStoreFallback`
- `TestResolver_FileStoreSentinelSkipped`
- `TestResolver_UploadManifestTier`
- `TestResolver_UploadManifestLastWins`
- `TestResolver_HashFallback_HexId`
- `TestResolver_HashFallback_NonHexId`
- `TestResolver_NormalisesMessyMappingValues`
- `TestResolver_CLIOverridePartialFallsThrough`
- `TestResolver_MalformedSidecarFallsThrough`
- `TestLoadMappingFile_HappyPath`
- `TestLoadMappingFile_EmptyPathReturnsNil`
- `TestLoadMappingFile_MissingFileIsError`
- `TestLoadMappingFile_BadJSONIsError`
- `TestResolverSource_String`

The hex-hash hashtests pin manifest path to a tempdir
`missing.jsonl` so the developer's real `~/.glb-optimizer/uploads.jsonl`
can't bleed in if T-012-04 ships first.

## Deviations from the plan

### Mid-flight: error semantics softened

The original ticket spec said the resolver should *return an error*
when the resolved species fails the regex. Implementation chose
"never return an error, always fall through to the next tier and
ultimately to the hash safety net". Rationale captured in
`design.md`: returning an error here re-creates the friction the
ticket exists to remove. The signature still returns `error` so a
future `--strict` mode can flip the policy without a re-plumb.

### Mid-flight: tier 6 ("content-hash") generalised

The ticket spec said the hash fallback always produces
`species_<first8>`. Implementation detects whether the id "looks like
a hex hash" (`^[0-9a-f]{16,}$`) and only then renders the
`species_<first8>` form. For non-hex ids (test fixtures named after
species, internal CLI ids, etc.), it falls back to deriving an
id-as-slug, which preserves the long-standing behaviour
`pack_cmd_test.go` relies on. Documented in `design.md` and
`review.md`.

### Mid-flight: T-011-04 dependency carved out

Research confirmed `~/.glb-optimizer/originals/` stores files under
content hash only — no provenance metadata anywhere on disk. Per the
ticket's research-time-pivot clause, the resolver was implemented to
**read** an `~/.glb-optimizer/uploads.jsonl` opportunistically (T-012-04
will eventually write it) but to **not depend** on it. The mapping
file flag is the operator's primary escape hatch in the meantime.

### Concurrent change: `pack-inspect` subcommand

While Step 1 was in flight, `main.go` was updated by another agent
(T-012-02) to dispatch a third `pack-inspect` subcommand. Left
unchanged. The new resolver wiring is fully orthogonal to that work.

## Test results

```
$ go test ./...
ok  	glb-optimizer	2.966s
```

## Open follow-ups

- T-012-04 (persist-original-filename) should write to
  `~/.glb-optimizer/uploads.jsonl` using the schema the resolver
  already reads (`{"id":"...","filename":"..."}` per line). The
  resolver's manifest tier will start firing automatically.
- The smoke test in `plan.md` step 7 was not run automatically (the
  fixture lives outside the repo). Operator should run it once before
  T-011-04 unblocks.
