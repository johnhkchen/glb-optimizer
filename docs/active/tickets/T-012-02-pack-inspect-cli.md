---
id: T-012-02
story: S-012
title: pack-inspect-cli
type: task
status: open
priority: high
phase: done
depends_on: [T-010-02]
---

## Context

After `glb-optimizer pack-all` writes packs to `dist/plants/`, the only way today to verify what's actually inside one of those files is to load it in three.js or write a one-off Go test against `pack_writer.go`'s parser. The handshake's Phase 1 ("verify pack is valid before handoff") and demo-morning sanity checks both need a fast, scriptable inspect command.

## Acceptance Criteria

### CLI subcommand

- New subcommand: `glb-optimizer pack-inspect <species_id_or_path>`
  - If the argument matches `^[a-z][a-z0-9_]*$`, treat as species id and look up `~/.glb-optimizer/dist/plants/{id}.glb`
  - Otherwise treat as a filesystem path
  - Errors loud if the file doesn't exist
- Default output (human-readable, fits in a terminal):
  ```
  pack: achillea_millefolium.glb
    path:        ~/.glb-optimizer/dist/plants/achillea_millefolium.glb
    size:        1.84 MB (1842311 bytes)
    sha256:      a3f4...92b1
    format:      Pack v1
    bake_id:     2026-04-08T19:42:00Z

  metadata
    species:           achillea_millefolium
    common_name:       Common Yarrow
    canopy_radius_m:   0.45
    height_m:          0.62
    fade.low_start:    0.30
    fade.low_end:      0.55
    fade.high_start:   0.75

  variants
    view_side:    4 variants × avg 312 KB
    view_top:     1 quad   × 218 KB
    view_tilted:  4 variants × avg 287 KB
    view_dome:    6 slices × avg 41 KB

  validation: OK
  ```
- `--json` flag emits the same data as JSON for scripting
- `--quiet` flag prints only sha256 + size + validation status (one line, for shell pipelines)
- Exit code is non-zero if the pack fails Pack v1 schema validation, zero otherwise

### Validation logic reuse

- `pack-inspect` MUST reuse the existing `PackMeta.Validate()` and the chunk parser from combine — no parallel implementation
- Validation failure prints a clear list of which schema rules were violated (missing field, invalid range, wrong type) — same format as the combine-time errors

### Tests

- Unit test: pack-inspect on a valid synthetic pack produces correctly-populated output
- Unit test: pack-inspect on a pack with missing optional variants reports them as `(absent)` rather than crashing
- Unit test: pack-inspect on a malformed file (truncated, wrong magic) returns clear error + non-zero exit
- Unit test: `--json` produces parseable JSON whose shape matches a documented schema
- Snapshot test: human-readable output for a known fixture pack matches a stored expected file (regenerate carefully on intentional format changes)

## Out of Scope

- Editing pack metadata in place
- Comparing two packs (a `pack-diff` subcommand is its own future ticket)
- Rendering the pack to an image (out of CLI scope; that's the production preview)
- Listing packs in `dist/plants/` (use `ls`; don't reinvent it)

## Notes

- This is the producer agent's main "did the pack come out right?" tool. Make the human-readable output **terse and scannable** — an operator should be able to spot a missing variant or out-of-range fade band in under three seconds.
- The sha256 line is the bridge to the handshake protocol: T-011-04 records sha256 in `progress.md`, and T-083-05 verifies the same sha256 after USB drop. Make sure pack-inspect's sha256 format matches the format the handshake protocol uses (lowercase hex, no separators).
