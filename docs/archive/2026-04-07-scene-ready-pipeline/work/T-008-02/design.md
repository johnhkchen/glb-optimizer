# Design — T-008-02: advanced disclosure + label rename

## Goals

1. Hide every technique button behind an `Advanced` disclosure that
   is collapsed by default. The toolbar after this ticket shows
   `Prepare for scene` plus the disclosure trigger plus the
   non-action UI (preview toggle, lod-toggle, wireframe, progress,
   stress controls).
2. Rename labels that fail the "would a landscape designer know what
   this does?" test, in the toolbar AND in the right-panel tuning
   section. Use one verb across all build actions.
3. Decide the fate of `Process All` and the toolbar
   `Reference Image` button.

## Disclosure mechanism

### Options considered

**A. Native `<details>` / `<summary>`.** Zero JS, native
keyboard support, the open-state stays in the DOM so we can target
it with CSS. Downside: the default browser triangle marker has to
be hidden, and `<summary>` inside a flex row needs `display:
list-item` overrides on some browsers. Otherwise it slots cleanly
into the existing toolbar flex layout.

**B. A regular `<button>` plus a sibling `<div>` whose
`style.display` is toggled by JS.** More control over animation
and ARIA, but reinvents what `<details>` already does and adds
state we have to remember to reset on file switches. We do not
need animation here.

**C. A modal popover.** Heavier than the disclosure pattern, breaks
the toolbar's "everything in one strip" affordance, and the user
loses the ability to glance at an Advanced action while watching
the preview.

### Decision: A — `<details>` / `<summary>`.

Native, durable, no JS state to manage, the `open` attribute is
persistable to `localStorage` if a follow-up wants to remember the
last expanded state. Browser triangle is hidden via
`summary::-webkit-details-marker { display: none }` and the
equivalent `details > summary { list-style: none }`. We add our
own caret glyph in the summary text (`▾` when open, `▸` when
collapsed) via a `[open]` selector.

The summary becomes a single button-styled trigger reading
`Advanced ▸`. Inside the `<details>` body sits a small panel
containing the eight technique buttons.

## Where the disclosure lives in the toolbar flex flow

The current `.toolbar-actions` row is one flat flex container that
holds the primary button, all eight technique buttons, the bake
stale hint, and the prepare-progress block. After the rename:

```
[Prepare for scene] [Advanced ▾]                  [bake stale hint]
                    └── (when open) ──────────────────────────────┐
                        [Build LOD chain]                         │
                        [Build LOD chain (Blender remesh)]        │
                        [Build camera-facing impostor]            │
                        [Build volumetric dome slices]            │
                        [Build volumetric LOD chain]              │
                        [Build hybrid impostor]                   │
                        [Run lighting diagnostic]                 │
                                                                  ┘
[prepare progress · view in scene]
```

Constraint from research: `#prepareProgress` and `#bakeStaleHint`
must NOT live inside `<details>` because the user needs them visible
regardless. So the structure becomes:

```html
<div class="toolbar-actions">
  <button #prepareForSceneBtn>Prepare for scene</button>
  <details class="toolbar-advanced">
    <summary>Advanced</summary>
    <div class="advanced-panel">
      ...eight technique buttons...
    </div>
  </details>
  <span #bakeStaleHint>...</span>
</div>
<div #prepareProgress>...</div>
```

`#prepareProgress` already lives outside `.toolbar-actions` after
T-008-01, so no move is needed for it. The bake stale hint stays a
sibling of the `<details>`.

## Label decisions

### One verb across build actions

Choices: `Generate` (current), `Render`, `Build`, `Make`, `Create`.
`Generate` reads as "machine output," `Render` reads as
"engineer-speak," `Build` reads as "I am making the thing I want."
**Decision: Build.** The primary action stays `Prepare for scene`
because that's a higher-level verb (the orchestration of multiple
build steps).

### Toolbar technique buttons

| Old | New |
|---|---|
| `LODs (gltfpack)` | `Build LOD chain` |
| `LODs (Blender)` | `Build LOD chain (Blender remesh)` |
| `Billboard` | `Build camera-facing impostor` |
| `Volumetric` | `Build volumetric dome slices` |
| `Vol LODs` | `Build volumetric LOD chain` |
| `Production Asset` | `Build hybrid impostor` |
| `Test Lighting` | `Run lighting diagnostic` |
| `Reference Image` | *(removed — see below)* |

The `Render` verbs that the JS uses transiently while a build is
in flight (`generateBillboardBtn.textContent = 'Rendering...'`) get
unified to `Building…` so the running state matches the resting
verb.

### LOD-toggle inspector buttons

