# Plan — T-007-02

Six commits, each independently revertable. Each step ends with
`go build ./... && go test ./...` (the touched code is JS but the Go
build is the cheapest sanity check the agent can run).

## Step 1 — Refactor `getActiveBakePalette` to share a helper

**Files**: `static/app.js`

**Change**: introduce a pure helper `resolvePresetColors(cfg)` that
takes a `bake_config`/`preview_config`-shaped object and returns
`{bright, mid, dark, key, fill}`. Refactor the body of
`getActiveBakePalette` to call it. Behavior is unchanged — the bake
still gets the same `{bright, mid, dark}` values out (the new fields
`key` and `fill` are added to the return shape so the live-preview
helper can read them too; bake call sites don't read those keys
yet).

**Verify**: `go build ./... && go test ./...` clean. Manual:
loading the page should still bake correctly (visual smoke test
deferred to step 6 manual run).

**Commit**: `Refactor bake palette resolution helper (T-007-02)`

## Step 2 — Add live-scene helpers as dead code

**Files**: `static/app.js`

**Change**: define
- `getActivePreviewPalette()` — mirrors `getActiveBakePalette` but
  reads `preview_config`. Falls back through reference → preset →
  neutral exactly like the bake.
- `applyPresetToLiveScene()` — `if (!scene) return;` then
  `scene.traverse` and mutate AmbientLight, HemisphereLight, and
  the three DirectionalLights per the mapping in structure.md D3.
  Identifies dirLight2 by `position.x < 0`, dirLight3 by
  `position.y < 0`, dirLight by neither.
- `setBakeStale(stale)` — `bakeStale = !!stale;` then toggles the
  `#bakeStaleHint` element's `display`. Module-level
  `let bakeStale = false;` declared near the top.

No call sites added yet — dead code.

**Verify**: `go build ./... && go test ./...` clean. JS still loads
(no syntax errors).

**Commit**: `Add live-scene preset helpers (T-007-02)`

## Step 3 — Hand-tune per-preset preview_config

**Files**: `static/presets/lighting.js`

**Change**: extend `makePreset` to accept an optional
`preview_overrides` argument. After cloning `bake_config` into
`preview_config`, merge any overrides on top. Then specify the
overrides per design.md D7:
- `default`: none.
- `midday-sun`: none.
- `overcast`: `ambient: 0.72, hemisphere_intensity: 0.90`.
- `golden-hour`: `key_intensity: 1.36`.
- `dusk`: `key_intensity: 0.55`.
- `indoor`: none.

**Verify**: `go build ./... && go test ./...` clean. Reload the
page; the dev-time assertion in `lighting.js` (which keys off
`bake_config.ambient` etc, NOT `preview_config`) should still pass
silently — confirm by reading the assertion source.

**Commit**: `Tune preview_config per preset (T-007-02)`

## Step 4 — Add the rebake hint DOM element

**Files**: `static/index.html`

**Change**: insert
```html
<span id="bakeStaleHint" class="bake-stale-hint" style="display:none">
  Bake out of date — regenerate to apply preset
</span>
```
into `.toolbar-actions` between `generateProductionBtn` and
`uploadReferenceBtn`. Add a small CSS rule
`.bake-stale-hint { color: #f5b942; font-size: 12px; padding: 0 8px; }`
to the existing `<style>` block (or wherever the toolbar styles
live; if no `<style>` block, add one to the `<head>`).

**Verify**: page loads, the element is in the DOM but hidden
(`document.getElementById('bakeStaleHint').style.display === 'none'`).

**Commit**: `Add stale-bake hint element (T-007-02)`

## Step 5 — Wire helpers into the live preview lifecycle

**Files**: `static/app.js`

**Change**: add the call sites:

1. In `applyLightingPreset`, after `populateTuningUI()`, call
   `applyPresetToLiveScene()` and `setBakeStale(true)`.
2. In `selectFile`'s `loadSettings(id).then` callback, after
   `populateTuningUI()`, call `applyPresetToLiveScene()` and
   `setBakeStale(false)`.
3. In `applyColorCalibration`, in the tear-down (else) branch,
   replace `resetSceneLights()` with `applyPresetToLiveScene()`.
4. In each of the four regenerate paths
   (`generateBillboard`, `generateVolumetric`,
   `generateVolumetricLODs`, `generateProductionAsset`), inside the
   `success = true;` branch (or right before the `finally` /
   `logEvent('regenerate', ...)`), call `setBakeStale(false)`.

**Verify**: `go build ./... && go test ./...` clean. Manual: load
the page, switch presets — live preview should change colors;
regenerate any asset — hint should disappear.

**Commit**: `Wire preset application into live preview (T-007-02)`

## Step 6 — Extend reference-image tint to all directionals

**Files**: `static/app.js`

**Change**: in `applyReferenceTint(palette)`, extend the
`scene.traverse` to also handle `obj.isDirectionalLight`. Map by
position the same way `applyPresetToLiveScene` does:
- dirLight (default position) → `palette.bright`
- dirLight2 (`position.x < 0`) → `palette.bright`
- dirLight3 (`position.y < 0`) → `palette.mid`

Leave intensities untouched (T-007-02 does not retune the reference
calibration intensity scale).

**Verify**: `go build ./... && go test ./...` clean. Manual: with
a reference image previously uploaded for the rose, switch to that
asset and confirm directional lights pick up the calibrated tint.

**Commit**: `Extend reference image tint to directional lights (T-007-02)`

## Testing strategy

- **Go**: nothing to add. Repo Go tests must remain green after
  every step.
- **JS**: no harness exists. The dev-time assertion in
  `static/presets/lighting.js` provides drift detection for the
  `default` preset. We rely on:
  - Step 1's refactor preserving `getActiveBakePalette`'s output
    shape (verified by reading the new helper).
  - Step 2's dead code being syntactically loadable.
  - Step 5/6's call site additions being grep-checkable.
- **Manual checklist** (for review.md):
  1. Load any asset.
  2. Pick `golden-hour` from the preset dropdown — the live preview
     should immediately turn warm/orange-tinted, the rebake hint
     should appear.
  3. Pick `dusk` — live preview should shift cool/blue, hint stays.
  4. Click "Production Asset" → wait for completion → hint clears.
  5. Pick `default` — live preview should return to the
     pre-ticket neutral look, hint reappears.
  6. Reload the page — hint should NOT persist (state is
     in-memory only per design.md D6).
  7. Upload a reference image → confirm directional lights also
     adopt the calibrated tint, not just ambient + hemisphere.

## Rollback plan

Each commit is small and orthogonal. To roll back, revert the
relevant commit. Step 5 (the wiring) is the riskiest — reverting
just step 5 leaves the helpers in place as dead code and the live
preview goes back to the pre-ticket neutral lighting.
