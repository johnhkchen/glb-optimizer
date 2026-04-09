# Research — T-005-03: color-calibration-mode

## Ticket scope

Promote the existing one-shot "Reference Image" calibration into a per-asset
setting. Two new fields land in `AssetSettings`: `color_calibration_mode`
(enum: `none` | `from-reference-image`) and `reference_image_path` (optional
string). UI gets a dropdown in the tuning panel. Bake/preview honors the
mode rather than auto-applying calibration whenever a reference image
exists. S-007 will absorb this enum into the larger lighting-preset
framework — keep this ticket additive and forward-compatible.

## What exists today

### Settings layer (backend)

- `AssetSettings` (`settings.go:22`) — 13 fields plus `SchemaVersion=1`.
  Field declaration order matches on-disk JSON order. **No** color
  calibration field today.
- `DefaultSettings()` (`settings.go:41`) — canonical defaults, mirrored
  by hand in `static/app.js:113` (`makeDefaults`).
- `Validate()` (`settings.go:81`) — per-field range/enum checks. Enum
  fields use `validXxx` map literals (`validResolutions`,
  `validLightingPresets`, `validSliceDistributionModes`).
- `LoadSettings()` (`settings.go:173`) — reads JSON, runs forward-compat
  normalization for `slice_distribution_mode` (empty → `visual-density`)
  and `ground_align` (`*bool` re-decode trick to distinguish absent vs
  explicit false). The same pattern applies to any new enum field.
- `SaveSettings()` (`settings.go:214`) — atomic temp-file + rename,
  2-space JSON.
- `SettingsDifferFromDefaults()` (`settings.go:140`) — explicit
  field-by-field comparison; **must** be visited when adding a new field
  or `SettingsDirty` will silently miss the new dimension.
- Schema doc: `docs/knowledge/settings-schema.md` is the source of truth
  table; needs a row for the new fields.

### Reference image flow (existing one-shot)

- **Server side**:
  - `handleUploadReference` (`handlers.go:503`) — POST
    `/api/upload-reference/:id`. Saves the multipart upload to
    `outputs/{id}_reference{.png|.jpg}` (extension sniffed from magic
    bytes). On success sets `FileRecord.HasReference=true` and
    `ReferenceExt`.
  - `handleReferenceImage` (`handlers.go:555`) — GET `/api/reference/:id`,
    serves the bytes back to the browser.
  - `models.go:59-60` — `HasReference bool` and `ReferenceExt string` on
    `FileRecord` (both `omitempty`).
  - **Gap noted**: `scanExistingFiles` (`main.go:163`) does **not**
    restore `HasReference`/`ReferenceExt` on startup. After a restart,
    the file on disk persists but the in-memory record forgets it until
    the user re-uploads. Pre-existing — out of scope here, but worth
    noting because the new "remember calibration mode" feature will
    visibly diverge from this gap on restart.

- **Client side** (`static/app.js`):
  - `let referenceEnvironment = null` / `let referencePalette = null`
    (lines 27–28) — module-level state.
  - `uploadReferenceImage(id, file)` (line 1786) — POSTs the file, then
    `loadReferenceEnvironment(id)`, then triggers a model reload.
  - `loadReferenceEnvironment(id)` (line 1815) — fetches
    `/api/reference/:id`, calls `extractPalette` → assigns
    `referencePalette` → `buildSyntheticEnvironment` (synthesizes a
    PMREM env map from the palette gradient) →
    `applyReferenceTint` (mutates ambient + hemisphere lights in the
    live `scene`).
  - `extractPalette` (line 1844) — corner-sampling background detection,
    tonal slicing into bright/mid/dark, channel-normalized "tints".
  - `buildSyntheticEnvironment` (line 1963) — bright/mid/dark gradient →
    `THREE.CanvasTexture` → `pmremGenerator.fromEquirectangular` →
    `scene.environment`.
  - `applyReferenceTint` (line 1992) — copies tint colors onto the
    hemisphere/ambient lights in the live preview scene.
  - **Bake side** — `setupBakeLights` (line 891) and the offscreen
    renderer paths (`~line 1154`) read `referencePalette` directly to
    tint sky/fill/ground colors. **No mode gate today**: if
    `referencePalette` is non-null, calibration applies. The calibration
    is therefore always active when a reference is loaded.

- **HTML toolbar**:
  - `static/index.html:60-61` — `#uploadReferenceBtn` and the hidden
    `#referenceFileInput`. Button lives in the preview toolbar
    (`.toolbar-actions`), not the tuning panel.
  - Listeners in `app.js:2980-2989` wire the button to the file input
    and the file input to `uploadReferenceImage`.
  - `updatePreviewButtons` (line 2828) flips the button label between
    "Reference Image" and "Reference ✓" based on `file.has_reference`.

