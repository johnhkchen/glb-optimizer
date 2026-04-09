# Research — T-006-02: scene template implementations + UI

## Ticket goal in one line

Land the actual designer-facing scene templates (`hedge-row`,
`mixed-bed`, `rock-garden`, `container`, `grid`), wire a picker UI
into the preview toolbar, add a ground plane toggle, and persist
the choices in `currentSettings`.

## What T-006-01 already shipped

The framework lives in a self-contained section of `static/app.js`
at lines 3002-3193 (`// ── Scene Templates (T-006-01) ──`):

- `makeInstanceSpec(position, rotationY, scale)` factory
  (`static/app.js:3014`).
- Scatter helpers:
  - `scatterRandomly(boundsXZ, count, seed, minDistance)` —
    rejection-sampled XZ scatter, returns `Vector3[]`
    (`static/app.js:3021`).
  - `scatterInRow(start, end, count, jitter, seed)` — linear interp
    with per-axis jitter (`static/app.js:3052`). **Currently has no
    consumers** — added for T-006-02 to drive `hedge-row`.
- Variation helpers:
  - `applyVariation(specs, scaleRange, jitterAmount, seed)`
    (`static/app.js:3068`)
  - `applyOrientationRule(specs, rule, seed)` — handles `random-y`,
    `fixed`, `aligned-to-row` (`static/app.js:3088`).
- `boundsFromSpecs(specs)` — XZ AABB derivation
  (`static/app.js:3106`).
- `SCENE_TEMPLATES` registry with two entries today
  (`static/app.js:3122`):
  - `benchmark` — line-for-line port of the legacy 100x grid math.
  - `debug-scatter` — 20-instance scatter, used to validate the
    framework. Will be removed/replaced.
- `activeSceneTemplate` module-level state (`static/app.js:3173`),
  `setSceneTemplate(id)` / `getActiveSceneTemplate()` accessors,
  exposed on `window.setSceneTemplate` /
  `window.__SCENE_TEMPLATES` for devtools driving.

`runStressTest` (`static/app.js:3496`) builds a `ctx = { bbox,
shapeCategory, orientationRule, seed }` from `originalModelBBox`
and `currentSettings`, calls
`SCENE_TEMPLATES[activeSceneTemplate].generate(ctx, count)`, and
routes the resulting `InstanceSpec[]` through three placement
helpers:

- `createInstancedFromModel` (regular GLB; spec-aware overload).
- `createBillboardInstances` (camera-facing quads; per-instance
  scale honored, side rotation overwritten by
  `updateBillboardFacing`).
- `createVolumetricInstances` (horizontal slices; per-instance
  rotation+scale honored on every layer).

LOD and production-hybrid paths still receive the legacy
`Vector3[]` form (`positions = specs.map(s => s.position)`) and
ignore per-instance scale variation. T-006-01's review.md flags
this as a known limitation; T-006-02 inherits it.

## Per-shape orientation rules already wired

`STRATEGY_TABLE` at `static/app.js:349` mirrors `strategy.go`'s
`shapeStrategyTable`. Each shape has an
`instance_orientation_rule` of `'random-y' | 'fixed' |
'aligned-to-row'`. `runStressTest` reads it via
`(STRATEGY_TABLE[cat] && STRATEGY_TABLE[cat]
.instance_orientation_rule) || 'random-y'`. Templates can rely on
`ctx.orientationRule` being one of these three values; templates
that want a *different* rule (e.g., `hedge-row` always uses
`fixed` for trellises) can override via `applyOrientationRule(...,
'fixed', ...)` directly.

Mapping that matters for the new templates:

| shape category | orientation rule today |
|---|---|
| `round-bush` | `random-y` |
| `tall-narrow` | `random-y` |
| `directional` | `fixed` |
| `planar` | `aligned-to-row` |
| `hard-surface` | `fixed` |
| `unknown` | `random-y` |

## UI surface that exists today

Toolbar HTML lives in `static/index.html:65-74`:

```html
<div class="stress-controls">
    <label class="stress-label">Count:</label>
    <input type="range" id="stressCount" min="1" max="100" value="1" ...>
    <span id="stressCountValue" class="stress-value">1x</span>
    <label class="stress-label"><input type="checkbox" id="stressUseLods"> LOD</label>
    <label class="stress-label" id="lodQualityLabel" ...>Quality:</label>
    <input type="range" id="lodQuality" ...>
    <span id="lodQualityValue" ...>50%</span>
    <button class="wireframe-toggle" id="stressBtn">Run</button>
</div>
```

