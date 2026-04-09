# Progress — T-001-04: organic-lod-chain

## Step 1: Server Data Model — DONE

- Added `LODMeta` struct to `models.go` with `Distances` and `TotalSize` fields
- Added `VolumetricLODs []LODLevel` and `VolumetricLODMeta *LODMeta` to `FileRecord`
- Go build passes

## Step 2: Server Upload Endpoint — DONE

- Implemented `handleUploadVolumetricLOD` in `handlers.go`
- Accepts `POST /api/upload-volumetric-lod/{id}?level={0-3}` with raw GLB body
- Initializes 4-entry `VolumetricLODs` slice on first upload
- Computes `VolumetricLODMeta` (distances + total size) when all 4 levels present
- Registered route in `main.go`
- Go build passes

## Step 3: Server Preview Routing + Cleanup — DONE

- Added `vlod0`-`vlod3` cases to `handlePreview` version switch
- Added `os.Remove` for `_vlod0.glb` through `_vlod3.glb` in `handleDeleteFile`
- Go build passes

## Step 4: Frontend UI — Buttons — DONE

- Added VL0-VL3 buttons to `lodToggle` div in `index.html`
- Added "Vol LODs" generate button to toolbar
- Added DOM ref for `generateVolumetricLodsBtn`
- Updated `updatePreviewButtons` to enable/disable vlod buttons based on `file.volumetric_lods`
- Updated `updatePreviewButtons` to enable/disable generate button
- Updated LOD toggle click handler to look up file size from `volumetric_lods`
- Updated `renderFileList` to show volumetric LOD sizes and total

## Step 5: Frontend Generation Logic — DONE

- Added `VOLUMETRIC_LOD_CONFIGS` constant with 4 levels: 8/256, 4/128, 2/64, 1/128
- Implemented `generateVolumetricLODs(id)` — sequential generation + upload
- Added click event listener for the generate button

## Step 6: Stress Test Integration — DONE

- Modified `runLodStressTest` to detect volumetric LODs and use `vlod0`-`vlod3` buckets
- Added version validation for vlod versions
- Updated memory estimation for vlod versions
- Updated instancing to use volumetric (random Y rotation) for vlod versions

## Deviations from Plan

None. All steps completed as planned.
