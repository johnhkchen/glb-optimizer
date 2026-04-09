# T-010-03 Review — Pack Button & Endpoint

## Summary

T-010-03 wires the existing `CombinePack` (T-010-02) and
`BuildPackMetaFromBake` (T-011-02) into the production preview UI:

- **New backend endpoint** `POST /api/pack/:id` reads the three baked
  intermediates from `outputs/`, runs the combine step, and writes
  the resulting GLB to `dist/plants/{species}.glb`.
- **New toolbar button** "Build Asset Pack" lives next to the
  existing "Build hybrid impostor" button inside the Advanced
  disclosure. It is enabled exactly when the asset has a side
  billboard plus at least one of the optional layers
  (tilted or dome).
- **New analytics event** `pack_built` is emitted via the existing
  `logEvent` helper on every click — both successes and failures —
  so the tuning pipeline sees attempt rates as well as outcomes.

The handler is a thin glue layer over two well-tested existing
units; the substantive new code is the test fixture and the
small JS state machine.

## Files Changed

### Created

- `handlers_pack_test.go` — 245 lines, 7 tests covering every
  status-code branch of the new handler.
- `docs/active/work/T-010-03/{research,design,structure,plan,progress,review}.md` —
  the six RDSPI artifacts.

### Modified

- `handlers.go` — appended `handleBuildPack` (~106 lines) at the
  end of the file. No other handler touched, no imports changed.
- `main.go` — added `distPlantsDir`, included it in the startup
  mkdir loop, and registered `/api/pack/` on the mux. 4 lines added.
- `static/index.html` — one new `<button id="buildPackBtn">` inside
  `.advanced-panel`.
- `static/app.js` — element handle, `buildAssetPack(id)` async
  function, click listener, and one new line in
  `updatePreviewButtons`. ~85 lines added total.

### Deleted

None.

## Test Coverage

`go test ./...` is green. The new `handlers_pack_test.go` covers
the following branches:

| Branch                                         | Status code | Test                                 |
|------------------------------------------------|-------------|--------------------------------------|
| Happy path with all three intermediates        | 200         | `TestHandleBuildPack_HappyPath_AllThree` |
| Optional volumetric absent                     | 200         | `TestHandleBuildPack_TiltedOnly`     |
| Optional tilted absent                         | 200         | `TestHandleBuildPack_VolumetricOnly` |
| Required side intermediate missing             | 400         | `TestHandleBuildPack_MissingSide`    |
| Unknown asset id                               | 404         | `TestHandleBuildPack_UnknownID`      |
| Wrong HTTP method                              | 405         | `TestHandleBuildPack_MethodNotAllowed` |
| Combined output exceeds 5 MiB cap              | 413         | `TestHandleBuildPack_OversizePack`   |

The 413 test is the load-bearing one — it is the only assertion
that exercises the error-string match against
`CombinePack`'s "exceeds 5 MiB cap" error.

### Coverage Gaps

- **Generic 500 path** (e.g. an `os.WriteFile` failure when the
  dist dir is read-only) is not exercised. The branch is one
  `jsonError` line and the cost of constructing the test fixture
  (chmod-ing a tempdir, swallowing skip on Windows) outweighs the
  signal. Acceptable.
- **Frontend has no automated tests.** The repo has no JS test
  runner. The button enable-state logic is one boolean expression
  and the click handler is a straightforward HTTP call; both are
  trivially inspectable in the diff. Same testing posture as every
  other UI change in this codebase.
- **Concurrent pack requests for the same id** would race on the
  `os.WriteFile` of `dist/plants/{species}.glb`. Not tested
  because the demo is single-user / single-tab and the worst case
  is "the second writer wins". If T-010-04's batch packing later
  parallelizes by species, this becomes a non-issue (different
  species → different output paths) and only matters if the same
  asset id is packed twice in flight.

## Open Concerns

### Error-string match for the 5 MiB cap

`handleBuildPack` distinguishes the 413 path from a generic 500 by
substring-matching `"5 MiB cap"` in `CombinePack`'s error. If a
future change to `combine.go` reworks that error message, the
handler will silently downgrade the 413 to a 500 and the UI will
show a generic "combine failed" message instead of the canonical
"reduce variant count" hint.

