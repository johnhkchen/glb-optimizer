# Structure — T-008-02

Frontend-only ticket. No new files. No Go changes. No analytics
schema changes.

## Files modified

### `static/index.html`

1. **`.toolbar-actions` block (lines ~53-65).** Replace the flat
   list of technique buttons with:
   ```
   #prepareForSceneBtn  (kept first, label unchanged)
   <details class="toolbar-advanced">
     <summary class="toolbar-btn">Advanced</summary>
     <div class="advanced-panel">
       #generateLodsBtn          → "Build LOD chain"
       #generateBlenderLodsBtn   → "Build LOD chain (Blender remesh)"
       #generateBillboardBtn     → "Build camera-facing impostor"
       #generateVolumetricBtn    → "Build volumetric dome slices"
       #generateVolumetricLodsBtn→ "Build volumetric LOD chain"
       #generateProductionBtn    → "Build hybrid impostor"
       #testLightingBtn          → "Run lighting diagnostic"
     </div>
   </details>
   #bakeStaleHint  (kept as a sibling, NOT inside <details>)
   ```
   `#uploadReferenceBtn` and the sibling `#referenceFileInput` are
   **deleted** entirely. (See JS section for the listener cleanup.)
   The `title` attributes on the renamed buttons get a sentence
   each that frames the action for a non-engineer.

2. **`.lod-toggle` block (lines ~39-51).** Update the visible label
   text only. `data-lod` attributes stay identical:
   - `Billboard` → `Camera-facing`
   - `Volumetric` → `Dome slices`
   - `Production` → `Hybrid`
   - `VL0` → `Vol high`
   - `VL1` → `Vol med`
   - `VL2` → `Vol low`
   - `VL3` → `Vol billboard`
   - `LOD0..LOD3` unchanged.

3. **`.panel-actions` block (lines ~26-29).** Delete
   `#processAllBtn`. `#downloadAllBtn` stays.

4. **Tuning section (`#tuningSection`, lines ~231-341).**
   - `<h3>Tuning ...` → `<h3>Asset tuning ...` (preserves the
     `#tuningDirtyDot` span).
   - Each field label updated per the design table. The
     `<input>` / `<select>` ids and the
     `range-value` span ids stay unchanged so the JS that reads
     them by id keeps working.

### `static/app.js`

1. **Top-level constants** (lines ~45, ~59-62, etc.). Delete:
   - `const processAllBtn = ...`
   - `const uploadReferenceBtn = ...` (verify name; line ~55 area)
   - `const referenceFileInput = ...`

2. **`processAll()` function (lines 1210-1233).** Delete entirely.

3. **`processFile`** (line 1182) stays — it's still called by
   `prepareForScene` and the per-file process button rendered by
   `renderFileList`. Verify nothing else inside the file
   references `processAllBtn`.

4. **`updatePreviewButtons()`** (line ~4140-4186):
   - Delete the line `processAllBtn.disabled = !hasPending;`
     (line 2977 is in another function — `renderFileList` —
     verify both sites and remove from each).
   - Delete the `uploadReferenceBtn.disabled = !file;` line and
     the `if (file && file.has_reference) { uploadReferenceBtn.
     textContent = 'Reference ✓'; } else { ... }` block.

5. **Build-action label re-sets.** Each generate function resets
   its button label after the operation. Update the resting label
   strings to match the new names AND change the `'Generating...'`
   / `'Rendering...'` strings to `'Building…'`:
   - `app.js:1247` `'Generating...'` → `'Building…'`
   - `app.js:1261` `'LODs (gltfpack)'` → `'Build LOD chain'`
   - `app.js:1267` Blender remesh: `'Remeshing...'` →
     `'Building…'`
   - `app.js:~1280` Blender resting label →
     `'Build LOD chain (Blender remesh)'` (verify exact line)
   - `app.js:1294` `'Rendering...'` → `'Building…'`
   - `app.js:1316` `'Billboard'` → `'Build camera-facing impostor'`
   - `app.js:1718` `'Rendering...'` → `'Building…'`
   - `app.js:1739` `'Volumetric'` →
     `'Build volumetric dome slices'`
   - `app.js:2172` `'Generating...'` → `'Building…'`
   - `app.js:2195` `'Vol LODs'` → `'Build volumetric LOD chain'`
   - `app.js:2206` `'Rendering...'` → `'Building…'`
   - `app.js:2239` `'Production Asset'` → `'Build hybrid impostor'`

