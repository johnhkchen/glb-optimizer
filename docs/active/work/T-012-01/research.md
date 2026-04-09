# T-012-01 Research: Hash → Species Resolver

## Goal in one sentence

Replace the ad-hoc filename-derivation in `BuildPackMetaFromBake` with a
ranked, source-tagged resolver so the T-011-04 integration agent can pack
intermediates without hand-authoring per-asset `_meta.json` sidecars.

## Where the species id comes from today

`pack_meta_capture.go::BuildPackMetaFromBake` (current logic):

1. Looks up `store.Get(id)` → `FileRecord.Filename`. If the record exists
   AND the filename is non-empty AND it is *not* the post-restart sentinel
   `{id}.glb`, that filename is used.
2. Otherwise it falls back to the literal id.
3. Reads the optional `outputs/{id}_meta.json` (`captureOverride{Species,
   CommonName}`). Override wins over (1).
4. Runs `deriveSpeciesFromName` (lowercase, non-alnum→`_`, strip leading
   non-letters, collapse `__`) to produce a slug; empty → hard error
   pointing the operator at the override file.

So today the resolver is implicitly **two tiers**: sidecar > FileStore.
That works for the live server (uploads carry `Filename` in memory) but
breaks the moment a stale process is restarted, *or* whenever the demo
agent runs `glb-optimizer pack <id>` against an already-baked tree.

## On-disk reality (verified 2026-04-08)

```
~/.glb-optimizer/
├── originals/
│   └── 0b5820c3aaf51ee5cff6373ef9565935.glb     # ← only the hash
├── outputs/
│   └── 0b5820c3aaf51ee5cff6373ef9565935*.glb     # baked intermediates
└── (no uploads.jsonl, no originals/{id}_original.glb)
```

**Critical finding:** the originals directory stores files by content
hash only. There is *no* on-disk record anywhere of the upload-time
filename (e.g. `achillea_millefolium.glb`). The ticket's notes anticipated
this — "the path I'm assuming might not be exactly right". The original
filename only survives in `FileRecord.Filename` while the server process
that received the upload is alive. `scanExistingFiles` in `main.go` even
acknowledges the loss:

```go
record := &FileRecord{
    ID:           id,
    Filename:     e.Name(), // We lose original filename on restart
    ...
}
```

This means the resolution chain proposed in the ticket needs adjusting:

| Tier (ticket)            | On-disk source          | Status                           |
| ------------------------ | ----------------------- | -------------------------------- |
| 1. CLI override          | flags                   | feasible — wire through pack cmd |
| 2. `_meta.json` sidecar  | `outputs/{id}_meta.json`| already implemented              |
| 3. Original filename     | `originals/{id}_original.glb` | **does not exist** — only `{id}.glb` |
| 4. Upload manifest       | `~/.glb-optimizer/uploads.jsonl` | **does not exist yet** (T-012-04) |
| 5. Content hash fallback | n/a                     | feasible — hash → `species_<8>`  |

## Implication for ticket boundary

The ticket explicitly invites a research-time pivot:

> If `~/.glb-optimizer/originals/` doesn't store original filenames but
> only content (no provenance metadata), then the upload manifest tier
> becomes critical — and may not exist either. In that case T-012-04
> (persist original filename) becomes a hard prerequisite for this
> ticket. Document the finding in research.md and either escalate or
> proceed with the mapping-file flag as the primary tier.

Decision (rationale in `design.md`): **proceed without a hard
dependency on T-012-04**. The resolver will:

- Read `~/.glb-optimizer/uploads.jsonl` opportunistically — if T-012-04
  ships first, we get the tier "for free"; if not, the resolver simply
  skips that tier with no error.
- Promote the **mapping file** (`--mapping` flag on `pack-all`) and the
  **per-asset CLI flags** to be the operator's primary escape hatches
  for the demo. Both are zero-dependency and were already in scope.
- Keep the **FileStore in-memory filename** as a tier so today's HTTP
  upload flow continues to work without sidecars.

## Touched code surfaces

| File                       | Why                                                                 |
| -------------------------- | ------------------------------------------------------------------- |
| `pack_meta_capture.go`     | `BuildPackMetaFromBake` calls the new resolver; old derivation moves |
| `pack_cmd.go`              | `runPackCmd` gains `--species`/`--common-name`; `runPackAllCmd` gains `--mapping` |
| `pack_runner.go`           | `RunPack` signature: optional `ResolverOptions` threaded through    |
| `handlers.go::handleBuildPack` | Server flow: passes empty `ResolverOptions` → unchanged behaviour |
| **NEW** `species_resolver.go` | `ResolveSpeciesIdentity`, `SpeciesIdentity`, `ResolverSource`, `ResolverOptions`, mapping file loader |
| **NEW** `species_resolver_test.go` | Per-tier unit tests + normalisation tests                      |
| `pack_meta_capture_test.go` | Update existing tests for the new resolver signature; add an integration test |

## Constraints carried in from prior tickets

- **Slug regex (`pack_meta.go`)**: `^[a-z][a-z0-9_]*$`. Resolver MUST
  normalise *or* error — never return an invalid slug.
- **`titleCaseSpecies` already exists** in `pack_meta_capture.go`. Reuse
  it for the common-name fallback when only a species id is supplied.
- **`deriveSpeciesFromName` already exists**. Reuse it as the
  filename-normalisation primitive — do not re-implement.
- **No new dependencies.** Stdlib `encoding/json` + `os` is sufficient.
- **Logging**: every non-CLI tier logs a single `log.Printf` line at
  resolution time (`log` is already imported in `pack_meta_capture.go`).
- **Backward-compat**: The HTTP `handleBuildPack` path must keep working
  with no behavioural change for assets that still have a live
  `FileRecord.Filename`.

## Open questions answered during research

- **Q:** Where does the FileStore get populated for `pack`/`pack-all`?
  **A:** `pack_cmd.go::runPackCmd` calls `scanExistingFiles`, which
  fills `Filename` with `{id}.glb` (the post-restart sentinel). So the
  FileStore tier is effectively dead for the CLI flow today — only the
  HTTP upload flow benefits from it. This is fine: the CLI flow falls
  through to mapping/sidecar/hash tiers as designed.

- **Q:** Does the resolver need to *write* `_meta.json`?
  **A:** No. The ticket's "Out of Scope" section is explicit. The
  resolver is read-only; if the operator wants persistence they
  hand-author the sidecar (still supported) or ship T-012-04.

- **Q:** Should the mapping file be JSON or JSONL?
  **A:** JSON object `{hash: {species, common_name}}` per the ticket
  example. Easier to hand-edit; only the `pack-all` driver consumes it.
