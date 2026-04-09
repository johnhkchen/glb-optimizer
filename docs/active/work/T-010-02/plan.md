# T-010-02 Plan — Combine Implementation

Five ordered steps. Each step ends with a verifiable signal (build,
vet, or test). Commit after each step.

## Step 1 — Skeleton + container I/O

**Files touched:** create `combine.go`

**Code added:**
- Package, imports (`bytes`, `crypto/sha256`, `encoding/binary`,
  `encoding/json`, `fmt`, `sort`)
- The five GLB constants
- All schema struct types from `structure.md`
- `pad4(n)` helper
- `readGLB(raw []byte) (*gltfDoc, []byte, error)`
- `writeGLB(doc *gltfDoc, bin []byte) ([]byte, error)`
- An empty stub `CombinePack` that returns
  `nil, fmt.Errorf("not implemented")` so the package compiles

**Verification:**
- `go build ./...` clean
- `go vet ./...` clean

**Why first:** the entire ticket pivots on these two functions; if the
chunk-padding rules are wrong, every later test will fail in confusing
ways. Land them first and add a focused test in step 2 before any merge
logic exists.

**Commit:** `T-010-02: combine.go skeleton + GLB read/write helpers`

## Step 2 — Round-trip test for the I/O layer

**Files touched:** create `combine_test.go`

**Code added:**
- `makeMinimalGLB(meshNames, perMeshMinY) []byte` test helper. For each
  mesh name it adds: 3 indices + 3 positions to the BIN, an indices
  accessor, a position accessor (with `min`/`max` set so the volumetric
  slice ordering test works), one bufferView for indices, one
  bufferView for positions, one primitive, one mesh. No materials, no
  textures.
- `TestReadWriteRoundTrip(t *testing.T)`: builds a GLB with
  `["a","b"]`, parses it back via `readGLB`, asserts the JSON survived
  and the BIN length equals what was written.
- `TestRejectsNilSide(t *testing.T)`: confirms `CombinePack(nil, ...)`
  returns the right error message.
- `TestRejectsInvalidMeta(t *testing.T)`: bad meta → wrapped error.

**Verification:**
- `go test -run 'TestReadWriteRoundTrip|TestRejectsNilSide|TestRejectsInvalidMeta' ./...`
  passes
- The remaining `TestCombine_*` cases are added in step 4 once
  implementation exists; we don't pre-write tests we can't run

**Commit:** `T-010-02: round-trip test for GLB read/write helpers`

## Step 3 — `mergeContext` and `absorb` pass

**Files touched:** edit `combine.go`

**Code added:**
- `newMergeContext()` initializing `out` with asset/scene/buffer
- `(mc) absorb(in, inBin)` running the 7-step copy+remap pass:
  1. allocate `indexMap` slices sized to source slice lengths
  2. baseline = `mc.outBin.Len()`; copy `inBin` (4-aligned) into outBin
  3. for each source bufferView: clone, set `Buffer = 0`, shift
     `ByteOffset += baseline`, append to `mc.out.BufferViews`, record
     in `im.bufferView`
  4. for each source accessor: clone, remap `BufferView` via
     `im.bufferView`, append, record
  5. for each source image: call `mc.absorbImage(...)` (handles
     SHA256 dedup AND remaps any bufferView reference); record in
     `im.image`
  6. samplers: append verbatim, record indices
  7. textures: clone, remap `Source` via `im.image`, remap `Sampler`
     via sampler-index map, append, record
  8. materials: call `remapMaterialIndices(raw, im.texture)`, append,
     record
  9. meshes: clone primitives, remap each `attributes[k]` through
     `im.accessor`, remap `Indices` and `Material`, append, record
- `appendBufferView(srcBV, srcBin)` helper used by `absorbImage` for
  the dedup path that needs to add a *new* bufferView for newly-stored
  bytes
- `absorbImage` with SHA256 dedup
- `remapMaterialIndices` walking decoded JSON tree

**Verification:**
- `go build ./...` clean (no test yet — covered by step 5 tests)
- `go vet ./...` clean

**Commit:** `T-010-02: absorb pass (BIN merge + index remap + image dedup)`

## Step 4 — Routers, root node assembly, real `CombinePack`

