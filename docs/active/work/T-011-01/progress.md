# T-011-01 Progress — dist-output-path

Implementation log. Five steps in plan.md, all landed; one mid-flight
correction caused by a parallel-ticket merge that I had to absorb.

## Step 1 — `pack_writer.go` + `pack_writer_test.go`

Created `pack_writer.go` with the `DistPlantsDir` constant and the
`WritePack(distDir, species string, pack []byte) error` function. The
implementation is the four-line form from `design.md`:
species-empty guard → `MkdirAll(distDir, 0755)` → delegate to
`writeAtomic` from `settings.go`.

Created `pack_writer_test.go` with **four** tests rather than the
three planned:
- `TestWritePack_HappyPath` — write, read back, assert one entry
  in the directory (the leftover-`.tmp` smoke test).
- `TestWritePack_OverwritesExisting` — pre-write sentinel bytes,
  overwrite, assert final == new and one entry.
- `TestWritePack_CreatesMissingDir` — point at a deeply nested path
  that does not exist; assert `WritePack` materializes it.
- `TestWritePack_EmptySpeciesError` — added beyond the plan because
  it costs nothing and locks in the explicit guard. Without it, an
  upstream regression that hands `WritePack` an empty species would
  silently produce a `.glb` file.

`go vet ./... && go test -run TestWritePack ./...` passed first try.

## Step 2 — Migrate the call site (mid-flight surprise)

Plan said: migrate `handlers.go:1802–1807`'s `os.WriteFile` to use
`WritePack`. **The handler had already been refactored** by an
in-flight T-010-04 thread which extracted the pack pipeline into
a new `pack_runner.go` and a `RunPack` function. The actual write
now lived at `pack_runner.go:113`, not in `handlers.go`. The
handler is now an HTTP envelope that calls `RunPack` and maps
`PackResult.Status` to status codes.

Adjusted: migrated the `os.MkdirAll`/`os.WriteFile` pair in
`pack_runner.go`'s `RunPack` to a single `WritePack` call. The
original code had a now-unused `distPath` local that I removed.

A second surprise landed during the same edit: the same parallel
agent tried to add its **own** `WritePack` helper directly inside
`pack_runner.go` (a non-atomic `MkdirAll`+`WriteFile` form). That
collided with my atomic version in `pack_writer.go` — two
`WritePack` definitions in `package main` is a compile error. I
deleted the duplicate from `pack_runner.go`; the canonical
implementation in `pack_writer.go` is the one we want because it
honors the ticket's atomic-write requirement. The lesson recorded
for review: T-010-04's design assumed the write step was a trivial
MkdirAll+WriteFile, while T-011-01 specifically promotes it to an
atomic helper. The `pack_writer.go` version wins on functional
grounds.

`go vet ./... && go test ./...` after the dedup: clean.

## Step 3 — Constant in `main.go`

Single-line edit at `main.go:78`:

```go
distPlantsDir := filepath.Join(workDir, DistPlantsDir)
```

(Previously `filepath.Join(workDir, "dist", "plants")`.) The mkdir
loop and route registration are byte-identical. Verified by
re-running the full test suite.

## Step 4 — `.gitignore`

Added `dist/` after the `*.zip` archive entry, with a comment:

```
# Built asset packs (USB drop source — never committed)
dist/
```

`git status` now ignores `dist/plants/*.glb` if it ever appears
in the repo root.

## Step 5 — `just clean-packs`

Appended after the `clean:` recipe:

```just
# Remove all built asset packs from dist/plants/ (does not touch outputs/)
clean-packs:
    rm -rf dist/plants
    @mkdir -p dist/plants
    @echo "✓ cleaned dist/plants/"
```

Smoke test: `mkdir -p dist/plants && touch dist/plants/test.glb &&
just clean-packs && ls dist/plants` — directory present, empty,
test file gone. The trailing `mkdir -p` keeps the directory present
so `WritePack` doesn't have to recreate it from scratch on the next
pack. Cleaned up the smoke-test artifact (`rm -rf dist`) so the
working tree stays unchanged outside of the planned edits.

## Final verification

```
go vet ./...
go test ./...
```

Both clean. The full test suite runs in ~3s and reports `ok
glb-optimizer`. No warnings, no flaky tests, no skipped packages.

## Files touched

- `pack_writer.go` (NEW, 32 lines)
- `pack_writer_test.go` (NEW, 96 lines, 4 tests)
- `pack_runner.go` (write site migrated to `WritePack`; duplicate
  helper removed)
- `main.go` (one-line literal → constant)
- `.gitignore` (`dist/` added)
- `justfile` (`clean-packs` recipe added)

## Files NOT touched

- `combine.go`, `bake_stamp.go`, `pack_meta*.go`, `handlers.go`
  (handler proper), `handlers_pack_test.go`, `pack_runner_test.go`
  — none required modification. The `handlers_pack_test.go` E2E
  test continues to pass through the new `WritePack` path without
  source changes.
