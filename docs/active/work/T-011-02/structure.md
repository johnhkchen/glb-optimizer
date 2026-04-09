# T-011-02 — Structure

## Files touched

| File | Op | Purpose |
|------|----|---------|
| `pack_meta_capture.go` | **create** | `BuildPackMetaFromBake` + helpers |
| `pack_meta_capture_test.go` | **create** | unit + integration tests |

No edits to `pack_meta.go`, `settings.go`, `models.go`, `handlers.go`,
or `main.go`. Capture is purely additive — combine (T-010-02) will
later import `BuildPackMetaFromBake`, but wiring happens in that
ticket, not this one.

## `pack_meta_capture.go` — public surface

```go
package main

// BuildPackMetaFromBake reads the bake-time state for asset id and
// assembles a fully populated, validated PackMeta. Inputs:
//   - the un-decimated source mesh at originalsDir/{id}.glb
//   - the optional per-asset override at outputsDir/{id}_meta.json
//   - the current AssetSettings at settingsDir/{id}.json (or defaults)
//   - the FileRecord in store, for the original upload Filename
//
// Returns an error and zero-value PackMeta on any failure. Capture
// fails loudly: a missing source mesh, a derived species id that fails
// validation, or a degenerate footprint are all hard errors so combine
// surfaces them instead of shipping a half-baked pack.
func BuildPackMetaFromBake(
    id, originalsDir, settingsDir, outputsDir string,
    store *FileStore,
) (PackMeta, error)
```

## `pack_meta_capture.go` — internal helpers

All unexported, all in the same file (matches `pack_meta.go`'s
single-file convention recorded in memory ID 244):

```go
// readSourceFootprint opens originalsDir/{id}.glb, parses just the
// JSON chunk, and computes the local-space AABB across every
// primitive's POSITION accessor. Returns the Footprint or an error.
func readSourceFootprint(path string) (Footprint, error)

// loadCaptureOverride reads outputsDir/{id}_meta.json if present and
// returns the (possibly empty) species/common_name overrides. Missing
// file → zero values + nil error. Malformed JSON → error.
func loadCaptureOverride(path string) (captureOverride, error)

type captureOverride struct {
    Species    string `json:"species"`
    CommonName string `json:"common_name"`
}

// deriveSpeciesFromName turns a filename or id into a slug satisfying
// ^[a-z][a-z0-9_]*$. Strips extension, lowercases, replaces non-
// alphanum with _, strips leading non-letters and trailing
// underscores, collapses repeated underscores. Returns "" if nothing
// usable remains; caller emits the operator-facing error.
func deriveSpeciesFromName(name string) string

// titleCaseSpecies maps "rose_julia_child" → "Rose Julia Child".
func titleCaseSpecies(species string) string

// captureFadeFromSettings reads settingsDir/{id}.json (or defaults)
// and projects the three fade fields into a FadeBand. The full
// AssetSettings is loaded but only three fields are read.
func captureFadeFromSettings(id, settingsDir string) (FadeBand, error)
```

## Decomposition notes

`BuildPackMetaFromBake` is the only exported symbol. Its body is a
short orchestration:

1. Resolve filename: `store.Get(id)` → `Filename`. Empty / equal to
   `{id}.glb` → fall back to id.
2. Resolve override: `loadCaptureOverride(filepath.Join(outputsDir, id+"_meta.json"))`.
3. Resolve species:
   - if override has species → use it (validated by final `PackMeta.Validate`)
   - else `deriveSpeciesFromName(filename)` → if empty, error with
     hint pointing at the override file.
4. Resolve common_name: override wins, else `titleCaseSpecies(species)`.
5. Footprint: `readSourceFootprint(filepath.Join(originalsDir, id+".glb"))`.
6. Fade: `captureFadeFromSettings(id, settingsDir)`.
7. Assemble `PackMeta`:
   ```go
   meta := PackMeta{
       FormatVersion: PackFormatVersion,
       BakeID:        time.Now().UTC().Format(time.RFC3339),
       Species:       species,
       CommonName:    common,
       Footprint:     footprint,
       Fade:          fade,
   }
   ```
8. `meta.Validate()` → return.

## GLB parsing — internal types

`readSourceFootprint` reuses the byte/chunk handling from
`scene.go:CountTrianglesGLB` but its JSON sub-decoder differs. It
needs the position accessor min/max:

