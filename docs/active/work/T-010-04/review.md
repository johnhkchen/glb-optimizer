# T-010-04 Review — `just pack-all` recipe

## Summary

Adds a Go CLI subcommand pair (`glb-optimizer pack <id>` and
`glb-optimizer pack-all`) plus matching justfile recipes so the
demo operator can refresh `dist/plants/` from a clean shell
without poking the HTTP UI. The shared pack logic was extracted
out of `handleBuildPack` into a reusable `RunPack` helper, so the
HTTP path and the CLI path execute the exact same code.

All acceptance criteria from the ticket are met:

- ✅ `just pack-all` recipe exists and walks `outputs/` for ids
  with `_billboard.glb`.
- ✅ Per-asset combine via the Go subcommand (no HTTP-against-self).
- ✅ Writes `~/.glb-optimizer/dist/plants/{species}.glb`.
- ✅ Prints a summary table: species, size, has_tilted, has_dome,
  status (ok / failed / oversize) plus a TOTAL line.
- ✅ Non-zero exit code if any pack failed or exceeded the 5 MiB cap.
- ✅ Subcommand lives inside the existing binary; no HTTP server
  required to run it.
- ✅ `just pack <id>` packs a single asset.

## Files changed

### Added
- `pack_runner.go` — `PackResult` struct + `RunPack` helper. The
  shared pack pipeline (read intermediates → BuildPackMetaFromBake
  → CombinePack → WritePack) lives here. Errors are encoded in
  `PackResult.Status`/`Err` instead of bubbling up so both the
  HTTP handler and the CLI summary can consume them uniformly.
- `pack_runner_test.go` — five direct unit tests covering happy
  paths (all-three intermediates, billboard-only), missing side
  intermediate, oversize via the existing 6 MiB ballast fixture,
  and BuildPackMetaFromBake failure.
- `pack_cmd.go` — CLI plumbing: `resolveWorkdir`,
  `discoverPackableIDs`, `printPackSummary`, `runPackCmd`,
  `runPackAllCmd`. Uses `flag.NewFlagSet` so each subcommand owns
  its own flag namespace and tests can call them in-process.
- `pack_cmd_test.go` — seven tests covering the discovery walker,
  table formatter, single-asset CLI happy/bogus paths, and the
  pack-all happy/mixed-failure scenarios.

### Modified
- `handlers.go` — `handleBuildPack` body now delegates to
  `RunPack` and switches on `result.Status` to map back to HTTP
  status codes. URL parsing and 404 guards stay in the handler.
  A small `strings.HasPrefix("build meta:")` branch preserves the
  legacy 400-on-build-meta-failure mapping the existing test
  suite asserts.
- `main.go` — subcommand dispatch added at the top of `main()`.
  `glb-optimizer pack <id>` and `glb-optimizer pack-all`
  short-circuit before the gltfpack/blender PATH checks so the
  CLI runs cleanly on a laptop without the bake toolchain.
- `justfile` — two new recipes (`pack id:`, `pack-all:`), both
  depending on `build`.

### Not modified
- `pack_writer.go` (`WritePack`, T-011-01) — reused as-is.
- `combine.go` (`CombinePack`, `PackOversizeError`, T-010-02 /
  T-010-05) — reused as-is.
- `pack_meta_capture.go` (`BuildPackMetaFromBake`, T-011-02) —
  reused as-is.

## Test coverage

| Layer                       | Tests                                |
|-----------------------------|--------------------------------------|
| `RunPack` helper            | 5 cases in `pack_runner_test.go`     |
| `handleBuildPack` regression| Existing `handlers_pack_test.go` (8) |
| `discoverPackableIDs`       | 2 cases in `pack_cmd_test.go`        |
| `printPackSummary`          | 1 column-content snapshot test       |
| `runPackCmd`                | 2 cases (happy + bogus id)           |
| `runPackAllCmd`             | 2 cases (happy + mixed failure)      |

Final state: `go vet ./...` clean, `go test ./...` passes,
manual smoke `./glb-optimizer pack-all --dir /tmp/empty-glb-test`
returns exit 0 with an empty table.

## Coverage gaps

- **No automated end-to-end justfile execution.** The recipes
  shell out to the binary the unit tests already exercise, and
  spinning up `just` inside `go test` would add a toolchain
  assumption to CI. Verified manually instead.
- **No test for `printPackSummary` failure-detail truncation.**
  The 80-char truncation in `truncateOneLine` is exercised
  indirectly by the mixed-failure test (which prints a short
  message), but the long-message path is not pinned. Low risk
  given the existing `*PackOversizeError` already produces
  multi-line output the formatter has to handle.
- **No tests for `resolveWorkdir`.** It is exercised transitively
  by every `runPack*Cmd` test via `setupCLIWorkdir`, so coverage
  is implicit. A direct test would be additive but not
  load-bearing.

## Open concerns

1. **Filename loss across server restarts.** `scanExistingFiles`
   sets `record.Filename = "{id}.glb"`, so after a restart
   `BuildPackMetaFromBake` derives the species slug from the id
   instead of the upload filename. The CLI inherits this — every
   `pack-all` invocation runs the post-restart code path. For
   demo assets with id == species this is fine, but if an asset
   id is something like `01J7XYZ_uploaded.glb` the slug derivation
   will silently produce a junk species and the operator's only
   recourse is to add an override file. Worth noting in the
   demo runbook; a fix belongs in the upload pipeline (persist
   the original filename), not in this ticket.

2. **`humanBytes` precision quirk.** The summary table renders
   sizes like `"1.2 MB"` via the existing helper, which uses
   decimal MB but the cap is in MiB. The TOTAL line therefore
   reports e.g. `"4.7 MB"` for a pack just under the 5 MiB cap.
   Pre-existing — not introduced by this ticket — but operators
   eyeballing the table for "is this close to the cap?" should
   know.

3. **No cleanup of stale `dist/plants/{species}.glb`.**
   Out-of-scope per the ticket. If a species id changes between
   bakes, the old `.glb` lingers until the operator runs
   `just clean-packs`. Acceptable for demo prep.

4. **Sequential only.** No parallel packing. Out-of-scope per
   the ticket; the demo's asset count is small enough that
   parallelism would not move the needle.

## Critical issues for human attention

None. The refactor is behaviour-preserving for the HTTP handler
(verified by re-running the existing handler test suite without
modifications), the new CLI surface has direct unit coverage, and
the justfile recipes are thin shells over the tested binary.

## Suggested follow-ups

- File a small ticket to persist the original upload filename
  across restarts so post-restart `pack-all` does not silently
  fall back to id-derived species slugs.
- Consider adding a `--verbose` flag to `pack-all` that prints
  the full `*PackOversizeError` breakdown after the table for
  rows with `oversize` status.
- Add an integration test that runs the `pack-all` binary as a
  subprocess against a tempdir if/when CI gains a stable Go
  toolchain assumption it can rely on.
