# Design — T-002-02: Wire app.js bake constants to settings

## Goal

Replace hardcoded bake/preview constants in `static/app.js` with reads
from a per-asset `currentSettings` object loaded from `/api/settings/:id`,
without changing user-visible bake output for default settings and without
introducing any UI.

## Decision summary

| Question | Decision |
|----------|----------|
| How do bake functions read settings? | **Direct global read** of `currentSettings`. No parameter threading. |
| Where does `currentSettings` live? | New `let currentSettings = null;` in the State block at the top of `app.js`. |
| New helper names | `loadSettings(id)`, `saveSettings(id)`, `applyDefaults()`. |
| Save semantics | Debounced PUT, 500 ms trailing edge. |
| When does `selectFile` load settings? | Inside the existing `loadEnv.then(...)` chain, **before** `loadModel`. `await`ed. |
| Failure recovery | Any error in `loadSettings` → `applyDefaults()` + console warn. Never leave `currentSettings === null`. |
| Schema divergence: `1.6 → 1.4` (volumetric key light) | Accept the regression as documented by T-002-01 review.md. Wire to `key_light_intensity`. |
| Schema divergence: `0.1 → 0.15` (bake-export alpha) | Update T-002-01's schema default to `0.10` so the bake-export sites stay regression-free. |
| Runtime overrides at lines 1661/1683/1751 | **Leave alone.** Different concept (instance-time material reconfiguration). |
| Diagnostic literals (`testLighting`, `runPipelineRoundtrip`) | **Leave alone.** Diagnostics need numerical stability. |
| Live-preview renderer (`renderer.toneMappingExposure = 1.3`) | **Leave alone.** Not the bake. |

## Options considered

### Option A — Pass `settings` as a parameter to every bake function

Each of `renderBillboardAngle`, `renderBillboardTopDown`, `renderLayerTopDown`,
`setupBakeLights`, `cloneModelForBake`, `renderMultiAngleBillboardGLB`,
`renderHorizontalLayerGLB` gains a `settings` parameter, and every call
site forwards `currentSettings` (or a passed-in copy) explicitly.

**Pros.** Pure functions; trivial to test in isolation; harder for someone
to call a bake function "with no settings loaded" by accident.

**Cons.** Six function signatures change. Eight call sites change. The
file already uses module-scope state liberally for the same kind of cross-
cutting context (`referencePalette`, `currentModel`, `defaultEnvironment`).
The signature churn is pure noise relative to the existing pattern.

### Option B — Direct read of `currentSettings` global from inside bake functions ✅

Add `let currentSettings = null;` next to the other state. Each bake
function reads fields off the global directly. `selectFile` ensures the
global is populated before any bake button is reachable. Bake helpers
defensively fall back to `applyDefaults()` if `currentSettings === null`
(shouldn't happen, but trivially cheap insurance).

**Pros.** Matches existing patterns. Smallest diff. Zero call-site churn.
Adding a new tunable in T-002-03 means: add a slider → update one line in
the bake function. No function-signature ripple.

**Cons.** Implicit dependency. Mitigated by the `selectFile` ordering and
the defensive fallback. Slightly harder to unit-test in isolation, but
there are no JS unit tests in this repo and adding them is out of scope.

**Picked B.** The ticket explicitly invites this: *"read from
`currentSettings` directly OR accept it as a parameter — pick one pattern
and use it consistently."*

### Option C — `Object.freeze` plus a setter

Wrap `currentSettings` in a getter/setter that publishes a change event so
T-002-03 sliders can subscribe.

**Pros.** Cleaner reactive pattern.

**Cons.** Speculative — T-002-03 hasn't shipped, may use a different
mechanism, and the ticket forbids T-002-03 work landing here.

**Rejected.** Build it when T-002-03 actually needs it.

## Resolving the schema/literal divergences

Two literals already disagree with T-002-01's schema defaults:

### Divergence 1: `key_light_intensity` (1.4 vs 1.6)

- `setupBakeLights:363` uses `1.4`. Schema matches.
- `renderLayerTopDown:616` uses `1.6`. Schema does **not** match.

T-002-01's review.md explicitly accepted this delta as a documented one-
tick visual difference for T-002-02 to land. Volumetric layer baking is
a tiered/clipped render where the +0.2 was a localized hand-tune, not a
distinct conceptual axis. The "right" long-term answer is probably either
(a) two separate fields (`key_light_intensity_billboard` /
`key_light_intensity_volumetric`) or (b) accept that one number governs
both. We pick (b) because:
- It honors T-002-01's design.
- The user-visible delta on a single bake is small.
- Adding axes is cheap later if the tuning panel reveals a real need.

**Action.** Wire line 616 to `currentSettings.key_light_intensity`. No
schema change. Note the +0.2 absence in `progress.md` and `review.md`.

### Divergence 2: `alpha_test` (0.1 vs 0.15)

- Three bake-export sites (491, 518, 745) all use `0.1`.
- The schema default is `0.15`.
- A separate runtime override at line 1751 uses `0.15` and is **not**
  in scope for this ticket.

The bake-export literals are baked **into the persisted GLBs**. If we use
the schema default `0.15` here, freshly-baked GLBs would have an
alpha-cutoff 50% tighter than today, which makes foliage-edge alpha
fringes more aggressive. That is a visible regression on the rose asset.

The ticket's regression-check directive applies: "If existing assets
render differently after the inversion, the schema defaults are wrong —
fix them in T-002-01's schema doc." So:

**Action.** Update three places to set the canonical bake-time alpha
default to `0.10`:
- `settings.go` `DefaultSettings()` → `AlphaTest: 0.10`
- `docs/knowledge/settings-schema.md` defaults table → `0.10`
- `settings_test.go` if it asserts on the literal value (it doesn't —
  it only asserts that defaults *validate*, which `0.10` still does)

Then wire all three bake-export literals to
`currentSettings.alpha_test`. The runtime override at 1751 is unchanged
(stays `0.15`); it's a different concept (instance-time alpha cutoff for
the loaded GLB) and conflating them now would expand scope.

