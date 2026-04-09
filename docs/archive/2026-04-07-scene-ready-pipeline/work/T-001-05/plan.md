# Plan: T-001-05 Scene Budget System

## Step 1: Add Scene Types to models.go

Add the following types:
- `SceneBudget` struct: `MaxTriangles int`, `MaxTextureMemoryKB int`
- `SceneAsset` struct: `FileID string`, `AssetType string`, `SceneRole string`, `Label string`
- `SceneRequest` struct: `Budget SceneBudget`, `Assets []SceneAsset`
- `SceneAssetResult` struct: `Label string`, `FileID string`, `AssetType string`, `SceneRole string`, `Strategy string`, `TriangleCount int`, `TextureSizeKB int`, `OutputFile string`
- `SceneResult` struct: `SceneID string`, `BudgetUsed SceneBudget`, `BudgetTotal SceneBudget`, `Assets []SceneAssetResult`

**Verification:** `go build` compiles.

## Step 2: Create scene.go — Budget Allocation

Implement `AllocateBudget`:
- Input: budget + list of assets with roles
- Group assets by role
- Allocate: hero 50%, mid-ground 30%, background 15%, reserve 5%
- Within each tier, split equally among assets
- Return map[label]int (per-asset triangle budget)

Implement `SelectStrategy`:
- Input: asset_type, scene_role
- Return a `Strategy` struct with: tool name, gltfpack settings or volumetric config
- Strategy matrix per design.md

**Verification:** `go build` compiles.

## Step 3: Create scene.go — Triangle Counting

Implement `CountTrianglesGLB(path string) (int, error)`:
- Open GLB file, read 12-byte header
- Read JSON chunk
- Parse JSON, iterate mesh primitives
- Sum indices accessor counts / 3
- Return total triangle count

**Verification:** `go build` compiles. Manually test with a known GLB if available.

## Step 4: Create scene.go — Parametric Reconstruction Subprocess

Implement `RunParametricReconstruct(inputPath, outputPath string) (string, error)`:
- Execute: `python3 scripts/parametric_reconstruct.py --input <inputPath> --output <outputPath>`
- Capture combined output
- Return output string + error

**Verification:** `go build` compiles.

## Step 5: Implement handleOptimizeScene in handlers.go

The handler:
1. Parse `SceneRequest` from JSON body
2. Validate:
   - Budget fields > 0
   - Each asset has valid file_id (exists in store), valid asset_type (hard-surface/organic), valid scene_role (hero/mid-ground/background), non-empty label
   - Labels are unique
3. Generate scene ID
4. Create scene output directory: `outputs/scene_{id}/`
5. Allocate budget via `AllocateBudget`
6. For each asset, select strategy and execute:
   - **hard-surface + hero**: Run `RunParametricReconstruct`, copy output to scene dir
   - **hard-surface + mid/bg**: Run `RunGltfpack` with role-appropriate simplification
   - **organic + any**: Look for pre-generated volumetric LOD file (`{file_id}_vlod{level}.glb`), copy to scene dir. If not found, check for volumetric file, then fall back to gltfpack LOD.
7. Count triangles in each output via `CountTrianglesGLB`
8. Get file sizes for texture memory estimate
9. Build `SceneResult` manifest
10. Write `manifest.json` to scene dir
11. Return manifest as JSON response

**Verification:** `go build` compiles.

## Step 6: Wire Route in main.go

Add: `mux.HandleFunc("/api/optimize-scene", handleOptimizeScene(store, originalsDir, outputsDir))`

**Verification:** `go build` compiles. Server starts.

## Step 7: End-to-End Test

Test with curl or similar:
1. Upload a GLB file via existing `/api/upload`
2. POST to `/api/optimize-scene` with a scene request referencing the uploaded file
3. Verify response contains valid manifest with triangle counts and file paths
4. Verify output files exist in `outputs/scene_{id}/`

## Testing Strategy

- **Build verification**: Each step must compile (`go build`).
- **Integration test**: The end-to-end curl test in Step 7 validates the full pipeline.
- **No unit test file**: The project has no existing test infrastructure. Adding `scene_test.go` would be good practice but is not in the acceptance criteria. The budget allocation and strategy selection logic is straightforward enough that the integration test covers it.
- **Error path testing**: Verify 400/404/422 responses for invalid inputs, missing files, missing prerequisites.

## Commit Strategy

- Steps 1-2: commit together (types + allocation logic)
- Steps 3-4: commit together (GLB parsing + subprocess)
- Steps 5-6: commit together (handler + route)
- Step 7: no code change, just verification
