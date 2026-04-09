# T-012-01 Plan: Implementation Sequence

## Step 0 — Verify the current tree builds

Sanity check before touching anything: `go build ./... && go test ./...`
must be green. (Already known green from prior tickets, but the
resolver work is invasive enough to want a known-good baseline.)

## Step 1 — Create `species_resolver.go`

Self-contained file with:

- `SpeciesIdentity`, `ResolverSource` (+ `String()`), `ResolverOptions`
- `ResolveSpeciesIdentity` walking the 6-tier chain
- `LoadMappingFile`
- `captureOverride` + `loadCaptureOverride` MOVED here from
  `pack_meta_capture.go`
- `uploadManifestEntry` + `lookupUploadManifest`
- `hashFallbackSpecies`

`go build ./...` must pass: this file is standalone and only references
package symbols (`deriveSpeciesFromName`, `titleCaseSpecies`,
`FileStore`) that already exist.

`go test ./...` will FAIL because `pack_meta_capture.go` still defines
`captureOverride` and `loadCaptureOverride`. That's the cue for Step 2.

## Step 2 — Update `pack_meta_capture.go`

- Delete `captureOverride` and `loadCaptureOverride` (they live in the
  resolver file now).
- Add `opts ResolverOptions` to `BuildPackMetaFromBake`.
- Replace the species/common-name block with:
  ```go
  identity, source, _ := ResolveSpeciesIdentity(id, outputsDir, store, opts)
  log.Printf("pack_meta_capture: %s species=%s common_name=%q source=%s",
      id, identity.Species, identity.CommonName, source)
  ```
- Update `pack_meta_capture_test.go` to pass `ResolverOptions{}` at
  every call site, and rename the derivation-fails test to
  `TestBuildPackMetaFromBake_FallbackToHash` asserting the new
  hash-fallback behaviour.

Verify: `go test -run BuildPackMetaFromBake ./...`

## Step 3 — Thread `opts` through `RunPack`

- Add `opts ResolverOptions` to `RunPack`'s signature in
  `pack_runner.go`.
- Pass `opts` to `BuildPackMetaFromBake`.
- Update `handlers.go::handleBuildPack` to call `RunPack` with
  `ResolverOptions{}` (no behaviour change).
- Update `pack_runner_test.go` and `handlers_pack_test.go` to pass
  `ResolverOptions{}` at every call site.

Verify: `go test -run Pack ./...`

## Step 4 — CLI flags in `pack_cmd.go`

- `runPackCmd`: add `--species` and `--common-name` flags. Validate
  that they're provided together (both empty or both set). Build
  `ResolverOptions{CLISpecies, CLICommonName}` and pass to `RunPack`.
- `runPackAllCmd`: add `--mapping` flag. Call `LoadMappingFile` once;
  on error print the parse error and return 1. Build
  `ResolverOptions{Mapping}` and reuse it for every iteration.
- Update `pack_cmd_test.go` to pass `ResolverOptions{}` at existing
  call sites and add new tests for the flag wiring (one for `--species
  --common-name`, one for `--mapping`).

Verify: `go test -run Cmd ./...`

## Step 5 — Add `species_resolver_test.go`

Tests enumerated in `structure.md`. Helper:

```go
func mkResolverFixture(t *testing.T) (outputsDir string, store *FileStore)
```

Build the FileStore on demand inside each test rather than sharing
state. Each test sets up only what its tier needs.

Verify: `go test -run Resolver ./...`

## Step 6 — Full sweep

`go build ./... && go test ./...` — every package must pass. If
anything went wrong it's almost certainly a missed call site update;
the compiler will point at it.

## Step 7 — Smoke test against the real intermediate

`./glb-optimizer pack 0b5820c3aaf51ee5cff6373ef9565935`

Expected:

- The resolver picks the FileStore tier? No — `scanExistingFiles`
  wrote `Filename = {id}.glb`, which is the sentinel and is rejected.
- Then sidecar tier? No file → fall through.
- Then upload manifest? Doesn't exist → fall through.
- Then content hash → `species_0b5820c3` with WARNING logged.
- Pack succeeds and writes `~/.glb-optimizer/dist/plants/species_0b5820c3.glb`.

Then run with a CLI override:

`./glb-optimizer pack --species achillea_millefolium --common-name "Common Yarrow" 0b5820c3aaf51ee5cff6373ef9565935`

Expected: `dist/plants/achillea_millefolium.glb` is written, log line
shows `source=cli-override`.

Then a mapping file:

```
echo '{"0b5820c3aaf51ee5cff6373ef9565935":{"species":"achillea_millefolium","common_name":"Common Yarrow"}}' > /tmp/m.json
./glb-optimizer pack-all --mapping /tmp/m.json
```

Expected: same outcome via `source=mapping-file`.

## Test coverage strategy

| Tier              | Unit (resolver) | Capture-level | Smoke |
| ----------------- | --------------- | ------------- | ----- |
| CLI override      | T1              | —             | yes   |
| Mapping file      | T2              | —             | yes   |
| `_meta.json`      | T3              | existing      | —     |
| FileStore         | T4, T5          | existing      | —     |
| Upload manifest   | T6              | —             | —     |
| Content hash      | T7              | renamed test  | yes   |

The smoke step is operator-validated, not automated, because the
fixture lives in `~/.glb-optimizer/originals/` rather than the repo. If
that becomes a regression risk later, consider a follow-up that copies
the fixture under `assets/` and runs the same flow in CI.

## Commit slicing

Single commit per step (steps 1–5), six total. Step 6 is verification
only; step 7 is the operator smoke. The commit messages should each
quote the step number and a one-line summary so a reviewer can replay
the sequence.
