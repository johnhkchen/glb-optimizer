# Research — T-001-04: organic-lod-chain

## Scope

Map the codebase components relevant to generating a multi-level LOD chain for organic assets using volumetric distillation at varying quality levels.

## Existing Volumetric Pipeline (T-001-03)

The volumetric distillation system renders cross-plane impostors client-side in the browser:

- **Constants**: `VOLUMETRIC_SLICES = 6`, `VOLUMETRIC_RESOLUTION = 256` (app.js:439-440)
- **Core function**: `renderVolumetricGLB(model, numSlices, resolution)` — already parameterized (app.js:464)
- **Geometry**: N vertical planes at evenly-spaced angles across 180 degrees (double-sided), plus 1 horizontal canopy cap at 70% model height
- **Material**: `MeshBasicMaterial` with `CanvasTexture`, `alphaTest: 0.1`, `DoubleSide`
- **Rendering**: Uses `renderBillboardAngle()` for each slice, `renderBillboardTopDown()` for cap
- **Export**: `GLTFExporter` outputs binary GLB with embedded PNG textures
- **Upload**: `POST /api/upload-volumetric/{id}` saves single file `{id}_volumetric.glb`

Key: the rendering function already accepts `numSlices` and `resolution` as parameters. The constants are only used at the call site in `generateVolumetric()`.

## Existing Billboard Pipeline

- **Constants**: `BILLBOARD_ANGLES = 6`, resolution hardcoded to 512 in `renderMultiAngleBillboardGLB` (app.js:371)
- **Geometry**: N side quads arranged side-by-side (for preview), plus 1 top-down quad
- **Upload**: `POST /api/upload-billboard/{id}` saves `{id}_billboard.glb`
- **Stress test**: camera-facing instances with per-frame rotation + overhead fade

Billboard is explicitly called out as LOD3 in the acceptance criteria — it already exists.

## Existing gltfpack LOD System

The server generates mesh-based LODs via gltfpack (handlers.go:340-406):

- **4 levels**: lod0 (0.5 simplification) through lod3 (0.01 simplification)
- **Output files**: `{id}_lod0.glb` through `{id}_lod3.glb`
- **Data model**: `FileRecord.LODs []LODLevel` with Level, Size, Command, Error
- **Preview routing**: `?version=lod0` through `?version=lod3` (handlers.go:318-319)
- **UI**: LOD toggle buttons in toolbar (index.html:40-46)

This system is for mesh simplification of the original 3D model, not for volumetric impostors. The organic LOD chain reuses the naming concept but is a different pipeline.

## Server Data Model (models.go)

```go
type FileRecord struct {
    LODs          []LODLevel     // currently mesh-based LODs from gltfpack
    HasBillboard  bool           // single billboard GLB exists
    HasVolumetric bool           // single volumetric GLB exists
}
```

No support for multiple volumetric LOD levels. The `LODs` field is tied to gltfpack mesh LODs. Billboard and volumetric are boolean flags (single file each).

## Preview System (handlers.go:299-337)

The preview endpoint routes by version string:
- `original`, `optimized` — base files
- `lod0`-`lod3` — mesh LODs
- `billboard`, `volumetric` — impostor variants

File naming: `{id}_{version}.glb`. Adding new version strings (e.g., `vlod0`-`vlod3`) follows this pattern naturally.

## Frontend LOD Toggle (app.js:1099-1114)

LOD buttons are hardcoded in HTML and queried via `lodToggle.querySelectorAll('button')`. Each button has `data-lod` attribute matching the version string. Adding new buttons requires HTML changes and the JS will pick them up automatically via the existing `querySelectorAll` loop.

## Stress Test LOD Distribution (app.js:978-1066)

The LOD stress test assigns instances to buckets by distance from center, using quality-dependent thresholds. Currently uses: `lod0`, `lod1`, `lod2`, `lod3`, `volumetric`, `billboard`. The organic LOD chain needs its own distribution logic since volumetric LODs replace mesh LODs.

## File Cleanup (handlers.go:492-518)

Delete handler removes fixed set of files: `_lod0.glb` through `_lod3.glb`, `_billboard.glb`, `_volumetric.glb`. New volumetric LOD files need cleanup entries.

## Startup Scanner (main.go:137-168)

`scanExistingFiles` only checks for original and optimized files. Doesn't detect LODs, billboard, or volumetric on restart. This is a pre-existing limitation, not in scope.

## Size Budget Analysis

From T-001-03 design: a 6-slice volumetric at 256x256 is estimated ~89KB. The acceptance criteria:
- LOD0: 8 slices, full-res — "full-res" is ambiguous; 512x512 would be expensive
- LOD1: 4 slices, half-res
- LOD2: 2 slices, quarter-res
- LOD3: 1 billboard
- Total < 200KB

Rough estimates (PNG textures embedded in GLB):
- LOD0 at 512x512: 9 textures * ~40KB = ~360KB (over budget alone)
- LOD0 at 256x256: 9 textures * ~15KB = ~135KB (tight but possible)
- LOD1 at 128x128: 5 textures * ~5KB = ~25KB
- LOD2 at 64x64: 3 textures * ~2KB = ~6KB
- LOD3 billboard at 128x128: 2 textures * ~5KB = ~10KB

Using 256 as "full-res" (matching T-001-03): total ~176KB. Feasible within 200KB.

## Metadata Output

Acceptance criteria: "LOD selection metadata (recommended switch distances) included in output." This doesn't exist anywhere in the codebase. Needs a new data structure to communicate switch distances to consumers. Could be a JSON sidecar or embedded in the FileRecord response.

## Key Constraints

1. All rendering happens client-side in the browser — no server-side 3D rendering
2. `renderVolumetricGLB` is already parameterized — reuse is straightforward
3. The billboard pipeline is distinct from volumetric — LOD3 uses billboard, not a 1-slice volumetric
4. The existing `LODs` field on FileRecord is for mesh LODs — volumetric LODs need separate tracking
5. GLB export uses PNG textures — no compression option (WebP/KTX2 not available in GLTFExporter)
6. Total 200KB budget is tight; "full-res" must mean 256x256, not 512x512
