# Research — T-005-02: bake-quality-control-surface

## Ticket goal in one sentence

Surface every bake-quality knob that already lives in `AssetSettings` as a
tuning-panel control so the user can drive the "tune by eye" loop without
editing JSON, and add a per-asset visual cue in the file list so it's
obvious which assets have been tuned away from defaults.

## Where the relevant code lives

### Settings model — server side

- `settings.go:22-37` — `AssetSettings` is the canonical struct. The
  fields named in this ticket's AC all already exist on it:
  `BakeExposure`, `AmbientIntensity`, `HemisphereIntensity`,
  `KeyLightIntensity`, `BottomFillIntensity`, `EnvMapIntensity`,
  `SliceDistributionMode`, `DomeHeightFactor`, `GroundAlign`.
- `settings.go:41-58` — `DefaultSettings()` returns the canonical v1
  defaults. Today's defaults are already the "tuned" values that
  T-002-01 shipped (`AmbientIntensity: 0.5`, `HemisphereIntensity: 1.0`,
  `KeyLightIntensity: 1.4`, `BottomFillIntensity: 0.4`,
  `EnvMapIntensity: 1.2`, `BakeExposure: 1.0`,
  `DomeHeightFactor: 0.5`, `SliceDistributionMode: "visual-density"`,
  `GroundAlign: true`). No upstream constants to chase.
- `settings.go:80-123` — `Validate()` enforces ranges. Notably the
  server-side ranges are *wider* than the slider ranges the ticket
  asks for, e.g. `key_light_intensity` server-validated to `[0,8]`
  but the slider should be `[0,3]`. The slider is the more
  restrictive of the two; nothing on the server needs widening.
- `settings.go:73-77` — `validSliceDistributionModes` enumerates the
  three legal slice modes (`equal-height`, `vertex-quantile`,
  `visual-density`) — these are exactly what the dropdown needs to
  offer.
- `settings.go:142` — `SettingsExist(id, dir)` returns whether a
  per-asset file is present on disk; this is what currently powers
  `FileRecord.HasSavedSettings`.

### File record / API surface

- `models.go:45-63` — `FileRecord` struct. Already has
  `HasSavedSettings bool` (computed) and `IsAccepted bool`. There is
  no field that says "saved settings differ from defaults", which is
  what the new file-list indicator needs.
- `main.go:193` — `record.HasSavedSettings = SettingsExist(...)` is
  the population point during `scanExistingFiles`.
- `handlers.go:665` — `r.HasSavedSettings = true` is the population
  point on a successful settings PUT.

### Tuning UI — client side

- `static/index.html:219-304` — `<div class="settings-section"
  id="tuningSection">` already contains rows for *every* numeric
  setting in the AC: bake exposure, ambient, hemisphere, key, bottom
  fill, env, alpha test, dome height factor, lighting preset, plus
  the volumetric layer/resolution rows. **What is missing from
  index.html: a row for `slice_distribution_mode` (dropdown) and a
  row for `ground_align` (checkbox).**
- `static/app.js:262-281` — `TUNING_SPEC` already enumerates *every*
  AssetSettings field including `slice_distribution_mode` and
  `ground_align`. T-005-01 explicitly reserved the DOM ids
  `tuneSliceDistributionMode` and `tuneGroundAlign` so that the day
  the elements show up, both `populateTuningUI` and `wireTuningUI`
  pick them up automatically (both functions short-circuit on
  `if (!el) continue;`). No JS spec edits required.
- `static/app.js:283-293` — `populateTuningUI()` walks `TUNING_SPEC`,
  reads `currentSettings[field]`, sets `el.value`, and updates the
  sibling `…Value` span. For a `<select>` this just works. For a
  `<input type="checkbox">` it does *not* — `el.value = true` does
  nothing useful; we need `el.checked = bool`. The checkbox case
  needs special handling.
- `static/app.js:295-337` — `wireTuningUI()` adds an `'input'` event
  listener and on each change: parses, writes `currentSettings`,
  updates the value span, calls `updateTuningDirty()`,
  `saveSettings(...)`, and fires a `setting_changed` analytics event.
  For a checkbox, `el.value` is always the literal `"on"`; the real
  state lives on `el.checked`. The parse fn needs to read
  `el.checked`, not `el.value` — or the wiring code needs to branch
  on `el.type === 'checkbox'`.
