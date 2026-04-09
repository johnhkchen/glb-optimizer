# Research — T-008-02: advanced disclosure + label rename

T-008-01 added `Prepare for scene` as the toolbar's primary action and
left every other button untouched. This pass surveys what's still
visible to a first-time user and where the jargon lives.

## Toolbar surface inventory (`static/index.html`)

The center-panel `.preview-toolbar` is one flex row that wraps. From
left to right it contains five logically-distinct groups:

1. `.preview-toggle` — `Original` / `Optimized` (clear).
2. `.lod-toggle` — 11 inspector buttons keyed by `data-lod`:
   `lod0..lod3`, `billboard`, `volumetric`, `production`,
   `vlod0..vlod3`. Visible labels: `LOD0..3`, `Billboard`, `Volumetric`,
   `Production`, `VL0..3`. Each is a *preview swap*, not an action.
3. `#wireframeBtn` — `Wireframe` (clear).
4. `.toolbar-actions` — the eight technique buttons that the ticket
   targets. T-008-01 placed `Prepare for scene` as the first child.
   Current order:
   - `#prepareForSceneBtn` — `Prepare for scene` *(primary, T-008-01)*
   - `#generateLodsBtn` — `LODs (gltfpack)`
   - `#generateBlenderLodsBtn` — `LODs (Blender)` *(hidden until
     `/api/status` reports Blender available)*
   - `#generateBillboardBtn` — `Billboard`
   - `#generateVolumetricBtn` — `Volumetric`
   - `#generateVolumetricLodsBtn` — `Vol LODs`
   - `#generateProductionBtn` — `Production Asset`
   - `#bakeStaleHint` — text-only stale-bake hint *(T-007-02)*
   - `#uploadReferenceBtn` — `Reference Image` *(superseded — see
     concern below)*
   - `#testLightingBtn` — `Test Lighting`
5. `#prepareProgress` — per-stage progress block + `View in scene`.
6. `.stress-controls` — Scene/Count/Ground/LOD/Quality/`Run scene`.

## Left-panel surface (`.panel-left`)

Drop zone, file list, then `.panel-actions`:

- `#processAllBtn` — `Process All` *(T-008-02 candidate for removal)*
- `#downloadAllBtn` — `Download All (.zip)` *(hidden until any file
  is done; clear)*

`processAllBtn` calls `processAll()` (`static/app.js:1210`), which
POSTs `/api/process-all` — the same gltfpack optimize step that
Stage 1 of `prepareForScene` runs per-file. It is *batch optimize
only*; it does not classify, build LODs, or build the production
asset.

## Right-panel surface (tuning + auxiliary)

`<h3>` section names: `Mesh Settings`, `Texture Settings`, `Output`,
`Tuning`, `Profiles`, `Accepted`. The `Tuning` section is the heaviest
jargon block. Field labels currently in the section:

- `Volumetric layers`
- `Volumetric resolution`
- `Dome height factor`
- `Bake exposure`
- `Ambient intensity`
- `Hemisphere intensity`
- `Key light intensity`
- `Bottom fill intensity`
- `Env map intensity`
- `Alpha test`
- `Lighting preset`
- `Slice distribution mode` (`visual-density` / `vertex-quantile` /
  `equal-height`)
- `Ground align bottom slice`
- inline `Upload reference image` (only visible when the lighting
  preset is `from-reference-image` — see T-007-03)
- `Reclassify…`
- `Reset to defaults`

The other right-panel sections (`Mesh Settings`, `Texture Settings`,
`Output`) carry gltfpack-flag jargon (`-si`, `-cc`, `-tc`, …) but
the flag pills already mark them as "advanced detail," and they are
not in scope for this ticket beyond a quick label review.

## Where the labels are referenced from JS

Most button labels are read off `index.html` only at startup, but
several are *re-set* during state transitions and must be updated in
both places to keep them coherent:

| Button id | HTML label | JS re-sets in |
|---|---|---|
| `generateLodsBtn` | `LODs (gltfpack)` | `app.js:1247,1261` |
| `generateBlenderLodsBtn` | `LODs (Blender)` | `app.js:1267,~1280` |
| `generateBillboardBtn` | `Billboard` | `app.js:1294,1316` |
| `generateVolumetricBtn` | `Volumetric` | `app.js:1718,1739` |
| `generateVolumetricLodsBtn` | `Vol LODs` | `app.js:2172,2195` |
| `generateProductionBtn` | `Production Asset` | `app.js:2206,2239` |
| `uploadReferenceBtn` | `Reference Image` | `app.js:4180,4183` |
| `processAllBtn` | `Process All` | `app.js:1213,1230` |
| `prepareForSceneBtn` | `Prepare for scene` | `app.js:2297,2387` |

