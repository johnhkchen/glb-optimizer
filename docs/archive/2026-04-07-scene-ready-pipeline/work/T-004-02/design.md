# T-004-02 — Design

## Decisions

1. **Language**: Python script + Go subprocess wrapper.
2. **Wire format**: classifier prints a single JSON object on stdout;
   the Go wrapper parses it and persists it into `AssetSettings`.
3. **Auto-classify on upload**: synchronous, in the same request, with
   "swallow errors → unknown" semantics so a classifier failure never
   blocks an upload.
4. **Confidence formula**: switch to a softmax-style
   `exp(-d_best/scale) / Σ exp(-d_i/scale)`, scale tuned so the spike
   test cases land in the 0.5–0.95 range.
5. **Endpoint shape**: `POST /api/classify/:id` re-runs the classifier
   on disk and persists. No body. Returns the updated `AssetSettings`.

Each decision is grounded against the alternatives below.

## Decision 1 — Python or Go

| Option            | Pros                                          | Cons                                                                  |
|-------------------|-----------------------------------------------|-----------------------------------------------------------------------|
| **A. Python**     | numpy already a repo dep; spike is in Python; trivial linalg | Subprocess overhead, JSON serialization; requires `python3` on PATH |
| B. Go (gonum)     | No subprocess; type-safe                      | Adds gonum module dep; the rest of the repo is stdlib-only (CLAUDE.md) |
| C. Go (hand-roll) | No deps                                       | Hand-rolling 3×3 symmetric eigendecomp + power iteration is fragile and the spike's review says "the script is throwaway, re-implement in the language the production classifier needs" — that's a re-impl cost we owe regardless |

**Pick A.** The spike's review explicitly leaves the language choice
open and says either is fine. Go-side gonum would be a *new* third-party
dep and the project has been disciplined about staying stdlib-only.
The parametric pipeline already runs Python via subprocess
(`scene.go:84`), so the operational profile is identical.

## Decision 2 — Subprocess wire format

| Option                                | Pros                              | Cons                                                                |
|---------------------------------------|-----------------------------------|---------------------------------------------------------------------|
| **A. JSON on stdout, parsed by Go**   | Structured; fits analytics payload directly; matches the acceptance criteria's `{category,confidence,features}` shape | None worth listing |
| B. Script writes a JSON file Go reads | Mirrors the parametric pattern    | Extra disk I/O for a tiny payload; requires a temp-file dance |
| C. CLI exit code only                 | Simplest                          | Cannot carry confidence or features                                 |

**Pick A.** Print one JSON line on stdout, log progress to stderr.
`exec.Command(...).Output()` returns stdout cleanly while stderr is
left for human-readable logging on failures. The classifier's output is
small (≲ 2 KB) so we don't need to worry about pipe buffering.

## Decision 3 — Auto-classify on upload: sync vs async

| Option           | Pros                                          | Cons                                                              |
|------------------|-----------------------------------------------|-------------------------------------------------------------------|
| **A. Sync**      | Simplest; new uploads immediately have a category for the file list; no `processing` state to expose | Blocks the upload response on classifier latency |
| B. Async (queue) | Non-blocking upload                           | Requires the unused `queue` channel in main.go to actually do something, plus a status field to expose progress |
| C. Lazy (on-read)| No upload-time work                           | Pushes latency onto the user when they open the asset; T-004-03's strategy router would have to handle "no category yet" |

**Pick A.** Spike measurements: classifier on a 4k-vertex sample is
sub-second. On a real 50k-vertex TRELLIS output, sampling stays
linear and stays well under a second. The upload handler is already
sequential (it streams the file to disk and *then* returns). Adding
~500 ms of classification onto an upload that already involves a file
copy is acceptable. Failure tolerance is the key safety net: if the
classifier panics or `python3` is missing, we log to stderr and
continue with `category="unknown"`.

## Decision 4 — Confidence formula

The spike's `(d2 - d1)/(d2 + d1)` margin formula always picks the
right class but maxes around 0.6. Three alternatives:

- **A. Softmax with tuned temperature**:
  `confidence = exp(-d_best/T) / Σ exp(-d_i/T)`. Smooth, monotone,
  in [0,1] by construction. Tunable via T.
- B. Per-class learned scale. Overkill for first-pass scope.
- C. Lower the comparison-UI threshold from 0.7 to ~0.25. Hides the
  problem rather than fixing it; T-004-04 still has to read the value
  and the user-visible meaning of "70% confident" should match the
  number.

**Pick A.** Set `T = 0.20` based on the spike's measured distances:
the calibrated centroids put the nearest non-best class at distance
~0.4–0.6 from a clean rose, planar, or pole. With `T = 0.20`,
`exp(-0.05/0.20) ≈ 0.78` for the best class and `exp(-0.5/0.20) ≈ 0.08`
for the runner-up, yielding `~0.78 / (0.78 + 0.08 + ...) ≈ 0.85`.
That clears the 0.7 threshold for clean cases and sits below it for
borderline cases (the design intent of T-004-04).

## Decision 5 — Endpoint shape

| Option                                | Pros                          | Cons                                              |
|---------------------------------------|-------------------------------|---------------------------------------------------|
| **A. POST /api/classify/:id, no body** | Simplest; matches AC's "reclassify endpoint for existing files"; idempotent | None |
| B. POST /api/classify/:id, body=force | Lets caller force re-run     | Pointless — re-run is the *only* thing the endpoint does |
| C. PUT /api/settings/:id with shape fields | Reuses settings handler  | Crosses concerns; client could lie about the category, defeating "this is the analytics-grade truth" |

**Pick A.** No body. Server reads the GLB from `originalsDir`, runs
the classifier, merges the result into the asset's settings via
`LoadSettings → mutate → SaveSettings`, marks the FileRecord dirty,
emits an analytics `classification` event, and returns the updated
`AssetSettings`. This mirrors `handleAccept` more than `handleSettings`.

## Open questions resolved during design

1. **Does the classifier need its own JSONL artifact like the spike's
   per-asset CSV?** No. The analytics event already carries
   `{category, confidence, features}`; the export pipeline will pick
   it up via the existing event stream. No new directory.

2. **Should the categories include `synthetic:*` or only the six
   user-facing names?** Six only. Synthetic shapes are spike-only.

3. **What about the rotational-symmetry edge case?** When `r2/r3 > 0.5`
   (the trailing eigenvalues are too close to disambiguate the 2nd vs
   3rd axis), the classifier falls back to AABB axis-alignment instead
   of PCA-axis alignment for the hard-surface overlay. Documented in
   `structure.md`.

4. **Cube test case?** Add `synthetic:cube` to the production classifier's
   self-test entry-point. The acceptance criteria's "raised bed for
   hard-surface" lands as `(planar, hard_surface=Y)` per the spike, so
   the cube is what proves the overlay is the *only* discriminator
   between a cuboid and a round bush.

5. **Tree-trunk asset for the manual verification?** Use
   `synthetic:pole` from the spike (re-implemented inside the prod
   script) until a real trunk GLB is added to `assets/`. Document in
   review.md.

## What is rejected and why

- **Storing features outside settings.** Considered a sidecar JSON in
  `~/.glb-optimizer/classifications/`. Rejected because the only
  durable consumer is the analytics event stream and the
  `AssetSettings.shape_category/confidence` fields. Features are
  ephemeral data captured at classify-time.
- **Schema version bump.** Considered bumping `SettingsSchemaVersion`
  to 2 because two new fields are landing. Rejected per
  `settings-schema.md` §"Adding fields without a version bump":
  additive optional fields with sensible defaults do not require a
  bump. Forward-compat normalization handles missing fields.
- **Vertex sub-sampling cap.** Considered capping vertex count at
  10k to bound classifier latency. Rejected: the spike's PCA is
  O(N) in vertex count and a 50k mesh is still fast. Add only if
  a real measurement shows a problem.
