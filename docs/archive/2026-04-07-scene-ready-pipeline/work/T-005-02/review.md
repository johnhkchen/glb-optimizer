# Review — T-005-02: bake-quality-control-surface

## What changed

### Files modified

| File | Change |
|---|---|
| `settings.go` | +`SettingsDifferFromDefaults(s *AssetSettings) bool` — explicit field-by-field comparison against `DefaultSettings()`, deliberately excluding `SchemaVersion`. Enumerated rather than `reflect.DeepEqual` so adding a new field forces a compile-time visit. |
| `settings_test.go` | +`TestSettingsDifferFromDefaults` with three subtests: `defaults_match`, `single_field_mutated` (key_light, ground_align, slice_mode), `schema_version_change_ignored`. |
| `models.go` | +`SettingsDirty bool` (`json:"settings_dirty,omitempty"`) on `FileRecord`, next to `HasSavedSettings`. |
| `main.go` | In `scanExistingFiles`, when `HasSavedSettings` is true, also `LoadSettings` and assign `record.SettingsDirty = SettingsDifferFromDefaults(s)`. Errors swallowed. |
| `handlers.go` | In the settings PUT handler, compute `dirty := SettingsDifferFromDefaults(&s)` after validation, then set both `HasSavedSettings = true` and `SettingsDirty = dirty` inside the same `store.Update` callback. |
| `static/index.html` | +Two `setting-row` blocks inside `#tuningSection`, just above the reset-button row: a `<select id="tuneSliceDistributionMode">` (visual-density / vertex-quantile / equal-height) and a `<input type="checkbox" id="tuneGroundAlign">`. |
| `static/app.js` | `populateTuningUI`: branch on `el.type === 'checkbox'` so booleans use `el.checked = !!v` instead of `el.value = v`. `wireTuningUI`: read `el.checked` instead of `el.value` for checkbox controls. `renderFileList`: render a `<span class="settings-dirty-mark">●</span>` next to the filename when `f.settings_dirty` is true. No edit to `TUNING_SPEC`. |
| `static/style.css` | +`.file-item .settings-dirty-mark` rule (small accent-colored glyph beside the filename). |

No new files (other than RDSPI artifacts under
`docs/active/work/T-005-02/`). No deletions.

## Acceptance-criteria mapping

| AC bullet | Status | Notes |
|---|---|---|
| `bake_exposure` slider 0.5–2.5 | ✅ already present (T-002-03) | confirmed wired via `TUNING_SPEC` |
| `ambient_intensity` slider 0–2 | ✅ already present | |
| `hemisphere_intensity` slider 0–2 | ✅ already present | |
| `key_light_intensity` slider 0–3 | ✅ already present | |
| `bottom_fill_intensity` slider 0–1.5 | ✅ already present | |
| `env_map_intensity` slider 0–3 | ✅ already present | |
| `slice_distribution_mode` dropdown | ✅ added | `<select id="tuneSliceDistributionMode">` with the three legal modes from `validSliceDistributionModes` |
| `dome_height_factor` slider 0–1 | ✅ already present (T-005-01) | |
| `ground_align` checkbox | ✅ added | `<input type="checkbox" id="tuneGroundAlign">`, requires the new checkbox branch in `populateTuningUI` / `wireTuningUI` |
| Defaults produce noticeably better-shaded bake than current | ⚠️ deferred | The current `DefaultSettings()` already encodes T-002-01's hand-tuned values. Shifting them without the ability to actually run a bake is guesswork that could silently regress every default user. **Reviewer call** — see "Open concerns" #1. |
| One-line label, current value visible | ✅ | matches existing row pattern; the dropdown and checkbox use label-only (no `range-value` span) per the existing precedents (`lighting_preset`, `aggressiveSimplify`) |
| `setting_changed` analytics on every change | ✅ | Both new fields were enrolled in `TUNING_SPEC` by T-005-01 with reserved DOM ids; the auto-instrumentation in `wireTuningUI` now picks them up the moment the elements exist. Confirmed by reading the wiring; not exercised end-to-end by the agent. |
| Visual indicator on the asset (file list) when settings differ from defaults | ✅ | Server-side `settings_dirty` flag populated by both write paths (scan + PUT); rendered as a small accent dot in `renderFileList()`. |
| Manual: rose verification | ❌ deferred (operator) | Requires a browser session with a live asset |

## Test coverage

| Layer | What runs | Status |
|---|---|---|
| Go unit | `TestSettingsDifferFromDefaults` (4 leaf cases) | ✅ green |
| Go unit | All existing settings tests (defaults, validate, round-trip, migration paths) | ✅ green |
| Go build | `go build ./...` | ✅ clean |
| JS unit | None — project still has no JS test runner | ⚠️ untested |
| JS syntax | `node -c static/app.js` | ✅ clean |
| Manual integration | settings PUT round-trip, file-list refresh, control toggling | ❌ deferred (operator) |