`prepareForScene` snapshots the original label into a local
(`originalLabel = prepareForSceneBtn.textContent`) and restores it
in the `finally`, so renaming the HTML is sufficient — no JS edit
needed for that one.

The `lod-toggle` buttons are inspector-style and their labels live
only in HTML; JS only reads `data-lod`.

## Reference-image / lighting-preset entanglement (T-007-03)

`#uploadReferenceBtn` and the `from-reference-image` lighting preset
both reach the same upload sink. Per `T-007-03/review.md`:

- The tuning panel exposes `#tuneReferenceImageBtn` inside
  `#referenceImageRow`, which `syncReferenceImageRow()`
  (`app.js:4084`) shows only when
  `currentSettings.lighting_preset === 'from-reference-image'`.
- The toolbar `#uploadReferenceBtn` is the *legacy* path. It works
  but bypasses the lighting-preset gating: clicking it on an asset
  whose preset is `default` uploads an image that the bake then
  ignores until the preset is also flipped.

This argues for **removing** the toolbar reference button as part of
the rename pass — it is a confusing duplicate path. The ticket's AC
already softly authorizes this: *"Reference Image (if not already
absorbed into the lighting preset from T-007-03)"*. T-007-03 absorbed
the *control* but not the *button*; T-008-02 finishes the job.

## Test Lighting

`#testLightingBtn` triggers `testLighting()` (`app.js:2402`) which
renders an offscreen sphere + model side-by-side and dumps a textual
report to the console. It is a developer diagnostic, not a user
action, and the label "Test Lighting" reads to a designer like a
*configurator*. It belongs behind the `Advanced` disclosure with a
clearer "diagnostic" framing.

## Constraints from the existing code

1. **`data-lod` values are persisted via the active preview state.**
   Renaming the visible labels of the lod-toggle is safe; renaming
   the `data-lod` attribute values is not (they're read by
   `updatePreviewButtons` and the click handler at `app.js:4236+`).
   This pass touches only label text.
2. **Analytics event names must not change.** `regenerate`,
   `prepare_for_scene`, `preset_applied`, `setting_changed` are all
   referenced by `analytics-schema.md`. Button labels are display
   only — no analytics impact.
3. **The Blender LOD button is conditionally shown.** Any disclosure
   markup must keep `style.display = ''` working for the hide/show
   path in `app.js:4443`.
4. **`#prepareProgress` and `#bakeStaleHint` live inside
   `.toolbar-actions` today.** They must remain *outside* the
   collapsed disclosure — the user needs to see progress/stale-state
   regardless of whether they expanded Advanced.
5. **CSS for the toolbar is at `style.css:357-455`.** A new
   disclosure container will need new rules but should reuse
   `.toolbar-btn` so the technique buttons inherit the existing
   style without duplication.

## Open questions surfaced by research

- **Should `Process All` survive in the Advanced disclosure?** The
  ticket says "fold or remove, pick the simpler". `processAll`
  is one fetch that hits `/api/process-all`; the only thing it does
  that `Prepare for scene` does not is *batch* across multiple
  uploaded files. The single-asset tuning workflow that S-008 is
  built around does not exercise that path. Decision deferred to
  Design.
- **Should the lod-toggle inspector labels be renamed?** The
  ticket's AC explicitly mentions `VL0..VL3 → Volumetric (highest
  detail) / (medium) / (low) / (billboard)`, so yes for those four.
  `LOD0..LOD3` and `Billboard`/`Volumetric`/`Production` are not
  named. Decision: rename them to match the new toolbar verbs for
  consistency. Decision deferred to Design.
- **What "one verb" replaces all the `Generate` verbs?** The codebase
  already mixes `Generate`, `Render`, `Build`, `Process`. The
  primary action verb in T-008-01 is `Prepare`. The verb that reads
  most landscape-designer-friendly is `Build`. Decision deferred
  to Design.
