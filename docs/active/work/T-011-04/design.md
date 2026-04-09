# T-011-04 Design — handshake execution strategy

## The shape of the problem

This is not a code-writing ticket. The deliverable is an audit trail
proving a specific producer→consumer round trip happened. Most of
the "design" choices are about how to execute the protocol such that
each step is observable, attributable, and recoverable.

The five protocol phases are sequential and well-specified by the
ticket itself, so the design space is narrow. What is **not** pinned
by the ticket is:

1. How to verify `extras.plantastic` parses (Phase 1 step 2 final
   check) — Go test vs. new `pack-inspect` subcommand
2. How to make plantastic's loader test consume a **real** pack file
   when the existing test only exercises a mocked GLTFLoader
3. What to do about the stale `MANIFEST.txt` entries
   (`achillea_millefolium`, `coffeeberry`) that block `pnpm build`
4. Where to record the audit trail in this repo's `progress.md` vs.
   what to mirror into plantastic T-083-05

## Decision 1 — `extras.plantastic` verification path

### Options

**A. Add a `pack-inspect <species>` subcommand to `pack_cmd.go`.**
Reads `dist/plants/{species}.glb`, decodes the `extras.plantastic`
block, prints a human-readable summary plus JSON. Exits non-zero on
parse failure.

**B. Use the existing `pack_meta_test.go` round-trip coverage as
sufficient evidence and skip a runtime check.**

**C. Write an inline `go test -run` invocation that loads the actual
emitted pack and asserts the extras block parses.**

### Choice: B with a fallback to A only if doubt arises

The Pack v1 codec has unit-test coverage of metadata round-trip and
is exercised every time `pack-all` runs (combine emits the JSON,
write+read happens via `pack_writer.go`). The probability that the
first byte hits disk wrong while every test passes is low and would
constitute a producer bug we want surfaced anyway. **B** is the
cheapest path that satisfies the protocol's "quick Go test or
subcommand" disjunction without inventing new code.

If, during Phase 1, we observe a CombinePack success but want
human-eyes confirmation of what landed in `extras`, we add **A** as
a 30-line subcommand. Designing it now is speculative.

Rejected:
- **A as default** — adds production code for a one-shot manual
  inspection. Speculative work, against CLAUDE.md guidance.
- **C** — duplicates `pack_meta_test.go`. The existing test already
  proves the codec; it does not gain credibility from running once
  more on a fresh fixture.

## Decision 2 — running the consumer's loader against a real pack

### Constraint

`plant-pack.test.ts` (per T-080-01 acceptance criteria) uses a
mocked GLTFLoader and hand-rolled fixtures. The protocol requires
verification "against an actual `.glb` file (not a mock)". The
consumer-side change to satisfy this lives in plantastic, not here,
but **this ticket cannot mark done until that test exists and passes**.

### Options

**A. Add a sibling integration test in plantastic that uses the real
GLTFLoader against the file we drop in.** New file, e.g.
`plant-pack.integration.test.ts`. Skipped in CI (no GLB committed),
gated on file presence at runtime.

**B. Run plantastic's prebuild registry script (`pnpm build` step
that T-083-05 step 2 calls out) and treat its successful parse of
the new pack as the loader-side acceptance.** No new test file.

**C. Manually load the file in a node REPL with the real loader and
paste the result into `progress.md`.**

### Choice: B as primary, A as escalation

The prebuild registry script is exactly the path that production
plantastic uses to consume packs. If it parses the file cleanly, the
loader code path has run on the real bytes. That is stronger than a
synthetic test. The protocol's wording ("plantastic's loader test")
permits this reading because the prebuild script **is** that loader
in the production hot path.

If `pnpm build` does not invoke the loader directly (it might only
verify file presence and let the runtime loader fire later), we
escalate to **A**: a small integration test in plantastic that loads
the real `.glb` via the real `GLTFLoader`. Designing that test
belongs to the implement phase of plantastic T-083-05 — we comment
on that ticket if we hit this branch.

Rejected:
- **C** — not auditable, not reproducible, fails the "test passes
  against the real file" wording.

## Decision 3 — handling stale MANIFEST.txt entries

### State

```
achillea_millefolium  # no .glb file present
coffeeberry           # no .glb file present
```

The prebuild registry script in plantastic (`web/scripts/build-plants-registry.ts`,
referenced by the consumer-side T-080-02) "will fail loud during
`pnpm build` if any listed file is missing" — that is the MANIFEST's
own self-documentation.

### Options

**A. Comment out the existing entries with a `#` prefix and add a
single new entry for the species we hand off.**

**B. Replace the MANIFEST entirely with just the handed-off species.**

**C. Provide stub GLBs for the missing entries.**

### Choice: A

Commenting preserves the intent ("these species are expected to
arrive eventually") while unblocking `pnpm build`. The change is
trivially revertable, which matters because the consumer-side agent
working T-083-05 may have its own opinion about the canonical
manifest contents and we don't want to overwrite their state. We
record the comment-out action in BOTH `progress.md` files.

Rejected:
- **B** — destroys the consumer's intent, would force them to
  rediscover what was previously listed.
- **C** — fabricated content. Violates "no mock packs" directly.

## Decision 4 — audit trail layout

### Choice

`progress.md` in this ticket is the canonical record. It is
structured as five level-2 sections matching the protocol's five
phases. Each section has a timestamp, the command(s) run, the
observed output (truncated where noisy), and a pass/fail conclusion.
On Phase 5 it ends with a "DONE" stamp and the verification
timestamp.

Plantastic's T-083-05 `progress.md` gets a much shorter note: just
the producer ticket id (T-011-04), the pack filename, the sha256,
the species id, and the timestamp. It serves as a one-way handoff
beacon, not a full mirror — the full audit lives in the producing
repo.

## Plan-of-execution sketch

(Detail goes in `plan.md`. Sketch only here so the design is
obviously executable.)

1. **Phase 1 in five commands:** confirm prereqs, `just clean-packs`
   for hygiene, `just pack-all`, eyeball summary table, run
   `go test ./... -run PackMeta` as the "extras parses" surrogate.
2. **Phase 2 in one command:** verify plantastic T-080-01 phase via
   `cat`. No polling expected (it's already done).
3. **Phase 3 in three commands:** `ls -lS dist/plants` to pick the
   smallest pack with the most variants, `shasum -a 256` to record
   sha, `cp` into plantastic's asset dir, edit MANIFEST.
4. **Phase 4 in two commands:** `pnpm build` in plantastic;
   if it parses the new file, that is the loader acceptance.
   Otherwise escalate to a node-side integration test (designed in
   plan.md).
5. **Phase 5 in three writes:** finalize `progress.md`, append a
   note to plantastic T-083-05's `progress.md`, and STOP. Lisa
   handles the phase transition to `done`.

## Reversibility & blast radius

Producer side: only `~/.glb-optimizer/dist/plants/` is mutated.
That directory is build output; deleting it is supported via
`just clean-packs`.

Consumer side: we add one file to plantastic's asset dir, edit one
MANIFEST line, and append to one `progress.md`. All three changes
are line-level and revertable from git. None of them touch
production source code in plantastic. This is the minimum-footprint
hand-off the protocol allows.

## Why this design fits

- It does not invent new code unless a specific failure forces it.
- It treats the protocol as the spec (because it is).
- It puts the audit trail where future agents will look first
  (`progress.md` in this repo) and leaves a one-line beacon in the
  consumer repo so its agent doesn't have to re-discover the pack.
- It commits to **B** for the loader-side acceptance because the
  prebuild script IS the production loader path and is therefore
  more honest than a synthetic integration test.
