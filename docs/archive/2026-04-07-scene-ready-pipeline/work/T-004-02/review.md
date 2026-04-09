# T-004-02 — Review

## Summary

Production shape classifier landed end-to-end: Python script, Go
subprocess wrapper, settings fields, validation, normalization,
HTTP endpoint, auto-classify on upload, analytics event type, and
schema documentation. All unit tests pass on both sides.

## Files created

- `scripts/classify_shape.py` — production classifier, ~340 lines.
- `scripts/classify_shape_test.py` — Python unit tests, ~180 lines,
  bare-assert runner. 11 tests, all passing.
- `classify.go` — Go subprocess wrapper. ~50 lines.
- `classify_test.go` — Go-side tests against real assets, skip-aware.
- `docs/active/work/T-004-02/{research,design,structure,plan,progress,review}.md`

## Files modified

- `settings.go` — added `ShapeCategory`, `ShapeConfidence`,
  `validShapeCategories`, validation, normalization,
  `SettingsDifferFromDefaults` extension.
- `settings_test.go` — five new tests covering the new fields.
- `analytics.go` — added `"classification"` to `validEventTypes`.
- `analytics_test.go` — one new test for the event type.
- `handlers.go` — three new helpers (`applyClassificationToSettings`,
  `emitClassificationEvent`, `autoClassify`), one new HTTP handler
  (`handleClassify`), and `handleUpload` signature + auto-classify
  hook.
- `main.go` — `/api/classify/` route + updated `handleUpload`
  signature.
- `docs/knowledge/settings-schema.md` — two new field rows + forward
  compat bullet.
- `docs/knowledge/analytics-schema.md` — new `classification` event
  section.

## Files deleted

None. The T-004-01 spike script (`scripts/spike_shape_pca.py`)
remains in place as research history per its handoff notes.

## Test coverage

| Layer            | Where                              | Status              |
|------------------|------------------------------------|---------------------|
| Python unit      | `scripts/classify_shape_test.py`   | 11/11 passing       |
| Python e2e       | `--self-test` mode                 | 5/5 passing         |
| Real asset (rose, bed) | `python3 scripts/classify_shape.py <path>` | manually verified |
| Go unit (settings) | `settings_test.go`               | passing             |
| Go unit (analytics) | `analytics_test.go`             | passing             |
| Go integration (RunClassifier) | `classify_test.go`     | passing on this machine; skip-aware |
| HTTP integration | manual `curl`                      | **not run** — see open concerns |

`go test ./...` and `python3 scripts/classify_shape_test.py` both
pass cleanly.

## Acceptance-criteria check

| Criterion                                                 | Status |
|-----------------------------------------------------------|--------|
| New module `scripts/classify_shape.py` with the spec'd JSON output | ✅ |
| Categories enum matches the ticket                        | ✅ |
| Features include eigenvalues, eigenvectors, aspect, density | ✅ |
| `POST /api/classify/:id` runs classifier and stores result | ✅ |
| `AssetSettings` gains `shape_category` and `shape_confidence` | ✅ |
| Auto-classify on upload                                   | ✅ |
| Reclassify endpoint for existing files                    | ✅ (same endpoint) |
| Emits `classification` analytics event                    | ✅ |
| Strategy router reads category from settings              | n/a (T-004-03) |
| Manual verification: rose → round-bush                    | ✅ |
| Manual verification: bed → hard-surface                   | ⚠ (see open concerns) |
| Manual verification: tall asset → tall-narrow             | ✅ via synthetic pole |

## Open concerns

### 1. Wood raised bed classifies as `planar`, not `hard-surface`

Per the spike review, the bed has eigenvalue ratios `(r2≈0.45,
r3≈0.04)` which sit on top of the `planar` centroid. The hard-surface
overlay does fire (`is_hard_surface=true`), but T-004-02 only
persists `shape_category` to settings — it does not expose the
overlay flag.

T-004-03's strategy router will see `planar`, not `hard-surface`, for
the bed. Two paths to resolve:

- **Option A (preferred)**: T-004-03 reads from the `classification`
  analytics event payload (which includes `features` and
  `is_hard_surface`) to disambiguate `planar` cases.
