# T-012-02 — Plan

## Implementation steps

Each step is independently committable. Run `go vet ./... && go test ./...`
after every step.

### Step 1 — types + stubs in pack_inspect.go

Create `pack_inspect.go` with the four types (`PackInspectReport`,
`VariantSummary`, `VariantGroup`) and stub function signatures returning
zero values. This compiles before any logic exists, isolating syntax
errors from logic errors.

**Verification:** `go vet ./...` passes; `go build ./...` produces a
binary.

### Step 2 — main.go subcommand dispatch

Add `case "pack-inspect": os.Exit(runPackInspectCmd(os.Args[2:]))` to
the existing switch in main.go. Stub returns 1 with a "not implemented"
message — proves the dispatch wiring before logic lands.

**Verification:** `./glb-optimizer pack-inspect foo` exits 1 with the
stub message and does not start the HTTP server.

### Step 3 — InspectPack pipeline

Implement the read → sha256 → parse → extract meta → validate →
summarize variants chain. Returns a fully-populated
`*PackInspectReport`. Schema validation failures populate
`report.Validation` and set `report.Valid = false`; they do not return
a Go error. GLB-parse failures and I/O errors do return errors.

Helpers landed in this step:
- `extractPackMeta(doc)` — re-marshals `Scenes[Scene].Extras["plantastic"]`
  and runs through `ParsePackMeta`.
- `summarizeVariants(doc)` — walks pack_root.Children, finds groups by
  name, calls variantBytes per group.
- `variantBytes(doc, leafMeshes)` — sums unique bufferView byte
  lengths referenced by the given meshes' primitive accessors. Uses
  a `map[int]bool` to dedupe bufferView indices.

**Verification:** unit tests 1, 2, 3, 4, 12 pass.

### Step 4 — renderers

Implement renderHuman, renderJSON, renderQuiet. renderHuman uses
fmt.Fprintf with fixed-width left-justified labels (no tabwriter — the
labels are already fixed-width and tabwriter adds noise for static
layouts).

Format reference (matches ticket AC):

```
pack: <basename>
  path:        <abs path>
  size:        <human> (<bytes> bytes)
  sha256:      <hex>
  format:      Pack v1
  bake_id:     <bake_id>

metadata
  species:           <slug>
  common_name:       <name>
  canopy_radius_m:   <float>
  height_m:          <float>
  fade.low_start:    <float>
  fade.low_end:      <float>
  fade.high_start:   <float>

variants
  view_side:    N variants × avg <human>
  view_top:     1 quad × <human>            (or "(absent)")
  view_tilted:  N variants × avg <human>    (or "(absent)")
  view_dome:    N slices × avg <human>      (or "(absent)")

validation: OK | <error message>
```

**Verification:** unit tests 5, 6, 7 pass; binary smoke test on a real
pack file from `~/.glb-optimizer/dist/plants/` produces output that
"looks right" (judgment call until snapshot lands in step 6).

### Step 5 — runPackInspectCmd full implementation

Replace the stub with full implementation:

1. flag parse: `--dir`, `--json`, `--quiet` (+ usage on -h or no arg).
2. Mutual-exclusion check on `--json` + `--quiet` → exit 2.
3. resolveInspectTarget(arg, workDir).
4. InspectPack(path).
5. Pick renderer based on flags.
6. Return 0 if `report.Valid`, else 1.

resolveInspectTarget logic:
- If `arg` matches `speciesRe`: target = `filepath.Join(workDir, DistPlantsDir, arg+".glb")`.
- Else: target = `arg` (with `~` expanded if it starts with `~/`).
- Stat target; if missing, return error mentioning both the resolved
  path and (when --dir was implicit) the `--dir` flag.

Add the test helper `runPackInspectCmdW(args []string, w io.Writer) int`
so tests can capture stdout. The exported `runPackInspectCmd` becomes:

```go
func runPackInspectCmd(args []string) int {
    return runPackInspectCmdW(args, os.Stdout)
}
```

**Verification:** integration tests 5, 6, 7, 8, 9, 10 pass.

### Step 6 — snapshot test + testdata

Create `testdata/pack_inspect_human.txt` with the expected human
output for a deterministic fixture: `bake_id="2026-04-08T00:00:00Z"`,
species `salvia_officinalis`, single side variant `s0`, no tilted, no
dome. Calculate the sha256 by running the test once and capturing
stdout — this is the only place in the test suite where the snapshot
needs a one-time bootstrap.

Snapshot test:
```go
func TestRenderHuman_Snapshot(t *testing.T) {
    // build deterministic pack
    side := makeMinimalGLB(t, []string{"s0"}, nil)
    meta := PackMeta{
        FormatVersion: 1, BakeID: "2026-04-08T00:00:00Z",
        Species: "salvia_officinalis", CommonName: "Garden Sage",
        Footprint: Footprint{0.45, 0.62},
        Fade: FadeBand{0.30, 0.55, 0.75},
    }
    raw, err := CombinePack(side, nil, nil, meta)
    if err != nil { t.Fatal(err) }

    path := filepath.Join(t.TempDir(), "salvia_officinalis.glb")
    os.WriteFile(path, raw, 0644)

    report, err := InspectPack(path)
    if err != nil { t.Fatal(err) }

    var buf bytes.Buffer
    renderHuman(&buf, report)
    got := buf.String()

    // Strip volatile lines (path, sha256) before comparing — they
    // depend on tempdir and exact byte layout. Snapshot only stable
    // fields.
    got = stripVolatile(got)

    expected, _ := os.ReadFile("testdata/pack_inspect_human.txt")
    if got != string(expected) {
        t.Errorf("snapshot mismatch:\n--- got ---\n%s\n--- want ---\n%s", got, expected)
    }
}
```

`stripVolatile` replaces the `path:` and `sha256:` value portions with
fixed `<PATH>` and `<SHA256>` placeholders. The snapshot file uses the
same placeholders.

**Verification:** snapshot test 11 passes.

### Step 7 — full suite + manual smoke

`go vet ./... && go test ./...`. Then build the binary and run
`./glb-optimizer pack-inspect <some_existing_species_id>` against the
real `~/.glb-optimizer` if a pack exists; eyeball the output. (Not a
gating test — manual confirmation only.)

## Testing strategy

- **Unit:** parser pipeline, variantBytes dedup, renderer formatting,
  flag parsing, exit codes, mutual exclusion.
- **Integration (in-process):** runPackInspectCmd against a real
  workdir produced by setupCLIWorkdir + runPackCmd. No subprocess
  spawning — Go unit-test harness handles it.
- **Snapshot:** one stable fixture, volatile fields masked.
- **Manual smoke:** binary against real ~/.glb-optimizer.

No new test infrastructure. setupCLIWorkdir, registerAsset,
makeMinimalGLB are reused as-is.

## Risk register

| Risk                                              | Mitigation                              |
| ------------------------------------------------- | --------------------------------------- |
| variantBytes double-counts bufferViews            | Dedup map; explicit unit test (test 12) |
| Snapshot test brittle on Go map iter order        | Walk groups in fixed order, not by map  |
| ~/.glb-optimizer non-existent on first run        | resolveWorkdir already mkdirs           |
| --json + --quiet ambiguous if both present        | Exit 2 with usage message               |
| Path arg with spaces                              | os/exec args are slices, not strings    |
| ParsePackMeta wraps "pack_meta:" prefix           | Surfaced verbatim in Validation field   |

## Out-of-scope reminders

- No pack-diff (future ticket).
- No listing (use `ls`).
- No rendering preview (production preview owns this).
- No metadata editing.
- No caching.
