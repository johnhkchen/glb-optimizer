# T-012-03 Plan — Standalone Pack Verifier

## Steps

### Step 1 — Land the dependency

- Create `scripts/package.json` with `@gltf-transform/core` pinned.
- Run `cd scripts && npm install`.
- Verify `node -e "import('@gltf-transform/core').then(m => console.log(Object.keys(m).slice(0,5)))"` works.
- Commit `package.json` + `package-lock.json`. Do not commit
  `scripts/node_modules/`.

**Verification:** `node -e ...` exits 0 and prints exported names.

### Step 2 — Write `verify-pack.mjs`

- Header comment with canonical-source pointer.
- `SCHEMA` constant.
- `validateMeta(extras)` first — pure function, no I/O. Mirror the
  Go validation order exactly: format_version → species non-empty
  → species regex → common_name non-empty → bake_id non-empty →
  footprint dims (finite + > 0) → fade fields in [0,1] → fade
  ordering low_start < low_end < high_start <= 1.
- `validateScene(document)` — find active scene, walk root nodes,
  collect groups by name, check view_side + view_top required.
- `validateMeshRefs(document)` — for each mesh under any pack
  group, check primitive material indices and material→texture→
  image refs are in range.
- `loadPack(path)` using `NodeIO`, with try/catch wrapping.
- `main` glue: parse argv, call loaders/validators, format result.
- Exit 0/1/2 wired in.

**Verification:** Manually invoke against a real pack from
`~/.glb-optimizer/dist/plants/` if any exist. Ticket noted only one
species was packable but exceeded the cap (T-011-04). If no real
pack exists yet, defer to Step 4 where the synthetic fixtures
prove the script works.

### Step 3 — Build fixtures via `build-fixtures.mjs`

- Use `Document` API to assemble:
  - one buffer
  - one material (no texture, to keep validateMeshRefs path simple
    for the happy case)
  - one mesh with a 3-vertex primitive
  - nodes: `pack_root` → `view_side`(group) → `variant_0`(leaf
    with mesh ref) and `view_top`(group) → `top_quad`(leaf with
    mesh ref)
  - root scene with `pack_root` as its only root node and
    `extras.plantastic` populated with a valid `PackMeta` shape.
- Write to `scripts/fixtures/valid-pack.glb`.
- For the broken variant: build the same Document, then strip the
  `view_top` node from `pack_root.children` before write. Save as
  `scripts/fixtures/broken-pack-no-top.glb`.
- Run `node scripts/build-fixtures.mjs` once and commit the
  resulting `.glb` files. The builder script itself is also
  committed so future schema bumps can re-run it.

**Verification:** `ls -la scripts/fixtures/*.glb` shows two files,
both nonzero, both <10 KB.

### Step 4 — Wire shell tests

- `scripts/test-verify-pack.sh`:

  ```sh
  #!/usr/bin/env bash
  set -u
  cd "$(dirname "$0")/.."
  fail=0
  if ! node scripts/verify-pack.mjs scripts/fixtures/valid-pack.glb >/dev/null; then
    echo "FAIL: valid-pack rejected"
    fail=1
  fi
  out=$(node scripts/verify-pack.mjs scripts/fixtures/broken-pack-no-top.glb 2>&1)
  rc=$?
  if [ "$rc" -eq 0 ]; then
    echo "FAIL: broken-pack accepted (exit 0)"
    fail=1
  fi
  if ! echo "$out" | grep -q view_top; then
    echo "FAIL: broken-pack output missing view_top mention"
    echo "$out"
    fail=1
  fi
  if [ "$fail" -eq 0 ]; then
    echo "PASS: verify-pack tests"
  fi
  exit "$fail"
  ```

**Verification:** `bash scripts/test-verify-pack.sh` prints
`PASS: verify-pack tests` and exits 0.

### Step 5 — Justfile recipes

- `verify-pack <arg>`: pure shell species-id-or-path resolution.
- `verify-pack-test`: shells out to the test script.
- `verify-pack-install`: documents one-time `npm install`.

**Verification:** `just verify-pack-test` and `just verify-pack
scripts/fixtures/valid-pack.glb` both succeed.

### Step 6 — Gitignore

- Append `scripts/node_modules/` to `.gitignore`.

**Verification:** `git status` does not show `scripts/node_modules/`.

## Testing strategy

| What                     | How                                            |
|--------------------------|------------------------------------------------|
| Schema validation rules  | Exercised end-to-end via fixtures              |
| Scene-graph rules        | `view_top`-stripped fixture is the negative    |
| Malformed GLB handling   | Smoke-tested manually with `echo bad > t.glb`  |
| Mesh ref validation      | Covered by valid fixture (positive only)       |
| Justfile recipe          | `just verify-pack-test`                        |

Manual smoke for malformed input is acceptable for v1; the script's
try/catch ensures we never produce a stack trace, and adding a
synthetic-malformed fixture is low-value busywork.

## Rollback

All changes are additive — no Go code touched, no existing tests
modified. Reverting the commit removes the script wholesale.
