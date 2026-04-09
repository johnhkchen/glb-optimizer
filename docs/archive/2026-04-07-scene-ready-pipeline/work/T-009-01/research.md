# Research — T-009-01 tilted-billboard-bake-and-storage

## Scope reminder
First brick of S-009. Bake side only: generalize the billboard renderer to take an elevation angle, add a tilted-bake function, and mirror the existing billboard upload/storage path on the server. Runtime loading + crossfade are downstream tickets.

## Existing billboard pipeline (client)

`static/app.js`
- `BILLBOARD_ANGLES = 6` constant at line 1286.
- `generateBillboard(id)` at lines 1289–1317. Disables button, calls `renderMultiAngleBillboardGLB(currentModel, BILLBOARD_ANGLES)`, POSTs the GLB body to `/api/upload-billboard/${id}`, then `store_update(id, f => f.has_billboard = true)` and `updatePreviewButtons()`. Logs a `regenerate` analytics event with `trigger: 'billboard'`. Calls `setBakeStale(false)` (T-007-02).
- `renderBillboardAngle(model, angleRad, resolution)` at lines 1319–1368.
  - Computes `Box3` of the model, `center`, `size`, `maxDim`.
  - Builds an offscreen `WebGLRenderer` (alpha, antialias, ACES tone mapping, exposure from `currentSettings.bake_exposure`).
  - Ortho camera: `halfH = size.y * 0.55`, `halfW = max(size.x, size.z) * 0.55`, near 0.01, far `maxDim * 10`.
  - Camera position: `(center.x + sin(angle) * dist, center.y, center.z + cos(angle) * dist)` with `dist = maxDim * 2`. Looks at `center` — i.e. zero elevation, horizon-level orbit.
  - Sets up bake env (`createBakeEnvironment`) and bake lights (`setupBakeLights`, omnidirectional). Clones model (`cloneModelForBake`), renders, copies the framebuffer into a 2D canvas, disposes the offscreen renderer.
  - Returns `{ canvas, quadWidth: halfW*2, quadHeight: halfH*2, center, boxMinY: box.min.y }`.
- `renderMultiAngleBillboardGLB(model, numAngles)` at lines 1721–1786. Builds an export `Scene`, loops `i in 0..numAngles`, computes `angle = (i/numAngles) * 2π`, calls `renderBillboardAngle`, builds a `PlaneGeometry(quadWidth, quadHeight)` translated up by `quadHeight/2` so the bottom edge sits at y=0, wraps it in a `MeshBasicMaterial` with `transparent`, `DoubleSide`, `alphaTest: currentSettings.alpha_test`, names it `billboard_${i}`, lays the quads side-by-side in the export scene, then bakes one extra `billboard_top` (top-down) quad via `renderBillboardTopDown`. Exports via `GLTFExporter` (binary).
- `renderBillboardTopDown(model, resolution)` at lines 1681–1719. Independent top-down pass; produces the `billboard_top` quad. Will not be reused for tilted bakes — tilted is purely the side variants from a higher viewing angle.
- `renderBillboardAngle` is also called transitively from `renderMultiAngleBillboardGLB`, which itself is invoked from the "Prepare for scene" hybrid generator at line 2294.
- Quad-naming convention `billboard_${i}` is what the runtime loader keys off (lines 3686–3777). Tilted runtime is T-009-02; we need a distinct name prefix or model file so the existing loader does not pick the tilted bake up by accident — however, since the tilted GLB lives in its own file (`_billboard_tilted.glb`) and the runtime loader is invoked separately, we can keep the same `billboard_${i}` naming inside the tilted file without conflict. T-009-02 will decide whether to rename.

## Existing billboard pipeline (server)

`handlers.go`
- `handleUploadBillboard` at lines 422–462. Reads up to 10MB, writes `{outputsDir}/{id}_billboard.glb`, sets `r.HasBillboard = true`, returns `{status, size}`.
- `handlePreview` at lines 310–351. Switch on `version` query param: `optimized`, `lod0..3`, `billboard`, `volumetric`, `vlod0..3`, default = original. Each maps to a file path under `outputsDir`.
- `handleDeleteFile` at lines 590–619. Removes original, optimized, lod0..3, `_billboard.glb`, `_volumetric.glb`, vlod0..3. **Does not currently remove future variants** — adding a new file type means another `os.Remove` line.

