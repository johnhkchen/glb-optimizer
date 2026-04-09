# T-011-01 Plan — dist-output-path

Five-step implementation, each step is a candidate commit boundary.
The whole sequence should land in well under an hour of focused work.

## Step 1 — `pack_writer.go` + `pack_writer_test.go`

- Create `pack_writer.go` with `DistPlantsDir` constant and
  `WritePack(distDir, species string, pack []byte) error`.
- Create `pack_writer_test.go` with the three table-free tests
  from `design.md`:
  - `TestWritePack_HappyPath`
  - `TestWritePack_OverwritesExisting`
  - `TestWritePack_CreatesMissingDir`
- Run `go vet ./... && go test -run TestWritePack ./...`
- **Done when:** new tests pass; existing suite still passes.

## Step 2 — Migrate the handler call site

- In `handlers.go`, replace the `os.WriteFile`/`distPath` block at
  lines 1802–1807 with a `WritePack(distDir, meta.Species, packBytes)`
  call. Recompute `distPath` only after the write succeeds.
- Run `go vet ./... && go test ./...` — particularly
  `handlers_pack_test.go`, which exercises the full handler path
  end-to-end and should keep passing without modification.
- **Done when:** all tests green, no behavioral regression in the
  handler.

## Step 3 — Use the constant in `main.go`

- In `main.go:65`, change
  `filepath.Join(workDir, "dist", "plants")` to
  `filepath.Join(workDir, DistPlantsDir)`.
- Re-run `go vet ./... && go test ./...` — purely a string-equivalent
  refactor; nothing should move.
- **Done when:** suite green; the only `"dist/plants"` literal
  remaining in the Go source is in `pack_writer.go`'s constant.

## Step 4 — `.gitignore`

- Add a `dist/` entry near "Downloaded archives" with a one-line
  comment explaining it is the USB drop directory.
- Run `git status` to confirm no spurious changes.
- **Done when:** `dist/plants/*.glb` (if present) is not listed as
  untracked.

## Step 5 — `just clean-packs` recipe

- Append the recipe under `clean:` in `justfile`.
- Smoke test: `mkdir -p dist/plants && touch dist/plants/test.glb &&
  just clean-packs && ls dist/plants` — directory should exist and
  be empty.
- Final `go vet ./... && go test ./...`.
- **Done when:** recipe wipes the directory and recreates it empty.

## Risks and mitigations

- **`writeAtomic` requires the destination directory to exist** —
  mitigated by `MkdirAll` at the top of `WritePack`.
- **Cross-filesystem rename** — unlikely on demo laptop (workDir is
  under `~/.glb-optimizer`); not handled. If it ever bites, the
  fix is `os.CreateTemp` in the destination dir, which is what
  `writeAtomic` already does.
- **`just clean-packs` from a non-repo CWD** — recipe operates on
  the just-invocation directory's `dist/plants/`, not on workDir.
  Documented in design.md; the morning-of-demo flow runs from the
  repo root, so this is fine.

## Out of scope (won't do, won't fix in this ticket)

- Refactoring `bake_stamp.go` to use `writeAtomic`.
- Adding a `dist/plants/` README or `.keep` file.
- Path-traversal sanitization on `species`.
- Aggregating clean recipes into `clean-all` (T-010-04 territory).
