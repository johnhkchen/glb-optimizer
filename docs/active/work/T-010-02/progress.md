# T-010-02 Progress — Combine Implementation

## Status: implementation complete, all tests pass

## Step 1 — Skeleton + container I/O ✅
Created `combine.go` with all schema types, GLB constants, `pad4`,
`readGLB`, `writeGLB`. Build + vet clean from the first compile.

## Step 2 — Round-trip test for I/O layer ✅
Created `combine_test.go` with `makeMinimalGLB` helper, the round-trip
test, and the two argument-validation tests.
**Deviation from plan:** wrote all test cases in one batch alongside
step 5 rather than splitting between steps 2 and 5. The plan called
for landing only the minimal trio first. In practice the test helper
was already general enough that the rest came together in one pass —
no point fragmenting the commit.

## Step 3 — `mergeContext` and `absorb` pass ✅
Added `newMergeContext`, `alignBin`, `appendBytes`, `absorb`,
`absorbImage`, `remapMaterialIndices`, `walkRemapTextures`,
`textureKeyRe`, `addLeafNode`, `addGroupNode`. The 7-step absorb pass
runs in the order from design.md (bufferViews → accessors → images →
samplers → textures → materials → meshes).

## Step 4 — Routers, root assembly, real `CombinePack` ✅
Added `routeSideMeshes`, `routeTiltedMeshes`, `routeVolumetricMeshes`,
`meshMinY`, `attachExtras`, and the full `CombinePack` body that
chains parse → validate → absorb → route → root → extras → write →
cap.

## Step 5 — End-to-end test coverage ✅
All 9 tests from plan.md present. Final test list:
- `TestReadWriteRoundTrip`
- `TestCombine_RejectsNilSide`
- `TestCombine_RejectsInvalidMeta`
- `TestCombine_SideOnly_RoutesBillboardTop`
- `TestCombine_SideOnly_NoBillboardTop`
- `TestCombine_TiltedAdded_VariantNaming`
- `TestCombine_VolumetricSliceOrder`
- `TestCombine_EmbedsExtras`
- `TestCombine_RoundTripParseable`
- `TestCombine_ImageDedup`
- `TestCombine_SizeCapRejection`
- `TestCombine_DoesNotMutateInputs`

## Verification

```
$ go test ./...
ok  	glb-optimizer	0.521s

$ go vet ./...
(clean)
```

Full suite green, no regressions in PackMeta or any other package.
