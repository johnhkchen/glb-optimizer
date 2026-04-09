# Review — T-009-01 tilted-billboard-bake-and-storage

## Files changed

| File | Change |
|---|---|
| `models.go` | New `HasBillboardTilted bool` field on `FileRecord` (omitempty). |
| `handlers.go` | New `handleUploadBillboardTilted` (mirror of `handleUploadBillboard`); `case "billboard-tilted"` added to `handlePreview`'s version switch; one new `os.Remove` line in `handleDeleteFile` for the new file. |
| `main.go` | Route registration `mux.HandleFunc("/api/upload-billboard-tilted/", ...)`; `scanExistingFiles` now stats `{outputsDir}/{id}_billboard_tilted.glb` and sets `record.HasBillboardTilted` accordingly. |
| `handlers_billboard_test.go` | New test file. Single test `TestHandleUploadBillboardTilted` covering happy path (200, file lands on disk, flag flipped), 404 for unknown id, 405 for wrong method. |
| `static/app.js` | Three new constants (`TILTED_BILLBOARD_ANGLES`, `TILTED_BILLBOARD_ELEVATION_RAD`, `TILTED_BILLBOARD_RESOLUTION`); `renderBillboardAngle` signature gains `elevationRad = 0` and the camera/frustum math is rewritten in the algebraically-equivalent form that reduces to the legacy expressions when elevation is 0; new `renderTiltedBillboardGLB` (side-only counterpart of `renderMultiAngleBillboardGLB`); new `async generateTiltedBillboard(id)` mirroring `generateBillboard`; `window.generateTiltedBillboard` and `window.selectedFileId` exposed for devtools reach. |
| `docs/knowledge/analytics-schema.md` | One-line addition to the `regenerate.trigger` row noting the new `billboard_tilted` value. |

## Test coverage

| Layer | Coverage | Gap |
|---|---|---|
| Go upload handler | `TestHandleUploadBillboardTilted` — happy + 404 + 405. `go test ./...` green. | Does not exercise the 10MB body limit; the limit is shared with the existing billboard upload, no new code path to cover. |
| Go preview / delete / scan touchpoints | None. Each is a one-line addition that shares its execution shape with the existing billboard / volumetric variants. | A focused integration test that POSTs to upload, GETs preview, DELETEs, and re-scans would close the loop, but the existing test suite does not have the harness for it and adding one is out of scope. The manual verification flow in the ticket covers all three. |
| JS `renderBillboardAngle` regression at `elevationRad === 0` | Algebraic argument: `cos(0) = 1.0`, `sin(0) = 0.0` are exact in IEEE-754, so `x * 1 == x` and `x + 0 == x` for all finite `x`. Every new expression reduces to the legacy expression when the elevation arg is the literal `0`. No JS test harness exists. | A visual regression test would be the best safety net but the project has no infra for it. The verifier should manually re-bake the existing horizontal billboard on a known asset and compare against a screenshot. |
| JS `renderTiltedBillboardGLB` + `generateTiltedBillboard` | None (manual). | This is the verification path the ticket explicitly asks for: `await generateTiltedBillboard(selectedFileId)` from devtools, then check the file landed, the preview endpoint serves it, restart re-detects it, and delete cleans it up. **Not run in this implement pass — owner must run before closing.** |

## Acceptance-criteria checklist

- ✅ `renderBillboardAngle(model, angleRad, resolution)` → `renderBillboardAngle(model, angleRad, resolution, elevationRad = 0)`.
- ✅ `elevationRad = 0` reproduces existing behavior bit-for-bit (algebraically guaranteed; see "Test coverage" above).
- ✅ When `elevationRad > 0`, camera lifts to `center.y + dist * sin(elev)` and orbits at horizontal radius `dist * cos(elev)`, looking at `center`.
- ✅ Captured `quadHeight` accounts for the elevated viewing angle (`(size.y * cos + maxHoriz * sin) * 0.55 * 2`) so the rendered image fits the quad.
- ✅ `renderTiltedBillboardGLB(model, numAngles, elevationRad, resolution)` exists; uses elevated camera; defaults supplied via constants.
- ✅ Constants `TILTED_BILLBOARD_ANGLES = 6`, `TILTED_BILLBOARD_ELEVATION_RAD = Math.PI / 6`, `TILTED_BILLBOARD_RESOLUTION = 512`.
- ✅ `generateTiltedBillboard(id)` async function bakes + POSTs.
- ✅ `POST /api/upload-billboard-tilted/:id` stores `{outputsDir}/{id}_billboard_tilted.glb`.
- ✅ `FileRecord.HasBillboardTilted bool` field with `omitempty`.
- ✅ `scanExistingFiles` detects `_billboard_tilted.glb` on startup.
- ✅ `handleDeleteFile` removes the new file.
- ✅ `handlePreview` accepts `version=billboard-tilted`.
- ⏳ Manual verification (devtools bake of a rose): **owner-run, not yet executed**.

## Open concerns

1. **Pre-existing scan gap.** `scanExistingFiles` does **not** detect `_billboard.glb` or `_volumetric.glb` on startup — those flags are flipped at upload time only and disappear across restarts. The new `HasBillboardTilted` detection is the first scan-side billboard-family check. The asymmetry is intentional (the ticket scopes only the new flag) but worth a follow-up: a single helper that walks all known suffixes once per record would close the gap and remove the chance of future drift.
2. **`handleDeleteFile` is a fragile flat list.** Adding the tenth `os.Remove` line was mechanical, but the next variant will be the eleventh. A loop over a `[]string{"_billboard.glb", "_billboard_tilted.glb", "_volumetric.glb", ...}` slice is the obvious refactor; out of scope here, recommended as a separate small ticket.
3. **`window.selectedFileId` getter.** The simplest way to make `selectedFileId` reachable from the devtools console (since `app.js` is a module) was a `defineProperty` getter on `window`. This is a small global-scope leak — fine for a devtools-only verification path, but if any future test framework starts running in the same global it will see this property. Document, do not block on it.
4. **Tilted-bake quad naming reuses `billboard_${i}`.** T-009-02 (the runtime loader) may want a distinct prefix to disambiguate when both files are loaded into the same scene. The current code uses the file path as the discriminator, which is sufficient for this ticket but a renaming may be cheaper now than later. Flag for the T-009-02 owner to decide.
5. **No JS visual regression for `renderBillboardAngle`.** The algebraic argument is solid but a single screenshot diff against a known asset would be cheap and worthwhile insurance. Out of scope for this ticket.

## TODOs / known limitations

- Manual devtools verification on a real asset still owed before close.
- Tilted bake currently produces side variants only — no top quad. T-009-03 will decide whether the runtime needs a tilted top variant or whether the existing horizontal `billboard_top` is sufficient.
- No toolbar UI to invoke the new bake. By design — explicitly out of scope.
- No settings slider for elevation. By design — explicitly out of scope.

## Critical issues requiring human attention

None. All acceptance criteria are met by the code; only the manual verification step from the ticket remains for the verifier to run.
