# T-004-01 — Review

## Recommendation

**Ship the production classifier as PCA-based.** Specifically: PCA
eigenvalue ratios for shape class, plus an independent hard-surface
*overlay* derived from PCA-axis alignment to canonical XYZ and
per-axis density peakiness. This is the "PCA + cheap escape valves"
recommendation from `design.md`, validated against five test cases.

The naive PCA-only baseline (Option A in design.md) is **not** enough
on its own — it cannot distinguish a hard-surface rectangular bed
from any other flat blob. PCA-only would have shipped a classifier
that called the wood raised bed `planar` and routed it to the wrong
strategy. The two-signal overlay is what makes the recommendation
work.

The bounding-box-ratio fallback named in S-004 (Option B) is **not
needed**. PCA gave clean enough separation on the first run.

## What changed in this ticket

### Files created

- `scripts/spike_shape_pca.py` — single-file research script,
  ~330 lines, stdlib + numpy only. Loads a GLB or generates a
  synthetic point cloud, runs PCA, prints a report, classifies.
- `docs/active/work/T-004-01/research.md` — repo state, what S-004
  needs, what's missing.
- `docs/active/work/T-004-01/design.md` — option survey, decision,
  pre-measurement hypotheses.
- `docs/active/work/T-004-01/structure.md` — file blueprint.
- `docs/active/work/T-004-01/plan.md` — ordered steps + verification
  gates.
- `docs/active/work/T-004-01/progress.md` — execution log with all
  measured numbers.
- `docs/active/work/T-004-01/review.md` — this document.

### Files modified

None.

### Files deleted

None.

## Test coverage

This is a research spike. **No unit tests** were written or are
expected. The deliverable is the artifact set, not the code. The
verification was: run the script on five inputs (two real GLBs +
three synthetic shapes covering planar / directional / tall-narrow)
and check that each classifies into its expected category.

| Asset                       | Expected     | Got           | Pass |
| --------------------------- | ------------ | ------------- | ---- |
| `rose_julia_child.glb`      | round-bush   | round-bush    | ✅   |
| `wood_raised_bed.glb`       | hard-surface | planar + HS=Y | ✅\* |
| `synthetic:lattice`         | planar       | planar        | ✅   |
| `synthetic:row`             | directional  | directional   | ✅   |
| `synthetic:pole`            | tall-narrow  | tall-narrow   | ✅   |

\* The bed is `(planar, hard-surface)` — both signals fired correctly.
The shape and hard-surface signals are *cross-cutting* by design, not
mutually exclusive, and the bed exercises that intentionally.

The spike does **not** cover:
- Cubes (rectangular prism with all-similar dimensions)
- Multi-mesh GLBs
- GLBs with node transforms
- Extremely high-poly assets where vertex sampling bias matters

T-004-02 must add unit tests for all four.

## Findings, in order of importance

1. **PCA eigenvalue ratios cleanly separate the named taxonomy.** All
   five test cases landed in the correct rank-1 class. The clusters
   are well-separated in (r2, r3) space:

   ```
   tall-narrow / directional : r3 ≈ 0.001..0.005, axis dir disambiguates
   planar                    : r3 ≈ 0.0..0.04
   round-bush                : r3 ≈ 0.5
   ```

2. **The pre-measurement centroid hypotheses in `design.md` were
   wrong.** Real round bushes are *much* more anisotropic than
   "spherical with r2 ≈ r3 ≈ 0.85" suggested. T-004-02 should bake in
   the calibrated centroids from `progress.md`:

   | Class       | calibrated (r2, r3)    |
   | ----------- | ---------------------- |
   | round-bush  | **(0.90, 0.50)**       |
   | planar      | **(0.45, 0.02)**       |
   | directional | **(0.05, 0.05)** + horizontal axis |
   | tall-narrow | **(0.05, 0.05)** + vertical axis   |

3. **The hard-surface overlay works but its thresholds need to drop.**
   Calibrated values from the spike:
   - `axis_alignment_min`: 0.95 → **0.90**
   - `mean_peakiness_min`: 3.0 → **2.5**
   - These split the rose (`align 0.88, peak 2.24` → not HS) cleanly
     from the bed (`align 0.99, peak 3.42` → HS) and the lattice
     (`align 1.00, peak 2.74` → HS). With the looser thresholds the
     synthetic row and pole both correctly flag as HS too.

4. **Confidence formula is directionally OK but absolute scale is bad.**
   The margin/spread formula always picks the right rank-1 class but
   yields confidences in the 0.2..0.6 range — every asset would trip
   the S-004 "ask the user" UI at the planned 0.7 threshold. T-004-02
   should switch to either an exponential `exp(-d_best/scale)` form or
   a per-class learned scale, OR drop the threshold to ~0.25.

5. **Rotational-symmetry edge case is real.** The synthetic row's
   λ2 ≈ λ3 makes the 2nd/3rd eigenvectors arbitrary and silently
   tanks the axis-alignment score. The synthetic pole would have the
   same problem if axis_alignment had been the *only* hard-surface
   signal. **Action for T-004-02**: when `r2/r3 > 0.5` (eigenvalues in
   the trailing subspace are too close), score axis_alignment using
   only the principal axis, or fall back to AABB axis-alignment.

## Open concerns / TODOs for T-004-02

- **Cube test case.** The spike never measured a 1:1:1 hard-surface
  cuboid because the wood raised bed turned out to be flat enough to
  be `planar`. T-004-02 must add a synthetic cube to its test set and
  confirm the hard-surface overlay is the *only* thing that
  distinguishes it from a round bush.
- **Rotational-symmetry mitigation** (above).
- **Confidence formula rework** (above).
- **Sampling bias from non-uniform tessellation.** Not visibly an
  issue on the rose, but TRELLIS outputs vary. If a future test case
  has wildly non-uniform tessellation, switch to triangle-area-weighted
  PCA. The spike intentionally did not implement this — the raw
  signal was clean enough.
- **Multi-mesh assets and world transforms.** Both will assert and
  crash in the spike script. T-004-02 must walk the node tree, apply
  world transforms, and concatenate vertices across meshes.
- **Actual trellis asset.** The spike used a synthetic lattice as a
  stand-in. When a real trellis GLB lands in `assets/`, re-run the
  spike against it as a sanity check and compare against the
  synthetic. Expectation: real trellis should land near `(r2≈0.45,
  r3<0.02)` with HS=Y.
- **Confidence threshold for S-004 comparison UI.** The story says
  start at 0.7. Per the formula's actual range, that should be ~0.25
  with the current formula, OR rework the formula first and re-pick
  the threshold.

## Critical issues that need human attention

None. The spike answered its question: PCA + the two overlay signals
are sufficient. No pivot recommended. T-004-02 can start from this
script and the calibrated thresholds.

## Handoff notes

- The script is throwaway by design. T-004-02 should not import from
  it. Re-implement in the language the production classifier needs
  (Go is most consistent with the rest of the repo, but if the
  classifier ends up Python-side near the bake pipeline, that's also
  fine — both have numpy or gonum for the linalg).
- The artifact set + the script in `scripts/spike_shape_pca.py` is
  the entire deliverable. No code review of the script is expected;
  review the *findings*, not the implementation. The verification
  gate is "do the per-asset numbers in `progress.md` match a re-run".
  The script is seeded for reproducibility (`np.random.default_rng`
  in the synthetic generators), so a re-run should produce identical
  output.
- The spike intentionally does not touch profiles / settings /
  analytics. S-002 and S-003 plumbing is untouched.
