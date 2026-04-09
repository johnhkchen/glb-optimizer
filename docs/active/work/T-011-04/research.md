# T-011-04 Research — integration handshake gate

## What this ticket is

The producer-side half of a two-sided integration handshake between
glb-optimizer (this repo) and plantastic. Its purpose is **not** to
write code; it is to gate the moment glb-optimizer can declare its
E-002 work shipped. The gate fires only when a real `dist/plants/*.glb`
pack from this repo has been verified to load and render through the
consumer's loader test against the actual file (no mocks).

The matching consumer ticket is **plantastic T-083-05** (see
`/Users/johnchen/swe/repos/plantastic/docs/active/tickets/T-083-05-integration-handshake-gate.md`).
The two tickets are mutually aware and neither marks done until both
have observed the round trip.

## Upstream dependency status (Phase 1 prereq scan)

All eight prereq tickets in this repo are at `phase: done`:

| Ticket    | Phase | Notes |
|-----------|-------|-------|
| T-010-01  | done  | Pack v1 metadata schema landed (`pack_meta.go`) |
| T-010-02  | done  | `CombinePack` writes a single GLB with all variants |
| T-010-03  | done  | "Build Asset Pack" button + handler shipped |
| T-010-04  | done  | `just pack-all` recipe + `pack-all` subcommand shipped today |
| T-010-05  | done  | `PackOversizeError` (5 MiB cap) shipped today |
| T-011-01  | done  | `dist/plants/{species}.glb` write path |
| T-011-02  | done  | `BuildPackMetaFromBake` reads bake-time metadata |
| T-011-03  | done  | `bake_id` is set at bake time, stable across combine runs |

So **Phase 1 of the protocol is unblocked** the moment this ticket
hits `implement`. There is no upstream work left to wait on inside
this repo.

## Consumer-side state (Phase 2 prereq scan)

Inspected `/Users/johnchen/swe/repos/plantastic` on this host:

- `T-080-01-plant-pack-types-and-loader.md` — `phase: done`. The
  loader (`web/src/lib/three/plant-pack.ts`) and the unit test the
  protocol's Phase 4 references both exist.
- `T-080-02-registry-interface-and-local-impl.md` — present, used by
  the prebuild script Phase 4 step 2 invokes.
- `T-083-05-integration-handshake-gate.md` — exists; this is the peer
  ticket the protocol calls out by name.
- `web/static/potree-viewer/assets/plants/` already exists with a
  `MANIFEST.txt` listing two species ids (`achillea_millefolium`,
  `coffeeberry`). Both files are absent from disk per the MANIFEST's
  own comment ("arrive via USB drop"). The prebuild script will fail
  on these unless we either ship the listed files or rewrite the
  MANIFEST to point at whatever we hand off.

So **Phase 2 of the protocol resolves immediately** — the consumer is
already past `done` on its prerequisite. There is no polling state to
sit in.

## Bake inventory (Phase 3 candidate scan)

Inspected `~/.glb-optimizer/outputs/`. There is exactly one full bake
suite present:

```
0b5820c3aaf51ee5cff6373ef9565935.glb
0b5820c3aaf51ee5cff6373ef9565935_billboard.glb          <- side
0b5820c3aaf51ee5cff6373ef9565935_billboard_tilted.glb   <- tilted
0b5820c3aaf51ee5cff6373ef9565935_lod0..3.glb            <- top candidates
0b5820c3aaf51ee5cff6373ef9565935_volumetric.glb         <- dome candidate
```

Other ids in the directory are reference PNGs only (no bake), so they
are not packable. `discoverPackableIDs` in `pack_cmd.go:48` filters on
`*_billboard.glb`, so `pack-all` will produce exactly **one** pack
file from this state. That single pack is the candidate for hand-off.

`view_tilted` is present (`_billboard_tilted.glb`) and there is a
`_volumetric.glb` that the combine path can drop into `view_dome`, so
the resulting pack has a strong chance of meeting the protocol's
preferred "all four variants" condition. Worst case it satisfies the
fallback condition (`view_side` + `view_top` + tilted-or-dome).

`~/.glb-optimizer/dist/plants/` does **not** exist yet — `pack-all`
has never been run on this workdir. Phase 1 step 2 will create it.

## Code paths the protocol touches

The protocol invokes existing code only — this ticket should not
introduce production source. Relevant entry points:

