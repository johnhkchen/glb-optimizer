# T-004-02 — Research

Production classifier for the S-004 shape taxonomy. T-004-01's spike
validated the PCA approach (`docs/active/work/T-004-01/review.md`) and
left calibrated thresholds + a list of "must-fix in T-004-02" items.
This research maps the codebase touchpoints the production classifier
will land in.

## Inputs from the spike (T-004-01)

- `scripts/spike_shape_pca.py` — research script. Numpy + stdlib only.
  Implements GLB position extraction, PCA, axis-alignment, density
  peakiness, hard-surface overlay, and nearest-centroid classification.
  The review document explicitly says **do not import from this**;
  T-004-02 re-implements with the calibrated numbers and the gaps the
  spike intentionally skipped.
- Calibrated centroids (round-bush `(0.90,0.50)`, planar `(0.45,0.02)`,
  directional/tall-narrow `(0.05,0.05)` with axis disambiguation).
- Calibrated hard-surface thresholds: `axis_alignment ≥ 0.90`,
  `mean_peakiness ≥ 2.5`.
- Confidence formula needs rework — current margin/spread yields ≤ 0.6
  even on clean classifications.
- Open gaps the spike skipped: cube test case, multi-mesh GLBs, node
  world transforms, rotational-symmetry edge case (when
  `λ2 ≈ λ3` the trailing eigenvectors are arbitrary).

## Existing Go ↔ Python subprocess pattern

`scene.go:84` — `RunParametricReconstruct(inputPath, outputPath)`
shells out to `python3 scripts/parametric_reconstruct.py` with
`exec.Command`, returns `(combinedOutput, err)`. The output is *not*
parsed; the Go side just looks at the file the script wrote. T-004-02
will follow the same pattern, but with one twist: the classifier's
output is structured data (category + confidence + features), not a
file, so the Go wrapper will need to parse stdout JSON.

`main.go:18-69` — server boot. The Python interpreter is *not*
verified at startup the way `gltfpack` and Blender are. The
parametric path silently fails at first request if `python3` or
numpy is missing. T-004-02 should at least surface a clear error
when the classifier subprocess fails.

## Settings touchpoints

`settings.go` defines `AssetSettings` and the `SettingsSchemaVersion`
constant. Adding `ShapeCategory` and `ShapeConfidence` is purely
additive: per `docs/knowledge/settings-schema.md` §"Adding fields
without a version bump", new optional fields with sensible defaults
do not require a schema bump. Concrete checklist:

1. Add the two fields to `AssetSettings` and to `DefaultSettings()`.
2. Add validation for `ShapeCategory` (enum membership including
   `unknown`) and `ShapeConfidence` (`[0,1]`).
3. Extend `Validate()`.
4. Extend `SettingsDifferFromDefaults()` so the file list "tuned"
   indicator stays accurate (otherwise every classified asset would
   read as dirty).
5. Forward-compat normalization: an old document missing
   `shape_category` should load as `"unknown"` (the empty-string Go
   zero would fail enum validation, exactly the case
   `slice_distribution_mode` solved with normalization).

`settings_test.go` already exercises round-trip + per-field
out-of-range cases — T-004-02 must add cases for the new fields.

## Analytics touchpoints

`analytics.go:25` — `validEventTypes` is the v1 enum. Per
`docs/knowledge/analytics-schema.md` §"Versioning and migration
policy", **new event types are additive and do not bump
schema_version**. T-004-02 adds `"classification"` to the map and
documents the per-type payload shape.

The acceptance criteria says the event payload is
`{category, confidence, features}`. Features are bulky (eigenvalues,
eigenvectors, peakiness arrays, dimensions); the schema doc explicitly
allows this — payload contents are opaque to the validator. The accept
handler at `handlers.go:1300` is the closest precedent for emitting
a server-side event: it calls `LookupOrStartSession(id)` then
`AppendEvent`. T-004-02's classify handler will mirror that pattern.

## Upload + handler touchpoints

`handlers.go:41` — `handleUpload`. Iterates uploaded `.glb` files,
writes them to `originalsDir`, registers a `FileRecord`. T-004-02
needs an "auto-classify on upload" hook here. Two design choices to
make in design.md:

- **Sync vs async**: classifier runs on a 4k-vertex sample in <1s
  for the spike's test cases, but TRELLIS outputs can be much
  denser. Blocking the upload response on classification could be
  user-visible.
- **Failure behavior**: if classification crashes, the upload should
  still succeed — `shape_category` just defaults to `"unknown"`.

`handlers.go:629` — `handleSettings` is the precedent for "decode +
validate + write atomically" handlers. The classify handler will be
simpler (no body) but should follow the same write-then-update-store
shape, including the `SettingsDirty` recompute.

`main.go:103-144` mounts routes. New route:
`mux.HandleFunc("/api/classify/", handleClassify(...))`. The handler
needs `store`, `originalsDir`, `settingsDir`, and `analyticsLogger`.

## GLB parsing for the production classifier

The spike's `extract_positions()` (`spike_shape_pca.py:75`) asserts
single-mesh, single-primitive, and *no* node transforms. The review
document flags this as a hard requirement to fix in T-004-02.
Production behavior must:

- Walk `gltf.nodes` recursively, composing TRS / matrix to get a
  world transform per leaf node.
- Concatenate POSITION accessors from every primitive of every
  mesh referenced by the node tree.
- Apply the world transform to each vertex block before PCA.

`scripts/parametric_reconstruct.py` already handles single-primitive
GLBs and is the closest reference for the byte-slice → numpy step,
but it does *not* handle node transforms either.

## Test asset coverage in `assets/`

Currently present in `assets/`:
- `rose_julia_child.glb` — round-bush, validated by the spike
- `wood_raised_bed.glb` — hard-surface (planar), validated
- `wood_raised_bed_parametric.glb` — derived, ignore for classification
- `wood_raised_bed_textured.glb` — derived, ignore

Missing for the acceptance-criteria manual verification:
- A "tall asset like a tree trunk" for `tall-narrow` — the spike
  used `synthetic:pole`. T-004-02 manual verification will need a
  real tall GLB or it will have to keep using the synthetic for
  the third manual case. Flag this for design.md.
- A real cube / cuboid hard-surface case independent of the bed.
  The bed comes back as `(planar, hard_surface=Y)` not pure
  `hard-surface`; the spike never measured a 1:1:1 cuboid.

## Constraints and assumptions

- No new external dependencies. The whole repo is stdlib + gltfpack +
  optional Blender; the only Python dep is `numpy` (already required by
  `parametric_reconstruct.py`).
- Single-process, single-user, local. No need for queueing or rate
  limiting on the classify endpoint.
- T-004-03 (strategy router) reads `shape_category` from settings and
  needs to land *after* this ticket. T-004-04 (multi-strategy
  comparison) consumes `shape_confidence`. Both downstream tickets are
  out of scope here, but the field shapes must match what they will
  expect.
- The classifier "doesn't need to be right 100% of the time"
  (ticket §First-Pass Scope). The 0.7 confidence threshold gates the
  comparison UI in T-004-04; the spike's review notes that with the
  current margin/spread formula even clean classifications fall under
  0.7, so design.md must pick a fix.
