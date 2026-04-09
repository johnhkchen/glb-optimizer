# Plan — T-002-02: Wire app.js bake constants to settings

## Step sequencing

Each step is small enough to commit independently, though for this ticket
we'll likely fold steps 1–6 into a single commit since they're a
coordinated rename and split commits would temporarily break the build.

### Step 1 — Flip the schema default for `alpha_test`

**File**: `settings.go`
**Change**: `AlphaTest: 0.15,` → `AlphaTest: 0.10,` in `DefaultSettings()`.
**Why**: design.md decision 2 — keep bake-export GLBs regression-free.
**Verify**: `go test ./...` still passes.

### Step 2 — Sync the schema doc

**File**: `docs/knowledge/settings-schema.md`
**Change**: `alpha_test` row in the defaults table from `0.15` → `0.10`.
**Why**: keep doc/code in sync.
**Verify**: visual diff of the row.

### Step 3 — Add `currentSettings` state

**File**: `static/app.js`
**Change**: insert two `let` declarations into the State block (after
`referencePalette`):

```js
let currentSettings = null;
let _saveSettingsTimer = null;
```

**Verify**: file still parses (browser load, or any quick syntax check).

### Step 4 — Add the asset settings helper block

**File**: `static/app.js`
**Change**: insert a new section directly before `function getSettings()`
(line ~72):

```js
// ── Asset Settings ──
// Per-asset bake/tuning settings. Loaded from /api/settings/:id when a
// file is selected. Bake/preview functions read from currentSettings
// directly. T-002-03 will wire UI sliders into saveSettings().

async function loadSettings(id) {
  try {
    const res = await fetch(`/api/settings/${id}`);
    if (!res.ok) throw new Error(`HTTP ${res.status}`);
    currentSettings = await res.json();
  } catch (err) {
    console.warn(`loadSettings(${id}) failed, using defaults:`, err);
    applyDefaults();
  }
  return currentSettings;
}

function saveSettings(id) {
  if (_saveSettingsTimer) clearTimeout(_saveSettingsTimer);
  _saveSettingsTimer = setTimeout(async () => {
    _saveSettingsTimer = null;
    if (!currentSettings) return;
    try {
      const res = await fetch(`/api/settings/${id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(currentSettings),
      });
      if (!res.ok) throw new Error(`HTTP ${res.status}`);
    } catch (err) {
      console.warn(`saveSettings(${id}) failed:`, err);
    }
  }, 500);
}

function applyDefaults() {
  // Mirror of DefaultSettings() in settings.go. Keep in sync by hand.
  currentSettings = {
    schema_version: 1,
    volumetric_layers: 4,
    volumetric_resolution: 512,
    dome_height_factor: 0.5,
    bake_exposure: 1.0,
    ambient_intensity: 0.5,
    hemisphere_intensity: 1.0,
    key_light_intensity: 1.4,
    bottom_fill_intensity: 0.4,
    env_map_intensity: 1.2,
    alpha_test: 0.10,
    lighting_preset: 'default',
  };
  return currentSettings;
}
```

**Verify**: load app in browser; no console errors at startup; the
helpers exist on `window` if exposed (they don't need to be).

### Step 5 — Replace literals in `setupBakeLights`

**File**: `static/app.js` (lines 359, 361, 363, 367)
- `AmbientLight(sky, 0.5)` → `AmbientLight(sky, currentSettings.ambient_intensity)`
- `HemisphereLight(sky, ground, 1.0)` → `HemisphereLight(sky, ground, currentSettings.hemisphere_intensity)`
- `DirectionalLight(sky, 1.4)` → `DirectionalLight(sky, currentSettings.key_light_intensity)`
- `DirectionalLight(fill, 0.4)` → `DirectionalLight(fill, currentSettings.bottom_fill_intensity)`

### Step 6 — Replace literals in `cloneModelForBake`

**File**: `static/app.js` (line 422)
- `c.envMapIntensity = 1.2;` → `c.envMapIntensity = currentSettings.env_map_intensity;`

### Step 7 — Replace literals in `renderBillboardAngle`

**File**: `static/app.js` (line 301)
- `offRenderer.toneMappingExposure = 1.0;` → `... = currentSettings.bake_exposure;`

### Step 8 — Replace literals in `renderBillboardTopDown`

**File**: `static/app.js` (line 442)
- Same exposure replacement.

### Step 9 — Replace literals in `renderMultiAngleBillboardGLB`

**File**: `static/app.js` (lines 491, 518)
- `alphaTest: 0.1,` → `alphaTest: currentSettings.alpha_test,` (×2)

### Step 10 — Replace literals in `renderLayerTopDown`

**File**: `static/app.js` (lines 580, 614, 615, 616)
- Exposure → `bake_exposure`
- Ambient → `ambient_intensity`
- Hemisphere → `hemisphere_intensity`
- DirectionalLight `1.6` → `key_light_intensity` (the documented +0.2 delta vanishes)

### Step 11 — Replace literals in `renderHorizontalLayerGLB`

**File**: `static/app.js` (lines 738, 745)
- `const domeHeight = layerThickness * 0.5;` → `... * currentSettings.dome_height_factor;`
- `alphaTest: 0.1,` → `alphaTest: currentSettings.alpha_test,`

### Step 12 — Delete `VOLUMETRIC_*` consts and rewrite call sites

**File**: `static/app.js` (lines 542–543, 552, 819)
- Delete the two `const VOLUMETRIC_*` lines.
- `generateVolumetric:552`: arguments become
  `currentSettings.volumetric_layers, currentSettings.volumetric_resolution`.
- `generateProductionAsset:819`: same swap.
- Verify `VOLUMETRIC_LOD_CONFIGS` (line 766) still works — it has its own
  literal values per LOD level and is **not** wired to settings (LOD chain
  is a different concept; out of scope per ticket).

### Step 13 — Rewrite `selectFile` to load settings before model

**File**: `static/app.js` (line 2076)

Before:
```js
loadEnv.then(() => {
  loadModel(`/api/preview/${id}?version=original&t=${Date.now()}`, file.original_size);
});
```

After:
```js
loadEnv.then(async () => {
  await loadSettings(id);
  loadModel(`/api/preview/${id}?version=original&t=${Date.now()}`, file.original_size);
});
```

### Step 14 — Run `go test ./...`

```bash
go test ./...
```

Expected: PASS. The only Go change is a default-value flip; existing
tests verify defaults validate and round-trip, both of which still hold.

### Step 15 — Manual smoke test (regression check)

This is the **acceptance criterion** the ticket cares most about.

```bash
# Start the server (clean state — no settings file for the test asset)
go build && ./glb-optimizer

