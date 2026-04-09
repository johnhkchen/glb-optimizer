# Structure — T-005-02: bake-quality-control-surface

## File-level changes

| File | Change | Summary |
|---|---|---|
| `static/index.html` | MODIFY | Add two `setting-row` blocks inside `#tuningSection`: a `<select id="tuneSliceDistributionMode">` and a `<input type="checkbox" id="tuneGroundAlign">`. No other DOM edits. |
| `static/app.js` | MODIFY | Branch on `el.type === 'checkbox'` in `populateTuningUI` and `wireTuningUI` to handle boolean controls. Add file-list indicator rendering in `renderFileList()` when `f.settings_dirty` is true. |
| `static/style.css` | MODIFY | Add a `.file-item .settings-dirty-mark` rule (small inline glyph next to the filename). |
| `settings.go` | MODIFY | Add `SettingsDifferFromDefaults(s *AssetSettings) bool` helper. |
| `settings_test.go` | MODIFY | Add `TestSettingsDifferFromDefaults` covering: defaults → false, mutated single field → true, schema_version-only divergence → false. |
| `models.go` | MODIFY | Add `SettingsDirty bool \`json:"settings_dirty,omitempty"\`` to `FileRecord`. |
| `main.go` | MODIFY | In `scanExistingFiles`, after `record.HasSavedSettings = SettingsExist(...)`, when true also `LoadSettings` and set `record.SettingsDirty = SettingsDifferFromDefaults(s)`. |
| `handlers.go` | MODIFY | In the settings PUT handler, after validation and before responding, set `r.SettingsDirty = SettingsDifferFromDefaults(&s)` inside the same `store.Update` callback that flips `HasSavedSettings`. |

No new files. No deletions.

## DOM additions (`static/index.html`)

Inserted between the existing dome-height row (lines 238-243) area and
the bake-exposure row, OR at the bottom of the tuning section above
the reset button. Bottom placement keeps the existing slider stack
contiguous and avoids reflowing the visual order users are already
accustomed to. Final placement: just above the reset row.

```html
<div class="setting-row">
    <label>Slice distribution mode</label>
    <select id="tuneSliceDistributionMode">
        <option value="visual-density">visual-density</option>
        <option value="vertex-quantile">vertex-quantile</option>
        <option value="equal-height">equal-height</option>
    </select>
</div>

<div class="setting-row">
    <div class="checkbox-row">
        <input type="checkbox" id="tuneGroundAlign">
        <label for="tuneGroundAlign">Ground align bottom slice</label>
    </div>
</div>
```

The slice-mode row matches the existing `lighting_preset` row's
shape (label + select). The ground-align row matches the existing
`aggressiveSimplify` row's shape (checkbox-row container).

## JS edits (`static/app.js`)

### `populateTuningUI` — add checkbox branch

```js
function populateTuningUI() {
    if (!currentSettings) return;
    for (const spec of TUNING_SPEC) {
        const el = document.getElementById(spec.id);
        if (!el) continue;
        const v = currentSettings[spec.field];
        if (el.type === 'checkbox') {
            el.checked = !!v;
        } else {
            el.value = v;
        }
        const valEl = document.getElementById(spec.id + 'Value');
        if (valEl) valEl.textContent = spec.fmt(v);
    }
    updateTuningDirty();
}
```

### `wireTuningUI` — branch on event source for checkboxes

```js
el.addEventListener('input', () => {
    if (!currentSettings || !selectedFileId) return;
    const oldValue = currentSettings[spec.field];
    const raw = el.type === 'checkbox' ? el.checked : el.value;
    const v = spec.parse(raw);
    if (v === oldValue) return;
    /* …existing body unchanged… */
});
```

The `parse` field for `ground_align` is already
`v => v === true || v === 'true'`, which accepts the boolean handed
in from `el.checked` correctly.

Also: `change` is the more conventional event for checkboxes; the
existing code uses `input` which fires for checkboxes too, so no
edit there.

### `renderFileList` — add a dirty marker beside the filename

```js
const dirtyMark = f.settings_dirty
    ? '<span class="settings-dirty-mark" title="Tuned away from defaults">●</span>'
    : '';
div.innerHTML = `
    <div class="filename" title="${f.filename}">${f.filename}${dirtyMark}</div>
    <div class="file-meta">${metaHTML}</div>
    ...