**Files touched:** edit `combine.go`

**Code added:**
- `routeSideMeshes(mc, im, in)` — walks `in.Meshes`, for each mesh
  decides side vs top by name, appends a leaf node with the remapped
  mesh index, builds the group nodes, returns their indices
- `routeTiltedMeshes(mc, im, in)`
- `routeVolumetricMeshes(mc, im, in)` — uses `meshMinY` + stable sort
- `meshMinY(mesh, accessors)` reading POSITION accessor `min[1]`
- `attachExtras(mc, meta)` setting
  `mc.out.Scenes[0].Extras = map[string]any{"plantastic": meta.ToExtras()}`
- Replace the stub body of `CombinePack` with the full sequence
  (parse → validate → absorb → route → root → extras → write → cap)

**Verification:**
- `go build ./...` clean
- `go vet ./...` clean

**Commit:** `T-010-02: mesh routers + CombinePack assembly`

## Step 5 — End-to-end test coverage

**Files touched:** edit `combine_test.go`

**Tests added (each ~20–30 lines):**

1. `TestCombine_SideOnly_RoutesBillboardTop` —
   side has `["billboard_top","s0","s1"]`. Output should have:
   - one `view_side` group with two children named `variant_0`,
     `variant_1`
   - one `view_top` group with one child whose mesh maps to the
     original `billboard_top` mesh's data (verify by checking the
     remapped mesh's primitive accessor count matches)
   - no `view_tilted`, no `view_dome`

2. `TestCombine_SideOnly_NoBillboardTop` —
   side has `["s0","s1"]` only. Output has `view_side` but **no**
   `view_top` group at all.

3. `TestCombine_TiltedAdded_VariantNaming` —
   side `["billboard_top","a"]` + tilted `["t0","t1","t2"]`. Output's
   `view_tilted` group has children named `variant_0..variant_2`.

4. `TestCombine_VolumetricSliceOrder` —
   volumetric input has 4 meshes with `perMeshMinY = [0.5, 0.0, 0.75,
   0.25]`. Output's `view_dome` children should be named
   `slice_0..slice_3` with their underlying mesh order being the
   ascending Y permutation `[1, 3, 0, 2]`.

5. `TestCombine_EmbedsExtras` — assert
   `out.Scenes[0].Extras["plantastic"]["species"]` matches the meta
   we passed in.

6. `TestCombine_RoundTripParseable` — feed the output of `CombinePack`
   back through `readGLB` and assert it parses cleanly with the
   expected node names.

7. `TestCombine_ImageDedup` — synthesize two intermediates whose
   bufferView-backed images point at byte-identical PNG payloads,
   confirm the merged doc has exactly **one** image entry.

8. `TestCombine_SizeCapRejection` — synthesize a side intermediate
   with a 6 MiB BIN payload (one accessor + one bufferView pointing
   into it). Assert `CombinePack` returns the size-cap error and the
   number in the message is > 5 MiB.

9. `TestCombine_DoesNotMutateInputs` — capture
   `bytes.Clone(side)` before the call, run `CombinePack`, assert the
   original `side` slice still equals the snapshot.

**Verification:**
- `go test ./...` passes
- All 9 new test cases pass
- All 14 PackMeta tests still pass (sanity)

**Commit:** `T-010-02: end-to-end CombinePack test coverage`

## Testing Strategy Summary

| Test type | Where | What it covers |
|-----------|-------|----------------|
| Unit (round trip) | step 2 | I/O layer correctness in isolation |
| Unit (validation) | step 2 | Argument errors before any merge work |
| Integration | step 5 | Full CombinePack pipeline against synthetic fixtures |

There are no external dependencies and no real baked assets in any
test. The whole suite runs in `go test ./...` in well under a second.

## Known Limitations Captured for Review

These will move into `review.md` once implemented:
- Sparse accessors not handled (intermediates don't use them; would
  panic-or-corrupt if encountered)
- Animations / skins / cameras / lights silently dropped if present
- External-URI textures hashed by URI string, not bytes
- Material remap walks decoded JSON via `interface{}` — slower than a
  typed walker but ~5× shorter and material counts are tiny (≤16 per
  pack)
