# T-012-03 Structure — Standalone Pack Verifier

## File-level changes

### Created

- `scripts/package.json` (new)
  - `name: "glb-optimizer-scripts"`, `private: true`, `type: "module"`.
  - One dep: `"@gltf-transform/core"` pinned at the latest 4.x patch.
  - One devDep: none. No test runner.
  - One script: `"verify-pack": "node verify-pack.mjs"`.
- `scripts/verify-pack.mjs` (new, ≤200 LOC target)
  - Header comment naming `pack_meta.go::PackMeta.Validate` as the
    canonical schema source and listing the validation rule order.
  - `SCHEMA` constant block (D3).
  - `main(argv)` async — parses argv, reads file, returns `Result`.
  - `loadPack(path)` — `await new NodeIO().read(path)`, returns
    `{document}`. Wraps errors with `pack_load:` prefix.
  - `validateMeta(extras)` — runs the eight rules in the same
    order as `PackMeta.Validate`. Returns array of `string` errors
    (empty = pass).
  - `validateScene(document)` — finds the active scene, walks its
    root nodes, builds a `{view_side, view_top, view_tilted,
    view_dome}` map by group name, asserts required groups exist
    with ≥1 mesh leaf each. Returns array of error strings.
  - `validateMeshRefs(document)` — for every mesh referenced under
    pack_root, walks primitives and checks material/texture/image
    indices are in range.
  - `formatResult(result)` — single-line PASS or multi-line FAIL.
  - Bottom: `await main(process.argv.slice(2))` with try/catch
    that prints `FAIL: <msg>` and exits 1 on any uncaught throw.
- `scripts/build-fixtures.mjs` (new, tooling — not run by tests)
  - Builds `scripts/fixtures/valid-pack.glb` and
    `scripts/fixtures/broken-pack-no-top.glb` using
    `@gltf-transform/core` Document API.
  - Header comment: "RUN ONCE — output committed. Re-run only on
    Pack v1 schema bump."
- `scripts/fixtures/valid-pack.glb` (new, binary, ~2 KB)
- `scripts/fixtures/broken-pack-no-top.glb` (new, binary, ~2 KB)
- `scripts/test-verify-pack.sh` (new, ~30 LOC)
  - Exits non-zero on first assertion failure.
  - Asserts: valid fixture → exit 0, FAIL fixture → exit 1 +
    output contains `view_top`.

### Modified

- `justfile`
  - New recipe `verify-pack <species_or_path>`. Resolves a species
    id to `~/.glb-optimizer/dist/plants/{id}.glb` in pure shell
    using a small `case` on `^[a-z][a-z0-9_]*$`, then invokes
    `node scripts/verify-pack.mjs <path>`.
  - New recipe `verify-pack-test`: `bash scripts/test-verify-pack.sh`.
  - New recipe `verify-pack-install` (one-time setup):
    `cd scripts && npm install`. Documented in the script header.

- `.gitignore`
  - Add `scripts/node_modules/` (and `scripts/package-lock.json`
    only if we decide not to commit it; default is to commit lock
    for reproducibility — leave it out of gitignore).

### Deleted

None.

## Public interfaces

`scripts/verify-pack.mjs` is a CLI; its public surface is:

```
node verify-pack.mjs <path-to-pack.glb>
```

Exit codes:

- `0` — pack is a valid Pack v1.
- `1` — pack is missing, malformed, or fails validation.
- `2` — usage error (no arg, too many args).

stdout: a single PASS line or multi-line FAIL with one reason per
line. stderr is reserved for unexpected exceptions.

## Module boundaries

The script is intentionally one file. Splitting validation into a
sibling module would create an import-graph that the operator has
to chase during a demo-morning audit. The four validators
(`validateMeta`, `validateScene`, `validateMeshRefs`,
`formatResult`) are private functions in the same file with no
shared mutable state.

## Ordering of changes

1. `scripts/package.json` + `npm install` to land the dep.
2. `verify-pack.mjs` skeleton + `validateMeta`.
3. `build-fixtures.mjs` + run it + commit binaries.
4. `validateScene` + `validateMeshRefs`.
5. `test-verify-pack.sh` + manual run + green.
6. `justfile` recipes.
7. `.gitignore` update.

This ordering lets each step be exercised against the next: the
fixtures are useless without `validateMeta`, but `validateMeta` is
testable in isolation against a hand-built extras object before
the fixtures exist.
