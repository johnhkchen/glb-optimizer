# Structure — T-002-02: Wire app.js bake constants to settings

## Files touched

| File | Action | Net effect |
|------|--------|-----------|
| `static/app.js` | MODIFY | New state + helper block, ~14 literal replacements |
| `settings.go` | MODIFY | `AlphaTest` default `0.15 → 0.10` (regression fix per design.md) |
| `docs/knowledge/settings-schema.md` | MODIFY | `alpha_test` defaults table cell `0.15 → 0.10` |

No new files, no deletions, no JS modules. The ticket forbids it and the
existing patterns don't justify it.

## `static/app.js` — change map

### 1. State block (line ~28, after `referencePalette`)

Add:

```js
let currentSettings = null; // per-asset bake/tuning settings, populated by selectFile()
let _saveSettingsTimer = null; // debounce handle for saveSettings()
```

### 2. New helper block

Drop a new `// ── Asset Settings ──` section near the existing
`// ── Helpers ──` block (around line 63), **above** the unrelated
`getSettings()` (gltfpack pipeline) so the two namespaces visually
separate.

Functions added:

- `loadSettings(id)` — async; fetch + assign + return; falls back to
  `applyDefaults()` on any error.
- `saveSettings(id)` — debounced 500 ms trailing PUT; no-op if
  `currentSettings` is null. Not called from any code path in this
  ticket; lives here for T-002-03.
- `applyDefaults()` — sets `currentSettings` to a literal mirror of
  `DefaultSettings()` in `settings.go`.

Total: ~45 lines including comments.

### 3. Removed module-scope constants

```js
const VOLUMETRIC_LAYERS = 4;       // line 542
const VOLUMETRIC_RESOLUTION = 512; // line 543
```

These two `const` declarations are deleted entirely. Both are used at
exactly two call sites (`generateVolumetric:552`, `generateProductionAsset:819`),
both rewritten to read `currentSettings.volumetric_layers` /
`currentSettings.volumetric_resolution`.

### 4. Literal replacements (in source order)

| line  | before | after |
|-------|--------|-------|
| 301   | `offRenderer.toneMappingExposure = 1.0;` | `offRenderer.toneMappingExposure = currentSettings.bake_exposure;` |
| 359   | `new THREE.AmbientLight(sky, 0.5)` | `new THREE.AmbientLight(sky, currentSettings.ambient_intensity)` |
| 361   | `new THREE.HemisphereLight(sky, ground, 1.0)` | `new THREE.HemisphereLight(sky, ground, currentSettings.hemisphere_intensity)` |
| 363   | `new THREE.DirectionalLight(sky, 1.4)` | `new THREE.DirectionalLight(sky, currentSettings.key_light_intensity)` |
| 367   | `new THREE.DirectionalLight(fill, 0.4)` | `new THREE.DirectionalLight(fill, currentSettings.bottom_fill_intensity)` |
| 422   | `c.envMapIntensity = 1.2;` | `c.envMapIntensity = currentSettings.env_map_intensity;` |
| 442   | `offRenderer.toneMappingExposure = 1.0;` | `offRenderer.toneMappingExposure = currentSettings.bake_exposure;` |
| 491   | `alphaTest: 0.1,` (side billboard mat) | `alphaTest: currentSettings.alpha_test,` |
| 518   | `alphaTest: 0.1,` (top billboard mat) | `alphaTest: currentSettings.alpha_test,` |
| 552   | `VOLUMETRIC_LAYERS, VOLUMETRIC_RESOLUTION` | `currentSettings.volumetric_layers, currentSettings.volumetric_resolution` |
| 580   | `offRenderer.toneMappingExposure = 1.0;` | `offRenderer.toneMappingExposure = currentSettings.bake_exposure;` |
| 614   | `new THREE.AmbientLight(sky, 0.5)` | `new THREE.AmbientLight(sky, currentSettings.ambient_intensity)` |
| 615   | `new THREE.HemisphereLight(sky, ground, 1.0)` | `new THREE.HemisphereLight(sky, ground, currentSettings.hemisphere_intensity)` |
| 616   | `new THREE.DirectionalLight(sky, 1.6)` ⚠ | `new THREE.DirectionalLight(sky, currentSettings.key_light_intensity)` |
| 738   | `const domeHeight = layerThickness * 0.5;` | `const domeHeight = layerThickness * currentSettings.dome_height_factor;` |
| 745   | `alphaTest: 0.1,` (volumetric layer mat) | `alphaTest: currentSettings.alpha_test,` |
| 819   | `VOLUMETRIC_LAYERS, VOLUMETRIC_RESOLUTION` | `currentSettings.volumetric_layers, currentSettings.volumetric_resolution` |

