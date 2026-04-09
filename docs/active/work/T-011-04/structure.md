# T-011-04 Structure — file-level shape of the work

## Files this ticket creates or modifies

This ticket is a coordination protocol, not a code change. The
file footprint is intentionally tiny: artifacts in this repo's
work directory, mutations against an external repo's asset
manifest, and a beacon left in that external repo's progress.

### In this repo (`/Volumes/ext1/swe/repos/glb-optimizer`)

| Path | Action | Purpose |
|------|--------|---------|
| `docs/active/work/T-011-04/research.md`  | create | Phase 1 RDSPI artifact |
| `docs/active/work/T-011-04/design.md`    | create | Phase 2 RDSPI artifact |
| `docs/active/work/T-011-04/structure.md` | create | Phase 3 RDSPI artifact (this file) |
| `docs/active/work/T-011-04/plan.md`      | create | Phase 4 RDSPI artifact |
| `docs/active/work/T-011-04/progress.md`  | create | Phase 5 RDSPI artifact, holds the protocol audit trail |
| `docs/active/work/T-011-04/review.md`    | create | Phase 6 RDSPI artifact, handoff summary |

No production source files in this repo are modified. No tests are
added. `pack_cmd.go`, `combine.go`, and friends stay untouched.

### In `~/.glb-optimizer/` (build output, not git-tracked)

| Path | Action | Purpose |
|------|--------|---------|
| `dist/plants/`            | created (by `just pack-all`) | Output dir |
| `dist/plants/{species}.glb` | written (by `pack-all`)  | Hand-off candidate |

Build output. Cleanup is `just clean-packs`. Not in git.

### In plantastic (`/Users/johnchen/swe/repos/plantastic`)

| Path | Action | Purpose |
|------|--------|---------|
| `web/static/potree-viewer/assets/plants/{species}.glb` | create (cp) | Hand-off pack |
| `web/static/potree-viewer/assets/plants/MANIFEST.txt`  | edit | Add the new species id; comment out stale entries |
| `docs/active/work/T-083-05/progress.md`                | append (or create + write) | Cross-repo handoff beacon |

The third row is conditional: if T-083-05 has not yet produced a
`progress.md`, we create one with just the beacon section. If it
exists, we append a level-2 section without touching anything else.

## Anatomy of `progress.md`

Five sections, one per protocol phase, plus a final "DONE" stamp.
Each section has a fixed shape so future agents can grep for state:

```markdown
## Phase 1 — local prerequisites

- timestamp: <ISO8601>
- prereq scan: T-010-01..05 + T-011-01..03 — all `phase: done` ✓
- `just clean-packs`: <result>
- `just pack-all`: exit code <n>, summary table excerpt
- packs under 5 MB: <yes/no>
- extras.plantastic verification: `go test -run PackMeta` <result>
- conclusion: PASS / FAIL (if FAIL, reason + which upstream owns it)

## Phase 2 — wait for the consumer to be ready

- timestamp: <ISO8601>
- plantastic T-080-01 phase: <observed value>
- decision: proceed / pause-and-poll
- (if polling) timestamps and observations of each subsequent poll

## Phase 3 — hand off a real pack

- timestamp: <ISO8601>
- candidate selection rationale (smallest with most variants)
- pack filename, size_bytes, bake_id, sha256
- copy command + result
- MANIFEST.txt edit: lines added, lines commented out

## Phase 4 — verify the consumer accepts the pack

- timestamp: <ISO8601>
- `pnpm install` (if needed): <result>
- `pnpm build` in plantastic: <result, with prebuild script output>
- loader test result: <test name + pass/fail>
- (if fail) producer-side or consumer-side bug + fix-forward note

## Phase 5 — mark done

- timestamp: <ISO8601>
- verification result: <test name, pack file, sha256, timestamp>
- beacon left in plantastic T-083-05 progress.md: <yes/no>
- DONE
```

The shape is rigid so a future agent can resume mid-protocol if
this session is interrupted.

## Anatomy of the beacon in plantastic T-083-05

A single appended section, no more than ten lines:

```markdown
## Producer handoff — glb-optimizer T-011-04

- producer ticket: glb-optimizer T-011-04
- pack file: web/static/potree-viewer/assets/plants/<species>.glb
- species id: <species>
- bake_id: <RFC3339 UTC>
- sha256: <hex>
- timestamp: <ISO8601>
- producer Phase 4 result: PASS
```

That is the entirety of the consumer-facing artifact. It carries
just enough information for the consumer-side agent to proceed
without re-running anything in this repo.

## Decision points where the structure may grow

There are exactly two structural escape hatches the implement
phase may invoke. Both produce small new files; both are
conditional on a specific Phase failure.

1. **`pack_cmd.go: pack-inspect <species>` subcommand.**
   Created only if Phase 1 succeeds at the surface but we want
   human-eyes confirmation that `extras.plantastic` parses. Would
   live alongside the existing `pack` and `pack-all` subcommands,
   ~30 lines, calls `pack_meta_capture.go` helpers.

2. **`web/src/lib/three/plant-pack.integration.test.ts` in plantastic.**
   Created only if Phase 4's `pnpm build` route does not invoke
   the real `GLTFLoader` against the dropped pack file. Would
   load the file via the real loader and assert the prototypes
   parse. Skipped in CI (gated on file presence).

Neither is created speculatively. They live in this structure
document so the implement phase has a known landing pattern if
escalation is needed.

## What the structure intentionally does not include

- No new production Go code in this repo.
- No new tests in this repo (existing `pack_meta_test.go` covers
  the round trip the protocol asks about).
- No CI changes.
- No edits to `combine.go`, `pack_runner.go`, or `pack_writer.go`.
- No changes to `MANIFEST.txt` *format* — only content.
- No git commits to plantastic source files outside the asset
  directory and the T-083-05 work directory.

## Ordering constraint

Phases run strictly in order. Within a phase, steps are also
ordered (the protocol is sequential). The only ordering decision
that matters is: **never edit the consumer MANIFEST before the
hand-off file is in place.** If we list a species id whose file
is missing, the prebuild script breaks and we cannot tell whether
it was our hand-off that broke it or the pre-existing stale
entries. Order: copy GLB first, then edit MANIFEST, then run
`pnpm build`.
