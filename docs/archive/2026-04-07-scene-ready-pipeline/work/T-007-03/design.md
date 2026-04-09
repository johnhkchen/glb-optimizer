# Design — T-007-03: reference-image-as-preset-option

## Goal

Fold `color_calibration_mode` into `lighting_preset` so the tuning
panel has one dropdown that lists both built-in lighting presets and
"calibrated from your reference image."

## Options considered

### Option A — Add `from-reference-image` to the preset registry, delete the old field

Add a seventh entry to `LIGHTING_PRESETS` whose `bake_config` is a
neutral baseline (≈ `default`). Selection is detected by id in
`applyLightingPreset`; when the id matches, run the existing
reference-image flow (`loadReferenceEnvironment`,
`applyEnvironmentToModel`) in addition to the cascade. Delete
`ColorCalibrationMode` from the Go struct, JS defaults, HTML, and
the diff helper. Forward-compat: in `LoadSettings`, if a legacy
file with `color_calibration_mode: from-reference-image` is read,
overwrite `lighting_preset` with the new id (and only then null
the old field).

**Pros:**
- Single source of truth (the preset registry) for the dropdown.
- Existing `getActiveBakePalette` / `getActivePreviewPalette`
  reference-palette priority already does the right thing — no
  bake-pipeline changes.
- Cascade analytics (`preset_applied`) unify with the rest of the
  preset switching story; no parallel `setting_changed` event for
  the calibration toggle.
- Deletes ~30 lines of dead enum / sync / handler code.
- Matches the ticket's stated AC literally.

**Cons:**
- The `from-reference-image` "preset" is a polymorphic-by-id
  special case inside the preset registry — it carries
  intensities like every other preset but its colors are
  ignored at runtime when a palette is loaded. Worth a comment.
- A user who selects `from-reference-image` on an asset *without*
  an uploaded image sees the cascade rewrite their sliders to
  the neutral baseline AND no calibration. The upload row appears
  inline, prompting them to fix it. Acceptable per AC.

### Option B — Keep the two enums but visually merge them in the UI

Render a single combined dropdown that's a virtual concatenation:
six lighting presets followed by "from reference image." Picking
the seventh entry sets `color_calibration_mode = from-reference-image`
and `lighting_preset = default`; picking any of the first six does
the inverse.

**Pros:**
- No schema changes.
- No migration needed.

**Cons:**
- Two source-of-truth enums for one user concept; the AC
  explicitly says the calibration mode should be removed.
- The combined dropdown becomes a polymorphic dispatcher — every
  time you read the user's choice you have to inspect both
  fields.
- Saved profiles (T-003-03) and analytics events (`preset_applied`)
  get more confusing, not less.
- Schema gunk grows. Out of step with the consolidation direction
  S-007 is moving in.

### Option C — Add `from-reference-image` as a preset BUT keep `color_calibration_mode` as a derived field

Have `color_calibration_mode` become a computed view of
`lighting_preset === 'from-reference-image'`. Server validates and
ignores it; client doesn't display it.

**Pros:**
- "Strict" backwards compatibility — old API consumers see the
  same shape.

**Cons:**
- We have no other API consumers — this is a single-user dev
  tool. The "compatibility" is entirely fictional.
- More code, not less.
- The dead field on disk grows ambiguous if it ever drifts from
  the derived value.

## Decision: Option A

