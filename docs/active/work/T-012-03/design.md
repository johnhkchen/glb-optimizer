# T-012-03 Design — Standalone Pack Verifier

## Decisions

### D1 — Parser: `@gltf-transform/core` (chosen)

**Rejected alternatives:**

- **Hand-rolled GLB+JSON parser in node.** Removes the dep but
  duplicates a chunk of `combine.go`'s GLB parser in JS. Would
  trade ~50 LOC of code for one transitive dep. The dep is
  battle-tested, supports Document/Scene/Node/Mesh walking out of
  the box, and produces good error messages on malformed input.
- **`gltf-loader-ts` / three.js GLTFLoader.** Both pull in DOM
  shims (`jsdom`, blob, image) which is exactly what we are trying
  to avoid. Loses the "no DOM dependency" property the ticket
  emphasises.

**Why core wins:** zero DOM, single tiny dep, MIT-licensed,
maintained, and the Document API maps 1:1 to the walks the ticket
requires. The price is one `package.json` and a `node_modules/`
under `scripts/` — acceptable per the Out of Scope of putting
node tooling at the repo root.

### D2 — Location: `scripts/verify-pack.mjs` + `scripts/package.json`

**Rejected:**

- Repo-root `package.json`. Implies repo is a node project. Repo
  is Go + python today; pollution would be confusing.
- `tools/` subtree. New top-level dir for one script is more
  ceremony than `scripts/` (which already exists for python).

`scripts/` already houses ad-hoc python tooling (`bake_textures.py`
etc.). Adding a node script next to them is consistent.

### D3 — Schema replication strategy: hand-mirrored, header comment points to canonical

**Rejected:**

- Code generation from a Go-side JSON schema dump. Adds a build
  step and a fourth artefact (the schema file) to keep in sync.
  Pack v1 is frozen at one page; the ROI is negative.
- Importing the rules from a shared JSON file committed alongside
  Go. Same problem in reverse: someone has to commit and sync it.

The validator hardcodes:

```js
const SCHEMA = {
  formatVersion: 1,
  speciesRe: /^[a-z][a-z0-9_]*$/,
  requiredStrings: ['bake_id', 'species', 'common_name'],
  footprintFields: ['canopy_radius_m', 'height_m'],
  fadeFields: ['low_start', 'low_end', 'high_start'],
};
```

Header comment names `pack_meta.go::PackMeta.Validate` as the
canonical source and lists the rule order so a future drift can be
caught by code review.

### D4 — Scene-graph walk: match `pack_inspect.go`, treat view_top as a group

The ticket AC text says "view_top mesh exists" but the actual
combine output (combine.go:613–617) wraps view_top as a group with
one mesh leaf, identical structurally to view_side. The verifier
must match what packs actually emit, not the AC's loose phrasing.
Treat all four variants uniformly: each is a child of `pack_root`,
each is a group with ≥1 mesh-bearing leaf child.

Required: `view_side`, `view_top`. Optional: `view_tilted`,
`view_dome`. Unknown groups are tolerated (forward compat) but
counted in a debug log line so a typo in a future combine pass is
visible. The deliverable spec in the AC says "All meshes
referenced have valid material + texture references" — we
interpret that as: every mesh primitive must reference an in-range
material index, and any material that references a texture must
reference an in-range texture index that resolves to an in-range
image. We do **not** validate the texture's pixel data.

### D5 — Output format: human-first, exit-code-as-API

Single-line PASS on success, multi-line FAIL with one reason per
line on failure. No `--json`, `--quiet`, etc. — the ticket says
"exit 0 on PASS, 1 on FAIL" and the consumer is an operator
running it as a Phase 4 gate. `pack-inspect` already covers the
JSON-emitting case. Keeping the verifier opinionated keeps it
short.

### D6 — Test fixtures: committed binary blobs, builder script for one-shot regen

Two fixtures live in `scripts/fixtures/`:

- `valid-pack.glb` — one root scene, pack_root → view_side(1
  variant) + view_top(1 leaf), valid extras.plantastic.
- `broken-pack-no-top.glb` — same, but with view_top stripped.

The builder is a short script `scripts/build-fixtures.mjs` that
uses `@gltf-transform/core` to assemble both files in one run.
Committed once; re-run only if Pack v1 schema changes. The builder
itself is not part of the production code path and is not
exercised by tests — it's tooling.

### D7 — Tests: shell harness invoked by justfile

A POSIX shell script `scripts/test-verify-pack.sh` runs the
verifier against both fixtures and asserts exit codes + output
substrings (the FAIL case must mention `view_top`). Exit 0 on all
green, 1 on first failure. Wired into `just verify-pack-test`.

The repo has no node test runner today; importing one for two
shell-level cases is overkill. If a future ticket adds proper JS
tests we can migrate then.

## Risks

- **Dep drift.** `@gltf-transform/core` major version bumps could
  break the script. Mitigation: pin the exact version in
  `package.json`, not a caret range.
- **Schema drift.** Pack v1 changes in `pack_meta.go` won't break
  the verifier — they'll cause silent under-validation. Mitigation:
  the header comment is the only line of defence; a future
  follow-up could add a CI grep to detect mismatches.
- **`pack_root` rename.** combine.go literally names the root
  node `pack_root`; if a future combine pass renames it, the
  verifier's anchor breaks. Mitigation: don't anchor on
  `pack_root` by name. Walk from `scene.nodes[0]`'s children
  instead and look up groups by name.