```go
var gltf struct {
    Accessors []struct {
        Min []float64 `json:"min"`
        Max []float64 `json:"max"`
    } `json:"accessors"`
    Meshes []struct {
        Primitives []struct {
            Attributes struct {
                POSITION int `json:"POSITION"`
            } `json:"attributes"`
        } `json:"primitives"`
    } `json:"meshes"`
}
```

Index validation: a primitive whose POSITION index is out of range,
or whose accessor lacks 3-component min/max, is treated as a parse
error (not silently skipped). At least one primitive with valid
POSITION is required; otherwise error "no POSITION accessors with
min/max".

Reduce: start with sentinels `[+inf,+inf,+inf]` / `[-inf,-inf,-inf]`,
component-wise update from each primitive. Final dims:

- `height_m   = max[1] - min[1]`
- `width_x    = max[0] - min[0]`
- `depth_z    = max[2] - min[2]`
- `canopy_radius_m = max(width_x, depth_z) / 2`

Both must be > 0; otherwise return a "degenerate mesh" error so
`PackMeta.Validate`'s positivity check is doubly enforced (clear
error message at the point of computation, not later).

## `pack_meta_capture_test.go` — layout

Six unit tests + one integration test:

| # | Name | What it locks down |
|---|------|--------------------|
| 1 | `TestBuildPackMetaFromBake_HappyPath` | synthetic GLB + filename + default settings → valid PackMeta |
| 2 | `TestBuildPackMetaFromBake_OverrideWins` | override JSON sets species + common_name; derivation skipped |
| 3 | `TestBuildPackMetaFromBake_LeadingDigitsStripped` | filename `123_planter.glb` → species `planter` |
| 4 | `TestBuildPackMetaFromBake_DerivationFails` | filename `2026-04-08.glb` → error mentions override path |
| 5 | `TestBuildPackMetaFromBake_TunedFadeFlowsThrough` | settings.json on disk with non-default fades → captured |
| 6 | `TestBuildPackMetaFromBake_MissingSource` | no GLB at originalsDir/{id}.glb → error |
| 7 | `TestBuildPackMetaFromBake_RoseJuliaChildFixture` | uses `assets/rose_julia_child.glb`, asserts footprint within 5% |

### Synthetic GLB helper

Tests need a tiny GLB on disk. Add an unexported helper in the test
file:

```go
// writeMinimalGLB writes a 1-mesh 1-primitive GLB with a single
// POSITION accessor whose min/max are the supplied vectors. The
// binary chunk is empty (0 bytes) — POSITION min/max is metadata
// only and the parser never touches buffers.
func writeMinimalGLB(t *testing.T, path string, min, max [3]float64)
```

Body sketch:
1. Build a `gltf` JSON object as a `map[string]any` with one
   `accessors` entry carrying the min/max, one `meshes[0].primitives[0]`
   pointing at accessor 0 via `attributes.POSITION`, plus the
   `asset.version` glTF requires.
2. Marshal, pad with `' '` to 4-byte alignment.
3. Write the 12-byte glTF header (`magic=0x46546C67`,
   `version=2`, `length=12+8+jsonLen+8+0`).
4. Write the JSON chunk header (`chunkLen=jsonLen`,
   `chunkType=0x4E4F534A`) + the padded JSON.
5. Write a zero-length BIN chunk header (`chunkLen=0`,
   `chunkType=0x004E4942`). Padding rule for BIN is moot when length
   is 0.

This is ~30 lines and lets every unit test be hermetic.

### Fixture integration test

The repo already ships `assets/rose_julia_child.glb`. The test:

1. `t.TempDir()` for originals/, settings/, outputs/.
2. Copies (or symlinks) `assets/rose_julia_child.glb` into
   `originals/{id}.glb`.
3. Adds a `FileRecord{ID: id, Filename: "rose_julia_child.glb"}` to
   a fresh store.
4. Calls `BuildPackMetaFromBake`.
5. Asserts species == `rose_julia_child`, common_name ==
   `Rose Julia Child`, footprint within 5% of constants captured
   from a one-time measurement (recorded in a comment block on
   the test).
6. Skips with `t.Skip` if the fixture is unexpectedly absent — keeps
   CI green on a checkout that prunes `assets/`.

## Naming and conventions

- Package `main` (matches every other Go file in this directory).
- Error wrapping: `fmt.Errorf("pack_meta_capture: …: %w", err)` —
  same prefix discipline as `pack_meta.go`'s `pack_meta:` prefix.
- No new external deps. Stdlib only: `encoding/json`, `fmt`, `math`,
  `os`, `path/filepath`, `regexp`, `strings`, `time`.
- File-level doc comment at the top mirroring `pack_meta.go`'s tone.
