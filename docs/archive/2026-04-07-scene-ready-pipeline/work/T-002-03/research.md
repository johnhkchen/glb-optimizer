# Research — T-002-03: tuning-panel-ui-skeleton

## Ticket Goal in One Line
Add a minimal "Tuning" section to the right panel exposing one control per
`AssetSettings` field, wired into the existing `currentSettings` /
`saveSettings` plumbing from T-002-02.

## What Already Exists (the "given")

### Backend
- `settings.go` — defines `AssetSettings` (T-002-01) and `DefaultSettings()`.
  Eleven fields plus `schema_version`. Validation lives on the type.
- `handlers.go:617-668` — `handleSettings(store, settingsDir)` serves
  `GET /api/settings/:id` and `PUT /api/settings/:id`. GET falls back to
  `DefaultSettings()` if no file exists. PUT validates, writes atomically,
  flips `HasSavedSettings` on the `FileRecord`. Both require the file id
  to exist in the store (404 otherwise).
- There is **no** `/api/settings/defaults` endpoint. The closest substitute
  is "GET on any existing file id whose settings file is absent", which
  returns the canonical defaults but mutates nothing.

### Frontend plumbing (T-002-02 deliverable)
- `static/app.js:29-30` — module-level `currentSettings` and
  `_saveSettingsTimer`.
- `static/app.js:81-91` — `loadSettings(id)` async; falls back to
  `applyDefaults()` on error.
- `static/app.js:93-109` — `saveSettings(id)` debounced **500 ms**, PUTs
  `currentSettings` to the backend.
- `static/app.js:111-128` — `applyDefaults()` writes a hand-synced literal
  mirror of `DefaultSettings()` into `currentSettings`. T-002-02 review
  flags this as a known sync hazard.
- `static/app.js:2152` — `selectFile()` already `await`s `loadSettings(id)`
  before `loadModel()`. File switch automatically refreshes
  `currentSettings`.
- `static/app.js:359, 417-425, 480, 500, 549, 576, 611, 639, 673-675, 797,
  804, 878` — every bake call site reads `currentSettings.*` directly. No
  intermediate cache.

### Existing UI shape
- `static/index.html:94-218` — right panel structure:
  - `.presets` rows (existing buttons; unchanged scope)
  - `.settings-section > h3 + .setting-row*` blocks for "Mesh Settings",
    "Texture Settings", "Output". This is the visual idiom to match.
- `static/style.css:536-580` — `.settings-section`, `.settings-section h3`,
  `.setting-row`, `.setting-row label`, `.setting-row input[type="range"]`,
  `.setting-row .range-value`. The slider styling already exists and is
  the right hook.
- `select` is globally styled (`style.css:625-637`).
- DOM lookups in `app.js` are by id (`document.getElementById(...)`) and
  cached at module top (`app.js:33-64`).

### "Generate" buttons that consume tuning
- `generateBillboardBtn`, `generateVolumetricBtn`,
  `generateProductionBtn`, `generateVolumetricLodsBtn` — all already
  read from `currentSettings` indirectly (via `setupBakeLights`,
  `cloneModelForBake`, `renderHorizontalLayerGLB`). No re-wiring needed.

## Schema Reference (canonical from `settings.go:39-54`)

| field                   | type   | default | UI control       | range hint      |
|-------------------------|--------|---------|------------------|-----------------|
| `volumetric_layers`     | int    | 4       | number/range     | 1–12            |
| `volumetric_resolution` | int    | 512     | dropdown         | 256/512/1024    |
| `dome_height_factor`    | float  | 0.5     | slider           | 0.0–1.0         |
| `bake_exposure`         | float  | 1.0     | slider           | 0.5–2.5         |
| `ambient_intensity`     | float  | 0.5     | slider           | 0.0–2.0         |
| `hemisphere_intensity`  | float  | 1.0     | slider           | 0.0–2.0         |
| `key_light_intensity`   | float  | 1.4     | slider           | 0.0–3.0         |
| `bottom_fill_intensity` | float  | 0.4     | slider           | 0.0–1.5         |
| `env_map_intensity`     | float  | 1.2     | slider           | 0.0–3.0         |
| `alpha_test`            | float  | 0.10    | slider           | 0.0–0.5         |
| `lighting_preset`       | string | default | dropdown         | {default}       |

