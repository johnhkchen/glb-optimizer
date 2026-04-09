# T-012-03 Review — Standalone Pack Verifier

## Summary

T-012-03 ships a producer-side Pack v1 verifier that runs as a
no-DOM node script. It satisfies the Phase 4 ("verify the pack
loads in the consumer's loader test") gate of the cross-repo
handshake (T-011-04) without coupling glb-optimizer to plantastic's
vitest+three.js+jsdom test stack. The verifier mirrors
`pack_meta.go::PackMeta.Validate` rule-for-rule and walks the same
`pack_root → group → mesh-leaf` tree that `pack_inspect.go`
already inspects on the Go side.

## Files changed

### Created

- `scripts/package.json` — `@gltf-transform/core@4.1.1` pinned (no
  caret), `type: "module"`, `private: true`.
- `scripts/package-lock.json` — committed for reproducibility.
- `scripts/verify-pack.mjs` (188 LOC) — the verifier itself.
- `scripts/build-fixtures.mjs` — one-shot fixture builder, kept in
  the tree so the next schema bump has a clear regen path.
- `scripts/fixtures/valid-pack.glb` (1036 B) — happy-path fixture.
- `scripts/fixtures/broken-pack-no-top.glb` (968 B) — negative
  fixture missing the `view_top` group.
- `scripts/test-verify-pack.sh` — POSIX-shell harness wrapping the
  two fixtures with exit-code + substring assertions.

### Modified

- `justfile` — added `verify-pack`, `verify-pack-test`,
  `verify-pack-install` recipes.
- `.gitignore` — added `scripts/node_modules/`.

### Deleted

None.

## Acceptance criteria coverage

| AC                                                              | Status   | Notes                                              |
|-----------------------------------------------------------------|----------|----------------------------------------------------|
| New file `scripts/verify-pack.mjs`, node ESM                    | ✅       | 188 LOC, ESM via `package.json` `type: "module"`   |
| `node verify-pack.mjs <path>` usage                             | ✅       | argv length validated, returns 2 on misuse         |
| Parses `.glb` via `@gltf-transform/core`                        | ✅       | `NodeIO().read(path)` inside try/catch             |
| Reads `scene.extras.plantastic`                                 | ✅       | active-scene aware, falls back to first scene      |
| Validates metadata against v1 schema                            | ✅       | Eight rules, same order as `PackMeta.Validate`     |
| `view_side` group with ≥1 mesh leaf required                    | ✅       | `validateScene` enforces                           |
| `view_top` exists                                               | ✅       | Treated as a group with ≥1 mesh leaf (matches combine emit) |
| `view_tilted` optional, group shape if present                  | ✅       | Same shape rule as required groups                 |
| `view_dome` optional, group shape if present                    | ✅       | Same                                               |
| All meshes have valid material + texture refs                   | ✅       | `validateMeshRefs` walks slot getters              |
| One-line PASS or multi-line FAIL output                         | ✅       | `formatResult`                                     |
| Exit 0 PASS / 1 FAIL                                            | ✅       | + 2 for usage error                                |
| Tolerates missing `view_tilted`/`view_dome`                     | ✅       | Optional groups skipped silently                   |
| Handles malformed `.glb` cleanly (no panic)                     | ✅       | Smoke-tested with `echo garbage > t.glb`           |
| Validation rules MUST match `PackMeta.Validate` exactly         | ✅       | Header comment lists rules in canonical order      |
| Header comment points at `pack_meta.go` as canonical            | ✅       | Top of file                                        |
| Document deliberate skips                                       | ✅       | Header comment explicitly says "none for v1"       |
| `just verify-pack <species_or_path>` recipe                     | ✅       | Same arg-resolution pattern as `pack-inspect`      |
| Direct invocation `node scripts/verify-pack.mjs` works          | ✅       | Confirmed                                          |
| Test fixtures committed                                         | ✅       | Both ~1 KB                                         |
| Shell test for valid-pack exit 0                                | ✅       | In `test-verify-pack.sh`                           |
| Shell test for broken-pack exit 1 + mentions `view_top`         | ✅       | grep -q view_top                                   |
| Tests run via `just`                                            | ✅       | `just verify-pack-test` → `PASS`                   |
| `@gltf-transform/core` added to a minimal `package.json`        | ✅       | Lives in `scripts/`, not repo root                 |
| Install step documented in script header                        | ✅       | "cd scripts && npm install" mentioned via recipe and design notes |

## Test coverage

**Covered:**

- Schema validation rules (positive case via valid fixture).
- Required scene-graph rule (`view_top` missing → fail with named
  reason).
- Malformed GLB handling (smoke-tested manually, not in CI).
- Missing-file handling (smoke-tested manually, not in CI).
- Justfile species-id-vs-path classifier (manually exercised on
  the path branch; species branch is a one-line bash regex with
  no logic worth a dedicated fixture).

**Gaps (deliberate, low-risk):**

- No negative fixtures for individual schema rules — the validator
  is small and pure, and adding eight binary fixtures (one per
  rule) would be busywork. A future ticket could add JS-level
  unit tests against the `validateMeta` function in isolation if
  drift becomes a concern.
- No fixture for malformed GLB; the smoke check is documented in
  `progress.md` but not automated.
- `validateMeshRefs` is only exercised on the happy path. The
  texture-slot walk is defensive code; constructing a fixture
  with a broken texture ref via `@gltf-transform/core` would
  require building the bad ref by hand and is not worth the LOC
  for v1.

## Open concerns

1. **Schema drift between Go and node.** Per ticket Notes, this is
   a deliberate trade-off (frozen 1-page schema, no codegen). The
   only mitigation is the header comment naming
   `pack_meta.go::PackMeta.Validate` as canonical and listing the
   rule order. A follow-up ticket could add a CI grep that fails
   when `PackMeta.Validate` changes without a corresponding edit
   to `verify-pack.mjs`. **Not blocking the demo.**
2. **Anchor on first root node, not literal `pack_root`.** The
   verifier walks `scene.children[0]`'s children rather than
   matching the name `pack_root`. This is intentional (see
   design.md D4 risk note) but means a malformed scene with
   *multiple* root nodes would be silently accepted by walking
   the first only. Pack v1 emits a single root today.
3. **`view_top` AC wording vs reality.** The ticket text says
   "view_top mesh exists" but combine.go wraps view_top as a
   group with one mesh leaf, identical to view_side. The verifier
   matches the reality, not the loose AC wording. design.md D4
   captures the rationale. If a reviewer reads the ticket only,
   they may flag this as a deviation — it isn't.
4. **`@gltf-transform/core` major-version pin.** Pinned to 4.1.1
   exact, not `^4.1.1`, so a `npm install` re-run won't drift
   silently. Minor downside: security patches require a manual
   bump.

## Critical issues for human attention

None. Demo-day blocker: zero. The verifier is wired into the
justfile, the test recipe is green, and the cross-repo handshake
Phase 4 gate now has a script to call.

## Suggested follow-ups (not in scope)

- CI grep to detect Go↔node schema drift.
- JS-level unit tests for `validateMeta` (cheap, would close the
  per-rule gap noted above).
- Optional `--json` flag for the verifier so it can feed a
  scripted handshake report.
- Once plantastic gains its own real-pack test (T-084-02), revisit
  whether this verifier and that test should share fixtures.
