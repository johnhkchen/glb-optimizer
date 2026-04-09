---
id: T-012-05
story: S-012
title: stale-pack-cleanup
type: task
status: open
priority: low
phase: done
depends_on: [T-011-01]
---

## Context

T-010-04's review noted Open Concern #3: "No cleanup of stale `dist/plants/{species}.glb`." When the operator regenerates packs after deleting or renaming intermediates, leftover packs from the previous generation linger in `dist/plants/` and could get USB-copied to the demo laptop alongside fresh files. The result on demo morning: a species that was supposed to be removed shows up in `MANIFEST.txt` validation as an unexpected file (warning) or, worse, gets loaded by the consumer because the operator forgot to update the manifest.

This ticket adds two cleanup paths: a standalone `just clean-stale-packs` recipe and a `--clean` flag on `pack-all` that removes packs whose source intermediates no longer exist.

## Acceptance Criteria

### Cleanup logic

- New function `IdentifyStalePacks(distDir, outputsDir string) ([]string, error)`
  - For each `.glb` in `distDir`: extract the species id from its filename
  - Look up the source intermediate for that species via `ResolveSpeciesIdentity` (T-012-01) reverse-mapping, OR by checking if any intermediate in `outputsDir` resolves to that species
  - A pack is stale if no current intermediate maps to its species id
  - Returns the list of stale pack paths
- New function `RemoveStalePacks(stale []string, dryRun bool) error`
  - With `dryRun=true`: prints what would be removed, removes nothing
  - With `dryRun=false`: removes each file, prints what was removed
  - Removal failures (permission, etc.) log + continue, return aggregated error at the end

### CLI integration

- New justfile recipe: `just clean-stale-packs` — invokes `glb-optimizer clean-stale-packs --dir ~/.glb-optimizer/outputs --dist ~/.glb-optimizer/dist/plants` in dry-run mode by default
- `just clean-stale-packs -- --apply` actually deletes (or use `just clean-stale-packs-apply` if `--` passthrough is awkward in this justfile dialect)
- New `--clean` flag on `glb-optimizer pack-all` runs cleanup AFTER successful packing (so a failed pack-all doesn't trigger cleanup)
- Cleanup output is appended to the pack-all summary table:
  ```
  Cleaned stale packs:
    - removed: old_species_a.glb (1.2 MB)
    - removed: old_species_b.glb (0.9 MB)
  ```

### Tests

- Unit test: identify stale with synthetic dist + outputs dirs
- Unit test: empty dist returns empty stale list
- Unit test: all-stale dist returns all packs
- Unit test: removal in dry-run mode leaves files in place
- Unit test: removal with apply mode deletes files
- Unit test: removal of non-existent file logs warning, continues
- Integration test: pack-all + --clean removes a stale pack from a previous run

## Out of Scope

- Versioned packs (`{species}-v1.glb`, `{species}-v2.glb`) — the design is one pack per species
- Cleanup of stale intermediates in `outputs/` (different problem)
- Automatic cleanup based on file age
- Trash / undo (deletions are permanent — the dry-run default is the safety net)
- Backup before delete (out of scope; operator's responsibility)

## Notes

- Dry-run is the default for safety. Operators should always run cleanup once in dry-run before applying.
- The `--clean` flag on pack-all is convenient for demo-morning regeneration: `just pack-all --clean` produces a fresh, complete `dist/plants/` from current intermediates with no leftovers.
- Lowest priority in S-012. Only valuable when re-baking; one-time bake doesn't need it.
