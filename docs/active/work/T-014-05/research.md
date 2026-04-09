# T-014-05 Research: UI Button Calls API

## Ticket Summary

Replace the `generateProductionAsset()` function in `static/app.js` so it calls the server-side `POST /api/build-production/{id}` endpoint instead of running four client-side three.js render passes plus individual uploads.

## Current Client-Side Flow

### Entry Points

1. **Direct click**: `generateProductionBtn` click handler at L4925 calls `generateProductionAsset(selectedFileId)`.
2. **Prepare-for-scene flow**: Stage 4 at ~L2660 calls `generateProductionAsset(id, onSubstage)` with a callback that updates the prepare UI progress labels.

### `generateProductionAsset(id, onSubstage)` — L2414-L2478

The function currently:

1. Disables button, shows "Building..." text (L2417-2419).
2. Calls `renderMultiAngleBillboardGLB()` — client-side three.js render.
3. Uploads result via `POST /api/upload-billboard/{id}`.
4. Calls `store_update(id, f => f.has_billboard = true)`.
5. Calls `renderTiltedBillboardGLB()` — client-side three.js render.
6. Uploads via `POST /api/upload-billboard-tilted/{id}`.
7. Calls `store_update(id, f => f.has_billboard_tilted = true)`.
8. Calls `renderHorizontalLayerGLB()` — client-side three.js render.
9. Uploads via `POST /api/upload-volumetric/{id}`.
10. Calls `store_update(id, f => f.has_volumetric = true)`.
11. Calls `POST /api/bake-complete/{id}` to stamp bake metadata.
12. Calls `refreshFiles()` and `updatePreviewButtons()`.
13. Calls `setBakeStale(false)`.
14. Logs analytics event `regenerate` with trigger `production`.
15. Re-enables button, restores "Build hybrid impostor" text.

The `onSubstage` callback is invoked before each pass ('horizontal', 'tilted', 'volumetric') for progress reporting in the prepare flow.

### Button Element

HTML at `static/index.html:69`:
```html
<button class="toolbar-btn" id="generateProductionBtn" disabled title="...">Build hybrid impostor</button>
```

### State Management

- `store_update(id, fn)` at L1248 — mutates the in-memory `files` array entry.
- `refreshFiles()` — re-fetches `/api/files` from server and re-renders file list.
- `updatePreviewButtons()` — enables/disables toolbar buttons based on current state.
- `setBakeStale(false)` — clears the "stale bake" indicator (T-007-02).

## Server-Side Endpoint (T-014-03)

### `handleBuildProduction` — handlers.go L1764

- Route: `POST /api/build-production/{id}?category={cat}`
- Validates: method POST, blender available, render script exists, asset exists, status=done.
- Resolves category from query param > saved settings > "unknown".
- Looks up strategy, builds config JSON, writes temp config to `{outputsDir}/{id}_render_config.json`.
- Acquires `blenderRenderMu` mutex (serializes renders).
- Runs Blender with 300s timeout via `render_production.py`.
- Verifies intermediate files: `{id}_billboard.glb`, `{id}_billboard_tilted.glb`, `{id}_volumetric.glb`.
- Updates FileStore flags (`HasBillboard`, `HasBillboardTilted`, `HasVolumetric`).
- Returns JSON: `{id, billboard, tilted, volumetric, duration_ms}`.

### Key Observations

1. **The server does NOT call bake-complete itself.** The client must still call `POST /api/bake-complete/{id}` after a successful build-production, or the bake-complete logic must be folded into the endpoint. Currently T-014-03 doesn't stamp bake metadata.
2. **Category from JS**: `currentSettings.shape_category` (L445, L759). No `currentCategory` global — it's always `currentSettings.shape_category`.
3. **The `onSubstage` callback won't get per-stage updates** since the server does all three stages in one HTTP call. The prepare flow caller at L2660 uses it for stage progress — this will degrade to a single "Rendering via Blender..." for the whole operation.
4. **Error response format**: The server uses `jsonError(w, status, msg)` — likely `{"error": msg}` JSON body.

## Files Involved

| File | Role |
|------|------|
| `static/app.js` L2414-2478 | `generateProductionAsset()` — primary edit target |
| `static/app.js` L4925-4927 | Click handler — no change needed |
| `static/app.js` L2660-2675 | Prepare flow caller — onSubstage behavior changes |
| `static/index.html` L69 | Button element — no change needed |
| `handlers.go` L1764-1935 | Server endpoint — already implemented |

## Constraints

- Old render functions (`renderMultiAngleBillboardGLB`, `renderTiltedBillboardGLB`, `renderHorizontalLayerGLB`) must NOT be deleted — individual regenerate buttons and debugging still use them.
- Must work over Tailscale (use relative URL, no hardcoded origin).
- No progress bar or cancellation in v1 — just a spinner/text.
- The prepare-for-scene flow must still work, though per-substage progress labels will degrade.

## Open Questions

1. Should `generateProductionAsset` call `POST /api/bake-complete/{id}` after a successful build-production, or should the server endpoint be updated to do it? (Research finding: server currently doesn't do it.)
2. The `onSubstage` callback from the prepare flow loses granularity — acceptable per ticket ("just a spinner for v1").
