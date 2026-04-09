# Review — T-002-02: Wire app.js bake constants to settings

## Summary

Inverted every hardcoded volumetric/billboard bake constant in
`static/app.js` to read from a per-asset `currentSettings` object that is
loaded from `GET /api/settings/:id` in `selectFile`. Added the
`loadSettings` / `saveSettings` (debounced) / `applyDefaults` helpers that
T-002-03 will consume from its UI. No new UI was added. The bake pipeline
is now data-driven end to end.

The change also flipped the v1 schema default for `alpha_test` from
`0.15` to `0.10` so that bake-export GLBs stay regression-free with
default settings (the three bake-export sites all used `0.1`). The
runtime instance-time alpha overrides at lines 1661/1683/1751 are
deliberately untouched — they are a different concept (instance-creation
material reconfiguration) and conflating them with the export-time
threshold would have expanded scope.

## Files Changed

| File | Action | Notes |
|------|--------|-------|
| `static/app.js` | MODIFY | New State + helper section, 14 literal-replacement edits, `selectFile` await |
| `settings.go` | MODIFY | `AlphaTest` default `0.15 → 0.10` |
| `docs/knowledge/settings-schema.md` | MODIFY | Defaults table + JSON example synced; field description clarified to call out the runtime override that this field does **not** govern |
| `docs/active/work/T-002-02/*.md` | CREATE | Full RDSPI artifact set (this set) |

No deletions, no new files outside the work artifacts. No changes to
`handlers.go`, `main.go`, `models.go`, `processor.go`, `blender.go`,
`scene.go`, `static/index.html`.

## Acceptance Criteria — Status

- [x] `currentSettings` global, populated from `/api/settings/:id` on
      file select.
- [x] `loadSettings(id)` async fetch + assign + return; falls back to
      `applyDefaults()` on any error.
- [x] `saveSettings(id)` PUT, **debounced** (500 ms trailing edge); used
      later by T-002-03.
- [x] `applyDefaults()` resets `currentSettings` to a literal mirror of
      `DefaultSettings()` in `settings.go`.
- [x] `VOLUMETRIC_LAYERS`, `VOLUMETRIC_RESOLUTION` consts removed; both
      call sites in `generateVolumetric` and `generateProductionAsset`
      read from `currentSettings`.
- [x] `domeHeight = layerThickness * 0.5` literal replaced with
      `layerThickness * currentSettings.dome_height_factor`.
- [x] All `toneMappingExposure = 1.0` instances in the **bake**
      renderers replaced with `currentSettings.bake_exposure`. Diagnostic
      and live-preview exposures intentionally untouched.
- [x] All bake `AmbientLight` intensities (in `setupBakeLights` and
      `renderLayerTopDown`) read from `currentSettings.ambient_intensity`.
- [x] All bake `DirectionalLight` intensities read from
      `currentSettings.key_light_intensity` /
      `currentSettings.bottom_fill_intensity`. The `1.6` literal in
      `renderLayerTopDown` collapses to `1.4` (the schema default) — a
      documented +0.2 delta inherited from T-002-01's review.
- [x] `HemisphereLight` intensities (both bake sites) read from
      `currentSettings.hemisphere_intensity`.
- [x] `envMapIntensity = 1.2` in `cloneModelForBake` reads from
      `currentSettings.env_map_intensity`.
- [x] `alphaTest` values in **bake-export** volumetric/billboard
      materials (lines 491, 518, 745 pre-edit) read from
      `currentSettings.alpha_test`.
- [x] All bake functions read `currentSettings` directly (one chosen
      pattern, used consistently — see design.md).
- [x] `selectFile()` `await`s `loadSettings()` before `loadModel()`.
- [x] File switch resets `currentSettings` to the new file's saved
      values (or defaults). Falls out from `selectFile → loadSettings`.
- [x] **No new sliders, no new buttons.**
- [x] Regression strategy documented in `design.md` and `progress.md`.
      Manual visual smoke test deferred to the human reviewer (no
      headless browser available in this session).

## Test Coverage

### Automated

- `go test ./...` → **PASS** (`ok glb-optimizer 0.309s`).
- `go build ./...` → **PASS** (clean exit).
- The Go test suite covers the `AlphaTest` default flip implicitly:
  - `TestDefaultSettings_Valid` re-validates the defaults — `0.10`
    still validates against `[0,1]`.
  - `TestSaveLoad_Roundtrip` round-trips the new default value byte-
    for-byte.
  - `TestValidate_RejectsOutOfRange` does not assert on the default
    literal, only on out-of-range values.

### Manual (deferred to human reviewer)

The plan documents a step-by-step browser smoke test in `plan.md` §15
covering:

1. Default `GET /api/settings/{id}` returns `alpha_test: 0.10`.
2. Browser `selectFile` populates `currentSettings`.
3. Visual diff of billboard / volumetric / production-asset bakes
   against pre-change main.
4. File-switch round-trip.
5. `saveSettings` end-to-end via console.

