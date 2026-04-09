# T-004-03 — Review

Self-assessment of the shape-strategy-router work.

## What changed

### Files created

- **`strategy.go`** (~115 lines) — `ShapeStrategy` struct, the
  closed `shapeStrategyTable` lookup, `getStrategyForCategory()`,
  and the `SliceAxis*` sentinel constants. Pure module, no I/O,
  no globals other than the lookup table.
- **`strategy_test.go`** (~95 lines) — five black-box tests:
  every taxonomy member resolves; unknown / corrupt input falls
  through to `"unknown"`; pinning tests for the directional axis,
  hard-surface NA sentinel, and the round-bush-matches-defaults
  invariant.
- **`strategy_handlers_test.go`** (~115 lines) — three end-to-end
  tests of the stamping behavior in `applyClassificationToSettings`:
  fresh-asset stamping, user-override preservation, and
  hard-surface-leaves-slice-fields-alone.
- **`docs/active/work/T-004-03/{research,design,structure,plan,progress,review}.md`**
  — RDSPI artifacts.

### Files modified

- **`settings.go`** — `SliceAxis` field appended to `AssetSettings`,
  `validSliceAxes` enum, default of `"y"`, validation, dirty-tracking,
  forward-compat normalization in `LoadSettings`.
- **`analytics.go`** — `"strategy_selected"` registered as a v1
  event type. Additive change, no schema bump.
- **`handlers.go`** — `applyShapeStrategyToSettings` helper,
  `applyClassificationToSettings` integrates the stamp,
  `emitStrategySelectedEvent` helper, and both classification call
  sites (`autoClassify`, `handleClassify`) emit the new event.
- **`static/app.js`** — `makeDefaults()` carries `slice_axis`,
  `TUNING_SPEC` reserves a dormant row for it, new helper
  `resolveSliceAxisRotation`, and `renderHorizontalLayerGLB` slices
  in a rotated working frame for non-Y axes.
- **`settings_test.go`** — slice-axis enum cases added to
  `TestValidate_RejectsOutOfRange`, new
  `TestValidate_AcceptsAllSliceAxes`,
  `TestLoadSettings_OldDocMissingSliceAxis`, and a slice_axis check
  in `TestSettingsDifferFromDefaults_ShapeFields`.
- **`analytics_test.go`** — `TestEventValidate_AcceptsStrategySelectedType`.
- **`docs/knowledge/settings-schema.md`** — `slice_axis` row.
- **`docs/knowledge/analytics-schema.md`** — `strategy_selected`
  event section.

### Files deleted

None.

## Test coverage

### Unit tests (Go)

- **Router** — every taxonomy member resolves to a non-empty
  strategy whose `Category` field matches; unknown inputs fall
  through to `"unknown"`; directional axis pinned to
  `auto-horizontal`; hard-surface slice fields pinned to the
  `n/a` sentinel; round-bush pinned to `DefaultSettings()` so the
  rose-asset baseline is preserved.
- **Settings** — `SliceAxis` enum check, all-axes acceptance,
  forward-compat normalization for documents missing the key,
  dirty-tracking flips on `SliceAxis` mutation.
- **Analytics** — `strategy_selected` envelope passes `Validate()`.
- **Stamping** — directional classification stamps the strategy
  end-to-end through round-trip; user override of
  `slice_distribution_mode` survives re-classification while a
  still-default `SliceAxis` gets stamped; hard-surface
  classification leaves all slice fields untouched.

### Tests not added

- **Frontend bake rotation.** `static/app.js` has no JS test
  harness in this project, so `resolveSliceAxisRotation` and the
  wrapper / reframe path in `renderHorizontalLayerGLB` are covered
  only by the manual verification step. **This is the biggest
  coverage gap in the change** — see "Open concerns" below.
- **`emitStrategySelectedEvent`.** No direct test for the
  analytics emission helper. Mirrors `emitClassificationEvent`
  one-for-one (which itself has no direct test), and the failure
  mode is "stderr log on best-effort failure", so the regression
  cost is low.

## Open concerns

1. **Manual verification of the bake rotation has not been
   performed.** The directional code path in
   `renderHorizontalLayerGLB` builds a wrapper Group with the
   forward rotation, slices in that frame, then re-frames the
   exportScene under an inverse-rotation wrapper before passing it
   to `GLTFExporter`. The math is straightforward but the
   exporter / scene-graph plumbing is the riskiest part of the
   diff. The plan's verification step (upload
   `assets/wood_raised_bed.glb`, classify, bake, view in a GLB
   viewer, confirm slice planes are perpendicular to the long
   horizontal axis) is the load-bearing acceptance check and
   needs a human to run it before this ticket is mergeable.
   Same for the round-bush regression check.

2. **`InstanceOrientationRule` and `DefaultBudgetPriority` are
   not persisted.** They live on `ShapeStrategy` (and are emitted
   in the `strategy_selected` analytics payload) but are not
   stamped onto `AssetSettings`. No consumer needs them in this
   ticket — S-006 scene preview reads orientation rules and
   T-004-04 reads budget priority. When those tickets land they
   should call `getStrategyForCategory(s.ShapeCategory)` directly
   rather than persisting more fields.

3. **Stamping happens *only* during classification.** A user
   who deletes their settings file and *doesn't* re-classify will
   never see the strategy applied to that asset (they'll get
   `DefaultSettings()` instead). This is consistent with how
   `ShapeCategory` itself works today, and the upload-time
   `autoClassify` hook means the only way to land in this state
   is to manually delete the settings file without restarting —
   acceptable.

4. **The frontend wrapper-and-reframe path mutates the model's
   parent temporarily.** `workModel.remove(model)` after the slice
   loop returns the original `model` to a parentless state. This
   matches its pre-bake state (the bake function receives a
   fresh `currentModel` reference), but a caller that re-uses the
   same model reference across two bake calls without
   re-attaching it to its scene would see it appear detached.
   Given the call sites (one-shot LOD generation), this is fine,
   but worth flagging for any future refactor.

## Critical issues for human attention

- **Run the manual verification before merging** (concern #1
  above). Specifically:
  1. Start the server, upload `assets/wood_raised_bed.glb`.
  2. Browser console: confirm
     `currentSettings.shape_category === 'directional'` and
     `currentSettings.slice_axis === 'auto-horizontal'`.
  3. Trigger a volumetric bake; download the produced GLB; load
     in a third-party viewer (e.g. https://gltf-viewer.donmccurdy.com/)
     and confirm the slice planes are perpendicular to the bed's
     long horizontal axis.
  4. Upload `assets/rose_julia_child.glb`; confirm
     `slice_axis === 'y'` and the bake produces visually identical
     output to pre-T-004-03 (regression check).

## Build / test status

- `go build ./...` — clean.
- `go vet ./...` — clean.
- `go test -count=1 ./...` — green.
- `static/app.js` — no automated tests; relies on manual
  verification.

## Acceptance criteria — status

| Criterion                                                            | Status |
|----------------------------------------------------------------------|--------|
| `getStrategyForCategory(category)` exists and returns full struct    | done   |
| Strategy mappings for round-bush, directional, tall-narrow, planar, hard-surface | done   |
| Bake reads strategy via settings (slice_axis, slice_distribution_mode, slice_count) | done (slice_axis via app.js; slice_count + slice_distribution_mode via existing settings plumbing) |
| User can override strategy defaults per-setting                      | done (override semantics tested) |
| `strategy_selected` analytics event with `{category, strategy}`      | done   |
| Manual verification: directional asset bake slices perpendicular     | **pending human verification** |
