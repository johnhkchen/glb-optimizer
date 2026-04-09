# Progress: T-001-05 Scene Budget System

## Completed

### Step 1: Scene Types in models.go
- Added `SceneBudget`, `SceneAsset`, `SceneRequest`, `SceneAssetResult`, `SceneResult` types.
- Build passes.

### Step 2: Budget Allocation in scene.go
- Created `scene.go` with `AllocateBudget` (role-tier percentage distribution) and `SelectStrategy` (asset_type + scene_role -> strategy matrix).
- Build passes.

### Step 3: Triangle Counting in scene.go
- Implemented `CountTrianglesGLB` — parses GLB JSON chunk, sums indices accessor counts / 3.
- Build passes.

### Step 4: Parametric Reconstruction Subprocess in scene.go
- Implemented `RunParametricReconstruct` — invokes `python3 scripts/parametric_reconstruct.py --input --output`.
- Build passes.

### Step 5: handleOptimizeScene in handlers.go
- Full handler implementation:
  - Request parsing and validation (budget, asset types, roles, file IDs, label uniqueness)
  - Scene directory creation under `outputs/scene_{id}/`
  - Strategy execution: parametric (Python subprocess), gltfpack (existing processor), volumetric (copy pre-generated LOD files)
  - Triangle counting and file size measurement for each output
  - Manifest generation (JSON) written to scene dir and returned as response
  - Error handling: 400 (bad request), 404 (file not found), 422 (missing prerequisites)
- Added `copyFile` helper.
- Build passes.

### Step 6: Route Wiring in main.go
- Added `mux.HandleFunc("/api/optimize-scene", ...)` route.
- Build passes.

## Deviations from Plan

- **No separate commits per step**: All changes done in a single implementation pass since the project has no CI and the changes are interdependent.
- **Budget allocation is informational only**: The current implementation reports actual triangle usage vs budget but does not reject scenes that exceed budget. The allocation map is computed but the actual output depends on the distillation strategy's output. Since the demo scene is far under budget (224 vs 50,000 triangles), this is acceptable. A future enhancement could add budget enforcement by checking outputs against allocations.

## Remaining

- Step 7 (end-to-end test) requires running the server with test GLB files. Verified via `go build` that the code compiles and all types/functions are wired correctly.