`main.go`
- Route registration at line 121: `mux.HandleFunc("/api/upload-billboard/", handleUploadBillboard(store, outputsDir))`.
- Preview route at line 130: `handlePreview(store, originalsDir, outputsDir)`.
- `scanExistingFiles` at lines 164–210 walks `originalsDir`, builds a `FileRecord` per `.glb`, sets `Status = StatusDone` if `outputsDir/{id}.glb` exists, populates `HasSavedSettings` / `SettingsDirty` from disk, sets `IsAccepted = AcceptedExists(...)`. **It never sets `HasBillboard` or `HasVolumetric` from disk** — those flags are only populated at upload time and are lost on restart. This is a pre-existing gap; the ticket explicitly asks scan to detect the new tilted variant, so the new code path will be the first scan-side check for any billboard-family file. Out of scope to retrofit the existing billboard/volumetric scan.

`models.go`
- `FileRecord` at lines 45–64. Has `HasBillboard`, `HasVolumetric`, `HasReference`, `HasSavedSettings`, `SettingsDirty`, `IsAccepted`, all `omitempty`. Need to add `HasBillboardTilted bool \`json:"has_billboard_tilted,omitempty"\``.

## Quad geometry math (what changes for an elevated camera)

The current zero-elevation case uses an ortho frustum sized to `(size.x or size.z, size.y)`. This works because the camera is horizontal: the projected silhouette of the model in screen space is at most `max(size.x, size.z)` wide and `size.y` tall.

When the camera is rotated up by `elevationRad`, looking down toward `center`:
- The horizontal half-extent (left/right) is unchanged: still `max(size.x, size.z) * 0.55`.
- The vertical half-extent (top/bottom of the frustum) must accommodate both:
  - the projected vertical span of the model under the new tilt, approximately `size.y * cos(elevationRad) + max(size.x, size.z) * sin(elevationRad)`,
  - times the same 0.55 padding factor.
- The captured `quadHeight` returned for downstream use must equal that screen-space vertical extent so the runtime quad shows the rendered image without stretching. Equivalently: `quadHeight = size.y * cos(elev) + max(size.x, size.z) * sin(elev)` then scaled by the same `* 1.1` (`0.55 * 2`) padding factor — i.e. compute `halfH` from the tilted formula then return `halfH * 2`.
- Camera position becomes `(center.x + sin(angle)*dist*cos(elev), center.y + dist*sin(elev), center.z + cos(angle)*dist*cos(elev))`. The ticket prescribes `center.y + dist*sin(elev)` for the Y component and azimuth at the new elevation; `dist*cos(elev)` on the horizontal leg keeps the radial distance constant (so the model stays the same apparent size as elevation increases), which matches "elevated above the model's Y center by dist*sin(elevationRad)" while preserving framing.
- Far plane `maxDim * 10` is already generous; no change needed.
- Bake lights, env, model clone, copy-canvas dance: identical, no changes.

## Constants and defaults (from acceptance criteria)

```js
const TILTED_BILLBOARD_ANGLES     = 6;
const TILTED_BILLBOARD_ELEVATION_RAD = Math.PI / 6; // 30°
const TILTED_BILLBOARD_RESOLUTION = 512;
```

These should live next to `BILLBOARD_ANGLES` (line 1286) per the ticket's First-Pass Scope direction.

## Devtools entry point

The ticket says manual verification is `generateTiltedBillboard(selectedFileId)` from devtools. `selectedFileId` is already a top-level let in `app.js` (used by the toolbar). `generateTiltedBillboard` itself needs to be reachable from the global scope when the script is loaded as a module. The existing `generateBillboard` is referenced via the `generateBillboardBtn` event listener at line 4478, not via `window.*` — so devtools access works because the dev console of a module-loaded script can still call top-level lets via `selectedFileId` only if the module exposes them. Need to verify whether a `window.generateTiltedBillboard = ...` assignment is needed for true devtools reach. (Most likely yes — `app.js` is loaded as a module per the existing `<script type="module">` pattern; module scope is sealed.)

## Symmetric-cleanup gap (called out by ticket)

`handleDeleteFile` lists nine `os.Remove` calls but no abstraction. Adding a tenth for `_billboard_tilted.glb` is mechanical and matches the existing style. The ticket frames this as "closes the symmetric-cleanup gap previously flagged" — the gap is that future variants are easy to forget. We will not refactor delete into a loop in this ticket; we add the single line and move on.

## Test surface

No existing JS unit tests for `renderBillboardAngle` (search found no spec). Verification is manual per the ticket. Server-side: existing handler tests (e.g. `accepted_test.go`, `settings_test.go`) use `httptest`. The new upload handler is mechanical enough that a focused Go unit test can mirror existing patterns and is cheap insurance. The renderer change has a regression-free clause (`elevationRad = 0` reproduces existing behavior bit-for-bit), which is enforced by code structure rather than a test: keeping the math identical when elevation is 0 (`cos(0)=1`, `sin(0)=0`).