- `justfile:34` — `pack-all: build` recipe runs `./glb-optimizer pack-all`
- `pack_cmd.go:resolveWorkdir` — guarantees `dist/plants/` exists
- `pack_cmd.go:discoverPackableIDs` — enumerates ids from `outputs/`
- `pack_runner.go:RunPack` — the per-id worker delegating to
  `CombinePack` then `WritePack`
- `combine.go:CombinePack` — returns `*PackOversizeError` when the
  result would exceed the 5 MiB cap (Phase 1 step 2 acceptance)
- `pack_meta.go` / `pack_meta_capture.go` — produce the
  `extras.plantastic` block the protocol's Phase 1 final check
  references

The protocol mentions a `glb-optimizer pack-inspect <species>` debug
subcommand "or" a quick Go test as the way to verify
`extras.plantastic` parses cleanly. No `pack-inspect` subcommand
currently exists in `pack_cmd.go`. The "quick Go test" path is
already covered by `pack_meta_test.go` round-trip tests, so we do
**not** need to invent a new subcommand for the protocol to be
runnable. The simpler hook for human-visible inspection is to add a
read-only inspector only if the existing tests do not satisfy us at
implement time.

## Cross-repo transfer mechanics

Per the ticket: the agreed transfer location is the dev laptop's
plantastic clone at `web/static/potree-viewer/assets/plants/`. The
agent is currently running on a host that has both repos cloned at
the documented paths, so this **is** the dev laptop case — no temp
directory fallback is needed. The plantastic copy already has the
target directory and a `MANIFEST.txt` ready to be edited.

Permissions: the user has explicitly granted cross-repo read on
plantastic. Writing to plantastic is also necessary (copying the GLB
in, editing MANIFEST, leaving a note in T-083-05's `progress.md`).
The protocol assumes write access — no escalation needed.

## What "done" means for THIS ticket

A successful Phase 4 run, captured in `progress.md`, with:

1. The pack filename, sha256, and `bake_id` recorded
2. A copy of that pack present in plantastic's asset dir
3. `MANIFEST.txt` in plantastic listing the species id of the
   handed-off pack
4. plantastic's loader test (per T-080-01) passing against the real
   file (not the mock fixture used in the unit test)
5. A note appended to plantastic T-083-05's `progress.md` referencing
   the pack filename + verification timestamp

Any failure at any step → ticket stays in `implement`, error captured
in `progress.md`, root cause attributed to producer or consumer side,
fix-forward in the responsible ticket.

## Risks and unknowns

- **Single-bake fragility.** Only one species is bakeable on this
  workdir. If its combine fails the size cap or its `extras` block
  is malformed, there is no fallback species — a producer-side fix
  is the only path forward, and we will catch it during Phase 1.
- **Species id derivation.** The bake id `0b5820...` is a content
  hash, not a slug. `pack_cmd.go`'s `deriveSpeciesFromName` (per
  memory IDs 377-379) converts hyphens to underscores and strips
  leading non-letters. A pure-hex id starts with digit `0`, which
  the existing rule strips. So the resulting species slug will be
  the hex string with the leading `0` shaved — we will need to
  observe the actual emitted filename and update plantastic's
  MANIFEST to match it exactly.
- **MANIFEST hygiene.** plantastic's MANIFEST currently lists two
  species (`achillea_millefolium`, `coffeeberry`) for which no GLB
  file exists on disk. If `pnpm build` runs the prebuild registry
  script Phase 4 step 2 invokes, those two missing files will fail
  the build before our hand-off pack is even checked. Phase 4 may
  require commenting out or removing those entries — note that this
  is a producer-side action against the consumer repo and should be
  recorded in plantastic T-083-05's `progress.md` as well so the
  consumer-side agent does not revert it.
- **Loader test path.** The protocol says "per T-080-01 acceptance
  criteria; the test path is documented there." The plantastic
  ticket names `plant-pack.test.ts` but does not pin its directory.
  We will need to locate it (`web/src/lib/three/plant-pack.test.ts`
  is the obvious guess) at implement time and confirm it can be
  pointed at a real file rather than the mocked GLTFLoader fixture.
  The unit test uses a mock — we may need to **add** a small
  integration test or temporarily swap the fixture, then revert.
  Whichever we choose, the change belongs in plantastic, not here.

## Out of scope for this ticket

- Automating the cross-repo handshake (future epic)
- Verifying more than one species (one is enough to prove the format)
- Checking that the pack renders correctly in the viewer (T-083-05's
  job, not ours)
- Recovery from a mutual deadlock (escalate to a human)