# In another terminal: confirm GET returns defaults including new alpha_test
ID=$(curl -s http://localhost:8787/api/files | jq -r '.[0].id')
curl -s http://localhost:8787/api/settings/$ID | jq .alpha_test
# expected: 0.10

curl -s http://localhost:8787/api/settings/$ID | jq .key_light_intensity
# expected: 1.4
```

Then in the browser:

1. Upload `assets/rose.glb` (or load existing). Select it.
2. Open browser devtools console — confirm no errors from
   `loadSettings`.
3. Confirm `currentSettings` is populated:
   ```js
   currentSettings  // should be the full default object, not null
   ```
4. Click **Billboard** — should produce a billboard GLB. Visually
   compare against a screenshot from the pre-change main branch. Check
   foliage edges, brightness, alpha cutoff.
5. Click **Volumetric** — same comparison. Note: there will be a small
   global brightness drop in the volumetric pass because the +0.2 delta
   on `key_light_intensity` (1.6 → 1.4) is gone. Documented and
   acceptable per design.md.
6. Click **Production Asset** — should produce both billboard and
   volumetric.
7. Click another file (different id), then click back. Confirm
   `currentSettings` was re-fetched:
   ```js
   // Add a temp `console.log` in loadSettings if needed during smoke
   ```

### Step 16 — `applyDefaults()` regression sanity

In the console:

```js
applyDefaults();
saveSettings(selectedFileId);
// wait 600 ms
// then on disk: cat ~/.glb-optimizer/settings/<id>.json
// → should match defaults; alpha_test should be 0.10
```

Confirms the save path works end-to-end (since T-002-03 hasn't shipped a
UI yet, this is the only way to exercise `saveSettings` in this ticket).

## Testing strategy summary

| What | How | Mandatory? |
|------|-----|-----------|
| `settings.go` defaults flip | `go test ./...` | yes |
| Helper block parses | Browser load, no console errors | yes |
| Bake regression | Manual visual diff against pre-change screenshots | yes |
| Settings load on file select | Console: `currentSettings` non-null | yes |
| File switch resets settings | Console after two `selectFile`s | yes |
| Save round-trip | Console `saveSettings(...)` + file inspection | yes |
| `loadSettings` error path | Stop server mid-load → expect `applyDefaults()` warn | optional |

## Verification criteria (acceptance check)

- [ ] `go test ./...` passes.
- [ ] Browser loads `app.js` with no errors after the changes.
- [ ] `currentSettings` is populated after `selectFile`.
- [ ] Billboard bake of the rose with default settings is visually
      indistinguishable from a pre-change bake.
- [ ] Volumetric bake of the rose with default settings is visually
      indistinguishable from a pre-change bake **except** for the
      documented `1.6 → 1.4` directional light delta.
- [ ] No new UI elements visible.
- [ ] Switching files resets `currentSettings` to the new file's value.
- [ ] `saveSettings(id)` persists to disk (confirmed via cat).
- [ ] Schema doc and `settings.go` agree on `alpha_test = 0.10`.

## Commit strategy

Single commit covering steps 1–13:

```
Wire app.js bake constants to per-asset settings (T-002-02)

Replace hardcoded volumetric/billboard bake literals in static/app.js
with reads from currentSettings, populated by loadSettings(id) on file
select. Add saveSettings(id) (debounced PUT) and applyDefaults() for
T-002-03 to consume. Flip the alpha_test schema default from 0.15 to
0.10 to keep bake-export GLBs regression-free.
```

## Deviations log policy

Document any deviation from this plan in `progress.md` before making the
deviation, with rationale.
