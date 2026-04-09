# T-010-02 Review — Combine Implementation

## Summary

Implemented `CombinePack(side, tilted, volumetric, meta) ([]byte, error)`,
a zero-dependency Go function that merges up to three intermediate GLBs
into a Pack v1 asset pack matching the contract frozen in T-010-01 /
E-002. Handles mesh routing into the four named groups, BIN chunk
concatenation with full index remapping, SHA256-based image dedup,
metadata embedding under `extras.plantastic`, and the 5 MiB size cap.

## Files

| File | Status | Lines | Purpose |
|------|--------|-------|---------|
| `combine.go` | created | 568 | Schema types, GLB read/write, mergeContext, absorb, routers, CombinePack |
| `combine_test.go` | created | 437 | makeMinimalGLB helper + 12 unit/integration tests |
| `docs/active/work/T-010-02/research.md` | created | — | RDSPI research artifact |
| `docs/active/work/T-010-02/design.md` | created | — | RDSPI design artifact |
| `docs/active/work/T-010-02/structure.md` | created | — | RDSPI structure artifact |
| `docs/active/work/T-010-02/plan.md` | created | — | RDSPI plan artifact |
| `docs/active/work/T-010-02/progress.md` | created | — | RDSPI progress artifact |
| `docs/active/work/T-010-02/review.md` | created | — | This file |

No files modified outside the new ticket work directory. No deletions.

## Public API Added

```go
func CombinePack(side []byte, tilted []byte, volumetric []byte, meta PackMeta) ([]byte, error)
```

This is the only new exported symbol. T-010-03 (HTTP endpoint) is the
intended first consumer.

## Acceptance Criteria — Verification

| Criterion | Status | Evidence |
|-----------|--------|----------|
| Signature `CombinePack(side, tilted, volumetric, meta)` | ✅ | combine.go:498 |
| `side` required, others may be nil | ✅ | TestCombine_RejectsNilSide; tilted/vol guarded by `if != nil` |
| `billboard_top` → `view_top`; others → `view_side` (variant_N) | ✅ | TestCombine_SideOnly_RoutesBillboardTop |
| Tilted meshes → `view_tilted` (variant_N) | ✅ | TestCombine_TiltedAdded_VariantNaming |
| Volumetric meshes → `view_dome` (slice_N) sorted by min Y bottom→top | ✅ | TestCombine_VolumetricSliceOrder asserts both names and underlying min-Y order |
| Buffer merge with rebased bufferView offsets | ✅ | absorb step 1 + TestCombine_RoundTripParseable confirms accessors still resolve |
| Texture dedup by SHA256 image hash | ✅ | TestCombine_ImageDedup |
| Each mesh keeps its own material instance | ✅ | absorb step 6 walks materials in source order, never deduping; only texture indices are remapped |
| `extras.plantastic` populated from meta | ✅ | TestCombine_EmbedsExtras |
| `meta.Validate()` called before write | ✅ | CombinePack second statement; TestCombine_RejectsInvalidMeta |
| Error if final size > 5 MiB | ✅ | TestCombine_SizeCapRejection |
| Round-trip test re-parses cleanly | ✅ | TestCombine_RoundTripParseable |
| Inputs not mutated | ✅ | TestCombine_DoesNotMutateInputs (bytes.Clone snapshot diff) |

All ticket acceptance criteria met.

## Test Coverage

12 tests, all passing under `go test ./...`:

| Test | Surface area |
|------|--------------|
| TestReadWriteRoundTrip | GLB binary read/write helpers in isolation |
| TestCombine_RejectsNilSide | Argument validation |
| TestCombine_RejectsInvalidMeta | Meta validation wiring |
| TestCombine_SideOnly_RoutesBillboardTop | Side mesh routing + view_top special case |
| TestCombine_SideOnly_NoBillboardTop | view_top correctly omitted when absent |
| TestCombine_TiltedAdded_VariantNaming | Tilted router + variant naming |
| TestCombine_VolumetricSliceOrder | Y-sorted dome slice ordering, deterministic |
| TestCombine_EmbedsExtras | Pack v1 metadata reaches scene.extras.plantastic |
| TestCombine_RoundTripParseable | Output is a valid GLB that re-parses with all expected nodes |
| TestCombine_ImageDedup | SHA256 dedup collapses duplicate image bytes across inputs |
| TestCombine_SizeCapRejection | 5 MiB cap fires with the right error |
| TestCombine_DoesNotMutateInputs | Input slices unchanged after CombinePack |

