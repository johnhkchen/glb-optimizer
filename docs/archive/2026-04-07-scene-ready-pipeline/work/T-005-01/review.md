# Review — T-005-01: slice-distribution-and-shape-restoration

## What changed

### Files modified

| File | Change |
|---|---|
| `settings.go` | +`SliceDistributionMode string`, +`GroundAlign bool` on `AssetSettings`. New defaults `"visual-density"` and `true`. New enum map `validSliceDistributionModes`. New validation rule for the slice mode. New forward-compat normalization in `LoadSettings` (empty enum → default; absent bool → `true` via `*bool` re-decode of the same byte slice). |
| `settings_test.go` | +`TestDefaultSettings_NewFields`. +2 cases on the existing `TestValidate_RejectsOutOfRange` table (empty + unknown slice mode). +`TestLoadSettings_MigratesOldFile`. +`TestLoadSettings_ExplicitFalseGroundAlign`. |
| `docs/knowledge/settings-schema.md` | +2 rows in the field table. JSON example expanded. New "Forward-compat normalization" subsection under "Migration Policy". |
| `static/app.js` | `makeDefaults()` mirrors the two new keys. `TUNING_SPEC[]` enrolled with reserved DOM ids `tuneSliceDistributionMode` / `tuneGroundAlign`. New `computeEqualHeightBoundaries(model, numLayers)` helper (~10 lines). New `computeVisualDensityBoundaries(model, numLayers)` helper (~70 lines): trunk-filtered, radial-weighted vertex quantile with cum-weight lookup. `renderHorizontalLayerGLB` switches on `currentSettings.slice_distribution_mode` to pick the boundary picker; falls through to the legacy `vertex-quantile` picker on missing/unknown mode. After the per-quad loop, `exportScene.position.y = -boundaries[0]` when `ground_align` is true. Code-comment breadcrumb added next to the `dome_height_factor` read. |

No files created (other than RDSPI artifacts under
`docs/active/work/T-005-01/`). No files deleted. No
`index.html` / `style.css` edits — UI sliders are explicitly
T-005-02's job.

## Acceptance-criteria mapping

- **New `visual-density` mode in `renderHorizontalLayerGLB`** — implemented via `computeVisualDensityBoundaries`, dispatched in the new `switch (mode)`.
  - **Trunk filter (bottom 10% of bbox height)** — `trunkY = minY + 0.10 * (maxY - minY)`, vertices below it dropped before the cum-weight build.
  - **Radial weight** — `w = clamp(sqrt(x²+z²) / maxRadius, 0.05, 1.0)`. The 0.05 floor keeps central canopy contributing instead of vanishing.
- **`slice_distribution_mode` enum field, default `visual-density`** — added to `AssetSettings`, defaulted in `DefaultSettings`, enforced by `validSliceDistributionModes`.
- **`dome_height_factor` wired through `createDomeGeometry`** — the wiring already existed end-to-end as of T-002-02 (line 1278 reads `currentSettings.dome_height_factor`); I left a comment breadcrumb pointing to both tickets and verified by reading the call chain. The ticket text was stale on this point — see design.md "Question 4" for the analysis. **If the reviewer expected a *new* parameter on `createDomeGeometry`, this is the divergence to flag.**
- **Ground alignment via Y offset on bottom slice** — `exportScene.position.y = -boundaries[0]` after the per-quad loop. Affects only the GLTF root node transform; bake textures and inter-quad spacing are unchanged.
- **`ground_align` bool, default `true`** — added; gating the translate.
- **Backwards compat: `equal-height` reproduces original simple slicing** — `computeEqualHeightBoundaries` is exactly the linear-interp loop the original `computeAdaptiveSliceBoundaries` falls back to when `ys.length === 0`.
- **`vertex-quantile` mode preserved** — the legacy `computeAdaptiveSliceBoundaries` is untouched and remains the dispatch target for `vertex-quantile` (and the default-case fallback).
- **All three new settings emit `setting_changed` analytics** — Both new fields are enrolled in `TUNING_SPEC`, which `wireTuningUI` (app.js:286) auto-instruments. The third "new setting" referenced by the ticket — `dome_height_factor` — was enrolled in `TUNING_SPEC` by T-002-03 already (row at line 263). All three are covered.
- **Manual rebake of the rose** — **NOT performed by the agent.** Requires a browser session with a live asset; recorded as an open TODO below for the operator.

