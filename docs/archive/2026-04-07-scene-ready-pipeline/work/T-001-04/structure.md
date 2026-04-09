# Structure — T-001-04: organic-lod-chain

## Files Modified

### models.go

Add two fields to `FileRecord`:

```go
VolumetricLODs    []LODLevel   `json:"volumetric_lods,omitempty"`
VolumetricLODMeta *LODMeta     `json:"volumetric_lod_meta,omitempty"`
```

Add new struct:

```go
type LODMeta struct {
    Distances []float64 `json:"distances"`
    TotalSize int64     `json:"total_size"`
}
```

No changes to `LODLevel`, `FileStore`, `Settings`, or `FileStatus`.

### handlers.go

**New endpoint**: `handleUploadVolumetricLOD` for `POST /api/upload-volumetric-lod/{id}`

- Reads query param `level` (0-3)
- Reads raw GLB body (10MB limit, matching existing pattern)
- Saves to `{id}_vlod{level}.glb` in outputs dir
- Updates `FileRecord.VolumetricLODs[level]` with size
- When all 4 levels are uploaded (checked by counting non-empty entries), computes `VolumetricLODMeta`:
  - `TotalSize` = sum of all 4 level sizes
  - `Distances` = `[5.0, 15.0, 30.0]` (fixed ratios of bounding sphere radius)
- Returns updated FileRecord

**Modify** `handlePreview`:
- Add case for `vlod0`, `vlod1`, `vlod2`, `vlod3` in version switch

**Modify** `handleDeleteFile`:
- Add removal of `_vlod0.glb` through `_vlod3.glb`

### main.go

**Add route**: `mux.HandleFunc("/api/upload-volumetric-lod/", handleUploadVolumetricLOD(store, outputsDir))`

### static/index.html

**Add buttons** to `lodToggle` div:
- 4 new buttons: `<button data-lod="vlod0" disabled>VL0</button>` through `vlod3`
- Place after existing volumetric button

**Add button** to `toolbar-actions`:
- `<button class="toolbar-btn" id="generateVolumetricLodsBtn" disabled>Vol LODs</button>`

### static/app.js

**New constants**:
```javascript
const VOLUMETRIC_LOD_CONFIGS = [
    { level: 0, slices: 8, resolution: 256, label: 'vlod0' },
    { level: 1, slices: 4, resolution: 128, label: 'vlod1' },
    { level: 2, slices: 2, resolution: 64,  label: 'vlod2' },
    { level: 3, slices: 1, resolution: 128, label: 'vlod3' },
];
```

**New DOM ref**: `generateVolumetricLodsBtn`

**New function** `generateVolumetricLODs(id)`:
- For each config in `VOLUMETRIC_LOD_CONFIGS`:
  - Call `renderVolumetricGLB(currentModel, config.slices, config.resolution)`
  - Upload to `/api/upload-volumetric-lod/${id}?level=${config.level}`
- Refresh file record
- Update preview buttons

**Modify** `updatePreviewButtons`:
- Show vlod0-vlod3 buttons when `file.volumetric_lods` exists and has entries
- Enable each button only if the corresponding level has been generated (no error)

**Modify** `renderFileList`:
- Add volumetric LOD size info: `VL0:XXK VL1:XXK ...`

**Modify** LOD toggle button event listener:
- Already handles any `data-lod` value via the `querySelectorAll` loop — no changes needed for click handling
- File size lookup needs to check `volumetric_lods` array for vlod versions

**Modify** `runLodStressTest`:
- When volumetric LODs are available, use them instead of mesh LODs for distance-based distribution
- Bucket mapping: vlod0 for near, vlod1 for mid, vlod2 for far, vlod3 for distant

**New event listener**: `generateVolumetricLodsBtn` click handler

## Files NOT Modified

- `processor.go` — gltfpack wrapper, not involved in volumetric pipeline
- `blender.go` — Blender integration, not involved
- `static/style.css` — existing button styles cover new buttons
- `scripts/remesh_lod.py` — Blender script, not involved

## Component Boundaries

**Client responsibility**: 3D rendering, texture generation, GLB export, LOD generation orchestration
**Server responsibility**: GLB storage, file record management, metadata computation, preview serving

The client drives the generation loop. The server is a dumb store with metadata computation triggered by upload completeness.

## Interface Contracts

### Upload Volumetric LOD
```
POST /api/upload-volumetric-lod/{id}?level={0-3}
Content-Type: application/octet-stream
Body: raw GLB bytes

Response: FileRecord (JSON)
```

### Preview Volumetric LOD
```
GET /api/preview/{id}?version=vlod{0-3}
Response: model/gltf-binary
```

### FileRecord JSON shape (new fields)
```json
{
    "volumetric_lods": [
        { "level": 0, "size": 135000 },
        { "level": 1, "size": 25000 },
        { "level": 2, "size": 6000 },
        { "level": 3, "size": 10000 }
    ],
    "volumetric_lod_meta": {
        "distances": [5.0, 15.0, 30.0],
        "total_size": 176000
    }
}
```