Option A is the only one that delivers what the AC asks for. The
ticket's First-Pass Scope explicitly authorizes deleting the old
field outright if no production data exists ("the migration step
is cosmetic since this is a single-user dev tool"). Options B/C
keep the dual-enum tax forever to dodge a one-line forward-compat
hop.

## Open decisions

### D1 — `bake_config` for the new preset

Mirror `default` numerically (ambient 0.5, hemi 1.0, key 1.4,
fill 0.4, env 1.2, exposure 1.0). The runtime palette override
takes precedence whenever an image is loaded, so the fallback
only matters when the user picks the preset on an asset that
has no reference image yet. A neutral baseline is the least
surprising thing in that case.

Colors (`hemisphere_sky`, `key_color`, `fill_color`, `env_gradient`)
also mirror `default`'s near-white values for the same reason —
they only ever render when no palette is loaded.

`preview_overrides` is not needed; the `default` numbers already
look fine in the live preview because the calibration tint is
applied on top.

### D2 — Where the upload-row visibility lives

`syncReferenceImageRow()` currently reads
`currentSettings.color_calibration_mode`. After this ticket it
reads `currentSettings.lighting_preset === 'from-reference-image'`.
Single-line change; no rename of the function (its job hasn't
changed semantically).

### D3 — Where the calibration side-effect lives

Today it's in `applyColorCalibration(id)`, called from the
`color_calibration_mode` change handler and the reset path.
After this ticket the side effect needs to run when the user
picks `from-reference-image` from the preset dropdown — the
natural place is `applyLightingPreset(id)`.

`applyLightingPreset` currently:
1. Looks up the preset, falling back to `default`.
2. Cascades intensities into `currentSettings`.
3. `populateTuningUI`.
4. `applyPresetToLiveScene` (lights only — no env map change).
5. `setBakeStale(true)`.
6. Save + emit `preset_applied`.

After this ticket, between (5) and (6) we add: if the new id is
`from-reference-image` AND the asset has a reference image,
`loadReferenceEnvironment` + reload the preview model; otherwise,
if the OLD id was `from-reference-image` (i.e. we're switching
*away* from calibration), tear down `referencePalette` /
`referenceEnvironment`, restore `defaultEnvironment`, and reload
the preview. The legacy `applyColorCalibration(id)` function
becomes the body of this branch and is renamed in spirit; I'll
keep the name and just call it from `applyLightingPreset` for
minimum diff.

### D4 — `selectFile` honors the new preset on asset open

`selectFile()` currently has a `if (mode === 'from-reference-image'
&& file.has_reference) { loadReferenceEnvironment(id) }` block at
line 3050. Same shape, swap the predicate to read the preset id.

### D5 — Reset to defaults

`tuneResetBtn` calls `applyDefaults()` which writes
`lighting_preset: 'default'`. The existing follow-up
`applyColorCalibration(selectedFileId)` then tears down any
calibration state. After this ticket the reset path can call
`applyLightingPreset('default')` instead, which already handles
the tear-down inside the new branch — but applyLightingPreset
also rewrites the slider values via the cascade, which is
exactly what reset wants. Net: replace
`{ applyDefaults(); populateTuningUI(); save; applyColorCalibration }`
with `{ applyDefaults(); applyLightingPreset('default') }` —
fewer lines, same effect, AND it correctly tears down calibration.

### D6 — Migration of legacy on-disk files

`LoadSettings` runs a normalization pass before `Validate`. New
normalization step: if `s.ColorCalibrationMode == "from-reference-image"`,
set `s.LightingPreset = "from-reference-image"`. Then drop the
field altogether (struct removal).

But: the field is being removed from the struct, so the JSON
key is silently ignored on decode. The migration therefore needs
a manual probe — re-decode `data` into a tiny `*string` shim,
same trick used for `ground_align`. If the key was present and
its value was `"from-reference-image"`, set
`s.LightingPreset` accordingly. If the JSON also has
`lighting_preset` set to one of the named presets, the explicit
`lighting_preset` wins (the user has already migrated by hand
or has an inconsistent file).

### D7 — Analytics

`preset_applied` already captures `from`/`to`/`changed`. The
removed `setting_changed` for `color_calibration_mode` falls
out naturally — selecting the new preset emits one `preset_applied`
event with `to: "from-reference-image"`. No analytics-schema
change needed (E-001 epic context noted analytics-first
architecture, so this is the right channel).

### D8 — `reference_image_path` field

Stays. It's still useful as a tag for the upload flow and is
orthogonal to the calibration mode (which has been folded into
the preset). Removing it is out of scope and would force the
upload-side code to track has-reference-or-not via a different
mechanism.

## Rejected variations

- **Renaming `applyColorCalibration` to `syncReferenceCalibration`.**
  Pure churn; the name still describes what it does.
- **Splitting the new preset's bake_config into a sentinel object
  with all-zero intensities.** Tempting because "this preset has
  no intensities of its own," but breaks the cascade — the user
  would see all sliders snap to 0. Neutral baseline is the
  least-surprising fallback.
- **Auto-uploading the reference image when the user picks the
  new preset.** Out of scope and intrusive.
