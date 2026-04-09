# Structure — T-008-03: inline help and first-run hint

## Files

### Created

#### `static/help_text.js` (new)

ES module. Single export `HELP_TEXT`, a flat object keyed by the
DOM id of the control it describes. Each value is a one-line
plain-English description of what the control changes.

```js
// One-line help strings shown below each tuning control. Edit
// here; app.js paints these into the DOM at startup by walking
// [data-help-id] attributes. Keep each entry to one sentence.
export const HELP_TEXT = {
  tuneVolumetricLayers:     "...",
  tuneVolumetricResolution: "...",
  tuneDomeHeightFactor:     "...",
  tuneBakeExposure:         "...",
  tuneAmbientIntensity:     "...",
  tuneHemisphereIntensity:  "...",
  tuneKeyLightIntensity:    "...",
  tuneBottomFillIntensity:  "...",
  tuneEnvMapIntensity:      "...",
  tuneAlphaTest:            "...",
  tuneLightingPreset:       "...",
  tuneSliceDistributionMode:"...",
  tuneGroundAlign:          "...",
  tuneReferenceImageBtn:    "...",
  tuneReclassifyBtn:        "...",
  tuneResetBtn:             "...",
};
```

Public interface: one named export, `HELP_TEXT`. No functions,
no side effects, no DOM access. Trivially translatable later by
swapping the file or wrapping the values in an i18n call.

### Modified

#### `static/index.html`

Two surgical changes:

1. **Add `data-help-id="<controlId>"` to each tuning row's
   container `<div class="setting-row">`** in `#tuningSection`
   (lines 242–352). One attribute per row. The attribute names
   the *control* the row contains, which is also the key used
   by `app.js` to look up the help string. No new elements; the
   attribute is the only addition.

   Sixteen rows total — one per entry in `HELP_TEXT`.

2. **Replace the body of `#previewPlaceholder`** (line 111-113)
   with two child blocks:

   ```html
   <div class="preview-placeholder" id="previewPlaceholder">
       <div class="first-run-hint" id="firstRunHint">
           <h2>Get started</h2>
           <ol>
               <li><strong>Drop a .glb file</strong> on the left, or click <strong>Browse Files</strong>.</li>
               <li>Click <strong>Prepare for scene</strong> above the preview once it loads.</li>
               <li><strong>Tune by eye</strong> from the right-hand panel if it doesn't look right.</li>
               <li>Mark it <strong>Accepted</strong> when you're happy with the result.</li>
           </ol>
       </div>
       <div class="placeholder-fallback" id="placeholderFallback" style="display:none">
           Select or drop a file to preview
       </div>
   </div>
   ```

   The two children are mutually exclusive; `app.js` toggles
   between them via `updatePlaceholderState()`.

3. **Import `help_text.js`** by either letting `app.js` import
   it directly (preferred — `app.js` is already a `<script
   type="module">`) or adding a separate `<script
   type="module">` tag. Preferred: app.js import. No
   `index.html` change for this point.

#### `static/style.css`

Add a small block at the end of the file (or near
`.preview-placeholder` at line 615) for the new hint elements:

```css
.first-run-hint {
    max-width: 360px;
    color: var(--text-muted);
    font-size: 13px;
    line-height: 1.6;
}

.first-run-hint h2 {
    font-size: 14px;
    color: var(--text);
    margin-bottom: 10px;
    font-weight: 600;
}

.first-run-hint ol {
    padding-left: 20px;
}

.first-run-hint li {
    margin-bottom: 6px;
}

.first-run-hint strong {
    color: var(--text);
    font-weight: 600;
}

.placeholder-fallback {
    color: var(--text-muted);
    font-size: 14px;
}
```

The existing `.preview-placeholder` rule (`display: flex;
align-items: center; justify-content: center`) already centers
the children; the hint inherits that.

The existing `.tooltip` rule (style.css:758) is reused as-is.

#### `static/app.js`

Three additions:

1. **Import** at the top alongside other module imports:

   ```js
   import { HELP_TEXT } from './help_text.js';
   ```

2. **`paintHelpText()`** — pure function called once from `init()`.
   Walks `document.querySelectorAll('[data-help-id]')`, looks up
   the help string in `HELP_TEXT`, and appends a
   `<div class="tooltip">` child to each row. Skips entries with
   no matching string (defensive but not strictly required —
   structure.md is the source of truth for the id list).

3. **First-run hint state.** A module-scope `let firstRunHintDismissed = false;`
   plus an `updatePlaceholderState()` function:

   ```js
   function updatePlaceholderState() {
       const hintEl = document.getElementById('firstRunHint');
       const fallbackEl = document.getElementById('placeholderFallback');
       if (!hintEl || !fallbackEl) return;
       if (files.length > 0) firstRunHintDismissed = true;
       const showHint = !firstRunHintDismissed && files.length === 0;
       hintEl.style.display = showHint ? 'block' : 'none';
       fallbackEl.style.display = showHint ? 'none' : 'block';
   }
   ```

   Called from:
   - The end of `init()` after `loadFiles()` resolves.
   - The end of `renderFileList()` (so add/delete update it).

   No call from `selectFile()`/`hidePreview()`/`showPreview()` is
   needed — the inner toggle reflects file-list state, not
   selection state. The `display:none/flex` toggle on
   `#previewPlaceholder` itself stays untouched.

### Deleted

None.

## Ordering

The four artifacts are independent — `help_text.js` and the
`index.html` `data-help-id` attributes can be added in any
order, and the placeholder + CSS changes are local. The walker
must run after the DOM is parsed and before the user can
interact, so the import + walker call go at the existing
top of `init()`.

## Public interface changes

- New module: `static/help_text.js` exporting `HELP_TEXT`. No
  one outside `app.js` consumes this.
- New DOM ids: `firstRunHint`, `placeholderFallback`. No
  external consumers; analytics is unaffected.
- New DOM attribute: `data-help-id` on tuning rows. No
  external consumers; analytics is unaffected.

## Out of scope (held to AC)

- Section-header `?` icons.
- Migrating the four existing `.tooltip` rows in
  `Mesh Settings` / `Output` into `help_text.js`.
- Help text for non-tuning controls.
- localStorage persistence of hint visibility.
- A11y audit / aria-describedby linking the tooltip to the
  control.
