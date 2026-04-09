# T-004-01 — Research

## Goal

Validate (or invalidate) PCA-on-vertex-cloud as the basis for S-004's
geometry-aware shape classifier. Output: a recommendation grounded in
measurements, not intuition. No production code.

## What exists in the repo today

### Reference assets (`assets/`)

| File                          | Bytes | Verts   | Tris   | Class (expected)        |
| ----------------------------- | ----- | ------- | ------ | ----------------------- |
| `rose_julia_child.glb`        | 7.8M  | 162,958 | ~325K  | round-bush              |
| `wood_raised_bed.glb`         | 1.2M  | 6,571   | ~13K   | hard-surface (box)      |
| `wood_raised_bed_parametric.glb` | (out of S-001 reconstruction) | — | — | reconstructed |
| `wood_raised_bed_textured.glb`   | (out of S-001 baking)         | — | — | baked   |

There is **no trellis-class asset on disk**. The ticket explicitly allows
generating one for the spike. Online fetch is not viable in this
environment, so the spike will synthesize two extra point clouds in
Python (lattice panel + linear row) to cover the missing taxonomy slots.

Both source GLBs share the same trivial structure:

- 1 scene, 2 nodes, 1 mesh, 1 primitive
- No node-level transforms (`matrix`/`rotation`/`translation`/`scale`
  all absent on every node)
- POSITION accessor is plain `float32[count][3]`
- Indices are present but the spike does not need them

→ For these assets "extract vertex cloud with world transforms applied"
collapses to "read the POSITION buffer". A future production classifier
must walk the node tree, but the spike does not need to.

### Existing GLB tooling (`scripts/`)

- `parametric_reconstruct.py` — hand-rolled GLB parser (`parse_glb`,
  `extract_mesh_data`). Avoids the `trimesh` dependency. Numpy is
  optional but available in this environment (`numpy 2.4.4`).
- `bake_textures.py`, `export_tuning_data.py`, `remesh_lod.py` — also
  pure-stdlib + optional numpy.

→ The spike must follow the same dep policy: stdlib + numpy, no
trimesh, no scipy. Numpy is required because PCA on 160k vertices in
pure Python would be unbearable.

### Existing classification / shape code

`grep -r` for `pca`, `eigen`, `principal`, `classify`, `shape_class`
returns nothing under repo root. There is no prior shape classifier to
extend, contradict, or align with. Greenfield.

`profiles.go`, `analytics.go`, `accepted.go` exist (S-002 / S-003) but
they are settings / event plumbing — they have no opinion on geometry.

## What S-004 needs from this spike

From `docs/active/stories/S-004.md` and the ticket frontmatter:

1. A **decision**: ship classifier as PCA-based, or pivot.
2. **Classification thresholds** (axis-magnitude ratios) that separate
   `round-bush`, `directional`, `tall-narrow`, `planar`, `hard-surface`.
3. A **confidence formula** in `[0, 1]` so S-004's multi-strategy
   comparison UI knows when to ask the user.
4. **Edge cases** that the production classifier (T-004-02) must handle.

The taxonomy has five named buckets plus `unknown`. Mapping each bucket
to PCA eigenvalue ratios (λ1 ≥ λ2 ≥ λ3, normalized so λ1 = 1):

- `round-bush`: λ2 ≈ λ3 ≈ 1 (roughly isotropic)
- `tall-narrow`: λ1 ≫ λ2 ≈ λ3, AND principal axis is roughly vertical
- `directional`: λ1 ≫ λ2, λ2 ≈ λ3 (elongated, principal axis horizontal)
- `planar`: λ3 ≪ λ1, λ2 (one axis collapsed)
- `hard-surface`: not directly a shape class — it is the round-bush /
  planar / directional cases that *also* have low surface curvature or
  axis-aligned dominant axes. PCA alone may not separate this from a
  round bush; the ticket asks the spike to flag this gap.

→ The spike must check whether `hard-surface` falls out of PCA at all,
or whether a separate signal (axis-alignedness, density histogram
peaks, vertex-on-grid evidence) is needed.

## What "axis-aligned" looks like for hard-surface

The raised bed is a rectangular cuboid. Its principal axes should align
with model-local X/Y/Z to within numerical precision, because the
vertex distribution is dominated by six axis-aligned planes. A round
bush, by contrast, has principal axes that *can* point anywhere — the
vertex cloud has no preferred orientation, so the PCA basis ends up
arbitrary (driven by sampling noise).

Two candidate signals to capture this:

1. **Axis-alignment score**: max dot product between each PCA axis and
   the canonical basis. For an axis-aligned box this is ≈ 1.0; for a
   sphere it is uniformly distributed.
2. **Density histogram peakiness**: project vertices onto each
   principal axis, bin them, and look for sharp peaks. A box has 2
   tall peaks per axis (the two faces). A sphere has a smooth
   semi-elliptical density.

`parametric_reconstruct.py` already uses peak detection on the Y-axis
density histogram for board layer detection — proves the signal is
strong enough to be useful.

## Constraints / assumptions

- **Sampling bias**: GLB vertices come from the mesh, not a uniform
  surface sample. A fine-tessellated curved patch contributes more
  vertices than a coarse flat patch, biasing PCA toward whatever the
  modeler over-tessellated. The TRELLIS outputs in this repo seem
  reasonably uniform but the spike should at least *measure* the
  worst-case bias by comparing raw-vertex PCA against a triangle-area
  weighted variant if time permits. (Stretch goal — if the raw signal
  is already clean enough, skip.)
- **Scale**: assets are at arbitrary world-units. The spike should
  report eigenvalue *ratios*, not absolute eigenvalues, so the
  classifier is scale-invariant.
- **Numerical**: 162k × 3 covariance is trivial for numpy
  (`np.cov` + `np.linalg.eigh`). No need for incremental PCA.
- **No production code path**: T-004-02 will rebuild this in the
  appropriate language with the right plumbing. The spike script can
  be ugly.

## Open questions the spike must answer

1. Do the eigenvalue ratios meaningfully separate `round-bush` from
   `hard-surface` (rectangular cuboid) from `directional` from `planar`?
2. If not — does adding axis-alignment + density-peakiness recover
   separation?
3. What is a reasonable confidence formula? Distance to the nearest
   class centroid in ratio-space? Margin between top-1 and top-2 class?
4. Are there edge cases (degenerate / hollow / disconnected meshes)
   where PCA silently misbehaves?

## Out of scope for the spike

- The classifier API surface (T-004-02)
- The strategy router (T-004-03)
- The comparison UI / analytics events (T-004-04, S-003 integration)
- Performance optimization (the script can be slow)
- Walking node trees / applying world transforms (the reference assets
  don't need it)

## Deliverables

- `scripts/spike_shape_pca.py` — runnable, ugly is fine
- `docs/active/work/T-004-01/research.md` — this document
- `docs/active/work/T-004-01/design.md` — option survey + decision
- `docs/active/work/T-004-01/structure.md` — file-level blueprint
- `docs/active/work/T-004-01/plan.md` — ordered steps
- `docs/active/work/T-004-01/progress.md` — execution log w/ measurements
- `docs/active/work/T-004-01/review.md` — handoff
