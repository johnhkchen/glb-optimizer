# T-011-02 — Research

## Goal recap

Bridge bake state → `PackMeta`. A function `BuildPackMetaFromBake` reads
the un-decimated source mesh, current `AssetSettings`, and an optional
per-asset `_meta.json` override, and assembles a fully-populated, validated
`PackMeta` ready for combine to embed.

## Where the inputs live

### Original source mesh

`main.go:56` defines `originalsDir := filepath.Join(workDir, "originals")`.
Uploads land at `originalsDir/{id}.glb` (`handlers.go:75`,
`destPath := filepath.Join(originalsDir, id+".glb")`). This is the
un-decimated source the ticket calls "the original mesh" — the file that
exists *before* gltfpack/blender/volumetric bake produces an output in
`outputsDir`. The asset id is the FileRecord.ID, generated at upload time;
`FileRecord.Filename` (`models.go`) preserves the original upload name and
is the right input for filename → species derivation.

### Settings (fade thresholds)

`settings.go` owns `AssetSettings` and the on-disk per-asset JSON at
`settingsDir/{id}.json`. The three fields the ticket needs are
`TiltedFadeLowStart`, `TiltedFadeLowEnd`, `TiltedFadeHighStart` (added
in T-009-03; lines 33–43). `LoadSettings(id, dir)` returns
`DefaultSettings()` if the file does not exist (settings.go), so capture
gracefully degrades to defaults — that matches the ticket's "current
settings" wording for assets that have never been tuned.

`DefaultSettings()` (settings.go:80) hard-codes `0.30 / 0.55 / 0.75`,
which already satisfies the PackMeta `low_start < low_end < high_start`
ordering checked in `pack_meta.go:Validate`.

### Per-asset override (`_meta.json`)

The ticket specifies `outputs/{id}_meta.json`. This file does *not* yet
exist — Out-of-Scope confirms the format is intentionally minimal: a
small JSON object with optional `species` and `common_name` keys. There
is no producer; this is purely an escape hatch for the demo cases like
`sample_2026-04-08T010040.068 → dahlia_blush`.

### Outputs dir

`main.go:57` — `outputsDir`. Already passed through to every handler
that writes baked artifacts (billboard, tilted billboard, volumetric).
The per-asset `_meta.json` will live alongside those outputs and is read
during capture.

## Existing PackMeta surface (T-010-01)

`pack_meta.go` (read in full) defines:

- `PackFormatVersion = 1`
- `Footprint{CanopyRadiusM, HeightM}` (positive, finite)
- `FadeBand{LowStart, LowEnd, HighStart}` ([0,1], strictly ordered, HighStart ≤ 1)
- `PackMeta{FormatVersion, BakeID, Species, CommonName, Footprint, Fade}`
- `(PackMeta).Validate()` — full v1 contract enforcement
- `(PackMeta).ToExtras()` — canonical map for glTF embedding
- `ParsePackMeta(json.RawMessage)` — consumer-side decoder
- `speciesRe = ^[a-z][a-z0-9_]*$`
- `checkPositive` (local), `checkRange` (defined in settings.go and shared)

`pack_meta_test.go` is 14 tests, all green (memory ID 257). Capture
must produce a `PackMeta` that round-trips `Validate` cleanly — that
is the contract the test suite locks down.

## GLB AABB extraction — what's already in the repo

`scene.go:CountTrianglesGLB` (read in full) is the prior art for "open
a GLB, parse just the JSON chunk, walk accessors and meshes". It:

1. Reads the file with `os.ReadFile`.
2. Verifies the `glTF` magic (0x46546C67) at offset 0.
3. Reads the first chunk header at offset 12 — chunk length + chunk
   type — and verifies type is `JSON` (0x4E4F534A).
4. JSON-unmarshals just the `accessors[].count` and `meshes[].primitives[].indices`
   subset it cares about.