6. **Event-listener cleanup** (lines 4198-4318):
   - Delete `processAllBtn.addEventListener('click', processAll);`
   - Delete `uploadReferenceBtn.addEventListener(...)` and
     `referenceFileInput.addEventListener(...)`.
   - Verify `uploadReferenceImage` (the JS function) stays — it's
     still called by the in-tuning-panel
     `#tuneReferenceImageBtn` flow from T-005-03/T-007-03.
     `tuneReferenceImageBtn` and `tuneReferenceFileInput` stay.

7. **`prepareForSceneBtn` originalLabel snapshot** (line 2296).
   No change required — the snapshot reads from the live DOM, so
   relabeling in HTML automatically flows through.

### `static/style.css`

Add (and keep tight — reuse `.toolbar-btn` rules):

```css
/* T-008-02: Advanced disclosure */
.toolbar-advanced {
    display: inline-block;
    position: relative;
}
.toolbar-advanced > summary {
    /* inherits .toolbar-btn — see HTML class */
    list-style: none;
    cursor: pointer;
}
.toolbar-advanced > summary::-webkit-details-marker {
    display: none;
}
.toolbar-advanced > summary::after {
    content: ' ▸';
    font-size: 9px;
    opacity: 0.7;
}
.toolbar-advanced[open] > summary::after {
    content: ' ▾';
}
.advanced-panel {
    position: absolute;
    top: calc(100% + 4px);
    left: 0;
    z-index: 5;
    display: flex;
    flex-direction: column;
    gap: 4px;
    padding: 8px;
    background: var(--panel-bg);
    border: 1px solid var(--panel-border);
    border-radius: 4px;
    box-shadow: 0 4px 16px rgba(0, 0, 0, 0.4);
    min-width: 220px;
}
.advanced-panel .toolbar-btn {
    text-align: left;
}
```

The popover is positioned absolutely so it floats over the preview
canvas without pushing the toolbar row taller.

The lod-toggle font-size and padding stay as-is — the new labels
fit within the existing button geometry, but the `.lod-toggle`
parent already inherits `flex-wrap` from `.preview-toolbar`, so
narrow viewports degrade gracefully.

## Files NOT touched

- `analytics.go` — no analytics changes.
- `docs/knowledge/analytics-schema.md` — no analytics changes.
- `settings.go` / `settings_test.go` — no setting renames; only
  display labels move.
- `main.go` — `/api/process-all` route stays even though the UI
  button is gone. Removing the route is server cleanup; out of
  scope.
- All Go test files — nothing test-relevant changes.
- The `lod-toggle` `data-lod` attribute *values*.
- The `analytics_session_id` and any other analytics plumbing.
- The `Profiles`, `Accepted`, presets, and gltfpack setting rows.

## Ordering of changes (the only ordering that matters)

1. Wire the new disclosure CSS first (so the visible result of
   the HTML change in step 2 has style applied).
2. Restructure HTML: add `<details>`, move technique buttons
   inside, remove `processAllBtn` / `uploadReferenceBtn`, rename
   labels.
3. Remove the JS references to the deleted elements (constants,
   listeners, `updatePreviewButtons` lines).
4. Update the build-action JS resting/running label strings.
5. Rename tuning section + field labels in HTML.
6. Run `node --check static/app.js`, `go build ./...`,
   `go test ./...`.
7. Write `review.md` with the rename log.

Steps 1-5 are independent enough to commit individually if
desired, but the most natural commit boundary is "all UI rename
+ disclosure" as one commit and "Process All removal" as a
second commit (it touches the most surfaces and is the one most
likely to need a follow-up revert).