The right fix is a sentinel error in `combine.go` (e.g.
`var ErrPackTooLarge = errors.New("...")`) that the handler can
match with `errors.Is`. Deferred because:

1. T-010-02 only just landed and is being reviewed in parallel.
2. The string is asserted by both `TestCombine_SizeCapRejection`
   (combine_test.go:391) and the new
   `TestHandleBuildPack_OversizePack`. If anyone changes the
   message in `combine.go` without updating both call sites, both
   tests fail loudly. The match is therefore safe at HEAD; the
   "future drift" risk is bounded.

Recommendation: file a follow-up to introduce the sentinel during
post-demo cleanup, but not in this ticket.

### Species derivation collisions

`{species}.glb` is the on-disk name. If two assets resolve to the
same `species` slug (e.g. two uploads of the same plant model),
the second pack overwrites the first. This is intentional —
re-baking should replace the old artifact — but it does mean the
endpoint is not idempotent across asset ids. Documented in
`design.md`; not flagged as a bug.

### `prepareError` reuse

The "existing toast/log area" called for in the ticket does not
exist. We reused the `prepareError` div from the prepare-for-scene
flow because it is the only in-toolbar text surface. This works,
but the same div is now multiplexed between two unrelated flows;
a future user could see a "Pack failed" message lingering after
they trigger Prepare-for-scene. Two mitigations are already in
place: `buildAssetPack` clears the field on entry, and
`prepareForScene` clears it on entry as well. A real toast widget
is the long-term fix; out of scope here.

## TODOs / Follow-ups

- [ ] (post-demo) Introduce `ErrPackTooLarge` in `combine.go` and
      switch the handler from substring matching to `errors.Is`.
- [ ] (post-demo) Replace `prepareError` reuse with a real toast
      component once a second consumer needs it.
- [ ] T-010-04 — `just pack-all` recipe — is now unblocked: it can
      either shell out to `glb-optimizer pack <id>` (preferred per
      that ticket's notes) or call this HTTP endpoint against a
      running server.

## Critical Issues for Human Attention

None. The implementation matches the acceptance criteria
verbatim, all tests pass, and `go vet` is clean.

The two design choices most worth reviewer eyes are:

1. **`distDir` location.** Placing it under `workDir/dist/plants/`
   makes it a sibling of `outputs/` (and therefore consistent with
   the sibling T-010-04 ticket's "walks `outputs/` for asset ids,
   refreshes `dist/plants/`" framing). If reviewers expected
   `dist/` to live in the source tree instead of the working
   directory, this is the line to flip.
2. **Always-emit analytics events.** Failures are recorded with
   `size: 0` and the species field empty. This matches the
   convention used by the `regenerate` events in the existing
   generate-* functions. If the tuning pipeline expects only
   successful events, the click handler needs an `if (success)`
   guard around the `logEvent` call.

## Acceptance Criteria Crosswalk

- ✅ New handler in `handlers.go`: `handleBuildPack` registered at
  `POST /api/pack/:id`
- ✅ Reads the three intermediates from `outputsDir` for the asset
- ✅ Constructs `PackMeta` via `BuildPackMetaFromBake(id)`
- ✅ Calls `CombinePack(...)`
- ✅ Writes result to `dist/plants/{species}.glb`
  (under `workDir/dist/plants/` — see "Critical Issues" #1 above
  for the location rationale)
- ✅ Returns JSON `{ pack_path, size, species }`
- ✅ Returns 400 if required intermediates are missing, 500 on
  combine error, 413 if pack exceeds 5 MB
- ✅ New button in `static/index.html` "Build Asset Pack" — visible
  only when the asset has both `has_billboard` and either
  `has_billboard_tilted` or `has_volumetric`
- ✅ Click handler in `static/app.js` calls the endpoint, surfaces
  success/error in `prepareError`, fires `pack_built` analytics
  event with `{species, size, has_tilted, has_dome}`
- ✅ 400/413 errors render a clear message in the UI ("Pack
  exceeds 5 MB — reduce variant count or texture resolution and
  re-bake")

All acceptance criteria are satisfied.
