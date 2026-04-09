# T-010-04 Progress

## Step 1 — Extract `RunPack` ✅

Created `pack_runner.go` with `PackResult` struct and `RunPack`
helper. Logic mirrors the prior body of `handleBuildPack`: read
side intermediate (required) → probe optional tilted/volumetric
intermediates → BuildPackMetaFromBake → CombinePack → write to
distDir.

Status taxonomy: `ok` / `missing-source` / `oversize` / `failed`.
HasTilted/HasDome populated up-front so the table reflects what
the operator baked even when a later stage fails.

`go vet` clean.

## Step 2 — Refactor `handleBuildPack` ✅

`handleBuildPack` now delegates the heavy lifting to `RunPack`
and switches on `result.Status` to map back to HTTP codes:

| Status            | HTTP                          |
|-------------------|-------------------------------|
| ok                | 200 + JSON                    |
| missing-source    | 400 + legacy message          |
| oversize          | 413 + oversize.Error()        |
| failed (build meta:) | 400 (preserves legacy mapping) |
| failed (other)    | 500                           |

Deviation from plan: needed a small special case so
"build meta:" failures keep producing 400 instead of 500. The
existing `TestHandleBuildPack_BuildMetaFails`-style tests would
have regressed otherwise. Documented in the handler with a
comment.

`go test ./... -run TestHandleBuildPack` clean — full pre-existing
handler suite still passes against the refactored implementation.

## Step 3 — `pack_runner_test.go` ✅

Five tests, all green:
- `TestRunPack_HappyPath_AllThree`
- `TestRunPack_HappyPath_BillboardOnly`
- `TestRunPack_MissingSide`
- `TestRunPack_Oversize`
- `TestRunPack_BuildMetaFails`

Reuses `packTestEnv` and `makeOversizeGLB` from
`handlers_pack_test.go`. Verifies dist file presence on success,
absence on failure (no leaked files in `dist/plants/`).

## Step 4 — `pack_cmd.go` + `pack_cmd_test.go` ✅

Implemented:
- `resolveWorkdir(dirFlag) (string, error)` — uses the existing
  `DistPlantsDir` constant from T-011-01.
- `discoverPackableIDs(outputsDir) ([]string, error)` — sorted,
  rejects directories and `_billboard_tilted.glb` collisions.
- `printPackSummary(w, results)` — text/tabwriter output, indented
  failure-detail line, TOTAL summary line.
- `runPackCmd(args) int` — single-asset CLI, exit 1 on non-ok.
- `runPackAllCmd(args) int` — discovery walker + summary table,
  exit 1 if any row failed.

Both subcommands use `flag.NewFlagSet(..., ContinueOnError)` so
tests can drive them in-process. They seed the FileStore with
`scanExistingFiles` (the existing helper in `main.go`).

Tests, all green:
- `TestDiscoverPackableIDs_Filters`
- `TestDiscoverPackableIDs_EmptyDir`
- `TestPrintPackSummary_FormatsTable`
- `TestRunPackAllCmd_HappyPath`
- `TestRunPackAllCmd_MixedFailure`
- `TestRunPackCmd_SingleAssetHappy`
- `TestRunPackCmd_BogusIDExits1`

Deviation from plan: the first run of TestRunPackAllCmd_HappyPath
failed because asset ids `asset-1`/`asset-2` derive to species
`asset_1`/`asset_2`, not the human-readable filenames I had
hard-coded. Fixed by picking ids that already match the species
regex (`achillea_millefolium`, etc.) so the slug derivation
short-circuits cleanly and the test does not need a settings
override fixture. Documented in the test helper comment so future
maintainers do not re-introduce the same gotcha.

Linter intervention: while I was writing `pack_runner.go` the
file already had a `WritePack(distDir, species, packBytes)` call
inserted by an editor/linter, pulling in the helper from
`pack_writer.go` (T-011-01). I aligned the rest of the runner
with that helper rather than duplicating the MkdirAll/WriteFile
pair.

## Step 5 — Dispatch + justfile ✅

`main.go` now dispatches `pack` / `pack-all` subcommands at the
very top of `main()`, before `flag.Parse` and the
gltfpack/blender checks. The CLI subcommands therefore work on a
fresh laptop without those binaries on PATH — verified by
running `glb-optimizer pack-all --dir /tmp/empty-glb-test` against
a non-existent directory: it created the workdir tree and printed
the empty-table summary with exit 0.

Justfile gained two recipes (`pack id:` and `pack-all:`), both
depending on `build` so a stale binary cannot poison the demo
recipe.

Note: `main.go` already had the `DistPlantsDir` constant wired in
by T-011-01, so the per-subcommand workdir resolver in
`pack_cmd.go` reuses the same constant rather than hard-coding
`"dist/plants"`.

## Final verification

```
go vet ./...                               # clean
go test ./...                              # PASS
./glb-optimizer pack-all --dir /tmp/empty  # exit 0, empty table
```
