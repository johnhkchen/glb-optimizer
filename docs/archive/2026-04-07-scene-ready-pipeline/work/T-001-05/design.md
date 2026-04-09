# Design: T-001-05 Scene Budget System

## Problem

The optimizer handles individual files. We need a system that takes a collection of assets with roles and a total budget, then automatically selects and applies the right distillation strategy per asset (parametric for hard-surface, volumetric for organic), producing an optimized scene bundle with a manifest.

## Options Considered

### Option A: Scene as a New Entity with Full CRUD
Add a `Scene` model with its own store, CRUD endpoints, and a scene editor UI. Users create scenes, add assets, assign roles, set budgets, then trigger optimization.

**Pros:** Clean separation, reusable scene definitions.
**Cons:** Heavy — requires new UI panels, scene management, persistence. Over-engineered for the current use case where the goal is proving the pipeline works.

### Option B: Single Endpoint, Scene Defined in Request Body
Add a `POST /api/optimize-scene` endpoint that accepts the full scene definition (assets + roles + budget) in the request body. Assets reference already-uploaded files by ID. No persistent scene entity.

**Pros:** Minimal new state. The endpoint is self-contained. The request body IS the scene definition. Easy to test with curl. Matches the acceptance criteria exactly.
**Cons:** No scene persistence. Users must re-specify the scene each time. No UI for scene composition.

### Option C: Scene Config File
Accept a JSON scene config file as upload, process it batch-style.

**Pros:** Portable scene definitions.
**Cons:** Still needs the same server-side logic as Option B, plus file parsing. Adds complexity without clear benefit over a JSON request body.

## Decision: Option B

The acceptance criteria call for `POST /api/optimize-scene` with assets + budget returning an optimized bundle. Option B delivers exactly this with minimal architectural disruption. The scene definition lives in the request body — no new persistent entities, no new UI panels required for the core feature.

Key reasons:
1. **Matches AC directly** — the endpoint signature is specified.
2. **Minimal new state** — no scene store, no scene CRUD, no additional persistence concerns.
3. **Testable in isolation** — curl a JSON body, get back results.
4. **Builds on existing patterns** — uses FileStore for asset lookup, subprocess calls for processing.

## Architecture

### Scene Budget Schema

```json
{
  "budget": {
    "max_triangles": 50000,
    "max_texture_memory_kb": 2048
  },
  "assets": [
    {
      "file_id": "abc123",
      "asset_type": "hard-surface",
      "scene_role": "hero",
      "label": "raised-bed-01"
    },
    {
      "file_id": "def456",
      "asset_type": "organic",
      "scene_role": "mid-ground",
      "label": "rose-bush-01"
    }
  ]
}
```

### Budget Allocation Strategy

Budget shares by scene role (percentage of total triangle budget):
- **hero**: 50% of remaining budget (after base allocations)
- **mid-ground**: 30%
- **background**: 15%
- **reserve**: 5% held back for overhead

Within each role tier, budget is split equally among assets of that tier.

### Strategy Selection

| Asset Type | Scene Role | Strategy | Expected Output |
|------------|-----------|----------|----------------|
| hard-surface | hero | Parametric reconstruction | ~192 tris, <50KB |
| hard-surface | mid-ground | gltfpack LOD1 (si=0.2) | Reduced mesh |
| hard-surface | background | gltfpack LOD2 (si=0.05) | Minimal mesh |
| organic | hero | Volumetric LOD0 (8 slices, 256px) | ~16 tris |
| organic | mid-ground | Volumetric LOD1 (4 slices, 128px) | ~10 tris |
| organic | background | Volumetric LOD2 (2 slices, 64px) | ~6 tris |

For hard-surface assets, parametric reconstruction is only used at hero level where quality matters most and the script can analyze the full-detail mesh. Lower LODs use gltfpack simplification since the parametric script is template-specific (raised beds).

For organic assets, volumetric distillation is used at all levels with varying slice counts and resolutions. This is because gltfpack simplification destroys the visual structure of foliage — the volumetric approach maintains recognizability even at very low triangle counts.

### Distillation Execution

**Hard-surface (parametric):** Server invokes `scripts/parametric_reconstruct.py` as a subprocess, similar to Blender. Input: original GLB path. Output: optimized GLB written to outputs dir.

**Hard-surface (gltfpack LOD):** Existing `RunGltfpack` with appropriate simplification settings.

**Organic (volumetric):** This is currently client-side. For the scene endpoint, we have two options:
1. Pre-generate volumetric LODs via the existing UI flow, then reference them in the scene request.
2. Generate server-side.

We'll go with option 1: the scene endpoint references already-generated outputs. The volumetric generation requires WebGL (Three.js rendering), which is inherently a browser operation. The scene endpoint orchestrates and bundles — it doesn't re-generate. Assets must have their LODs/volumetric outputs already generated before scene optimization.

For hard-surface parametric: the scene endpoint can invoke the Python script server-side since it has no browser dependency.

### Response Format (Manifest)

```json
{
  "scene_id": "scene_abc123",
  "budget_used": {
    "triangles": 224,
    "texture_memory_kb": 156
  },
  "budget_total": {
    "max_triangles": 50000,
    "max_texture_memory_kb": 2048
  },
  "assets": [
    {
      "label": "raised-bed-01",
      "file_id": "abc123",
      "asset_type": "hard-surface",
      "scene_role": "hero",
      "strategy": "parametric",
      "representation": "parametric_reconstruct",
      "triangle_count": 192,
      "texture_size_kb": 3,
      "output_file": "scene_abc123/raised-bed-01.glb",
      "lod_chain": [
        {"level": 0, "triangles": 192, "file": "scene_abc123/raised-bed-01_lod0.glb"}
      ]
    }
  ]
}
```

### What Gets Rejected

- **Over-budget scenes**: If allocated triangles exceed the budget even at minimum LOD levels, the endpoint returns an error with a breakdown showing which assets consume the most budget.
- **Missing prerequisites**: If an organic asset doesn't have volumetric LODs pre-generated, the endpoint returns an error listing what's missing.
- **Unknown file IDs**: Standard 404.

## What Was Rejected and Why

1. **Server-side volumetric generation**: Would require headless WebGL (puppeteer/playwright or node canvas) — too heavy a dependency for this project. The browser renders better anyway.

2. **Dynamic budget optimization (knapsack solver)**: Over-engineered. The role-based tier system with percentage allocation is simple, predictable, and sufficient. A knapsack solver would be harder to debug and explain.

3. **Scene persistence**: No CRUD for scenes. The request body is the scene definition. If persistence is needed later, it's a straightforward addition (save the request body to a file).

4. **Frontend scene builder UI**: Out of scope for this ticket. The endpoint is the deliverable. A UI can be added in a future ticket.
