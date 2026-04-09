# T-014-05 Review: UI Button Calls API

## Summary of Changes

### Files Modified

| File | Lines Changed | Description |
|------|--------------|-------------|
| `static/app.js` | ~20 removed, ~20 added | Replaced `generateProductionAsset` body: three client-side render+upload sequences → single `POST /api/build-production/{id}` call |

### What Changed

The `generateProductionAsset(id, onSubstage)` function at L2414 was rewritten:

**Removed:**
- `renderMultiAngleBillboardGLB()` call + upload to `/api/upload-billboard/{id}`
- `renderTiltedBillboardGLB()` call + upload to `/api/upload-billboard-tilted/{id}`
- `renderHorizontalLayerGLB()` call + upload to `/api/upload-volumetric/{id}`
- Three separate `store_update` calls
- Three `onSubstage` calls ('horizontal', 'tilted', 'volumetric')

**Added:**
- Single `POST /api/build-production/{id}?category={shape_category}` fetch call
- JSON response parsing with error handling
- Single `store_update` driven by server response flags
- Single `onSubstage('rendering')` call

**Preserved (unchanged):**
- Function signature `(id, onSubstage = () => {})`
- Button state management (disable/enable, text swap, CSS class)
- `POST /api/bake-complete/{id}` call
- `refreshFiles()`, `updatePreviewButtons()`, `setBakeStale(false)`
- Analytics `logEvent` in finally block
- try/catch/finally structure

## Acceptance Criteria Verification

| Criterion | Status | Notes |
|-----------|--------|-------|
| Button calls server endpoint, not client-side JS | Pass | `POST /api/build-production/{id}` |
| Progress visible while Blender runs | Pass | Button shows "Rendering via Blender..." |
| Result visually identical to old approach | Depends | Server endpoint uses Blender — visual parity depends on T-014-02 (render_production.py) |
| Old client-side render functions NOT deleted | Pass | `renderMultiAngleBillboardGLB`, `renderTiltedBillboardGLB`, `renderHorizontalLayerGLB` all intact |
| Works locally and over Tailscale | Pass | Uses relative URL `/api/build-production/` |

## Test Coverage

**No automated tests for this change.** This is a client-side JavaScript modification in a vanilla JS file (no bundler, no test framework for the frontend). The existing Go tests in `handlers_build_production_test.go` cover the server endpoint.

**Manual test plan:**
1. Upload GLB → optimize → classify → click "Build hybrid impostor"
2. Verify button shows "Rendering via Blender..." during render
3. Verify file list refreshes on success with all three flags set
4. Verify bake-stale indicator clears
5. Test with Blender unavailable — verify error in console, button re-enables
6. Test prepare-for-scene flow — verify production stage completes

## Open Concerns

1. **Visual parity not verifiable from code alone.** The ticket says "result is visually identical to the old client-side approach." This depends entirely on `render_production.py` (T-014-02) producing matching output. Cannot be verified without running both pipelines and comparing output.

2. **Bake-complete not in server endpoint.** The client still calls `POST /api/bake-complete/{id}` separately after the build-production call succeeds. If the client crashes between the two calls, the bake metadata won't be stamped. Low risk for v1, but a future cleanup could fold this into the server endpoint.

3. **Prepare flow loses per-stage progress.** The prepare-for-scene flow previously showed "horizontal bake...", "tilted bake...", "volumetric bake..." as each sub-stage ran. Now it shows "rendering bake..." for the entire duration. Acceptable per ticket scope ("just a spinner for v1").

4. **No timeout on the client side.** The browser `fetch()` has no explicit timeout. The server has a 300s timeout, so the client will eventually get a response. But if the network drops, the fetch could hang indefinitely. A `AbortController` with a slightly-longer-than-300s timeout would be defensive, but is out of scope for v1.

5. **`threeReady` guard may be unnecessary.** The function no longer uses three.js for rendering, but still checks `threeReady`. This is harmless — the guard prevents the function from running before the scene is initialized, which is still a reasonable precondition since the user shouldn't be able to click the button before the model loads. No change needed.