### Coverage gaps

- **No automated test exercises the new HTTP path that sets
  `SettingsDirty` on PUT.** The handler test surface is thin in this
  repo generally; adding a focused HTTP test is doable but
  out-of-pattern. A cheap follow-up if this regresses: spin up a
  test `httptest.Server`, PUT a non-default body, GET `/api/files`,
  assert `settings_dirty: true` on the matching record.
- **No JS test exercises the checkbox-branch in `populateTuningUI`
  / `wireTuningUI`.** Same constraint as every prior tuning ticket
  — there is no JS test runner. Verified by reading.
- **No automated test confirms `scanExistingFiles` swallows a
  corrupt settings file gracefully.** The error path is one line and
  visually obvious, but unverified.

## Open concerns / TODOs

1. **Defaults shift was deferred.** The AC says "Default values
   produce a noticeably better-shaded bake than current — leaf-to-leaf
   contrast visible without going pitch black on the underside."
   Today's `DefaultSettings()` (`KeyLightIntensity: 1.4`,
   `HemisphereIntensity: 1.0`, `BottomFillIntensity: 0.4`,
   `EnvMapIntensity: 1.2`, etc.) is already T-002-01's tuned pass.
   I did not shift them because I cannot actually run a bake to
   judge "noticeably better". If the operator decides current
   defaults are still too dark, the change is a one-line edit in
   two synced places (`settings.go` `DefaultSettings()` and
   `static/app.js` `makeDefaults()`). **Flag for reviewer** — the
   AC reads as a directive but the verification it requires is
   visual.

2. **Manual rose verification not performed.** The ticket's last AC
   bullet asks for an end-to-end pass: load the rose, bump
   `key_light_intensity` to 2.0 and `dome_height_factor` to 0.7,
   regenerate production asset, see visible improvement. This is
   an operator step. The wiring needed to make those tweaks
   discoverable in the UI is in place.

3. **File-list dirty marker only refreshes when `renderFileList()`
   is called.** The list is not auto-refreshed on every settings
   PUT; the next natural refresh trigger (selection change, drop,
   delete, processing-completion poll) will pick up the change.
   This is the existing pattern for `HasSavedSettings` rendering
   and matches the cost/benefit of debounced PUTs. If a reviewer
   wants instant feedback, the cheapest fix is calling
   `renderFileList()` from inside `saveSettings`'s success branch,
   *after* updating `files[i].settings_dirty` locally — but that
   couples client-side state to server-side dirty computation in a
   way that introduces a small inconsistency window. Left as-is.

4. **`SettingsDifferFromDefaults` enumerates fields explicitly.**
   This is intentional (compile-time forcing function for new
   fields) but is also a footgun for refactors that rename a field
   without revisiting this helper. Mitigation: one of the test
   cases (`single_field_mutated`) covers a representative sample
   from each type (float, bool, enum string), so a missed field
   would at least *break* if the renamed field happened to be
   covered. A stronger guarantee would require generated code or
   reflection — not worth it.

5. **`schema_version` exclusion from the dirty comparison.** I
   chose to skip it so that loading a future schema version does
   not falsely flag every asset. Today this is purely
   forward-looking — every loaded settings file has
   `SchemaVersion=1`, so the bug would not manifest. The test case
   `schema_version_change_ignored` documents the intent.

6. **The settings PUT handler now computes
   `SettingsDifferFromDefaults` on every save.** This is a 13-field
   memory comparison and is essentially free. No perf concern.

## Files for the reviewer to read first

1. `settings.go` — the `SettingsDifferFromDefaults` helper. The
   exclusion of `SchemaVersion` is the only subtle bit.
2. `static/app.js` — the two-line branch in `populateTuningUI` and
   the matching branch in `wireTuningUI`. This is the only piece
   of the JS plumbing where a future bug could go unnoticed (the
   project has no JS test runner).
3. `handlers.go` and `main.go` — confirm both write paths populate
   `SettingsDirty` and that the scan-side error path is benign.
4. `static/index.html` — confirm the two new rows sit inside
   `#tuningSection` and use the reserved DOM ids.

## Out-of-scope items NOT touched (per ticket)

- Color calibration mode (T-005-03).
- Lighting presets (S-007).
- Live re-render on slider drag.
- Reset-to-defaults UI (already covered in T-002-03).
- `Validate()` ranges in `settings.go` (server ranges remain wider
  than the slider ranges, which is fine).
