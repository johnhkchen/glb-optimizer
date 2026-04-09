# Progress — T-008-03

## Done

- **Step 1.** Created `static/help_text.js`. ES module exporting
  `HELP_TEXT` keyed by control id; 16 entries covering every
  control in `#tuningSection`. Validated by importing the module
  via `node --input-type=module` — parses and reports 16 keys.

- **Step 2.** Added `data-help-id="<controlId>"` to all 16
  `setting-row` divs in `#tuningSection` (`static/index.html`
  lines 245–352). Each attribute matches a key in
  `HELP_TEXT`. The `referenceImageRow` already had an `id`; the
  attribute was added alongside it.

- **Step 3.** Replaced the body of `#previewPlaceholder` with
  the structured `firstRunHint` (visible by default) +
  `placeholderFallback` (hidden by default) blocks. The default
  visibility means the user sees the hint immediately on first
  paint, even before `app.js` runs.

- **Step 4.** Added `.first-run-hint`, its child rules, and
  `.placeholder-fallback` to `static/style.css` directly after
  the existing `.preview-placeholder` rule. No edits to existing
  rules. The hint inherits the centering of `.preview-placeholder`.

- **Step 5.** Wired `paintHelpText()` and
  `updatePlaceholderState()` into `static/app.js`:
  - Imported `HELP_TEXT` from `./help_text.js` next to the other
    module imports.
  - Added `let firstRunHintDismissed = false;` to the state block.
  - Added the two helper functions next to `showPreview()` /
    `hidePreview()`.
  - Called `paintHelpText()` and `updatePlaceholderState()` from
    the existing top-level init block right after `initThreeJS()`.
  - Hooked `updatePlaceholderState()` into the tail of
    `renderFileList()` so add/delete keep the hint in sync.

- **Step 6.** Self-checked against the AC checklist:
  - `paintHelpText` is idempotent — guarded by a
    `data-help-paint` marker so re-running is a no-op.
  - `firstRunHintDismissed` is sticky once `files.length > 0`,
    so deleting the last file mid-session does not bring the
    hint back.
  - `updatePlaceholderState()` no-ops gracefully if the
    elements are missing (defensive but expected to be present).
  - Manual smoke not run in this autonomous pass — flagged in
    `review.md` as the only outstanding verification.

## Deviations from plan

None of substance. The "two sub-edits" of Step 5 became three
small edits in three different parts of `app.js` (import, state
declaration, function definition + caller insertion), but the
shape is identical to plan.md.

I added an idempotency guard (`data-help-paint` attribute) to
`paintHelpText()` that wasn't strictly required by the plan.
Cost is one DOM attribute per row; benefit is that any future
caller that re-runs the walker after a hot reload or HMR
won't double-paint.

## Not done

- Manual browser smoke test against the five AC scenarios
  (empty workspace → hint, upload → hint hides, etc.). This
  was Step 6 in the plan; documented in `review.md` as the
  outstanding verification step.
