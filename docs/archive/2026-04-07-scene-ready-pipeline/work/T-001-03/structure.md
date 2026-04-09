# Structure: T-001-03 Rose Volumetric Distillation

## Files Modified

### `models.go`

Add `HasVolumetric bool` field to `FileRecord` struct:

```go
type FileRecord struct {
    // ... existing fields ...
    HasBillboard  bool `json:"has_billboard,omitempty"`
    HasVolumetric bool `json:"has_volumetric,omitempty"`  // NEW
}
```

No new types needed — volumetric uses the same upload/storage pattern as billboard.

### `handlers.go`

**1. New handler `handleUploadVolumetric`** — follows exact pattern of `handleUploadBillboard`:
- Route: `POST /api/upload-volumetric/{id}`
- Reads GLB blob from request body (10MB limit)
- Saves to `{outputsDir}/{id}_volumetric.glb`
- Sets `FileRecord.HasVolumetric = true`
- Returns JSON status + size

**2. Modify `handlePreview`** — add case to version switch:
```go
case "volumetric":
    filePath = filepath.Join(outputsDir, id+"_volumetric.glb")
```

**3. Modify `handleDeleteFile`** — add cleanup:
```go
os.Remove(filepath.Join(outputsDir, id+"_volumetric.glb"))
```

### `main.go`

Register new route:
```go
mux.HandleFunc("/api/upload-volumetric/", handleUploadVolumetric(store, outputsDir))
```

### `static/app.js`

**1. New constants and state** (after billboard section ~line 251):

```js
const VOLUMETRIC_SLICES = 6;
const VOLUMETRIC_RESOLUTION = 256;
```

**2. New function `generateVolumetric(id)`** (~20 lines):
- Guard: check `currentModel` and `threeReady`
- Update button state (disable, show "Rendering...")
- Call `renderVolumetricGLB(currentModel, VOLUMETRIC_SLICES, VOLUMETRIC_RESOLUTION)`
- Upload result to `/api/upload-volumetric/{id}`
- Update file record: `has_volumetric = true`
- Restore button state

**3. New function `renderVolumetricGLB(model, numSlices, resolution)`** (~80 lines):
- Core rendering function. Returns Promise<ArrayBuffer> (binary GLB).
- Compute model bounding box (reuse pattern from `renderBillboardAngle`)
- For each of `numSlices` vertical slices:
  - Calculate angle: `i * Math.PI / numSlices` (180 degree spread)
  - Render orthographic view from that angle (reuse `renderBillboardAngle` logic)
  - Create `PlaneGeometry(quadWidth, quadHeight)` with origin at bottom
  - Apply `MeshBasicMaterial` with rendered texture, alphaTest, DoubleSide
  - **Key difference from billboard**: Rotate quad to match capture angle, position at center
  - `quad.rotation.y = angle` — plane faces the camera angle it was rendered from
  - `quad.position.set(0, 0, 0)` — all quads intersect at center
- Render one top-down view (reuse `renderBillboardTopDown` logic)
  - Create horizontal quad at 70% of model height
- Add all quads to export scene
- Export via `GLTFExporter` as binary

**4. Modify toolbar section** in HTML (via DOM or template):
- Add "Volumetric" button next to existing "Billboard" button in toolbar-actions div
- Button ID: `generateVolumetricBtn`

**5. Modify `updatePreviewButtons()`**:
- Enable/disable volumetric button based on file selection
- Enable volumetric LOD toggle button when `file.has_volumetric`

**6. Modify LOD toggle**:
- The existing `data-lod="billboard"` button pattern extends naturally
- Add `data-lod="volumetric"` button to LOD toggle row

**7. Modify stress test LOD distribution** (`runLodStressTest`):
- Add volumetric to the LOD version list when available
- Volumetric slots between lod3 and billboard in the distance hierarchy

**8. Add event listener** for generate button:
```js
generateVolumetricBtn.addEventListener('click', () => {
    if (selectedFileId) generateVolumetric(selectedFileId);
});
```

### `static/index.html`

**1. Add volumetric button** to toolbar-actions div:
```html
<button class="toolbar-btn" id="generateVolumetricBtn" disabled>Volumetric</button>
```

**2. Add volumetric option** to LOD toggle:
```html
<button data-lod="volumetric" disabled>Volumetric</button>
```

## File-Level Change Summary

| File | Action | Size of Change |
|------|--------|---------------|
| `models.go` | Modify | +1 line (new field) |
| `handlers.go` | Modify | +30 lines (new handler + 2 small edits) |
| `main.go` | Modify | +1 line (route registration) |
| `static/index.html` | Modify | +2 lines (buttons) |
| `static/app.js` | Modify | +120 lines (generation logic + integration) |

## Module Boundaries

- **Server side**: Minimal — just blob storage and serving. No processing logic. The server
  treats the volumetric GLB as an opaque blob, same as billboard.
- **Client side**: All rendering and GLB construction happens in the browser. The
  `renderVolumetricGLB` function is self-contained and follows the `renderMultiAngleBillboardGLB`
  pattern exactly.
- **Data model**: Single boolean flag on `FileRecord`. No new data structures needed.

## Ordering Constraints

1. Server changes (models.go, handlers.go, main.go) can be done in any order but must
   compile together — do them as one commit.
2. Frontend changes (index.html, app.js) depend on server routes existing but can be
   tested with the billboard endpoint temporarily.
3. The `renderVolumetricGLB` function should be implemented and tested before integration
   with upload/preview pipeline.

## Interface Contracts

**Upload endpoint**: `POST /api/upload-volumetric/{id}`
- Request: `Content-Type: application/octet-stream`, body = binary GLB
- Response: `{ "status": "ok", "size": <int64> }`
- Side effect: creates `{id}_volumetric.glb`, sets `has_volumetric=true`

**Preview endpoint**: `GET /api/preview/{id}?version=volumetric`
- Response: `Content-Type: model/gltf-binary`, body = GLB file
- Prerequisite: volumetric GLB exists

**GLB structure** (output of `renderVolumetricGLB`):
- N meshes named `volumetric_0` through `volumetric_{N-1}` (vertical slices)
- 1 mesh named `volumetric_top` (horizontal canopy cap)
- Each mesh: PlaneGeometry with embedded PNG texture
- Meshes pre-positioned and pre-rotated at their final world positions
