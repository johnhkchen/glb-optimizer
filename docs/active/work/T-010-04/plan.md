# T-010-04 Plan — implementation sequence

Five commits, each independently buildable and testable. Order is
load-bearing: the refactor lands and is verified by the existing
handler tests *before* the CLI surface is added on top.

## Step 1 — Extract `RunPack` into `pack_runner.go`

**What**
Create `pack_runner.go` with `PackResult` and `RunPack`. Copy the
read-side / read-optional / build-meta / combine / write logic
verbatim from `handleBuildPack`. Map errors into `Status` strings
per the design table. Probe the optional intermediates *before*
calling `BuildPackMetaFromBake` so `HasTilted` / `HasDome` are
populated even when meta-build later fails.

**Verify**
`go vet ./...` clean. No tests yet — file is unused.

## Step 2 — Refactor `handleBuildPack` to call `RunPack`

**What**
Replace the body of `handleBuildPack` with the switch shown in
structure.md. Keep the URL-parsing and 404 guards in the handler.
Drop the now-unused imports (`errors`, the `os.IsNotExist`
branch) from `handlers.go` if they are not needed elsewhere
(grep first to confirm).

**Verify**
`go test ./... -run TestHandleBuildPack` must pass without
modification — this is the regression check that the refactor is
behaviour-preserving for every status code the handler reports.

## Step 3 — `pack_runner_test.go` (RunPack unit coverage)

**What**
Add the five tests enumerated in structure.md. They reuse
`packTestEnv` from `handlers_pack_test.go`. The oversize test
borrows the existing fixture pattern that pads a texture to push
past the 5 MiB cap.

**Verify**
`go test ./... -run TestRunPack` passes.

## Step 4 — `pack_cmd.go` + `pack_cmd_test.go` (CLI plumbing)

**What**
- Factor `resolveWorkdir(dirFlag string) (string, error)` out of
  `main.go` into `pack_cmd.go` (or a small `workdir.go` if cleaner).
  Update `main()` to call it. Run server tests to confirm no
  regression in the directory tree.
- Implement `discoverPackableIDs`, `printPackSummary`,
  `runPackCmd`, `runPackAllCmd`. Both subcommands use
  `flag.NewFlagSet("pack", flag.ContinueOnError)` so test code
  can capture errors instead of `os.Exit`.
- Add the five `pack_cmd_test.go` cases.

**Verify**
`go test ./...` clean. Manually: `./glb-optimizer pack-all`
against an empty workdir should print an empty table with
`TOTAL: 0 packs, 0 ok, 0 failed` and exit 0.

## Step 5 — Wire dispatch in `main.go` + add justfile recipes

**What**
- Add the `os.Args` dispatch shim at the top of `main()`.
- Append `pack id:` and `pack-all:` recipes to the justfile.

**Verify**
- `just build` succeeds.
- `./glb-optimizer pack-all` (no args) on a fresh workdir exits
  0 with the empty-table message.
- `./glb-optimizer pack bogus-id` exits 1 with a `missing-source`
  row.
- `just pack-all` runs the recipe end-to-end.
- Final regression: `go test ./...` and `go vet ./...` both
  clean.

## Testing strategy

| Layer            | Coverage                                          |
|------------------|---------------------------------------------------|
| `RunPack`        | Direct unit tests in `pack_runner_test.go`        |
| `handleBuildPack`| Existing `handlers_pack_test.go` (regression)     |
| Discovery        | `TestDiscoverPackableIDs_*`                       |
| Summary table    | `TestPrintPackSummary_FormatsTable`               |
| `runPackAllCmd`  | `TestRunPackAllCmd_*` (in-process, tempdir)       |
| End-to-end CLI   | Manual smoke after step 5; not automated          |

End-to-end justfile execution is left manual because: (a) the
recipe just shells out to the binary the unit tests already
exercise, and (b) spinning up `just` inside `go test` would add
toolchain assumptions to CI for no additional confidence.

## Deviation policy

If `RunPack` extraction reveals that the handler test depends on
behaviour the helper cannot reproduce (e.g. specific error
message wording), the helper wins and the handler test is updated
to match the new shape. Document the deviation in `progress.md`.

## Out of scope reminders

- No watch mode.
- No parallel packing — `RunPack` calls happen sequentially in a
  for-loop. The summary table assumes deterministic ordering.
- No cleanup of stale `dist/plants/{species}.glb` files. If a
  species id changes, the old file lingers — operator concern,
  separate ticket if it bites.
