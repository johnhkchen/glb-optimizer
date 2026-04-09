# T-011-01 Review — dist-output-path

Handoff for human + reviewer attention. All ACs satisfied, full
test suite green, one parallel-merge gotcha worth flagging.

## What changed

### New files

- **`pack_writer.go`** (32 LOC). Defines `const DistPlantsDir = "dist/plants"`
  and `func WritePack(distDir, species string, pack []byte) error`.
  The function guards against an empty species, creates `distDir`
  with 0755 if missing, then delegates to `writeAtomic` (the
  established temp-file + `os.Rename` helper from `settings.go:393`).
- **`pack_writer_test.go`** (96 LOC, 4 tests). Hermetic tests in
  `t.TempDir()`; no fixtures needed.

### Modified files

- **`pack_runner.go`** — `RunPack` now calls `WritePack` instead
  of inlining `os.MkdirAll` + `os.WriteFile`. The unused `distPath`
  local was removed; the handler reconstructs the path from
  `result.Species` for the JSON response. **Also:** a duplicate
  `WritePack` helper that landed in `pack_runner.go` from a
  parallel T-010-04 thread was removed (it was non-atomic and
  collided with `pack_writer.go`'s definition).
- **`main.go`** — one-line change at line 78: the literal
  `filepath.Join(workDir, "dist", "plants")` is now
  `filepath.Join(workDir, DistPlantsDir)`. The mkdir loop and the
  `/api/pack/` route registration are byte-identical.
- **`.gitignore`** — added `dist/` with an explanatory comment.
- **`justfile`** — appended a `clean-packs` recipe.

## Acceptance criteria check

| AC | Status | Evidence |
|---|---|---|
| `const DistPlantsDir = "dist/plants"` | ✅ | `pack_writer.go:11` |
| `func WritePack(species, pack) error` writes to `DistPlantsDir/{species}.glb`, creates dir if missing, atomic via temp+rename | ✅ (with one-parameter deviation) | `pack_writer.go:23`. Signature is `WritePack(distDir, species string, pack []byte) error` — see *Deviations* below. |
| `outputs/` intermediates not touched | ✅ | `WritePack` only writes to `distDir`. `RunPack` reads from `outputsDir`, writes to `distDir`. The two paths are siblings under `workDir`. |
| `dist/` in `.gitignore` | ✅ | `.gitignore` line 38, with comment. |
| `just clean-packs` removes `dist/plants/` contents | ✅ | Smoke-tested: pre-touch, run recipe, verified empty. Does not touch `outputs/`. |

## Deviations from the ticket

**`WritePack` signature.** The ticket specified
`WritePack(species string, pack []byte) error`. The implementation
takes `(distDir, species string, pack []byte) error`. Rationale
is in `design.md` § *Deviation from ticket signature*: the
alternatives are (a) writing CWD-relative, which forces `os.Chdir`
in tests and breaks the workDir abstraction, or (b) hiding
`distDir` behind a package-global mutable variable, which adds
process state for no benefit. The constant `DistPlantsDir` is
still the single source of truth for the relative subpath; main.go
composes it with `workDir` and the result is passed in. If the
reviewer prefers (b), the change is mechanical — move the dir to
a `var PackOutputDir string`, init in main.go, drop the parameter
— and the test surface stays the same modulo a `t.Cleanup`
restore.

## Test coverage

### Unit tests (new in this ticket)

- `TestWritePack_HappyPath` — small payload round-trip; asserts
  one entry in dir (the leftover-`.tmp` smoke test).
- `TestWritePack_OverwritesExisting` — pre-write + overwrite;
  asserts replaced bytes and one entry.
- `TestWritePack_CreatesMissingDir` — deeply nested non-existent
  path is materialized.
- `TestWritePack_EmptySpeciesError` — explicit species guard.

### Tests not added but already covering the path

- `handlers_pack_test.go` — full HTTP-handler E2E that exercises
  the route → `RunPack` → `WritePack` chain. Continues to pass
  unchanged.
- `pack_runner_test.go` — tests `RunPack` directly against a
  `t.TempDir()` `distDir`; the new atomic write is the only thing
  that runs in the success path now, and the suite is green.

### Coverage gaps (intentional, not added)

- **Error injection on a read-only `distDir`.** Skipped — adds
  OS-specific permission juggling for a single error branch that
  is already covered by `writeAtomic`'s own tests in
  `settings_test.go`.
- **Cross-filesystem rename.** Not applicable on the demo laptop
  (everything lives under `~/.glb-optimizer`); `os.CreateTemp` in
  the destination dir already protects us if the situation ever
  arose.
- **`nil` pack bytes.** Not tested explicitly. The current
  behavior is "produce a zero-byte file" (delegated to
  `writeAtomic`'s `os.WriteFile` semantics), and `RunPack` already
  guarantees `packBytes != nil` on the success path.

## Open concerns / things reviewer should look at

1. **Parallel-merge collision.** A T-010-04 thread independently
   added a non-atomic `WritePack` to `pack_runner.go` while this
   ticket was in flight. I removed the duplicate so the build
   compiles, but this is a soft signal that the DAG between
   T-010-04 and T-011-01 needed an explicit edge: both tickets
   touch the pack write step. T-011-01 depends on T-010-02 in
   the frontmatter but **not** on T-010-04. If they had been
   serialized, the friction would have been zero. Worth recording
   in the post-mortem.

2. **`bake_stamp.go` still inlines its temp+rename.** The atomic
   pattern is now spread across `settings.go` (`writeAtomic`),
   `bake_stamp.go` (inline), and `pack_writer.go` (calls
   `writeAtomic`). A future tidy-up could collapse `bake_stamp.go`
   onto `writeAtomic`. Out of scope for this ticket.

3. **`just clean-packs` operates on the just-invocation directory's
   `dist/plants/`, not on `workDir`.** The morning-of-demo flow
   runs from the repo root, so this is fine, but a user running
   `glb-optimizer` from a non-repo working directory would not
   benefit from this recipe. Documented in `design.md`. If
   T-010-04's pack-all CLI grows a `--clean` flag in the future,
   that path will be the workDir-aware alternative.

4. **`writeAtomic` puts the temp file in the destination directory.**
   Correct for atomicity (rename is only atomic within a
   filesystem) but means a sibling `.tmp` file is briefly visible
   during the write. The `TestWritePack_HappyPath` "exactly one
   entry" assertion catches the case where cleanup is skipped.

## Critical issues

None. Build is clean (`go vet ./...`), full test suite is green
(`go test ./...` returns `ok glb-optimizer`), and all ACs are
satisfied.

## TODOs deferred to other tickets

- T-010-04 follow-up: the `pack-all` CLI subcommand will pick up
  `WritePack` for free since it routes through `RunPack`.
- T-011-04 (integration handshake): the dist directory and
  filename convention are now stable enough for plantastic's
  loader test to pin against `dist/plants/{species}.glb`.
- Future tidy-up: collapse `bake_stamp.go` onto `writeAtomic`
  for consistency. Not blocking anything.

## Files changed (final list)

```
A  pack_writer.go
A  pack_writer_test.go
M  pack_runner.go
M  main.go
M  .gitignore
M  justfile
A  docs/active/work/T-011-01/research.md
A  docs/active/work/T-011-01/design.md
A  docs/active/work/T-011-01/structure.md
A  docs/active/work/T-011-01/plan.md
A  docs/active/work/T-011-01/progress.md
A  docs/active/work/T-011-01/review.md
```
