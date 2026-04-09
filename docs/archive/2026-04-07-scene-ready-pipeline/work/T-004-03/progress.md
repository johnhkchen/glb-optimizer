# T-004-03 — Progress

## Completed

- **Step 1.** `strategy.go` + `strategy_test.go` — `ShapeStrategy` struct,
  `shapeStrategyTable` lookup, `getStrategyForCategory()`, sentinel
  constants. Five pinning tests including the directional axis and
  round-bush-matches-defaults invariants. `go test ./...` green.
- **Step 2.** `settings.go` — `SliceAxis` field appended to
  `AssetSettings`, `validSliceAxes` enum, default `"y"`, `Validate()`
  enum check, `SettingsDifferFromDefaults()` clause, `LoadSettings()`
  forward-compat normalization. `settings_test.go` extended with
  `TestValidate_AcceptsAllSliceAxes`,
  `TestLoadSettings_OldDocMissingSliceAxis`, the existing
  `TestValidate_RejectsOutOfRange` table grew two slice-axis cases,
  and `TestSettingsDifferFromDefaults_ShapeFields` got a slice_axis
  assertion.
- **Step 3.** `analytics.go` — `"strategy_selected"` added to
  `validEventTypes`. `analytics_test.go` extended with
  `TestEventValidate_AcceptsStrategySelectedType`.
- **Step 4.** `handlers.go` —
  - New helper `applyShapeStrategyToSettings(s, strategy)` implements
    the override-semantics rule (only stamp fields that are still at
    `DefaultSettings()`).
  - `applyClassificationToSettings` calls the new helper after
    setting the shape fields and before `Validate`.
  - New helper `emitStrategySelectedEvent(logger, id, strategy)`
    mirrors `emitClassificationEvent`.
  - `autoClassify` and `handleClassify` now both call
    `emitStrategySelectedEvent` immediately after
    `emitClassificationEvent`.
  - New test file `strategy_handlers_test.go`:
    `TestApplyClassificationStampsStrategy_Directional`,
    `TestApplyClassificationPreservesUserOverride`,
    `TestApplyClassificationHardSurfaceLeavesSliceFieldsAlone`.
- **Step 5.** `static/app.js` —
  - `makeDefaults()` includes `slice_axis: 'y'`.
  - `TUNING_SPEC` row reserved for `tuneSliceAxis` (no HTML control
    yet; the auto-instrumentation short-circuits on absent elements,
    matching the existing T-005-01 pattern).
  - New helper `resolveSliceAxisRotation(model, mode)` returning
    `{rotation, inverse}` THREE.Quaternions. `'y'` → identity;
    `'auto-horizontal'` → longer of X/Z; `'auto-thin'` → shortest
    bbox axis. Unknown / empty falls through to identity.
  - `renderHorizontalLayerGLB` builds a temporary `THREE.Group`
    wrapper when the rotation is non-identity, runs the existing
    slicing pipeline against the wrapped (rotated) model, then
    re-frames the resulting `exportScene` under a wrapper carrying
    the inverse rotation before passing it to `GLTFExporter`.
- **Step 6.** Documentation —
  - `docs/knowledge/settings-schema.md`: new `slice_axis` row.
  - `docs/knowledge/analytics-schema.md`: new `strategy_selected`
    section with payload table.

## Verification

- `go test -count=1 ./...` — green (1 package, all tests passing).
- `go build ./...` — clean.
- `go vet ./...` — clean.

## Deviations from plan

None. The plan landed step-for-step. The wrapper-and-reframe shape of
the `static/app.js` change is exactly what plan.md described
("rotate model into a Y-aligned working frame, slice as today, then
apply inverse rotation to the export root").

## Open / not-attempted

- The manual verification step (upload `wood_raised_bed.glb`, bake,
  view output in third-party GLB viewer) is **not** automated and
  has not been performed in this session — it requires a running
  server, browser, and human eyes. Flagged in review.md as the
  highest-priority pre-merge task for the human reviewer.
- No tuning UI control for `slice_axis` was added (intentional —
  it lives in the dormant `TUNING_SPEC` row, ready for a future
  ticket to add the `<select>` to `index.html`).
