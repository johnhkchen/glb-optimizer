# Plan — T-005-02: bake-quality-control-surface

## Step sequence

### Step 1 — Backend `SettingsDifferFromDefaults` helper + test

**Files:** `settings.go`, `settings_test.go`

**Goal:** Land a pure comparison helper with unit-test coverage so the
rest of the work can rely on it.

- Append `SettingsDifferFromDefaults(s *AssetSettings) bool` to
  `settings.go` (see structure.md for the explicit field-by-field
  body).
- Append `TestSettingsDifferFromDefaults` to `settings_test.go` with
  three subtests:
  - `defaults_match` — `SettingsDifferFromDefaults(DefaultSettings())`
    is `false`.
  - `single_field_mutated` — for each of 3 representative fields
    (a float, the bool, the enum string), mutate the field and
    assert `true`.
  - `schema_version_change_ignored` — bump `SchemaVersion` only and
    assert `false` (the helper deliberately ignores it).
- Run `go test ./...`. Must be green before continuing.

**Verification:** `go test ./... -run TestSettingsDifferFromDefaults`
green.

**Commit:** `Add SettingsDifferFromDefaults helper (T-005-02)`

### Step 2 — Wire `SettingsDirty` onto `FileRecord` and the two write paths

**Files:** `models.go`, `main.go`, `handlers.go`

**Goal:** Make every code path that flips `HasSavedSettings` also
populate `SettingsDirty`.

- Add `SettingsDirty bool \`json:"settings_dirty,omitempty"\`` to
  `FileRecord` next to `HasSavedSettings`.
- In `main.go` `scanExistingFiles`, after the existing
  `record.HasSavedSettings = SettingsExist(...)`, when true,
  `LoadSettings` and assign `record.SettingsDirty =
  SettingsDifferFromDefaults(s)`. Swallow the load error (default to
  `false`).
- In `handlers.go`'s settings PUT handler at line ~665, extend the
  `store.Update` callback to also set
  `r.SettingsDirty = SettingsDifferFromDefaults(&s)`.
- Run `go build ./...` and `go test ./...`. Both green.

**Verification:**
1. `go build ./...` clean.
2. Manual: start the server with an empty `~/.glb-optimizer/settings/`,
   upload a file, GET `/api/files`, expect `settings_dirty` absent
   (defaults). PUT a non-default settings body, GET again, expect
   `"settings_dirty": true`. Reset to defaults via PUT, GET again,
   expect `settings_dirty` absent.

**Commit:** `Compute settings_dirty for FileRecord (T-005-02)`

### Step 3 — Add the two new tuning controls to the HTML

**Files:** `static/index.html`

**Goal:** Surface `slice_distribution_mode` and `ground_align` in the
tuning panel using the reserved DOM ids.

- Inside `#tuningSection`, just before the existing reset-button
  row, add:
  - A `.setting-row` containing a label and
    `<select id="tuneSliceDistributionMode">` with three options:
    `visual-density`, `vertex-quantile`, `equal-height`.
  - A `.setting-row` containing a `.checkbox-row` with
    `<input type="checkbox" id="tuneGroundAlign">` and a label
    "Ground align bottom slice".
- Verify by reload only — JS plumbing for these ids already exists
  in `TUNING_SPEC`.

**Verification:**
1. Open the page, select an asset, watch the two new controls
   appear and reflect the asset's saved values (or defaults).
2. Change the dropdown — the panel-header dirty dot turns blue,
   debounced PUT lands, `setting_changed` analytics event fires
   (visible in devtools network or in the JSONL log).
3. Toggle the checkbox — same behavior. **Expected to fail** at
   this step because `populateTuningUI` does `el.value = bool` for
   checkboxes; this is fixed in Step 4.

**Commit:** `Add slice mode + ground align tuning controls (T-005-02)`

### Step 4 — Teach `populateTuningUI` and `wireTuningUI` about checkboxes

**Files:** `static/app.js`

**Goal:** Make the spec walker handle boolean inputs correctly.

- In `populateTuningUI`, branch on `el.type === 'checkbox'` and use
  `el.checked = !!v` instead of `el.value = v`.
- In `wireTuningUI`'s input handler, read
  `const raw = el.type === 'checkbox' ? el.checked : el.value;`
  and pass `raw` to `spec.parse`.
- No edit to `TUNING_SPEC`. No edit to `updateTuningDirty`.
- `node -c static/app.js` clean.

