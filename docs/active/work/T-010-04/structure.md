# T-010-04 Structure — file-level changes

## New files

### `pack_runner.go`

Single helper extracted from `handleBuildPack`. Public API:

```go
type PackResult struct {
    ID        string
    Species   string
    Size      int64
    HasTilted bool
    HasDome   bool
    Status    string // ok | missing-source | oversize | failed
    Err       error
}

// RunPack reads the (up to three) baked intermediates for id,
// calls BuildPackMetaFromBake + CombinePack, and writes the
// resulting pack to {distDir}/{species}.glb. It never panics on
// I/O errors — every failure is encoded in the returned
// PackResult.
func RunPack(
    id string,
    originalsDir, settingsDir, outputsDir, distDir string,
    store *FileStore,
) PackResult
```

Internals mirror `handleBuildPack` lines 1730-1812 verbatim:
read side (required), read tilted/volumetric (optional),
`BuildPackMetaFromBake`, `CombinePack`, `os.WriteFile`. The only
behavioural changes vs the handler:

- Errors are categorised into `Status` strings instead of HTTP
  codes.
- `HasTilted` / `HasDome` are reported even when the pack
  ultimately fails downstream of the reads (informational for the
  table).
- The function takes the store as an argument and does not touch
  global state.

### `pack_runner_test.go`

Reuses `packTestEnv` from `handlers_pack_test.go`. Tests:

1. `TestRunPack_HappyPath_AllThree` — three intermediates present,
   asserts result.Status == "ok", file exists at
   `dist/plants/{species}.glb`, HasTilted/HasDome true.
2. `TestRunPack_HappyPath_BillboardOnly` — only side, both
   optional flags false.
3. `TestRunPack_MissingSide` — no `_billboard.glb` →
   `missing-source`.
4. `TestRunPack_Oversize` — synthesise an intermediate that
   exceeds the cap (the existing
   `TestHandleBuildPack_OversizeReturns413` fixture pattern), assert
   `Status == "oversize"`, no file in `dist/plants/`.
5. `TestRunPack_BuildMetaFails` — register a record but skip the
   source GLB, assert `Status == "failed"`.

### `pack_cmd.go`

CLI entry points and helpers. Public surface limited to
`runPackCmd` and `runPackAllCmd` (called from `main`).

```go
func runPackCmd(args []string) int
func runPackAllCmd(args []string) int

func discoverPackableIDs(outputsDir string) ([]string, error)
func printPackSummary(w io.Writer, results []PackResult)
func resolveWorkdir(dirFlag string) (string, error) // shared with main
```

`runPackCmd` parses `--dir`, requires exactly one positional id,
calls `RunPack`, prints a one-row summary, returns 0 or 1.

`runPackAllCmd` parses `--dir`, walks `outputs/` via
`discoverPackableIDs`, loops over the ids calling `RunPack`,
prints the table, returns 0 if every row is `ok` else 1.

Both subcommands seed the FileStore by reusing `scanExistingFiles`
from `main.go` (already package-internal).

`printPackSummary` uses `text/tabwriter` for column alignment.
The header is `SPECIES\tSIZE\tTILTED\tDOME\tSTATUS`. Failure rows
get a second indented line with the error message (truncated to
~80 chars to keep the table readable for the long
`PackOversizeError` text).

### `pack_cmd_test.go`

1. `TestDiscoverPackableIDs_Filters` — tempdir with side, side +
   tilted, all three, stray tilted-only, plus an unrelated
   `.glb`; assert returned slice is sorted and excludes
   tilted-only and unrelated entries.
2. `TestDiscoverPackableIDs_EmptyDir` — empty outputs/ → empty
   slice, no error.
3. `TestPrintPackSummary_FormatsTable` — feed a fixed slice of
   results, assert tabwriter output contains expected substrings
   (header row, status column, totals line).
4. `TestRunPackAllCmd_HappyPath` — tempdir with two registered
   assets, both with `_billboard.glb`; assert exit code 0 and
   both `.glb` files written into `dist/plants/`.
5. `TestRunPackAllCmd_MixedFailure` — one happy asset, one
   asset whose source GLB is missing; assert exit code 1 and the
   happy asset still wrote successfully.

## Modified files

### `handlers.go`

`handleBuildPack` body shrinks to:

```go
result := RunPack(id, originalsDir, settingsDir, outputsDir, distDir, store)
switch result.Status {
case "ok":
    jsonResponse(w, http.StatusOK, map[string]interface{}{
        "pack_path": filepath.Join(distDir, result.Species+".glb"),
        "size":      result.Size,
        "species":   result.Species,
    })
case "missing-source":
    jsonError(w, http.StatusBadRequest,
        "missing intermediate: build the hybrid impostor first")
case "oversize":
    jsonError(w, http.StatusRequestEntityTooLarge, result.Err.Error())
default: // "failed"
    jsonError(w, http.StatusInternalServerError, result.Err.Error())
}
```

The 404 / id-validation guards stay in the handler (URL parsing
is not RunPack's job). Imports `errors` and the `os.IsNotExist`
branch in the optional-reads block move into `pack_runner.go`.

### `main.go`

Two changes:

1. Subcommand dispatch at the very top of `main()`:
   ```go
   if len(os.Args) > 1 {
       switch os.Args[1] {
       case "pack":
           os.Exit(runPackCmd(os.Args[2:]))
       case "pack-all":
           os.Exit(runPackAllCmd(os.Args[2:]))
       }
   }
   ```
   Placed *before* `flag.Parse()` so the subcommands own their
   own flag set and the gltfpack/blender checks never fire for
   pack mode.

2. The `~/.glb-optimizer` default + directory creation logic is
   factored into `resolveWorkdir(dirFlag string) (string, error)`
   so the CLI subcommands can call the same helper. The server
   path inside `main()` switches to use it. No behavioural
   change for the server.

### `justfile`

Two new recipes appended after `check`:

```
# Pack a single asset by id
pack id: build
    ./glb-optimizer pack {{id}}

# Pack every asset that has a baked side billboard
pack-all: build
    ./glb-optimizer pack-all
```

Both depend on `build` so a stale binary cannot poison the
output.

## Deletions

None.

## Test ordering

`pack_runner_test.go` is purely unit-level and runs against
tempdirs — no server, no goroutines. `pack_cmd_test.go` calls
`runPackAllCmd` directly (in-process) with `--dir` pointed at a
tempdir, so neither test suite touches `~/.glb-optimizer`. Both
slot into the existing `go test ./...` run with no special
setup.

## Backward compatibility

Calling `glb-optimizer` with no arguments still starts the
server. Calling it with `--port=…` (the existing flag) is
unchanged because the dispatch only triggers on positional
subcommand names. The new subcommands consume their own
`flag.NewFlagSet`, so they cannot collide with server flags.