- `static/app.js:339-351` — `updateTuningDirty()` compares
  `currentSettings` to `makeDefaults()` for every TUNING_SPEC field
  and toggles the existing `tuningDirtyDot` element in the section
  header. This is the *panel-level* indicator and is already wired.
  It is **not** the file-list indicator the AC asks for.

### File list rendering

- `static/app.js:2000-2069` — `renderFileList()` builds a `.file-item`
  per file. It already shows a `✓` `accept-mark` for accepted assets.
  It does **not** currently render anything based on
  `has_saved_settings` or any "dirty vs defaults" signal. This is the
  insertion point for the new visual indicator.

### Style hooks

- `static/style.css:646-659` — `.dirty-dot` and `.dirty-dot.dirty`
  already exist. They're scoped purely by class, so the same dot
  styling can be reused inside a file-item if we want a consistent
  look between the panel header and the file list.
- `static/style.css:109-225` — file-item rules. There is already
  a `.accept-mark` style precedent in this file for tiny inline
  status glyphs.

## Analytics

- `static/app.js:295-326` — the auto-instrumentation in `wireTuningUI`
  fires `setting_changed` for every spec entry on every input event.
  Because both new fields (`slice_distribution_mode`,
  `ground_align`) are already enrolled in `TUNING_SPEC`, the
  analytics half of the AC ("Changing any control triggers a
  `setting_changed` analytics event") is satisfied for free the
  moment the DOM elements exist. No new logEvent calls needed.

## Defaults question

The AC says: *"Default values produce a noticeably better-shaded bake
than current — leaf-to-leaf contrast visible without going pitch black
on the underside."*

The current `DefaultSettings()` values were already the result of
T-002-01's hand-tuning pass and represent the "better than the
original baseline" state. T-005-01 did not touch them. Whether the
*current* defaults are good *enough* is a visual call that requires
running a bake, which the agent cannot do. The defaults question is
flagged here as a research finding; the design phase will decide
whether to ship a defaults shift or defer that to operator
verification.

## File-list "dirty vs defaults" indicator — current state

There is no existing field on `FileRecord` that says "the saved
settings differ from defaults". `HasSavedSettings` is the closest
proxy but it's a strict superset — an asset can have a settings file
that happens to match defaults (e.g. because the user reset to
defaults after experimenting; the PUT still writes the file). The
ticket explicitly asks the indicator to show "when settings differ
from defaults", so `HasSavedSettings` alone is not sufficient.

Two viable signals exist:
1. **Server computes a `SettingsDirty bool`** by comparing the loaded
   settings to `DefaultSettings()` at scan time and after every PUT,
   and ships it on `FileRecord` next to `HasSavedSettings`.
2. **Client computes it lazily** by reading the asset's settings on
   demand. Expensive — requires fetching `/api/settings/{id}` for
   every file in the list — and racy if the server pushes nothing.

Option 1 is cheaper and matches the existing pattern for
`HasSavedSettings`. Design will pick.

## Constraints / boundaries

- The tuning UI lives entirely in the right panel; do not touch the
  left or center panels except to add the file-list dot in
  `renderFileList()`.
- T-005-01 already reserved the JS-side hooks for the two missing
  controls. The clean shape of this ticket is "wire the DOM, no JS
  spec edits" plus the file-list indicator. Avoid expanding scope into
  presets / live re-render / tooltips per the explicit Out-of-Scope.
- The `Validate()` server ranges are *wider* than the slider ranges
  the AC asks for. This is fine — clamping is enforced by the input
  element, and out-of-range writes are still validated server-side.
  Do not narrow `Validate()`.
- `populateTuningUI`'s `el.value = ...` assumption breaks for
  checkboxes; this is the one non-trivial bit of plumbing this
  ticket has to fix.
- All settings PUTs are debounced 300ms in `saveSettings()`
  (app.js:108). The new controls inherit this for free.