### Coverage gaps

- **No JS tests, no JS test infra.** The repo has zero JS test
  scaffolding; adding it for one ticket would dwarf the actual change.
  Recommend a follow-up to pick a JS test runner during S-008.
- **No automated visual regression.** A pixel-diff harness against
  reference bakes would be ideal but is well beyond this ticket — same
  follow-up.
- **No HTTP-handler test for the GET path** (already flagged by
  T-002-01's review). Still cheap to add later.

## Open Concerns

1. **Documented +0.2 directional-light delta on volumetric bake**.
   `renderLayerTopDown` previously hand-tuned its key light to `1.6`
   while `setupBakeLights` (used by billboards) used `1.4`. T-002-01's
   schema collapsed both to one field with default `1.4`, accepting the
   delta. Result: default-settings volumetric bakes are ~14% dimmer than
   they were pre-T-002-02. Two ways to recover, both out of scope:
   - Bump the schema default to `1.6` (regresses billboards in the
     other direction).
   - Split into two fields. Worth doing if T-002-03 tuning surfaces a
     real need.

2. **`alpha_test` field is overloaded by name only**. The schema field
   governs the bake-export literals; the runtime instance-time overrides
   at lines 1661/1683/1751 use independent values (0.5, 0.5, 0.15) that
   are *not* exposed by the schema. The schema doc was updated to call
   this out explicitly. Future readers may still be confused — a
   follow-up could rename to `bake_alpha_test` if T-002-03's UI surface
   the distinction.

3. **`testLighting` semantic shift**. The diagnostic at line ~841 calls
   `setupBakeLights` and `cloneModelForBake`, which now read
   `currentSettings`. Previously the diagnostic was constant; now it
   reflects the active asset's tuning. Arguably correct (you're testing
   the *current* bake config) but worth flagging — somebody chasing a
   regression by re-running the diagnostic on different assets will see
   different numbers.

4. **`applyDefaults` duplicates `DefaultSettings()`**. The JS literal
   has to be hand-kept-in-sync with `settings.go`. Cheap to fix with a
   `/api/settings/defaults` endpoint (or by serving an empty/sentinel id
   from the existing GET handler), but not in scope. Mitigation: every
   field name is also a JSON field that the round-trip path exercises,
   so a typo in the JS literal would cause an immediate visual
   regression.

5. **`saveSettings` is unreachable from this ticket's code paths**. It
   exists for T-002-03. Smoke-testable via the console
   (`saveSettings(selectedFileId)` after mutating `currentSettings`).

6. **No `loadSettings` rejection path during `selectFile`**. If the
   GET races a server restart, `loadSettings` falls back to
   `applyDefaults` and logs a warning, then `loadModel` proceeds. The
   user will see defaults applied — graceful but possibly surprising.
   Acceptable for a single-user dev tool.

## Things a Human Reviewer Should Check

- **Run the smoke test in `plan.md` §15** before merging. Specifically,
  bake the rose with default settings on this branch and visually
  diff against a pre-change bake. The volumetric pass *will* be
  slightly dimmer (the +0.2 delta); everything else should be
  indistinguishable.
- **Confirm the schema-doc edit is acceptable**. T-002-02 modified
  T-002-01's schema default for `alpha_test`. The ticket explicitly
  authorized this, but it's the kind of cross-ticket touch worth
  acknowledging in review.
- **Check `index.html` is untouched**. It is, but worth confirming.
- **Run `go test ./...`** locally — the CI guarantee here is just that
  it passes in this session.
- **Spot-check the new helper section** in `app.js` for the
  field-by-field literal in `applyDefaults`. Five fields wrong here
  would mean five subtle bake regressions.

## Known Limitations (Not Bugs)

- The +0.2 volumetric directional delta described above.
- `applyDefaults` constants must be hand-synced with `settings.go`.
- `saveSettings` debounce is fixed at 500 ms (no config). Fine for now.
- No retry on transient HTTP failures in `loadSettings`/`saveSettings`
  — they log + fall through.
- `currentSettings` is mutable shared state; T-002-03 will need to
  remember to call `saveSettings(selectedFileId)` after each mutation.
  A future setter wrapper could automate this, but the contract is
  documented in `design.md`.

## Handoff to T-002-03

T-002-03 can now build a tuning panel by:

1. Reading from `currentSettings.<field>` to populate slider values
   when a file is selected.
2. Writing back to `currentSettings.<field>` and calling
   `saveSettings(selectedFileId)` on each `input` event.
3. Calling `applyDefaults(); saveSettings(selectedFileId)` for a
   "reset to defaults" button.
4. Re-running the relevant bake function (`generateBillboard`,
   `generateVolumetric`, `generateProductionAsset`) on demand to apply
   tuning. Live re-render is explicitly out of scope for T-002-03 too,
   but the plumbing is there.

No further plumbing in `app.js` is required. The contract is final
unless T-002-03 surfaces a real need to evolve it.
