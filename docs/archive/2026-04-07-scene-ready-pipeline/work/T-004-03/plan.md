# T-004-03 — Plan

Sequenced steps. Each step is independently committable and verifiable.

## Step 1 — `strategy.go` + unit tests

**What.** Create `strategy.go` with `ShapeStrategy` struct, the
`shapeStrategyTable` lookup, the `SliceAxis*` constants, and
`getStrategyForCategory(category) ShapeStrategy`. Create
`strategy_test.go` with the cases enumerated in structure.md.

**Verification.** `go test ./... -run TestGetStrategyForCategory` and
`-run TestStrategyTable` passes. The package still builds.

**Commit.** "Add shape-strategy router and lookup table (T-004-03)"

## Step 2 — `settings.go`: `SliceAxis` field

**What.**
- Append `SliceAxis string \`json:"slice_axis,omitempty"\`` to
  `AssetSettings`.
- Add `validSliceAxes` map.
- `DefaultSettings()`: `SliceAxis: "y"`.
- `Validate()` enum check.
- `SettingsDifferFromDefaults()` clause.
- `LoadSettings()` forward-compat: `if s.SliceAxis == "" { s.SliceAxis = "y" }`.

**Verification.** Add to `settings_test.go`:
- `TestValidate_RejectsBadSliceAxis`
- `TestValidate_AcceptsAllValidSliceAxes`
- `TestLoadSettings_OldDocMissingSliceAxis` — write a JSON doc with no
  `slice_axis` key, load, assert `"y"`.
- `TestSettingsDifferFromDefaults_SliceAxis`.

`go test ./...` is green.

**Commit.** "Persist slice_axis on AssetSettings (T-004-03)"

## Step 3 — `analytics.go`: new event type

**What.**
- Add `"strategy_selected": true` to `validEventTypes`.

**Verification.** Add `TestEventValidate_StrategySelected` to
`analytics_test.go` mirroring the `classification` case.

**Commit.** "Register strategy_selected analytics event (T-004-03)"

## Step 4 — `handlers.go`: stamp + emit

**What.**
- Add `applyShapeStrategyToSettings(s *AssetSettings, strategy ShapeStrategy)`.
- Modify `applyClassificationToSettings` to call it after the
  shape-field assignment, before `Validate`.
- Add `emitStrategySelectedEvent(logger, id, strategy)` mirroring
  `emitClassificationEvent`.
- `autoClassify` and `handleClassify`: call
  `emitStrategySelectedEvent` immediately after the existing
  `emitClassificationEvent`.

**Verification.** Add a new test file `strategy_handlers_test.go`:
- `TestApplyClassificationStampsStrategy_Directional` — uses
  `t.TempDir()` as the settings dir, calls
  `applyClassificationToSettings(id, dir, &ClassificationResult{Category:"directional", Confidence:0.9})`,
  reloads, asserts `SliceAxis == "auto-horizontal"`,
  `SliceDistributionMode == "equal-height"`,
  `VolumetricLayers == 4`.
- `TestApplyClassificationPreservesUserOverride` — pre-write a
  settings file with custom `SliceDistributionMode = "vertex-quantile"`,
  classify as `directional`, assert the user value survives but
  `SliceAxis` is stamped (because it was at default).
- `TestApplyClassificationHardSurfaceLeavesSliceFieldsAlone` — start
  from defaults, classify as `hard-surface`, assert
  `SliceDistributionMode` and `SliceAxis` are unchanged from defaults.

`go test ./...` is green.

**Commit.** "Stamp shape strategy onto settings during classify (T-004-03)"

## Step 5 — `static/app.js`: bake reads `slice_axis`

**What.**
- Add `slice_axis: 'y'` to `makeDefaults()`.
- Add a `TUNING_SPEC` row for `slice_axis` (DOM id `tuneSliceAxis`,
  parse/fmt as identity strings). No HTML change — the spec entry
  is dormant until a future ticket adds the control.
- New helper `resolveSliceAxisRotation(model, mode)` returning a
  three-element `{rotation, inverse}` (each a `THREE.Quaternion` or
  `THREE.Euler`):
  - `'y'` → identity.
  - `'auto-horizontal'` → pick the longer of size.x / size.z, build
    a quaternion that rotates that axis to +Y.
  - `'auto-thin'` → pick the smallest of size.x / size.y / size.z,
    rotate to +Y.
- `renderHorizontalLayerGLB`:
  - Compute `{rotation, inverse} = resolveSliceAxisRotation(model, currentSettings.slice_axis || 'y')`.
  - If non-identity: clone the model into a temporary
    `THREE.Group`, apply `rotation`, use that group as the slicing
    target throughout the existing pipeline.
  - After the existing slicing loop, set the `exportScene`'s
    `quaternion` (or rotation) to `inverse` so the exported GLB
    sits in the original world frame.
  - Ground-align (existing branch) continues to operate on
    `boundaries[0]` of the rotated frame; this is correct because
    the slice planes are still horizontal in the rotated space.

**Verification (manual).**
1. `go build && ./glb-optimizer`.
2. Upload `assets/wood_raised_bed.glb` (classified `directional` per
   T-004-02 spike data).
3. Open the asset; in the browser console, check
   `currentSettings.slice_axis === 'auto-horizontal'`.
4. Trigger a volumetric bake; download the produced GLB; load it
   in any GLB viewer; confirm the slice planes are perpendicular
   to the bed's long horizontal axis (not Y).
5. Upload `assets/rose_julia_child.glb`; confirm `slice_axis ===
   'y'` and the bake produces the same Y-sliced output as before
   T-004-03 (regression check).

**Commit.** "Bake slices along strategy slice_axis (T-004-03)"

## Step 6 — Documentation

**What.**
- `docs/knowledge/settings-schema.md`: add `slice_axis` row with
  enum and forward-compat note.
- `docs/knowledge/analytics-schema.md`: add `strategy_selected`
  section with payload table.

**Verification.** `markdownlint` clean if available; otherwise visual
review.

**Commit.** "Document slice_axis and strategy_selected event (T-004-03)"

## Testing strategy summary

**Unit tests (Go).**
- Router: every category resolves; unknown falls through; pinning
  tests for the directional axis and the round-bush match-defaults
  contract.
- Settings: enum validation, round-trip, forward-compat,
  difference-from-defaults.
- Analytics: envelope validates with the new event type.
- Stamping: stamps on default fields, preserves user overrides,
  leaves slice fields alone for hard-surface.

**Integration tests.** None automated. The bake function runs in
the browser and is exercised by the manual verification step.

**Manual verification criteria.**
- Directional asset: slice planes are perpendicular to the long
  horizontal axis of the model.
- Round-bush asset: bake output is byte-for-byte
  *behaviorally* identical to pre-T-004-03 (slice axis Y, same
  defaults). Allowed to differ on the new persisted `slice_axis`
  key.
- Re-classification of an asset whose user has tuned
  `slice_distribution_mode` does **not** revert that field.

## Risks / open

- The pre-slice rotation in `renderHorizontalLayerGLB` is the
  highest-risk piece. If the inverse rotation is applied to the
  wrong scene-graph node, the exported GLB renders tilted. The
  manual verification step (download + view in a third-party
  viewer) is what catches it.
- `static/app.js` has no automated test harness; this ticket does
  not introduce one.