Style hooks (`static/style.css:412-435`):

- `.stress-controls` — flex row, `margin-left: auto` to push right.
- `.stress-label` — small muted text.
- `.stress-slider` — 80 px range.
- `.stress-value` — accent-coloured value readout.

Wire-up at `static/app.js:4049-4077`:

```js
const stressSlider = document.getElementById('stressCount');
...
stressBtn.addEventListener('click', () => {
    const count = parseInt(stressSlider.value);
    if (count <= 1) clearStressInstances(); else
        runStressTest(count, stressUseLods.checked, quality);
});
```

The slider has range `1..100` and the existing FPS-bench
`benchmark` template ignores higher values implicitly. The new
templates may want different ranges (`container` is 5-10), but
the `count` slider stays the master control — template `generate`
functions decide whether to honor it (`benchmark`/`grid`/
`hedge-row`/`mixed-bed`/`rock-garden`) or clamp/ignore it
(`container`).

There is **no** template picker, ground plane toggle, or
analytics emission for template selection.

## Per-asset settings persistence

`currentSettings` is the JS-side mirror of `AssetSettings`
(`settings.go:21`). Round-trip:

- Load: `loadSettings(id)` → `GET /api/settings/:id` →
  `currentSettings` (`static/app.js:88`).
- Save: `saveSettings(id)` → debounced 300 ms PUT
  (`static/app.js:100`).
- Defaults: `makeDefaults()` (`static/app.js:120`) mirrors
  `DefaultSettings()` in `settings.go:55` by hand. Both must be
  kept in sync.

Adding per-asset state for the scene template selection means
adding three fields to the on-disk schema:

- `scene_template_id` (string, default `"grid"`)
- `scene_instance_count` (int, default `100`)
- `scene_ground_plane` (bool, default `false`)

The existing pattern for additive schema changes (`ShapeCategory`,
`SliceAxis` were added without bumping `SettingsSchemaVersion`)
applies here:

1. Add fields to `AssetSettings` (`settings.go:21`).
2. Set defaults in `DefaultSettings()` (`settings.go:55`).
3. Add validation in `Validate()` (`settings.go:132`).
4. Add forward-compat normalization in `LoadSettings()`
   (`settings.go:238`) so legacy on-disk files get the defaults.
5. Update `SettingsDifferFromDefaults()` (`settings.go:201`).
6. Mirror in `makeDefaults()` (`static/app.js:120`).
7. Add tests in `settings_test.go` (existing pattern at lines
   116-205, 329-450 covers new-fields golden-checks and roundtrip).

## Analytics emission

Analytics goes through `logEvent(type, payload, assetId)`
(`static/app.js:288`), which POSTs to `/api/analytics/event`. The
event_type allow-list lives in `analytics.go:25` `validEventTypes`.
Adding `scene_template_selected` requires:

- One line in `validEventTypes` (`analytics.go:25`).
- A `logEvent('scene_template_selected', { from, to, count,
  ground_plane }, assetId)` call from the picker change handler.
- Documentation in `docs/knowledge/analytics-schema.md`
  (consistent with prior tickets — T-003-03, T-004-03 etc. all
  added an event type and a doc section).

`setting_changed` already auto-fires for any tuning-panel control
via `wireTuningUI` (`static/app.js:682`), but the scene template
picker is *not* in the tuning panel — it lives in the preview
toolbar — so a manual `logEvent` call is required.

## Three.js scene state and ground plane integration

`initThreeJS()` (`static/app.js:2783`) builds the scene with a
single `GridHelper` at line 2820:

```js
scene.add(new THREE.GridHelper(10, 20, 0x2a2a4a, 0x1a1a3e));
```

The grid is decorative; nothing else lives at Y=0 today. Adding
a ground plane means:

- `THREE.PlaneGeometry(size, size)` rotated `-π/2` around X
  (XZ plane).
- `MeshStandardMaterial` (so it picks up scene lighting like the
  asset does — important for "consistency from S-007") with a
  brown base color (~`#6b5544`).
