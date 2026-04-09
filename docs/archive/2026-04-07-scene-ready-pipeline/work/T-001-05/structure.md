# Structure: T-001-05 Scene Budget System

## Files Modified

### models.go
Add new types for scene budget system:
- `SceneBudget` — total triangle and texture memory limits
- `SceneAsset` — per-asset definition: file_id, asset_type (hard-surface/organic), scene_role (hero/mid-ground/background), label
- `SceneRequest` — top-level request: budget + assets array
- `SceneAssetResult` — per-asset output: strategy used, triangle count, texture size, output file path, LOD chain
- `SceneResult` — response: scene_id, budget used/total, array of asset results

No changes to existing types. The scene system is additive.

### handlers.go
Add `handleOptimizeScene` handler function:
- Validates request body (budget, assets, file IDs exist in store)
- Allocates budget per role tier
- For each asset, selects strategy based on asset_type + scene_role
- Executes distillation:
  - Hard-surface hero: invoke parametric_reconstruct.py
  - Hard-surface mid/bg: invoke gltfpack with role-appropriate simplification
  - Organic: look up pre-generated volumetric LOD files
- Collects results: triangle counts, file sizes
- Writes output files to a scene subdirectory in outputs
- Returns manifest JSON

### main.go
Add route registration:
- `mux.HandleFunc("/api/optimize-scene", handleOptimizeScene(store, originalsDir, outputsDir))`

### scene.go (new file)
Budget allocation and strategy selection logic, separated from HTTP handling:
- `AllocateBudget(budget SceneBudget, assets []SceneAsset) map[string]int` — returns per-asset triangle budget
- `SelectStrategy(assetType, sceneRole string) Strategy` — returns strategy config (which tool to use, parameters)
- `Strategy` struct — tool name, simplification ratio, slice count, resolution, etc.
- `RunParametricReconstruct(inputPath, outputPath string) (string, error)` — subprocess wrapper for the Python script
- Helper: `CountTrianglesGLB(path string) (int, error)` — reads GLB binary to extract triangle count from accessor data (lightweight, no full parse needed — just read the mesh primitive's indices accessor count / 3)

## File Organization

```
models.go        — +SceneBudget, SceneAsset, SceneRequest, SceneAssetResult, SceneResult
scene.go         — budget allocation, strategy selection, parametric subprocess, triangle counting
handlers.go      — +handleOptimizeScene
main.go          — +route registration
```

## Module Boundaries

- **scene.go** owns all budget/strategy logic. No HTTP concerns.
- **handlers.go** owns request parsing, validation, response formatting. Calls into scene.go for logic.
- **models.go** owns type definitions only.
- **processor.go** and **blender.go** are unchanged — scene.go calls `RunGltfpack` from processor.go when needed.

## Public Interfaces

### Request: POST /api/optimize-scene
```
Content-Type: application/json
Body: SceneRequest
```

### Response: 200 OK
```
Content-Type: application/json
Body: SceneResult (manifest)
```

### Error Responses
- 400: Invalid request (bad JSON, missing fields, unknown asset type/role)
- 404: File ID not found in store
- 422: Missing prerequisites (organic asset without volumetric LODs)
- 507: Budget exceeded even at minimum LOD levels

## Output Directory Structure

Scene outputs go into a subdirectory of `outputs/`:
```
outputs/
  scene_{id}/
    manifest.json
    {label}.glb          — primary representation per asset
    {label}_lod0.glb     — LOD chain entries (if applicable)
    ...
```

The scene ID is generated the same way as file IDs (crypto/rand hex).

## Ordering of Changes

1. models.go — types first (no dependencies)
2. scene.go — logic (depends on types + processor.go's RunGltfpack)
3. handlers.go — HTTP handler (depends on scene.go)
4. main.go — route wiring (depends on handler)

## Triangle Counting Approach

GLB is a binary container: 12-byte header, then JSON chunk + binary chunk. The JSON chunk contains accessors. For each mesh primitive, the `indices` accessor's `count` field gives the number of index entries. Total triangles = sum of (indices.count / 3) across all primitives.

We parse only the JSON chunk (no need to read binary data). This is fast and requires no external tool — pure Go, no dependencies.

## Parametric Reconstruction Integration

The Python script (`scripts/parametric_reconstruct.py`) is invoked as:
```
python3 scripts/parametric_reconstruct.py --input <path> --output <path>
```

We need to verify the script's CLI interface. If it doesn't support `--input`/`--output` flags, we'll adapt the invocation to match its actual interface (it may use positional args or read from a hardcoded path). This will be confirmed in the Plan phase.
