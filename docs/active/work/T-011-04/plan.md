# T-011-04 Plan — step-by-step execution

Five steps mapped 1:1 onto the protocol's five phases. Each step
lists the commands, the success criteria, the failure mode, and
the audit-trail entry it produces in `progress.md`.

## Step 1 — Phase 1: local prerequisites

### Commands

```bash
# (a) Confirm prereqs
grep -H "^phase:" docs/active/tickets/T-010-0*.md docs/active/tickets/T-011-0*.md

# (b) Hygiene
just clean-packs

# (c) Build all packs
just pack-all

# (d) List output, verify size cap
ls -lhS ~/.glb-optimizer/dist/plants/

# (e) Surrogate for "extras.plantastic parses cleanly"
go test ./... -run PackMeta
```

### Success criteria

- (a) All eight upstream tickets show `phase: done`.
- (b) `dist/plants/` is empty after `clean-packs`.
- (c) `pack-all` exits 0 and prints a summary table; the TOTAL row
  shows at least one pack written.
- (d) Every file under 5 MiB (`5242880` bytes; `humanBytes` will
  show `< 5.0 MB`).
- (e) `go test` exits 0; `PackMeta` round-trip suite is green.

### Failure handling

- Any prereq not at `done` → STOP. The ticket stays in `implement`.
  Document which prereq is short, identify its owner ticket, and
  exit. Do not patch the upstream ticket from this work directory.
- `pack-all` exit non-zero → capture stderr in `progress.md`,
  determine whether it is a CombinePack error, an oversize error
  (`PackOversizeError`), or a write error. Producer-side fix
  belongs in the originating ticket, not here.
- A pack over 5 MiB → impossible if `pack-all` succeeds (the cap
  is enforced inside CombinePack), but if it does occur, treat as
  a producer bug against T-010-05.
- `go test` failures → producer bug against the relevant T-010 or
  T-011 ticket.

### Audit entry

`progress.md` Phase 1 section, populated per the structure.md
template.

## Step 2 — Phase 2: wait for the consumer

### Commands

```bash
grep "^phase:" /Users/johnchen/swe/repos/plantastic/docs/active/tickets/T-080-01-plant-pack-types-and-loader.md
```

### Success criteria

- T-080-01 reports `phase: done`.

### Failure handling

- Anything other than `done` → enter polling mode. Re-run the
  command after each external nudge. Record every poll in the
  Phase 2 section of `progress.md` so the wait state is auditable.
- T-080-01 missing entirely → STOP and ask the user. The consumer
  story may have been renumbered.

### Audit entry

Single observation line. (No actual polling expected — the
research scan already showed `phase: done`.)

## Step 3 — Phase 3: hand off a real pack

### Commands

```bash
# (a) Pick the smallest pack file in dist/plants/
ls -lhS ~/.glb-optimizer/dist/plants/ | tail

# (b) Record sha256
shasum -a 256 ~/.glb-optimizer/dist/plants/<species>.glb

# (c) Optional sanity check on variants present (uses gltf parser
#     via existing combine_test helpers if needed)
go test ./... -run TestCombinePack_Variants  # only if helpful

# (d) Copy into plantastic
cp ~/.glb-optimizer/dist/plants/<species>.glb \
   /Users/johnchen/swe/repos/plantastic/web/static/potree-viewer/assets/plants/<species>.glb

# (e) Edit plantastic MANIFEST.txt:
#     - Comment out achillea_millefolium and coffeeberry (no files on disk)
#     - Add the new species id on its own line
```

### Variant selection rule

Per protocol: prefer a pack with all four variants (`view_side`,
`view_top`, `view_tilted`, `view_dome`). Fall back to any pack
with `view_side` + `view_top` + one of (tilted, dome). The
research scan suggests the only bakeable id (`0b58...`) has all
four because the underlying outputs include `_billboard.glb`
(side), an `_lod*.glb` set (top candidate), `_billboard_tilted.glb`
(tilted), and `_volumetric.glb` (dome candidate). Confirm via the
emitted GLB structure if there is any doubt.

Variant inspection: read the GLB scene tree via a small ad-hoc
test if the file's variant set is unclear. If still unclear,
escalate to the optional `pack-inspect` subcommand from
structure.md decision-point #1.

### Bake ID extraction

Read `bake_id` from `extras.plantastic` after combine. The
Pack v1 metadata is embedded in the GLB; the cleanest way to
read it is via a tiny Go invocation reusing
`pack_meta_capture.go` helpers. If we end up needing this more
than once, that is the trigger for the optional
`pack-inspect <species>` subcommand.