This is the pattern to copy verbatim for footprint extraction. The
*delta* for capture is: walk every primitive's `POSITION` accessor and
read its `min` / `max` arrays (glTF requires `min`/`max` on POSITION
accessors). Across all primitives, take the component-wise min of
`min` and component-wise max of `max` to get the *local* AABB.

## Node transforms — a deliberate non-goal

A perfectly correct world AABB requires walking the scene graph and
applying each node's TRS. The repo has no glTF node-traversal code and
the in-scope assets — single-mesh plant scans from TRELLIS — sit at
the scene root with identity transforms. The 5%-tolerance integration
test in the AC is consistent with treating the local-space accessor
AABB as the world AABB. If a future asset arrives pre-translated, the
override `_meta.json` is the escape hatch (can extend with footprint
keys later; out of scope for this ticket).

## Filename → species derivation

`FileRecord.Filename` is the original upload filename (e.g.
`Rose_Julia-Child.glb`). Ticket rule: lowercase, non-alphanum → `_`,
strip leading digits. Concrete example to validate against:

- `Rose_Julia-Child.glb` → strip ext → `Rose_Julia-Child` →
  lowercase → `rose_julia-child` → non-alphanum → `_` →
  `rose_julia_child`. Passes `^[a-z][a-z0-9_]*$`.
- `123_planter.glb` → `123_planter` → `_planter` → after leading-digit
  strip leaves `_planter` which fails `^[a-z]…`. Need to *also* strip
  any leading underscores left over. Codify: strip leading [^a-z]+.
- `_meta.json` companion is the override for cases like
  `sample_2026-04-08T010040.068.glb` → derived would start with the
  bare year and fail the regex; the override file sets the right id.

If the derived id still fails the regex after both strips → return
an explicit error so combine fails loudly (matches the "fail loudly,
not silently use defaults" Note in the ticket).

`FileRecord.Filename` may also be lost across server restart — see
`main.go:183` "We lose original filename on restart" comment in
`scanExistingFiles`. After a restart, `Filename` becomes the on-disk
`{id}.glb`. The id itself is then the only stable derivation source.
Capture must therefore accept *the id as a fallback* when no original
filename is known, and the override `_meta.json` is the user-facing
fix when that fallback is wrong.

## common_name derivation

Title-case the species id with `_` → space:
`rose_julia_child` → `Rose Julia Child`. Use `strings.Title` is
deprecated; use `golang.org/x/text/cases` *or* a simple manual loop.
The codebase has no `golang.org/x/text` import — manual loop is
lower-overhead and avoids a new dep.

## bake_id format

Ticket: "current UTC time in RFC3339". `time.Now().UTC().Format(time.RFC3339)`.
T-011-03 plans to embed this in the GLB scene extras and notes a
"stability gotcha" (memory ID 221) — capture is the upstream producer,
so the format choice here freezes the gotcha downstream.

## Function signature (proposed)

The ticket spells the signature `BuildPackMetaFromBake(id string)`.
Every other capture-adjacent function in this repo takes paths as
explicit args (`LoadSettings(id, dir)`, `SettingsFilePath(id, dir)`,
`SettingsExist(id, dir)`). Following that local convention:

```go
func BuildPackMetaFromBake(id, originalsDir, settingsDir, outputsDir string, store *FileStore) (PackMeta, error)
```

`store` is the source of `Filename`. Passing it keeps the function
testable (no global) and matches how `handleProcess`, `handleClassify`,
etc. take the store as a dependency.

## Open questions / assumptions

1. Should capture refuse to run when settings are at defaults? No —
   defaults are valid fade thresholds, the consumer expects every pack
   to have a `fade` block, and forcing user tuning before pack would
   block the demo. Defaults flow through.
2. Should the override `_meta.json` be allowed to set `footprint` /
   `fade` too? The ticket is explicit: only `species` / `common_name`
   in v1. Anything else is silently ignored.
3. Where does the new code live? Ticket allows `pack_meta_capture.go`
   *or* `pack.go`. Pick `pack_meta_capture.go` — `pack.go` does not
   exist yet, and putting capture in its own file keeps `pack_meta.go`
   focused on the schema/validation contract.
