# T-011-03 — Plan

## Implementation sequence

Five small commits, each independently `go vet ./... && go test ./...`
clean.

### Step 1 — `bake_stamp.go` + tests

Create `bake_stamp.go` with:
- `bakeStamp` struct (json tags)
- `bakeStampPath(outputsDir, id) string`
- `WriteBakeStamp(outputsDir, id) (string, error)` — captures one
  `time.Now().UTC().Format(time.RFC3339)` value, populates both
  fields, marshals indented (2-space, matching `SaveSettings`),
  writes via temp+rename
- `ReadBakeStamp(outputsDir, id) (bakeStamp, error)` — `os.ReadFile`,
  `os.IsNotExist` → zero+nil, `json.Unmarshal` errors wrapped with
  `bake_stamp:`

Create `bake_stamp_test.go` with the five unit tests from
structure.md. Verify: `go test ./... -run BakeStamp`.

**Commit message**: `T-011-03: bake stamp file writer + reader`

### Step 2 — `pack_meta_capture.go` resolveBakeID

Edit `pack_meta_capture.go`:
- Add `log` to the import block
- Add `resolveBakeID(id, outputsDir) (string, error)` (private)
- Replace the inline `time.Now()` in `BuildPackMetaFromBake` with a
  call to `resolveBakeID`

Update `pack_meta_capture_test.go`:
- Add `silenceLog(t *testing.T)` helper
- Call it at the top of `TestBuildPackMetaFromBake_HappyPath` (the
  fallback warning would otherwise pollute test output)
- Add `TestBuildPackMetaFromBake_StableBakeID` — stages
  `{id}_bake.json`, calls capture twice, asserts both BakeIDs equal
  the staged value
- Add `TestBuildPackMetaFromBake_MissingStampLogsWarning` — captures
  log to a `bytes.Buffer`, asserts the warning text

Verify: `go vet ./... && go test ./...`. The full suite must pass —
the existing happy-path test should still hold because the file is
absent → fallback path → non-empty `time.Now()` id.

**Commit message**: `T-011-03: capture reads bake stamp with time.Now() fallback`

### Step 3 — `handleBakeComplete` + route

Edit `handlers.go`:
- Append `handleBakeComplete(store *FileStore, outputsDir string) http.HandlerFunc`
  immediately after `handleUploadVolumetric`
- Mirror the existing handlers' shape (method check, store lookup,
  do work, JSON response)

Edit `main.go`:
- Add `mux.HandleFunc("/api/bake-complete/", handleBakeComplete(store, outputsDir))`
  next to the other upload routes

**No new test file** in this step; the next step adds the handler test.

Verify: `go vet ./... && go build ./...`.

**Commit message**: `T-011-03: /api/bake-complete endpoint + route`

### Step 4 — `handleBakeComplete` test

Add `handlers_bake_complete_test.go` (or extend
`handlers_billboard_test.go` if the team prefers consolidation —
default to a new file, mirroring the per-feature naming).

Tests:
1. `TestHandleBakeComplete_Happy` — populated store, POST returns
   200, file exists on disk with valid JSON, response carries the
   same `bake_id`
2. `TestHandleBakeComplete_NotFound` — empty store, POST → 404
3. `TestHandleBakeComplete_WrongMethod` — GET → 405
4. `TestHandleBakeComplete_StableAcrossCalls` — POST twice; the
   second response has a *new* id (rebake mints a new stamp), and
   the file on disk reflects the latest write — the stability
   property lives in the *combine* test, not here

Verify: `go test ./... -run BakeComplete`.

**Commit message**: `T-011-03: handler tests for /api/bake-complete`

### Step 5 — JS wire-up

Edit `static/app.js` `generateProductionAsset`:
- After the volumetric upload `await fetch(...)` and
  `store_update(...)` block (around line 2455), before
  `await refreshFiles()`, add:
  ```js
  // T-011-03: stamp {id}_bake.json so future combines reuse a
  // stable bake_id even when the same intermediates are re-packed.
  await fetch(`/api/bake-complete/${id}`, { method: 'POST' });
  ```

No JS test exists in this repo for `generateProductionAsset` (it is
exercised by manual production button flows), so the verification
is a manual smoke test:

1. `just dev` (or whatever the main run command is)
2. Upload an asset, click **Build hybrid impostor**
3. Wait for the button to return to its idle label
4. `cat outputs/{id}_bake.json` → should contain valid JSON with
   the two timestamps
5. Click **Build hybrid impostor** again → file is rewritten with
   a fresh timestamp
6. (If T-010-02 / T-010-03 are merged) click the new pack button
   and confirm the resulting `dist/plants/*.glb` carries the same
   `bake_id` as `outputs/{id}_bake.json`

**Commit message**: `T-011-03: bake driver stamps bake-complete after volumetric upload`

## Test strategy

| Layer | What | Where |
|---|---|---|
| Unit | bakeStamp write/read round trip, missing file fallback, malformed file error | `bake_stamp_test.go` |
| Unit | resolveBakeID stability, fallback warning log | `pack_meta_capture_test.go` |
| Handler | bake-complete happy/404/405, file written | `handlers_bake_complete_test.go` |
| Manual | full bake → file appears → re-bake rewrites it → combine reuses id | smoke |

The **AC unit test** is
`TestBuildPackMetaFromBake_StableBakeID`: it directly proves that
combining the same intermediates twice produces the same `bake_id`
in the output `PackMeta`, which is the property the asset server
will rely on.

## Verification gates

Before each commit:
- `go vet ./...`
- `go build ./...`
- `go test ./...`

Before opening for review:
- Manual smoke against the live server (Step 5 sub-list)
- `git diff main` reviewed for accidental edits to unrelated files

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| `log.Printf` global pollution between tests | Tests that exercise the warning install `log.SetOutput(buf)` and restore in `t.Cleanup`. |
| `resolveBakeID` masking a malformed JSON file | The function returns the wrap error; only the explicit `os.IsNotExist` branch in `ReadBakeStamp` falls through to the warning path. |
| Bake driver fetch failure leaves the asset un-stamped | The next bake re-stamps, and combine still works (with a warning + a fresh time.Now() id). The demo flow is not blocked. |
| Devtools volumetric rebake mints a new id without clearing the stamp | Intentional. Devtools paths do not call `bake-complete`, so the stamp from the last full bake survives. The trade-off favors stability over precision; documented in design.md. |

## Out of scope

- Any change to combine.go or PackMeta validation
- Backfill scripts for pre-existing intermediates (the fallback
  path is the documented escape hatch)
- A devtools-side button for stamping bake-complete manually
- Hash-based bake ids (E-002 follow-up)
