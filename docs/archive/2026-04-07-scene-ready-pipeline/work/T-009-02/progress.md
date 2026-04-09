# Progress — T-009-02 tilted-billboard-loader-and-instances

## Plan steps

- [x] Step 1 — `tiltedBillboardInstances` state declaration added next
      to `billboardTopInstances`.
- [x] Step 2 — `createTiltedBillboardInstances` and
      `updateTiltedBillboardFacing` added immediately after
      `updateBillboardVisibility`.
- [x] Step 3 — `clearStressInstances` resets the new array.
- [x] Step 4 — animate loop calls `updateTiltedBillboardFacing()` when
      the array is non-empty (separate guard from the horizontal one).
- [x] Step 5 — `updatePreviewButtons` predicate and per-button
      `disabled` switch updated.
- [x] Step 6 — LOD click handler `fileSize` predicate extended.
- [x] Step 7 — `static/index.html` button added between
      `data-lod="billboard"` and `data-lod="volumetric"`.
- [ ] Step 8 — manual verification on a real asset (owner-run, see
      `plan.md`).

## Smoke checks run

- `go build ./...` — clean.
- `go test ./...` — `ok glb-optimizer` (cached). No Go files were
  changed in this ticket; the run is regression insurance only.
- `node --check static/app.js` — exit 0 (no syntax errors).

## Deviations from plan

None. The insertions landed exactly where `structure.md` placed them.

## Files touched

- `static/app.js` (six insertions, one declaration block, two new
  functions, three predicate extensions, one reset-line, one
  animate-loop guard).
- `static/index.html` (one new `<button>`).
- `docs/active/work/T-009-02/{research,design,structure,plan,
  progress,review}.md`.

## Outstanding before merge

- Manual browser verification of the click → preview swap path on a
  real asset (Step 8). The loader is purely additive — every behavior
  change is gated on a still-empty array or a brand-new `data-lod`
  value — so a regression on the existing horizontal/volumetric paths
  is structurally unlikely, but the new `data-lod="billboard-tilted"`
  arm is exercised end-to-end only by a browser session.
