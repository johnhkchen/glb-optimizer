# Design — T-005-02: bake-quality-control-surface

## Decisions at a glance

| Decision | Choice | Why |
|---|---|---|
| Where to add slice-mode + ground-align controls | New rows in `index.html`'s existing `#tuningSection`, reusing reserved DOM ids `tuneSliceDistributionMode` / `tuneGroundAlign` | Zero JS spec churn — T-005-01 already enrolled them in `TUNING_SPEC` |
| How to render the slice-mode control | `<select>` with three `<option>`s mirroring `validSliceDistributionModes` | Matches `lighting_preset` row precedent and `populateTuningUI`'s value-set assumption |
| How to render the ground-align control | `<input type="checkbox">` inside an existing `.checkbox-row` | Matches the precedent set by `aggressiveSimplify`, `lockBorders`, etc. |
| How to teach the spec walker about checkboxes | Branch on `el.type === 'checkbox'` in `populateTuningUI` and `wireTuningUI` | One small surgical change in two functions; preserves the auto-instrumentation contract |
| Where to compute "settings differ from defaults" for the file list | Server-side, new `SettingsDirty bool` field on `FileRecord`, populated next to `HasSavedSettings` | Cheap, matches existing pattern, avoids per-file fetches from the client |
| How to render the file-list indicator | A small inline `•` glyph (or reused `.dirty-dot.dirty`) next to the filename when `settings_dirty` is true | Matches the `accept-mark` precedent in the same file list |
| Whether to shift `DefaultSettings()` values | **No** — leave defaults alone | The defaults already represent T-002-01's tuned pass; an unverified visual shift here is riskier than the AC's literal reading is worth. Document as operator-verifiable. |

## Options considered

### Option A — Two new rows in the tuning panel + server-side dirty flag (chosen)

**What it looks like:**
- `index.html` gains exactly two `setting-row` blocks inside
  `#tuningSection`: a `<select id="tuneSliceDistributionMode">` and
  a `<input type="checkbox" id="tuneGroundAlign">`.
- `app.js` makes `populateTuningUI` and `wireTuningUI` aware of
  `el.type === 'checkbox'`. The `parse` field on the
  `ground_align` `TUNING_SPEC` entry is left alone for the analytics
  payload, but the wire path reads `el.checked` directly.
- A new `SettingsDirty bool \`json:"settings_dirty,omitempty"\`` is
  added to `FileRecord`. A new helper `SettingsDifferFromDefaults`
  in `settings.go` compares a loaded `*AssetSettings` to
  `DefaultSettings()` field-by-field. Both `scanExistingFiles`
  (main.go) and the settings PUT handler (handlers.go) populate it.
- `renderFileList()` in app.js renders a small marker beside the
  filename when `f.settings_dirty` is true.

**Pros:**
- Smallest possible surface area: one HTML block, two JS branches,
  one Go field, one Go helper, one CSS class addition.
- Auto-analytics already covers the new controls.
- Server-side dirty flag is correct without any client-side
  per-asset round trips.
- Future tickets that want to *filter* by dirty assets can use the
  same field.

**Cons:**
- Slightly couples server to client rendering concern. Mitigation:
  the helper is a pure comparison and is testable in isolation.
- A user who PUTs default settings explicitly (e.g. after a reset)
  will see the file-list dot stay off — which is actually the
  desired semantics (they re-tuned to match defaults), but is worth
  noting in case anyone reads "dirty" as "has settings file at all".

### Option B — Client-side dirty computation by fetching `/api/settings/{id}` per file (rejected)

**Why rejected:** O(N) round trips on every file-list refresh, and
the existing list refresh is called from many places (drop, delete,
selection, polling). Cache-coherence work would dwarf the size of
the indicator.

### Option C — Reuse `HasSavedSettings` as a proxy for "dirty" (rejected)

**Why rejected:** Strictly wrong. A file may exist with default
contents (after a reset, after a profile apply that matches
defaults, after a no-op manual save). The AC asks for "differs from
defaults", not "has any saved file".

### Option D — Compute dirty flag from a hash stored in the settings file (rejected)

**Why rejected:** Adds a persisted column the schema doesn't need,
when a 13-field comparison against an in-memory struct is trivial
and authoritative.

### Option E — Bump default values to a more aggressive lighting profile (rejected for now)

