# Plan — T-009-01 tilted-billboard-bake-and-storage

## Step 1 — Add `HasBillboardTilted` field
**File:** `models.go`
**Change:** Add `HasBillboardTilted bool \`json:"has_billboard_tilted,omitempty"\`` to `FileRecord` after `HasBillboard`.
**Verify:** `go build ./...` succeeds.

## Step 2 — New upload handler + preview case + delete cleanup
**File:** `handlers.go`
**Changes:**
- Append `handleUploadBillboardTilted(store, outputsDir)` after `handleUploadBillboard`. Body mirrors `handleUploadBillboard` exactly with `_billboard_tilted.glb` and `r.HasBillboardTilted = true`.
- Add `case "billboard-tilted":` to `handlePreview`'s switch, mapping to `outputsDir/{id}_billboard_tilted.glb`.
- Add `os.Remove(filepath.Join(outputsDir, id+"_billboard_tilted.glb"))` line in `handleDeleteFile` next to the existing `_billboard.glb` removal.
**Verify:** `go build ./...` succeeds.

## Step 3 — Route registration + scan detection
**File:** `main.go`
**Changes:**
- Register `/api/upload-billboard-tilted/` route in the mux block (next to existing billboard route).
- In `scanExistingFiles`, after `IsAccepted` is set, stat `_billboard_tilted.glb` and set `record.HasBillboardTilted = true` if present.
**Verify:** `go build ./...` succeeds; `go test ./...` passes (existing tests must not regress).

## Step 4 — Tilted upload handler unit test
**File:** `handlers_test.go` (new test only — file may not exist yet; if not, create it minimally; if it does, append).
**Test name:** `TestHandleUploadBillboardTilted`
**Test body:**
- Create a temp `outputsDir`, a `FileStore` with one record, an `http.NewRequest("POST", "/api/upload-billboard-tilted/{id}", body)` carrying a small payload.
- Invoke the handler, assert HTTP 200, assert the file exists on disk at `outputsDir/{id}_billboard_tilted.glb`, assert `store.Get(id).HasBillboardTilted == true`, assert response JSON has expected `size`.
- Negative case: 404 when id is unknown.
**Verify:** `go test ./... -run TestHandleUploadBillboardTilted -v` passes.

> **Note:** if no `handlers_test.go` exists yet, check whether the project's existing test files (e.g. `accepted_test.go`, `settings_test.go`) already declare an in-package test scaffolding. If so, place this test in a new file `handlers_billboard_test.go` to avoid clobbering existing test imports. Decision deferred to Implement after a quick `ls *_test.go`.

## Step 5 — Constants and `renderBillboardAngle` parameterization
**File:** `static/app.js`
**Changes:**
- Add the three `TILTED_BILLBOARD_*` constants after `BILLBOARD_ANGLES`.
- Update `renderBillboardAngle` signature to `(model, angleRad, resolution, elevationRad = 0)`.
- Replace the camera position lines and `halfH` line with the tilted formulas described in `structure.md`.
**Verify (manual):**
- Reload page, click the existing **Build camera-facing impostor** button on a model. The result must be visually identical to before (regression-free at `elevationRad = 0`). Confirm in preview viewer.
- (Algebraic check is the primary guarantee; manual visual check is a backstop.)

## Step 6 — `renderTiltedBillboardGLB` and `generateTiltedBillboard`
**File:** `static/app.js`
**Changes:**
- Add `renderTiltedBillboardGLB(model, numAngles, elevationRad, resolution)` after `renderMultiAngleBillboardGLB`.
- Add `async function generateTiltedBillboard(id)` after `generateBillboard`.
- Expose on `window`: `window.generateTiltedBillboard = generateTiltedBillboard;` near the end of the module.
**Verify (manual, this is the ticket's stated verification path):**
- Load a rose, select it (`selectedFileId` populated), open devtools console.
- Run `await generateTiltedBillboard(selectedFileId)`.
- Inspect the network tab: POST to `/api/upload-billboard-tilted/{id}` returns 200.
- Inspect the outputs directory on the server: `{id}_billboard_tilted.glb` exists, non-trivial size (~tens of KB).
- Hit `/api/preview/{id}?version=billboard-tilted` directly in a new browser tab; browser should download the GLB.
- Restart the server, confirm `scanExistingFiles` re-detects the file and `GET /api/files` returns `has_billboard_tilted: true` for the asset.
- Delete the asset via the UI; confirm `_billboard_tilted.glb` is removed from disk.

## Step 7 — Analytics doc sync
**File:** `docs/knowledge/analytics-schema.md`
**Change:** Add `billboard_tilted` to the documented `regenerate.trigger` enum (one-line edit).
**Verify:** Visual diff only.

## Testing strategy summary

| Layer | Coverage |
|---|---|
| Go upload handler | Unit test (`TestHandleUploadBillboardTilted`) — happy + 404. |
| Go preview/delete/scan | No new tests; the touchpoints are one-line additions sharing the same code paths as existing variants. Manual verification step 6 covers them end-to-end. |
| JS `renderBillboardAngle` regression | Algebraic argument (`elev=0` reduces to legacy expressions exactly under IEEE-754) + manual visual check in step 5. No JS test harness exists. |
| JS `renderTiltedBillboardGLB` + `generateTiltedBillboard` | Manual devtools verification per ticket acceptance. |

## Commit boundaries

1. After step 3: server-side scaffolding (model + handlers + routes + scan).
2. After step 4: handler unit test.
3. After step 6: client-side bake + generator + devtools hook.
4. After step 7: analytics doc.

Four small commits, each independently buildable and runnable.

## Risks and mitigations

- **Risk:** `selectedFileId` is module-scoped and not reachable from devtools. **Mitigation:** test the global access during step 6; if blocked, also attach `window.selectedFileId` getter or instruct verifier to grab the id from the URL/file row instead.
- **Risk:** Quad height math wrong → image stretched in T-009-02. **Mitigation:** manual visual check at step 6 against a non-symmetric model (a tall/narrow rose); confirm aspect ratio.
- **Risk:** Existing scan does not detect non-tilted billboard files (pre-existing gap). **Mitigation:** documented in `review.md` as known limitation, not fixed in this ticket.
