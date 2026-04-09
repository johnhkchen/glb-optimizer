# Research: T-001-05 Scene Budget System

## Current Architecture

### Server (Go)
The optimizer is a Go HTTP server (`main.go`) that wraps gltfpack and Blender for GLB optimization. Key files:

- **main.go** — HTTP server setup, route registration, startup scan of existing files. Routes are flat `/api/*` handlers. Working directory holds `originals/` and `outputs/` subdirs.
- **models.go** — In-memory `FileStore` (map + order slice, mutex-protected). `FileRecord` tracks per-file state: status, sizes, LODs, billboard/volumetric flags. `Settings` struct maps to gltfpack CLI flags.
- **handlers.go** — Individual file operations: upload, process, download, delete, LOD generation (gltfpack and Blender), billboard/volumetric upload, preview serving. Each handler operates on a single file by ID.
- **processor.go** — `BuildCommand`, `FormatCommand`, `RunGltfpack` — translates `Settings` into gltfpack CLI args and executes.
- **blender.go** — Blender detection, `BlenderLODConfig`, `RunBlenderLOD`. Embeds `scripts/remesh_lod.py`.

### Client (Browser)
- **static/app.js** — Three.js-based preview, file management UI, billboard/volumetric generation (client-side rendering to textures, exported as GLB via GLTFExporter), stress testing with instanced meshes.
- **static/index.html** — Layout: left panel (file list), center (3D preview + toolbar), right (settings).

### Processing Pipelines (from T-001-01 and T-001-03)

**Hard-surface (T-001-01):** `scripts/parametric_reconstruct.py` — Standalone Python script. Analyzes a TRELLIS-generated raised bed GLB, detects board components via vertex density, reconstructs as parametric box primitives. Output: 192 triangles, 17.9KB. Invoked externally, not integrated into the web server.

**Organic/Volumetric (T-001-03):** Client-side cross-plane impostor generation. 6 vertical slices + horizontal cap = 7 quads, 14 triangles, ~89KB. Volumetric LOD chain (`vlod0`-`vlod3`) varies slice count and resolution. Uploaded to server as opaque GLB blobs.

### LOD System
Two LOD pipelines exist:
1. **gltfpack LODs** (`handleGenerateLODs`): 4 levels with decreasing simplification ratios (0.5, 0.2, 0.05, 0.01). Server-side.
2. **Blender LODs** (`handleGenerateBlenderLODs`): 4 levels using decimate modifier. Server-side.
3. **Volumetric LODs** (`generateVolumetricLODs` in app.js): 4 levels varying slices (8/4/2/1) and resolution (256/128/64/128). Client-side.

LOD metadata (`LODMeta`) stores switch distances and total size but uses hardcoded distance values `[5.0, 15.0, 30.0]`.

## What Doesn't Exist Yet

1. **No scene concept** — Everything operates on individual files. No way to group assets, assign roles, or define a combined budget.
2. **No budget allocation** — No triangle budget or texture memory budget tracking. File sizes are tracked but not triangle counts on the server side (only client-side via Three.js).
3. **No asset type classification** — No way to mark an asset as "hard-surface" vs "organic" to route it to the appropriate distillation strategy.
4. **No scene role assignment** — No hero/mid-ground/background classification that would determine LOD level and budget share.
5. **No strategy selection** — No logic to decide: "this is a hard-surface asset, use parametric reconstruction" vs "this is organic, use volumetric distillation."
6. **No parametric reconstruction integration** — The Python script from T-001-01 is standalone, not callable from the server.
7. **No manifest output** — No combined scene manifest describing all assets' representations.

## Relevant Patterns and Constraints

- **In-memory store**: FileStore is ephemeral (resets on restart except for basic file scanning). Any scene state would follow this pattern unless we add persistence.
- **Synchronous processing**: All handlers process synchronously (the `queue` channel in main.go is created but unused). Scene optimization with multiple assets will need to handle this.
- **File naming convention**: Outputs use `{id}.glb`, `{id}_lod{0-3}.glb`, `{id}_billboard.glb`, `{id}_volumetric.glb`, `{id}_vlod{0-3}.glb`. A scene bundle would need its own naming.
- **No Go dependencies**: `go.mod` has zero external dependencies. All routing is stdlib `net/http`.
- **Python as external tool**: The parametric script runs as a subprocess, similar to gltfpack/Blender. This is an established pattern.
- **Module boundary**: Go server handles storage + orchestration; browser handles rendering + visual generation. Scene budgeting is fundamentally a server-side concern.

## Key Questions Answered by Dependencies

- **T-001-01 proved**: Hard-surface models can be reconstructed as parametric primitives with dramatic triangle reduction (6,571 vertices -> 192 triangles). The script is self-contained with graceful fallback tiers.
- **T-001-03 proved**: Organic models can be distilled to cross-plane impostors with 14 triangles. Volumetric LOD chains provide quality scaling. Generation is client-side but output is a standard GLB blob.

## Triangle Count Estimation

For the demo scene (1 raised bed + 3 rose bushes at different distances, <50K budget):
- Raised bed (parametric): 192 triangles (hero) — well under budget
- Rose bush hero (LOD0 volumetric): 16 triangles (8 slices × 2 tris)
- Rose bush mid (LOD1 volumetric): 10 triangles (4 slices + cap × 2 tris)
- Rose bush background (LOD2 volumetric): 6 triangles (2 slices + cap × 2 tris)
- Total: ~224 triangles — massively under 50K budget

The real value of the budget system is when scaling to full garden scenes with many assets, not this demo. The demo proves the routing logic works.