**Why rejected:** The AC asks for "noticeably better than current",
but "current" is already T-002-01's tuned pass. Without the ability
to actually run a bake and eyeball the result, shifting numbers is
guessing. The cost of guessing wrong is silent regression for every
user with default settings. Documented as an operator follow-up in
the review. If a future operator decides the current defaults are
still too dark, the change is a one-line edit in two places
(`settings.go` `DefaultSettings`, `static/app.js` `makeDefaults`)
and trivially landable as a hotfix.

## How the chosen design satisfies each AC item

| AC bullet | How |
|---|---|
| `bake_exposure` slider 0.5–2.5 | Already present at index.html:246-250 — no change |
| `ambient_intensity` slider 0–2 | Already present at index.html:252-257 — no change |
| `hemisphere_intensity` slider 0–2 | Already present at index.html:259-264 |
| `key_light_intensity` slider 0–3 | Already present at index.html:266-271 |
| `bottom_fill_intensity` slider 0–1.5 | Already present at index.html:273-278 |
| `env_map_intensity` slider 0–3 | Already present at index.html:280-285 |
| `slice_distribution_mode` dropdown | NEW row, `<select>` with three options |
| `dome_height_factor` slider 0–1 | Already present at index.html:238-243 |
| `ground_align` checkbox | NEW row, `<input type="checkbox">` |
| Defaults produce noticeably better-shaded bake than current | Existing defaults already represent T-002-01's tuned pass; deferred to operator verification (see Option E rejection). Documented in review. |
| One-line label, current value visible | Matches existing row pattern (`.range-value` span for sliders, label text for select/checkbox) |
| `setting_changed` analytics on every change | Auto via `wireTuningUI` once DOM elements exist for all `TUNING_SPEC` entries |
| Visual indicator when settings differ from defaults (file list) | New `settings_dirty` field on `FileRecord`, rendered as a small glyph in `renderFileList()` |
| Manual rose verification | Operator step, recorded in review.md |

## Subtleties / risks

1. **Checkbox plumbing in TUNING_SPEC.** The current `populateTuningUI`
   does `el.value = currentSettings[field]`. For a `<select>`,
   setting `value` to a string that matches an `<option value=…>`
   selects that option — works for `slice_distribution_mode`. For a
   checkbox, `el.value` is the *form value*, not the checked state;
   `el.checked = bool` is what's needed. The minimal fix is a
   one-line branch in each of the two functions
   (`populateTuningUI` and `wireTuningUI`). The branch is keyed on
   `el.type === 'checkbox'`, which is a single check that does not
   ripple to any other spec entry.

2. **`updateTuningDirty()` already handles the comparison.** Its
   `currentSettings[spec.field] !== defs[spec.field]` works for
   booleans the same way it works for strings and numbers — no
   change needed there. Adding the `tuneGroundAlign` checkbox does
   not break the panel-header dot.

3. **The auto-analytics `setting_changed` event** records `old_value`
   and `new_value` from the parsed JS values. For `ground_align`
   that's a boolean, for `slice_distribution_mode` that's a string —
   both are valid JSON for the analytics envelope and require no
   changes to the consumer side.

4. **`SettingsDirty` recomputation timing.** Two write paths exist:
   the PUT handler in `handlers.go` and `scanExistingFiles` in
   `main.go`. The PUT handler currently sets
   `r.HasSavedSettings = true` unconditionally after a successful
   write. We need to also set `r.SettingsDirty =
   SettingsDifferFromDefaults(s)` *after* validation succeeds and
   *before* the response is written. Order matters because the PUT
   response is the file record (or callers refresh it via
   `/api/files`). Skipping recomputation means the file-list
   indicator lags by one refresh cycle.

5. **`SettingsDifferFromDefaults` and the schema_version field.**
   The comparison must skip `SchemaVersion` (an internal field) or
   it will report dirty=true on every loaded settings file with a
   future schema version. For v1 today, every loaded settings file
   has `SchemaVersion=1`, identical to the default, so the bug is
   latent. The helper will explicitly compare the user-facing fields
   instead of using `reflect.DeepEqual` to avoid future surprise.

6. **CSS reuse vs new class.** The `.dirty-dot` class assumes a
   transparent default that turns blue when `.dirty` is added. In a
   file-item context we want it always-on (only render when dirty)
   so the transparent base is wasted but harmless. Reusing avoids
   churn; if the visual reads poorly we can swap to a glyph in a
   one-line edit.

## Out of scope (re-affirming the ticket)

- No color-calibration mode (T-005-03).
- No lighting presets (S-007).
- No live re-render on slider drag — debounced PUT + manual
  regenerate is the loop.
- No new unit tests for the JS controls (no JS test runner exists);
  the new Go helper does get a unit test.
