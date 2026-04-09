# Design — T-005-03: color-calibration-mode

## Decisions at a glance

| Question | Decision |
|---|---|
| Where does the enum live? | New `ColorCalibrationMode` field on `AssetSettings`, parallel to `LightingPreset`. |
| Default value? | `"none"` (neutral lighting). |
| Empty-string handling on disk? | Forward-compat normalization in `LoadSettings` → `"none"`. |
| Where is `reference_image_path` written from? | Client side, after a successful upload (`uploadReferenceImage`). |
| Where is calibration *applied*? | Same place as today (`loadReferenceEnvironment` + bake-light reads of `referencePalette`); only the *gate* changes. |
| What gates calibration? | `currentSettings.color_calibration_mode === "from-reference-image"`. Live preview and bake both consult the gate via the existing `referencePalette` state variable. |
| New tuning panel control? | A `<select id="tuneColorCalibrationMode">` row + a conditional "Upload reference image" button row. |
| How does the user upload from the panel? | A small `<button>` that triggers `referenceFileInput.click()` (the existing hidden input). No DOM duplication. |
| Existing toolbar button? | Left in place. T-005-03 is additive; S-007 will collapse the toolbar button. |

## Options considered

### Option A — Add the enum to `AssetSettings` as its own field (chosen)

- Pros: forward-compatible with S-007's lighting-preset story (which
  will *consume* this field rather than replace it), additive schema
  change, mirrors the existing pattern (`SliceDistributionMode`,
  `LightingPreset`), enrolls cleanly in the tuning UI walker, gets
  free `setting_changed` analytics, plays nicely with
  `SettingsDifferFromDefaults`.
- Cons: introduces a second mode dimension that S-007 will need to
  reconcile; risk of ambiguous semantics if S-007's expanded preset
  enum ALSO grows a `from-reference-image` value. Mitigated by keeping
  the new field deliberately tiny in v1 (`none` and
  `from-reference-image` only), so S-007 can either keep both fields
  or fold them with a one-way migration without breaking on-disk
  shape.

### Option B — Reuse `LightingPreset` and add a `from-reference-image` value to it

- Pros: zero new schema fields; one less form control; one less enum
  to validate.
- Cons: conflates two orthogonal concepts (lighting *style* vs color
  *source*). The S-007 story explicitly enumerates lighting presets
  (`midday-sun`, `overcast`, etc.) AND treats `from-reference-image`
  as one of them — so this *is* the destination shape. But landing it
  here would force this ticket to also (a) preview-rename the enum,
  (b) ship the migration logic, and (c) reach the rest of S-007's
  preset surface area to avoid an inconsistent partial state. That
  scope creep is exactly what the ticket warns against in "Out of
  Scope". Rejected.

### Option C — A boolean `use_reference_image_calibration` instead of an enum

- Pros: simplest schema; no enum churn.
- Cons: dead end. S-007 explicitly needs this to be an enum (more
  preset values incoming), so a boolean would need to be migrated
  away in S-007. Forces extra work to no benefit. Rejected.

### Option D — Have the server populate `reference_image_path` from `handleUploadReference`

- Pros: server is the only writer; client doesn't need to construct
  paths or know the on-disk layout.
- Cons: introduces a hidden write side-effect on the upload endpoint
  (it now also touches the settings file), which complicates error
  handling (what if the settings save fails after the image saved?)
  and couples two on-disk artifacts that are otherwise independent.
- **Decision**: rejected. Client-side write keeps the upload endpoint
  doing one thing. The path string is just a tag the bake later reads
  to construct a `/api/reference/:id?…` URL anyway — the client
  already knows the id.

## Chosen approach

### Schema additions (single struct, two fields, no version bump)

```go
// AssetSettings (settings.go)
ColorCalibrationMode string `json:"color_calibration_mode"`
ReferenceImagePath   string `json:"reference_image_path,omitempty"`
```

- Defaults: `ColorCalibrationMode = "none"`,
  `ReferenceImagePath = ""`.
- Enum table: `validColorCalibrationModes = {"none", "from-reference-image"}`.
- `Validate()`: rejects unknown enum values; allows any string for
  `ReferenceImagePath` (free text, sanitized only by virtue of
  being a JSON string — this is a single-user local tool and the
  path is used purely as a tag, not dereferenced server-side).
- `LoadSettings` normalization: if `ColorCalibrationMode == ""` after
  decode, set to `"none"`. (`ReferenceImagePath` zero value is
  meaningful — empty literally means "not set".)
- `SettingsDifferFromDefaults` extends by two field comparisons.
- `DefaultSettings()` adds the field initializers in declaration
  order.
- `static/app.js:makeDefaults()` mirrors the same defaults (mandatory
  hand-sync per the schema doc).
- `docs/knowledge/settings-schema.md` gets two new rows in the field
  table and a JSON example update.

### Bake/preview gate

The single state variable that today says "calibration is on" is
`referencePalette`. Keep that the source of truth and gate writes to
it on the mode:

