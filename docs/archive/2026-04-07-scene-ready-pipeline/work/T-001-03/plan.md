# Plan: T-001-03 Rose Volumetric Distillation

## Step 1: Server Data Model and Endpoint

**Files**: `models.go`, `handlers.go`, `main.go`

Add `HasVolumetric` to `FileRecord`. Create `handleUploadVolumetric` handler (clone of
`handleUploadBillboard` with `_volumetric.glb` suffix). Add `volumetric` case to
`handlePreview`. Add `_volumetric.glb` cleanup to `handleDeleteFile`. Register route
in `main.go`.

**Verification**: `go build .` succeeds. Server starts without errors.

## Step 2: HTML Buttons

**Files**: `static/index.html`

Add "Volumetric" button (`id="generateVolumetricBtn"`, disabled) to toolbar-actions div
after the Billboard button. Add `data-lod="volumetric"` button to LOD toggle row after
the Billboard button.

**Verification**: Page loads, new buttons visible but disabled.

## Step 3: Core Rendering Function

**Files**: `static/app.js`

Implement `renderVolumetricGLB(model, numSlices, resolution)`:
1. Compute bounding box of model
2. For each slice `i` in `[0, numSlices)`:
   - angle = `i * Math.PI / numSlices`
   - Render orthographic view using existing `renderBillboardAngle(model, angle, resolution)`
   - Create PlaneGeometry sized to model bounds
   - Shift geometry origin to bottom edge (translate up by halfHeight)
   - Create MeshBasicMaterial with rendered texture, transparent=true, alphaTest=0.1,
     side=DoubleSide
   - Create Mesh, set `rotation.y = angle`, position at origin (0, 0, 0)
   - Name: `volumetric_{i}`
   - Add to export scene
3. Render top-down view using `renderBillboardTopDown(model, resolution)`
   - Create PlaneGeometry, rotate to horizontal (rotateX -PI/2)
   - Position at 70% of model height above base
   - Name: `volumetric_top`
   - Add to export scene
4. Export via GLTFExporter (binary mode)
5. Return Promise<ArrayBuffer>

**Verification**: Call from browser console with a loaded model, inspect exported GLB
structure via Three.js scene graph.

## Step 4: Generation Trigger and Upload

**Files**: `static/app.js`

Implement `generateVolumetric(id)`:
1. Guard: require `currentModel && threeReady`
2. Set button state: disabled, text "Rendering..."
3. Call `renderVolumetricGLB(currentModel, VOLUMETRIC_SLICES, VOLUMETRIC_RESOLUTION)`
4. Upload result to `/api/upload-volumetric/{id}` via POST
5. Update local file record: `has_volumetric = true`
6. Call `updatePreviewButtons()`
7. Restore button state

Wire up:
- DOM ref for `generateVolumetricBtn`
- Click event listener
- Enable/disable logic in `updatePreviewButtons()`

**Verification**: Click Volumetric button, confirm network request to upload endpoint
succeeds, `has_volumetric` flag set on file record.

## Step 5: Preview Integration

**Files**: `static/app.js`

Wire volumetric into the preview/LOD system:
1. In `updatePreviewButtons()`: enable volumetric LOD toggle button when
   `file.has_volumetric` is true
2. In the LOD button click handler: handle `data-lod="volumetric"` — set
   `previewVersion = 'volumetric'` and load via preview endpoint
3. In the file-info display: show "+Vol" indicator when has_volumetric
4. Enable generateVolumetricBtn when a file is selected and model is loaded

**Verification**: Generate volumetric, click Volumetric in LOD toggle, see the
cross-plane model in the preview with correct alpha rendering.

## Step 6: Stress Test Integration

**Files**: `static/app.js`

Add volumetric to the stress test LOD distribution:
1. In `runLodStressTest`: add `volumetric` to `lodVersions` array when available
2. In LOD bucket assignment: volumetric sits between lod3 and billboard
3. For volumetric instances: clone the model group directly (no camera-facing needed)
   using standard `InstancedMesh` or group cloning

**Verification**: Run stress test with LOD checkbox enabled on a file that has
volumetric + billboard generated. Confirm volumetric instances appear in the mid-far
range, static (no camera-facing rotation).

## Testing Strategy

**Manual testing** (primary — this is a visual feature):
1. Upload `rose_julia_child.glb`
2. Click "Volumetric" button, confirm generation completes
3. Switch to volumetric preview, verify:
   - Recognizable as rose bush from multiple angles
   - Alpha cutout works (no opaque rectangles)
   - Quads visibly intersect through center
   - Wireframe shows ~14 triangles
   - Stats show correct triangle/vertex count
4. Check file size in stats — should be <100KB
5. Run stress test with volumetric, confirm instances render correctly
6. Delete file, confirm `_volumetric.glb` cleaned up

**Build verification**:
- `go build .` succeeds after server changes
- No console errors when loading the page
- All existing functionality (upload, process, billboard, LODs) still works

**Edge cases**:
- Generate volumetric before processing (should work — uses original model)
- Re-generate volumetric (should overwrite previous)
- Generate on very small model (single mesh, few triangles)
- Generate on model with no textures/materials
