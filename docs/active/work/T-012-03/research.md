# T-012-03 Research — Standalone Pack Verifier

## Goal

Build a producer-side, no-DOM node script that validates a Pack v1 GLB
end-to-end (metadata schema + scene graph shape) so the Phase 4
verification step in T-011-04's handshake can run on the producer host
without booting plantastic's vitest/three.js/jsdom stack.

## Codebase facts

### Pack v1 schema (Go source of truth)

`pack_meta.go` defines the contract that the verifier must mirror.

- `PackFormatVersion = 1` — embedded as `format_version` in extras.
- `PackMeta` fields, in declaration order:
  `format_version`, `bake_id`, `species`, `common_name`,
  `footprint{canopy_radius_m, height_m}`, `fade{low_start, low_end, high_start}`.
- `speciesRe = ^[a-z][a-z0-9_]*$` enforced on `species`.
- `Validate()` order: version → species (non-empty + regex) → common_name
  non-empty → bake_id non-empty → footprint dims finite & > 0 → each fade
  field in [0,1] → `low_start < low_end < high_start <= 1.0`.
- The metadata blob lives at `scene.extras.plantastic` of the **active**
  scene (`doc.scene` index, defaulting to 0).

### Pack scene graph (combine.go)

`combine.go` lines 586–671 build the canonical pack tree:

```
Scene[0] (extras.plantastic)
└── pack_root (node)
    ├── view_side  (group, N leaves named variant_0..variant_{N-1}, each with mesh)
    ├── view_top   (group, 1 leaf with mesh)
    ├── view_tilted (group, optional, N leaves with mesh)
    └── view_dome   (group, optional, N leaves named slice_0..slice_N with mesh)
```

The root scene has exactly one root node (`pack_root`); each variant
group is a child of `pack_root`; each variant leaf is a child of its
group with a `mesh` pointer.

`pack_inspect.go::summarizeVariants` already walks this tree in Go and
is the closest reference for the verifier's traversal logic. It treats
`view_top` as a **group with one mesh leaf** (not a bare leaf), even
though the ticket AC phrases it as "view_top mesh exists". The verifier
must match the actual emit, not the loose AC wording — i.e. accept
view_top as a group with ≥1 mesh leaf child.

### GLB binary layout (combine.go top)

`glbMagic = "glTF"`, `glbVersion = 2`, two chunks:
JSON (`chunkType 0x4E4F534A`) followed by an optional BIN
(`0x004E4942`). All chunk lengths are 4-byte aligned. The verifier does
not need to parse BIN for schema validation; only the JSON chunk is
required to walk extras + nodes + meshes + materials + textures.

### Existing CLI integration points

- `justfile` already has `pack`, `pack-all`, `clean-packs`, `check`
  recipes that wrap `./glb-optimizer` subcommands. A new `verify-pack`
  recipe slots in next to them.
- `pack_cmd.go::resolveWorkdir` and `pack_inspect.go::resolveInspectTarget`
  already implement the species-id-vs-path resolution pattern that the
  ticket asks the new recipe to mirror. The justfile recipe can either
  call out to the node script directly with a path, or shell out
  through bash to do the species→path resolution. Resolving in bash
  inside the recipe keeps the node script free of any glb-optimizer
  layout knowledge.
- `DistPlantsDir = "dist/plants"` is the canonical relative path under
  the workdir; absolute path on demo box is
  `~/.glb-optimizer/dist/plants/{species}.glb`.

### Why a node script lives in the producer repo

Per ticket Context: producer is the source of truth for Pack v1, and
forcing plantastic's vitest+three.js+jsdom stack into Phase 4 of the
handshake would couple repos. The script is small enough (≤200 LOC
target) that an operator can audit it on demo morning. The node side
is the producer's "did I make a pack the consumer can actually load?"
gate.

## External dependency

`@gltf-transform/core` is the only dep needed. It is a node-friendly
glTF/GLB parser with no DOM requirement and a clean Document API for
walking scenes/nodes/meshes/materials. No `@gltf-transform/extensions`
needed because Pack v1 uses no glTF extensions.

There is **no `package.json` in the repo root yet** — `ls` confirms
the repo is pure Go + python scripts. The verifier needs a fresh
`package.json` (either at repo root or in `scripts/`). Putting it at
`scripts/package.json` keeps node tooling out of the repo root and
avoids any implication that the rest of the repo is a node project.

## Constraints / open questions

- **Synthetic fixture authoring.** No node lib in the repo can produce
  a valid GLB on demand, so test fixtures must either be (a) committed
  binary blobs built by hand once via `@gltf-transform/core` itself, or
  (b) generated at test time by a small builder in the script's test
  file. (a) is simpler and matches the ticket AC verbatim
  ("`scripts/fixtures/valid-pack.glb`"), so the plan is: write a
  one-shot builder, run it, commit the .glb output, never run it
  again.
- **Test runner.** The repo's existing `just check` recipe only checks
  toolchain presence. There is no node test runner yet. The simplest
  AC-satisfying approach is a shell-based test that invokes
  `node verify-pack.mjs <fixture>` and asserts on exit code. A
  `scripts/test-verify-pack.sh` glued into a `just verify-pack-test`
  recipe is the lowest-toolchain path.
- **Schema duplication.** Deliberate per ticket Notes. The verifier
  hardcodes the field list and a `species` regex literal. A header
  comment must point at `pack_meta.go` as canonical and call out which
  rules are deliberately skipped (none expected for v1, but we should
  state it explicitly).
- **GLB malformed inputs.** `@gltf-transform/core` throws on bad GLBs;
  the verifier must catch and exit 1 with a one-line reason rather
  than printing a node stack trace. Need a try/catch around the read.

## Out of scope (per ticket)

- Validating texture sensibility / geometry plausibility.
- Looping over `dist/plants/`.
- CI integration.
- Code-generated schema.
