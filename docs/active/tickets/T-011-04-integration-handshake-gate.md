---
id: T-011-04
story: S-011
title: integration-handshake-gate
type: task
status: open
priority: critical
phase: done
depends_on: [T-010-04, T-011-03]
---

## Context

Pipeline tickets in E-002 can finish without ever proving that a pack from this repo is actually consumable by plantastic. Schemas pass round-trip in Go tests; combine produces a `.glb`; the file looks valid. None of that is the same as "the consumer's loader successfully parses one of our packs and the consumer's instancer successfully renders it."

This ticket is the **producer-side half** of a two-sided integration handshake. Its only job is to gate the moment glb-optimizer can declare its E-002 work shipped: a real pack from `dist/plants/` must demonstrably load in plantastic. There is no automated cross-repo CI; the handshake is performed by a coding agent (or human) who reads both repos' `lisa status`, transfers a pack file, and verifies the consumer accepts it.

The plantastic counterpart is **plantastic T-083-05**. The two tickets are mutually aware via this protocol — neither marks done until both have verified the round trip.

## Coordination Protocol

This is the protocol a coding agent follows. Steps are sequential and each is observable so the agent never sits idle waiting on an unspecified condition.

### Phase 1 — local prerequisites

1. Confirm `lisa status` in this repo shows all of `T-010-01..05` and `T-011-01..03` at `phase: done`
2. Run `just pack-all`. Verify:
   - Exit code 0
   - At least one pack file exists in `dist/plants/`
   - All packs are under 5 MB
   - `extras.plantastic` parses cleanly on a sample pack via a quick Go test or a `glb-optimizer pack-inspect <species>` debug subcommand

If any prereq fails: this ticket stays in `phase: implement`. Fix forward in the responsible upstream ticket. Do NOT proceed to Phase 2.

### Phase 2 — wait for the consumer to be ready

1. Run `lisa status` in the **plantastic** repo (path: `/Users/johnchen/swe/repos/plantastic` on the dev laptop, or whatever the agent's local clone is)
2. Look for ticket `T-080-01` in the output
3. If `T-080-01` is at `phase: done`: proceed to Phase 3
4. If `T-080-01` is at `phase: ready` / `research` / `design` / `structure` / `plan` / `implement` / `review`: this ticket is **time-gated**. Pause here. Re-run `lisa status` in plantastic periodically (or after any nudge) until the phase advances. Document each poll in this ticket's `progress.md` so the agent's wait state is auditable.
5. If `T-080-01` does not exist: investigation needed — the consumer-side loader story may have been renumbered. Stop and ask.

### Phase 3 — hand off a real pack

1. Pick the smallest pack from `dist/plants/` that has all four variants (`view_side`, `view_top`, `view_tilted`, `view_dome`). If no pack has all four, pick the smallest with at least `view_side` + `view_top` + one of (tilted, dome).
2. Record in `progress.md`: pack filename, size in bytes, `bake_id`, sha256 of the file
3. Copy the pack file to the agreed transfer location. The agreed location for the demo is **the dev laptop's plantastic clone** at `web/static/potree-viewer/assets/plants/`. If the agent is not running on the dev laptop, the agreed alternate is a temp directory both repos can read; document which.
4. Update `MANIFEST.txt` in plantastic to list this pack's species id (the agent has read access to the other repo per the user's instruction)

### Phase 4 — verify the consumer accepts the pack

1. In the plantastic repo, run `pnpm install` if needed
2. Run `pnpm build` — verify the prebuild registry script accepts the new pack file (no validation errors)
3. Run plantastic's loader test (per T-080-01 acceptance criteria; the test path is documented there). Verify it passes against the real pack file, not a hand-rolled mock.
4. If any step fails: the gate is **NOT done**. Capture the error in `progress.md`, identify whether the bug is producer-side or consumer-side, and either fix forward in this ticket (producer bug) or comment on plantastic T-083-05 + the relevant upstream consumer ticket (consumer bug). Pause this ticket until the responsible side ships a fix.

### Phase 5 — mark done

Only when Phase 4 ends with green:
1. Update `progress.md` with the verification result (test name, pack file used, timestamp)
2. Set `phase: done`
3. Notify plantastic T-083-05 by leaving a comment in its progress.md noting the pack filename + verification timestamp, so the consumer-side gate can proceed

## Acceptance Criteria

- `phase: done` is set ONLY after a real pack from this repo's `dist/plants/` has been verified to parse and load via plantastic's loader test against an actual `.glb` file (not a mock)
- `progress.md` records the full Phase 1-4 audit trail (poll timestamps, file sha256, test result)
- A pack file has been physically transferred to plantastic's asset directory
- A note has been left in plantastic T-083-05's progress.md identifying which pack was handed off

## Out of Scope

- Automating the handshake (CI integration is a future epic)
- Verifying multiple species at once (one is enough to prove the format works; bulk happens at demo prep time)
- Verifying the rendered output looks correct (that's plantastic T-083-05's job)
- Cross-repo deadlock recovery (if both gates wait forever, escalate to a human)

## Notes

- The polling-on-other-repo's-`lisa status` pattern is the agreed coordination primitive for this integration. It's slower than direct messaging but it's auditable: every state change in either repo is visible to the other agent without any shared infrastructure beyond the filesystem.
- The mutual wait between this ticket and plantastic T-083-05 has a built-in deadlock-breaker: T-011-04 only waits for plantastic T-080-01 (a Wave-1 ticket that finishes long before the consumer-side gate), then proceeds to ship a pack. T-083-05 waits for THIS ticket to be in `phase: implement` before its own Phase 4 verification. The producer always moves first.
- The dev laptop is the canonical execution environment for the handshake. If the handshake happens on a different host, document that in `progress.md` so the demo-day agent knows which artifacts are where.
