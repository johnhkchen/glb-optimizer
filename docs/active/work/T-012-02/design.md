# T-012-02 — Design

## Decision summary

1. **Single new file `pack_inspect.go`** holds the parser + renderer +
   CLI entry. Sibling to `pack_cmd.go`. No changes to combine.go,
   pack_meta.go, or pack_writer.go.
2. **Reuse `readGLB` from combine.go** for chunk parsing. No new
   parser. The "MUST reuse" AC is satisfied because there is no
   parallel implementation.
3. **Reuse `ParsePackMeta` for validation**. The inspect pipeline
   re-marshals `Scenes[0].Extras["plantastic"]` and runs it through
   ParsePackMeta. Validation rules and error messages stay in
   pack_meta.go where the rest of the system reads them.
4. **One `PackInspectReport` struct** carries both human-readable and
   JSON output state. Render functions take a `*PackInspectReport`
   and an `io.Writer`. Tests assert against the report struct
   (structural) and against rendered text (snapshot).
5. **Per-variant byte attribution by mesh-primitive walk**, not by
   averaging. More accurate, only ~30 lines, surfaces real outliers.
6. **Subcommand entry point `runPackInspectCmd(args []string) int`**
   matching the existing runPackCmd / runPackAllCmd shape, dispatched
   from main.go's `os.Args[1]` switch. The function takes an
   `io.Writer` for stdout to make tests hermetic — wrapper from main
   passes `os.Stdout`.

## Options considered

### Option A — new file `pack_inspect.go` (chosen)

Self-contained. ~250 LOC including helpers. No risk of stomping
combine.go's tightly-tested parser. Easy review boundary.

### Option B — extend pack_cmd.go

Pro: collocates all CLI subcommands. Con: pack_cmd.go is already 215
LOC and inspect adds another 200; the file becomes a grab bag. The
existing split between pack_cmd.go (CLI) and pack_runner.go (logic)
also breaks if we put inspect's logic in pack_cmd.go. Rejected.

### Option C — split into pack_inspect_cmd.go + pack_inspect.go

Mirrors pack_cmd.go / pack_runner.go. Pro: matches existing pattern
exactly. Con: inspect's "logic" is small (parse + extract + validate
+ render), and an artificial split would create two ~100-line files
each with file-level docstrings. Rejected as over-engineering for
the size — but if inspect grows (e.g. a future `pack-diff`), the
split is the natural follow-up.

## Key design decisions

### D1. sha256 is computed over the raw on-disk bytes, not over the
parsed-then-reserialized doc.

The handshake protocol in T-011-04 records the sha256 of the file as
shipped over USB. The reserialized form differs from the on-disk
form (JSON whitespace, key order), so any reserialize-then-hash
approach would produce a sha256 that does not match the bytes the
consumer verifies. Computed early, before parsing.

```go
raw, err := os.ReadFile(path)
sum := sha256.Sum256(raw)
hexSum := fmt.Sprintf("%x", sum)  // lowercase hex, no separators
```

### D2. Argument classification uses `speciesRe` from pack_meta.go.

```go
if speciesRe.MatchString(arg) {
    // species id → look up dist/plants/{arg}.glb
} else {
    // path → use as-is, expand ~ and resolve relative
}
```

Single source of truth for what counts as a species id. If E-002
ever loosens or tightens the regex, inspect tracks automatically.

A bare filename like `achillea.glb` (no slash, ends in .glb) would
not match `speciesRe` because of the dot. That's actually correct:
treat it as a path. The dot + .glb suffix is the disambiguator, and
operators who want to inspect a pack by id type the id, not the
filename.

### D3. PackInspectReport is the single source of truth.

```go
type PackInspectReport struct {
    Path        string         `json:"path"`
    Size        int64          `json:"size_bytes"`
    SizeHuman   string         `json:"size_human"`
    SHA256      string         `json:"sha256"`
    Format      string         `json:"format"`        // "Pack v1" | "unknown"
    BakeID      string         `json:"bake_id"`
    Meta        *PackMeta      `json:"metadata,omitempty"`
    Variants    VariantSummary `json:"variants"`
    Validation  string         `json:"validation"`    // "OK" | error message
    Valid       bool           `json:"valid"`
}

type VariantSummary struct {
    Side    *VariantGroup `json:"view_side,omitempty"`
    Top     *VariantGroup `json:"view_top,omitempty"`
    Tilted  *VariantGroup `json:"view_tilted,omitempty"`
    Dome    *VariantGroup `json:"view_dome,omitempty"`
}

type VariantGroup struct {
    Count    int   `json:"count"`
    AvgBytes int64 `json:"avg_bytes"`
}
```

The `--json` output is `json.NewEncoder(w).Encode(report)`. The
human-readable output is a render function that walks the same
struct. Snapshot tests compare against the rendered text; structural
tests compare against the struct.

### D4. Variant-byte attribution

For each variant leaf node:

```
node.Mesh → mesh
mesh.Primitives → []primitive
primitive.Attributes (POSITION, NORMAL, TEXCOORD_0, …) → accessor index
primitive.Indices → accessor index
accessor.BufferView → bufferView
bufferView.ByteLength
```

Sum bufferView.ByteLength across all (deduped) bufferViews referenced
by the primitives of all meshes under a group. Dedup is necessary
because multiple primitives in a mesh can share an accessor and
multiple accessors can share a bufferView. The avg per variant is
`sum / variantCount`.

Helper: `variantBytes(doc *gltfDoc, leafNodes []int) int64`.

This deliberately ignores image bytes — they're shared across
variants and counting them per-variant inflates everything. Variant
size is "mesh + index data", which is what the operator wants when
deciding whether the bake is too dense.

### D5. Validation: prints all the errors, not just the first.

`PackMeta.Validate()` returns the **first** failing field. For
inspect, the operator wants to see *all* the violations at once so
they don't fix one and re-run only to hit the next. Two paths:

(a) Walk the validation rules locally, collecting errors. Duplicates
the rules (rejected by the ticket).

(b) Run `Validate()` once. If it returns an error, print that error.
Operator fixes it, re-runs, gets the next. Acceptable for v1 — the
common failure modes (missing common_name, fade out of order) are
single-rule violations in practice.

**Chosen: (b).** No duplication. The "single error at a time" UX is
the same as combine's HTTP 422 path, so operators are already
trained on it. If users complain, switch to (a) later.

### D6. Output mode flags are mutually exclusive.

`--json` and `--quiet` cannot be combined. If both are passed, exit
with usage error. JSON is for scripting, quiet is for shell pipes —
they don't compose. Documented in the usage line.

### D7. Exit codes

| Outcome                          | Exit |
| -------------------------------- | ---- |
| File loaded, valid Pack v1       | 0    |
| File missing                     | 1    |
| Read error                       | 1    |
| Not a GLB / corrupt chunks       | 1    |
| Pack v1 validation failure       | 1    |
| Bad CLI args (parse error)       | 2    |

The ticket says "non-zero on Pack v1 schema validation failure". 1
is the natural value across all failure modes; 2 is reserved for
arg parse failures (matches the existing runPackCmd convention).

## Rejected ideas

- **A `pack-diff` mode in this ticket.** Out of scope — the ticket
  says it's a future ticket.
- **Listing dist/plants/.** Out of scope — the ticket says use ls.
- **Re-rendering the GLB to a thumbnail.** Out of scope — that's
  the production preview.
- **Streaming output as the file is parsed.** Pack files are <5 MiB
  by spec; reading the whole file first is fine.
- **Caching parsed packs.** Inspect is interactive; one read per
  invocation is the right model.
