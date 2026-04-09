# T-004-05 — Design

Validation ticket. Two real bits of code change (a synthetic asset
generator and a stress-test orientation fix); the rest is wiring tests
and documentation. The design choices are about *what to actually
test* and *how synthetic the synthetic asset is allowed to be*.

## Decision 1: synthetic vs. sourced trellis asset

Options considered:

- **(A) Synthetic GLB built in Python.** Re-use `parametric_reconstruct.py`'s
  `write_glb` pattern in a small new script. Asset is reproducible, the
  generator is checked in, the GLB is byte-stable across machines.
- **(B) Source from TRELLIS.** Run TRELLIS on a trellis prompt, post-
  process, drop into `assets/`. Higher fidelity but the asset is
  opaque, regenerating it requires GPU compute, and the ticket says
  "(sourced from TRELLIS or wherever — even a synthetic one is fine)".
- **(C) Hand-crafted in Blender.** Reproducible only via the .blend
  source file, which would also need to live in `assets/`. No upside
  over (A) for a validation asset.

**Pick: (A).** The whole point of this ticket is to verify the seam
between classifier and strategy router. Visual fidelity matters only to
the manual review portion of acceptance, and even there the bar is
"reads as a row of trellises" (ticket §First-Pass Scope) not "production
quality lattice geometry". A 7-slat horizontal trellis panel built from
axis-aligned boxes is enough to: (i) measure as `directional` via PCA,
(ii) bake into recognizable horizontal slats, (iii) tile in a row.

Asset shape: 2.0 m wide × 0.8 m tall × 0.04 m thick — wider than tall
so the principal PCA axis is horizontal, lifting it into the
`directional` half of the directional/tall-narrow centroid pair.
Approximate make-up: 4 horizontal slats × full width, 7 vertical
stiles × full height; both axis-aligned for hard-surface overlay
robustness.

## Decision 2: where to add the integration test

Options:

- **(A) `classify_shape_test.py`** (Python). Tests the classifier in
  isolation; can't reach the Go strategy layer or settings stamping.
  Already covers `synth_row → directional`.
- **(B) `strategy_handlers_test.go`** (Go). Already covers the
  stamping path with a *synthesized* `ClassificationResult`. Adding a
  test here that drives the real `RunClassifier` against the on-disk
  asset gets full coverage of the seam and reuses existing helpers.
- **(C) New end-to-end Go test file.** Same coverage as (B) but
  splatters tests across more files for no organizational gain.

**Pick: (B).** A new test `TestTrellisAssetClassifiesAsDirectional`
adjacent to the existing strategy-stamping tests. It runs the *actual*
Python classifier on `assets/trellis_synthetic.glb`, feeds the result
through `applyClassificationToSettings`, and pins:

- `ShapeCategory == "directional"`
- `SliceAxis == SliceAxisAutoHorizontal`
- `SliceDistributionMode == "equal-height"`
- `VolumetricLayers == 4`

This is the marquee test the ticket asks for — one assertion that any
future drift in any of the four pieces (Python classifier, Go subprocess
wrapper, strategy table, stamping helper) breaks immediately.

The test is gated on `python3` being on `PATH`. We `t.Skip` rather
than fail if the subprocess errors with `exec: "python3": executable
file not found in $PATH`, mirroring how the existing
`classify_shape_test.py` is opt-in via the test runner.

## Decision 3: stress-test orientation fix scope

The ticket calls out the row-of-5 stress test as part of acceptance.
The current behavior is hardcoded `randomRotateY=true` for the regular
preview path (app.js:3284). Three options:

- **(A) Hardcode `false` for category=directional.** Smallest delta,
  but bakes a special-case into the stress test that S-006 will have to
  unwind.
- **(B) Read `instance_orientation_rule` from the JS STRATEGY_TABLE.**
  Adds the field that strategy.go already declares but JS hasn't
  mirrored yet, then derives `randomRotateY` from it. Same one-line
  effective change at the call site, but the source of truth lives in
  the strategy table where the rest of the per-category policy already
  lives.
- **(C) Persist `instance_orientation_rule` to disk.** Schema bump,
  forward-compat normalization, settings_test.go updates. Bigger move
  than the ticket asks for; S-006's job.

**Pick: (B).** Mirror the rule into `STRATEGY_TABLE` and read it via
`currentSettings.shape_category`. The strategy table is already
documented (app.js:345) as a hand-mirror of strategy.go's table;
adding one more field follows the existing pattern. The runtime
behavior is: `randomRotateY = (rule !== 'fixed' && rule !== 'aligned-to-row')`.

`aligned-to-row` (planar) doesn't get its full S-006 implementation in
this ticket — but it should also not randomly rotate, so the same
gating applies.

## Decision 4: visual / bake validation

The bake itself runs in the browser. Two ways to handle the visual
acceptance criteria:

- **(A) Headless screenshots via Playwright/Puppeteer.** Heavy
  dependency to land for one screenshot.
- **(B) Manual + documented in review.md.** A "designer-grade demo" is
  by definition an eyes-on judgment.

**Pick: (B).** review.md will:

1. Document the manual steps (start the server, drag the trellis asset
   into the upload UI, observe the auto-classification toast, hit Bake,
   look at the produced volumetric GLB in the preview, run the stress
   test at count=5).
2. Include a checklist for the human reviewer.
3. Mark "screenshots" as a TODO that the next human session should
   attach. The ticket text says "with screenshots"; the artifact format
   does not support binary attachments from a CLI environment, so this
   is the honest representation.

## Rejected: rewriting the strategy router

The ticket says "Any classifier or router bugs found are fixed (this
is the first real validation)". The integration test is the lever that
*surfaces* such bugs; if the test passes on the first run there is no
router fix to make. The plan reserves a contingency step for "if the
test fails, diagnose and fix" — but does not pre-suppose a bug.

## Risks

- **PCA-vs-disambiguation edge case.** If the synthetic trellis is too
  close to square (width / height ≈ 1.0), the principal axis could
  flip on minor numerical perturbation and tip the classifier into
  `tall-narrow`. Mitigation: 2.0 × 0.8 has a 2.5:1 horizontal aspect
  ratio, well clear of any boundary. Validate by running the
  classifier on the asset before relying on the test.
- **Hard-surface overlay false positive.** The trellis is built from
  axis-aligned boxes, which is exactly what the hard-surface overlay
  hunts. If `mean_peakiness ≥ 2.5` and `axis_alignment ≥ 0.9` the
  category gets clobbered to `hard-surface`. Mitigation: enough slats
  / stiles that vertex density is *uniform* along each axis (not
  spiked at face boundaries). The lattice spacing is the lever — too
  few struts spike the histogram, too many smear it. Empirically
  tuned in the implementation step.
