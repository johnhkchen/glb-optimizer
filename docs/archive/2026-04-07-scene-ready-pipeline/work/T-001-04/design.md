# Design — T-001-04: organic-lod-chain

## Decision Summary

Extend the existing volumetric distillation pipeline to generate 4 LOD levels with decreasing slice counts and texture resolutions. Store them as separate GLB files using a `vlod` prefix to distinguish from mesh LODs. Include LOD metadata in the FileRecord response.

## Options Evaluated

### Option A: Reuse existing LODs field, overwrite mesh LODs

Overwrite `FileRecord.LODs` when generating volumetric LODs, same `_lod0.glb` through `_lod3.glb` files.

**Pros**: No new data model, preview buttons already work.
**Cons**: Destroys mesh LOD data. Can't have both mesh and volumetric LODs simultaneously. Confusing semantics — lod0 could be mesh or volumetric depending on how it was generated.

**Rejected**: breaks existing functionality and creates ambiguity.

### Option B: New volumetric LOD fields with `vlod` prefix (Selected)

Add `VolumetricLODs []LODLevel` to FileRecord. Store files as `{id}_vlod0.glb` through `{id}_vlod3.glb`. Add new preview buttons (VL0-VL3) and version strings. Billboard becomes vlod3.

**Pros**: Clean separation from mesh LODs. Both can coexist. Clear naming. Preview routing follows existing pattern.
**Cons**: More buttons in toolbar. More files to clean up on delete.

**Selected**: clean, non-destructive, follows existing patterns.

### Option C: Single combined GLB with multiple scenes

Pack all LOD levels into one GLB using GLTF's multi-scene support. Switch by scene index.

**Pros**: Single file download. Could embed metadata in GLTF extras.
**Cons**: GLTFExporter doesn't support multi-scene export. Three.js GLTFLoader loads single scene by default. Would need custom export/load logic. Can't preview individual levels via URL.

**Rejected**: too much custom infrastructure, fights existing tooling.

## Architecture

### LOD Level Specifications

| Level | Slices | Texture Res | Expected Geometry | Expected Size |
|-------|--------|-------------|-------------------|---------------|
| vlod0 | 8      | 256x256     | 8 vertical + 1 cap = 18 tris | ~135KB |
| vlod1 | 4      | 128x128     | 4 vertical + 1 cap = 10 tris | ~25KB  |
| vlod2 | 2      | 64x64       | 2 vertical + 1 cap = 6 tris  | ~6KB   |
| vlod3 | 1 billboard | 128x128 | camera-facing quad = 2 tris  | ~10KB  |
| **Total** | | | | **~176KB** |

"Full-res" = 256x256 (matching T-001-03 baseline). This keeps the total under 200KB.

vlod3 uses the billboard pipeline (single camera-facing quad + top-down quad), not a 1-slice volumetric. The billboard is the correct far-distance representation for organic assets — it's view-dependent and cheaper than a static plane.

### Data Model Changes (models.go)

Add to `FileRecord`:
```go
VolumetricLODs []LODLevel `json:"volumetric_lods,omitempty"`
```

Each entry uses the existing `LODLevel` struct (Level, Size, Command, Error). The Command field stores the generation parameters for debugging/reproducibility.

### LOD Metadata

Add a new struct for switch distance recommendations:

```go
type LODMetadata struct {
    Distances []float64 `json:"distances"` // recommended switch distances in world units
    TotalSize int64     `json:"total_size"` // sum of all LOD sizes
}
```

Embed in the `FileRecord` as `VolumetricLODMeta *LODMetadata`. The distances are ratios of model bounding sphere radius (unitless, consumer scales to scene):
- vlod0: 0-5x radius (close-up)
- vlod1: 5-15x radius (mid-ground)  
- vlod2: 15-30x radius (background)
- vlod3: 30x+ radius (far)

### Server Changes (handlers.go)

New endpoint: `POST /api/generate-volumetric-lods/{id}`
- Accepts the volumetric LOD GLBs uploaded from the client
- Or: the client calls `POST /api/upload-volumetric-lod/{id}/{level}` per level

Since rendering is client-side, the client generates each level and uploads individually. This matches the existing billboard/volumetric upload pattern. A single "generate" button triggers all 4 levels sequentially client-side, then the FileRecord is refreshed.

**Decision**: Use a batch upload approach. Client generates all 4 GLBs, then sends them in sequence via `POST /api/upload-volumetric-lod/{id}` with level in the body or URL. After all uploads, client calls `GET /api/files` to refresh.

Simpler alternative: reuse the existing single-upload pattern but with level-specific endpoints or a level query param. `POST /api/upload-volumetric-lod/{id}?level=0` with raw GLB body.

### Preview Routing (handlers.go)

Add cases to the preview switch:
```go
case "vlod0", "vlod1", "vlod2", "vlod3":
    filePath = filepath.Join(outputsDir, id+"_"+version+".glb")
```

### Frontend Changes (app.js)

1. **New generation function** `generateVolumetricLODs(id)`:
   - Calls `renderVolumetricGLB(model, slices, resolution)` for each level
   - For vlod3: calls `renderMultiAngleBillboardGLB(model, 1)` with 128x128 resolution
   - Actually, vlod3 should be a simpler billboard: 1 front-facing quad + 1 top. Use a modified `renderVolumetricGLB(model, 1, 128)` — a single-slice volumetric is effectively a billboard.
   - Uploads each level to server
   - Updates file record

2. **LOD toggle buttons**: Add VL0-VL3 buttons to the `lodToggle` div in HTML.

3. **Stress test integration**: Add volumetric LOD buckets to `runLodStressTest`.

### File Cleanup

Delete handler adds: `_vlod0.glb`, `_vlod1.glb`, `_vlod2.glb`, `_vlod3.glb`.

### LOD3 Billboard Decision

The acceptance criteria says "1 billboard (existing billboard approach)." This means vlod3 should use the existing billboard rendering (camera-facing, not static cross-plane). However, to keep the pipeline simple and self-contained:

**Decision**: vlod3 = 1-slice volumetric (single vertical plane + top cap) at 128x128. This is close enough to a billboard for far-distance use, doesn't require separate billboard generation, and keeps the LOD chain as a single coherent pipeline. The existing billboard feature remains separate for direct use.

Rationale: At 30x+ radius distance, a single cross-plane is indistinguishable from a camera-facing billboard. The slight quality difference is invisible at that distance. This avoids coupling the volumetric LOD chain to the billboard pipeline.

## Rejected Alternatives

- **WebP texture compression in GLB**: GLTFExporter doesn't support it. Would require post-processing the GLB binary, adding complexity for modest savings.
- **Texture atlas across LOD levels**: Shared atlas would prevent loading individual LODs independently. Defeats the purpose of LOD switching.
- **Variable cap height per LOD**: Diminishing returns. 70% is good enough at all distances.
- **Separate billboard for vlod3**: Adds pipeline coupling and a special case. Single-slice volumetric is simpler and equivalent at far distances.
