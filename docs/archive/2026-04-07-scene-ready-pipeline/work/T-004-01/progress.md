# T-004-01 — Progress

## Status

Spike script written and run. All five test cases produced sensible
output on the first run. No deviations from `plan.md` were necessary.
No fallbacks (triangle-area weighting etc.) were needed.

## Steps completed

1. ✅ Skeleton + GLB loading. `parse_glb` / `extract_positions` ported
   from `parametric_reconstruct.py`. Assertions about single-mesh /
   single-primitive / no node transforms passed for both real assets
   (matches research.md).
2. ✅ Synthetic generators (`lattice`, `row`, `pole`). Added a third
   synthetic (`pole`) to cover `tall-narrow` so the spike covers four
   of the five named taxonomy classes (the fifth, `hard-surface`, is
   an overlay flag, not a shape class).
3. ✅ PCA + axis-alignment + density peakiness.
4. ✅ Classification (nearest centroid in (r2, r3) space) +
   hard-surface overlay.
5. ✅ Single run captured below.
6. ✅ Review handed off to `review.md`.

Ran: `python3 scripts/spike_shape_pca.py`

## Per-asset results

### `assets/rose_julia_child.glb` — expected `round-bush`

```
n_points        : 162958
bbox dims       : [+0.993, +0.899, +0.947]
aspect (h/w)    : +0.905
eigenvalues     : [+0.055292, +0.051157, +0.026124]
ratios r2,r3    : r2=0.9252 r3=0.4725
principal axis  : [+0.827, +0.090, +0.555] (horizontal)
axis alignment  : 0.8814
peakiness x/y/z': [+1.986, +2.042, +2.689] (mean 2.239)
-> class        : round-bush   confidence=0.299 hard_surface=False
   ranking      : [('round-bush', 0.288), ('planar', 0.533),
                   ('directional', 0.794), ('tall-narrow', 1.405)]
```

✅ Class correct. ✅ hard_surface correct.
⚠️  r3 = 0.47 is *much* lower than the design.md hypothesis of 0.75 —
the rose is genuinely anisotropic (one principal axis carries ~2× the
variance of the smallest). Round-bush centroid should move from
(0.85, 0.75) → roughly **(0.85, 0.50)**.
⚠️  Confidence 0.30 is low. Margin to `planar` is small because the
rose's r3 lands halfway between the two centroids. Either move the
centroids (above) or rescale the confidence formula.

### `assets/wood_raised_bed.glb` — expected `hard-surface` (rectangular)

```
n_points        : 6571
bbox dims       : [+1.001, +0.219, +0.648]
aspect (h/w)    : +0.218
eigenvalues     : [+0.161104, +0.069737, +0.005818]
ratios r2,r3    : r2=0.4329 r3=0.0361
principal axis  : [-0.988, -0.011, -0.152] (horizontal)
axis alignment  : 0.9922
peakiness x/y/z': [+4.440, +3.140, +2.688] (mean 3.422)
-> class        : planar       confidence=0.214 hard_surface=True
   ranking      : [('planar', 0.168), ('directional', 0.259),
                   ('round-bush', 0.827), ('tall-narrow', 0.839)]
```

✅ hard_surface overlay fires (alignment 0.99, peakiness 3.4).
✅ Shape class `planar` is also defensible — the bed is genuinely flat
(height 0.22 vs width 1.00). The cross-cutting `(planar, hard-surface)`
labelling is exactly the design intent: shape and hard-surface are
*independent* signals so the strategy router gets two pieces of info,
not a forced single bucket.
⚠️  Confidence 0.21 is low — `planar` and `directional` are close in
ratio space for this bed. The strategy router can use the hard-surface
overlay to break ties: a hard-surface, principal-axis-horizontal,
`planar`-or-`directional` asset is a "rectangular crate" and should
go to the parametric pipeline regardless of which of the two it lands
in.

### `synthetic:lattice` — expected `planar`

```
n_points        : 3997
bbox dims       : [+1.250, +1.850, +0.050]
eigenvalues     : [+0.370635, +0.171934, +0.000208]
ratios r2,r3    : r2=0.4639 r3=0.0006
principal axis  : [-0.000, +1.000, -0.001] (vertical)
axis alignment  : 1.0000
peakiness x/y/z': [+3.272, +3.206, +1.754] (mean 2.744)
-> class        : planar       confidence=0.445 hard_surface=False
```

✅ Class correct. r3 ≈ 0.0006 is gloriously planar.
⚠️  hard_surface = **False**, but the lattice is a wooden trellis —
clearly hard-surface in the strategy-router sense. axis_alignment is
1.0 (perfect) but mean_peakiness 2.74 < 3.0 threshold. **Threshold
needs to drop to ~2.5** to capture the lattice without false-positives
on the rose (rose mean_peak is 2.24).

### `synthetic:row` — expected `directional`

```
n_points        : 3996
bbox dims       : [+6.000, +0.400, +0.400]
eigenvalues     : [+5.018733, +0.022475, +0.022193]
ratios r2,r3    : r2=0.0045 r3=0.0044
principal axis  : [-1.000, +0.000, -0.000] (horizontal)
axis alignment  : 0.9174
peakiness x/y/z': [+4.931, +2.274, +2.045] (mean 3.083)
-> class        : directional  confidence=0.420 hard_surface=False
```