Note: the validator's allowed ranges (`settings.go:68-106`) are wider than
the UI ranges above. This is intentional — UI ranges are "useful" ranges
the human will tune within; the schema permits a broader programmatic
range. PUT will still succeed even if the UI ever sends a higher value.

`volumetric_resolution`'s validator allows `{128,256,512,1024,2048}`. The
ticket only exposes three of those — `256/512/1024`. The other two stay
reachable via the JSON file but not via the UI skeleton.

## Constraints / Boundaries

- **No live preview**: explicit non-goal in ticket. Bake button is the
  feedback loop.
- **No new visual idiom**: re-use `.settings-section`, `.setting-row`,
  `.range-value` exactly. Don't introduce a new component class.
- **Debounced PUT 300 ms**: ticket overrides the existing `saveSettings`
  500 ms debounce. Need a knob (or a second debounce path) to comply
  with the AC literally. Smallest change: parameterize the existing
  debounce.
- **Reset to defaults**: ticket says "fetches schema defaults and applies
  them". The existing `applyDefaults()` does this purely client-side and
  is the documented hand-off from T-002-02. The literal phrasing
  "fetches" can be honored either by adding a backend endpoint *or* by
  treating `applyDefaults()` as the canonical local fetch. The
  T-002-02 review flagged the duplication as a known limitation, not a
  bug. Design phase will pick.
- **Dirty indicator**: subtle dot next to the section header when
  `currentSettings` differs from defaults. Needs a comparison helper.
- **Existing mesh/texture/output sections must not regress.** They use
  hardcoded ids on `getElementById`. Adding a new section with new ids
  is additive — no conflict expected.

## Surfaces I Need to Touch

1. `static/index.html` — add `<div class="settings-section">` after the
   "Output" block. Add hand-written controls with stable ids.
2. `static/app.js` — add a new "Tuning UI" section that:
   - Caches the new DOM nodes.
   - On init, attaches `input` listeners that mutate `currentSettings`
     and call `saveSettings(selectedFileId)`.
   - On `selectFile`'s `loadSettings` resolution, populates control
     values from `currentSettings` (need a hook).
   - Updates the dirty indicator after every load + every input.
   - Wires the "Reset to defaults" button to `applyDefaults()` +
     `saveSettings()` + `populate()`.
3. `static/style.css` — small additions:
   - `.dirty-dot` styling for the section header indicator.
4. `static/app.js` — modify `saveSettings` to accept (or default to)
   the 300 ms debounce required by the ticket. Cleanest path: change
   the constant from 500 → 300. The 500 was speculative in T-002-02.
   Confirm via review of any other caller — there are none yet.

## Files NOT Touched

- `handlers.go`, `settings.go`, `models.go`, `main.go`, `processor.go`,
  `blender.go`, `scene.go` — backend is complete.
- `docs/knowledge/settings-schema.md` — schema unchanged.
- Any other section of `app.js` outside the new tuning block and the
  one-line debounce constant tweak.

## Open Questions Surfaced

- **Q1**: Add `/api/settings/defaults` or rely on `applyDefaults()`?
  Resolved in Design.
- **Q2**: Use auto-generated DOM (`spec[]` → `forEach`) or hand-written
  HTML for the eleven controls? Resolved in Design.
- **Q3**: How does the dirty indicator decide "dirty"? Strict deep
  compare against `applyDefaults()` output. Resolved in Design.
- **Q4**: Should the "Reset" button trigger an immediate (non-debounced)
  PUT? Yes — explicit user action, no need to wait. Resolved in Design.

## Risks

- **Hand-sync drift** between `applyDefaults()` JS literal and
  `DefaultSettings()` Go literal — already a known limitation. This
  ticket inherits it and adds one more consumer (the dirty compare).
- **Layout overflow** if the new section pushes the right panel past
  its scroll height. The panel already scrolls (per
  `style.css:536-580` chain ending in body overflow). Low risk.
- **Debounce 500 → 300** affects the only other (currently nonexistent)
  caller of `saveSettings`. Safe to change.
