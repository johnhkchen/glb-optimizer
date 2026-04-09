# Research — T-002-02: Wire app.js bake constants to settings

## Scope reminder

Invert every hardcoded constant that affects the volumetric/billboard
**bake** so it reads from a per-asset settings object loaded from the
backend. **No new UI**, no new params, no live re-render. The user-visible
behavior must be (essentially) regression-free with default settings.

## Backend (already shipped by T-002-01)

- `settings.go` → `AssetSettings` struct, `DefaultSettings()`,
  `LoadSettings`, `SaveSettings`, `Validate`. Field declaration order is the
  on-disk JSON order.
- HTTP: `GET /api/settings/{id}` returns `DefaultSettings()` when no file
  exists; `PUT /api/settings/{id}` validates + atomically writes. Wired in
  `main.go:121`.
- `FileRecord.HasSavedSettings bool` is populated at scan time and after
  any successful PUT. Currently unused on the client.
- Schema doc: `docs/knowledge/settings-schema.md`. JSON field names use
  `snake_case` (`volumetric_layers`, `bake_exposure`, etc.).

The eleven schema fields and v1 defaults (verified against `settings.go`):

| field                    | default   |
|--------------------------|-----------|
| `volumetric_layers`      | `4`       |
| `volumetric_resolution`  | `512`     |
| `dome_height_factor`     | `0.5`     |
| `bake_exposure`          | `1.0`     |
| `ambient_intensity`      | `0.5`     |
| `hemisphere_intensity`   | `1.0`     |
| `key_light_intensity`    | `1.4`     |
| `bottom_fill_intensity`  | `0.4`     |
| `env_map_intensity`      | `1.2`     |
| `alpha_test`             | `0.15`    |
| `lighting_preset`        | `"default"` |

`schema_version` is the 12th field, set to `1`.

## Client (`static/app.js`) — current state

`app.js` is 2341 lines. Globals declared at lines 9–28. There is an
**unrelated** `getSettings()` at line 72 — that returns the *gltfpack*
optimization settings (simplification, compression, …). Different concept,
different API surface (`POST /api/process`). The new per-asset bake
settings live in their own namespace; no name collision if we call the new
helpers `loadSettings`/`saveSettings`/`applyDefaults`, which is what the
ticket asks for. (`getSettings` stays untouched.)

### Hardcoded constants in scope

Grep over the bake/preview paths produced this inventory:

| line | function | literal | maps to schema field |
|------|----------|---------|---------------------|
| 542  | module-scope | `const VOLUMETRIC_LAYERS = 4` | `volumetric_layers` |
| 543  | module-scope | `const VOLUMETRIC_RESOLUTION = 512` | `volumetric_resolution` |
| 552  | `generateVolumetric` | uses `VOLUMETRIC_LAYERS`, `VOLUMETRIC_RESOLUTION` | (callsite) |
| 819  | `generateProductionAsset` | uses `VOLUMETRIC_LAYERS`, `VOLUMETRIC_RESOLUTION` | (callsite) |
| 738  | `renderHorizontalLayerGLB` | `const domeHeight = layerThickness * 0.5` | `dome_height_factor` |
| 301  | `renderBillboardAngle` | `offRenderer.toneMappingExposure = 1.0` | `bake_exposure` |
| 442  | `renderBillboardTopDown` | `offRenderer.toneMappingExposure = 1.0` | `bake_exposure` |
| 580  | `renderLayerTopDown` | `offRenderer.toneMappingExposure = 1.0` | `bake_exposure` |
| 359  | `setupBakeLights` | `new THREE.AmbientLight(sky, 0.5)` | `ambient_intensity` |
| 361  | `setupBakeLights` | `new THREE.HemisphereLight(sky, ground, 1.0)` | `hemisphere_intensity` |
| 363  | `setupBakeLights` | `new THREE.DirectionalLight(sky, 1.4)` (top key) | `key_light_intensity` |
| 367  | `setupBakeLights` | `new THREE.DirectionalLight(fill, 0.4)` (bottom fill) | `bottom_fill_intensity` |
| 614  | `renderLayerTopDown` | `new THREE.AmbientLight(sky, 0.5)` | `ambient_intensity` |
| 615  | `renderLayerTopDown` | `new THREE.HemisphereLight(sky, ground, 1.0)` | `hemisphere_intensity` |
| 616  | `renderLayerTopDown` | `new THREE.DirectionalLight(sky, 1.6)` ⚠ | `key_light_intensity` |
| 422  | `cloneModelForBake` | `c.envMapIntensity = 1.2` | `env_map_intensity` |
| 491  | `renderMultiAngleBillboardGLB` | `alphaTest: 0.1` (side quad mat) | `alpha_test` |
| 518  | `renderMultiAngleBillboardGLB` | `alphaTest: 0.1` (top quad mat) | `alpha_test` |
| 745  | `renderHorizontalLayerGLB` | `alphaTest: 0.1` (volumetric layer mat) | `alpha_test` |

