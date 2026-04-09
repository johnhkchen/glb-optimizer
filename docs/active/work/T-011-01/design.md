# T-011-01 Design — dist-output-path

## Goal

Replace the inline `os.WriteFile` in `handleBuildPack` with a named,
atomic, directory-aware helper. Centralize the relative subpath as
a Go constant. Add a justfile cleaner. Add `dist/` to `.gitignore`.

## Public surface

A new file `pack_writer.go` (`package main`) introduces:

```go
// DistPlantsDir is the relative subpath, under workDir, where finished
// asset packs are written. The directory is the USB-drop source for
// the demo laptop. Consumers compose it with workDir at startup.
const DistPlantsDir = "dist/plants"

// WritePack atomically writes a finished pack GLB to
// distDir/{species}.glb. distDir is created with 0755 if missing.
// The write goes through a sibling temp file followed by os.Rename
// so a crashed process never leaves a half-written .glb on the
// USB-drop directory.
//
// distDir is passed in (rather than being read from a constant)
// because callers — main.go's handler wiring and T-010-04's CLI —
// already hold the absolute, workDir-rooted form, and tests want a
// hermetic t.TempDir().
func WritePack(distDir, species string, pack []byte) error {
    if species == "" {
        return fmt.Errorf("WritePack: empty species")
    }
    if err := os.MkdirAll(distDir, 0755); err != nil {
        return fmt.Errorf("WritePack: mkdir %s: %w", distDir, err)
    }
    final := filepath.Join(distDir, species+".glb")
    return writeAtomic(final, pack)
}
```

### Deviation from ticket signature

The ticket text proposes `WritePack(species string, pack []byte) error`.
I am amending it to `WritePack(distDir, species string, pack []byte) error`.
Reason recorded in `research.md` § *Constant placement* — the only
alternatives are CWD-relative writes (breaks the workDir abstraction
and forces `os.Chdir` in tests) or a package-global mutable variable
(adds hidden state for no benefit). The ticket-stated AC for "writes
to `DistPlantsDir/{species}.glb`" is honored: callers compose
`filepath.Join(workDir, DistPlantsDir)` and pass the result. The
constant is the single source of truth for the subpath.

### Validation

`species == ""` returns an explicit error rather than producing a
file named `.glb`. No path-traversal scrubbing — `species` originates
from `PackMeta` produced by `BuildPackMetaFromBake`, which already
runs through the analytics-validated naming pipeline. Adding another
sanitization layer would duplicate logic and silently mask upstream
bugs.

`pack == nil` is **allowed**: `writeAtomic` will produce a zero-byte
file. This matches `os.WriteFile` semantics; callers that want a
non-empty guarantee should check `len(pack)` themselves. The handler
already has `len(packBytes) == 0` covered upstream by `CombinePack`,
which never returns an empty success.

### Reuse of `writeAtomic`

Delegating to `writeAtomic` (settings.go:393) — not re-implementing
the temp+rename inline like `bake_stamp.go` does — buys two things:

1. Single temp-file cleanup discipline. Every error branch already
   removes the temp; we don't have to retest it.
2. Future-proof: if the team ever swaps the implementation (e.g. to
   `os.CreateTemp` with `O_DSYNC`), every atomic-write site benefits
   without a sweep.

The cost is a single function call's worth of indirection.

## Caller migration

`handlers.go:1801–1807` becomes:

```go
if err := WritePack(distDir, meta.Species, packBytes); err != nil {
    jsonError(w, http.StatusInternalServerError,
        "failed to write pack: "+err.Error())
    return
}
distPath := filepath.Join(distDir, meta.Species+".glb")
```

The `distPath` recomputation after the write is cosmetic — we need
the path string for the JSON response body, and recomputing it
locally is cheaper than threading it back out of `WritePack` (which
would push the signature to `(distDir, species, pack) (string, error)`
and force every test to ignore the first return). Alternative
considered and rejected: have `WritePack` return the path. Not worth
the extra return value for one call site.

`main.go:65` is touched to use the new constant:

```go
distPlantsDir := filepath.Join(workDir, DistPlantsDir)
```

The local variable name stays lowercase `distPlantsDir` (consistent
with sibling `originalsDir`, `outputsDir`, …); the constant is the
exported, capitalized form.

## `pack_writer_test.go`

Three table-free tests, each in its own `t.TempDir()`:

1. `TestWritePack_HappyPath` — small payload, asserts file exists,
   contents byte-equal, mode 0644 (or whatever umask gives — we'll
   assert the bits we care about), and `os.ReadDir` returns exactly
   one entry (no leftover `.tmp`).
2. `TestWritePack_OverwritesExisting` — pre-write sentinel bytes,
   call `WritePack` with new bytes, assert final == new and dir
   has exactly one entry.
3. `TestWritePack_CreatesMissingDir` — point at `tmp/nested/dist`
   that does not exist, assert `WritePack` creates the chain.

Test fixtures are byte-literal, no GLB needed — `WritePack` is
payload-agnostic.

## `justfile` recipe

```just
# Remove all built asset packs from dist/plants/ (does not touch outputs/)
clean-packs:
    rm -rf dist/plants
    @mkdir -p dist/plants
    @echo "✓ cleaned dist/plants/"
```

Inserted directly after the existing `clean:` recipe. The trailing
`mkdir -p` keeps the directory present so subsequent `WritePack`
calls don't need to recreate it from scratch and `git status` stays
quiet under the new `.gitignore` rule.

Note: this recipe operates on the **repo-root** `dist/plants/`, not
on `workDir`. The morning-of-demo human runs `just clean-packs &&
just pack-all` from the repo, so workDir = repo-root in that flow.
Production users running `glb-optimizer` from a non-repo workDir
would not use this recipe and don't need to.

## `.gitignore`

Add a single line:

```
# Built asset packs (USB drop source — never committed)
dist/
```

Placed in the "Downloaded archives" neighborhood, which is the
closest thematic match for "generated artifacts not committed."

## Out of scope (defer)

- A `clean-all` aggregator recipe (T-010-04 territory).
- Path traversal sanitization on `species` (origins are already
  validated upstream; double-validation hides bugs).
- Cross-filesystem rename detection (irrelevant on the demo laptop).
- Returning the written path from `WritePack` (caller already has
  the components needed to recompute it).
