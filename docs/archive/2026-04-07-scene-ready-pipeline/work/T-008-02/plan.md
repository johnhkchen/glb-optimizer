# Plan — T-008-02

Frontend-only, no test harness, manual verification only. The plan
breaks into five small steps; each leaves the app in a buildable
state.

## Step 1 — Add disclosure CSS

Append the `.toolbar-advanced` / `.advanced-panel` ruleset from
`structure.md` to `static/style.css`. No HTML changes yet.

**Verify:** none — pure addition.

## Step 2 — Restructure toolbar HTML + rename build-action labels

In `static/index.html`:

- Wrap the seven non-primary technique buttons in
  `<details class="toolbar-advanced">` with a
  `<summary class="toolbar-btn">Advanced</summary>` header and an
  `.advanced-panel` body.
- Delete `#uploadReferenceBtn` and the sibling
  `#referenceFileInput` from the toolbar.
- Rename each technique button's visible text per the design table.
- Add a one-sentence `title` to each renamed button.
- Update the lod-toggle button labels (text only; `data-lod`
  unchanged).

**Verify:** Open in a browser. The toolbar should show
`Prepare for scene` and `Advanced ▸`. Clicking Advanced should
reveal the floating panel with the seven build buttons stacked
vertically. The lod-toggle still has 11 buttons with their new
labels.

## Step 3 — Remove `Process All` and clean JS references

In `static/index.html`:

- Delete `#processAllBtn` from `.panel-actions`.

In `static/app.js`:

- Delete the `const processAllBtn = ...` declaration.
- Delete the `processAll()` function.
- Delete the `processAllBtn.addEventListener('click', processAll);`
  line.
- Delete `processAllBtn.disabled = !hasPending;` from
  `updatePreviewButtons` (and from `renderFileList` if it's also
  set there — verify with grep).
- Delete the `const uploadReferenceBtn` and
  `const referenceFileInput` declarations.
- Delete the `uploadReferenceBtn` listener and its child
  `referenceFileInput` listener.
- Delete the `uploadReferenceBtn.disabled = ...` and
  `if (file && file.has_reference) { uploadReferenceBtn.textContent
  = ... }` block in `updatePreviewButtons`.
- **Keep** `uploadReferenceImage` (the function), the
  `tuneReferenceImageBtn` element, the
  `tuneReferenceFileInput` element, and `syncReferenceImageRow` —
  they are the *new* path from T-007-03.

**Verify:** `node --check static/app.js` succeeds; `grep -n
processAllBtn static/app.js static/index.html` returns nothing;
`grep -n uploadReferenceBtn static/app.js static/index.html`
returns nothing.

## Step 4 — Update build-action JS label strings

For each generate function in `static/app.js`, update the resting
and running label strings per the table in `structure.md`. Use
`Building…` (with the ellipsis character, matching the
`Preparing…` style from T-008-01) for every running state and the
new "Build X" verb for every resting state.

**Verify:** `grep -n "Generating\\.\\.\\.\\|Rendering\\.\\.\\.\\|Remeshing\\.\\.\\." static/app.js` returns nothing
inside the generate functions (it may still appear in unrelated
strings; spot-check). Spot-check each generate function's
`.textContent` assignments.

## Step 5 — Rename tuning section labels

In `static/index.html`, inside `#tuningSection`:

- `<h3>Tuning ...` → `<h3>Asset tuning ...`
- Update each field `<label>` text per the design table. The
  inner `<span class="range-value">` and `<input>` ids stay
  unchanged.

**Verify:** Open the right panel in a browser. The section header
reads `Asset tuning`. Each slider/select still moves; the
range-value display still updates (since the JS updates the span
by id, not by surrounding label text).

## Step 6 — Build / test / lint

Run:

```
node --check static/app.js
go build ./...
go test ./...
```

All three should be clean. The Go suite is unaffected by this
ticket; it runs as a regression sanity check.

## Step 7 — Write `review.md` with rename log

Produce the handoff document with:
1. A summary of files modified.
2. The full label rename log (toolbar, lod-toggle, tuning section)
   so future contributors can grep old names → new names.
3. Test coverage notes (none added — manual verification only).
4. Open concerns / known limitations.

## Testing strategy

- **Automated:** none added. The change is presentation-only and
  the project has no JS test harness.
- **Existing Go tests:** must still pass. Run as a sanity check.
- **Manual verification checklist** (for the human reviewer):
  1. Load the app. Toolbar shows `Prepare for scene` + `Advanced
     ▸`. Confirm no other technique buttons are visible.
  2. Click `Advanced`. The floating panel reveals the seven
     renamed build buttons (six if Blender is unavailable, since
     `generateBlenderLodsBtn` is hidden by `app.js:4443`).
  3. Click `Build hybrid impostor` on a selected file. The button
     label flips to `Building…` while running and back to
     `Build hybrid impostor` when done. Repeat for the other six
     build buttons.
  4. Click `Run lighting diagnostic`. Console reports the
     lighting test as before.
  5. Look at the lod-toggle row. Confirm the four `Vol high/med/
     low/billboard` labels are present and that clicking each
     still swaps the preview to the correct volumetric LOD.
  6. Look at the right panel. The tuning section is now
     `Asset tuning`. Each renamed slider still updates its value
     readout when dragged. The reset button still works.
  7. Open `from-reference-image` lighting preset and confirm the
     in-tuning-panel `Upload reference image` button still
     appears (it was preserved from T-007-03).
  8. Confirm the left panel no longer has a `Process All` button.
     Drag in a new GLB; it auto-uploads and the user-flow is
     "select asset, click Prepare for scene." Confirm
     `Download All (.zip)` still appears once at least one file
     is `done`.
  9. **Fresh-eyes check:** show the toolbar to a teammate or to
     anyone in the project who has not seen the new labels.
     Walk them through the seven Advanced buttons. Note any
     label that gets a "what does that do?" reaction in the
     `review.md` open concerns so a follow-up can iterate.

## Risks during implementation

- **`processAll` is referenced from a place I missed.** Mitigated
  by `grep -n processAllBtn` after Step 3.
- **`uploadReferenceImage` accidentally deleted along with the
  toolbar button.** Mitigated by keeping a `grep -n
  uploadReferenceImage` check after Step 3 to confirm it's
  still defined (it is called by `tuneReferenceFileInput`'s
  change handler from T-007-03).
- **A renamed tuning field accidentally also has its `id`
  renamed.** Mitigated by reading each row's full HTML before
  editing — only the inner `<label>` text content changes.
