# Plan — T-008-03: inline help and first-run hint

## Steps

### Step 1 — Create `static/help_text.js`

Author the `HELP_TEXT` map with the 16 keys identified in
structure.md. Each value is one sentence describing what the
control changes in user-visible terms (avoid bake-pipeline
jargon — prefer "how it looks" over "what it sets").

Verify: file parses as ES module (`node -e "import('./static/help_text.js').then(m => console.log(Object.keys(m.HELP_TEXT).length))"`
or just open the page).

Commit: `T-008-03: help_text.js with tuning control descriptions`

### Step 2 — Add `data-help-id` to tuning rows in `index.html`

Edit `static/index.html` lines 242–352. For each `<div
class="setting-row">` inside `#tuningSection`, add a
`data-help-id="<id>"` attribute where `<id>` is the id of the
control inside that row. Sixteen rows.

Verify: open the page, view source, check that every tuning row
has the attribute and matches the `HELP_TEXT` keys.

Commit: `T-008-03: tag tuning rows with data-help-id`

### Step 3 — Replace placeholder body in `index.html`

Replace the body of `#previewPlaceholder` (line 111-113) with
the structured `firstRunHint` + `placeholderFallback` markup
from structure.md. Hint visible by default; fallback hidden by
default. Final state is corrected by `updatePlaceholderState()`
at runtime.

Verify: page loads, hint visible immediately even before any JS
runs (since it is the default state). Once a file exists, the
hint should hide after `init()` completes.

Commit: `T-008-03: first-run hint markup in preview placeholder`

### Step 4 — Add CSS for hint elements in `style.css`

Append the `.first-run-hint` and `.placeholder-fallback` rules
from structure.md. Reuse `.preview-placeholder`'s flex centering.
No edits to existing rules.

Verify: hint renders as a centered ~360px-wide ordered list with
muted text, bold verbs in `--text` color. Matches the dark theme.

Commit: `T-008-03: hint CSS`

### Step 5 — Wire `paintHelpText()` and `updatePlaceholderState()` in `app.js`

Three sub-edits to `static/app.js`:

1. Import `HELP_TEXT` from `./help_text.js` near the other top-of-file
   imports / module init.
2. Add `paintHelpText()` and `updatePlaceholderState()` functions
   (and a `let firstRunHintDismissed = false;` module-scope flag)
   somewhere near the existing UI helpers (e.g., next to
   `showPreview` / `hidePreview`).
3. Call `paintHelpText()` once near the top of `init()`.
4. Call `updatePlaceholderState()` at the end of `init()` after
   `loadFiles()` resolves AND at the end of `renderFileList()`.

Verify: hover/inspect a tuning row — a `<div class="tooltip">…</div>`
sits below the label. Refresh with empty workspace — hint shows.
Refresh with files in workspace — fallback shows. Upload one file
— hint hides immediately.

Commit: `T-008-03: paint help text + first-run hint state`

### Step 6 — Manual smoke test

Per ticket AC:

1. Open the app fresh with an empty workspace → see the hint.
2. Upload a `.glb` → hint disappears, normal preview placeholder
   logic resumes.
3. Hover (or just look at) a tuning control → see the inline
   help text below the label.
4. Reload after upload → fallback line shows (not hint).
5. Delete the file → fallback line shows (not hint), because
   `firstRunHintDismissed` is sticky once set in this session.

If any step fails, document the divergence in `progress.md`
before fixing.

Commit: not required if no fixes needed; otherwise per fix.

## Testing strategy

This is a presentation-only ticket. There are no Go test files
to write, no analytics events to verify, and no existing JS
test harness. Verification is manual against the AC, plus
self-review of the diff.

What we don't test:
- The text accuracy of `help_text.js` strings — that's a content
  review item, not a unit test.
- The `firstRunHintDismissed` flag across browser reloads — it
  resets per page load by design, and the file-list state takes
  over after that.

What we *would* test if a JS test harness existed:
- `paintHelpText()` is idempotent (calling twice doesn't double-paint).
- `updatePlaceholderState()` correctly toggles based on
  `files.length` and the dismissed flag.

These would be small enough to add later in a single test file
once a harness exists; not justified to introduce one for this
ticket.

## Verification checklist

- [ ] `help_text.js` exports `HELP_TEXT` with 16 entries.
- [ ] Every `[data-help-id]` in `index.html` has a matching key.
- [ ] Every tuning row in `#tuningSection` has a `data-help-id`.
- [ ] `<div class="tooltip">` appears below each tuning row label
      after page load.
- [ ] Empty workspace → hint visible.
- [ ] Non-empty workspace → fallback line visible.
- [ ] First upload in session → hint hides and stays hidden.
- [ ] No console errors.
- [ ] No analytics events fire as a side effect of these changes.
