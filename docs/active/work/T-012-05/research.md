# T-012-05 Research — Stale Pack Cleanup

## Problem statement

`pack-all` writes one `.glb` per asset to `~/.glb-optimizer/dist/plants/`,
keyed on the resolved species slug, never on the upload hash id. After a
re-bake cycle that removes or renames an intermediate, the old pack file
remains in `dist/plants/` because nothing in the pipeline knows it is
"orphaned." T-010-04 review Open Concern #3 flagged this as a USB-copy
hazard for demo morning: stale species could land on the demo laptop and
either trip MANIFEST.txt validation or be loaded by the consumer.

## Relevant code surfaces

### Pack writer / dist path

- `pack_writer.go:12` — `const DistPlantsDir = "dist/plants"` is the
  canonical relative subpath under workDir. Every caller composes
  `filepath.Join(workDir, DistPlantsDir)` rather than hardcoding.
- `pack_writer.go:19` — `WritePack(distDir, species, pack)` is the only
  function that writes into `distDir`. The filename pattern is exactly
  `{species}.glb` — no version suffix, no namespacing, no extension
  variants. This means a stale-pack identifier only needs to walk
  `*.glb` in `distDir` and read the basename.
- `pack_cmd.go:36, 169, 211` — every CLI dispatcher (pack, pack-all,
  pack-inspect) resolves `distDir := filepath.Join(workDir, DistPlantsDir)`
  via `resolveWorkdir`, which already MkdirAlls the dir. The cleanup
  command can reuse `resolveWorkdir` verbatim.
- `main.go:80` — server initialisation does the same. There is no other
  process that writes into `dist/plants/`.

### Discovery of intermediate ids

- `pack_cmd.go:48` — `discoverPackableIDs(outputsDir)` walks `outputsDir`,
  filters entries ending in `_billboard.glb` (excluding the
  `_billboard_tilted.glb` false-positive), and returns the recovered ids
  in deterministic sorted order. This is the authoritative source of
  "what intermediates exist right now" — if an id is not in this list,
  there is no live source for it.

### Resolver

- `species_resolver.go:135` — `ResolveSpeciesIdentity(id, outputsDir,
  store, opts)` walks the six-tier fallback chain and returns
  `(SpeciesIdentity, ResolverSource, nil)`. The hash-fallback tier
  guarantees a non-empty result, so the function never errors today.
- The resolver's output for a given id is deterministic *given the
  same inputs* (mapping file, store contents, uploads.jsonl). For
  cleanup we want to know "what species would `pack-all` produce for
  this id today?" — running the same resolver against the same store
  is the only correct answer.
- T-012-04 (already shipped this session) added the uploads.jsonl
  manifest, so `originalsDir` rescans on a fresh process via
  `scanExistingFiles` are no longer the only source of truth — the
  resolver picks up renamed-after-restart cases from the manifest.

### Pack runner

- `pack_runner.go:40` — `RunPack(...)` calls
  `BuildPackMetaFromBake(id, ..., store, opts)` which itself calls
  `ResolveSpeciesIdentity` to populate `meta.Species`. So the species
  slug a pack file *was written under* is exactly what the resolver
  produces today, modulo intervening edits to overrides or the
  uploads manifest.

### CLI dispatch

- `main.go:25` — subcommand switch routes the first arg through
  `runPackCmd`, `runPackAllCmd`, `runPackInspectCmd`. Adding a new
  `clean-stale-packs` subcommand is the same one-line case + handler
  pattern that T-012-02 used for `pack-inspect`.

### justfile

- `justfile` already has `clean-packs` (rm -rf dist/plants — nuclear
  reset) and `pack-all`. The new `clean-stale-packs` recipe sits next
  to them.

## What "stale" means precisely

A pack file `dist/plants/{S}.glb` is stale iff there is no id in
`outputsDir` whose current resolver output has species slug == `S`.
Equivalently: build the set L = { ResolveSpeciesIdentity(id).Species |
id ∈ discoverPackableIDs(outputsDir) }. A pack file `S.glb` is stale
iff S ∉ L.

This is forward-mapping (id → species) plus set membership, which the
ticket lists as the "OR by checking if any intermediate in outputsDir
resolves to that species" path. The alternative (reverse map filename
→ id) is harder because the species slug strips the original hash and
collisions between two ids resolving to the same species are
indistinguishable from a stale-vs-live pair.

## Edge cases worth noting

1. **Two ids resolving to the same species.** Live — the slug is in L,
   so neither is removed. Acceptable; the operator's last `pack-all`
   would have overwritten the file anyway.
2. **Pack file whose basename does not parse as a species slug.**
   Example: a stray `.DS_Store` or hand-dropped `foo.bin`. The walker
   filters on `*.glb`, so `.DS_Store` is ignored. A `foo.glb` whose
   basename is not in L is stale by definition and gets removed.
3. **Empty `dist/plants/`.** Cleanup is a no-op, prints `0 stale`,
   exits 0.
4. **Empty `outputs/`.** L is empty; every pack in `dist/plants/` is
   stale. The dry-run default protects the operator from a misclick.
5. **Resolver picks a different slug after a manifest edit.** The old
   pack file has the old slug, the new resolver returns the new slug.
   The old file is correctly classified as stale.
6. **`--clean` after a partial pack-all failure.** Ticket says cleanup
   only runs after a *fully successful* pack-all. We gate on
   `failCount == 0` after the loop.

## Test infrastructure available

- `pack_cmd_test.go` shows the pattern: `t.TempDir()`, write fake
  files, call the function, assert. No live binaries needed.
- `pack_runner_test.go:25` shows the FileStore + ResolverOptions
  setup: `env := setupPackEnv(t)` then `RunPack(...)`. The cleanup
  tests can be lighter — they only need `discoverPackableIDs` (which
  reads files directly) and `ResolveSpeciesIdentity` (which can take
  a nil store).
- `humanBytes(int64) string` lives in `combine.go:181` and is reused
  by the pack summary table; the cleanup output should reuse it for
  the "removed: foo.glb (1.2 MB)" lines.

## Constraints from the ticket

- Dry-run default — `--apply` flag is required to actually delete.
- Removal failures log + continue, return aggregated error at end.
- `--clean` on `pack-all` runs only after `failCount == 0`.
- Cleanup output appended to the pack-all summary table (after the
  TOTAL line, before process exit).

## Out of scope (per ticket)

Versioned packs, intermediates cleanup, age-based GC, trash/undo,
backups. The dry-run default is the only safety net by design.
