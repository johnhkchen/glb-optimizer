# T-004-02 — Structure

File-level blueprint for the production shape classifier.

## Files created

### `scripts/classify_shape.py`

Production classifier. ~300 lines. Stdlib + numpy. **Does not import
from `spike_shape_pca.py`** per the spike's review handoff.

Public CLI:

```
python3 scripts/classify_shape.py <input.glb>
python3 scripts/classify_shape.py --self-test
```

Stdout (one line, JSON):

```json
{
  "category": "round-bush",
  "confidence": 0.84,
  "is_hard_surface": false,
  "features": {
    "n_points": 12345,
    "dimensions": [1.2, 0.9, 1.1],
    "aspect_ratio": 0.81,
    "eigenvalues": [...],
    "eigenvectors": [[..],[..],[..]],
    "ratios": {"r2": 0.87, "r3": 0.49},
    "principal_axis": [0,1,0],
    "principal_orientation": "vertical",
    "axis_alignment": 0.93,
    "peakiness": [2.1,2.4,2.0],
    "mean_peakiness": 2.17
  }
}
```

Stderr: human-readable progress / warnings only. Non-zero exit on
parser errors, classifier crashes, or unsupported GLBs.

Internal organization (top-down):

- Constants: `CLASS_CENTROIDS` (calibrated from spike), 
  `HARD_SURFACE_THRESHOLDS` (calibrated), `CONFIDENCE_TEMPERATURE = 0.20`,
  `ROT_SYMMETRY_RATIO = 0.5`.
- GLB loader: `load_all_positions(path)` — walks the node tree,
  composes TRS / matrix transforms, concatenates positions across all
  primitives of all meshes, returns a single `(N,3)` float64 array.
  Replaces the spike's single-mesh assertion.
- PCA: `compute_pca(points)` — same shape as the spike's, returns a
  feature dict.
- Hard-surface overlay: `apply_hard_surface_overlay(features, points)`
  — uses PCA-axis alignment when `r2/r3 ≤ ROT_SYMMETRY_RATIO`,
  AABB-axis alignment as fallback when the trailing eigenvectors are
  unreliable.
- Classification: `classify(features)` returns `(category, confidence,
  is_hard_surface, ranking)` with the softmax confidence.
- `main()`: argparse, single-input → JSON-on-stdout, or `--self-test`
  to run the bundled synthetic + asset suite and print a table.

The synthetic test cases (used by `--self-test`) are inlined: cube,
sphere/round-bush, planar lattice, horizontal row, vertical pole.

### `scripts/classify_shape_test.py`

Pytest-style unit tests, ~120 lines, runnable as
`python3 scripts/classify_shape_test.py`. The repo currently has no
pytest dependency so tests use bare `assert` and a tiny test runner.
Coverage:

- `test_round_bush_synthetic` — round bush points classify correctly.
- `test_planar_lattice` — `(planar, HS=Y)`.
- `test_directional_row` — `(directional, HS=Y)`.
- `test_tall_pole` — `(tall-narrow, HS=Y)`.
- `test_cube` — `(round-bush or planar, HS=Y)` — overlay is the only
  thing that calls it hard-surface; the shape class is whatever PCA
  returns (acceptable per ticket).
- `test_rotational_symmetry_fallback` — pole-shaped input where the
  trailing eigenvectors are arbitrary; AABB fallback still flags HS.
- `test_node_transform_compose` — synthetic 2-mesh GLB with non-identity
  node matrix; positions in world space.
- `test_softmax_confidence_in_range` — confidence stays in `[0,1]`.

Real-asset checks (`assets/rose_julia_child.glb`,
`assets/wood_raised_bed.glb`) live in the `--self-test` mode of the
main script, not the unit test file, so the unit test file has no
fixtures-on-disk dependency.

### `classify.go`

New Go file. ~120 lines. Public surface:

```go
type ClassificationResult struct {
    Category      string                 `json:"category"`
    Confidence    float64                `json:"confidence"`
    IsHardSurface bool                   `json:"is_hard_surface"`
    Features      map[string]interface{} `json:"features"`
}

func RunClassifier(glbPath string) (*ClassificationResult, error)
```

Implementation:

- `exec.Command("python3", "scripts/classify_shape.py", glbPath)`
- Returns `(*ClassificationResult, error)`. On non-zero exit, returns
  an error wrapping the stderr text. On JSON parse failure, same.
- No retries, no timeout (single-user local tool).

### `classify_test.go`