### Failure handling

- No pack has the minimum variant set → producer bug. Stop and
  document. Most likely cause: the bake suite for `0b58...` is
  missing one of the source files (would be visible in
  `~/.glb-optimizer/outputs/`).
- Copy fails → permission error. Investigate; do not retry blindly.
- MANIFEST edit conflict (file content has changed since the
  research scan) → re-read the file, reconcile, and proceed.

### Audit entry

Phase 3 section with: filename, size_bytes, bake_id, sha256, the
exact MANIFEST diff applied.

## Step 4 — Phase 4: verify consumer accepts the pack

### Commands

```bash
cd /Users/johnchen/swe/repos/plantastic
pnpm install      # only if node_modules is missing
pnpm build
```

### Success criteria

- `pnpm build` exits 0.
- The prebuild registry script (`web/scripts/build-plants-registry.ts`)
  reports the new species file as accepted, not skipped.
- The output bundle includes the species in whatever registry
  format the prebuild script produces (typically a generated
  `.ts` file under `web/src/generated/` or similar — locate via
  T-080-02).

### Loader test execution

The protocol calls for "plantastic's loader test (per T-080-01)".
The unit test `plant-pack.test.ts` uses a mock GLTFLoader. To
satisfy the "real file, not a mock" wording we have two options
ranked by reversibility:

**Option A (preferred):** treat `pnpm build`'s prebuild registry
script as the loader-side verification. It is the production hot
path and it parses the file. If it succeeds, Phase 4 passes.

**Option B (escalation):** if option A is insufficient (e.g., the
prebuild script only checks file presence and does not invoke
the loader), open plantastic T-083-05's plan and request a small
integration test there. Do **not** add the test to plantastic
from this ticket — that work belongs to T-083-05.

### Failure handling

- `pnpm build` fails on our pack → root-cause: producer bug
  (malformed pack) or consumer bug (loader regression). Capture
  the error verbatim in `progress.md`. If producer-side, the fix
  belongs upstream in T-010-02 / T-010-04 / T-011-02. If
  consumer-side, file an observation on plantastic T-083-05 and
  the relevant upstream consumer ticket; pause this ticket.
- `pnpm build` fails on a stale MANIFEST entry we forgot to
  comment out → fix the MANIFEST and retry. Not an upstream bug.
- `pnpm install` failures unrelated to pack content → not in
  scope; document and escalate to a human.

### Audit entry

Phase 4 section with the build command output (truncated to the
relevant prebuild registry lines), the loader-acceptance route
chosen (A or B), and a clear PASS / FAIL conclusion.

## Step 5 — Phase 5: mark done

### Actions

1. Append a final "Phase 5 — DONE" section to this repo's
   `progress.md` with the verification timestamp, test/build
   name, pack filename, and sha256.
2. Append a beacon section (per structure.md) to plantastic's
   `docs/active/work/T-083-05/progress.md`. If the file does
   not exist yet, create it containing only the beacon section.
3. STOP. **Do not** edit this repo's ticket frontmatter. Lisa
   detects `progress.md` plus `review.md` and advances the phase
   automatically.

### Success criteria

- Both `progress.md` files contain the beacon information.
- This repo's `progress.md` Phase 5 section ends with the literal
  string `DONE` so a future grep can find it.

### Failure handling

- Plantastic `progress.md` write fails → permission error.
  Investigate; do not partially commit.

## Testing strategy

The "test suite" for this ticket is the protocol itself. There are
no unit tests to add. The verification artifacts are:

1. `go test ./... -run PackMeta` (Phase 1, surrogate for the
   extras-parses check)
2. `just pack-all` (Phase 1, exercises the full pack pipeline
   end-to-end)
3. `pnpm build` in plantastic (Phase 4, exercises the consumer's
   real loader path against the real file)
4. Optional: a plantastic-side integration test if the prebuild
   script proves insufficient (Phase 4 escalation)

A passing run of all four (or three plus the escalation test) is
the acceptance criterion. The audit trail in `progress.md` is the
durable proof.

## Commit strategy

There are no source code commits required by this ticket. Each
artifact write (`research.md`, `design.md`, `structure.md`,
`plan.md`, `progress.md`, `review.md`) is a small, isolated
filesystem change. They can be committed individually or in a
single commit at the end — the user will direct.

The cross-repo MANIFEST edit and the GLB drop are working-tree
changes in plantastic, not in this repo, so they do not produce
commits here at all. Whether plantastic commits them is a
separate decision the consumer-side ticket makes.
