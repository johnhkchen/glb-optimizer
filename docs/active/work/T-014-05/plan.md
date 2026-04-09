# T-014-05 Plan: UI Button Calls API

## Implementation Steps

### Step 1: Replace `generateProductionAsset` function body

**File**: `static/app.js` L2414-L2478

**Action**: Replace the function body from the three render+upload sequences with a single `fetch()` to `/api/build-production/{id}`.

**What changes**:
- Remove the three `renderMultiAngleBillboardGLB` / `renderTiltedBillboardGLB` / `renderHorizontalLayerGLB` calls and their corresponding upload `fetch()` calls
- Remove the three individual `store_update` calls
- Add: single `onSubstage('rendering')` call at function start
- Add: `fetch('/api/build-production/{id}?category=...')` POST call
- Add: JSON response parsing and error handling
- Add: single `store_update` from response flags
- Keep: `POST /api/bake-complete/{id}` call
- Keep: `refreshFiles()`, `updatePreviewButtons()`, `setBakeStale(false)`
- Keep: analytics `logEvent` in finally block
- Change: button text from `'Building…'` to `'Rendering via Blender…'`

**Verification**: The file should save without syntax errors. The function should be ~40 lines (down from ~60).

### Step 2: Verify prepare-for-scene integration

**File**: `static/app.js` L2660-2675

**Action**: Read-only verification. The prepare flow calls `generateProductionAsset(id, (substage) => {...})`. With the new implementation, `substage` will be `'rendering'` instead of `'horizontal'`/`'tilted'`/`'volumetric'`. The `markPrepareStage` call formats this as `"rendering bake…"` which is acceptable.

**No code change needed** — the prepare flow works correctly with the new single-substage behavior.

### Step 3: Verify post-conditions

Check that:
1. After the API call succeeds, the prepare flow's validation at L2670-2671 still passes: `after.has_billboard && after.has_billboard_tilted && after.has_volumetric`. This depends on `refreshFiles()` updating the `files` array — which it does (the server's `store.Update` sets the flags, and `refreshFiles` re-fetches from `/api/files`).
2. The `setBakeStale(false)` call is preserved.
3. The analytics event is logged with the same shape.

## Testing Strategy

This ticket modifies client-side JavaScript only. There are no Go test files to update.

**Manual verification**:
1. Load the UI, upload a GLB, optimize it, classify it
2. Click "Build hybrid impostor" — should show "Rendering via Blender..."
3. On success: file list refreshes, preview buttons update, bake-stale indicator clears
4. On error (e.g., Blender not installed): error logged to console, button re-enables
5. Prepare-for-scene flow: should complete the production stage with "rendering bake..." label

**Edge cases**:
- Server returns non-JSON error body (malformed): caught by `.catch(() => ({}))`
- Category not yet classified: query param omitted, server falls back to saved settings
- Network error: caught by the try/catch, logged to console

## Risk Mitigations

1. **Old render functions preserved**: If the API approach fails in production, individual regenerate buttons still work via client-side rendering.
2. **Blender not available**: Server returns clear error, button re-enables, no crash.
3. **Timeout**: Server has 300s timeout, browser `fetch` has no timeout — the server will respond with a timeout error if Blender takes too long.

## Commit Plan

Single atomic commit: "T-014-05: replace client-side production render with server API call"