`;
```

The marker sits inside the filename div so it tracks the filename's
hover and selection states.

## CSS edits (`static/style.css`)

```css
.file-item .settings-dirty-mark {
    color: var(--accent);
    font-size: 10px;
    margin-left: 4px;
    vertical-align: middle;
}
```

## Go edits

### `settings.go`

Append at the bottom:

```go
// SettingsDifferFromDefaults reports whether any user-facing field of
// s diverges from DefaultSettings(). SchemaVersion is intentionally
// excluded so that loading a future schema version does not falsely
// flag every asset as dirty.
func SettingsDifferFromDefaults(s *AssetSettings) bool {
    d := DefaultSettings()
    return s.VolumetricLayers != d.VolumetricLayers ||
        s.VolumetricResolution != d.VolumetricResolution ||
        s.DomeHeightFactor != d.DomeHeightFactor ||
        s.BakeExposure != d.BakeExposure ||
        s.AmbientIntensity != d.AmbientIntensity ||
        s.HemisphereIntensity != d.HemisphereIntensity ||
        s.KeyLightIntensity != d.KeyLightIntensity ||
        s.BottomFillIntensity != d.BottomFillIntensity ||
        s.EnvMapIntensity != d.EnvMapIntensity ||
        s.AlphaTest != d.AlphaTest ||
        s.LightingPreset != d.LightingPreset ||
        s.SliceDistributionMode != d.SliceDistributionMode ||
        s.GroundAlign != d.GroundAlign
}
```

This deliberately enumerates fields rather than using
`reflect.DeepEqual`, so adding a future field forces a compile-time
visit of this function.

### `models.go`

```go
SettingsDirty     bool       `json:"settings_dirty,omitempty"`
```

Inserted next to `HasSavedSettings`.

### `main.go` — `scanExistingFiles` patch

After the existing `record.HasSavedSettings = SettingsExist(id, settingsDir)`:

```go
if record.HasSavedSettings {
    if s, err := LoadSettings(id, settingsDir); err == nil {
        record.SettingsDirty = SettingsDifferFromDefaults(s)
    }
}
```

Errors are intentionally swallowed: a bad settings file should not
crash the scan, and falling through with `SettingsDirty=false` is
the right conservative default. (If `LoadSettings` succeeds, the
result is meaningful; if it fails, the user has bigger problems and
will see it the next time they touch the asset.)

### `handlers.go` — settings PUT patch

The existing `store.Update` callback near line 665 currently sets
`HasSavedSettings = true`. Extend it:

```go
store.Update(id, func(r *FileRecord) {
    r.HasSavedSettings = true
    r.SettingsDirty = SettingsDifferFromDefaults(&s)
})
```

`s` here is the validated `AssetSettings` already in scope from the
PUT decode + validate path.

## Ordering of changes

The changes fall into three independent islands:

1. **Backend dirty-flag plumbing** — `settings.go`, `settings_test.go`,
   `models.go`, `main.go`, `handlers.go`. Lands first because the
   `settings_dirty` field needs to be on the wire before the client
   can render it.
2. **HTML controls + JS plumbing** — `static/index.html`,
   `static/app.js` (`populateTuningUI` + `wireTuningUI`). Lands
   second.
3. **File-list indicator + CSS** — `static/app.js`
   (`renderFileList`), `static/style.css`. Lands third.

Within an island, changes are grouped into a single commit. Across
islands, three commits.

## Public interface impact

- New JSON field `settings_dirty` on `FileRecord` (`omitempty` —
  invisible to consumers when false). No breaking change.
- New Go function `SettingsDifferFromDefaults` in package `main` —
  internal-only.
- New DOM ids `tuneSliceDistributionMode` and `tuneGroundAlign` —
  already-reserved by `TUNING_SPEC`, no collision.
- New CSS class `.settings-dirty-mark` — namespaced under
  `.file-item`.

## What is NOT changed

- `Validate()` ranges in `settings.go`.
- `DefaultSettings()` values.
- The `TUNING_SPEC` array in `app.js` (already complete from T-005-01).
- `updateTuningDirty()` (works for booleans as-is).
- The settings PUT request/response shape.
- Any analytics envelope or consumer.
