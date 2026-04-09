# T-012-02 — Structure

## File footprint

```
NEW  pack_inspect.go               (~280 LOC)
NEW  pack_inspect_test.go          (~260 LOC)
NEW  testdata/pack_inspect_human.txt   (snapshot expected output)
MOD  main.go                       (+3 lines: case "pack-inspect")
```

No changes to combine.go, pack_meta.go, pack_writer.go, pack_runner.go,
pack_cmd.go, or any handler.

## pack_inspect.go layout

### Section 1 — public types (~50 LOC)

```go
// PackInspectReport is the structured outcome of a pack-inspect run.
// It is the source of truth for both the JSON output (--json) and the
// rendered human-readable output. Field declaration order controls
// JSON key order.
type PackInspectReport struct { ... }

type VariantSummary struct { ... }
type VariantGroup struct { ... }
```

### Section 2 — entry point (~70 LOC)

```go
// runPackInspectCmd implements `glb-optimizer pack-inspect <arg>`.
// Returns 0 on a valid Pack v1 file, 1 on any read/parse/validation
// failure, 2 on flag parse failure.
func runPackInspectCmd(args []string) int { ... }

// resolveInspectTarget classifies arg as either a species id (matches
// speciesRe) or a path. Species ids are looked up under
// {workDir}/dist/plants/{id}.glb. Paths are taken as-is, with ~
// expansion and absolute resolution.
func resolveInspectTarget(arg, workDir string) (string, error) { ... }
```

### Section 3 — inspect pipeline (~80 LOC)

```go
// InspectPack reads a pack file and returns a populated report. The
// caller decides how to render it. Returns a non-nil error only on
// I/O failure or unparseable GLB; schema-validation failures are
// surfaced via report.Valid=false and report.Validation containing
// the error message.
func InspectPack(path string) (*PackInspectReport, error) { ... }

// extractPackMeta pulls scenes[scene].extras.plantastic out of a
// parsed gltfDoc and runs it through ParsePackMeta. Returns a nil
// meta + descriptive error if the extras block is absent or
// malformed.
func extractPackMeta(doc *gltfDoc) (*PackMeta, error) { ... }

// summarizeVariants walks the pack_root → group → leaf node tree and
// builds a VariantSummary by group name. Side is required; the rest
// are optional and stay nil if absent.
func summarizeVariants(doc *gltfDoc) VariantSummary { ... }

// variantBytes sums the unique bufferView byte lengths referenced by
// every primitive of every mesh under the given leaf nodes. Image
// payloads (Images[*].BufferView) are excluded so the per-variant
// number is mesh+index data only.
func variantBytes(doc *gltfDoc, leafMeshes []int) int64 { ... }
```

### Section 4 — renderers (~70 LOC)

```go
// renderHuman writes the multi-block terminal-friendly layout shown
// in the ticket AC. Section order: header, metadata, variants,
// validation. Group rows print in fixed order: side, top, tilted, dome.
func renderHuman(w io.Writer, r *PackInspectReport) { ... }

// renderJSON writes a single JSON document representing r. The
// encoder uses Indent("", "  ") so the output is human-readable too,
// but the structure is the contract.
func renderJSON(w io.Writer, r *PackInspectReport) error { ... }

// renderQuiet writes a single space-separated line:
//   <sha256> <size_bytes> <OK|FAIL>
// for shell pipelines. No trailing comma, no JSON. Always one line.
func renderQuiet(w io.Writer, r *PackInspectReport) { ... }
```

## Public surface added

| Symbol                       | Visibility | Purpose                                         |
| ---------------------------- | ---------- | ----------------------------------------------- |
| `PackInspectReport`          | exported   | Tests in other files may construct/inspect.     |
| `VariantSummary`             | exported   | Field of PackInspectReport.                     |
| `VariantGroup`               | exported   | Field of VariantSummary.                        |
| `InspectPack(path)`          | exported   | Reusable from future ticket (e.g. pack-diff).   |
| `runPackInspectCmd(args)`    | unexported | Subcommand entry point — only main.go calls.    |
| `resolveInspectTarget`       | unexported | Internal classifier.                            |
| `extractPackMeta`            | unexported | Internal helper.                                |
| `summarizeVariants`          | unexported | Internal helper.                                |
| `variantBytes`               | unexported | Internal helper.                                |
| `renderHuman/JSON/Quiet`     | unexported | Renderers.                                      |

