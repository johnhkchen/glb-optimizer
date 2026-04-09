# Plan — T-002-03: tuning-panel-ui-skeleton

## Step Sequence (atomic, in order)

### Step 1 — CSS: dirty dot
Append the `.dirty-dot` and `.dirty-dot.dirty` rules to
`static/style.css` after the `.tooltip` rule (~line 644).

**Verify**: `git diff` shows additive lines only. No existing rules
modified.

### Step 2 — HTML: Tuning section
Insert the new `<div class="settings-section" id="tuningSection">`
block in `static/index.html` after the closing `</div>` of the
"Output" section (line ~217), before the closing `</div>` of
`.panel-right`.

**Verify**:
- File parses (open in browser; page renders without JS errors).
- All eleven controls visible in the right panel.
- Section scrolls naturally with the rest of the panel.
- No layout regression on the existing Mesh/Texture/Output sections.

### Step 3 — JS: refactor `applyDefaults` to use `makeDefaults`
Pure refactor in `static/app.js`. No behavior change.

**Verify**: page still loads; bake functions still produce identical
output (no manual bake required — this is a pure rename + extract).

### Step 4 — JS: change debounce 500 → 300
One numeric literal change in `saveSettings`.

**Verify**: `grep` confirms exactly one occurrence updated.

### Step 5 — JS: add `TUNING_SPEC`, `populateTuningUI`,
            `wireTuningUI`, `updateTuningDirty`
Insert the new "Tuning UI (T-002-03)" block after `applyDefaults()`
(~line 128) and before `getSettings()`.

**Verify**: parses (no syntax error), `wireTuningUI` is the only new
identifier consumed by step 6.

### Step 6 — JS: hook `selectFile`
Add a single `populateTuningUI();` call after the existing
`await loadSettings(id);` line in `selectFile`.

**Verify**: file selection still works; selecting a file with no
saved settings shows defaults; selecting a file with saved settings
populates the controls from disk.

### Step 7 — JS: module init
At the end of the event-listener section, add:

```js
applyDefaults();
wireTuningUI();
populateTuningUI();
```

**Verify**: page loads → controls show defaults → dirty dot is hidden
→ moving any slider lights up the dirty dot → 300 ms after release,
network panel shows a `PUT /api/settings/:id` (this only fires when a
file is selected, since `wireTuningUI` short-circuits without
`selectedFileId`).

## Verification Strategy

### Static checks (after every step)
- `node --check static/app.js` (or open in browser console; the file
  is an ES module so it must be served)
- `git diff --stat` to confirm only the planned files changed

### Build checks
- `go build ./...` — should be a no-op since no Go files change, but
  worth running once to confirm the binary still serves the new
  static assets.
- `go test ./...` — same; no Go tests should regress, none added.

### Manual smoke test (browser)
1. Start the server (`go run .` or whatever the existing run command is).
2. Open `http://localhost:<port>`.
3. Drop a `.glb` file into the drop zone.
4. Click the file in the file list.
5. **Verify**: Tuning section's controls populate from the file's
   `currentSettings`. Dirty dot is hidden (defaults).
6. Drag the "Bake exposure" slider to 1.5.
7. **Verify**: Live readout updates immediately. Dirty dot lights up.
   ~300 ms after the last drag, Network tab shows
   `PUT /api/settings/{id}` with `bake_exposure: 1.5`.
8. Click the existing "Production Asset" button.
9. **Verify**: bake completes. Visual brightness is higher than the
   default bake (proves `currentSettings.bake_exposure` is consumed).
10. Click "Reset to defaults".
11. **Verify**: All controls snap back to schema defaults. Dirty dot
    disappears. Network tab shows a PUT with the default values.
12. Drop a second `.glb`. Click it.
13. **Verify**: Tuning controls update to the new file's settings
    (which are also defaults, since it's never been tuned).
14. Switch back to the first file.
15. **Verify**: Tuning controls are back to defaults (from step 11),
    not the 1.5 exposure from step 6 (which was overwritten by the
    reset).
16. **Visual check**: existing Mesh/Texture/Output sections are
    unchanged in style and behavior. Existing presets row still works.
17. **Resize/scroll check**: panel scrolls to reveal the Tuning section.

## Tests Added
None. The repo has zero JS test infrastructure (T-002-02 review §coverage
gaps). Adding it for one ticket would dwarf the change. Listed as a
follow-up in `review.md`.

## Tests Not Added (and why not)
- **No HTTP-handler test for `handleSettings`** — backend untouched.
- **No browser/headless smoke test** — no infrastructure to host one.
- **No visual regression** — same.

## Commit Strategy

Five commits (steps 1+2 grouped, then 3, 4, 5+6+7 grouped):

1. `T-002-03: add Tuning section markup and dirty-dot CSS`
   (HTML + CSS)
2. `T-002-03: extract makeDefaults() helper`
   (pure refactor)
3. `T-002-03: drop saveSettings debounce 500 → 300 ms`
   (one-liner)
4. `T-002-03: wire tuning controls to currentSettings`
   (TUNING_SPEC, populate/wire/dirty + selectFile hook + init)

If any step fails verification, roll back at that step and re-plan
without leaving partial state in the working tree.

## Risks / Mitigations

| Risk | Likelihood | Mitigation |
|------|-----------|------------|
| Hand-sync drift between `makeDefaults()` JS literal and `DefaultSettings()` Go literal | Medium | Already a known limitation; flagged again in review.md. Net change: one new consumer (the dirty compare) but no new copies of the literal. |
| Layout overflow in right panel on small screens | Low | Right panel already scrolls; section uses the same idiom. |
| `populateTuningUI` called before `currentSettings` is set | Low | Function short-circuits on `!currentSettings`. Init path calls `applyDefaults()` first. |
| Dirty dot stays lit after a successful Reset | Low | Reset calls `applyDefaults()` *then* `populateTuningUI()`, which calls `updateTuningDirty()` post-population. |
| User drags slider with no file selected → spurious PUT to undefined id | Low | Listener short-circuits on `!selectedFileId`. |
| 300 ms debounce too aggressive — every bake during drag | Mitigated | Debounce is on the PUT, not on the in-memory mutation. UI feels live; backend writes catch up. |
| `parseInt`/`parseFloat` mis-parsing select values | Low | `lighting_preset` uses identity parser; `volumetric_resolution` uses `parseInt` which handles `"512"` cleanly. |

## Out of Scope (reaffirmed)
- Live re-bake on slider drag.
- Tooltips, presets beyond `default`, named profiles, undo.
- Backend defaults endpoint (filed in review).
- JS test infra.