- **Selection flow**:
  - `selectFile(id)` (around line 2785) — when switching assets:
    - Disposes the existing `referenceEnvironment`.
    - Resets `referencePalette = null`, `scene.environment = defaultEnvironment`.
    - Calls `resetSceneLights()` (neutral white).
    - If `file.has_reference`, calls `loadReferenceEnvironment(id)`
      — i.e., **always** auto-applies calibration when an image exists.
    - Then `await loadSettings(id)`, `populateTuningUI`, `loadModel(...)`.

### Tuning UI plumbing

- `TUNING_SPEC` (`static/app.js:262`) — declarative array of
  `{field, id, parse, fmt}`. `populateTuningUI` and `wireTuningUI`
  walk it. Adding a new control = one row in this array + one DOM
  element with the matching id.
- Both walkers handle `el.type === 'checkbox'` (T-005-02). Plain
  `<select>` and `<input type="range">` work via `el.value`.
- `wireTuningUI` auto-fires `setting_changed` analytics events
  (`logEvent('setting_changed', …)`). Free instrumentation — no extra
  work needed for the new dropdown.
- Reset-to-defaults button uses `applyDefaults()` → `populateTuningUI` →
  `saveSettings`.
- Panel-header dirty dot computed by `updateTuningDirty()` against
  `makeDefaults()`. Reads every field in `TUNING_SPEC`.

### Bake-side dependencies on the palette

- `setupBakeLights` (`app.js:891`): sky/fill/ground colors come from
  `referencePalette` if non-null, neutral white otherwise.
- `buildSyntheticEnvironment` writes `scene.environment`, which is what
  the preview shows; the bake renderer's offscreen scene re-runs the
  same logic at `app.js:1154` — also keyed on `referencePalette`.
- The "is calibration active right now?" signal is therefore "is
  `referencePalette` truthy?" — a single state variable. Anything that
  needs to *bypass* calibration just needs to keep that variable null.

## Constraints and assumptions

- **Schema is additive.** Two new fields land without bumping
  `SchemaVersion`. Forward-compat normalization in `LoadSettings`
  handles older files (pre-T-005-03) by defaulting both fields to their
  documented zero values.
- **`color_calibration_mode` empty string is invalid** under enum
  validation (same as `slice_distribution_mode`). The normalization
  pass must therefore fill in `"none"` before `Validate` runs, when the
  key is absent on disk.
- **`reference_image_path` is a free string.** It's not enum-validated;
  empty means "not set". Same `omitempty` pattern as `LightingPreset`'s
  string handling but without an enum check.
- **`SettingsDifferFromDefaults` must be extended.** Both new fields
  participate in the `SettingsDirty` calculation, and the
  `single_field_mutated` test should grow a representative case.
- **Default mode is `none`.** Behavior change: existing assets that
  have a reference image on disk but no settings file will *no longer*
  auto-apply calibration on selection (today they do, because the
  client checks `file.has_reference` directly). This is a small
  observable regression that deserves a migration knob — see Design.
- **The reference upload control today lives in the preview toolbar.**
  AC says the tuning panel "shows the existing Reference Image upload
  control (or directs the user to it)". The cheapest path is a
  conditional row in the tuning panel that triggers the same hidden
  file input — no DOM duplication needed.
- **S-007 will replace `lighting_preset` and absorb
  `color_calibration_mode`.** Keep both fields independent so the S-007
  refactor can fold them without a breaking schema bump. Don't reuse
  `lighting_preset` for the new enum.
- **No JS test runner.** Same constraint as every prior tuning ticket;
  JS plumbing is verified by reading + manual exercise.
- **Single-user local tool.** No concurrency concerns beyond the
  existing last-writer-wins on PUT.

## What is *not* in scope (per ticket)

- The full S-007 lighting preset enum.
- Any improvement to `extractPalette` or the synthetic env generation.
- New calibration sources (HDR, color picker, etc.).
- Restoring `HasReference` in `scanExistingFiles` on startup (pre-existing gap).

## Open questions resolved during research

- **Where does `reference_image_path` get populated?** — Most natural
  point is the client, immediately after `uploadReferenceImage` succeeds:
  set `currentSettings.reference_image_path = "outputs/" + id +
  "_reference" + ext` and call `saveSettings`. Server also has the path
  trivially available in `handleUploadReference` and could write it
  there, but that creates a write-side coupling between the upload
  endpoint and the settings file. Client-side keeps the existing
  endpoints untouched.
- **How is calibration bypassed when mode is `none`?** — Gate the
  *application* of calibration, not the load. In `selectFile`, only
  call `loadReferenceEnvironment` when both `has_reference` is true
  *and* the just-loaded settings have mode `from-reference-image`.
  Reorder so `loadSettings` runs first, then the calibration decision.
