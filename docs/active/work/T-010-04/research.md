# T-010-04 Research — `just pack-all` recipe

## Goal recap

Demo prep needs a one-shot CLI that walks `outputs/`, packs every
asset that has a baked side billboard, drops the result in
`dist/plants/{species}.glb`, and prints a per-asset status table. The
ticket prefers a Go subcommand over an HTTP-against-self approach.

## Existing pack pipeline (where the work lives)

The `/api/pack/:id` endpoint added in T-010-03 already does the
combine work end-to-end. Reading `handlers.go:1713-1815`:

1. Resolve `FileRecord` from the in-memory store (`store.Get(id)`).
2. Read `{outputsDir}/{id}_billboard.glb` (required).
3. Read `_billboard_tilted.glb` and `_volumetric.glb` (optional —
   missing → nil; CombinePack handles nil natively).
4. Call `BuildPackMetaFromBake(id, originalsDir, settingsDir,
   outputsDir, store)` → `PackMeta` (T-011-02).
5. Call `CombinePack(side, tilted, volumetric, meta)` → bytes
   (T-010-02). Oversize is reported as `*PackOversizeError`
   (T-010-05) and surfaced as 413.
6. Write bytes to `{distDir}/{meta.Species}.glb`, return JSON.

The CLI just needs to perform steps 2–6 in a loop without the HTTP
shell. Step 1's store dependency is the main wrinkle: `FileStore`
holds the original filename, which `BuildPackMetaFromBake` needs
to derive a species slug when no override is present.

## How the store gets populated

`main.go:scanExistingFiles` (around line 175) walks `originalsDir`
for `*.glb`, registers a `FileRecord` for each id, then probes
`outputsDir` and `settingsDir` to set status flags. Crucially: the
filename is *not* preserved across restarts — `record.Filename` is
set to `e.Name()` (the on-disk file `{id}.glb`). That means after a
fresh process start, `BuildPackMetaFromBake` will fall through to
`deriveSpeciesFromName(filename)` operating on the id, which
strips leading digits and lowercases — fine for ids that already
look like species, but it does mean a CLI invocation will always
take the "derived from id" branch unless an explicit override is
present in `settings/`.

This is the same behaviour the HTTP server gets after a restart, so
the CLI is not introducing a new failure mode. Worth calling out in
the design doc as a known property, not a bug.

## Discovery surface for "every packable asset"

The ticket's selection rule: **an asset is packable iff
`{id}_billboard.glb` exists in `outputs/`**. Tilted and volumetric
are optional. So the discovery walker:

- Reads `outputsDir`.
- Filters entries matching `*_billboard.glb` (literally that suffix —
  must reject `_billboard_tilted.glb`).
- Strips the `_billboard.glb` suffix to recover the asset id.

The `_billboard_tilted.glb` collision means a naive `HasSuffix` will
catch tilted entries too. Either anchor with
`strings.HasSuffix(name, "_billboard.glb") && !strings.HasSuffix(name, "_billboard_tilted.glb")`
or do the suffix check after rejecting names that contain `_tilted`.
First option is unambiguous and shorter.

## Existing CLI surface in `main.go`

`main.go` is currently a single-mode binary. `flag.Parse` runs
unconditionally, then the server starts. Two flags exist: `--port`
and `--dir`. There is no subcommand dispatch yet.

`gltfpack` discovery (`exec.LookPath`) and Blender detection both
hard-fail or warn early. Pack mode does **not** need either: combine
is pure-Go byte manipulation. The CLI subcommand should skip those
checks so `just pack-all` can run on the demo laptop without those
binaries on `$PATH`.

## Working directory layout

`main.go:48-66` defines:

```
$workDir/
  originals/
  outputs/
  settings/
  tuning/
  profiles/
  accepted/
  dist/plants/        # T-010-03: pack output root
```

Default `workDir = ~/.glb-optimizer`, overridable via `--dir`. The
CLI subcommand needs the same default + flag so the recipe can be
pointed at a non-default working dir if needed.

## Existing test fixtures we can lean on

`handlers_pack_test.go` defines `packTestEnv` with helpers
`registerSource`, `writeIntermediate`, `makeMinimalGLB` (from
`combine_test.go`). These build a hermetic tempdir tree with the
exact directory layout and a synthetic-but-parseable source GLB. A
`pack_cmd_test.go` can reuse them: build a tempdir with N
intermediates, run the pack-all entry point, assert files landed in
`dist/plants/` with the right species and that the summary captured
each asset's status.

## Output formatting precedent

`combine.go:181 humanBytes(n int64) string` already renders bytes as
"5.2 MB" / "850 KB" / "120 B". The summary table should reuse it for
the size column. `*PackOversizeError.Error()` already produces a
multi-line breakdown — for the table we want a single short line, so
the table prints `oversize` and a `--verbose` mode (or post-table
detail block) can dump the full breakdown for the failed entries.

## Justfile state

The current `justfile` is 28 lines: `default`, `build`, `run`,
`serve port=`, `down`, `clean`, `check`. No recipes touch
`dist/plants/` yet, and there is no precedent for recipes that take
a positional argument — `serve port="8787"` is the only template.
Adding `pack id` and `pack-all` follows the same `recipe arg=:` and
`recipe:` patterns.

## Exit-code expectations

The ticket asks for non-zero exit if any pack fails or exceeds the
cap. The natural shape: collect a `[]packResult{ID, Species, Size,
HasTilted, HasDome, Status, Err}`, print the table, then
`os.Exit(1)` if any row has Status != "ok". `Status == "oversize"`
counts as a failure (it must trip CI if a real bake regresses past
the cap).

## Constraints surfaced

- The CLI must not require gltfpack/blender.
- The CLI must use the same `workDir` resolution as the server so
  outputs from a `just run` session are visible.
- Discovery must distinguish `_billboard.glb` from
  `_billboard_tilted.glb`.
- `BuildPackMetaFromBake` requires a populated `*FileStore` even
  though the CLI does not use the store after the call — easiest
  path is to call the existing `scanExistingFiles` and reuse the
  same store the server would build.
- `dist/plants/` must exist before write; `os.MkdirAll` is cheap
  and matches what `main.go` already does on startup.
- Sequential is fine (out of scope: parallel).
