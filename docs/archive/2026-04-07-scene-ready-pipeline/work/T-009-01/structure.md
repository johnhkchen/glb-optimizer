# Structure — T-009-01 tilted-billboard-bake-and-storage

## Files modified

### `models.go`
- Add field to `FileRecord` (after `HasBillboard` on line 55):
  ```go
  HasBillboardTilted bool `json:"has_billboard_tilted,omitempty"`
  ```

### `handlers.go`
- New handler `handleUploadBillboardTilted(store, outputsDir) http.HandlerFunc`, placed immediately after `handleUploadBillboard` (current ~line 462). Body is a near-mechanical clone:
  - Trim `/api/upload-billboard-tilted/` prefix from path → `id`.
  - Existence check via `store.Get(id)`.
  - Read body with `io.LimitReader(r.Body, 10<<20)`.
  - Write to `filepath.Join(outputsDir, id+"_billboard_tilted.glb")` with mode `0644`.
  - `store.Update(id, func(r *FileRecord) { r.HasBillboardTilted = true })`.
  - JSON response `{status: "ok", size: <int64>}`.
- `handlePreview` (current lines 310–351): add a `case "billboard-tilted":` arm in the version switch, mapping to `filepath.Join(outputsDir, id+"_billboard_tilted.glb")`. Place adjacent to the existing `case "billboard":` line.
- `handleDeleteFile` (current lines 590–619): add `os.Remove(filepath.Join(outputsDir, id+"_billboard_tilted.glb"))` next to the existing `_billboard.glb` removal.

### `main.go`
- Route registration block (current line 121): add
  ```go
  mux.HandleFunc("/api/upload-billboard-tilted/", handleUploadBillboardTilted(store, outputsDir))
  ```
  immediately under the existing `/api/upload-billboard/` registration.
- `scanExistingFiles` (lines 164–210): after the existing `record.IsAccepted = AcceptedExists(...)` line, stat the tilted file and set the flag:
  ```go
  if _, err := os.Stat(filepath.Join(outputsDir, id+"_billboard_tilted.glb")); err == nil {
      record.HasBillboardTilted = true
  }
  ```

### `static/app.js`
- Constants (current line 1286 area): add immediately after `BILLBOARD_ANGLES`:
  ```js
  const TILTED_BILLBOARD_ANGLES        = 6;
  const TILTED_BILLBOARD_ELEVATION_RAD = Math.PI / 6; // 30°
  const TILTED_BILLBOARD_RESOLUTION    = 512;
  ```
- `renderBillboardAngle` (current lines 1319–1368): change signature to
  ```js
  function renderBillboardAngle(model, angleRad, resolution, elevationRad = 0)
  ```
  Update the camera position and `halfH` to the tilted formulas (algebraically reducing to the legacy values when `elevationRad === 0`):
  ```js
  const cosE = Math.cos(elevationRad);
  const sinE = Math.sin(elevationRad);
  const maxHoriz = Math.max(size.x, size.z);

  const halfH = (size.y * cosE + maxHoriz * sinE) * 0.55;
  const halfW = maxHoriz * 0.55;
  // ... ortho camera as before with new halfH/halfW

  const dist = maxDim * 2;
  offCamera.position.set(
      center.x + Math.sin(angleRad) * dist * cosE,
      center.y + dist * sinE,
      center.z + Math.cos(angleRad) * dist * cosE,
  );
  offCamera.lookAt(center);
  ```
  Return tuple unchanged: `{ canvas, quadWidth: halfW * 2, quadHeight: halfH * 2, center, boxMinY: box.min.y }`.
- New function `renderTiltedBillboardGLB(model, numAngles, elevationRad, resolution)` placed immediately after `renderMultiAngleBillboardGLB` (current lines 1721–1786). Body parallels `renderMultiAngleBillboardGLB` but:
  - Uses the passed `resolution` instead of a hard-coded `512`.
  - Calls `renderBillboardAngle(model, angle, resolution, elevationRad)` in the loop.
  - Does not call `renderBillboardTopDown`; does not append a `billboard_top` quad.
  - Quads named `billboard_${i}` (same naming, different file).
  - Exports via `GLTFExporter` (binary), same Promise wrapping.
- New `async function generateTiltedBillboard(id)` placed immediately after `generateBillboard` (current lines 1289–1317). Mirrors `generateBillboard`:
  - Guards on `currentModel && threeReady`.
  - Calls `renderTiltedBillboardGLB(currentModel, TILTED_BILLBOARD_ANGLES, TILTED_BILLBOARD_ELEVATION_RAD, TILTED_BILLBOARD_RESOLUTION)`.
  - POSTs to `/api/upload-billboard-tilted/${id}` with `Content-Type: application/octet-stream`.
  - On success: `store_update(id, f => f.has_billboard_tilted = true)`, `updatePreviewButtons()`, `setBakeStale(false)`.
  - Logs analytics: `logEvent('regenerate', { trigger: 'billboard_tilted', success }, id);`.
  - No button text/disabled state changes (no toolbar button in this ticket).
- Devtools exposure: add `window.generateTiltedBillboard = generateTiltedBillboard;` near the bottom of the module (after the function is declared) so the console can call it.

## Files NOT modified
- `static/index.html` — no toolbar button this ticket.
- `static/style.css` — no UI changes.
- `static/help_text.js` — no help-text additions; devtools-only.
- `docs/knowledge/analytics-schema.md` — `regenerate` event already exists; only the `trigger` enum gains `billboard_tilted`. Worth a one-line addition to keep the doc in sync.

### `docs/knowledge/analytics-schema.md`
- Add `billboard_tilted` to the documented `regenerate.trigger` enum (one-line edit).

## Public interfaces (signatures)

```go
// handlers.go
func handleUploadBillboardTilted(store *FileStore, outputsDir string) http.HandlerFunc
```

```js
// app.js
function renderBillboardAngle(model, angleRad, resolution, elevationRad = 0)
function renderTiltedBillboardGLB(model, numAngles, elevationRad, resolution) // returns Promise<ArrayBuffer>
async function generateTiltedBillboard(id)
```

## Internal organization notes

- Constants block stays grouped at line ~1286: existing `BILLBOARD_ANGLES` then the three new tilted constants.
- New JS functions live next to their analogues in source order: `generateTiltedBillboard` after `generateBillboard`; `renderTiltedBillboardGLB` after `renderMultiAngleBillboardGLB`.
- New Go handler lives next to `handleUploadBillboard`. No new file is needed.

## Ordering of changes

The dependency chain forces this order:
1. `models.go` — field must exist before any handler can set it.
2. `handlers.go` — new upload handler + preview/delete touchpoints.
3. `main.go` — route registration + scan detection.
4. `static/app.js` — client constants, function changes, generator, devtools hook.
5. `docs/knowledge/analytics-schema.md` — doc sync.

Each step is independently compilable / loadable. Server-side steps 1–3 can be one commit; client-side step 4 a second; doc step 5 folded into step 4.
