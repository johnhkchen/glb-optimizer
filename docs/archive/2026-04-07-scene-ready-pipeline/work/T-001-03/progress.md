# Progress: T-001-03 Rose Volumetric Distillation

## Completed

### Step 1: Server Data Model and Endpoint
- Added `HasVolumetric bool` field to `FileRecord` in `models.go`
- Created `handleUploadVolumetric` handler in `handlers.go` (mirrors billboard pattern)
- Added `volumetric` case to `handlePreview` version switch
- Added `_volumetric.glb` cleanup to `handleDeleteFile`
- Registered `/api/upload-volumetric/` route in `main.go`
- Verified: `go build .` succeeds

### Step 2: HTML Buttons
- Added "Volumetric" button (`id="generateVolumetricBtn"`) to toolbar-actions div
- Added `data-lod="volumetric"` button to LOD toggle row
- Both placed after their Billboard counterparts

### Step 3: Core Rendering Function
- Implemented `renderVolumetricGLB(model, numSlices, resolution)` in `app.js`
- Uses `renderBillboardAngle` for each vertical slice (reuses existing rendering pipeline)
- Creates `numSlices` vertical planes evenly spaced across 180 degrees
- Each plane positioned at origin, rotated to match its capture angle (cross-plane layout)
- Adds horizontal canopy cap at 70% model height using `renderBillboardTopDown`
- Exports via GLTFExporter as binary GLB

### Step 4: Generation Trigger and Upload
- Implemented `generateVolumetric(id)` function
- Wired DOM ref for `generateVolumetricBtn`
- Added click event listener
- Uploads generated GLB to `/api/upload-volumetric/{id}`
- Updates local file record's `has_volumetric` flag

### Step 5: Preview Integration
- `updatePreviewButtons()` enables volumetric LOD toggle when `has_volumetric` is true
- LOD toggle shows when volumetric is available (alongside billboard/lods)
- LOD button click handler handles `volumetric` version
- File list shows "+Vol" indicator when volumetric exists
- Generate button enabled when file selected and model loaded

### Step 6: Stress Test Integration
- Added `volumetric` bucket to LOD distance-based distribution
- Volumetric sits between lod3 and billboard in the distance hierarchy
- `volumetric` added to `lodVersions` array
- Volumetric instances use standard `createInstancedFromModel` (no camera-facing needed)
- Memory estimate uses 50KB per volumetric instance (same as billboard)

## Deviations from Plan

None. Implementation followed the plan as written.

## Remaining

All implementation steps complete. Ready for review phase.
