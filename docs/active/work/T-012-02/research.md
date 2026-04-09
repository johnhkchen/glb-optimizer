# T-012-02 — Research

## Ticket summary

Add a `glb-optimizer pack-inspect <species_id_or_path>` subcommand that
prints metadata, variant inventory, and integrity (sha256, size, Pack v1
validation) for a finished asset pack — without rendering it. The command
is the producer agent's "did the pack come out right?" tool and the
sha256 bridge to the T-011-04 handshake protocol.

## Codebase map

### Existing CLI subcommand wiring

`main.go` lines 21–30 short-circuit on `os.Args[1]`:

```
case "pack":     os.Exit(runPackCmd(os.Args[2:]))
case "pack-all": os.Exit(runPackAllCmd(os.Args[2:]))
```

The dispatch happens before the gltfpack/blender PATH probes, so a new
`pack-inspect` subcommand can run on a laptop with neither tool
installed. Pattern to copy verbatim.

`pack_cmd.go` already houses both runPackCmd and runPackAllCmd plus the
shared `resolveWorkdir` helper that creates `~/.glb-optimizer/...`
subdirs (DistPlantsDir included). `pack-inspect` is the natural third
runXxxCmd in the same file.

### Pack v1 metadata model (frozen by E-002)

`pack_meta.go`:

- `PackMeta` struct: format_version, bake_id, species, common_name,
  Footprint{canopy_radius_m, height_m}, Fade{low_start, low_end, high_start}.
- `(PackMeta).Validate()` is the canonical schema check — ticket says
  inspect MUST reuse it, no parallel impl.
- `ParsePackMeta(json.RawMessage) (PackMeta, error)` decodes + validates
  in one call.
