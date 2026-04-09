# T-004-01 — Structure

## Files

### Created

- `scripts/spike_shape_pca.py` — the runnable spike. Single file.
- `docs/active/work/T-004-01/research.md` — done in research phase
- `docs/active/work/T-004-01/design.md` — done in design phase
- `docs/active/work/T-004-01/structure.md` — this file
- `docs/active/work/T-004-01/plan.md` — next phase
- `docs/active/work/T-004-01/progress.md` — implement phase
- `docs/active/work/T-004-01/review.md` — review phase

### Modified

None. The spike is purely additive.

### Deleted

None.

## `scripts/spike_shape_pca.py` — internal layout

Single file, ~250 lines, no package, no classes that aren't worth it.
Sections in order:

```
# 1. Imports + constants
#    - stdlib only outside numpy
#    - CLASS_CENTROIDS dict (the (r2, r3) hypotheses from design.md)
#    - HARD_SURFACE_THRESHOLDS dict
#
# 2. GLB loading
#    - parse_glb(path)              -> (gltf_json, bin_data)
#    - extract_positions(gltf, bin) -> np.ndarray[N, 3]
#      (lifted from parametric_reconstruct.py, simplified — positions
#       only, mesh[0].primitives[0], assert no node transforms)
#
# 3. Synthetic generators
#    - synth_lattice(n=4000)  -> planar trellis panel point cloud
#    - synth_row(n=4000)      -> long horizontal directional row
#    - synth_pole(n=4000)     -> tall narrow pole (sanity check)
#    - dispatch via load_points("synthetic:lattice") etc.
#
# 4. PCA + features
#    - compute_pca(points) -> dict with:
#        centroid, eigenvalues (sorted desc), eigenvectors (cols match),
#        r2, r3, principal_axis_world, aspect_ratio,
#        bbox_min, bbox_max, dimensions
#    - axis_alignment_score(eigenvectors) -> float in [1/sqrt(3), 1]
#    - density_peakiness(points, axis, n_bins=50) -> float
#    - all_peakiness(points, eigenvectors) -> [p1, p2, p3]
#
# 5. Classification
#    - nearest_class(r2, r3) -> (class_name, confidence, ranking)
#      using design.md confidence formula
#    - apply_hard_surface_overlay(class_name, axis_align, peakiness)
#    - principal_axis_orientation(v) -> 'vertical' | 'horizontal' | 'diagonal'
#      (used to disambiguate tall-narrow vs directional)
#
# 6. Reporting
#    - print_report(label, features, classification)
#      Pretty table per asset; also dumps a one-line JSON summary so
#      progress.md can paste it.
#
# 7. main()
#    - argparse: positional list of inputs (paths or synthetic:* tokens)
#    - default input list = the four spike test cases
#    - prints reports + a final cross-asset comparison table
```

## Public interface

Spike has no API. It is a CLI:

```
python3 scripts/spike_shape_pca.py [INPUT ...]

Defaults to:
  assets/rose_julia_child.glb
  assets/wood_raised_bed.glb
  synthetic:lattice
  synthetic:row
```

Exit 0 on success. Non-zero only on hard parse errors.

## Data shapes

Internal `features` dict:

```python
{
    "label": str,
    "n_points": int,
    "centroid": [float, float, float],
    "bbox_min": [float, float, float],
    "bbox_max": [float, float, float],
    "dimensions": [float, float, float],
    "aspect_ratio": float,            # height / max(width, depth)
    "eigenvalues": [float, float, float],   # sorted desc
    "eigenvectors": [[..],[..],[..]],       # rows = axes
    "ratios": {"r2": float, "r3": float},
    "principal_axis": [float, float, float],
    "principal_orientation": "vertical" | "horizontal" | "diagonal",
    "axis_alignment": float,
    "peakiness": [float, float, float],
}
```

Internal `classification` dict:

```python
{
    "class": str,                # one of taxonomy + 'unknown'
    "confidence": float,         # 0..1
    "is_hard_surface": bool,
    "ranking": [(class_name, distance), ...],
    "rationale": str,
}
```

Both dicts are dumped as one-line JSON so the progress.md table can
quote them verbatim.

## Module boundaries

There are none worth defining — it's a single throwaway script. The
production version (T-004-02) will define the boundaries; the spike
intentionally does not, because anything more elaborate here would be
prematurely committing to an API surface T-004-02 may not want.

## Ordering of changes

1. Write the script (one commit).
2. Run it on all four test cases; capture output to `progress.md`.
3. Update `design.md`'s class centroid table only if measurements
   show the hypotheses are wildly off — otherwise leave the
   hypotheses in design.md and put the *measured* values in
   `progress.md` and `review.md`.

(`design.md` is the pre-measurement reasoning; the spike artifacts
should preserve that pre/post structure so a reader can tell which
numbers came from where.)
