# Structure — T-006-02

## Files touched

| File | Action | Lines (approx) |
|---|---|---|
| `static/app.js` | modify | +120 / -40 |
| `static/index.html` | modify | +12 / -5 |
| `static/style.css` | modify | +10 / 0 |
| `settings.go` | modify | +35 / -2 |
| `settings_test.go` | modify | +60 / 0 |
| `analytics.go` | modify | +1 / 0 |
| `docs/knowledge/analytics-schema.md` | modify | +20 / 0 |

No new files. No deletions.

## `static/app.js` changes

### Section: `// ── Scene Templates (T-006-01) ──` (lines 3002-3193)

**Replace** the registry contents (lines 3122-3171) with the
five new templates. Keep all helpers (`makeInstanceSpec`,
`scatterRandomly`, `scatterInRow`, `applyVariation`,
`applyOrientationRule`, `boundsFromSpecs`) unchanged.

New registry:

```js
const SCENE_TEMPLATES = {
  grid:         { id, name: 'Grid (benchmark)', generate },
  'hedge-row':  { id, name: 'Hedge Row',        generate },
  'mixed-bed':  { id, name: 'Mixed Bed',        generate },
  'rock-garden':{ id, name: 'Rock Garden',      generate },
  container:    { id, name: 'Container',        generate },
};
```

`grid.generate` is the body that was `benchmark.generate`.
`debug-scatter` is removed entirely. `activeSceneTemplate`
default changes from `'benchmark'` to `'grid'` (line 3173).

The placement-helper section (3195-3494) is unchanged; the
templates already produce the same `InstanceSpec[]` shape and
the helpers already accept it.

### DOM ref additions (insert near lines 39-72)

```js
const sceneTemplateSelect = document.getElementById('sceneTemplateSelect');
const sceneInstanceCount  = document.getElementById('sceneInstanceCount');
const sceneGroundToggle   = document.getElementById('sceneGroundToggle');
```

### `initThreeJS` (line 2783)

Add ground plane creation just below the `GridHelper` line
(2820):

```js
const groundGeom = new THREE.PlaneGeometry(100, 100);
const groundMat = new THREE.MeshStandardMaterial({
  color: 0x6b5544, roughness: 0.95, metalness: 0,
});
groundPlane = new THREE.Mesh(groundGeom, groundMat);
groundPlane.rotation.x = -Math.PI / 2;
groundPlane.position.y = 0;
groundPlane.visible = false;
groundPlane.frustumCulled = false;
scene.add(groundPlane);
```

`groundPlane` is a new module-level `let` near the top of the
file (insert after line 36).

### `populateScenePreviewUI()` — new helper

Inserted near `populateTuningUI` (which is invoked from
`selectFile`). Reads `currentSettings.scene_*` fields, writes
them into the three new DOM controls, calls `setSceneTemplate`
and `groundPlane.visible = ...`.

```js
function populateScenePreviewUI() {
  if (!currentSettings) return;
  const tplId = currentSettings.scene_template_id || 'grid';
  const count = currentSettings.scene_instance_count || 100;
  const ground = !!currentSettings.scene_ground_plane;
  if (sceneTemplateSelect) sceneTemplateSelect.value = tplId;
  if (sceneInstanceCount)  sceneInstanceCount.value  = count;
  if (sceneGroundToggle)   sceneGroundToggle.checked = ground;
  setSceneTemplate(tplId);
  if (groundPlane) groundPlane.visible = ground;
}
```

Call sites:
- `selectFile`: after `loadSettings(id).then(...)` resolves,
  inside the chain that already calls `populateTuningUI()`
  (around `static/app.js:3766`).
- `applyDefaults`: at the end, so cold-start state populates
  the controls.

### `populateScenePreviewSelect()` — new boot helper

Populates the `<select>` with `<option>` elements from
`SCENE_TEMPLATES` so adding a template doesn't require HTML
changes. Called once from the init block (~line 4080).

```js
function populateScenePreviewSelect() {
  if (!sceneTemplateSelect) return;
  sceneTemplateSelect.innerHTML = '';
  for (const id of Object.keys(SCENE_TEMPLATES)) {
    const opt = document.createElement('option');
    opt.value = id;
    opt.textContent = SCENE_TEMPLATES[id].name;
    sceneTemplateSelect.appendChild(opt);
  }
}
```

### Event listeners (insert in the Event Listeners section, ~3922+)

Replace the existing stress-test wiring block (lines 4049-4077)
with:

```js
const stressBtn = document.getElementById('stressBtn');

sceneTemplateSelect.addEventListener('change', () => {
  const from = getActiveSceneTemplate();
  const to = sceneTemplateSelect.value;
  if (from === to) return;
  setSceneTemplate(to);
  if (currentSettings) {
    currentSettings.scene_template_id = to;
    if (selectedFileId) saveSettings(selectedFileId);
  }
  logEvent('scene_template_selected', {
    from, to,
    instance_count: parseInt(sceneInstanceCount.value, 10) || 0,
    ground_plane: sceneGroundToggle.checked,
  }, selectedFileId);
});

sceneInstanceCount.addEventListener('change', () => {
  const n = Math.max(1, Math.min(500, parseInt(sceneInstanceCount.value, 10) || 1));
  sceneInstanceCount.value = n;
  if (currentSettings) {
    currentSettings.scene_instance_count = n;
    if (selectedFileId) saveSettings(selectedFileId);
  }
});

sceneGroundToggle.addEventListener('change', () => {
  const on = sceneGroundToggle.checked;
  if (groundPlane) groundPlane.visible = on;
  if (currentSettings) {
    currentSettings.scene_ground_plane = on;
    if (selectedFileId) saveSettings(selectedFileId);
  }
});

stressBtn.addEventListener('click', () => {
  const count = parseInt(sceneInstanceCount.value, 10) || 1;
  if (count <= 1) {
    clearStressInstances();
    if (currentModel) { currentModel.position.set(0, 0, 0); frameCamera(currentModel); }
  } else {
    const quality = parseInt(lodQualitySlider.value, 10) / 100;
    runStressTest(count, stressUseLods.checked, quality);
  }
});
```

The `stressUseLods` / `lodQualitySlider` block stays as-is
(LOD checkbox + quality slider remain).

### `clearStressInstances` (line 2885)

Remove the two lines that reset `stressCount` (the slider is
gone):

```js
- document.getElementById('stressCount').value = 1;
- document.getElementById('stressCountValue').textContent = '1x';
```

### `makeDefaults()` (line 120)

Add three new keys:

```js
scene_template_id: 'grid',
scene_instance_count: 100,
scene_ground_plane: false,
```

### Init block (~line 4079)

Add `populateScenePreviewSelect();` and call
`populateScenePreviewUI()` from inside `applyDefaults()` so
boot-time state is populated.

## `static/index.html` changes

### `.stress-controls` (lines 65-74)

**Remove** the `stressCount` range slider, its value span, and
the orphaned `stressLabel` for "Count:". **Add** the picker,
number input, and ground toggle. Final structure:

```html
<div class="stress-controls">
    <label class="stress-label">Scene:</label>
    <select id="sceneTemplateSelect" class="scene-select"></select>
    <label class="stress-label">Count:</label>
    <input type="number" id="sceneInstanceCount" min="1" max="500"
           value="100" class="scene-count">
    <label class="stress-label">
        <input type="checkbox" id="sceneGroundToggle"> Ground
    </label>
    <label class="stress-label">
        <input type="checkbox" id="stressUseLods"> LOD
    </label>
    <label class="stress-label" id="lodQualityLabel" style="display:none">Quality:</label>
    <input type="range" id="lodQuality" min="0" max="100" value="50"
           class="stress-slider" style="display:none;width:60px"
           title="Higher = more high-detail instances">
    <span id="lodQualityValue" class="stress-value" style="display:none">50%</span>
    <button class="wireframe-toggle" id="stressBtn">Run scene</button>
</div>
```