The line numbers above are **pre-edit** and will drift as the new state +
helper block is inserted near the top of the file. Edits will be done by
matching unique substrings, not line numbers.

### 5. `selectFile(id)` change (line 2076)

Wrap the `loadEnv.then(...)` body so settings load *before* `loadModel`:

```js
loadEnv.then(async () => {
  await loadSettings(id);
  loadModel(`/api/preview/${id}?version=original&t=${Date.now()}`, file.original_size);
});
```

This satisfies the ticket's "settings in place when bake/preview functions
run" requirement and the file-switch reset requirement (each new
`selectFile` call rebinds `currentSettings` to the new id's value or
defaults).

### 6. Defensive null guard (chosen approach)

Bake functions assume `currentSettings` is non-null. If a developer ever
calls a bake helper before `selectFile` has loaded settings (impossible
through the UI but possible via console), reads off `null` would throw
`TypeError`. To guard:

- **Approach A**: every bake function calls `if (!currentSettings) applyDefaults();` at the top.
- **Approach B**: leave the `TypeError` to surface — fail loud, easier
  to debug.

Picking **Approach B**: failing loud is preferable to silently filling in
defaults that may not match a saved profile. The UI flow guarantees
non-null by the time any bake button is clickable.

## `settings.go` — change map

One line in `DefaultSettings()`:

```go
AlphaTest: 0.15,  →  AlphaTest: 0.10,
```

`Validate()` is unchanged — `[0,1]` admits `0.10` already.

## `docs/knowledge/settings-schema.md` — change map

The defaults table row for `alpha_test`: change the `default` cell from
`0.15` to `0.10`. No range/description change.

## Tests

- Existing `go test ./...` must still pass. The only change is the
  default value, which:
  - `TestDefaultSettings_Valid` exercises (still validates → still passes).
  - `TestSaveLoad_Roundtrip` exercises (round-trips bytewise → still
    passes since the value is just different).
  - `TestValidate_RejectsOutOfRange` doesn't touch `alpha_test`'s default.

- No new Go tests.
- No JS tests (no JS test infra in repo, out of scope).
- Manual smoke: documented in `plan.md`.

## Ordering

Edits can be applied in any order, but a sensible serialization is:

1. `settings.go` defaults flip (atomic, smallest blast radius).
2. `docs/knowledge/settings-schema.md` defaults table sync.
3. `app.js`: insert state block + helper block at the top.
4. `app.js`: replace literals in source order.
5. `app.js`: rewrite `selectFile`'s `loadEnv.then` chain.
6. `app.js`: delete the two `VOLUMETRIC_*` consts.
7. Run `go test ./...`.
8. Manual smoke test recipe in `plan.md`.

## Public interfaces

After this ticket, T-002-03 can rely on:

- Global mutable: `currentSettings` (read/write).
- Mutator: assign-then-`saveSettings(selectedFileId)` for any field
  change.
- Loader: `loadSettings(id)` (already called by `selectFile`).
- Reset: `applyDefaults()` followed by `saveSettings(selectedFileId)`.

This is the entire surface T-002-03 needs. No additional plumbing.
