# Plan — T-005-01

Eight ordered steps. Each is a clean atomic commit; the order respects
the dependency chain (Go schema → JS mirror → JS algorithm → wiring).

## Step 1 — `settings.go`: add fields, defaults, enum, validation

- Append `SliceDistributionMode string` and `GroundAlign bool` to
  `AssetSettings`.
- Add the two defaults to `DefaultSettings()`.
- Add `validSliceDistributionModes` map.
- Add the enum check + bool no-op to `Validate()`.

**Verification:** `go build ./...` succeeds; existing
`TestDefaultSettings_Valid` keeps passing.

## Step 2 — `settings.go`: `LoadSettings` migration

- Decode into `*AssetSettings` as today.
- Re-decode the same `data []byte` into a tiny
  `struct { GroundAlign *bool `json:"ground_align"` }`.
- After: if `slice_distribution_mode == ""`, set to `"visual-density"`.
- After: if the temp pointer is `nil`, set `s.GroundAlign = true`.

**Verification:** `TestSaveLoad_Roundtrip` continues to pass (a fresh
default round-trip is unaffected).

## Step 3 — `settings_test.go`: new tests

- `TestDefaultSettings_NewFields`: assert `"visual-density"` and
  `true`.
- Add cases to `TestValidate_RejectsOutOfRange`:
  - `"empty slice mode"` → `s.SliceDistributionMode = ""`
  - `"unknown slice mode"` → `s.SliceDistributionMode = "spirals"`
- New `TestLoadSettings_MigratesOldFile`: write a JSON file
  containing only the pre-T-005-01 keys (use a hand-rolled string
  literal, not `SaveSettings`, so the test exercises the migration
  path), then `LoadSettings` and assert both new fields take their
  default values.

**Verification:** `go test ./...` is green.

## Step 4 — `docs/knowledge/settings-schema.md`

- Add the two field-table rows.
- Update the JSON example to include both new keys.
- Add the "Forward-compat normalization" subsection under
  "Migration Policy" with the rules for empty enum → default and
  absent bool → `true`.

**Verification:** Markdown lint clean (no tooling beyond eyeballing).

## Step 5 — `static/app.js`: settings mirror

- Add `slice_distribution_mode: 'visual-density'` and
  `ground_align: true` to `makeDefaults()`.
- Add the two `TUNING_SPEC` rows with reserved DOM ids
  `tuneSliceDistributionMode` and `tuneGroundAlign`.

**Verification:** Open the app, select an asset, confirm the
analytics dirty-dot logic still works (no console errors). The new
spec rows are inert because their DOM ids don't exist yet.

## Step 6 — `static/app.js`: boundary helpers

- Add `computeEqualHeightBoundaries(model, numLayers)` immediately
  above `computeAdaptiveSliceBoundaries`.
- Add `computeVisualDensityBoundaries(model, numLayers)` immediately
  below it.

Both helpers are pure functions on a `THREE.Object3D` and a number.
No side effects, no settings reads.

**Verification:** Manually invoke from devtools console on the
loaded rose model; sanity-check that the returned boundary arrays
have length `numLayers + 1` and are sorted ascending.

## Step 7 — `static/app.js`: `renderHorizontalLayerGLB` dispatch + ground align

- Replace the unconditional `computeAdaptiveSliceBoundaries` call
  with a `switch (mode)` over `currentSettings.slice_distribution_mode`.
- After the layer-quad loop, before `new GLTFExporter()`, add the
  `if (currentSettings.ground_align) exportScene.position.y = -boundaries[0]`.
- Add the breadcrumb comment on line 1278 referencing T-005-01.

**Verification:** Manual rebake of the rose. Three checks:

1. **Curvature.** Toggle `dome_height_factor` to `0.7` via the
   browser devtools (`currentSettings.dome_height_factor = 0.7;
   await generateProductionAsset(currentFileId)`). Open the
   resulting volumetric in the preview. Silhouette should bulge
   visibly more than the existing pancake stack — at minimum, the
   bottom-slice dome should rise above the next layer's floor.
2. **Ground.** Open the volumetric GLB in the scene preview; the
   floor of the lowest leaf must touch (not penetrate) the ground
   grid.
3. **Mode parity.** Set
   `currentSettings.slice_distribution_mode = 'equal-height'`,
   regenerate. The boundary array (logged via a temporary
   `console.log(boundaries)`) should be a clean linear interpolation
   from `min.y` to `max.y`. Remove the `console.log` before commit.

## Step 8 — `progress.md` final write + manual rebake notes

- Mark all steps complete.
- Capture any deviations (e.g. if step 7's manual checks reveal a
  required tweak to the trunk-filter percentage).
- Note that the radial-weight v1 ships per design.md, and that the
  T-005-02 UI work is unblocked.

## Testing strategy

| Layer | Coverage |
|---|---|
| Unit (Go) | `settings_test.go` covers defaults, new validation, migration |
| Unit (JS) | None — the project has no JS test runner today; relying on devtools-driven manual smoke |
| Integration | Manual rebake of the rose, the test asset called out by the ticket |
| Regression | Existing `go test ./...` must remain green |

There is intentionally no JS test runner introduced for this ticket;
S-002 noted the project has no Vitest/Jest harness and the ticket
"first-pass scope" calls for shipping the algorithm, not building
test infrastructure for it.

## Commit boundaries

- **Commit A** (steps 1+2+3): Go schema, validation, migration, tests.
- **Commit B** (step 4): docs.
- **Commit C** (steps 5+6+7): JS mirror, helpers, dispatch, ground align.

Three commits total. Splitting commit C is tempting (helpers vs. wire-up)
but the helpers without the dispatch are dead code, and the dispatch
without the helpers doesn't compile. Keep them together.

## What could go wrong

- **Old `settings/{id}.json` files in the user's `~/.glb-optimizer`
  break.** Mitigated by step 2's migration. The tests in step 3 are
  the regression net.
- **`currentSettings.slice_distribution_mode` is `undefined` on a
  fresh load before settings arrive from the server.** The default
  case in the `switch` falls through to `vertex-quantile` (the legacy
  behavior), so the worst-case is "current behavior" rather than a
  crash.
- **Ground-align breaks the per-quad camera bounds.** Should not —
  the bake happens before the translate, and `renderLayerTopDown`
  uses `floorY`/`ceilingY` directly. But the manual smoke test in
  step 7 covers this.
- **Trunk filter discards too much for short ground-cover plants.**
  Falls back to vertex-quantile via the empty-survivors guard, with
  a `console.warn`. Not great but not silent.