`Run` button text changes to `Run scene` per the AC ("'Run
scene' button").

## `static/style.css` changes

### Add (after `.stress-value`, line 435)

```css
.scene-select {
    background: var(--bg-2);
    color: var(--text);
    border: 1px solid var(--border);
    border-radius: 4px;
    padding: 2px 4px;
    font-size: 11px;
    max-width: 130px;
}

.scene-count {
    width: 56px;
    background: var(--bg-2);
    color: var(--text);
    border: 1px solid var(--border);
    border-radius: 4px;
    padding: 2px 4px;
    font-size: 11px;
}
```

CSS variable names match the existing palette
(`var(--bg-2)`, `var(--text)`, `var(--border)` are used
elsewhere in the file). If any are absent, fall back to
literal hex during implementation.

## `settings.go` changes

### `AssetSettings` struct (line 21)

Add three fields at the end (after `SliceAxis`):

```go
// SceneTemplateId is the active scene preview template id from the
// T-006-02 picker. Default "grid". Empty string on disk is normalized
// to "grid" at load time.
SceneTemplateId string `json:"scene_template_id,omitempty"`
// SceneInstanceCount is the per-asset instance count for the scene
// preview. Default 100. Zero on disk is normalized to 100 at load
// time.
SceneInstanceCount int `json:"scene_instance_count,omitempty"`
// SceneGroundPlane is whether the optional textured ground plane
// is shown in the scene preview. Default false.
SceneGroundPlane bool `json:"scene_ground_plane,omitempty"`
```

### `DefaultSettings()` (line 55)

Add three lines:

```go
SceneTemplateId:    "grid",
SceneInstanceCount: 100,
SceneGroundPlane:   false,
```

### Add `validSceneTemplates` (after `validSliceAxes`)

```go
var validSceneTemplates = map[string]bool{
    "grid":        true,
    "hedge-row":   true,
    "mixed-bed":   true,
    "rock-garden": true,
    "container":   true,
}
```

### `Validate()` (line 132)

Add three checks at the end:

```go
if !validSceneTemplates[s.SceneTemplateId] {
    return fmt.Errorf("scene_template_id %q is not a known template", s.SceneTemplateId)
}
if s.SceneInstanceCount < 1 || s.SceneInstanceCount > 500 {
    return fmt.Errorf("scene_instance_count out of range [1,500]: %d", s.SceneInstanceCount)
}
// SceneGroundPlane is bool; both values valid.
```

### `LoadSettings()` (line 238)

Add forward-compat normalization (mirroring the pattern at
`SliceAxis`):

```go
if s.SceneTemplateId == "" {
    s.SceneTemplateId = "grid"
}
if s.SceneInstanceCount == 0 {
    s.SceneInstanceCount = 100
}
// SceneGroundPlane: Go zero (false) is the migration default;
// no probe needed.
```

### `SettingsDifferFromDefaults()` (line 201)

Add three new comparison terms:

```go
s.SceneTemplateId != d.SceneTemplateId ||
s.SceneInstanceCount != d.SceneInstanceCount ||
s.SceneGroundPlane != d.SceneGroundPlane
```

## `settings_test.go` changes

Add tests mirroring the `_NewFields` and `_ShapeFields` patterns:

```go
func TestDefaultSettings_SceneFields(t *testing.T) {
    s := DefaultSettings()
    // Assert defaults: scene_template_id="grid",
    // scene_instance_count=100, scene_ground_plane=false.
}

func TestValidate_RejectsBadSceneTemplate(t *testing.T) { ... }

func TestValidate_AcceptsAllSceneTemplates(t *testing.T) { ... }

func TestValidate_RejectsSceneCountOutOfRange(t *testing.T) {
    // 0, -1, 501, 1000 → error.
}

func TestSettingsDifferFromDefaults_SceneFields(t *testing.T) {
    // Mutate each field individually, expect true.
}

func TestLoadSettings_NormalizesLegacySceneFields(t *testing.T) {
    // Write a JSON file missing the scene_* fields, load, expect
    // "grid" / 100 / false.
}
```

## `analytics.go` changes

### `validEventTypes` (line 25)

Add one line:

```go
"scene_template_selected": true, // T-006-02
```

## `docs/knowledge/analytics-schema.md` changes

Add a new event-type section near the existing
`classification_override` section:

```markdown
### `scene_template_selected`

Fired by the scene preview picker when the user changes the
active template. Payload:

| key | type | description |
|---|---|---|
| `from` | string | Previous template id (`grid`, `hedge-row`, ...) |
| `to` | string | New template id |
| `instance_count` | number | Snapshot of the count input at change time |
| `ground_plane` | bool | Snapshot of the ground toggle at change time |

Emitted only on actual change (`from !== to`). The instance
count and ground toggle do NOT emit their own events — they
ride along with the next `scene_template_selected` or are
captured in the `final_settings` of the next `session_end`.
```

## Ordering of changes (commits)

1. **Settings schema (Go-side)** — `settings.go` + `settings_test.go`
   additions. Self-contained, easy to review.
2. **Analytics allow-list** — `analytics.go` + analytics-schema.md.
3. **JS templates** — replace `SCENE_TEMPLATES` registry, rename
   `benchmark` → `grid`, delete `debug-scatter`. No UI change yet.
4. **HTML + CSS** — picker, count input, ground toggle, button
   rename. Still inert (no JS handlers).
5. **JS wiring** — DOM refs, `populateScenePreviewSelect`,
   `populateScenePreviewUI`, settings mirror, change handlers,
   ground plane init, hydration in `selectFile` /
   `applyDefaults`.
6. **Cleanup pass** — manual verification + minor polish.

Each commit leaves the app in a working state. Commit 5 is the
behavioral change; commits 1-4 are wiring + dead UI until then.

## Testing strategy

- **Go tests** — `go test ./...` covers `settings.go` changes via
  the new `settings_test.go` cases.
- **No new JS tests** — same constraint as T-006-01 (no JS test
  infra in repo). Manual verification gates the JS commits.
- **Manual verification (matches the AC):**
  1. Boot, select any asset. The picker reflects `grid` and the
     count input shows 100. Ground toggle is off.
  2. Switch to `mixed-bed`, set count 50, click Run. Expect ~50
     scattered instances with ±15% scale variation. Round-bush
     assets have random Y rotation; directional ones don't.
  3. Switch to `grid`, click Run. Expect the legacy 100x grid
     layout when count=100.
  4. Switch to `hedge-row`, set count 8, click Run. Expect a
     straight row of 8 along X.
  5. Switch to `container`, set count 100, click Run. Expect a
     tight cluster of 10 (clamped from 100 → 10).
  6. Toggle ground plane. Expect a brown plane to appear at Y=0
     under the instances.
  7. Switch to a second asset. The picker, count, and ground
     restore the second asset's saved values (or defaults).
     Switch back: the first asset's values are restored.
  8. Reload the page. The most-recently-selected asset's saved
     values survive.
  9. Open the JSONL log for the session. Expect a
     `scene_template_selected` event for each picker change,
     none for count or ground toggles alone.
- **Trellis test from T-004-05**: load a trellis (directional
  shape category), pick `hedge-row`, click Run. Expect a
  believable straight row of trellises all facing the same way.