## Test coverage

| Layer | What runs | Status |
|---|---|---|
| Go unit | `settings_test.go` — defaults, validation (including the two new failure cases), round-trip, missing-file, two migration paths | ✅ `go test ./...` green |
| JS unit | None — the project still has no JS test runner; the boundary helpers are pure functions and were sanity-checked by reading | ⚠️ untested |
| Build | `go build ./...` clean; `node -c static/app.js` clean | ✅ |
| Manual integration | Rebake the rose, eyeball curvature + ground contact + mode parity | ❌ deferred (operator) |

### Coverage gaps

- **`computeVisualDensityBoundaries` has zero automated tests.** It is the most algorithmically dense addition in this ticket and the one most likely to regress on a future "simplification". The cleanest fix is a JS test harness, which is out of scope here. A cheaper interim is a `// @ts-check`-style invariant assertion at the bottom of the function (length, sortedness, finite values) that throws in dev. I did not add this — keeping the function readable seemed more valuable than belt-and-suspenders runtime checks for a single in-house caller. **Reviewer call.**
- **`computeEqualHeightBoundaries` is trivial enough to eyeball,** so the lack of a test is acceptable.
- **The ground-align Y offset has no test** — there's no JS test runner. The behavior is one line and visually verifiable in the manual smoke test.

## Open concerns / TODOs

1. **Manual rebake of the rose.** The ticket's "manual verification" criterion is unmet by the agent. The operator should:
   - Open the rose asset, set `currentSettings.dome_height_factor = 0.7;` in devtools, click "Production Asset".
   - Confirm the silhouette has more curvature than the existing pancake stack.
   - Confirm the bottom of the volumetric output sits at Y=0 in the scene preview (no leaves clipping into the ground).
   - Toggle to `equal-height` and `vertex-quantile` modes and confirm both still produce valid GLBs.
2. **Stale ticket text on `dome_height_factor`.** The ticket says the field is "currently hardcoded at 0.5"; it has not been hardcoded since T-002-02. I treated this as a no-op (added a code comment, no behavior change). If a reviewer expected a new parameter on `createDomeGeometry` itself, the design.md "Question 4" entry explains why I rejected that.
3. **`maxRadius === 0` edge case.** If every vertex is exactly on the central axis (degenerate model), I clamp `maxRadius` to 1 to avoid div-by-zero. The downstream weight collapses to the 0.05 floor for every vertex, so the cum-weight quantile degrades to a uniform vertex-count quantile over the survivors — i.e. the function quietly does the right thing. Worth noting in case future profiling cares.
4. **Trunk filter percentage is hardcoded at 10%.** Per ticket "don't over-engineer the heuristic." If analytics show users frequently overriding into `vertex-quantile` mode, the next iteration should make `trunk_filter_height_fraction` a tunable. Not in scope here.
5. **The third "new analytics setting" depends on interpretation.** The ticket says "all three new settings emit `setting_changed` analytics." My reading: the three settings are `slice_distribution_mode`, `ground_align`, and `dome_height_factor` (the latter being newly user-facing in this ticket). The first two are enrolled here; the third was already enrolled by T-002-03. If the reviewer reads it as "three *brand-new* settings," then there is one missing field — but I can't see what the third would be. **Flag for reviewer.**
6. **Old on-disk `~/.glb-optimizer/settings/{id}.json` files** will load with `slice_distribution_mode = "visual-density"` and `ground_align = true` (the migration defaults). The first regenerate after T-005-01 lands will therefore *visibly change* the bake for any asset that already had a tuned settings file, because the slicing algorithm and ground placement both shift. This is the desired behavior but is worth a heads-up in any release notes.

## Files for the reviewer to read first

1. `settings.go` — the `LoadSettings` migration block is the highest-leverage change. The `*bool` re-decode is subtle.
2. `static/app.js` — `computeVisualDensityBoundaries` (the algorithm) and the new `switch` inside `renderHorizontalLayerGLB`.
3. `docs/knowledge/settings-schema.md` — the new "Forward-compat normalization" subsection should be skimmed to confirm the migration semantics match team expectations.
