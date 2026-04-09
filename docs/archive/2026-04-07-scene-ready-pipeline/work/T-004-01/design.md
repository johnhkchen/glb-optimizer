# T-004-01 — Design

## Decision

**Use vertex-cloud PCA as the *primary* shape signal, augmented with a
cheap axis-alignment score and a per-axis density peakiness score.**
Confidence is derived from the margin between the top-1 and top-2 class
in ratio space. The spike will measure all three signals and report
which ones actually move the needle. The production classifier
(T-004-02) is then free to drop signals that didn't help.

This is a "PCA + cheap escape valves" recommendation, not a "ship pure
PCA" recommendation. The escape valves are needed to address the
hard-surface ambiguity flagged in research.

## Options considered

### Option A — Naive PCA only

Compute eigenvalues of the vertex covariance, classify by ratio.

- ✅ Trivial, ~10 lines of numpy.
- ✅ Scale-invariant once you normalize.
- ❌ Cannot tell a rectangular box from an isotropic blob if the box's
  bounding ratios happen to be ~1:1:1 (a cube).
- ❌ `hard-surface` is *not* a shape — a hard-surface trellis and a
  hard-surface raised bed live in totally different ratio cells. Any
  classifier that treats hard-surface as a single PCA cell is wrong by
  construction.

### Option B — Bounding-box ratios only (the ticket's named fallback)

Compute axis-aligned bounding box, look at side ratios.

- ✅ Even simpler.
- ❌ Sensitive to model orientation. If TRELLIS produces a trellis
  rotated 30° about Y, the AABB ratios are wrong. PCA at least finds
  the *true* principal axes regardless of model rotation.
- ❌ Same hard-surface ambiguity as A.

### Option C — PCA + axis-alignment + density peakiness (chosen)

PCA gives the *shape* (round / elongated / planar). Axis-alignment of
the PCA basis to canonical XYZ flags "this looks like it was modeled
on a grid" → a hard-surface signal that is independent of shape.
Density peakiness on each principal axis flags "vertices are
concentrated at discrete planes" → another hard-surface signal.

- ✅ Three orthogonal signals; if any one is noisy the others can carry.
- ✅ Cheap — all three are O(N) after the covariance and one
  eigendecomposition.
- ✅ Lets `hard-surface` cross-cut the shape taxonomy: a hard-surface
  trellis is `(planar, hard-surface)`, a hard-surface bed is
  `(round-ish, hard-surface)`. The strategy router (T-004-03) gets
  more useful information than a single bucket.
- ❌ More knobs to tune. Mitigation: the spike measures all three on
  every test asset and reports thresholds.

### Option D — Train a classifier on labeled examples (ML)

- ❌ We have 2 labeled assets. Not enough.
- ❌ Defeats the point of the spike.
- Revisit after S-004 ships and analytics has accumulated overrides.

### Option E — Use trimesh / Open3D

- ❌ Project policy (per existing scripts) is stdlib + numpy. Adding a
  multi-MB binary dep for a research spike is wrong.

## Chosen approach in detail

### Inputs

- Path to a GLB **or** a synthetic shape token (`synthetic:lattice`,
  `synthetic:row`). Synthetic shapes generate vertex clouds in-memory
  to cover the missing planar / directional taxonomy slots.

### Pipeline

1. **Load** vertices. For real GLBs, use the same hand-rolled parser
   as `parametric_reconstruct.py`. For synthetic shapes, generate
   directly. (No node-tree walking — research confirmed reference
   assets have identity transforms.)
2. **Center** at the centroid.
3. **Covariance + eigendecomposition** via `np.cov` + `np.linalg.eigh`.
   Sort eigenvalues descending. Eigenvectors become the PCA basis.
4. **Eigenvalue ratios** (`r2 = λ2/λ1`, `r3 = λ3/λ1`) — the core
   signal. Scale-invariant.
5. **Aspect ratio** = bbox_height / max(bbox_width, bbox_depth) —
   computed in *world* axes, kept for sanity checking against PCA.
6. **Axis-alignment score** = mean over the three PCA eigenvectors of
   `max(|v · e_x|, |v · e_y|, |v · e_z|)`. ∈ [1/√3, 1]. ≈ 1 = aligned
   to canonical axes (boxy / hard-surface), ≈ 0.577 = arbitrary.
7. **Density peakiness** per principal axis: project vertices onto the
   axis, histogram into 50 bins, compute `(max_count - mean) / std`.
   High = sharp peaks → discrete surfaces → hard-surface signal.
8. **Classification** by nearest centroid in (`r2`, `r3`) space, with a
   `hard-surface` overlay derived from axis-alignment + peakiness.

### Class centroids in (r2, r3) space (initial guesses)

| Class        | r2 (λ2/λ1) | r3 (λ3/λ1) | Notes                       |
| ------------ | ---------- | ---------- | --------------------------- |
| round-bush   | ~0.85      | ~0.75      | near (1,1) corner           |
| tall-narrow  | ~0.10      | ~0.10      | one axis dominates, vertical |
| directional  | ~0.20      | ~0.15      | one axis dominates, horizontal |
| planar       | ~0.60      | ~0.05      | two axes large, one collapsed |
| (cube)       | ~0.95      | ~0.85      | indistinguishable from round-bush by PCA — hence the overlay |

The spike measures actual r2/r3 for the four test assets and either
confirms or revises these centroids. **The numbers in this table are
hypotheses, not the recommendation.**

`tall-narrow` vs `directional` is disambiguated by the *direction* of
the principal axis (close to world Y vs close to world horizontal
plane), not by the eigenvalue ratios — they look identical otherwise.

### Confidence formula proposal

```
d_i = euclidean distance from (r2, r3) to centroid_i
margin = d_2nd_best - d_best                # how much daylight
spread = d_2nd_best + d_best
confidence = clip(margin / spread, 0, 1)
```

Properties:
- 0 when the point is equidistant from the two best classes
- → 1 when the point is right on top of one centroid and far from
  others
- Bounded, no tuning constants
- Adjustable threshold for the comparison UI (S-004 starts at 0.7)

The spike reports the per-asset confidence so we can sanity-check
that obvious cases (the rose, the bed) score high and synthetic edge
cases score lower.

### Hard-surface overlay

Independent of the shape class:

```
hard_surface = (axis_alignment > 0.95) AND (mean_peakiness > 3.0)
```

Both thresholds are guesses; the spike measures actual values and
proposes data-driven thresholds in `progress.md`.

## What the spike will NOT do

- Triangle-area weighting (mentioned as stretch in research; only added
  if raw-vertex PCA is visibly biased on the rose, which has uniform
  tessellation).
- World-transform composition (research confirmed reference assets
  don't need it).
- Multi-mesh assets (reference assets are single-mesh; flag as edge
  case for T-004-02).
- Any persistence / settings integration (out of spike scope per
  ticket).