✅ Class correct (and tall-narrow vs directional disambiguation by
horizontal principal axis worked).
⚠️  axis_alignment 0.917 < 0.95, so the hard-surface overlay does not
fire. Why: the row is rotationally symmetric about its long axis, so
λ2 ≈ λ3, and the eigenvectors in that 2D subspace are *arbitrary* —
they happen not to land on world Y/Z. **Real edge case** for any
classifier that uses axis-alignment. Mitigation: when `r2 ≈ r3`,
ignore the second/third eigenvector contributions to alignment and
score only the first.

### `synthetic:pole` — expected `tall-narrow`

```
n_points        : 3996
bbox dims       : [+0.100, +3.000, +0.100]
eigenvalues     : [+1.262489, +0.001406, +0.001391]
ratios r2,r3    : r2=0.0011 r3=0.0011
principal axis  : [+0.001, +1.000, +0.000] (vertical)
axis alignment  : 0.9537
peakiness x/y/z': [+4.967, +2.700, +2.675] (mean 3.447)
-> class        : tall-narrow  confidence=0.622 hard_surface=True
```

✅ All correct. The vertical-disambiguation rule pulled this off
`directional` (which has identical r2/r3) into `tall-narrow`.
Confidence 0.62 — highest of the five — because vertical penalty
opens daylight between the two candidates.

## Cross-asset summary

```
asset                          r2       r3    align   peak   class         conf   hs
rose_julia_child.glb         0.9252  0.4725   0.881   2.24   round-bush    0.30   N
wood_raised_bed.glb          0.4329  0.0361   0.992   3.42   planar        0.21   Y
synthetic:lattice            0.4639  0.0006   1.000   2.74   planar        0.45   N
synthetic:row                0.0045  0.0044   0.917   3.08   directional   0.42   N
synthetic:pole               0.0011  0.0011   0.954   3.45   tall-narrow   0.62   Y
```

## Measured vs predicted centroids

| Class       | predicted (r2, r3) | measured (r2, r3) | suggested (T-004-02) |
| ----------- | ------------------ | ----------------- | -------------------- |
| round-bush  | (0.85, 0.75)       | (0.93, 0.47)      | **(0.90, 0.50)**     |
| planar      | (0.60, 0.05)       | (0.46, 0.001) lat / (0.43, 0.04) bed | **(0.45, 0.02)** |
| directional | (0.20, 0.15)       | (0.005, 0.004)    | **(0.05, 0.05)**     |
| tall-narrow | (0.10, 0.10)       | (0.001, 0.001)    | **(0.05, 0.05)**     (disambiguated by axis dir) |

The hypothesis centroids in `design.md` were too far from each other
in r3 — the actual data clusters tighter at r3 ∈ {≈0.5, ≈0.001}. The
suggested centroids above are what T-004-02 should bake in.

## Threshold calibrations

- `hard_surface.axis_alignment_min`: drop **0.95 → 0.90**, plus the
  rotational-symmetry mitigation (when r2 ≈ r3, score only the first
  eigenvector). Otherwise the row gets a false negative.
- `hard_surface.mean_peakiness_min`: drop **3.0 → 2.5**. Captures the
  lattice (2.74) without the rose (2.24).
- `tall-narrow vs directional` vertical penalty: 0.5 worked; keep.
- `confidence` formula: works directionally but the absolute numbers
  are uniformly low because centroid distances in (r2, r3) ∈ [0,1]² are
  small. T-004-02 should either (a) use exp-based decay
  (`exp(-d_best / scale)`) or (b) compare margin to a calibrated
  per-asset baseline. The spike's formula is *good enough* for ranking
  (the right class is always rank-1) but bad for thresholding the
  comparison UI at 0.7 — every asset would trip the UI.

## Edge cases observed

1. **Rotational-symmetry degeneracy** (synthetic:row): when λ2 ≈ λ3,
   the second/third eigenvectors are arbitrary and axis_alignment
   becomes meaningless for those axes. T-004-02 must detect this and
   either skip those axes when scoring alignment, or fall back to
   bbox-axis alignment.
2. **Anisotropic round bush** (rose): a "round bush" is not actually
   spherically symmetric — the rose's r3 is 0.47, not ~1.0. The
   `round-bush` centroid had to move halfway across the (r2, r3)
   plane. Production needs to be tolerant of `round-bush` covering a
   fairly broad ratio region.
3. **Cube vs sphere ambiguity**: not actually exercised by any test
   asset — the bed is flat enough to be `planar`, not a cube. The
   spike *could not* test the worst-case "rectangular cuboid with
   r2 ≈ r3 ≈ 0.85" scenario. **Open**: T-004-02 should add a unit
   test for a synthetic cube, with the expectation that the
   hard-surface overlay (alignment + peakiness) is the only signal
   that distinguishes it from a round bush.
4. **Confidence is consistently low**: expected — see the formula
   note above. Use a different formula in T-004-02.
5. **Multi-mesh / node-transform GLBs**: not exercised by reference
   assets but very real in the wild. The spike's hard assertions will
   fail loudly, which is the correct fail-mode for a spike — T-004-02
   must handle these.
