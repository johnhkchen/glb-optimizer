# Progress — T-005-02: bake-quality-control-surface

## Status: implementation complete, ready for review

All five planned commits landed in order. Build green, Go tests
green, JS syntax check clean. No deviations from plan.md.

## Steps

- [x] **Step 1 — Backend `SettingsDifferFromDefaults` helper + test**
  - `settings.go`: appended explicit field-by-field comparison helper.
  - `settings_test.go`: added `TestSettingsDifferFromDefaults` with
    `defaults_match`, `single_field_mutated` (3 sub-cases),
    `schema_version_change_ignored`.
  - `go test ./... -run TestSettingsDifferFromDefaults -v` → all green.
  - **Commit:** `db575c6 Add SettingsDifferFromDefaults helper (T-005-02)`

- [x] **Step 2 — Wire `SettingsDirty` onto `FileRecord` and the two write paths**
  - `models.go`: added `SettingsDirty bool` with
    `json:"settings_dirty,omitempty"`.
  - `main.go`: in `scanExistingFiles`, when `HasSavedSettings` is
    true, `LoadSettings` and assign
    `record.SettingsDirty = SettingsDifferFromDefaults(s)`. Errors
    swallowed (default false).
  - `handlers.go`: in the settings PUT handler, computed
    `dirty := SettingsDifferFromDefaults(&s)` outside the
    `store.Update` callback (to avoid the `s` shadow inside the
    `r *FileRecord` callback), then set both `HasSavedSettings` and
    `SettingsDirty` inside the callback.
  - `go build ./...` and `go test ./...` green.
  - **Commit:** `4b23491 Compute settings_dirty for FileRecord (T-005-02)`

- [x] **Step 3 — Add the two new tuning controls to the HTML**
  - `static/index.html`: inserted two `setting-row` blocks
    immediately above the reset-button row inside `#tuningSection`:
    a `<select id="tuneSliceDistributionMode">` with the three legal
    modes, and a `.checkbox-row` containing
    `<input type="checkbox" id="tuneGroundAlign">`.
  - **Commit:** `d202b68 Add slice mode + ground align tuning controls (T-005-02)`

- [x] **Step 4 — Teach `populateTuningUI` and `wireTuningUI` about checkboxes**
  - `static/app.js`: in `populateTuningUI`, branched on
    `el.type === 'checkbox'` to use `el.checked = !!v` instead of
    `el.value = v`.
  - `static/app.js`: in `wireTuningUI`'s input listener, computed
    `const raw = el.type === 'checkbox' ? el.checked : el.value;`
    and passed `raw` to `spec.parse`.
  - No edit to `TUNING_SPEC` (the `ground_align` parse fn already
    accepts a boolean directly).
  - `node -c static/app.js` clean.
  - **Commit:** `3a8ad20 Handle checkbox controls in tuning spec walker (T-005-02)`

- [x] **Step 5 — File-list dirty marker (JS + CSS)**
  - `static/app.js`: in `renderFileList()`, computed `dirtyMark`
    from `f.settings_dirty` and injected a
    `<span class="settings-dirty-mark">●</span>` into the filename
    div with a `title` of "Tuned away from defaults".
  - `static/style.css`: added `.file-item .settings-dirty-mark` rule.
  - `node -c static/app.js` clean.
  - **Commit:** `3329403 Render settings_dirty marker in file list (T-005-02)`

## Deviations from plan

None. Only a minor implementation detail: in Step 2 I hoisted the
`SettingsDifferFromDefaults` call out of the `store.Update`
callback to avoid shadowing the local `s AssetSettings` with the
callback parameter `r *FileRecord`. The structure.md sketch had
the call inside the callback; the structure of the variable
binding made the hoist cleaner.

## Verification done by the agent

- `go build ./...` clean.
- `go test ./...` green (all suites).
- `go test ./... -run TestSettingsDifferFromDefaults -v` green
  (4 leaf cases).
- `node -c static/app.js` clean after each JS edit.
- All five commits land on a green tree.

## Verification deferred to operator

- Visual inspection of the new dropdown + checkbox in the tuning
  panel.
- Round-trip: PUT a non-default body via the UI, observe the
  file-list dot appear after the next list refresh.
- Manual rose verification per the AC's last bullet (load rose,
  bump `key_light_intensity` to 2.0 and `dome_height_factor` to
  0.7, regenerate production asset, see visible improvement).