### Out of scope literals (intentionally NOT wired)

- `renderer.toneMappingExposure = 1.3` (line 1395) — *main preview*
  renderer, not the bake. Different exposure tuned for the live viewer.
- Live-preview lights at 1409–1419 — these belong to the on-screen
  viewer, not the bake pipeline. Reset by `resetSceneLights` at 2100.
- `mat.alphaTest = 0.5` at 1661 / 1683 — runtime **billboard instance**
  override. Billboards are reconfigured to opaque alpha-cutout at 0.5
  in `createBillboardInstances`; this is a separate runtime concept
  from the bake-time export literal.
- `mat.alphaTest = 0.15` at 1751 — runtime **volumetric instance**
  override. Same story: applied to the *loaded* GLB at instance time.
  Conceptually it could share the schema field, but it's a runtime
  decision divorced from the bake; leaving it alone keeps the diff
  surgical and avoids touching `createVolumetricInstances`.
- `runPipelineRoundtrip` at 974–978 — diagnostic that should stay
  numerically stable. Not on the bake path.
- `testLighting` at 850 — diagnostic, same reasoning.
- `simplificationSlider` / `getSettings()` at 72 — gltfpack pipeline,
  unrelated namespace.

## Discrepancies between current literals and T-002-01 schema defaults

Two literals already disagree with the v1 schema defaults that T-002-01
landed:

1. **`key_light_intensity`** — schema default `1.4`. `setupBakeLights:363`
   uses `1.4` (matches). `renderLayerTopDown:616` uses **`1.6`** (does
   not). T-002-01's review.md flagged this as a known one-tick visual
   delta.
2. **`alpha_test`** — schema default `0.15`. The three bake-export sites
   (491, 518, 745) all use **`0.1`**. The runtime volumetric override at
   1751 uses `0.15` (and is NOT being wired here).

The ticket says: "If existing assets render differently after the
inversion, the schema defaults are wrong — fix them in T-002-01's schema
doc, not by hacking around them here." Design phase will pick a
resolution; for now we record both deltas.

Magnitudes: `1.4 → 1.6` is +14% on one of three lights in one of two bake
paths. `0.1 → 0.15` is +50% relative on the alpha cutoff but in absolute
terms shifts which texels survive by ~5% intensity — visible only at
foliage edges.

## `selectFile` and the load-order constraint

`selectFile(id)` at line 2076:
1. resets reference environment + lights
2. (optionally) loads reference image
3. calls `loadModel(...)` inside the `loadEnv.then(...)` chain

`loadModel` only triggers display; the bake functions
(`generateBillboard`, `generateVolumetric`, `generateProductionAsset`,
`testLighting`) are user-driven via button clicks **after** the model has
loaded. So we have a window between `selectFile` and the first bake button
press to populate `currentSettings`. The ticket requires settings to be in
place before bake/preview functions run — putting `await loadSettings(id)`
inside the `loadEnv.then(...)` chain (before `loadModel`) satisfies this
trivially.

## Constraints

- **No new UI**, no new buttons, no new sliders. T-002-03 owns UI.
- **Pattern consistency**: bake functions either read the global directly
  or take a settings parameter — the ticket says pick one. Direct global
  read matches the existing precedent (`referencePalette`, `currentModel`,
  `referenceEnvironment`) and minimizes call-site churn.
- **Debounce on save**: `saveSettings(id)` is called from places that
  might fire rapidly (T-002-03 sliders) — must debounce. Not exercised in
  this ticket but the contract has to land here.
- **File switch**: `currentSettings` must reset on file change so a stale
  value from file A doesn't leak into file B. Falls out naturally from
  `selectFile` → `loadSettings` ordering.
- **Tests**: there is no JS test infrastructure in the repo
  (`settings_test.go` is the *first* test file, period). Verification is
  visual + manual smoke tests, plus the existing `go test ./...`.

## Risks / open questions for design

1. How to resolve the `1.6` and `0.1` literal/schema divergences. Options
   in design.md.
2. `loadSettings` failure mode: `/api/settings/:id` returns 200 with
   defaults if no file exists, so the only failure modes are network or
   500. Default to `applyDefaults()` on any failure and log.
3. Should `selectFile` `await` the settings load or fire-and-forget?
   `await` is correct because bake buttons become enabled after the model
   loads; if settings haven't arrived, a fast user could click before
   they do. Cheap to wait — settings JSON is ~400 bytes.
