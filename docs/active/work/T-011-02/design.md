# T-011-02 — Design

## Decision summary

- **AABB extraction**: Local-space, pure-Go GLB JSON-chunk parser.
  Walks `meshes[].primitives[].attributes.POSITION` accessors and
  takes component-wise min/max of each accessor's `min`/`max` arrays.
  No node transforms. (Option A below.)
- **Function shape**: `BuildPackMetaFromBake(id, originalsDir, settingsDir, outputsDir string, store *FileStore) (PackMeta, error)`.
  Diverges from the ticket's bare `(id string)` to match this repo's
  path-as-explicit-arg convention.
- **File**: new `pack_meta_capture.go`. `pack.go` does not exist yet
  and capture is logically separable from the schema/validation in
  `pack_meta.go`.
- **Override**: `outputsDir/{id}_meta.json`, opaque JSON, only
  `species` and `common_name` keys honored in v1.

## Option survey

### Footprint extraction

**Option A — local-space accessor min/max (CHOSEN).** Reuse the
JSON-chunk reader from `scene.go:CountTrianglesGLB`. For each primitive,
look up the POSITION accessor; the glTF spec mandates `min`/`max` arrays
on POSITION accessors so the data is already there with no buffer
decoding. Component-wise reduce across all primitives.

- Pros: tiny code, ~50 lines, mirrors prior art, no new deps, no buffer
  decoding, no node-graph traversal. Fast (single mmap-able file read).
- Cons: ignores node TRS. For an asset whose root node is translated
  upward, height_m would still be the local height — usually still
  correct because TRELLIS exports come centered. The 5% AC tolerance
  absorbs noise.
- Mitigation: when wrong, the override file is the escape hatch (and
  v2 of the schema can add a footprint override block).

**Option B — full scene-graph walk with node TRS.** Decode every
primitive's POSITION buffer, multiply by accumulated node matrices,
take a true world AABB.

- Pros: correct in all cases.
- Cons: ~300+ lines, requires a buffer/bufferView decoder and a
  matrix library, no precedent in this codebase. Disproportionate for
  a demo unblocker.

**Option C — shell out to a Python script that uses `pygltflib`.**
- Pros: leverages a tested library.
- Cons: adds a Python dep on the bake path, slows capture, fights
  the "everything in Go where possible" tilt the rest of the bake
  pipeline already follows. Rejected.

**Option D — re-bake-time caching.** Have the bake path itself drop a
sidecar with footprint when it produces an output, and capture just
reads the sidecar.

- Pros: correctness wherever bake already loads the mesh fully.
- Cons: invasive — touches blender.go and processor.go which T-011-02
  is explicitly not in scope of. Holds up the demo. Defer to a v2
  ticket if the local-space approach proves wrong on real assets.

### Species id derivation

**Option A — filename + override file (CHOSEN).** Pipeline:

1. If `outputsDir/{id}_meta.json` exists and has a `species` key
   passing the `^[a-z][a-z0-9_]*$` regex, use it. Same for
   `common_name`.
2. Else: derive from `FileRecord.Filename`. Strip extension. Lowercase.
   Replace `[^a-z0-9_]` → `_`. Strip *all* leading non-`[a-z]`
   characters (digits, underscores from leading-digit collapse). Strip
   trailing underscores. Collapse runs of `_`. Reject if empty or
   regex-fail → error.
3. If `Filename` was lost on restart and equals `{id}.glb` literally,
   derive from the id; same algorithm.

- Pros: deterministic, no new state, demo-friendly because the override
  fixes the timestamped-id case the memory log already flagged
  (`sample_2026-04-08T010040.068`).

**Option B — settings-stored species field.** Add `Species` to
`AssetSettings`, expose in tuning UI.

- Pros: editable in-app.
- Cons: schema migration on `AssetSettings`, JS UI work, out of
  T-011-02 scope (and explicitly Out-of-Scope per the ticket).

### common_name derivation

Title-case the species id, `_` → space. Manual loop, no
`golang.org/x/text` dep introduction. `rose_julia_child` →
`Rose Julia Child`. Override file wins when present.

### bake_id

`time.Now().UTC().Format(time.RFC3339)`. T-011-03 will embed this and
its stability story is downstream of capture; freezing RFC3339 here
means downstream can rely on a sortable string.

### Function signature alternatives

1. `BuildPackMetaFromBake(id string)` — ticket wording. Requires
   package-level globals for the dirs and store. Rejected: this repo
   threads dirs as explicit args everywhere.
2. `BuildPackMetaFromBake(id, originalsDir, settingsDir, outputsDir string, store *FileStore) (PackMeta, error)` (CHOSEN).
3. `type PackCapture struct{...}; (*PackCapture).Build(id) (PackMeta, error)`.
   Cleaner if there were many methods, overkill for one function.

## Error model

Capture fails loudly. Every failure is a `fmt.Errorf` wrapping the
relevant filename / id with a `pack_meta_capture:` prefix:

- Source mesh missing → `pack_meta_capture: source mesh not found for id %q`
- Source mesh unparseable (bad magic, JSON chunk truncated, no POSITION
  accessors with min/max) → wrapped parse error
- Footprint computes to ≤ 0 (degenerate mesh) → explicit error, do not
  silently fall back to a default
- Override JSON unparseable → wrap; do not silently ignore
- Derived species fails the regex → return the offending derived string
  with a hint to write `outputsDir/{id}_meta.json`
- Final `Validate()` failure → propagate

The `Notes` section of the ticket frames this: "fail loudly, not
silently use defaults". The combine step calling capture must surface
the error to the operator.

## Test strategy

Two layers:

1. **Unit test (`pack_meta_capture_test.go`)** — synthesized inputs:
   - A tiny in-memory GLB (built from a hand-rolled JSON chunk and a
     null binary chunk) with one mesh, one primitive, POSITION min
     `[-0.5, 0, -0.4]` and max `[0.5, 1.8, 0.4]`. Expected:
     `height_m == 1.8`, `canopy_radius_m == 0.5`.
   - Override file present → species/common_name come from the
     override and bypass the derivation.
   - Filename `Rose_Julia-Child.glb` → species
     `rose_julia_child`, common_name `Rose Julia Child`.
   - Filename `123_planter.glb` (leading-digit case) → leading non-letters
     stripped → `planter`.
   - Filename whose derivation fails (e.g. `2026-04-08.glb` →
     pure digits + dashes → empty after strips) → error mentioning
     "write outputs/{id}_meta.json".
   - Settings file present with non-default fades → those fades land
     in the meta.
   - No settings file → defaults flow through.
   - Source mesh missing → error.
   - Generated PackMeta passes `Validate()`.

2. **Integration test** — a real fixture from `assets/`:
   `assets/rose_julia_child.glb`. Stage it under a temp `originals/`
   as `{id}.glb`, set `FileRecord.Filename = "rose_julia_child.glb"`,
   call `BuildPackMetaFromBake`, assert footprint values are within
   5% of expected ground-truth measured once (record the expected
   values inline as constants with a comment showing how they were
   measured). The fixture file already exists in the repo.

## Risks

- The local-space AABB simplification could mis-measure pre-translated
  assets. Mitigation: 5% AC tolerance is wide enough for the demo
  fixtures; v2 ticket can add full TRS walking if real assets break.
- `FileRecord.Filename` loss on restart degrades species derivation
  to id-based. Acceptable because the override file is the documented
  fix and the operator-facing error message points there.
- glTF accessors are not strictly required to provide min/max on
  *non*-POSITION accessors, but they ARE required on POSITION
  (glTF 2.0 spec §3.6.2.4). We rely on that requirement; bail with a
  clear error if missing.