This *technically* is a schema-doc edit in T-002-01's territory, but the
ticket explicitly authorizes it. The schema range stays `[0,1]`, the
field validates the same, and the migration policy says reflowing a
default does not bump `schema_version`.

## API contracts

### `currentSettings` shape

Identical to the JSON returned by `GET /api/settings/:id`. Snake-case
field names. Example:

```js
{
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
  lighting_preset: "default",
}
```

### `loadSettings(id)`

```js
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
```

Always resolves with a non-null `currentSettings`.

### `saveSettings(id)` — debounced

```js
let _saveSettingsTimer = null;
function saveSettings(id) {
  if (_saveSettingsTimer) clearTimeout(_saveSettingsTimer);
  _saveSettingsTimer = setTimeout(async () => {
    _saveSettingsTimer = null;
    if (!currentSettings) return;
    try {
      await fetch(`/api/settings/${id}`, {
        method: 'PUT',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify(currentSettings),
      });
    } catch (err) {
      console.warn(`saveSettings(${id}) failed:`, err);
    }
  }, 500);
}
```

500 ms trailing edge — fast enough to feel responsive, slow enough that
slider drags collapse to one PUT. This function is **not called from this
ticket's code paths** (no UI yet), but the contract has to exist for
T-002-03.

### `applyDefaults()`

A literal that mirrors `DefaultSettings()` from `settings.go`. Yes, this
duplicates the constants — the alternative would be a third HTTP call to
hit some `/api/settings/defaults` endpoint, which doesn't exist and isn't
worth adding. Keep them in sync by convention; flag the duplication in
`review.md` as a known coupling.

## Regression strategy

1. Before any code changes: bake the rose with the **current main** code
   and capture screenshots of the live preview for billboard, volumetric,
   and production asset variants.
2. Apply the changes.
3. Bake the rose again with default settings (no settings file present —
   GET returns defaults). Capture the same screenshots.
4. Diff visually. Acceptable: pixel-level noise from PMREM regen and
   tone-mapping rounding. Not acceptable: foliage shape changes, alpha
   fringe shifts, brightness shifts.
5. The remaining `1.4 vs 1.6` delta on volumetric will produce a small
   global brightness drop on the volumetric layers. Document the
   magnitude in `progress.md`.

The comparison is manual / visual. There is no automated rendering test
infrastructure and adding one is out of scope (and would belong in
S-006/S-008, not here).

## Why no parameter for `cloneModelForBake`?

The function is called from `renderBillboardAngle`,
`renderBillboardTopDown`, `renderLayerTopDown`, and `testLighting`
(diagnostic — should retain its current behavior). To honor the
diagnostic, `cloneModelForBake` reads from `currentSettings` directly,
and the diagnostic call from `testLighting:881` accepts the wired value.
That is acceptable: a developer running `testLighting` mid-tuning *wants*
to see what the current settings produce. Document this nuance.

## What is **not** changed

- Server: zero changes (besides the `settings.go` default-flip for
  `AlphaTest`).
- Schema version: stays `1`.
- File layout: no new JS modules, no new files. Pure edits to `app.js`,
  `settings.go`, `docs/knowledge/settings-schema.md`.
- Other tickets' scopes (T-002-03 UI, S-005 new params, S-007 lighting
  presets).