The exported types make the report introspectable from tests in the
same package without leaking too much; future ticket T-083-05
(post-USB-drop verification) can reuse `InspectPack` to fetch the
sha256 line in one call.

## main.go change

Single case branch added to the existing switch:

```go
switch os.Args[1] {
case "pack":
    os.Exit(runPackCmd(os.Args[2:]))
case "pack-all":
    os.Exit(runPackAllCmd(os.Args[2:]))
case "pack-inspect":               // NEW
    os.Exit(runPackInspectCmd(os.Args[2:]))
}
```

## CLI surface

```
glb-optimizer pack-inspect [--dir PATH] [--json|--quiet] <species_id_or_path>

Flags:
  --dir PATH    working directory (default: ~/.glb-optimizer)
  --json        emit machine-readable JSON to stdout
  --quiet       emit a single-line "<sha256> <bytes> <OK|FAIL>"

Exit codes:
  0   pack is valid Pack v1
  1   read / parse / validation failure
  2   bad CLI args
```

## Test plan (file-level)

`pack_inspect_test.go` will contain:

1. `TestInspectPack_HappyPath` — build a fixture pack via CombinePack
   on makeMinimalGLB inputs (side + tilted + vol), call InspectPack on
   the on-disk file, assert all report fields are populated and
   report.Valid is true.
2. `TestInspectPack_AbsentOptionalVariants` — side-only pack. Assert
   `Variants.Side != nil`, `Variants.Top == nil`,
   `Variants.Tilted == nil`, `Variants.Dome == nil`, no crash.
3. `TestInspectPack_TruncatedFile` — write 4 bytes of garbage. Assert
   InspectPack returns a non-nil error and the error mentions GLB
   parsing.
4. `TestInspectPack_MissingMetadataBlock` — build a doc with no
   `extras.plantastic`, write as GLB, call InspectPack. Assert error
   mentions metadata extraction.
5. `TestRunPackInspectCmd_HappyPathStdout` — set up a workdir,
   produce a pack via runPackCmd, then call runPackInspectCmd with
   the species id. Capture stdout, assert it contains "pack:",
   "sha256:", and "validation: OK".
6. `TestRunPackInspectCmd_JSONFlag` — same setup, `--json` flag.
   Decode stdout into a `PackInspectReport`, assert fields.
7. `TestRunPackInspectCmd_QuietFlag` — same setup, `--quiet`. Assert
   stdout is exactly one line of three space-separated fields, third
   field is `OK`.
8. `TestRunPackInspectCmd_NonExistentSpecies` — call with a species
   id that has no pack file, assert exit 1.
9. `TestRunPackInspectCmd_BadFlagsExit2` — `--json --quiet` together,
   assert exit 2.
10. `TestRunPackInspectCmd_PathArg` — call with an absolute path
    rather than a species id; assert it works.
11. `TestRenderHuman_Snapshot` — build a fixture with a fixed
    BakeID, render to a buffer, compare against
    `testdata/pack_inspect_human.txt`. The fixture must produce
    deterministic bytes (fixed bake_id, deterministic mesh count).
12. `TestVariantBytes_DedupesSharedBufferViews` — synthetic doc with
    two primitives sharing the same accessor; assert variantBytes
    counts the bufferView once.

Stdout capture is handled by passing an `io.Writer` into a private
helper `runPackInspectCmdW(args, w)`. The exported
`runPackInspectCmd` is a 1-line wrapper that passes os.Stdout. Same
shape we'd want for runPackCmd if it were retrofitted.

## testdata/

```
testdata/
  pack_inspect_human.txt    # snapshot of a deterministic fixture's
                            # human-readable output. Regenerate with
                            # `go test -run TestRenderHuman_Snapshot
                            # -update` (only if we add the flag — for
                            # v1 the file is hand-maintained).
```

## Ordering of changes (for the Plan phase)

1. Types + skeleton functions in pack_inspect.go (compiles, all funcs
   return zero values).
2. Wire main.go subcommand dispatch.
3. Implement InspectPack pipeline (read → parse → extract → validate
   → summarize).
4. Implement renderHuman / renderJSON / renderQuiet.
5. Add tests, fixing any latent bugs found.
6. Add testdata snapshot file once the renderer is stable.
7. `go vet ./... && go test ./...`
