# T-011-01 Structure — dist-output-path

Precise mutation points. Numbers are pre-edit line references; they
will shift as edits land.

## New file: `pack_writer.go`

Top of file:

```go
package main

import (
    "fmt"
    "os"
    "path/filepath"
)

// DistPlantsDir is the relative subpath, under workDir, where finished
// asset packs are written. The directory is the USB-drop source for
// the demo laptop.
const DistPlantsDir = "dist/plants"

// WritePack atomically writes a finished pack GLB to
// distDir/{species}.glb, creating distDir if missing. The write goes
// through a sibling .tmp file followed by os.Rename so a crashed
// process never leaves a half-written .glb on the USB-drop directory.
func WritePack(distDir, species string, pack []byte) error {
    if species == "" {
        return fmt.Errorf("WritePack: empty species")
    }
    if err := os.MkdirAll(distDir, 0755); err != nil {
        return fmt.Errorf("WritePack: mkdir %s: %w", distDir, err)
    }
    return writeAtomic(filepath.Join(distDir, species+".glb"), pack)
}
```

## New file: `pack_writer_test.go`

```go
package main

import (
    "bytes"
    "os"
    "path/filepath"
    "testing"
)

func TestWritePack_HappyPath(t *testing.T) { ... }
func TestWritePack_OverwritesExisting(t *testing.T) { ... }
func TestWritePack_CreatesMissingDir(t *testing.T) { ... }
```

Each test:
- `dir := t.TempDir()` (or a subpath)
- call `WritePack(dir, "fern", []byte(...))`
- assert `os.ReadFile` matches
- assert `os.ReadDir(dir)` returns exactly one entry — this is the
  leftover-`.tmp` smoke test

## Edit: `handlers.go`

**Line 1802–1807**, inside `handleBuildPack`:

Before:
```go
distPath := filepath.Join(distDir, meta.Species+".glb")
if err := os.WriteFile(distPath, packBytes, 0644); err != nil {
    jsonError(w, http.StatusInternalServerError,
        "failed to write pack: "+err.Error())
    return
}
```

After:
```go
if err := WritePack(distDir, meta.Species, packBytes); err != nil {
    jsonError(w, http.StatusInternalServerError,
        "failed to write pack: "+err.Error())
    return
}
distPath := filepath.Join(distDir, meta.Species+".glb")
```

The `distPath` recomputation is reordered to come after the write
because it is only used by the JSON response immediately below.

## Edit: `main.go`

**Line 65**:

Before:
```go
distPlantsDir := filepath.Join(workDir, "dist", "plants")
```

After:
```go
distPlantsDir := filepath.Join(workDir, DistPlantsDir)
```

No other touches in main.go — the mkdir loop and route registration
stay byte-identical.

## Edit: `justfile`

After the existing `clean:` recipe (line 21–22), add:

```just

# Remove all built asset packs from dist/plants/ (does not touch outputs/)
clean-packs:
    rm -rf dist/plants
    @mkdir -p dist/plants
    @echo "✓ cleaned dist/plants/"
```

Leading blank line preserves the one-blank-between-recipes rhythm.

## Edit: `.gitignore`

Add near the "Downloaded archives" block (currently line 27 area):

```
# Built asset packs (USB drop source — never committed)
dist/
```

## Files NOT touched

- `combine.go` — `CombinePack` is payload-only, never touches the
  filesystem. T-010-02's design choice still holds.
- `bake_stamp.go` — its inline temp+rename is fine for `_bake.json`;
  not in scope to refactor onto `writeAtomic` here.
- `handlers_pack_test.go` — the existing E2E test passes `t.TempDir()`
  as `distDir`, which is exactly what `WritePack` accepts. No changes.
- `main.go` mkdir loop — `distPlantsDir` is still in the slice, so
  the `0755` directory still exists at startup. `WritePack`'s
  `MkdirAll` is the belt to that suspenders, for the case where the
  directory is wiped between server startup and a pack request
  (e.g. by `just clean-packs`).
- `pack_meta.go`, `pack_meta_capture.go`, `bake_stamp.go` — all
  unrelated, the bake_id and metadata pipelines do not change.

## Order of operations (preview of plan.md)

1. Write `pack_writer.go` + `pack_writer_test.go`, run `go test ./...`
2. Migrate `handlers.go` call site
3. Use `DistPlantsDir` constant in `main.go`
4. Add `.gitignore` entry
5. Add `just clean-packs` recipe and smoke-test it
6. Final `go vet ./... && go test ./...`