```
$ go test ./...
ok  	glb-optimizer	0.521s

$ go vet ./...
(clean)
```

The full pre-existing test suite (PackMeta, classify, settings,
strategy, billboard handlers, etc.) still passes — no regressions.

## Coverage Gaps Worth Knowing

1. **No real-asset integration test.** Every test uses synthetic
   in-memory fixtures built by `makeMinimalGLB`. The first time this
   touches a real bake-pipeline GLB will be in T-010-03 (HTTP endpoint)
   when we wire up `pack-all`. Recommendation: add one smoke test
   under T-010-04 (justfile pack-all recipe) that combines an actual
   bake output.

2. **Material remap walker is not directly unit-tested.**
   `remapMaterialIndices` and `walkRemapTextures` are exercised only
   transitively, because none of the synthetic fixtures carry
   materials. The walker is small and the regex is simple, but a
   targeted test for the remap rule (`{"baseColorTexture": {"index":
   2}}` with `texMap = [9, 8, 7]` → `{"index": 7}`) would harden it.

3. **Sparse accessors not handled.** If a future intermediate uses a
   sparse accessor, its `sparse.indices.bufferView` and
   `sparse.values.bufferView` fields will not be remapped because the
   `gltfAccessor` struct does not type the `sparse` object. Result:
   silent corruption, not a panic. The bake pipeline does not
   currently emit sparse accessors, so this is theoretical, but it
   should be flagged in the next bake-step ticket.

## Open Concerns / TODOs

1. **Material remap relies on key-name regex.** The `*Texture$`
   convention covers all standard glTF 2.0 texture references and the
   well-known KHR extensions, but a custom extension that names a
   texture reference differently (e.g. `mySpecialMap`) would silently
   skip the remap. Acceptable for the demo because we control the
   bake pipeline; worth noting for future contributors.

2. **No structured `PackTooLargeError` type.** The size cap returns a
   plain `fmt.Errorf` with the byte count. T-010-05 introduces the
   structured error type and will need to update the one call site
   in `combine.go`. This is intentional separation per the ticket
   scope.

3. **External-URI textures hashed by URI string, not bytes.** If two
   intermediates point at the same external image via different URIs
   (e.g. one absolute, one relative), they will not collapse. The
   bake pipeline currently emits bufferView-backed images only, so
   this is dormant.

4. **Animations / skins / cameras / lights silently dropped.** The
   `gltfDoc` struct has no fields for these, so they round-trip into
   and back out of `json.RawMessage` only as far as `asset` and
   `samplers` (which we do preserve). Anything else gets dropped on
   `json.Unmarshal`. Acceptable: the bake intermediates are static
   geometry. Documented in research.md.

5. **`pack_root` is hardcoded as the root node name.** Not part of
   the Pack v1 contract — the consumer uses
   `scene.getObjectByName("view_*")` to find groups directly. If the
   contract ever specifies a root name, this is the place to change.

## Critical Issues Needing Human Attention

None. The implementation is feature-complete against the ticket and
the test suite is green. The four "open concerns" above are
intentional scope boundaries documented for the next ticket in the
chain (T-010-03 HTTP endpoint, T-010-04 justfile recipe, T-010-05
size cap structured error).

## Handoff Notes for T-010-03

T-010-03 is the HTTP endpoint that exposes `CombinePack` as a button
in the dashboard. Things the next agent should know:

- `CombinePack` is **synchronous and in-process**. No goroutines, no
  external processes. Safe to call from a request handler.
- Memory footprint per call: roughly `len(side) + len(tilted) +
  len(volumetric) + output size` because `readGLB` makes private
  copies of each input BIN. For demo-scale assets (~3 × 1.5 MiB
  inputs → ~4 MiB output) this is ~10 MiB, acceptable.
- Errors are wrapped with a `combine:` prefix; the handler can
  surface them verbatim.
- The 5 MiB cap is enforced *after* the combine assembles the output,
  so a 6 MiB result still costs the full assembly time. Not worth
  pre-sizing for demo loads, but flag for production.
