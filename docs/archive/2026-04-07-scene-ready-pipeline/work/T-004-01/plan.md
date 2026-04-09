# T-004-01 — Plan

## Overview

Build the spike script in one commit, then run it and write up the
findings. This is research, not a feature, so the plan is short on
purpose. Each step ends with a verification gate that says "if this
fails, stop and re-think before continuing".

## Steps

### Step 1 — Skeleton + GLB loading

Write `scripts/spike_shape_pca.py` with:
- imports, constants, CLASS_CENTROIDS, HARD_SURFACE_THRESHOLDS
- `parse_glb` and `extract_positions` (positions only; assert single
  mesh, single primitive, no node transforms — fail loudly if any
  reference asset violates the assumptions captured in research)
- `main()` that just loads and prints `n_points` for each input

**Verify:** running with the two real GLBs prints
`162958` and `6571`. If not, the GLB parser is wrong; fix before
moving on.

### Step 2 — Synthetic generators

Add `synth_lattice`, `synth_row`, `synth_pole`. Each returns an
`np.ndarray[N, 3]`. Wire them into `load_points` so
`synthetic:lattice` etc. work as inputs.

- `lattice`: ~4000 points sampled on a 1.2 × 1.8 × 0.05 grid of
  vertical and horizontal slats. Aligned to world XY plane.
- `row`: ~4000 points along a 6.0 × 0.4 × 0.4 box, oriented along
  world X.
- `pole`: ~4000 points along a 0.1 × 3.0 × 0.1 box, oriented along
  world Y.

**Verify:** for each synthetic, print bbox dimensions and confirm
they match the spec. The bbox check rules out "I rotated the lattice
by accident" bugs.

### Step 3 — PCA + features

Add `compute_pca`, `axis_alignment_score`, `density_peakiness`,
`all_peakiness`, `principal_axis_orientation`. Wire them into a
`features_for(label, points)` helper.

**Verify:** for `synthetic:lattice`, eigenvalue ratios should be
roughly `(r2 ≈ 0.4, r3 ≈ 0.001)` — one axis collapsed. For
`synthetic:row`, `(r2 ≈ 0.005, r3 ≈ 0.005)` — one axis dominant.
For `synthetic:pole`, similar to row but with the principal axis
near world Y. If these don't match design.md predictions, PCA isn't
behaving and we need to debug before classifying anything.

### Step 4 — Classification + report

Add `nearest_class`, `apply_hard_surface_overlay`, `print_report`.
Wire into `main()`.

**Verify:** the rose classifies as `round-bush`, the bed classifies
as either `round-bush` or `cube` (PCA can't tell the difference) —
but with `is_hard_surface = True` from the overlay. The lattice
classifies as `planar`. The row classifies as `directional`. If the
bed comes out *without* `is_hard_surface = True`, the overlay
thresholds are wrong and the spike has surfaced its first calibration
finding.

### Step 5 — Run + capture into `progress.md`

Run the script with the default input list, redirect stdout to a
buffer, paste the per-asset reports + the cross-asset table into
`progress.md`. Add a "measured vs predicted" comparison against the
hypotheses from `design.md`.

**Verify:** progress.md contains real numbers for all four assets and
a table comparing them.

### Step 6 — Write `review.md`

Summarize: what shipped, the recommendation (PCA + overlay or pivot),
calibrated thresholds, edge cases, open concerns, what T-004-02
should inherit. ~200 lines max.

## Testing strategy

- **Unit tests**: none. This is a research spike; the deliverable is
  the artifact, not the code. T-004-02 will own the unit tests for the
  production classifier.
- **Smoke run**: the verification gates above are the test plan.
- **Reproducibility**: all synthetic generators take a seed (`np.random`
  default seeded in `main()`), so re-runs of the spike produce
  identical numbers.

## Commit plan

The whole spike is small enough to land as a single commit:

```
T-004-01 spike: PCA-based shape classification

scripts/spike_shape_pca.py + research/design/structure/plan/progress/
review artifacts under docs/active/work/T-004-01/.
```

If verification at Step 4 fails (e.g. PCA gives nonsense for the
lattice), split into two commits: one for the script + a "PCA does
not separate these classes" review, and a second commit for any
additional features tried as fallbacks.

## Risks

- **GLB parser corner case**: a reference asset has a node transform
  we didn't notice. Mitigated by asserting and failing loud in Step 1.
- **Numpy version drift**: `np.cov` rowvar default. Pinning explicit
  `rowvar=False` to avoid surprises.
- **Cube ambiguity**: known and documented; the overlay handles it.
- **Sampling bias on the rose**: only fall back to triangle-area
  weighting if the report shows obviously wrong principal axes.
- **Time budget**: spike must stay small. If it grows past ~300 lines
  of script, that's a smell — cut features.
