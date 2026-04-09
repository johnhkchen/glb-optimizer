---
id: T-014-05
story: S-014
title: ui-button-calls-api
type: task
status: open
priority: high
phase: done
depends_on: [T-014-03]
---

## Context

The "Build hybrid impostor" button in the UI currently runs the entire production rendering pipeline client-side (three.js in the browser tab). After T-014-03, the server has `POST /api/build-production/{id}` that does the same work via Blender. The button should call the API instead.

## Changes

### `static/app.js`

The `generateProductionBtn` click handler (around the "Build hybrid impostor" button) currently:
1. Calls `renderBillboardGLB()` — client-side
2. Calls `renderBillboardTopDown()` — client-side
3. Calls `renderTiltedBillboardGLB()` — client-side
4. Calls volumetric rendering — client-side
5. Uploads each result to `POST /api/upload-billboard/{id}`, etc.

Replace with:
1. POST to `/api/build-production/{id}?category={currentCategory}`
2. Show a progress indicator ("Rendering via Blender...")
3. On success: refresh the file list (the has_billboard/tilted/volumetric flags are now true)
4. On error: show the error message from the server

### What stays client-side

- The interactive preview (loading a single billboard/tilted/volumetric GLB for preview)
- The "Production" button in the LOD toggle (already fixed to call runStressTest)
- The individual "Regenerate billboard" / "Regenerate tilted" buttons (these can also migrate to server-side, but v1 keeps them client-side as a fallback)

## Acceptance Criteria

- Clicking "Build hybrid impostor" in the UI calls the server endpoint, not client-side JS
- Progress is visible while Blender runs (a spinner or "Rendering..." text)
- The result is visually identical to the old client-side approach (same intermediates, same preview behavior)
- The old client-side render functions are NOT deleted (they remain available for the individual regenerate buttons and for debugging)
- Works with the server running locally AND over tailscale (the API call uses the current page's origin)

## Out of Scope

- Removing the old client-side render code (keep it as fallback)
- Progress bar with per-stage updates (just a spinner for v1)
- Cancellation of in-progress renders