- Added to `scene` once at init time, kept hidden by default
  (`mesh.visible = false`), toggled by the new UI checkbox.
- Sized large enough to extend beyond the largest template
  footprint. The biggest template footprint will be
  `mixed-bed`/`rock-garden` at maybe `~10× max(bbox.x, bbox.z)`,
  so a 100 × 100 plane is comfortably oversize for any practical
  asset.
- `frustumCulled = false` is unnecessary on the plane (it's
  bounded), but `receiveShadow = true` is irrelevant because
  none of the lights cast shadows in this scene
  (`initThreeJS:2807-2818` use no `shadow.*` config).

The same lighting that bake uses (`applyPresetToLiveScene`,
T-007-02) already applies to the live scene, so the plane
inherits the bake-time lighting "for free" — no extra wiring
needed for the AC's "same lighting preset as the bake" line.

Camera framing in `runStressTest` (`static/app.js:3558`) sets
`camera.far = camDist * 10`. The plane sits at Y=0 inside that
frustum, so no `far`-plane fix is needed.

`clearStressInstances()` (`static/app.js:2885`) tears down stress
state but should NOT touch the ground plane — the plane is a
persistent scene object, toggled by UI, not by stress lifecycle.

## Files in scope

- `static/app.js` — primary surface. Sections to touch:
  - Scene Templates section (3002-3193): add the five real
    templates, remove `debug-scatter`.
  - DOM refs section (39-72): grab the new picker / count input /
    ground toggle elements.
  - `initThreeJS` (2783): create the ground plane mesh.
  - `runStressTest` (3496): read the count from the new input
    (or fall back to the existing slider — see design.md).
  - Settings mirror `makeDefaults()` (120) and load/save
    helpers (88, 100): persist the three new fields.
  - Event listeners section (3922+): wire the picker change,
    count input change, ground toggle change, plus the
    analytics emission.
  - `selectFile` (3735): on asset load, restore the three new
    settings into the UI controls and call `setSceneTemplate`.
- `static/index.html:65-74` — extend `.stress-controls` with the
  picker dropdown, instance-count input, ground checkbox.
  Existing range slider and `Run` button stay.
- `static/style.css:408-435` — flex wrapping for the now-wider
  controls row; new selector for the dropdown.
- `settings.go` — add three new persisted fields, defaults,
  validation, normalization, differ-from-defaults audit.
- `settings_test.go` — extend new-fields tests to cover the
  scene fields, roundtrip, and validation rejection.
- `analytics.go` — extend `validEventTypes` with
  `scene_template_selected`.
- `docs/knowledge/analytics-schema.md` — document the new event.

## Constraints inherited

1. **`originalModelBBox` is the spacing source.** Templates must
   read from `ctx.bbox`, never recompute.
2. **The slider tops out at 100.** Templates that scatter
   hundreds of instances would need either a slider remap or a
   text input. AC says "Instance count input" — leaning text/
   number input lets us range 1-500 without breaking the existing
   `stressCount` slider's tactile feel. Decision deferred to
   design.md.
3. **`runLodStressTest` and `runProductionStressTest` use the
   legacy `Vector3[]` path** — per-instance scale variation does
   not propagate to LOD mode. T-006-02 doesn't change this; the
   AC doesn't require LOD support for the new templates.
4. **Side billboards camera-face every frame** — rotation
   variation is invisible in billboard side mode. Top quads
   honor rotation, scale is honored on all variants.
5. **`frustumCulled = false`** must remain on every InstancedMesh.
6. **Determinism** — every random stream goes through
   `seededRandom(seed * <prime> + i)`. Never use `Math.random()`.

## What does NOT exist today

- No template picker UI.
- No ground plane (just the GridHelper line at Y=0).
- No `scene_template_selected` analytics event.
- No `scene_template_id` / `scene_instance_count` /
  `scene_ground_plane` fields in `AssetSettings`.
- No `hedge-row`, `mixed-bed`, `rock-garden`, `container`, `grid`
  templates (the existing `benchmark` template is the closest
  thing to `grid` and will be renamed/aliased).

## Out-of-scope but adjacent

- T-006-03 (if it exists) — ground plane material editor, multi-
  asset scenes, animated lighting. AC explicitly cuts these.
- The LOD path migration to spec-aware placement — flagged in
  T-006-01's review.md, not addressed here.
