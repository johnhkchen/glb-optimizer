---
id: T-012-01
story: S-012
title: hash-to-species-resolver
type: task
status: open
priority: critical
phase: done
depends_on: []
---

## Context

Source GLBs uploaded to glb-optimizer are stored under content-hash filenames in `~/.glb-optimizer/outputs/{hash}.glb`. The original upload filename (e.g., `achillea_millefolium.glb`) is preserved separately in `~/.glb-optimizer/originals/{hash}_original.glb` (or wherever the upload pipeline places it — verify the actual location during research phase).

T-011-02's `BuildPackMetaFromBake` already supports a per-asset `_meta.json` sidecar that overrides species id and common name. But those sidecars don't exist on disk for the existing intermediates, and asking a human to author one per asset is the friction point T-011-04's agent will hit first.

This ticket builds the resolver: a function that, given an asset id (hash), returns a `{species, common_name}` tuple by walking a chain of fallback sources, with each source clearly logged so the operator knows which one was used.

## Acceptance Criteria

### Resolver function

- New function `ResolveSpeciesIdentity(assetID string) (SpeciesIdentity, ResolverSource, error)` in `pack_meta_capture.go` (or a new `species_resolver.go`)
  - `SpeciesIdentity{ Species, CommonName string }`
  - `ResolverSource` is an enum: `SourceMetaJSON`, `SourceOriginalFilename`, `SourceUploadManifest`, `SourceContentHash`, `SourceCLIOverride`
- Resolution order (each tier is tried in turn; first hit wins):
  1. **CLI override** — if a `--species <id> --common-name <name>` flag was passed for this asset id, use it
  2. **`_meta.json` sidecar** — `~/.glb-optimizer/outputs/{id}_meta.json` with `{species, common_name}` keys
  3. **Original filename** — read `~/.glb-optimizer/originals/{id}_original.glb` (or whatever the actual original-storage path is) and extract its filename. Strip extension, lowercase, normalize non-alphanum to underscore. Generate common name as title-case with underscores → spaces.
  4. **Upload manifest** — if the server keeps an upload-time provenance log (e.g., `~/.glb-optimizer/uploads.jsonl`), look up the entry for this hash and read the original filename from there
  5. **Content hash fallback** — last resort: use the hash itself as the species id, log a clear warning, set common name to "Unknown Species (hash...)"
- The resolved `SpeciesIdentity.Species` MUST match `^[a-z][a-z0-9_]*$`. If the resolved value doesn't, the resolver normalizes it (strip leading digits, lowercase, replace invalid chars). If normalization produces an empty string, return error.
- Return value includes the `ResolverSource` so the caller can log which tier was used (critical for operator visibility)

### Wiring into existing flow

- `BuildPackMetaFromBake(id)` is updated to call `ResolveSpeciesIdentity(id)` for the species + common_name fields, replacing its current ad-hoc filename derivation
- The CLI subcommand `glb-optimizer pack <id>` accepts new flags: `--species <id>` and `--common-name <name>` to thread through to `ResolveSpeciesIdentity` as the highest-priority source
- `glb-optimizer pack-all` accepts an optional `--mapping <file.json>` flag pointing to a JSON file like:
  ```json
  { "0b5820c3aaf51ee5cff6373ef9565935": { "species": "achillea_millefolium", "common_name": "Common Yarrow" } }
  ```
  Each entry overrides resolution for that asset id during the batch run

### Tests

- Unit test: `_meta.json` sidecar is read and used
- Unit test: original filename is parsed and normalized correctly (e.g., `achillea_millefolium.glb` → `species=achillea_millefolium, common_name=Achillea Millefolium`)
- Unit test: invalid chars in source filename are normalized (`Plant A v1.glb` → `plant_a_v1`)
- Unit test: empty fallback (no sidecar, no original, no manifest) returns the hash + warning, source = `SourceContentHash`
- Unit test: CLI override beats all other sources
- Unit test: mapping file beats sidecar but loses to per-asset CLI override
- Integration test: pack a real intermediate from `~/.glb-optimizer/outputs/` using auto-resolution, verify the produced pack has a sensible species id

## Out of Scope

- Editing original filenames after the fact (resolver is read-only)
- Building a UI for managing the mapping (CLI + sidecar files are sufficient for the demo)
- Cross-asset deduplication (two hashes mapping to the same species — that's a bake-side concern)
- Backfilling sidecars for all existing assets (the resolver makes them unnecessary)

## Notes

- This is the highest-leverage ticket in S-012. If T-011-04's agent has only this support ticket available, it can complete the handshake without manual sidecar work as long as the original filenames are still in `~/.glb-optimizer/originals/`.
- During research phase, **first inspect `~/.glb-optimizer/originals/`** to confirm what's actually there. The path I'm assuming might not be exactly right — the resolver should be coded against the real layout, not my guess.
- If `~/.glb-optimizer/originals/` doesn't store original filenames but only content (no provenance metadata), then the upload manifest tier becomes critical — and may not exist either. In that case T-012-04 (persist original filename) becomes a hard prerequisite for this ticket. Document the finding in research.md and either escalate or proceed with the mapping-file flag as the primary tier.