- `PackFormatVersion = 1`; `speciesRe = ^[a-z][a-z0-9_]*$`. The species
  regex doubles as the input-classification rule for the new subcommand
  ("looks like a slug → look up dist/plants/{id}.glb, else treat as
  path"). Reusing `speciesRe.MatchString` keeps the rule single-sourced.

### GLB / glTF parsing (pack reader)

`combine.go` defines:

- `glbMagic`, `glbVersion`, `chunkTypeJSON`, `chunkTypeBIN` constants.
- `gltfDoc` — the subset of glTF 2.0 the combine pass touches; relevant
  fields for inspect are `Scenes`, `Nodes`, `Meshes`.
- `gltfScene.Extras map[string]any` — where Pack v1 stamps the
  `plantastic` block via `attachExtras`.
- `gltfNode.Name`, `gltfNode.Children`, `gltfNode.Mesh` — pack inspect
  needs to walk these to discover the `view_side`, `view_top`,
  `view_tilted`, `view_dome` group names and their child counts.
- `readGLB(raw []byte) (*gltfDoc, []byte, error)` — public-enough name,
  takes raw bytes, returns parsed doc + BIN slice. **Inspect's parser is
  literally this function.** No new chunk parser needed.
- `humanBytes(int64) string` — already used by pack_cmd.go for the
  summary table; perfect for inspect's "1.84 MB" rendering.

### Node-tree convention from CombinePack

`routeSideMeshes`, `routeTiltedMeshes`, `routeVolumetricMeshes` build
this tree under `Scenes[0].Nodes[rootIdx]`:

```
pack_root
├── view_side       (children: variant_0, variant_1, …)
├── view_top        (single child leaf, also named "view_top")
├── view_tilted     (children: variant_0, …)
└── view_dome       (children: slice_0, slice_1, …)
```

Inspect can iterate root.Children, look up each child node by name, and
report `len(node.Children)` per group. view_top is the irregular case:
it's a group with exactly one child leaf — render as `1 quad` for
parity with the ticket AC mock-up.

For "average size per variant", pack inspect needs to attribute BIN
bytes to each group. The exact per-variant byte breakdown is hard
without re-walking accessors → bufferViews, but we can give a
**reasonable approximation**: divide the total BIN length minus image
bytes evenly across listed variants. The ticket AC shows
"4 variants × avg 312 KB" so an averaged number is acceptable. The
human-readable value matters for spotting outliers (a 0-byte vs a
2-MB variant), not for byte-perfect accounting.

A more accurate alternative: walk each variant's mesh primitives'
bufferView lengths and sum them. This is feasible because every
gltfMesh primitive has accessor → bufferView indices on hand. The
research recommendation is the more accurate version because it's not
much harder and avoids misleading "averages of garbage."

### Pack metadata extras location

`attachExtras` writes meta to `mc.out.Scenes[0].Extras["plantastic"]`
as a `map[string]any` (via `PackMeta.ToExtras()`). On parse, the doc
comes back with `Scenes[0].Extras` populated. To recover a `PackMeta`
from a freshly parsed pack:

1. `extras := doc.Scenes[doc.Scene].Extras["plantastic"]`
2. Re-marshal that submap to JSON.
3. Pass to `ParsePackMeta` for decode + validate.

This is one new helper (~10 lines) on the inspect side. It uses
`ParsePackMeta` for the validation, satisfying the ticket's "MUST reuse"
constraint.

### Atomic write helper

`pack_writer.go` has `WritePack(distDir, species, pack []byte)` and
calls `writeAtomic` (defined elsewhere — accepted.go references it).
Inspect doesn't write anything, so it doesn't need either.

### Test fixture helpers

`combine_test.go:30` defines `makeMinimalGLB(t, meshNames, perMeshMinY)`
which builds a valid synthetic GLB containing one buffer + N named
meshes. This is the standard fixture across pack_cmd_test.go,
handlers_pack_test.go, pack_runner_test.go. Inspect tests can build a
fixture pack by calling `CombinePack(side, tilted, vol, meta)` directly
on `makeMinimalGLB`-produced inputs — same pattern as
`TestRunPackAllCmd_HappyPath`. No new fixture infra needed.

### Existing CLI tests

`pack_cmd_test.go` shows the CLI test pattern:

- `setupCLIWorkdir(t)` — uses `resolveWorkdir(tempdir)` to create the
  full subdir layout.
- `registerAsset(t, workDir, id, writeSource)` — drops a synthetic
  source GLB and a side billboard intermediate so RunPack can produce
  a real pack.
- Capture stdout via `bytes.Buffer` (not done yet for the pack
  commands — they print to `os.Stdout` directly). For inspect we want
  to capture stdout to assert format. Two options:
  1. Refactor `runPackInspectCmd` to take an `io.Writer` (cleaner).
  2. Replace `os.Stdout` in tests via a swap (yuck).
  Option 1 is the standing pattern in `printPackSummary(w io.Writer, …)`.

## Constraints and assumptions

- Pack v1 schema is frozen (E-002). Inspect must NOT add fields the
  schema doesn't define; it can only surface what's there.
- sha256 must be lowercase hex with no separators. Go's
  `fmt.Sprintf("%x", sha256.Sum256(buf))` produces exactly this.
- Exit non-zero on validation failure, zero on success. Same convention
  as existing CLI commands.
- `--quiet` is for shell pipelines: `<sha256> <bytes> <ok|FAIL>` is the
  smallest useful tuple. Spaces (not tabs) so a `read` loop captures
  three fields.
- `--json` shape needs to be documented in `structure.md`. The simplest
  approach: a single struct with the same fields as the human-readable
  output, marshaled with stable key order via Go's encoding/json
  (which sorts struct fields by declaration order).
- Snapshot test: store a known-good fixture's expected human-readable
  output as a separate file under `testdata/`. Comparing strings is
  brittle on bake_id (timestamp) and sha256 (changes if any byte
  changes), so the snapshot must be of a deterministic fixture
  produced inside the test, not of "a real pack." The path forward is
  to use a fixed `BakeID` ("2026-04-08T00:00:00Z") in the synthetic
  pack and assert the rendered output equals a stored expected text
  block.

## Open questions

1. Should `pack-inspect` accept `--dir` like the other subcommands so
   tests don't need to rebind `~`? **Yes** — copy the
   `resolveWorkdir(*dirFlag)` pattern.
2. When the species id resolves to a non-existent file, should the
   error mention `--dir`? **Yes** — small QoL win for testing.
3. What does `(absent)` look like for missing optional variants? The
   ticket AC mock shows the four groups always rendered. If
   `view_tilted` is absent, print `view_tilted:  (absent)`. Same for
   view_top and view_dome. view_side is required (CombinePack rejects
   nil side input) so no need to render absence for it.