Go-side unit tests. Black-box via `RunClassifier` against
`assets/rose_julia_child.glb` (skip if missing) and a synthetic
in-process file written to `t.TempDir()`. Skips the whole file if
`python3` is not on PATH. ~60 lines.

## Files modified

### `settings.go`

Add two fields to `AssetSettings`, in declaration order at the **end**
of the struct (matters because declaration order is on-disk JSON
order — placing new fields last preserves the order of old, valid
files):

```go
ShapeCategory   string  `json:"shape_category,omitempty"`
ShapeConfidence float64 `json:"shape_confidence,omitempty"`
```

- Add `validShapeCategories` map: `round-bush`, `directional`,
  `tall-narrow`, `planar`, `hard-surface`, `unknown`.
- `DefaultSettings()`: `ShapeCategory: "unknown"`, `ShapeConfidence: 0`.
- `Validate()`: enum check + range check on confidence `[0,1]`.
- `SettingsDifferFromDefaults()`: include the two new fields.
- `LoadSettings()`: forward-compat normalization. If
  `s.ShapeCategory == ""`, set it to `"unknown"`. (Use the same
  inline pattern as `slice_distribution_mode`.)

### `analytics.go`

- Add `"classification"` to `validEventTypes`.

### `docs/knowledge/analytics-schema.md`

- Document the new event type. Payload shape:
  `{"category":string, "confidence":float, "features":object}`.

### `docs/knowledge/settings-schema.md`

- Add the two new rows to the field table.
- Note that they are populated by the classifier, not the user.

### `handlers.go`

- New handler: `handleClassify(store, originalsDir, settingsDir, logger)`.
  POST only. No body. Steps:
  1. Resolve `id` from URL.
  2. 404 if asset not in store.
  3. Call `RunClassifier(filepath.Join(originalsDir, id+".glb"))`.
  4. On error: 500 with the error message.
  5. `LoadSettings(id, settingsDir)`, mutate `ShapeCategory` /
     `ShapeConfidence`, validate, `SaveSettings`.
  6. `store.Update` to set `HasSavedSettings=true` and recompute
     `SettingsDirty`.
  7. Emit `classification` analytics event via
     `LookupOrStartSession + AppendEvent`. Failure to emit logs to
     stderr but does not fail the request (mirrors `handleAccept`).
  8. Respond with the updated `AssetSettings`.

- Modify `handleUpload`. After each successful file write and
  `store.Add`, run a best-effort classify:
  1. `RunClassifier(...)`. On error: log to stderr, continue.
  2. On success: `LoadSettings → mutate → SaveSettings`.
  3. `store.Update` flags.
  4. Emit analytics event (best-effort, errors swallowed).

  This is wrapped in a small helper `autoClassify(record, ..., logger)`
  to avoid bloating `handleUpload`.

### `main.go`

- Mount the new route:
  `mux.HandleFunc("/api/classify/", handleClassify(store, originalsDir, settingsDir, analyticsLogger))`.
- `handleUpload(...)` signature gains `settingsDir, logger` params so
  the auto-classify hook has what it needs.
- `scanExistingFiles` does **not** auto-classify on startup.
  Reclassification of existing files is opt-in via the endpoint, not
  a startup-time stampede.

### `settings_test.go`

- New cases for: round-trip with shape fields set; validate rejects
  unknown category; validate rejects out-of-range confidence; load
  of an old document missing `shape_category` returns
  `"unknown"`; `SettingsDifferFromDefaults` flips on shape changes.

### `analytics_test.go`

- New case: `classification` event type passes `Validate()`.

## Files deleted

None. The spike script (`scripts/spike_shape_pca.py`) stays in place
as research history per the spike's handoff notes.

## Ordering and dependencies

The order of changes is load-bearing for compile and test:

1. `settings.go` — add fields + validation.
2. `analytics.go` — add event type.
3. `scripts/classify_shape.py` — script lands first; `RunClassifier`
   has nothing to call without it.
4. `classify.go` — Go wrapper.
5. `handlers.go` — `handleClassify`, `autoClassify` helper.
6. `main.go` — route + handleUpload signature.
7. Tests follow the production code in each step.
8. Docs (`settings-schema.md`, `analytics-schema.md`) updated last.

## Public interface notes for downstream tickets

- T-004-03 reads `s.ShapeCategory` from settings and routes on it.
  The string values are the enum members above; T-004-03 must handle
  `"unknown"` as "use default strategy".
- T-004-04 reads `s.ShapeConfidence` and triggers the comparison UI
  when `< 0.7` (per ticket). With the softmax temperature in
  design.md, clean classifications land at ~0.8 and borderline cases
  drop below 0.7 — exactly the design intent.
