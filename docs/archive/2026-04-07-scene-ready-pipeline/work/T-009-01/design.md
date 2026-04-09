# Design — T-009-01 tilted-billboard-bake-and-storage

## Decisions

### D1 — Generalize `renderBillboardAngle` with a defaulted parameter
**Choice:** Add a fourth parameter `elevationRad = 0` to `renderBillboardAngle`. When `elevationRad === 0`, the function takes the legacy code path (or equivalent expressions that algebraically reduce to the legacy values). When `elevationRad > 0`, recompute camera position and quad height using the tilted formulas.

**Rejected alternatives:**
- *New function `renderTiltedBillboardAngle` parallel to the existing one.* Cleaner separation, but doubles the code, and any future tweak to bake env / lighting / clone has to be made twice. Drift risk outweighs the cleanliness win.
- *Pass an options object `{elevationRad}` instead of a positional fourth arg.* More extensible long-term, but would require touching every existing call site (`renderMultiAngleBillboardGLB`) and changes the call shape. The ticket explicitly tells us "Resist the urge to refactor `renderBillboardAngle` beyond adding the elevation parameter" — positional default wins.

**Regression-free guarantee:** When `elevationRad === 0`:
- `cos(0) = 1`, `sin(0) = 0`.
- Camera Y becomes `center.y + dist*0 = center.y`. ✓
- Camera horizontal radius becomes `dist*1 = dist`. ✓
- `halfH` formula `(size.y * cos(elev) + maxHoriz * sin(elev)) * 0.55` reduces to `size.y * 0.55`. ✓
- All other lines (renderer setup, env, lights, clone, render, copy, dispose, return shape) are untouched.

This is *bit-for-bit* in the sense that the floating-point operations are identical when the elevation argument is the literal `0`. Multiplying by `1.0` and adding `0.0` are no-ops in IEEE-754, so we can write the new expressions unconditionally without a branch. Verified mentally; called out for the implementer.

### D2 — New `renderTiltedBillboardGLB(model, numAngles, elevationRad, resolution)`
Parallels `renderMultiAngleBillboardGLB`. Differences:
- Threads `elevationRad` and `resolution` through to `renderBillboardAngle` instead of using the hard-coded `512`.
- Does **not** call `renderBillboardTopDown` or emit a `billboard_top` quad. The tilted bake is a side-only set; the existing horizontal billboard already handles top-down. T-009-03 will decide how the runtime crossfades between the three sets (horizontal sides, top, tilted sides).
- Quad naming stays `billboard_${i}` so the runtime loader (T-009-02) can reuse the existing parsing pattern. The fact that the tilted bake lives in a separate GLB file is what disambiguates it.

**Rejected alternative:** Reuse `renderMultiAngleBillboardGLB` with a flag. Same drift-risk argument as D1; the function is short enough to duplicate, and the tilted version sheds the top-quad code, so a flag would accumulate dead branches.

### D3 — `generateTiltedBillboard(id)` mirrors `generateBillboard`
- Disables button → no button to disable in this ticket (devtools-only entry per Out of Scope).
- Builds the GLB → POSTs to new endpoint → updates `store_update(id, f => f.has_billboard_tilted = true)` → calls `updatePreviewButtons()` → `setBakeStale(false)`.
- Logs a `regenerate` analytics event with `trigger: 'billboard_tilted'`. Reusing the existing event name keeps the analytics schema additive: a new `trigger` value, no new event.
- Expose on `window` so devtools can call it: `window.generateTiltedBillboard = generateTiltedBillboard;`. Same line goes for `selectedFileId` if not already exposed — verified during Implement.

### D4 — `POST /api/upload-billboard-tilted/:id`
Mechanical copy of `handleUploadBillboard`:
- Read up to 10MB body, write to `{outputsDir}/{id}_billboard_tilted.glb`, set `r.HasBillboardTilted = true`, return `{status, size}`.
- Registered in `main.go` next to the existing billboard route.

**Rejected alternative:** Reuse `handleUploadBillboard` with a `?variant=tilted` query string. Smaller surface, but mixes two state mutations through one path and would force `handleUploadBillboard`'s tests to grow a matrix. Single-purpose route is cleaner and matches the existing volumetric/reference precedent.

### D5 — `FileRecord.HasBillboardTilted bool` (omitempty)
Add directly under `HasBillboard` in `models.go`. JSON tag `has_billboard_tilted,omitempty`. No migration concerns: the field only exists in-memory + on the wire to the front end; nothing on disk encodes it.

### D6 — `scanExistingFiles` detects `_billboard_tilted.glb`
After the existing `IsAccepted` check, stat `{outputsDir}/{id}_billboard_tilted.glb` and set `record.HasBillboardTilted = true` if present. **Pre-existing gap (research §"Existing billboard pipeline (server)"):** the scan does not currently detect `_billboard.glb` or `_volumetric.glb` either. Out of scope to retrofit; the ticket only asks for the tilted variant. Document the inconsistency in `review.md`.

### D7 — `handlePreview` accepts `version=billboard-tilted`
Add a case `"billboard-tilted"` mapping to `{outputsDir}/{id}_billboard_tilted.glb`. Use the kebab-case form (matches the runtime convention; `version=billboard-tilted` is what T-009-02 will request). Keep the cases adjacent to `billboard` for readability.

### D8 — `handleDeleteFile` removes the new file
One additional `os.Remove` line next to the existing `_billboard.glb` removal. Will not refactor the cleanup list into a loop in this ticket — that is a separate piece of cleanup the ticket explicitly does not pull in (it only calls out closing the gap for *this* file).

## Trade-offs explicitly accepted

1. **No DRY pass on the cleanup list.** Tenth `os.Remove` line, same style as the previous nine. The ticket scopes this in.
2. **No regression test for the `elevationRad === 0` path.** The regression-free clause is enforced by code review (the math reduces to the legacy expressions when elev=0). Adding a JS visual regression harness is out of scope and overkill for a single function.
3. **Devtools-only entry point.** No toolbar button, no settings UI for elevation. Both are explicitly out of scope. Implementer should resist plumbing either through.
4. **`window.generateTiltedBillboard` global pollution.** Required because `app.js` is a module and the devtools console cannot reach module-scoped names. A single named export to `window` is the smallest surface that satisfies the manual-verification criterion.
5. **Pre-existing scan gap for `_billboard.glb` / `_volumetric.glb` left in place.** Mentioned in `review.md` as a known limitation and a candidate for a follow-up ticket.

## Open questions deferred to T-009-02 / T-009-03

- Whether the tilted GLB's quad naming should change from `billboard_${i}` (currently it stays the same; T-009-02 owns the runtime side and may rename).
- Per-angle elevation tuning (e.g. lower elevation for the first/last quads) — out of scope, deferred indefinitely.
- Whether `Prepare for scene` triggers the tilted bake — explicitly T-009-03.
