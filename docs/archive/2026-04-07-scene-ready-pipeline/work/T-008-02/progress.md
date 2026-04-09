# Progress — T-008-02

All seven plan steps executed in one continuous pass.

## Step status

- [x] **Step 1 — disclosure CSS.** Added the `.toolbar-advanced` /
  `.advanced-panel` ruleset to `static/style.css`, just above the
  T-007-02 stale-bake hint rule.
- [x] **Step 2 — toolbar HTML restructure + label rename.** Replaced
  the flat `.toolbar-actions` row with the `<details>`-based
  disclosure containing seven build buttons. Renamed each toolbar
  technique button. Removed `#uploadReferenceBtn`. Kept the hidden
  `#referenceFileInput` because the in-tuning-panel reference button
  reuses it.
- [x] **Step 3 — Process All removal + JS reference cleanup.**
  Deleted `#processAllBtn` from `index.html`, deleted the
  `processAll` function, the `processAllBtn` const, the
  `processAllBtn.disabled` line in `renderFileList`, and the
  `processAllBtn.addEventListener` wiring. Deleted the
  `uploadReferenceBtn` const + listener + the
  `updatePreviewButtons` block that toggled its label/title.
- [x] **Step 4 — build-action JS label strings.** Updated all
  twelve resting/running label assignments across `generateLODs`,
  `generateBlenderLODs`, `generateBillboard`, `generateVolumetric`,
  `generateVolumetricLODs`, and `generateProductionAsset`. Running
  labels are now `Building…`; resting labels match the new toolbar
  text.
- [x] **Step 5 — tuning section + field renames.** `<h3>Tuning ...`
  → `<h3>Asset tuning ...`. Twelve field labels updated; all
  `<input>` / `<select>` / range-value `<span>` ids preserved.
- [x] **Step 6 — build / lint.** `node --check static/app.js` clean.
  `go build ./...` clean. `go test ./...` passes (`ok glb-optimizer
  (cached)`).
- [x] **Step 7 — `review.md` written.** See `review.md`.

## Deviations from `plan.md`

- **Hidden `#referenceFileInput` was preserved**, not deleted. The
  `wireTuningUI` handler at `app.js:772-777` already reused this
  input for the in-tuning-panel reference button (added in
  T-005-03 / T-007-03). Deleting the input would have broken that
  flow. The const declaration and the change-event listener stay;
  only the visible toolbar button + its click listener are gone.
  This deviation is the safest answer to "remove the duplicate
  upload path" because the in-panel path is the canonical one.
- **`/api/process-all` Go route stays mounted** (not touched). It
  is reachable from devtools/curl. Removing it is server-side
  cleanup outside the ticket's UI scope.

## Verification performed

- `grep -n processAllBtn static/app.js static/index.html` →
  three matches, all in `T-008-02` removal comments.
- `grep -n uploadReferenceBtn static/app.js static/index.html` →
  three matches, all in `T-008-02` removal comments.
- `grep -n "Generating\\.\\.\\.\\|Rendering\\.\\.\\.\\|Remeshing\\.\\.\\." static/app.js`
  was the spot-check; the running-label `…` strings are now
  `Building…` inside every generate function. (Other `Rendering`
  references in unrelated strings — e.g. error messages — were
  intentionally left alone.)
- `node --check static/app.js` returns 0.
- `go build ./...` and `go test ./...` both pass.

## Manual verification

Not performed in this implementation pass — the agent has no
browser. The full manual checklist lives in `plan.md` step 7 and
the AC's bullet 8. The reviewer should walk through it before
merging.
