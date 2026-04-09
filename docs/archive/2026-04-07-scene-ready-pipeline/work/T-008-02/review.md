# Review — T-008-02: advanced disclosure + label rename

## What changed

The toolbar now shows two actions: `Prepare for scene` and an
`Advanced ▸` disclosure. Every direct technique button lives inside
the disclosure, which is collapsed by default. The
`Reference Image` toolbar button and the left-panel `Process All`
button were both removed — both had been superseded by other
flows. Toolbar / lod-toggle / tuning-section labels were rewritten
to read more clearly to a landscape designer who has not seen the
app before.

No backend, no analytics, no settings, and no test changes — this
is a presentation-layer pass.

## Files modified

- **`static/index.html`**
  - `.toolbar-actions`: replaced the flat list of nine technique
    buttons with `#prepareForSceneBtn` plus a
    `<details class="toolbar-advanced">` disclosure containing the
    seven build buttons. `#bakeStaleHint` and `#prepareProgress`
    stay outside the disclosure.
  - Removed `#uploadReferenceBtn`. Kept the hidden
    `#referenceFileInput` (still reused by the in-tuning-panel
    reference button from T-007-03).
  - `.lod-toggle`: relabelled the seven non-`lodN` inspector
    buttons. `data-lod` attribute values unchanged.
  - `.panel-actions`: removed `#processAllBtn`.
  - `#tuningSection`: header `<h3>Tuning ...` → `Asset tuning
    ...`; twelve field labels rewritten.

- **`static/style.css`**
  - Added `.toolbar-advanced` and `.advanced-panel` rules
    (vendor-prefixed `::-webkit-details-marker` reset and a
    floating popover positioned absolutely below the summary).

- **`static/app.js`**
  - Removed the `processAllBtn` const, the `processAll()`
    function, the `processAllBtn.disabled` line in
    `renderFileList`, and the listener wiring.
  - Removed the `uploadReferenceBtn` const, its click listener,
    and the `updatePreviewButtons` block that toggled its
    label/title.
  - Updated the resting and running label strings inside
    `generateLODs`, `generateBlenderLODs`, `generateBillboard`,
    `generateVolumetric`, `generateVolumetricLODs`, and
    `generateProductionAsset` to match the new toolbar labels and
    the unified `Building…` running verb.
  - Left `prepareForScene`'s label snapshot alone — it reads from
    the live DOM, so the HTML rename flows through automatically.

## Files NOT touched

`analytics.go`, `settings.go`, `main.go`, `*_test.go`,
`docs/knowledge/*` — all unchanged. The `/api/process-all` route
stays mounted server-side; it is now reachable only via devtools or
curl.

## Label rename log (canonical reference)

Future contributors searching for an old name should grep this
table.

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
| `Reference Image` | *(removed — use the in-tuning-panel "Upload reference image" button after picking the `From Reference Image` lighting preset, T-007-03)* |

The four running-state strings — `Generating...`, `Rendering...`,
`Remeshing...` — are all now `Building…`.

### LOD-toggle inspector buttons (preview area)

| `data-lod` | Old label | New label |
|---|---|---|
| `lod0..lod3` | `LOD0..LOD3` | *(unchanged)* |
| `billboard` | `Billboard` | `Camera-facing` |
| `volumetric` | `Volumetric` | `Dome slices` |
| `production` | `Production` | `Hybrid` |
| `vlod0` | `VL0` | `Vol high` |
| `vlod1` | `VL1` | `Vol med` |
| `vlod2` | `VL2` | `Vol low` |
| `vlod3` | `VL3` | `Vol billboard` |

The AC literally proposes `Volumetric (highest detail)` /
`(medium)` / `(low)` / `(billboard)` for the `vlod` group. Those
labels are 22-25 characters wide each, which would have wrecked the
existing 11-button inspector row layout. The shorter `Vol …` form
above carries the same information at 8-13 characters and is the
practical compromise. Logged here so anyone hunting the AC text
finds the actual labels.

### Tuning panel

Section header: `Tuning` → `Asset tuning`.

| Old field label | New field label |
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
`Upload reference image` already read clearly and were left as-is.
The `Mesh Settings`, `Texture Settings`, and `Output` sections —
which carry gltfpack-flag jargon (`-si`, `-cc`, `-tc`, …) — were
deliberately left alone: the flag pills already mark them as
power-user surface, and renaming them costs muscle memory for no
clarity gain.

### Removed surfaces

| What | Where | Why |
|---|---|---|
| `Process All` button | `.panel-actions` (left panel) | Only ran the gltfpack stage on pending files; superseded by the per-asset `Prepare for scene` flow. |
| `Reference Image` button | `.toolbar-actions` (toolbar) | Duplicated the in-tuning-panel `Upload reference image` button from T-007-03; uploading via the toolbar bypassed the `from-reference-image` lighting-preset gate and confused users. |
| `processAll()` JS function | `static/app.js` | No longer called anywhere. |

## Acceptance criteria check

| AC | Status |
|---|---|
| Collapsed `Advanced` disclosure containing the seven listed technique buttons | ✓ — `<details class="toolbar-advanced">` in `static/index.html`. |
| Disclosure collapsed by default, user can expand | ✓ — native `<details>` defaults to closed. |
| Label audit + rename: `Production Asset`, `Vol LODs`, `VL0..VL3`, `Billboard` | ✓ — see rename log above. |
| All `Generate` verbs unified to one verb | ✓ — `Build` for resting state, `Building…` for running state. |
| Toolbar action labels reviewed for jargon | ✓ — see rename log; per-button `title` attributes also rewritten to one-sentence designer-facing descriptions. |
| Tuning panel section name and field labels reviewed | ✓ — section is now `Asset tuning`; twelve fields renamed. |
| `Process` left-panel button folded or removed | ✓ — removed; rationale documented above. |
| All renames logged in `review.md` | ✓ — this file. |
| Manual fresh-eyes verification | ⚠ — not performed by this pass. See "Open concerns" #1. |

