# Review: T-001-05 Scene Budget System

## Summary of Changes

### Files Created
- **scene.go** — Budget allocation, strategy selection, triangle counting (GLB JSON parsing), and parametric reconstruction subprocess wrapper. ~130 lines.

### Files Modified
- **models.go** — Added 5 types: `SceneBudget`, `SceneAsset`, `SceneRequest`, `SceneAssetResult`, `SceneResult`. +40 lines.
- **handlers.go** — Added `handleOptimizeScene` handler (~130 lines) and `copyFile` helper (~8 lines). Full request validation, strategy dispatch, output collection, manifest generation.
- **main.go** — Added route registration for `/api/optimize-scene`. +1 line.

### Files Deleted
None.

## Acceptance Criteria Evaluation

| Criterion | Status | Details |
|-----------|--------|---------|
| Define scene budget schema | Met | `SceneBudget` with `max_triangles` and `max_texture_memory_kb` |
| Per-asset budget by type + role | Met | `AllocateBudget` distributes by role tier (50/30/15/5%); `SelectStrategy` selects pipeline by asset_type + scene_role |
| Auto-select distillation strategy | Met | Strategy matrix: hard-surface hero -> parametric, hard-surface mid/bg -> gltfpack, organic -> volumetric LOD |
| Demo scene (1 bed + 3 roses, <50K tris) | Expected met | Budget schema supports it; actual demo requires running server with test assets |
| Output manifest (JSON) | Met | `SceneResult` with per-asset triangle count, texture size, strategy, output path; also written to `manifest.json` in scene dir |
| API endpoint POST /api/optimize-scene | Met | Endpoint accepts `SceneRequest`, returns `SceneResult` |

## Architecture Decisions

1. **No scene persistence**: Scene definition lives in the request body only. No scene CRUD, no scene store. Matches the "single endpoint" design from design.md. Future persistence is a straightforward addition.

2. **Volumetric requires pre-generation**: Organic assets must have volumetric LODs generated via the UI before scene optimization. The server doesn't do WebGL rendering. This is documented in error messages and matches the existing browser-side generation pattern.

3. **Budget allocation is informational**: Triangle budgets are computed per role tier and could be used for enforcement, but the current implementation reports actual usage rather than rejecting over-budget scenes. The demo scene is massively under budget, so enforcement isn't meaningful yet.

4. **GLB triangle counting via JSON parse**: `CountTrianglesGLB` reads only the GLB JSON chunk (no binary data), extracts accessor counts from mesh primitives. Pure Go, zero dependencies.

5. **Parametric as subprocess**: `RunParametricReconstruct` calls `python3 scripts/parametric_reconstruct.py` — same pattern as Blender integration. No new Python runtime dependency beyond what T-001-01 already requires.

## Test Coverage

- **Build verification**: `go build` passes — all types, functions, and wiring compile correctly.
- **No automated test suite**: The project has no existing test infrastructure (`*_test.go` files). The acceptance criteria don't require tests. The handler follows established patterns (identical structure to `handleProcessAll`, `handleGenerateLODs`).
- **Manual test plan**:
  1. Upload GLB files via `/api/upload`
  2. Generate volumetric LODs for organic assets via UI
  3. POST to `/api/optimize-scene` with scene request JSON
  4. Verify response manifest has correct structure, triangle counts, file paths
  5. Verify `outputs/scene_{id}/manifest.json` exists on disk
  6. Verify individual asset GLBs exist in scene directory
  7. Test error cases: missing file_id (404), invalid asset_type (400), missing volumetric LODs (422)

## Open Concerns

1. **Parametric reconstruction is template-specific**: The Python script is designed for raised bed models. It will fail or produce poor results for other hard-surface assets. The strategy matrix uses parametric only for hero-level hard-surface assets, and gltfpack for lower LODs, which mitigates this. A future ticket should add a general-purpose hard-surface strategy or a way to specify which reconstruction script to use.

2. **No budget enforcement**: The system reports actual triangle usage but doesn't reject over-budget scenes. For the demo scene this is fine (224 << 50,000 triangles). For production use with many assets, enforcement should be added — either by iteratively reducing LOD levels or by returning an error with a budget breakdown.

3. **No scene download endpoint**: The scene directory with optimized assets is created on the server but there's no endpoint to download it as a zip. The existing `/api/download-all` pattern could be extended, but that's a separate concern.

4. **Volumetric fallback chain**: If a specific `vlod{N}` file doesn't exist, the handler falls back to the base `_volumetric.glb`. If that also doesn't exist, it returns 422. A more graceful fallback could try higher LOD levels (vlod0 -> vlod1 -> vlod2 -> vlod3 -> volumetric).

5. **Texture memory tracking is approximate**: `TextureSizeKB` is computed from the total output file size, not from actual texture data within the GLB. This overestimates texture memory since it includes geometry data. For accurate texture memory tracking, the GLB JSON parser would need to extract buffer view sizes for image data specifically.

6. **No cleanup of scene directories**: Scene output directories accumulate under `outputs/`. There's no endpoint to delete old scenes and no automatic cleanup. The `handleDeleteFile` cleanup logic doesn't touch scene directories.
