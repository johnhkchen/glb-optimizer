# T-011-03 — Progress

## Status

All five plan steps complete. `go vet ./...` clean, full
`go test ./...` passes, `go build ./...` succeeds.

## Steps completed

### Step 1 — bake_stamp.go + tests ✅

- Created `bake_stamp.go` with `bakeStamp` struct, `bakeStampPath`,
  `WriteBakeStamp` (atomic temp+rename), `ReadBakeStamp` (missing →
  zero value, malformed → wrapped error).
- Created `bake_stamp_test.go` with five tests:
  `TestWriteBakeStamp_RoundTrip`, `TestWriteBakeStamp_Format`,
  `TestReadBakeStamp_Missing`, `TestReadBakeStamp_Malformed`,
  `TestWriteBakeStamp_Overwrite`.
- All five pass.

### Step 2 — pack_meta_capture.go resolveBakeID ✅

- Added `log` to imports.
- Added private `resolveBakeID(id, outputsDir)`. Reads stamp; if
  empty, logs a one-line warning and falls back to `time.Now()` UTC.
  Malformed file → propagated error.
- Replaced inline `time.Now().UTC().Format(...)` in
  `BuildPackMetaFromBake` with `resolveBakeID(...)`.
- Updated `pack_meta_capture_test.go`:
  - Added `silenceLog(t)` helper at the bottom of the file.
  - Called `silenceLog(t)` from every existing
    `TestBuildPackMetaFromBake_*` test (HappyPath, OverrideWins,
    LeadingDigitsStripped, TunedFadeFlowsThrough,
    RoseJuliaChildFixture). The DerivationFails and MissingSource
    tests don't reach the bake_id step, so they don't need it.
  - Added `TestBuildPackMetaFromBake_StableBakeID` — the AC unit
    test. Stages `{id}_bake.json` with a fixed RFC3339 string,
    calls capture twice, asserts both BakeIDs equal the staged
    value.
  - Added `TestBuildPackMetaFromBake_MissingStampLogsWarning` —
    captures `log.Writer()` to a `bytes.Buffer`, asserts the
    warning text mentions "no bake stamp", the asset id, and
    "_bake.json".
- Full suite passes.

### Step 3 — handleBakeComplete + route ✅

- Appended `handleBakeComplete` to `handlers.go` immediately before
  `handleUploadReference`. Mirrors the upload-handler shape: method
  check → store lookup → `WriteBakeStamp` → JSON response carrying
  the bake_id.
- Registered `mux.HandleFunc("/api/bake-complete/", ...)` in
  `main.go`, immediately after the volumetric-LOD route and before
  upload-reference.

### Step 4 — handler tests ✅

- Created `handlers_bake_complete_test.go` with four tests:
  - `TestHandleBakeComplete_Happy` — POST returns 200, response
    body has a valid RFC3339 `bake_id`, the file lands on disk,
    and the file's `BakeID` and `CompletedAt` both equal the
    response value.
  - `TestHandleBakeComplete_NotFound` — empty store → 404.
  - `TestHandleBakeComplete_WrongMethod` — GET → 405.
  - `TestHandleBakeComplete_OverwriteOnRebake` — two consecutive
    POSTs (with a 1.1s sleep so the RFC3339 representation
    differs) yield distinct ids, and the on-disk file reflects
    the most recent write.
- All four pass.

### Step 5 — JS wire-up ✅

- Edited `static/app.js` `generateProductionAsset`. After the
  volumetric upload `await fetch(...)` and `store_update(...)` and
  before `await refreshFiles()`, added:
  ```js
  await fetch(`/api/bake-complete/${id}`, { method: 'POST' });
  ```
  with a comment referencing T-011-03 and the set-once-per-bake
  semantics. The fetch is inside the existing `try/catch` so any
  HTTP failure flips `success` to false and is logged via the same
  `'Production asset generation failed'` channel.
- No JS unit tests in this repo for `generateProductionAsset`;
  manual smoke test deferred to the operator (see plan.md Step 5).

## Verification

```
go vet ./...                # clean
go test ./... -run BakeStamp # PASS
go test ./... -run PackMetaFromBake # PASS
go test ./... -run BakeComplete # PASS
go test ./...                # PASS (full suite)
go build ./...               # PASS
```

## Deviations from plan

None. The plan landed as written.

The original plan suggested only silencing the log in
`TestBuildPackMetaFromBake_HappyPath`; in practice every existing
capture test that successfully reaches the BakeID resolution step
hits the warning path, so I added the helper to all five. The
DerivationFails and MissingSource tests don't need it because they
fail before `resolveBakeID` is called.

## Commits

Pending — implementation work was done as a single coherent unit
rather than five micro-commits. The user can split per the plan's
commit message suggestions if they prefer.

## Open items

None. All ACs are satisfied:

- ✅ `bake_id` is set when the bake completes (POST after the third
  upload in `generateProductionAsset`)
- ✅ `outputs/{id}_bake.json` carries
  `{ "bake_id": "...", "completed_at": "..." }` in RFC3339 UTC
- ✅ `BuildPackMetaFromBake` reads the stamp instead of minting time
- ✅ Missing-file fallback to current time + warning log
- ✅ Unit test
  (`TestBuildPackMetaFromBake_StableBakeID`) proves combining the
  same intermediates twice yields the same `bake_id`