- `selectFile(id)`:
  - `loadSettings(id)` runs **first** (it does today, but inside the
    `loadEnv.then(...)` continuation). Reorder so settings load
    happens before the calibration decision.
  - Then: if `currentSettings.color_calibration_mode ===
    "from-reference-image"` AND `file.has_reference`, call
    `loadReferenceEnvironment(id)`. Otherwise leave
    `referencePalette = null` (the neutral default already established
    by the existing reset earlier in `selectFile`).
- Dropdown change handler (auto-wired by `wireTuningUI`):
  - Add a *post-change hook* triggered when the field is
    `color_calibration_mode`. In the same handler that fires
    `setting_changed`, after the debounced save, call
    `applyColorCalibration(selectedFileId)`. This function reads
    `currentSettings.color_calibration_mode` and the file record and
    either runs `loadReferenceEnvironment` (mode flipped on) or
    tears it down (mode flipped off → `referencePalette = null`,
    `scene.environment = defaultEnvironment`, `resetSceneLights()`).
    Then re-applies the model so the new env takes effect.
- Reference upload (`uploadReferenceImage`):
  - On success, set `currentSettings.reference_image_path =
    relative path` and call `saveSettings`. If the current mode is
    `from-reference-image`, also call `loadReferenceEnvironment` as
    today (existing line — no edit). If mode is `none`, *do not*
    auto-apply — uploading an image while mode is `none` stages the
    image but does not mutate the live scene. Document this in the
    review.

### UI

Two new rows inside `#tuningSection`, just above the reset button:

```html
<div class="setting-row">
  <label>Color calibration mode</label>
  <select id="tuneColorCalibrationMode">
    <option value="none">none</option>
    <option value="from-reference-image">from-reference-image</option>
  </select>
</div>

<div class="setting-row" id="referenceImageRow" style="display:none">
  <button class="preset-btn" id="tuneReferenceImageBtn">Upload reference image</button>
</div>
```

- The conditional row's visibility is driven by the dropdown's value:
  shown when `from-reference-image`, hidden otherwise. Driven from
  `populateTuningUI` (after the dropdown's value is set) and from the
  same input handler that fires the analytics event.
- The button calls `referenceFileInput.click()` — same hidden input as
  the toolbar button. **No** new file input element.
- Label text on the button reflects current state via
  `updateReferenceImageButton()` (a tiny helper that mirrors the
  existing `updatePreviewButtons` toolbar logic for "Reference ✓" vs
  "Upload reference image"). Avoids hard-duplicating
  `updatePreviewButtons`.

### `TUNING_SPEC` entry

```js
{ field: 'color_calibration_mode', id: 'tuneColorCalibrationMode',
  parse: v => v, fmt: v => v },
```

Free analytics, free dirty-dot tracking, free populate.

### Behavior change documentation

- **Before**: any asset with a reference image on disk + `has_reference`
  set in memory auto-applied calibration on selection.
- **After**: calibration only applies when the *settings file* has
  `color_calibration_mode == "from-reference-image"`. Default is
  `none`.
- For an asset that already had a reference image but **no settings
  file**, the user will now see a neutral preview until they pick the
  mode. To avoid silently regressing existing tuned assets, the
  one-time migration path is: in `scanExistingFiles`, when
  `HasReference` is true (it isn't restored on startup today, so this
  branch never fires in practice — the gap is documented as a
  pre-existing issue and out of scope), upgrade the in-memory record
  to mode `from-reference-image`. Because the gap is pre-existing and
  out of scope, no operator-visible regression occurs in the only
  realistic flow today (upload, then tune, in the same session): the
  ticket is wired client-side so the moment the user uploads an
  image *or* sees the dropdown they get the new behavior.

## Why this is the smallest viable design

- One new struct field per AC bullet, no schema version bump, no new
  endpoints, no DOM duplication, no rewrites of existing palette code.
- Reuses the proven `TUNING_SPEC` walker for analytics, dirty dot,
  reset, and persistence — every feature comes "for free".
- Keeps S-007 unblocked by leaving `lighting_preset` untouched.
- The behavior gate is one branch in `selectFile`, one branch in the
  dropdown's input handler, and one branch in `uploadReferenceImage`.
  Three small touch points, all reading one mode field.

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| Ambiguous semantics with S-007's preset enum | Keep `color_calibration_mode` strictly limited to {none, from-reference-image} in v1. S-007 can absorb without breaking. |
| Migration regression for existing tuned assets | None in practice — `scanExistingFiles` doesn't restore `HasReference` today. Documented in research/review. |
| Path traversal or unsafe `reference_image_path` | Field is purely a tag, never dereferenced server-side. The image is fetched via the existing `/api/reference/:id` endpoint by id, not by path. Validation just stores the string. |
| Future field ordering churn | Append new fields at the bottom of `AssetSettings` (after `GroundAlign`). Declaration order = JSON order. |