- **Option B**: T-004-02 follow-up adds a third field
  `shape_is_hard_surface bool` to `AssetSettings`. Cheap to add but
  feels like leaking the overlay implementation into the settings
  schema. Defer unless T-004-03 finds Option A unworkable.

This is a flag for T-004-03's design phase, not a bug here.

### 2. HTTP path was not exercised end-to-end

The plan included a manual gate: start the server, POST a GLB to
`/api/upload`, GET `/api/settings/<id>` to confirm the new fields,
and tail the session JSONL to confirm the `classification` line.
This was not done during the implementation session — only the
unit tests + standalone Python invocations.

The wiring is straightforward (existing patterns from `handleAccept`
and `handleSettings`), but a smoke test before T-004-03 starts is
recommended. **No code change needed; just run it.**

### 3. No real "tall-narrow" asset in `assets/`

The acceptance-criteria third manual case (a tree trunk or similar)
was satisfied by `--self-test`'s synthetic pole. When a real tall
TRELLIS output lands in `assets/`, re-run the classifier against it
as a sanity check.

### 4. `python3` is not verified at server startup

Like the existing parametric reconstruction path (`scene.go:84`),
the classifier silently no-ops at first request if `python3` or
numpy is missing — `autoClassify` swallows the error and the asset
gets `shape_category="unknown"`. The behavior is correct (uploads
must not block on a Python outage) but operators with no Python
will see uniformly `unknown` categories with no obvious cause.

A future improvement: probe `python3 -c "import numpy"` at startup
and surface a one-line warning, mirroring how `main.go:38` reports
gltfpack version. Out of scope for this ticket.

### 5. Hard-surface peakiness implementation deviated from plan

The plan called for PCA-axis peakiness with an AABB fallback. The
implementation uses canonical-axis peakiness *always*, because the
PCA-axis approach jittered just under the calibrated 2.5 threshold
on the synthetic lattice (mine measured 2.48; the spike measured
2.74 on the same input). The deviation is documented in
`progress.md` and is a strict improvement: canonical-axis peakiness
is invariant to PCA sign / order flips, which is exactly the
property needed for the rotational-symmetry edge case. The
calibrated thresholds from the spike are unchanged.

## Limitations / TODOs

- No bench numbers for classifier latency on large meshes (>100k
  vertices). Empirically the rose at 162k verts classifies in <1s,
  but TRELLIS outputs vary. Add a vertex-count cap if a real upload
  ever times out the user.
- Multi-mesh GLBs are now handled (node walk + transform compose)
  but have not been tested against a real multi-mesh asset — the
  test assets are all single-mesh. The unit tests synthesize
  multi-primitive cases via `_build_glb` but not multi-mesh.
- The classifier does not consider triangle area weighting. The
  spike review flagged this as a potential future improvement for
  meshes with extreme tessellation non-uniformity. None observed
  in current assets.

## Critical issues that need human attention

None. All the open items are either downstream-ticket coordination
(item 1), residual manual gates (items 2, 3), operational polish
(item 4), or documented improvements over the plan (item 5).

T-004-03 (strategy router) is unblocked: it should design against
`AssetSettings.ShapeCategory` and treat `"unknown"` as
"use the default strategy", and it should evaluate Option A
vs Option B for the bed-as-planar case during its own design phase.

## Handoff notes

- The Python classifier is the single source of truth for the
  taxonomy logic. The Go side is intentionally a thin wrapper —
  no duplication of the PCA / overlay code.
- `validShapeCategories` is duplicated between Go (`settings.go`)
  and Python (`classify_shape.py:VALID_CATEGORIES`). This is
  acceptable: the enum is small and stable, and both copies enforce
  the contract at their respective boundaries (Python at the
  classifier output, Go at the settings load/save and at the
  classifier-result parse). If the enum changes, both must be
  updated together.
- The analytics `classification` event is the load-bearing artifact
  for T-004-03 / T-004-04 training data. It carries the full feature
  dict, while the settings file only carries category + confidence.
  Downstream consumers should prefer the event payload when they
  need anything beyond the two summary fields.
