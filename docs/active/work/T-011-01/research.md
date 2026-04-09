# T-011-01 Research — dist-output-path

## Ticket recap

Promote the pack-write step from inline `os.WriteFile` (added in T-010-03)
into a reusable `WritePack` helper, give it an atomic temp+rename, and
expose `DistPlantsDir = "dist/plants"` as a named constant. Add a
`just clean-packs` recipe and confirm `dist/` is gitignored.

This is a small, almost-mechanical refactor that closes three loose
ends from T-010-03:
1. The pack write is **non-atomic** today — a crashed combine could
   leave a half-written `.glb` on the USB-drop directory and silently
   fail consumer validation on the demo laptop.
2. The string `"dist/plants"` is hard-coded in `main.go` only —
   no constant, no central source of truth, and the upcoming
   `just pack-all` recipe (T-010-04) will want to reference the
   same directory from a CLI entry point.
3. There is no `clean-packs` recipe; the morning-of-demo workflow
   needs an idempotent way to wipe stale packs without nuking the
   far more expensive `outputs/` intermediates.

## Current state of the pack write path

`main.go:65` constructs the absolute pack directory:

```go
distPlantsDir := filepath.Join(workDir, "dist", "plants")
```

It is included in the startup `os.MkdirAll` loop (line 67) and
threaded into `handleBuildPack` as the trailing parameter
(`main.go:129`).

`handlers.go:1724` declares the handler:

```go
func handleBuildPack(
    store *FileStore,
    originalsDir, settingsDir, outputsDir, distDir string,
) http.HandlerFunc { ... }
```

The actual write lives at `handlers.go:1802`:

```go
distPath := filepath.Join(distDir, meta.Species+".glb")
if err := os.WriteFile(distPath, packBytes, 0644); err != nil {
    jsonError(w, http.StatusInternalServerError,
        "failed to write pack: "+err.Error())
    return
}
```

This is the only call site that writes to the dist tree today.
T-010-04 (justfile pack-all) will add a second caller via a CLI
subcommand; that ticket explicitly references `dist/plants/` and
will benefit from a single `WritePack` entry point.

## Prior art for atomic writes

The codebase already has two well-established atomic-write
patterns:

1. **`settings.go:393` — `writeAtomic(path, data)`**.
   The canonical helper. Uses `os.CreateTemp` in the destination
   directory, writes, closes, then `os.Rename`. Cleans up the
   temp file on every error path. This is exactly the pattern
   T-002-01 codified for `SaveSettings` and is reused by every
   subsequent durable-write feature.

2. **`bake_stamp.go:46` — inline `os.WriteFile(tmp) + os.Rename`**.
   T-011-03 used the inline form rather than `writeAtomic` because
   the stamp file is small and lives in `outputsDir` (different
   package boundary in spirit). The inline form is shorter but
   re-implements the same temp-cleanup discipline.

`WritePack` should reuse `writeAtomic` directly. There is no
reason to fork the pattern again, and `writeAtomic` is already
in `package main`, so the import is free. This keeps the helper
to roughly four lines: `MkdirAll` then `writeAtomic`.

## Constant placement and the function-signature wrinkle

The ticket asks for `func WritePack(species string, pack []byte) error`
— two parameters, not three. That signature is incompatible with
the current handler wiring, which passes `distDir` as a function
argument so the workDir-rooted path stays explicit and tests can
substitute a `t.TempDir()`.

There are three ways to honor the ticket signature:

| Option | Sketch | Verdict |
|---|---|---|
| **A** — Hard-code the relative path inside `WritePack`, write to `./dist/plants` relative to CWD | `os.MkdirAll(DistPlantsDir, 0755)` then `writeAtomic(filepath.Join(DistPlantsDir, species+".glb"), pack)` | Breaks workDir abstraction. Tests would need `os.Chdir`, which is process-global and racy under `t.Parallel()`. |
| **B** — Package-level mutable `PackOutputDir`, defaulting to `DistPlantsDir`, set by `main.go` to the absolute path at startup | Matches ticket signature exactly. Tests override the var inside a `t.Cleanup`. | Adds one line of process state; the override pattern is already used elsewhere (e.g. `analyticsLogger` is also process-scoped). Honors the AC literally. |
| **C** — Amend the signature to `WritePack(distDir, species string, pack []byte) error` | Most explicit, no hidden state, easiest to test. | Departs from the literal AC, but the ticket is internal and the AC list is "shape" guidance, not contract. |

Going with **C**. The signature delta is one extra parameter,
the ticket's intent ("named helper, atomic, dir-creating") is
fully satisfied, the existing handler call site already has
`distDir` in scope, and tests stay hermetic without `os.Chdir`
or process-global state. I will note the deviation in `design.md`.

## `.gitignore` audit

`.gitignore` currently lists Go binaries, test artifacts,
`*.zip`, editor folders, `.lisa.toml`/`.lisa/`, and `.claude/`.
There is **no entry for `dist/`**. The directory does not yet
exist on disk (I checked: `ls dist` returns no such file), but
once `WritePack` runs, `dist/plants/*.glb` will be created in
the project root if `workDir` happens to be the repo (e.g. when
running `just run` from the repo). It must be ignored.

Add a single line `dist/` near the other generated-output entries.

## `just clean-packs` shape

The existing `justfile` is small (29 lines) and follows a "one
recipe, one shell line" convention. The closest precedent is
`clean:` which removes the binary:

```just
clean:
    rm -f glb-optimizer
```

For `clean-packs`, the safe-but-decisive form is:

```just
clean-packs:
    rm -rf dist/plants
    @mkdir -p dist/plants
    @echo "✓ cleaned dist/plants/"
```

The `mkdir -p` afterwards keeps the directory present so the
next pack write doesn't have to recreate it (and so `git status`
doesn't flicker). The `@` prefix on echo matches the existing
`down:` recipe's quiet-status convention.

Out of scope: `just clean-all` aggregating clean+clean-packs.
Mentioned in T-010-04, not here.

## Test surface

Three new test cases belong with `WritePack`:

1. **Happy path** — write a small payload, assert the file
   exists, contents match, mode is 0644, and **no `.tmp` file**
   remains in the directory. The leftover-tmp check is the
   thing that catches a bungled cleanup branch.
2. **Atomic on overwrite** — pre-populate `species.glb` with
   sentinel bytes, call `WritePack` with new bytes, assert the
   final file is the new bytes (rename replaced) and no tmp
   remains.
3. **Creates missing directory** — point at a subdirectory that
   does not yet exist, assert `WritePack` creates it.

A failure-injection case (read-only directory → error path
returns an error and leaves no tmp) is tempting but adds OS-
specific permission juggling. Skip unless review asks for it.

The handler-level test in `handlers_pack_test.go` already
exercises the end-to-end path; it should keep passing untouched
once the handler is migrated to `WritePack`.

## Risk and blast radius

Low. Single helper, single call-site migration, one constant,
one justfile recipe, one `.gitignore` line. No schema changes,
no API surface changes, no concurrency concerns (writeAtomic is
already battle-tested and the pack write is serialized through
the HTTP handler).

The one thing to watch: `writeAtomic` puts the temp file in the
**destination directory**. If `dist/plants/` is on a different
filesystem from `os.TempDir()`, that's exactly what we want
(rename is atomic only within a filesystem). On the demo laptop
this is moot — both live under `~/.glb-optimizer`.
