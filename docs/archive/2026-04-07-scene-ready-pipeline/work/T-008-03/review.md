# Review — T-008-03: inline help and first-run hint

Final ticket of E-001 / S-008. Adds inline help to the asset-tuning
panel and a first-run "Get started" hint inside the preview
placeholder. Closes the workflow-clarity arc that T-008-01
(primary action) and T-008-02 (label cleanup) started.

## Files changed

| File | Change |
|---|---|
| `static/help_text.js` | **created** — `HELP_TEXT` map, 16 entries, one per tuning control |
| `static/index.html` | added `data-help-id` to all 16 tuning rows; replaced `#previewPlaceholder` body with `#firstRunHint` + `#placeholderFallback` |
| `static/style.css` | added `.first-run-hint`, child rules, `.placeholder-fallback`; no edits to existing rules |
| `static/app.js` | imported `HELP_TEXT`; added `firstRunHintDismissed` flag, `paintHelpText()`, `updatePlaceholderState()`; called from init + `renderFileList()` |

No deletions. No file moves. No backend touched. No analytics
events added or changed.

## How it satisfies the AC

| AC bullet | Implementation |
|---|---|
| One-line help per tuning control, single pattern | Inline italic via the existing `.tooltip` class — same treatment already used by four checkbox rows in `Mesh Settings`/`Output`. Painted by `paintHelpText()` from `HELP_TEXT`. |
| First-run hint when no assets are loaded | `#firstRunHint` block inside `#previewPlaceholder`, default-visible in HTML, hidden by `updatePlaceholderState()` when `files.length > 0`. |
| Hint disappears once any asset has been uploaded | `firstRunHintDismissed` is set to `true` permanently the moment `files.length > 0`. `renderFileList()` calls `updatePlaceholderState()` after every add/delete. |
| Help text in a single editable file | `static/help_text.js`. `index.html` only carries `data-help-id` attributes; the prose lives in one place. |
| Optional `?` icons next to section headers | Skipped per the ticket body's "anything more is scope creep". |
| Manual verification | Outstanding — not exercised in this autonomous pass; see "Outstanding" below. |

## Test coverage

This is a presentation-only change. There is no JS test harness in
the repo and no Go code was touched, so no automated tests were
added or run.

What I did verify:

- `static/help_text.js` parses as an ES module and exports
  `HELP_TEXT` with 16 keys (`node --input-type=module -e
  "import('./static/help_text.js').then(...)"`).
- The `data-help-id` keys in `index.html` match the keys in
  `HELP_TEXT` (audited by hand against the diff).
- `paintHelpText()` is idempotent — guarded by a
  `data-help-paint` marker attribute on the inserted tooltip.
- `updatePlaceholderState()` is null-safe — bails out if either
  child element is missing.
- No new DOM ids collide with existing ones.

What is *not* tested:

- Visual rendering of the hint and tooltips in a real browser.
- Behavior across the five manual AC scenarios (empty workspace,
  upload, hover, reload, delete-last-file).
- Help-text accuracy. The strings describe what each control
  changes in user-visible terms, but they were written without
  pipeline-engineer review and may need a content pass before
  ship. They are co-located in one file specifically to make
  that easy.

## Risks and open concerns

1. **Help-text accuracy.** Highest-impact concern. Wrong help is
   worse than no help. A human review of `static/help_text.js`
   by whoever owns the bake pipeline would be cheap insurance.

2. **`tuneReferenceImageBtn` row is conditionally hidden.** The
   row is shown only when `lighting_preset === 'from-reference-image'`.
   `paintHelpText()` runs once at startup against the row whether
   or not it is visible — the tooltip is painted into a hidden
   row and becomes visible together with the row when the user
   selects the matching preset. This is correct, but worth
   noting because it differs from the other rows.

3. **`tuneReclassifyBtn` row also contains `#tuneShapeCategoryHint`,**
   a span the classifier writes shape-category text into. The
   inline help tooltip is appended *after* it. Visually this
   means: button — shape hint — italic help. Acceptable, and
   probably even useful, but a future stylist might want to
   reorder.

4. **`firstRunHintDismissed` does not persist across reloads.**
   This is intentional — the file list is the source of truth.
   On reload with files in the workspace, the fallback line
   shows because `files.length > 0`. On reload with no files,
   the hint shows again, which is correct: that user is
   effectively starting over. Documented in `design.md` D3.

5. **Idempotency guard adds an attribute to the DOM.** Each
   tooltip carries `data-help-paint="1"`. Trivial cost, but
   noted in case some future selector trips on it.

## Outstanding before merge

- **Manual smoke test against the five AC scenarios.** The plan
  enumerates them in Step 6; they need a real browser session.
- **Content review of `static/help_text.js`.** One short pass
  by someone who knows the bake pipeline.

## Out of scope (held to ticket's "First-Pass Scope")

- Onboarding tour, multi-language, video walkthroughs, a11y
  audit, keyboard hint overlay (all out per ticket).
- Section-header `?` icons linking to `docs/` (optional per AC,
  skipped per ticket body).
- Migrating the four existing `.tooltip` rows in
  `Mesh Settings` / `Output` into `help_text.js` (not tuning
  controls — outside the AC's scope boundary).
- Help text for non-tuning right-panel controls (presets,
  profiles, accept). The tuning section is the help-deficient
  zone the ticket targets.

## End of epic

This is the last ticket in E-001. With T-008-01 (primary action),
T-008-02 (clutter hidden + labels renamed), and T-008-03 (inline
help + first-run hint), the workflow transformation from
"10 confusing clicks to remember" → "upload, click Prepare for
scene, tune by eye, accept" is in place end-to-end. The
remaining work is the manual smoke + content-review pass noted
above.
