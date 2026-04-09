# T-012-03 Progress — Standalone Pack Verifier

## Status: complete

All six plan steps executed in order with no deviations.

## Steps

### Step 1 — Land the dependency ✅

- Created `scripts/package.json` with `@gltf-transform/core@4.1.1`
  pinned (exact version, no caret).
- `cd scripts && npm install` succeeded: 2 packages added, 0
  vulnerabilities, no warnings.
- Smoke import (`import('@gltf-transform/core')`) succeeded.
- `package-lock.json` produced and intended to be committed for
  reproducibility.

### Step 2 — Write `verify-pack.mjs` ✅

- Header comment names `pack_meta.go::PackMeta.Validate` as
  canonical and lists the eight validation rules in order.
- `SCHEMA` constant + four pure validators
  (`validateMeta`, `validateScene`, `leafMeshes`, `validateMeshRefs`).
- `loadPack` wraps `NodeIO.read` in try/catch via `main`.
- Exit codes: 0 PASS, 1 FAIL/load-error, 2 usage error.
- Final length: 188 LOC including comments — under the ≤200 LOC
  target in the ticket.

### Step 3 — Build fixtures via `build-fixtures.mjs` ✅

- `build-fixtures.mjs` creates a one-triangle mesh, packages it as
  `pack_root → view_side(variant_0) [ + view_top(top_quad) ]`,
  stamps `scene.extras.plantastic` with the `VALID_META` constant.
- One run produced both fixtures (no manual editing required).
- Sizes: `valid-pack.glb` 1036 B, `broken-pack-no-top.glb` 968 B —
  well under the 10 KB ceiling we informally aimed for.
- Builder is committed alongside the binaries so a future schema
  bump has a clear regen path.

### Step 4 — Wire shell tests ✅

- `scripts/test-verify-pack.sh` runs both fixtures, asserts:
  - valid-pack exits 0
  - broken-pack exits 1
  - broken-pack output mentions `view_top`
- Run output: `PASS: verify-pack tests`.

### Step 5 — Justfile recipes ✅

- `verify-pack arg`: shells to a pure-bash species/path classifier
  (`grep -Eq '^[a-z][a-z0-9_]*$'`) before invoking the node script.
  No knowledge of glb-optimizer's workdir layout leaked into the
  node script.
- `verify-pack-test`: wraps the shell harness.
- `verify-pack-install`: documents the one-time `cd scripts &&
  npm install` setup.
- `just verify-pack-test` and `just verify-pack
  scripts/fixtures/valid-pack.glb` both exit 0.

### Step 6 — Gitignore ✅

- Appended `scripts/node_modules/` to `.gitignore` with a one-line
  rationale comment.

## Manual edge-case checks (post-plan)

Smoke-tested three off-plan failure modes:

| Input                          | Result                                            |
|--------------------------------|---------------------------------------------------|
| `echo garbage > t.glb`         | `FAIL: pack_load: Unexpected token 'g'…` exit 1   |
| missing path                   | `FAIL: pack_load: ENOENT…` exit 1                 |
| no argv                        | `usage: …` to stderr, exit 2                      |

All three handled cleanly without stack traces, confirming the
Plan's "manual smoke for malformed input" entry.

## Deviations from plan

None of substance. One small judgement call:

- The plan said `validateMeshRefs` would walk material → texture →
  image indices generically. The `@gltf-transform/core` API
  exposes per-slot getters (`getBaseColorTexture`, etc.) rather
  than a flat index map, so the implementation walks an explicit
  list of slot names. This is functionally equivalent for Pack v1
  (which only ever populates BaseColor) and slightly more robust
  to future slots.

## Files touched

- Created: `scripts/package.json`, `scripts/package-lock.json`,
  `scripts/verify-pack.mjs`, `scripts/build-fixtures.mjs`,
  `scripts/test-verify-pack.sh`, `scripts/fixtures/valid-pack.glb`,
  `scripts/fixtures/broken-pack-no-top.glb`.
- Modified: `justfile`, `.gitignore`.
