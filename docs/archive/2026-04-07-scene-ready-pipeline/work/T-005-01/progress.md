# Progress — T-005-01

| # | Step | Status | Notes |
|---|---|---|---|
| 1 | settings.go: fields, defaults, enum, validation | ✅ | `go build` clean |
| 2 | settings.go: LoadSettings forward-compat normalize | ✅ | `*bool` re-decode for explicit-false detection |
| 3 | settings_test.go: new tests | ✅ | `TestDefaultSettings_NewFields`, two new validation cases, `TestLoadSettings_MigratesOldFile`, `TestLoadSettings_ExplicitFalseGroundAlign` (added beyond plan to lock the explicit-false path) |
| 4 | settings-schema.md: docs | ✅ | Two new field rows, JSON example updated, "Forward-compat normalization" subsection added |
| 5 | app.js: settings mirror | ✅ | `makeDefaults` + `TUNING_SPEC` enrolled |
| 6 | app.js: boundary helpers | ✅ | `computeEqualHeightBoundaries` (~10 lines), `computeVisualDensityBoundaries` (~70 lines) |
| 7 | app.js: dispatch + ground-align | ✅ | switch over `slice_distribution_mode`; `exportScene.position.y = -boundaries[0]` when `ground_align` |
| 8 | progress.md + manual rebake | ✅ | progress written; manual rebake of the rose left to operator (see review.md) |

## Deviations from plan

- **Extra test added.** Plan called for one migration test
  (`TestLoadSettings_MigratesOldFile`); I added a second
  (`TestLoadSettings_ExplicitFalseGroundAlign`) because the `*bool`
  re-decode is the kind of code that silently regresses if someone
  later "simplifies" it. Cheap insurance.
- **Code-comment breadcrumb on `dome_height_factor`.** The plan has
  this as a one-liner; I expanded it to two lines noting both
  T-002-02 (the original wiring) and T-005-01 (this ticket's
  end-to-end exposure). No behavior change.
- **No commits made.** RDSPI flow leaves commit cadence to the
  operator; the working tree contains all changes for review and
  the orchestrator decides when/how to commit.

## Verification log

- `go build ./...` — clean
- `go test ./...` — `ok glb-optimizer 0.404s`
- `node -c static/app.js` — clean (syntax check)
- Manual rebake of the rose with `dome_height_factor: 0.7` —
  **deferred to operator**; this requires a running browser session
  with a loaded asset, which is outside the agent's reach. The
  ticket's manual-verification criterion is recorded as a TODO in
  review.md.
