# Design — T-008-03: inline help and first-run hint

## Decisions

### D1. Help-text pattern: inline italic, reusing `.tooltip`

The codebase already ships an inline-italic-below-label pattern via
`.tooltip` (`style.css:758`) and uses it for four checkbox controls.
Reusing it gives us:

- Zero new CSS for the common case.
- Consistent visual treatment across help text the user already
  sees today.
- No hover dependency (a `?` tooltip on hover is invisible on
  touch and discoverable only by accident).

**Rejected: `?` tooltip on hover.** Forces a discovery action to
read help on a panel that is *already* trying to teach the user.
Also requires deleting four working `.tooltip` rows for
consistency, plus building a new tooltip primitive.

**Rejected: per-control `?` icons that expand on click.** Same
discovery problem and adds DOM weight. Not worth it for a final
polish ticket.

### D2. Help-text source of truth: `static/help_text.js`

A single ES module exporting a flat `HELP_TEXT` map keyed by
control id:

```js
export const HELP_TEXT = {
  tuneVolumetricLayers:    "Number of horizontal slices used to fake volume from any angle.",
  tuneVolumetricResolution:"Resolution of each baked slice. Higher = sharper, slower bake.",
  // ...
};
```

`app.js` imports it at module load and, in `init()`, walks every
`[data-help-id]` element and inserts a `<div class="tooltip">…</div>`
immediately after its closest `.setting-row > label` (or at the end
of the row for checkbox controls). This means `index.html` only
adds an attribute per control; the prose lives in one file.

**Rejected: inline `<div class="tooltip">` in `index.html` plus a
copy in `help_text.js`.** Two sources of truth. The AC explicitly
says "live in a single file so they're easy to edit / translate".
Two copies fail that test.

**Rejected: server-rendered help text.** This app has no template
engine; markup is static. Pulling help into a JSON endpoint would
add a fetch on a page that already has too many fetches in init.

### D3. First-run hint: structured HTML inside `#previewPlaceholder`

Replace the single line `"Select or drop a file to preview"` with
two states living in the same placeholder:

- A *first-run* block (`<div class="first-run-hint">`) shown when
  the file list is empty.
- A *fallback* line (`<div class="placeholder-fallback">`) shown
  when at least one file exists but none is selected (e.g., user
  deleted the selected one).

`updatePlaceholderState()`, called from `init()` after `loadFiles()`
and from `renderFileList()` after any add/delete, picks which to
show. Once `files.length > 0` at any point in the session, the
first-run block stays hidden permanently — guarded by a session
flag `firstRunHintDismissed` that flips to `true` on the first
non-empty render and never flips back. Reloading the page with an
empty workspace shows the hint again (correct: that user is
effectively starting over).

**Rejected: separate sibling element below the canvas.** Pulls
the hint outside the existing `hidePreview()`/`showPreview()`
toggle, requires its own positioning, and visually competes with
the canvas. The placeholder is already centered in the right
place.

**Rejected: localStorage `seenHint` flag.** Two failure modes:
the user clears storage and is told they're new again, or they
*are* new and the flag from a coworker's earlier session hides
the hint. The empty-workspace heuristic is more honest — if you
have files, you're not first-run.

**Rejected: persistent dismissable banner.** Too heavy. The hint
is a single paragraph; once a file is uploaded, the canvas
replaces the placeholder anyway.

### D4. Hint content

Four bullets matching the four moments in the AC, in order:

1. **Drop a `.glb` file** here or use Browse Files (top left).
2. Click **Prepare for scene** above the preview once it loads.
3. **Tune by eye** — adjust the right-hand panel if it doesn't
   look right.
4. Mark as **Accepted** when you're happy with the result.

Each is short. Bold the verb-noun. No icons, no animation.

### D5. Section-header `?` icons: out of scope

The AC marks them optional and the ticket body says "Anything more
is scope creep." `docs/` does not yet contain per-section help
pages; we would have to author them. Skip.

### D6. Help text scope: `#tuningSection` only (the AC boundary)

The other right-panel sections already have flag pills marking
them as advanced detail and four of them already carry `.tooltip`
rows. Adding inline help to every gltfpack control is out of
scope per the AC ("each tuning panel control"). The four existing
tooltips remain in `index.html` as-is; we do not migrate them
into `help_text.js` because they are not tuning controls and
moving them buys nothing.

## Behavior summary

| Trigger | Effect |
|---|---|
| Page loads with `files = []` | First-run hint visible in placeholder |
| Page loads with `files != []` | Fallback line visible; hint hidden |
| User uploads first file | Hint hidden permanently for the session via `firstRunHintDismissed = true` |
| User deletes the last file mid-session | Fallback line shows, NOT the hint (`firstRunHintDismissed` is sticky) |
| User reloads with empty workspace | Hint shows again (correct — new session, no files) |
| User hovers a tuning control | They see the inline italic help text below the label (already painted at init) |

## Risks

- **Help text accuracy.** Wrong help is worse than no help. The
  prose in `help_text.js` should be reviewed by whoever owns the
  bake pipeline. Mitigation: keep the descriptions terse and
  factual (what each control changes), avoid recommending values.

- **Walker breakage on dynamic controls.** The reference-image
  row and reclassify hint are mutated by JS. The walker runs
  once at init; any control that doesn't exist at init won't
  get help. Mitigation: every tuning control listed in the
  acceptance scope is statically present in `index.html` —
  visibility is toggled, but the element exists. Walker runs
  fine.

- **Initial-render flash.** If `init()` runs the walker after
  paint, the user briefly sees labels with no help. Mitigation:
  walker runs at the top of `init()`, before any awaits.
