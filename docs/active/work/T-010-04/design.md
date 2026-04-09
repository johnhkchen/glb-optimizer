# T-010-04 Design — `just pack-all` recipe

## Decision

**Add a `pack` / `pack-all` subcommand to the existing `glb-optimizer`
binary, sharing core logic with `handleBuildPack` via an extracted
`runPack` helper. Justfile recipes shell out to the binary; no
separate `cmd/pack/main.go` package is created.**

## Options considered

### Option A — Subcommand inside the existing binary *(chosen)*

Detect `os.Args[1]` before `flag.Parse()` runs the server. Two
new modes: `glb-optimizer pack <id>` and `glb-optimizer pack-all`.
Both share a `--dir` flag (matching server defaults), construct
the same directory tree, scan the originals dir to seed a
`FileStore`, and call a shared `runPack` helper that wraps the
read-intermediates → BuildPackMetaFromBake → CombinePack → write
sequence already in `handleBuildPack`.

**Pros**
- Single binary; `just build` keeps producing one artifact.
- Refactor naturally extracts the pack logic out of the HTTP
  handler into a helper that the handler also consumes — both
  paths gain the same code coverage from the existing handler
  test suite.
- No new package boundary; `pack_runner.go` lives next to the
  rest of the pack code.
- Justfile recipes are trivial: `./glb-optimizer pack-all`.
- The CLI does not need gltfpack/blender on PATH because the
  subcommand short-circuits before those checks run.

**Cons**
- `main.go` grows a small dispatch shim. Mitigated by keeping the
  per-subcommand logic in `pack_cmd.go`.

### Option B — Separate `cmd/pack/main.go` package

Create `cmd/pack/main.go` as a second binary that imports the
shared pack helpers from package `main`. **Rejected**: package
`main` cannot be imported, so this would force a real refactor to
move pack code into a shared library package, doubling the size of
the change for no demo-day benefit. The ticket explicitly says the
Go subcommand approach should be the simpler option — splitting
packages is the opposite of simple here.

### Option C — Justfile loops over `curl` against `/api/pack/`

Spin up the server, then `for id in ...; do curl ...; done`. The
ticket explicitly disprefers this ("fewer moving parts in the
justfile and easier to debug from a clean shell"). **Rejected**:
adds a server-lifecycle dance to the recipe and hides errors
behind HTTP status codes.

## Architecture

```
+----------------------+        +-------------------+
| main.go              |        | handlers.go       |
|  - dispatch          |        |  handleBuildPack  |
|  - pack/pack-all     |        |  (HTTP wrapper)   |
+----------+-----------+        +---------+---------+
           |                              |
           v                              v
   +----------------+            +-------------------+
   | pack_cmd.go    |----------->| pack_runner.go    |
   |  runPackCmd    |            |  RunPack(...)     |
   |  runPackAllCmd |            |  PackResult{...}  |
   |  printSummary  |            +---------+---------+
   +----------------+                      |
                                           v
                              CombinePack / BuildPackMetaFromBake
```

`RunPack(id, paths, store)` returns a `PackResult{Species, Size,
HasTilted, HasDome, Status, Err}`. The HTTP handler converts
`Status == "oversize"` into 413 (via the existing
`errors.As(*PackOversizeError)` branch); the CLI converts it into
a row in the table and a non-zero exit.

## Public surface

```go
// pack_runner.go
type PackResult struct {
    ID         string
    Species    string  // empty if Status != "ok"
    Size       int64   // 0 if Status != "ok"
    HasTilted  bool
    HasDome    bool    // i.e. volumetric present
    Status     string  // "ok" | "missing-source" | "oversize" | "failed"
    Err        error
}

func RunPack(
    id string,
    originalsDir, settingsDir, outputsDir, distDir string,
    store *FileStore,
) PackResult
```

The handler keeps its current 200/400/404/413/500 mapping by
inspecting `result.Status` and `result.Err`. CLI consumers call
`RunPack` directly and hand the slice of results to
`printPackSummary(w io.Writer, results []PackResult)`.

## Discovery

```go
// pack_cmd.go
func discoverPackableIDs(outputsDir string) ([]string, error) {
    entries, err := os.ReadDir(outputsDir)
    if err != nil { return nil, err }
    var ids []string
    for _, e := range entries {
        n := e.Name()
        if !strings.HasSuffix(n, "_billboard.glb") { continue }
        if strings.HasSuffix(n, "_billboard_tilted.glb") { continue }
        ids = append(ids, strings.TrimSuffix(n, "_billboard.glb"))
    }
    sort.Strings(ids)
    return ids, nil
}
```

Sorted output makes the summary table stable across runs, which
helps the demo operator eyeball regressions and is friendly to
golden-test snapshots.

## Workdir resolution

A small helper `resolveWorkdir(dirFlag string) (string, error)`
moves the `~/.glb-optimizer` default out of `main()` so both the
server and the CLI subcommands share one source of truth. The
helper also runs `os.MkdirAll(distPlantsDir, 0755)` so a fresh
laptop with no prior server run can still call `pack-all`.

## Status taxonomy

| Status            | Trigger                                        | Exit non-zero? |
|-------------------|------------------------------------------------|----------------|
| `ok`              | CombinePack returned bytes, file written       | no             |
| `missing-source`  | `_billboard.glb` removed mid-walk              | yes            |
| `oversize`        | CombinePack returned `*PackOversizeError`      | yes            |
| `failed`          | any other error (read, build meta, write)      | yes            |

`missing-source` cannot happen for `pack-all` (the walker only
returns ids whose intermediate exists), but it *can* happen for
`pack <id>` if a user typoes an id. Treating it as a status row
keeps both code paths uniform.

## Summary table format

```
SPECIES                SIZE       TILTED  DOME    STATUS
achillea_millefolium   1.2 MB     yes     yes     ok
rose_julia_child       4.8 MB     yes     no      ok
big_oak_3              -          -       -       failed
                                                   build meta: ...
TOTAL: 3 packs, 2 ok, 1 failed
```

Single-line per asset; failures get an indented second line with
the truncated error message. Sizes use the existing `humanBytes`.
`-` placeholders for failed rows keep the column alignment.

## Test plan

- Unit test `discoverPackableIDs` against a tempdir containing
  every flavour of intermediate (billboard only, billboard +
  tilted, all three, plus a stray `_billboard_tilted.glb` with
  no plain billboard — must be ignored).
- Unit test `RunPack` for: happy path, missing side intermediate
  → `missing-source`, oversize → `oversize`, normal failure →
  `failed`. The handler tests already cover the same scenarios via
  HTTP — RunPack tests confirm the helper itself works in
  isolation.
- Unit test `printPackSummary` with a fixed slice of results to
  pin the column layout.
- Refactor `handleBuildPack` to call `RunPack` and re-run the
  existing `handlers_pack_test.go` to confirm no regression.

## Out of scope (per ticket)

- Watch mode, parallel packing, dist cleanup. The recipe is
  sequential and does not delete stale `.glb`s in `dist/plants/`.
