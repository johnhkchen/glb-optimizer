# Structure — T-005-03: color-calibration-mode

## File-level changes

| File | Change | Summary |
|---|---|---|
| `settings.go` | MODIFY | Append `ColorCalibrationMode string` and `ReferenceImagePath string` to `AssetSettings`. Add `validColorCalibrationModes` enum map. Initialize defaults in `DefaultSettings()`. Validate the enum in `Validate()`. Add forward-compat normalization in `LoadSettings` (empty mode → `"none"`). Extend `SettingsDifferFromDefaults` with both fields. |
| `settings_test.go` | MODIFY | Add cases to `TestValidate_RejectsOutOfRange` for invalid enum values. Add a `TestLoadSettings_NormalizesColorCalibrationMode` test (decode JSON missing the key, assert it's `"none"`, validates clean). Extend `TestSettingsDifferFromDefaults` `single_field_mutated` with mode + path mutations. |
| `docs/knowledge/settings-schema.md` | MODIFY | Add two rows to the field table. Update the JSON example. |
| `static/index.html` | MODIFY | Add two `setting-row` blocks inside `#tuningSection`, just above the existing reset-button row: a `<select id="tuneColorCalibrationMode">` and a conditional `#referenceImageRow` containing an `Upload reference image` button. |
| `static/app.js` | MODIFY | Add `color_calibration_mode` to `TUNING_SPEC`. Mirror the two new defaults in `makeDefaults()`. Add `applyColorCalibration(id)` helper. Reorder `selectFile` so `loadSettings` runs before the calibration decision and gate `loadReferenceEnvironment` on the mode. Patch the `wireTuningUI` input handler to call `applyColorCalibration` and toggle `#referenceImageRow` visibility when the dropdown changes. Patch `populateTuningUI` to set `#referenceImageRow` visibility when the field is populated. Patch `uploadReferenceImage` to write `reference_image_path` to `currentSettings` and trigger `saveSettings`, and to gate `loadReferenceEnvironment` on the mode. Wire the new in-panel button to `referenceFileInput.click()`. |

No new files. No deletions.

## Schema additions

```go
// settings.go — append to AssetSettings AFTER GroundAlign so on-disk
// field order grows monotonically.
ColorCalibrationMode string `json:"color_calibration_mode"`
ReferenceImagePath   string `json:"reference_image_path,omitempty"`
```

```go
// settings.go — package-level enum table.
var validColorCalibrationModes = map[string]bool{
    "none":                 true,
    "from-reference-image": true,
}
```

```go
// settings.go — DefaultSettings() additions.
ColorCalibrationMode: "none",
ReferenceImagePath:   "",
```

```go
// settings.go — Validate() additions.
if !validColorCalibrationModes[s.ColorCalibrationMode] {
    return fmt.Errorf("color_calibration_mode %q is not a known mode", s.ColorCalibrationMode)
}
// reference_image_path: free string. Empty means "not set".
```

```go
// settings.go — LoadSettings normalization. Append after the
// ground_align *bool re-decode block.
if s.ColorCalibrationMode == "" {
    s.ColorCalibrationMode = "none"
}
```

```go
// settings.go — SettingsDifferFromDefaults additions (chained ||).
... ||
s.ColorCalibrationMode != d.ColorCalibrationMode ||
s.ReferenceImagePath != d.ReferenceImagePath
```

## Test additions

```go
// settings_test.go — extend TestValidate_RejectsOutOfRange cases.
{"empty calibration mode", func(s *AssetSettings) { s.ColorCalibrationMode = "" }},
{"unknown calibration mode", func(s *AssetSettings) { s.ColorCalibrationMode = "preset-x" }},
```

```go
// settings_test.go — new test.
func TestLoadSettings_NormalizesColorCalibrationMode(t *testing.T) {
    dir := t.TempDir()
    id := "legacy"
    // Hand-craft a JSON file from before T-005-03 — no
    // color_calibration_mode key.
    raw := []byte(`{
      "schema_version": 1,
      "volumetric_layers": 4,
      "volumetric_resolution": 512,
      "dome_height_factor": 0.5,
      "bake_exposure": 1.0,
      "ambient_intensity": 0.5,
      "hemisphere_intensity": 1.0,
      "key_light_intensity": 1.4,
      "bottom_fill_intensity": 0.4,
      "env_map_intensity": 1.2,
      "alpha_test": 0.10,
      "lighting_preset": "default",
      "slice_distribution_mode": "visual-density",
      "ground_align": true
    }`)
    if err := os.WriteFile(SettingsFilePath(id, dir), raw, 0644); err != nil {
        t.Fatalf("seed: %v", err)
    }
    s, err := LoadSettings(id, dir)
    if err != nil {
        t.Fatalf("LoadSettings: %v", err)
    }
    if s.ColorCalibrationMode != "none" {
        t.Errorf("ColorCalibrationMode = %q, want \"none\"", s.ColorCalibrationMode)
    }
    if err := s.Validate(); err != nil {
        t.Errorf("normalized doc failed validation: %v", err)
    }
}
```

The `TestSettingsDifferFromDefaults` `single_field_mutated` subtest
already enumerates representative cases — extend it (not a new test)
with `ColorCalibrationMode = "from-reference-image"` and
`ReferenceImagePath = "outputs/x_reference.png"` mutations.

## DOM additions (`static/index.html`)

Inserted just above the existing `tuneResetBtn` row inside
`#tuningSection`:

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

The conditional row uses inline `style="display:none"` so the very
first paint (before JS runs) doesn't flash the button. The walker
flips it on after `populateTuningUI` runs.

## JS edits (`static/app.js`)

### `makeDefaults` additions (line ~113)

```js
return {
    schema_version: 1,
    // ...existing fields...
    slice_distribution_mode: 'visual-density',
    ground_align: true,
    color_calibration_mode: 'none',
    reference_image_path: '',
};
```

### `TUNING_SPEC` addition (line ~262)

Append after the `ground_align` row:

```js
{ field: 'color_calibration_mode', id: 'tuneColorCalibrationMode',
  parse: v => v, fmt: v => v },
```

### `populateTuningUI` patch

After the existing for-loop, append a single line that syncs the
conditional row's visibility:

```js
syncReferenceImageRow();
```

### `wireTuningUI` patch (input handler tail)

Inside the input handler, after `logEvent('setting_changed', …)` and
before the closing `});`, add a side-effect for the calibration mode:

```js
if (spec.field === 'color_calibration_mode') {
    syncReferenceImageRow();
    applyColorCalibration(selectedFileId);
}
```

After the `TUNING_SPEC` for-loop, wire the in-panel button:

```js
const refBtn = document.getElementById('tuneReferenceImageBtn');
if (refBtn) {
    refBtn.addEventListener('click', () => {
        if (selectedFileId) referenceFileInput.click();
    });
}
```

### New helpers

```js
// Hide/show the in-panel reference-image upload row based on the
// current color_calibration_mode.
function syncReferenceImageRow() {
    const row = document.getElementById('referenceImageRow');
    if (!row || !currentSettings) return;
    row.style.display =
        currentSettings.color_calibration_mode === 'from-reference-image'
            ? '' : 'none';
}

// Apply (or tear down) reference-image color calibration to match the
// current settings. Idempotent.
function applyColorCalibration(id) {
    const file = files.find(f => f.id === id);
    const wantCalibration =
        currentSettings &&
        currentSettings.color_calibration_mode === 'from-reference-image' &&
        file && file.has_reference;
    if (wantCalibration) {
        loadReferenceEnvironment(id).then(() => {
            if (currentModel) {
                const url = `/api/preview/${id}?version=${previewVersion}&t=${Date.now()}`;
                loadModel(url, lastModelSize);
            }
        });
    } else {
        if (referenceEnvironment) { referenceEnvironment.dispose(); referenceEnvironment = null; }
        referencePalette = null;
        scene.environment = defaultEnvironment;
        resetSceneLights();
        if (currentModel) {
            const url = `/api/preview/${id}?version=${previewVersion}&t=${Date.now()}`;
            loadModel(url, lastModelSize);
        }
    }
}
```

### `selectFile` reorder (around line 2790)

Change the calibration branch from "auto-apply if has_reference" to
"defer until settings load, then ask the gate":

```js
if (file) {
    // Reset to neutral; applyColorCalibration() decides whether to
    // re-apply once settings have loaded.
    if (referenceEnvironment) { referenceEnvironment.dispose(); referenceEnvironment = null; }
    referencePalette = null;
    scene.environment = defaultEnvironment;
    resetSceneLights();
    loadSettings(id).then(async () => {
        // Mode-gated calibration. Runs the same loadReferenceEnvironment
        // path, but only when both has_reference and the mode are set.
        if (
            currentSettings.color_calibration_mode === 'from-reference-image' &&
            file.has_reference
        ) {
            await loadReferenceEnvironment(id);
        }
        await startAnalyticsSession(id);
        populateTuningUI();
        populateAcceptedUI(id);
        loadModel(`/api/preview/${id}?version=original&t=${Date.now()}`, file.original_size);
    });
}
```

### `uploadReferenceImage` patch

```js
async function uploadReferenceImage(id, file) {
    const formData = new FormData();
    formData.append('image', file);
    try {
        await fetch(`/api/upload-reference/${id}`, { method: 'POST', body: formData });
        const ext = file.name.toLowerCase().endsWith('.jpg') ? '.jpg' : '.png';
        store_update(id, f => { f.has_reference = true; f.reference_ext = ext; });
        // Persist the path tag in the asset's settings so the
        // calibration mode has a referent on disk.
        if (currentSettings && selectedFileId === id) {
            currentSettings.reference_image_path = `outputs/${id}_reference${ext}`;
            saveSettings(id);
        }
        // Only mutate the live scene when the user has actually opted
        // into reference-image calibration.
        if (currentSettings && currentSettings.color_calibration_mode === 'from-reference-image') {
            await loadReferenceEnvironment(id);
            const f = files.find(x => x.id === id);
            if (f && currentModel) {
                const url = `/api/preview/${id}?version=${previewVersion}&t=${Date.now()}`;
                loadModel(url, lastModelSize);
            }
        }
    } catch (err) {
        console.error('Reference image upload failed:', err);
    }
}
```

## Schema doc edits

Two new rows appended to the field table in
`docs/knowledge/settings-schema.md`:

```
| `color_calibration_mode` | string | `"none"` | `{"none","from-reference-image"}` | Color calibration source. `none` = neutral lighting (default). `from-reference-image` = use the per-asset reference image to derive a tinted environment + bake lights via the existing palette extraction (T-005-03). The full preset enum lands in S-007. |
| `reference_image_path`   | string | `""`     | free string                       | Optional tag pointing to the asset's reference image on disk (e.g. `outputs/{id}_reference.png`). Set automatically by the client after a successful upload to `/api/upload-reference/:id`. Not dereferenced server-side; the image is served by `/api/reference/:id`. |
```

JSON example gains the two trailing keys (in declaration order):

```
"color_calibration_mode": "none",
```

`reference_image_path` is `omitempty`, so it's only present when set.

## Public interface impact

- New JSON fields `color_calibration_mode` and `reference_image_path`
  on the settings document. **No** new endpoints. **No** breaking
  change — existing files load via normalization.
- New Go enum table `validColorCalibrationModes` (package `main`).
- New DOM ids `tuneColorCalibrationMode`, `referenceImageRow`,
  `tuneReferenceImageBtn`. None collide with existing ids.
- New JS helpers `syncReferenceImageRow`, `applyColorCalibration`.

## Ordering of changes

Three commits, each leaving the tree buildable and tests green:

1. **Backend schema + tests + schema doc.** `settings.go`,
   `settings_test.go`, `docs/knowledge/settings-schema.md`. Lands first
   so the wire format is in place before the client tries to PUT it.
2. **HTML + JS plumbing.** `static/index.html`, `static/app.js` — the
   tuning panel control, the helpers, and the gate edits in
   `selectFile` / `uploadReferenceImage` / `wireTuningUI`. One commit
   so the JS is never half-wired.
3. *(no third commit needed; everything fits in two atoms)*

## What is NOT changed

- `lighting_preset` field, `validLightingPresets` map.
- `extractPalette`, `buildSyntheticEnvironment`, `applyReferenceTint`,
  `loadReferenceEnvironment` — calibration internals are untouched.
- The toolbar `#uploadReferenceBtn` — left in place; S-007 will
  collapse it.
- `handleUploadReference` and `handleReferenceImage` server endpoints.
- `scanExistingFiles` — does not gain a `HasReference` restore (a
  pre-existing gap, out of scope here).
- `SchemaVersion` — stays at `1`.
- `updatePreviewButtons` toolbar logic (the existing button still
  reflects `file.has_reference` independently).
