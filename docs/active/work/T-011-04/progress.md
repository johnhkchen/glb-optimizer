# T-011-04 Progress — handshake protocol execution

Audit trail of the producer-side handshake. One section per
protocol phase, per the structure.md template.

## Phase 1 — local prerequisites

- timestamp: 2026-04-08 12:31 PDT
- prereq scan: T-010-01..05 + T-011-01..03 — all `phase: done` ✓
  (verified with `grep -H "^phase:" docs/active/tickets/T-010-0*.md docs/active/tickets/T-011-0*.md`)
- `just clean-packs`: ok, `dist/plants/` empty
- `just pack-all`: **exit code 1**

  ```
  2026/04/08 12:31:44 pack_meta_capture: 0b5820c3aaf51ee5cff6373ef9565935:
    no bake stamp at /Users/johnchen/.glb-optimizer/outputs/0b5820c3aaf51ee5cff6373ef9565935_bake.json,
    falling back to current time as bake_id; rebake to get a stable id
  SPECIES                           SIZE  TILTED  DOME  STATUS
  0b5820c3aaf51ee5cff6373ef9565935  -     yes     yes   oversize
                                                        pack "b5820c3aaf51ee5cff6373ef9565935" exceeds 5 MB limit (actual: 8.0 MB)
  TOTAL: 1 packs, 0 ok, 1 failed
  ```

- packs under 5 MB: **NO** — the only candidate (`0b58...`) combines
  to 8.0 MB, exceeding the 5 MiB cap enforced by `PackOversizeError`
  in `combine.go` (T-010-05).
- extras.plantastic verification: `go test ./... -run PackMeta` →
  `ok glb-optimizer 0.281s` (codec is healthy; the cap failure is
  upstream of metadata).
- conclusion: **FAIL**

### Failure analysis

Two distinct producer-side issues surfaced:

1. **Oversize pack (blocker).** The single bakeable species
   (`0b5820c3aaf51ee5cff6373ef9565935`) produces an 8.0 MB pack,
   3 MB over the demo cap. Source bake sizes from
   `~/.glb-optimizer/outputs/`:

   | File | Size |
   |------|------|
   | `_billboard.glb`         | 1.8 MB |
   | `_billboard_tilted.glb`  | 1.4 MB |
   | `_lod0.glb`              | 10 MB  |
   | `_lod1.glb`              | 7.8 MB |
   | `_lod2.glb`              | 7.4 MB |
   | `_lod3.glb`              | 7.3 MB |
   | `_volumetric.glb`        | 736 KB |

   The combine pipeline picks the cheapest LOD it can for `view_top`
   plus the billboards and volumetric, but even the smallest selection
   over the 5 MB cap. This is a producer-side issue to fix forward in
   T-010-05 (cap policy) or, more likely, in whichever ticket owns
   the bake-side decimation budget for `_lod*.glb` outputs. T-011-04
   itself does **not** fix this.

2. **Missing bake stamp (warning, not blocker).** The capture path
   logged a fallback for `bake_id` because
   `~/.glb-optimizer/outputs/0b5820c3aaf51ee5cff6373ef9565935_bake.json`
   does not exist. This bake predates T-011-03 and was therefore
   never stamped. The fallback path works as designed, so this is
   only a warning — but it does mean the `bake_id` is unstable
   across pack runs of the same intermediates. T-011-03 acceptance
   criteria explicitly anticipate this: "If `outputs/{id}_bake.json`
   is absent (e.g., bake ran before this ticket), `BuildPackMetaFromBake`
   falls back to the current time and logs a warning". So T-011-03
   itself is correct; the remediation for the warning is to rebake
   the asset, which is upstream of this ticket.

3. **Species slug derivation drops the leading hex `0`.** The error
   message refers to the species as `b5820c3aaf51ee5cff6373ef9565935`
   (no leading `0`). This is `deriveSpeciesFromName` stripping
   leading non-letter characters. For real species names this is
   correct (`-rose-julia` → `rose_julia`); for content-hash ids it
   chews off significant bytes. Not a blocker because the error
   message is the only place this slug appears in this run, but it
   means the resulting filename in `dist/plants/` would be
   `b5820c3....glb` rather than `0b5820c3....glb`. Worth filing a
   producer-side observation, but not in scope for this ticket.

### Status decision

Per the protocol:

> If any prereq fails: this ticket stays in `phase: implement`.
> Fix forward in the responsible upstream ticket. Do NOT proceed
> to Phase 2.

Phase 1 has failed. **Phases 2–5 are not executed.** The ticket
remains in `implement` until an upstream fix lets `pack-all` emit
at least one pack under 5 MB.

## Phase 2 — wait for the consumer to be ready

- not executed (Phase 1 failed)
- side note from research: plantastic T-080-01 is already at
  `phase: done`, so Phase 2 would have been a no-op had we reached
  it. The blocker is purely producer-side.

## Phase 3 — hand off a real pack

- not executed (Phase 1 failed)

## Phase 4 — verify the consumer accepts the pack

- not executed (Phase 1 failed)

## Phase 5 — mark done

- not executed (Phase 1 failed)
- DONE flag NOT set

## Recommended next actions (for the upstream owner)

1. Audit the LOD-budget config that produced the 7-10 MB
   `_lod*.glb` outputs. The combine pipeline cannot squeeze 8 MB
   into 5 MB without dropping a variant; the right fix is at the
   bake stage, not at combine. Likely owners: whichever epic
   shipped the per-LOD decimation budgets (E-002 cousin tickets,
   not in this story).
2. Rebake the asset after the LOD-budget fix so that
   `outputs/{id}_bake.json` lands and `bake_id` becomes stable.
   This also clears the warning surfaced in Phase 1 step (c).
3. (Optional, smaller) File a follow-up against `pack_cmd.go`
   `deriveSpeciesFromName` so it does not chew leading hex digits
   off content-hash species ids. The content-hash slug case is
   unusual but real for raw bake outputs.
4. Re-run `just pack-all`. When the TOTAL row reads `1+ ok, 0
   failed`, this ticket can resume from Phase 1 step (c) without
   re-running any earlier work.

## Re-entry checklist for the next agent

When `pack-all` next succeeds, the next agent picks up here:

- [ ] Re-run `just clean-packs && just pack-all` — confirm green
- [ ] Append a new "Phase 1 retry" subsection above with the new
      timestamp, output, and PASS conclusion
- [ ] Continue at Phase 2 (consumer-ready check)
- [ ] Continue at Phase 3 (pick smallest pack with most variants,
      record sha256 + bake_id, copy to plantastic, edit MANIFEST)
- [ ] Continue at Phase 4 (`pnpm build` in plantastic)
- [ ] Continue at Phase 5 (beacon in plantastic T-083-05, DONE
      stamp here)

The plan.md commands are still accurate; nothing about the
research, design, or structure needs to be redone.