**Verification:**
1. Open an asset with a saved `ground_align: false` settings file
   (PUT one via curl first if needed). The checkbox renders
   unchecked.
2. Toggle the checkbox on — `currentSettings.ground_align`
   becomes `true`, debounced PUT writes, panel-header dirty dot
   updates correctly, `setting_changed` event fires with
   `old_value: false, new_value: true`.
3. Hit "Reset to defaults" — checkbox returns to `true` (the
   default).

**Commit:** `Handle checkbox controls in tuning spec walker (T-005-02)`

### Step 5 — File-list dirty indicator (JS + CSS)

**Files:** `static/app.js`, `static/style.css`

**Goal:** Render a small marker beside the filename when
`f.settings_dirty` is true.

- In `renderFileList()`, compute `dirtyMark` from `f.settings_dirty`
  and inject it into the filename div as a `<span
  class="settings-dirty-mark">●</span>` with a `title` of "Tuned
  away from defaults".
- In `style.css`, add the `.file-item .settings-dirty-mark` rule.

**Verification:**
1. Upload a fresh file → no marker.
2. Tweak any control → debounced PUT, then on the next
   `renderFileList()` call (selection change, completion poll,
   etc.) the marker appears.
   - **Note:** the file list does not currently auto-refresh on
     setting changes. This is intentional and out of scope. The
     marker will appear on the next natural list refresh (drop,
     delete, polling tick, selection change, processing
     completion). Document in review.
3. Reset to defaults → marker disappears on next refresh.

**Commit:** `Render settings_dirty marker in file list (T-005-02)`

## Testing strategy

| Layer | What runs | Where |
|---|---|---|
| Go unit | `TestSettingsDifferFromDefaults` (3 subtests) | `settings_test.go` (Step 1) |
| Go build | `go build ./...` | After Steps 1, 2 |
| JS syntax | `node -c static/app.js` | After Steps 4, 5 |
| Manual integration | settings PUT round-trip via curl + GET `/api/files` | After Step 2 |
| Manual UI | Toggle each new control, observe dirty-dot, observe setting_changed in network tab, observe file-list marker | After Steps 3, 4, 5 |
| Manual rose verification | Operator step recorded in review (cannot be done by agent) | After Step 5 |

There is no JS test runner in the project (per T-005-01 review),
so the JS-side controls are validated by manual exercise only. This
matches the testing posture of every prior tuning-UI ticket.

## Verification criteria (full ticket)

- [ ] `go test ./...` and `go build ./...` clean.
- [ ] `node -c static/app.js` clean.
- [ ] All AC slider controls present in the tuning panel (existing).
- [ ] New `slice_distribution_mode` dropdown wired and firing
      `setting_changed`.
- [ ] New `ground_align` checkbox wired and firing `setting_changed`.
- [ ] Panel-header dirty dot reflects all 13 fields including the
      two new ones.
- [ ] File-list marker appears when `settings_dirty: true` is on
      the wire.
- [ ] Every `store.Update` site that touches `HasSavedSettings`
      also touches `SettingsDirty`.
- [ ] No edits to `Validate()` ranges, `DefaultSettings()`, or
      `TUNING_SPEC`.

## Risks and mitigations

- **Risk:** A user PUTs a body that happens to match defaults — the
  marker stays off even though a settings file exists. **Mitigation:**
  This is the documented semantics; `HasSavedSettings` is the
  separate "file exists" signal if anything ever wants it.
- **Risk:** `LoadSettings` returning `nil, err` from a corrupt file
  during scan would propagate. **Mitigation:** Errors are swallowed
  in scan; `SettingsDirty` defaults to false.
- **Risk:** `renderFileList` is called from many places; the marker
  may flicker if a refresh races with a PUT. **Mitigation:** PUT is
  debounced 300ms and the file record is the source of truth on the
  next refresh — eventual consistency is acceptable.
- **Risk:** Adding HTML rows in the wrong place could disturb the
  reset-to-defaults button or the dirty dot. **Mitigation:** Insert
  immediately above the reset-button row, leaving every existing
  selector intact.

## Atomic commits

Five commits, in order:
1. `Add SettingsDifferFromDefaults helper (T-005-02)`
2. `Compute settings_dirty for FileRecord (T-005-02)`
3. `Add slice mode + ground align tuning controls (T-005-02)`
4. `Handle checkbox controls in tuning spec walker (T-005-02)`
5. `Render settings_dirty marker in file list (T-005-02)`

Each commit leaves the tree buildable and the test suite green.