## Test coverage

- **Automated:** none added. The change is presentation-only and
  the project has no JS test harness today.
- **Sanity checks run:**
  - `node --check static/app.js` — clean.
  - `go build ./...` — clean.
  - `go test ./...` — `ok glb-optimizer (cached)`.
  - `grep -n processAllBtn static/app.js static/index.html` —
    only T-008-02 removal comments remain.
  - `grep -n uploadReferenceBtn static/app.js static/index.html` —
    only T-008-02 removal comments remain.
- **Manual verification checklist** for the human reviewer (full
  version is in `plan.md` step 7):
  1. Toolbar shows `Prepare for scene` + `Advanced ▸` only.
  2. Click `Advanced` → floating panel reveals the seven build
     buttons. Six of them when Blender is unavailable
     (`#generateBlenderLodsBtn` is hidden imperatively).
  3. Click each build button on a selected asset; running label
     flips to `Building…` and back to the new resting label.
  4. Click `Run lighting diagnostic`; console emits the test
     report exactly as before.
  5. Walk the lod-toggle row; the new labels swap the preview to
     the same versions as before.
  6. Right panel shows `Asset tuning`; every renamed slider /
     select still moves and the value readout still updates.
  7. Pick the `From Reference Image` lighting preset; the
     in-tuning-panel `Upload reference image` button still
     appears and still triggers the file picker (proves the
     hidden `#referenceFileInput` survival is correct).
  8. Left panel: `Process All` is gone; `Download All (.zip)`
     still appears once at least one file is `done`.
  9. **Fresh-eyes check**: walk a teammate (or anyone in the
     project who has not seen the new labels) through the seven
     Advanced buttons. Note any "what does that do?" reactions.
     The point of the rename is to *eliminate* those reactions —
     a follow-up label tweak is cheap.

## Open concerns and known limitations

1. **Manual fresh-eyes verification not performed.** AC bullet 8
   ("a teammate / fresh-eyes review confirms each button is
   understandable without context") is the highest-leverage
   verification on this ticket and the agent cannot run it. The
   reviewer should do this and either accept the labels or note
   the reaction so a follow-up can iterate. The label log above
   makes any future tweak trivial — just edit `index.html` and
   the matching JS resting-label string.

2. **VL0..VL3 labels diverge from the AC's literal text.** The
   AC proposes `Volumetric (highest detail)` / `(medium)` /
   `(low)` / `(billboard)`. Those labels are too wide for the
   11-button inspector row and would force a multi-line wrap or a
   font-size collapse. The shorter `Vol high / med / low /
   billboard` carries the same meaning. If the reviewer wants the
   literal AC text instead, expand the labels and either let the
   row wrap to two lines or remove the `lod0..lod3` buttons from
   the row (they're inspector-only).

3. **The hidden `#referenceFileInput` and the cross-panel
   coupling.** `wireTuningUI` at `app.js:772-777` reaches across
   the DOM to trigger the toolbar's hidden file input. That's the
   reason the input survived the toolbar cleanup. A follow-up
   could move the `<input type="file">` adjacent to the
   `#tuneReferenceImageBtn` to make the coupling local; not done
   here because the AC scope is "rename + disclosure," not
   "restructure the reference-image plumbing."

4. **`/api/process-all` is now an orphan endpoint.** No frontend
   code calls it. Reachable from curl/devtools. Removing the
   route, its handler in `main.go`, and any associated tests is a
   one-line ticket — left for a future cleanup pass since the
   ticket scope was UI.

5. **`processFile` (single-file) is still alive** because
   `prepareForScene` Stage 1 calls it and because the per-file
   "Process" button rendered inline by `renderFileList` (the
   `file-process-btn` shown next to each pending file in the
   left panel) still uses it. Confirmed by `grep -n processFile`.
   That per-file button is *not* the same as `processAllBtn` and
   was not in scope for this ticket.

6. **Tuning field rename ripples to JSON nothing.** The Go
   `AssetSettings` field names (`volumetric_layers`,
   `volumetric_resolution`, …) are unchanged — only the
   *display* labels move. Profiles, stored settings, and
   analytics events all keep working untouched.

7. **`<details>` accessibility.** Native `<details>` is
   keyboard- and screen-reader-friendly out of the box (Space /
   Enter on the focused `<summary>` toggles it). No extra ARIA
   attributes were added. AC explicitly puts the a11y audit out
   of scope; flagged here so a future a11y pass knows the surface
   is already mostly correct.

## Critical issues

None. All sanity checks pass. The change is additive at the CSS
level, removes two surfaces that were already known-redundant,
and renames labels without touching `data-lod` attribute values,
field ids, settings keys, or analytics event names.

## Suggested reviewer focus

- Walk the manual checklist (especially the **fresh-eyes** step)
  on a real asset.
- Decide whether the `Vol …` lod-toggle compromise is acceptable
  or whether the literal AC text should win. If the latter, the
  follow-up is one HTML edit + a CSS width tweak.
- Confirm the `Process All` removal does not break any
  unobserved batch workflow — the only call site I could find
  was the deleted `processAll()` function.
