# Research — T-008-03: inline help and first-run hint

Final ticket of E-001 / S-008. T-008-01 added a primary action,
T-008-02 hid the technique-button clutter and renamed labels. This
pass adds the small explanatory touches that turn a now-uncluttered
UI into a self-explanatory one for a first-time user.

## What already exists

### Help-text pattern is already in the codebase

`static/style.css:758` defines a `.tooltip` class:

```
.tooltip {
    font-size: 11px;
    color: var(--text-muted);
    margin-top: 4px;
    font-style: italic;
}
```

It is used three times today (`static/index.html:155, 164, 173, 238`)
under `Mesh Settings` and `Output` to gloss `aggressiveSimplify`,
`permissiveSimplify`, `lockBorders`, and `floatPositions`. The
visual treatment — small italic muted text directly below the label —
is exactly the "inline italic text below the label" option offered
by the ticket's AC.

**This means the pattern is already established.** Picking the
inline option means reusing `.tooltip`; picking the `?`-tooltip
option would require adding a new class, deleting the four existing
`.tooltip` rows for consistency, and rewriting the help-on-hover
behavior. Inline is the lower-cost path and matches the AC's
"pick one pattern and use it consistently" requirement.

### The Tuning section is the help-deficient zone

`#tuningSection` (`static/index.html:242-352`) carries 14
controls (sliders, selects, buttons). None have inline help today.
Field labels were already plain-English-ified by T-008-02 (review:
`Volumetric layers` → `Dome layer count`, `Bake exposure` →
`Bake brightness`, etc.) but the labels still don't tell a first-
time user *what each one does to the picture*.

The other sections (`Mesh Settings`, `Texture Settings`, `Output`)
carry gltfpack-flag jargon (`-si`, `-cc`, `-tc`). Per
T-008-02 research, those sections are "advanced detail" and the
flag pills already mark them as such. The ticket's AC scopes help
text to "tuning panel controls", so the help-text work belongs in
`#tuningSection` — though there is no harm in filling out the few
remaining gaps in the other sections opportunistically.

### The preview placeholder is the only hint surface

`#previewPlaceholder` (`static/index.html:111-113`) is a single-line
muted message: `"Select or drop a file to preview"`. CSS at
`style.css:615-622`:

```
.preview-placeholder {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    color: var(--text-muted);
    font-size: 14px;
}
```

The placeholder is shown by `hidePreview()` (`app.js:3983`) and
hidden by `showPreview()` (`app.js:3976`). `showPreview()` is
called from `selectFile()` (`app.js:3990`), which fires whenever
the user clicks a file in the list. Once any file has been
uploaded and selected, the placeholder will not be shown again
in the same session. There is no separate "no files yet" state
distinct from "no file selected" — the same placeholder serves
both — but the practical effect is identical: the hint must
disappear once any asset has been uploaded.

There is no `firstRun` flag, no localStorage key for hint
visibility, and nothing reads the upload count for hint purposes.

## Where labels are referenced from JS

The tuning controls are read by id in `app.js` (mostly via the
`syncCurrentSettings*` family) but their *labels* are static HTML.
Adding `<div class="tooltip">` rows below each `<label>` requires
no JS changes — there is no JS code that walks the labels.

For the first-run hint, the only JS touch point is whatever
populates the placeholder. Today the placeholder is plain text in
HTML. We can either:

1. Replace the text with structured HTML in `index.html` and let
   it be the *default* state. `hidePreview()`/`showPreview()`
   already toggle `display`, so the structured content lives or
   dies with the placeholder element.
2. Build the hint in JS at startup.

Option 1 is the lower-cost path because the hint content is static.

## Constraints

1. **The placeholder is reshown on `hidePreview()`.** This is
   currently called only from one place (`app.js:3983`). The hint
   should not reappear after the user has uploaded a file. The
   ticket says "disappears once any asset has been uploaded *in
   the current session*" — so we need a session-scoped guard that
   suppresses the hint after the first upload, not based on
   `selectedFileId === null`.

2. **`/api/files` may already return files on page load** (assets
   from a prior session live in the workspace). The hint should
   only fire when the file list is empty *at load time*. `init()`
   in `app.js` calls `loadFiles()` which populates `files`. If
   `files.length > 0` at the end of init, the hint should already
   be suppressed.

3. **Help text must live in a single file** (`static/help_text.js`
   per AC). It should be a plain ES module exporting an id→string
   map. `index.html` cannot import a module synchronously *into*
   markup, so the help-text bindings must be applied by `app.js`
   at startup, walking `[data-help-id]` attributes and injecting
   `.tooltip` divs. Or — simpler — the help text can be inlined
   directly in `index.html` and `static/help_text.js` can be the
   *source of truth* that we copy from when editing. The AC says
   "live in a single file so they're easy to edit / translate
   later" — this argues for one source of truth, not two. So the
   JS-injection path is mandatory if we want the AC to mean what
   it says.

4. **No analytics impact.** Help text is display only.

5. **`?` icon next to section headers is optional.** Out of scope
   unless cheap. The ticket's body says "Anything more is scope
   creep."

## Open questions surfaced by research

- **Should the help text cover the non-tuning sections too?** AC
  scopes to "tuning panel controls". Mesh/Texture/Output already
  have a few `.tooltip` rows but most controls there have none.
  Decision: the AC is the boundary. Stretch only if cheap.

- **Where does "Get started" hint live structurally?** Inside the
  same `#previewPlaceholder` div, or as a sibling that
  `showPreview()`/`hidePreview()` also toggles? Decision deferred
  to Design.

- **Should the hint persist across reloads if no files exist?**
  Yes — that is the natural behavior of "show hint when file list
  is empty". No localStorage needed; the file list is the source
  of truth.
