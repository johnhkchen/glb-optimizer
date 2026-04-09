# T-011-04 Review — handshake gate, blocked at Phase 1

## TL;DR

This ticket is a coordination protocol, not a code change. Five
RDSPI artifacts (research, design, structure, plan, progress) plus
this review were produced. The Implement phase executed Phase 1 of
the protocol and **hit a producer-side blocker**: the only
bakeable species combines to an 8.0 MB pack, which exceeds the
5 MiB cap enforced by `PackOversizeError`. Per the protocol, the
ticket therefore stays in `phase: implement` until upstream fixes
the bake-output budgets and `pack-all` emits at least one pack
under 5 MB. No production source was touched in this repo. No
files were transferred to plantastic.

## Files created

| Path | Lines | Purpose |
|------|------:|---------|
| `docs/active/work/T-011-04/research.md`  | ~190 | Prereq scan, bake inventory, risks |
| `docs/active/work/T-011-04/design.md`    | ~180 | Four decisions with rejected alternatives |
| `docs/active/work/T-011-04/structure.md` | ~150 | File-level footprint of the protocol |
| `docs/active/work/T-011-04/plan.md`      | ~210 | Five steps mapped to the five protocol phases |
| `docs/active/work/T-011-04/progress.md`  | ~150 | Phase 1 execution audit + re-entry checklist |
| `docs/active/work/T-011-04/review.md`    | this | Handoff summary |

## Files modified outside this work directory

**None.** Specifically:

- No production Go files in this repo.
- No test files in this repo.
- No files in plantastic. The hand-off pack was never copied
  because Phase 1 did not produce a passing candidate.
- No edits to plantastic's `MANIFEST.txt`.
- No beacon written into plantastic T-083-05's `progress.md`.

## What ran

| Command | Result |
|---------|--------|
| `grep "^phase:" docs/active/tickets/T-010-0*.md docs/active/tickets/T-011-0*.md` | All eight upstream tickets at `phase: done` |
| `just clean-packs` | ok |
| `just pack-all` | **exit 1**, `0 ok, 1 failed (oversize 8.0 MB)` |
| `go test ./... -run PackMeta` | ok (codec is healthy) |

## Test coverage

This ticket adds no tests. The verification for a coordination
protocol is the protocol itself; the unit-test coverage that
matters is the upstream Pack v1 codec, which already exists in
`pack_meta_test.go` and was rerun successfully as part of Phase 1.

There are **no test gaps introduced by this ticket** because
there is no production code to cover.

## Open concerns

### Critical (blocks `done`)

1. **8 MB pack vs. 5 MB cap.** The only bakeable species
   (`0b5820c3aaf51ee5cff6373ef9565935`) combines to 8.0 MB. The
   underlying `_lod*.glb` files are 7-10 MB each, leaving the
   combine pipeline no slack to fit under the cap. This requires
   a producer-side fix in whichever ticket owns the per-LOD
   decimation budgets — **not** in this ticket. Until the bake
   budgets are tightened, there is no pack to hand off and the
   handshake cannot proceed.

### Notable (does not block, but worth fixing)

2. **Missing `bake_id` stamp on the existing bake.** The bake
   predates T-011-03 and lacks `outputs/{id}_bake.json`, so
   `BuildPackMetaFromBake` falls back to wallclock and logs a
   warning. T-011-03's fallback path is working as designed.
   Remediation is to rebake after T-011-03 was merged so the
   stamp lands.

3. **`deriveSpeciesFromName` chews leading hex digits.** The
   content-hash id `0b5820...` becomes the species slug
   `b5820...` because the leading `0` is non-letter and gets
   stripped. Harmless for human-named species; significant for
   raw content-hash bake ids. Worth a follow-up issue against
   `pack_cmd.go`. Not a blocker for this ticket.

### Resolved during execution

- **Consumer readiness.** The protocol's Phase 2 worried about
  waiting on plantastic T-080-01 to ship. Research-time scan
  confirmed T-080-01 is already `phase: done`. If Phase 1 had
  passed, Phase 2 would have been a no-op.
- **Stale `MANIFEST.txt` entries in plantastic.** Design picked
  the comment-out approach for `achillea_millefolium` and
  `coffeeberry`, but since Phase 1 failed we never reached the
  point of editing the MANIFEST. The design call survives for
  the next agent to apply when retry succeeds.

## Critical issues for human attention

- **The handshake cannot complete on this workdir until the
  producer-side oversize bug is fixed.** This is the human's
  decision: tighten the LOD-decimation budgets, or relax the
  5 MB cap (T-010-05's policy explicitly resisted this), or bake
  a different species. None of those choices belong inside
  T-011-04's scope.
- **Re-entry is cheap.** When the upstream fix lands, the next
  agent re-runs `just pack-all` and continues at Phase 1 step (c)
  per the checklist at the bottom of `progress.md`. Research,
  design, structure, and plan do **not** need to be redone.

## Why the artifacts still exist even though the protocol blocked

The user's task instructions called for all six RDSPI artifacts to
be written in a single pass. The artifacts also serve crash
recovery: when the upstream fix lands, the next agent has a
complete understanding of the protocol state, the exact commands
to re-run, and a clear pass-criteria definition without needing to
re-derive any of it. That is what RDSPI's "artifacts are
insurance" rule is designed for.

## Frontmatter status

Per task instructions, the ticket's `phase` and `status` fields
were **not** modified by this work. Lisa's automation will read
the artifacts and decide the appropriate next phase. The expected
outcome is that the ticket stays in `implement` (or moves to a
blocked state Lisa recognizes), since `progress.md` explicitly
records a Phase 1 failure with no DONE stamp.
