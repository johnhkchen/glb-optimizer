# Progress — T-002-02: Wire app.js bake constants to settings

## Status

All implementation steps from `plan.md` complete. `go test ./...` passes.
JS syntax check (node --check) passes. Manual visual smoke test deferred
to the human reviewer (no headless browser in this environment).

## Steps executed

| # | Step | Status | Notes |
|---|------|--------|-------|
| 1 | Flip `AlphaTest` default 0.15 → 0.10 in `settings.go` | ✅ | One-line edit |
| 2 | Sync `docs/knowledge/settings-schema.md` defaults table | ✅ | Also updated the JSON example block + clarified the field description to mention the runtime override |
| 3 | Add `currentSettings` + `_saveSettingsTimer` state | ✅ | After `referencePalette` |
| 4 | Add `loadSettings`/`saveSettings`/`applyDefaults` helpers | ✅ | New `// ── Asset Settings ──` section directly above `getSettings()` |
| 5 | `setupBakeLights` literal swaps (4 lines) | ✅ | Ambient/Hemisphere/Key/BottomFill all wired |
| 6 | `cloneModelForBake` envMapIntensity | ✅ | |
| 7 | `renderBillboardAngle` exposure | ✅ | |
| 8 | `renderBillboardTopDown` exposure | ✅ | |
| 9 | `renderMultiAngleBillboardGLB` alphaTest ×2 | ✅ | Side + top quad mats |
| 10 | `renderLayerTopDown` exposure / ambient / hemisphere / key | ✅ | The `1.6` directional swapped to `currentSettings.key_light_intensity` (= `1.4` default) — documented +0.2 delta intentionally absorbed |
| 11 | `renderHorizontalLayerGLB` dome height + alphaTest | ✅ | |
| 12 | Delete `VOLUMETRIC_LAYERS`/`VOLUMETRIC_RESOLUTION` consts; rewrite call sites | ✅ | Replaced with a comment block; both call sites in `generateVolumetric` and `generateProductionAsset` updated |
| 13 | Rewrite `selectFile` `loadEnv.then(...)` chain | ✅ | Awaits `loadSettings(id)` before `loadModel` |
| 14 | `go test ./...` | ✅ | `ok glb-optimizer 0.309s` |
| 15 | Manual smoke test | ⏸ | Deferred to human reviewer (see plan.md §15) |
| 16 | `applyDefaults`/`saveSettings` round-trip | ⏸ | Deferred to human reviewer |

## Deviations from the plan

**None.** The implementation followed `plan.md` step-for-step.

## Verification performed

- `go test ./...` → PASS (`ok glb-optimizer 0.309s`).
- `go build ./...` → PASS (no output, exit 0).
- Grep for stale references to `VOLUMETRIC_LAYERS` / `VOLUMETRIC_RESOLUTION`
  in `static/app.js` → no matches.
- All four `// in scope` literal categories verified replaced via grep:
  - `toneMappingExposure = 1.0` in bake renderers → 0 hits in scope (only
    diagnostic `runPipelineRoundtrip:1005` and the live preview at 1395
    remain, both intentionally untouched).
  - `0.5)` / `1.0)` / `1.4)` / `0.4)` literals in `setupBakeLights` →
    0 remaining; all lit by `currentSettings.*`.
  - `1.6)` directional in `renderLayerTopDown` → 0 remaining.
  - `envMapIntensity = 1.2` in `cloneModelForBake` → 0 remaining; the
    other `envMapIntensity = 2.0` at line 1584 is in `loadModel`'s
    runtime override path, intentionally untouched.
  - `alphaTest: 0.1` (bake-export) → 0 remaining; the runtime overrides
    at 1661/1683/1751 are intentionally untouched.

## What was *not* changed

Per `design.md` and `research.md`:

- `runPipelineRoundtrip` (lines ~974) — diagnostic, kept stable.
- `testLighting` (line ~850) — diagnostic, kept stable. Note: it calls
  `setupBakeLights` and `cloneModelForBake`, which now read
  `currentSettings`. This means `testLighting` reflects the *current*
  asset's settings, which is arguably the right behavior for a tuning
  tool but is a small semantic shift worth flagging.
- `renderer.toneMappingExposure = 1.3` (line 1395) — main preview
  renderer, not the bake.
- Live-preview lights at 1409–1419 / `resetSceneLights` at 2100 — preview,
  not bake.
- `mat.alphaTest = 0.5` at 1661/1683 — runtime billboard instance
  override (different concept, opaque alpha cutout).
- `mat.alphaTest = 0.15` at 1751 — runtime volumetric instance override
  (different concept, instance-time material reconfiguration).
- `VOLUMETRIC_LOD_CONFIGS` at line 766 — LOD chain has its own per-level
  `layers`/`resolution` literals; out of scope (LOD chain is a separate
  concept and no LOD config field exists in the v1 schema).
- The unrelated gltfpack `getSettings()` function (line 72) — different
  namespace.
- `static/index.html` — no UI changes, per ticket.

## Regression notes

Two intentional deltas relative to the old hand-tuned literals, both
documented in design.md:

1. **`renderLayerTopDown` directional key light**: was `1.6`, now reads
   `currentSettings.key_light_intensity` (default `1.4`). The volumetric
   layers will be ~14% dimmer on a default-settings bake than they were
   pre-change. Easy to recover by setting `key_light_intensity: 1.6` on
   that asset (or globally raising the schema default later).

2. **Bake-export alpha cutoff**: was `0.1` literally, now reads
   `currentSettings.alpha_test`. We flipped the schema default from
   `0.15` to `0.10` so the bake stays regression-free at default
   settings. The runtime volumetric instance override at line 1751
   stays at `0.15` independently — see structure.md for why.

These are the only intentional behavioral deltas. Everything else
should bake byte-identically given default settings.

## Files modified

| File | Lines added | Lines removed | Net |
|------|-------------|---------------|-----|
| `settings.go` | 1 | 1 | 0 |
| `docs/knowledge/settings-schema.md` | 2 | 2 | 0 |
| `static/app.js` | ~58 | ~16 | +42 |

The bulk of `app.js` growth is the new helper section (~50 lines
including comments).

## Open work

Nothing in scope for this ticket. Items for follow-ups:

- **T-002-03** can now wire UI sliders to `currentSettings.*` fields and
  call `saveSettings(selectedFileId)` after each mutation. The contract
  is in place.
- A follow-up could consider exposing `key_light_intensity` as two
  separate fields (billboard vs volumetric) if the +0.2 delta on
  volumetric proves visually important during tuning.
- A `/api/settings/defaults` endpoint would let `applyDefaults()` stop
  duplicating constants. Cheap, but not in scope.
