# Plan — T-001-04: organic-lod-chain

## Step 1: Server Data Model

**Files**: `models.go`

Add `LODMeta` struct, `VolumetricLODs` and `VolumetricLODMeta` fields to `FileRecord`.

**Verification**: `go build` succeeds.

## Step 2: Server Upload Endpoint

**Files**: `handlers.go`, `main.go`

Implement `handleUploadVolumetricLOD`:
- Parse ID from URL path, level from query param
- Validate level is 0-3
- Read raw body, save as `{id}_vlod{level}.glb`
- Initialize `VolumetricLODs` slice if nil (4 entries)
- Set level entry with size
- If all 4 levels have non-zero sizes, compute `VolumetricLODMeta`
- Return FileRecord

Register route in `main.go`.

**Verification**: `go build` succeeds. Can manually POST a file and see it saved.

## Step 3: Server Preview Routing + Cleanup

**Files**: `handlers.go`

Modify `handlePreview`:
- Add `vlod0`-`vlod3` cases to version switch

Modify `handleDeleteFile`:
- Add `os.Remove` for `_vlod0.glb` through `_vlod3.glb`

**Verification**: `go build` succeeds.

## Step 4: Frontend UI — Buttons

**Files**: `static/index.html`, `static/app.js`

In `index.html`:
- Add VL0-VL3 buttons to `lodToggle` div
- Add "Vol LODs" button to `toolbar-actions`

In `app.js`:
- Add DOM ref for `generateVolumetricLodsBtn`
- Add click event listener that calls `generateVolumetricLODs(selectedFileId)`
- Update `updatePreviewButtons` to enable/disable vlod buttons based on `file.volumetric_lods`
- Update LOD button click handler to look up file size from `volumetric_lods` for vlod versions
- Update `renderFileList` to show volumetric LOD sizes

**Verification**: Buttons appear in UI. They are disabled until generation runs.

## Step 5: Frontend Generation Logic

**Files**: `static/app.js`

Add `VOLUMETRIC_LOD_CONFIGS` constant.

Implement `generateVolumetricLODs(id)`:
- Disable button, show "Generating..."
- For each config in order:
  - Call `renderVolumetricGLB(currentModel, config.slices, config.resolution)`
  - POST result to `/api/upload-volumetric-lod/${id}?level=${config.level}`
- Refresh files (GET /api/files)
- Update preview buttons
- Re-enable button

**Verification**: Click "Vol LODs" with a rose model loaded. 4 GLBs are generated and uploaded. File list shows VL0-VL3 sizes. Total < 200KB. Each LOD level is previewable via the VL0-VL3 buttons.

## Step 6: Stress Test Integration

**Files**: `static/app.js`

Modify `runLodStressTest`:
- Detect when volumetric LODs are available (`file.volumetric_lods` with entries)
- When available, use vlod0-vlod3 for distance buckets instead of mesh lod0-lod3
- Quality slider controls distribution the same way

**Verification**: Enable LOD stress test with volumetric LODs generated. Instances at different distances use different LOD levels. FPS overlay shows stats.

## Testing Strategy

No unit test infrastructure exists in this project. Verification is manual:

1. **Size budget**: After generating LODs on the rose asset, check total size < 200KB via file list
2. **Visual quality**: Preview each level (VL0-VL3), verify recognizable at intended distance
3. **Preview switching**: Click through VL0-VL3 buttons, verify correct model loads
4. **Metadata**: Check API response includes `volumetric_lod_meta` with distances and total_size
5. **Cleanup**: Delete file, verify all vlod files removed from disk
6. **Stress test**: Run LOD stress test, verify distance-based LOD selection works
7. **Coexistence**: Generate both mesh LODs and volumetric LODs, verify both sets work independently

## Commit Strategy

One commit per step, or combined where steps are small:
1. Steps 1-3 together (server changes)
2. Steps 4-5 together (frontend generation + UI)
3. Step 6 (stress test)