| `data-lod` | Old label | New label |
|---|---|---|
| `lod0..lod3` | `LOD0..LOD3` | `LOD0..LOD3` *(unchanged — short, technical, fine for an inspector)* |
| `billboard` | `Billboard` | `Camera-facing` |
| `volumetric` | `Volumetric` | `Dome slices` |
| `production` | `Production` | `Hybrid` |
| `vlod0` | `VL0` | `Vol high` |
| `vlod1` | `VL1` | `Vol med` |
| `vlod2` | `VL2` | `Vol low` |
| `vlod3` | `VL3` | `Vol billboard` |

Note: the AC literally proposes
`Volumetric (highest detail) / (medium) / (low) / (billboard)` for
the `vlod` group. Those labels are 22-25 characters wide, which
breaks the existing `lod-toggle` flex row badly (it currently uses
`font-size: 10px; padding: 4px 8px` precisely because there are 11
buttons). The shorter `Vol high / med / low / billboard` carries
the same meaning at 8-13 characters and is the practical
compromise. Logged in `review.md` as the canonical rename so
contributors searching for the AC text find the actual labels.

### Tuning section

`Tuning` → **`Asset tuning`** — the section name now reads as a
noun phrase a designer can parse without context.

Field renames within the section:

| Old | New |
|---|---|
| `Volumetric layers` | `Dome layer count` |
| `Volumetric resolution` | `Bake texture size` |
| `Dome height factor` | `Dome height` |
| `Bake exposure` | `Bake brightness` |
| `Ambient intensity` | `Ambient light` |
| `Hemisphere intensity` | `Sky light` |
| `Key light intensity` | `Sun strength` |
| `Bottom fill intensity` | `Underside fill` |
| `Env map intensity` | `Reflection strength` |
| `Alpha test` | `Cutout edge threshold` |
| `Slice distribution mode` | `Slice spacing strategy` |
| `Ground align bottom slice` | `Snap bottom slice to ground` |

`Lighting preset`, `Reclassify…`, `Reset to defaults`,
`Upload reference image` already read clearly and stay as-is.

The `Mesh Settings` / `Texture Settings` / `Output` sections stay
named as-is — they are not in the same "designer surface" because
the gltfpack flag pills next to each row already mark them as
power-user knobs. Renaming them would invalidate muscle memory for
no clarity gain.

### `Process All` decision

Removed. Rationale:

- `processAll` only runs gltfpack optimize across pending files;
  `Prepare for scene` runs the full pipeline against the selected
  file (and Stage 1 of that pipeline is the same gltfpack call).
- The S-008 single-asset tuning workflow has no batch step that
  benefits from optimizing 50 GLBs in one click without then
  tuning each one individually.
- Keeping it in Advanced would just push the confusion one click
  deeper. The simpler answer is to delete it.
- The route `/api/process-all` stays — it's a server endpoint, not
  a UI element, and removing it is server-side cleanup beyond
  ticket scope. It is reachable via curl/devtools if a future
  bulk-import script wants it.

The `#processAllBtn` element, its `processAll()` JS function, and
the `processAllBtn.disabled` updates in `updatePreviewButtons` are
removed. The left-panel `.panel-actions` becomes just
`Download All (.zip)` (which still appears only when at least one
file is `done`).

### `Reference Image` toolbar button decision

Removed. Per research: T-007-03 already absorbed the upload control
into the tuning panel (visible when the lighting preset is
`from-reference-image`). The toolbar button is the legacy duplicate
path that confuses users by uploading an image the bake then
ignores until the preset is also flipped. The label log will note
that the upload control now lives in the right-panel tuning section.

## What stays untouched

- `Original` / `Optimized` toggle (clear).
- `Wireframe` (clear, common 3D term).
- `Run scene` (the stress-test trigger; clear).
- The flag-pill rows in Mesh / Texture / Output sections.
- The `Profiles` and `Accepted` sections.
- All `data-lod` attribute *values*; only the visible labels change.
- All analytics event names.

## Risks

1. **Muscle memory.** Anyone in the project who clicks
   `Production Asset` or `Vol LODs` daily will have to relearn the
   labels. The label log in `review.md` is the cushion: a single
   place to grep for the old name and find the new one.
2. **`<details>` styling drift across browsers.** Mitigated by
   using both `list-style: none` (Firefox) and the
   `::-webkit-details-marker` rule (Safari/Chrome). Verified in the
   T-004-04 modal CSS that the project targets evergreen browsers.
3. **Flex wrap on the lod-toggle row** when the labels grow from
   `VL0` to `Vol high`. Mitigated by keeping the new labels short
   (≤12 chars) and accepting that the row may take two lines on
   narrow viewports — the toolbar already has `flex-wrap: wrap`
   from T-008-01.
4. **Hidden Blender button inside the disclosure.** When Blender is
   not available the `Build LOD chain (Blender remesh)` button is
   `style.display = 'none'`. That hide-state is set imperatively
   from `app.js:4443`. The disclosure must not interfere — the JS
   already targets the button by id, not by parent, so this stays
   working.
