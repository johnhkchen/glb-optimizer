# T-012-01 Design: Hash → Species Resolver

## Decision summary

A new file `species_resolver.go` exposes one function and one options
struct. `BuildPackMetaFromBake` is updated to call the resolver
exclusively for species/common-name; its old `loadCaptureOverride` +
`deriveSpeciesFromName` logic is *moved into* the resolver as two of the
five tiers, not duplicated.

```go
// species_resolver.go
type SpeciesIdentity struct {
    Species    string
    CommonName string
}

type ResolverSource int
const (
    SourceCLIOverride ResolverSource = iota
    SourceMappingFile
    SourceMetaJSON
    SourceFileStore
    SourceUploadManifest
    SourceContentHash
)

type ResolverOptions struct {
    // CLI: per-asset flags from `glb-optimizer pack <id> --species ... --common-name ...`
    CLISpecies    string
    CLICommonName string
    // pack-all: parsed contents of --mapping <file.json>
    Mapping map[string]SpeciesIdentity
    // Optional explicit path; defaults to ~/.glb-optimizer/uploads.jsonl
    UploadManifestPath string
}

func ResolveSpeciesIdentity(
    id, outputsDir string,
    store *FileStore,
    opts ResolverOptions,
) (SpeciesIdentity, ResolverSource, error)
```

## Resolution chain (final)

| # | Tier                | Source                                                  | Logged as              |
| - | ------------------- | ------------------------------------------------------- | ---------------------- |
| 1 | CLI override        | `opts.CLISpecies` / `opts.CLICommonName` (both required, or fall through) | `cli-override`        |
| 2 | Mapping file        | `opts.Mapping[id]`                                      | `mapping-file`         |
| 3 | `_meta.json` sidecar| `outputs/{id}_meta.json`                                | `meta-json`            |
| 4 | FileStore filename  | `store.Get(id).Filename` (rejecting `{id}.glb` sentinel)| `file-store`           |
| 5 | Upload manifest     | last entry for hash in `~/.glb-optimizer/uploads.jsonl` | `upload-manifest`      |
| 6 | Content hash        | derives `species_<first8>`; logs WARNING                | `content-hash`         |

Each tier is *probed* in order; the first one that yields a non-empty
species id wins. The resolver normalises the result through
`deriveSpeciesFromName` regardless of source so a hand-edited mapping
file with `"Achillea Millefolium"` still becomes `achillea_millefolium`.
If normalisation produces an empty string AND a later tier exists, the
chain continues; if no tier produces a usable id, the resolver
short-circuits to tier 6 (hash) — never returns an error from the
resolver itself. (Returning an error here would re-create the very
friction T-012-01 is supposed to remove.)

The single error path is "id is not a 32-char hex hash so the content
hash fallback is meaningless" — but even that is recoverable: we accept
the id verbatim so this case cannot fire in practice. Net: the resolver
never returns a non-nil error to callers.

## Alternatives considered

### Alt A: Keep two-tier chain, fail loudly when neither tier hits

**Why rejected:** That is what we have today, and it is exactly the
friction the ticket exists to remove. The agent attempting T-011-04
would still need to author one sidecar per asset.

### Alt B: Make T-012-04 (upload manifest) a hard prerequisite

**Why rejected:** T-012-01 and T-012-04 are flagged as parallel-ready in
the lisa DAG. Sequencing them would block T-011-04 unnecessarily. The
mapping-file tier achieves the same operator outcome (one file edit
unblocks pack-all) without the prerequisite.

### Alt C: Persist resolved identities to a sidecar after first resolution

**Why rejected:** The ticket "Out of Scope" section explicitly bans this
("Backfilling sidecars for all existing assets"). Also it would couple
the read path to the write path and surprise an operator who later
edits a mapping file expecting it to win.

### Alt D: Error if multiple tiers disagree

**Why rejected:** The chain exists *because* tiers disagree — the
operator wants priority order, not consensus. Disagreement is the
normal case, not an exception.

## Why the Mapping tier sits *above* the sidecar

The ticket explicitly says: *"mapping file beats sidecar but loses to
per-asset CLI override"*. Operator mental model: a mapping file is a
batch override the operator just authored; a sidecar may have been
written days ago by automation. Trust the fresher signal.

## Why the FileStore tier still exists

The HTTP upload flow (`handleBuildPack`) populates a real `Filename` in
the store at upload time. Removing this tier would force every UI
"Build pack" click to also write a sidecar. Keeping it is free —
sentinel detection (`Filename != id+".glb"`) already filters the
post-restart noise.

## Wiring changes

### `BuildPackMetaFromBake`

New parameter: `opts ResolverOptions`. Body shrinks: the override
loading + filename derivation + error path are deleted in favour of:

```go
identity, source, _ := ResolveSpeciesIdentity(id, outputsDir, store, opts)
log.Printf("pack_meta_capture: %s: species=%s common_name=%q source=%s",
    id, identity.Species, identity.CommonName, source)
species, common := identity.Species, identity.CommonName
```

The tilted-fade and footprint capture below remain untouched. The
`captureOverride` struct moves to `species_resolver.go` (it's now a
private detail of one tier, not of the capture entry point).

### `RunPack` and CLI

`RunPack` gains a `ResolverOptions` parameter, passed straight through
to `BuildPackMetaFromBake`. The HTTP handler passes
`ResolverOptions{}` (empty) so server behaviour is unchanged. The CLI
constructs it from `flag.FlagSet`:

```
glb-optimizer pack [--dir ...] [--species SLUG] [--common-name NAME] <id>
glb-optimizer pack-all [--dir ...] [--mapping FILE] 
```

For `pack-all`, the mapping file is loaded once and the same
`ResolverOptions{Mapping: ...}` is reused for every id in the loop.

## Test plan (sketch — full plan in `plan.md`)

Unit tests in `species_resolver_test.go`:

- T1: CLI override beats every other tier
- T2: Mapping file beats sidecar
- T3: Sidecar beats FileStore filename
- T4: FileStore filename used when no sidecar/mapping/CLI
- T5: FileStore `{id}.glb` sentinel is rejected — falls through
- T6: Empty everything → content-hash tier with WARNING log
- T7: Normalisation: `Plant A v1` → `plant_a_v1`
- T8: Mapping file with messy keys: leading digits, mixed case
- T9: Upload manifest tier wins when present and FileStore is empty

Updates to `pack_meta_capture_test.go`:

- Existing happy-path test still passes (FileStore tier active)
- Existing override test still passes (sidecar tier active)
- Add new integration test exercising full resolver from pack flow

## Rollout / risk

- **Risk:** an existing operator workflow that relies on
  `BuildPackMetaFromBake` returning an error when a filename is
  un-derivable. **Mitigation:** none — the ticket explicitly removes
  that error. The new behaviour falls back to the hash with a loud
  warning, which is strictly safer (you get a working pack labelled
  badly rather than no pack at all). Documented in `review.md`.

- **Risk:** the hash-tier slug `species_<first8>` could collide if two
  hashes share a prefix. **Mitigation:** 8 hex chars = 32 bits = ~1 in
  4 billion. For a demo set of <100 assets the collision probability is
  negligible. Logged with the full hash so the operator can audit.
