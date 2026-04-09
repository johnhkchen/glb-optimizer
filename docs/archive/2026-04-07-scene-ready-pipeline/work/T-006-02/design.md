# Design — T-006-02: scene template implementations + UI

## Decision summary

1. **Implement five templates** in the existing `SCENE_TEMPLATES`
   registry: `grid`, `hedge-row`, `mixed-bed`, `rock-garden`,
   `container`. Rename T-006-01's `benchmark` template to `grid`
   (kept for FPS benchmarking, per AC). Delete `debug-scatter` —
   it was a framework smoke-test, not a designer-facing template,
   and the new ones supersede it.
2. **Toolbar picker UI**: extend `.stress-controls` in
   `static/index.html` with three new controls — a `<select>`
   for the template, a `<input type="number">` for instance
   count, and a checkbox for the ground plane. Reuse the existing
   `Run` button. The legacy `stressCount` range slider goes
   away — replaced by the number input — because (a) the AC
   says "Instance count input," (b) the new templates have wildly
   different sensible ranges (5-10 for `container`, 100-300 for
   `mixed-bed`), and (c) keeping both controls would be redundant
   and confusing.
3. **Ground plane**: a single `THREE.Mesh(PlaneGeometry,
   MeshStandardMaterial)` created at `initThreeJS` time, hidden
   by default, toggled visible by the checkbox. Brown base color
   (`#6b5544`), 100 × 100 m so it covers any practical template
   footprint. Receives the live scene lighting automatically
   (consistency from S-007 — the live scene's lighting *is* the
   bake's lighting after T-007-02).
4. **Per-asset persistence**: add three fields to `AssetSettings`
   — `scene_template_id`, `scene_instance_count`,
   `scene_ground_plane` — additively (no schema bump, matching
   how `ShapeCategory` and `SliceAxis` were added). The
   `loadSettings` flow restores them into the UI controls;
   change handlers debounce a `saveSettings` like every other
   tuning control.
5. **Analytics**: add `scene_template_selected` to
   `validEventTypes` and emit it from the picker change handler
   (the picker is in the toolbar, not the tuning panel, so the
   `wireTuningUI` auto-instrumentation does not cover it). Payload:
   `{ from, to, instance_count, ground_plane }`.

## Why these template definitions

The AC nails the visual intent for each template; the design
work is choosing the parameters that make the visuals legible
and the implementations short.

### `grid`

Identical to T-006-01's `benchmark`. Spacing = `max(size.x,
size.z) * 1.3`, square `cols = ceil(sqrt(count))`, centered on
origin. Orientation per `ctx.orientationRule`. Honors `count`.
Renamed from `benchmark` because the AC names it `grid` and
because the only consumer of the old name was the
`activeSceneTemplate = 'benchmark'` default in T-006-01, which
this ticket changes anyway.

### `hedge-row`

Single straight row along the X axis. Spacing = `max(size.x,
size.z) * 1.1` (tighter than `grid` because trellises are
designed to butt up). Z = 0. Orientation = `'fixed'`
unconditionally — the entire point of `hedge-row` is to
showcase directional assets all facing the camera the same
way, so we override `ctx.orientationRule`. No size variation
(uniformity is the look). Honors `count`.

Implementation uses `scatterInRow` from T-006-01 with
`jitter = 0`:

```js
generate(ctx, count) {
  const size = ctx.bbox.size;
  const spacing = Math.max(size.x, size.z) * 1.1;
  const half = (count - 1) * spacing / 2;
  const start = new THREE.Vector3(-half, 0, 0);
  const end = new THREE.Vector3(half, 0, 0);
  const positions = scatterInRow(start, end, count, 0, ctx.seed);
  const specs = positions.map(p => makeInstanceSpec(p));
  applyOrientationRule(specs, 'fixed', ctx.seed);
  return specs;
}
```

### `mixed-bed`

Natural-looking scatter with light scale variation. AC: "size
variation 0.85-1.15×". Footprint scales with count to keep
density stable: `span = max(size.x, size.z) * sqrt(count) * 1.4`.
Min-distance = `max(size.x, size.z) * 0.9` so instances can
slightly overlap (a real mixed bed has plants touching). Honors
`count`. Orientation per `ctx.orientationRule` so round bushes
get random Y, directional things stay aligned.

```js
generate(ctx, count) {
  const size = ctx.bbox.size;
  const spread = Math.max(size.x, size.z);
  const span = spread * Math.sqrt(count) * 1.4;
  const half = span / 2;
  const positions = scatterRandomly(
    { minX: -half, maxX: half, minZ: -half, maxZ: half },
    count, ctx.seed, spread * 0.9,
  );
  const specs = positions.map(p => makeInstanceSpec(p));
  applyVariation(specs, [0.85, 1.15], 0, ctx.seed);
  applyOrientationRule(specs, ctx.orientationRule, ctx.seed);
  return specs;
}
```

### `rock-garden`

Sparser scatter with larger size variation. AC: "0.7-1.3×".
Same shape as `mixed-bed` but `span` is wider and `minDistance`
larger so instances do not touch. `span = spread * sqrt(count)
* 2.2`, `minDistance = spread * 1.6`. Honors `count`. Orientation
per rule.

```js
// same shape as mixed-bed, with sparser params and wider scale range
applyVariation(specs, [0.7, 1.3], 0, ctx.seed);
```

### `container`

Tight cluster of 5-10 instances. The AC pins the count range
("5-10"); the user-facing count input gets clamped on the
container template specifically: if `count < 5` we use 5; if
`count > 10` we use 10. Layout: small radius scatter with
min-distance ≈ footprint, so the cluster reads as "potted
together":

```js
generate(ctx, count) {
  const n = Math.max(5, Math.min(10, count));
  const size = ctx.bbox.size;
  const spread = Math.max(size.x, size.z);
  const half = spread * 1.2; // tight ~2.4× footprint
  const positions = scatterRandomly(
    { minX: -half, maxX: half, minZ: -half, maxZ: half },
    n, ctx.seed, spread * 0.7,
  );
  const specs = positions.map(p => makeInstanceSpec(p));
  applyVariation(specs, [0.9, 1.1], 0, ctx.seed);
  applyOrientationRule(specs, ctx.orientationRule, ctx.seed);
  return specs;
}
```

The slider/input still says (e.g.) `15` — the *displayed*
instance count overlay reads `effectiveCount = specs.length`
which is correct. We considered disabling the input on
`container`, but leaving it enabled is the simpler default and
the clamp is documented in the dropdown title attribute.

## Why a `<select>` for the template picker (not radio buttons)

- Only one slot of toolbar real estate is needed; radio buttons
  scale poorly in a horizontal toolbar that already wraps.
- The tuning panel uses `<select>` for `lighting_preset` and
  `slice_distribution_mode` — same pattern, same hover-title
  pattern for descriptions.
- Future template additions don't require HTML changes; the
  options are populated from `SCENE_TEMPLATES` at boot.

Rejected: a button group like `lodToggle`. Five buttons would
need their own CSS row; users would scroll horizontally on
narrow viewports.

## Why `<input type="number">` (not range slider)

- The AC explicitly says "Instance count input" — input, not
  slider.
- The new templates need a wider numeric range than 1-100.
  `mixed-bed`/`rock-garden` look best with 50-300; `container`
  caps at 10. A single slider with stops everywhere is the
  worst of both worlds.
- A number input lets the user type, paste, or use the spin
  buttons. Validation: `min=1 max=500 step=1`; the template
  itself clamps for `container`.
- Existing `stressCount` range slider deleted entirely.
  `clearStressInstances()` resets it today (`static/app.js:2898-2899`)
  — that line goes away with the slider.

The tradeoff: users lose the tactile slider feel. Mitigation:
the input is wide enough to read clearly, defaults sensibly per
template (we default to 100 for `grid`, 50 for `mixed-bed`,
20 for `rock-garden`, 8 for `container`, 12 for `hedge-row`),
and analytics will tell us if anyone misses the slider.

## Why ground plane is `MeshStandardMaterial` (not `MeshBasicMaterial`)

The AC says "uses the same lighting preset as the bake." That
means the plane must respond to the same lights as the loaded
asset. `MeshStandardMaterial` is the only path that picks up
`scene.environment` + the directional/hemisphere lights set in
`initThreeJS` and modulated by `applyPresetToLiveScene`.
`MeshBasicMaterial` ignores all of them and would render the
plane as a flat color independent of the active preset, which
contradicts the AC.

A textured material was rejected (out of scope per AC: "no
texture asset needed for v1, just a brown base color").

## Why ground plane is created at `initThreeJS` (not lazily)

The plane is a persistent scene object — toggling it on and off
should be cheap. Creating it once at init means the toggle is
just `groundPlane.visible = on` rather than instantiating the
mesh + material on every change. Cost is one `Mesh` and one
`PlaneGeometry(100, 100)` at startup, both negligible.

## Persistence shape

Three new fields on `AssetSettings`:

```go
SceneTemplateId   string `json:"scene_template_id,omitempty"`
SceneInstanceCount int    `json:"scene_instance_count,omitempty"`
SceneGroundPlane   bool   `json:"scene_ground_plane,omitempty"`
```

Defaults (in `DefaultSettings()`):

- `SceneTemplateId = "grid"` — matches the legacy
  benchmark/grid behavior, so existing assets behave identically
  on first load.
- `SceneInstanceCount = 100` — matches the legacy slider's
  100x bench preset.
- `SceneGroundPlane = false` — opt-in.

Validation (in `Validate()`):

- `SceneTemplateId` must be in a `validSceneTemplates` map
  populated with `{grid, hedge-row, mixed-bed, rock-garden,
  container}`. Empty string is normalized to `"grid"` at load
  time, mirroring the `ShapeCategory`/`SliceAxis` pattern.
- `SceneInstanceCount` in `[1, 500]`.
- `SceneGroundPlane` is bool — both values valid.

Forward-compat normalization in `LoadSettings()`:

- `if s.SceneTemplateId == "" { s.SceneTemplateId = "grid" }`
- `if s.SceneInstanceCount == 0 { s.SceneInstanceCount = 100 }`
- `SceneGroundPlane`'s zero value (`false`) is the migration
  default, so no normalization needed — but a `*bool` probe
  like `GroundAlign` does is unnecessary overhead because the
  desired migration default is `false`, which IS the Go zero.

`SettingsDifferFromDefaults()` gains three new comparisons.

`makeDefaults()` in `static/app.js` mirrors the same three keys
with snake_case names.

## Schema version: do we bump?

No. The pattern set by `ShapeCategory`/`SliceAxis` (T-004-02 /
T-004-03) is to add fields additively without bumping
`SettingsSchemaVersion`. The forward-compat normalization in
`LoadSettings` makes legacy files transparently get the new
defaults. A schema bump would be a forced migration with no
benefit; the JSON is purely additive.

## How `runStressTest` consumes the new state

Today (`static/app.js:3496`) it reads `count` from a function
parameter. The new flow:

- Toolbar `Run` button reads `currentSettings.scene_instance_count`
  and `getActiveSceneTemplate()` (which is the template id), not
  the now-removed slider.
- Calls `runStressTest(count, useLods, quality)` — same
  signature.
- The active template id was set by the picker change handler
  via `setSceneTemplate(id)` and persisted to `currentSettings`
  in the same handler.

`runStressTest` itself does not need to read settings — the
`activeSceneTemplate` module-level state is the single source
of truth at template-resolution time. Settings are the
*persistence* of `activeSceneTemplate`, not its runtime source.

## How the picker hydrates from `currentSettings` on `selectFile`

`selectFile` (`static/app.js:3735`) calls `loadSettings` then
`populateTuningUI`. We add a sibling helper
`populateScenePreviewUI()` that:

1. Reads `currentSettings.scene_template_id`,
   `scene_instance_count`, `scene_ground_plane`.
2. Sets the dropdown's value, the number input's value, the
   ground checkbox's checked state.
3. Calls `setSceneTemplate(id)` to update the JS state.
4. Toggles `groundPlane.visible`.

Called from `selectFile` after `loadSettings` resolves and
also from boot via `applyDefaults()` so a never-selected app
state still has sensible UI defaults.

## Analytics event payload

```json
{
  "schema_version": 1,
  "event_type": "scene_template_selected",
  "session_id": "...",
  "asset_id": "...",
  "payload": {
    "from": "grid",
    "to": "mixed-bed",
    "instance_count": 50,
    "ground_plane": true
  }
}
```

`from` is the previous template id; `to` is the new one; the
other two fields snapshot the rest of the scene preview state
at the time of the change. Emitted only on actual change
(`from !== to`); count and ground toggles do not emit a
template-selected event (they would generate noise — the
existing per-asset settings round-trip captures them on
session_end via `final_settings`).

## What this design intentionally does not solve

- **LOD path still uses `Vector3[]`** — variation invisible in
  LOD mode. T-006-01 inherited this; T-006-02 does not fix it.
- **Production hybrid still uses `Vector3[]`** — same as above.
- **No template-specific UI** — the same number input is shown
  for all templates even though `container` clamps to [5,10].
  A future enhancement could swap the input mode per template.
- **Ground plane has no transparency or shadow** — the lights
  in `initThreeJS` don't cast shadows, so a shadow-receiving
  plane would be a no-op. If shadows ever land, the plane is
  the right place to set `receiveShadow = true`.
- **The grid helper at Y=0 stays** — visually distinct from the
  ground plane (one is a wireframe, one is solid). They can
  coexist or the user can mentally treat the wireframe as the
  "ruler" and the plane as the "soil."
