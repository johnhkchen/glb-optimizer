# Progress — T-009-01 tilted-billboard-bake-and-storage

All seven plan steps complete. No deviations from the plan.

| Step | Status | Notes |
|---|---|---|
| 1 — `HasBillboardTilted` field on `FileRecord` | ✅ | `models.go:56`. |
| 2 — `handleUploadBillboardTilted` + `handlePreview` case + `handleDeleteFile` cleanup | ✅ | New handler ~`handlers.go:464–509`; `case "billboard-tilted"` in `handlePreview`; one extra `os.Remove` line in `handleDeleteFile`. |
| 3 — Route registration + `scanExistingFiles` detection | ✅ | `main.go:122` registers `/api/upload-billboard-tilted/`; `scanExistingFiles` stats `_billboard_tilted.glb` and sets the flag. |
| 4 — Go unit test | ✅ | New file `handlers_billboard_test.go`, single test `TestHandleUploadBillboardTilted` with happy / 404 / wrong-method cases. `go test ./...` green. |
| 5 — Constants + `renderBillboardAngle` parameterization | ✅ | Three new `TILTED_BILLBOARD_*` constants next to `BILLBOARD_ANGLES`; `renderBillboardAngle` gains `elevationRad = 0` and the camera/`halfH` math is rewritten in the algebraic form that reduces to the legacy values when `elevationRad === 0`. |
| 6 — `renderTiltedBillboardGLB` + `generateTiltedBillboard` + devtools globals | ✅ | New JS functions next to their analogues; `window.generateTiltedBillboard` and `window.selectedFileId` (defineProperty getter) exposed for the manual verification path. |
| 7 — Analytics doc sync | ✅ | `docs/knowledge/analytics-schema.md` `regenerate.trigger` row notes the new `billboard_tilted` value. |

## Manual verification status

The manual verification flow described in the ticket (devtools `await generateTiltedBillboard(selectedFileId)` against a real rose, then visual + filesystem check, restart, delete) is **not** automated and was not run in this implement pass. The Go server build is green and the unit test covers the upload path end-to-end against the in-memory store. The JS side has only been syntax-checked. The verifier should run the manual flow before closing the ticket.

## Pre-existing issues observed but not fixed

- `scanExistingFiles` does not set `HasBillboard` or `HasVolumetric` from disk on startup — these flags are upload-only and are lost on restart. The new `HasBillboardTilted` detection is the first scan-side check for any billboard-family flag. Documented in `review.md` as a candidate for follow-up; not in scope for this ticket.
- `handleDeleteFile` is a flat list of `os.Remove` calls; adding a tenth is mechanical but the list is fragile. Not refactored — explicitly out of scope.
