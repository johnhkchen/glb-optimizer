# T-010-03 Plan — Pack Button & Endpoint

## Step Sequence

Each step is independently verifiable. Steps 1-3 are pure backend
and can be exercised by `go test ./...`; steps 4-5 are frontend and
verified manually + by `go build`.

### Step 1 — Add `handleBuildPack` to `handlers.go`

- Append the new constructor at the very end of `handlers.go`,
  immediately after `handleAccept`.
- Implementation order inside the closure:
  1. Method check ⇒ 405.
  2. Parse `id := strings.TrimPrefix(r.URL.Path, "/api/pack/")`,
     reject empty ⇒ 404.
  3. `store.Get(id)` ⇒ 404 if absent.
  4. Read `{id}_billboard.glb` ⇒ 400 on `os.IsNotExist`, 500 otherwise.
  5. Read `{id}_billboard_tilted.glb` and `{id}_volumetric.glb`
     with `os.IsNotExist` mapped to `nil` byte slices.
  6. `BuildPackMetaFromBake(...)` ⇒ 400 on error.
  7. `CombinePack(side, tilted, volumetric, meta)` ⇒
     - if `strings.Contains(err.Error(), "exceeds 5 MiB cap")` ⇒ 413
     - else ⇒ 500.
  8. `os.WriteFile(filepath.Join(distDir, meta.Species+".glb"), ...)` ⇒ 500 on error.
  9. `jsonResponse(w, 200, { "pack_path": ..., "size": ..., "species": ... })`.
- **Verification:** `go build ./...` succeeds.

### Step 2 — Wire the route in `main.go`

- Add `distPlantsDir := filepath.Join(workDir, "dist", "plants")`
  next to the other dir vars.
- Append `distPlantsDir` to the mkdir slice.
- Register `mux.HandleFunc("/api/pack/", handleBuildPack(...))`
  alongside the other handlers.
- **Verification:** `go build ./...` succeeds and a manual `curl
  -X POST localhost:8787/api/pack/<unknown>` returns a 404.

### Step 3 — Add `handlers_pack_test.go`

Tests, in this order:

- `TestHandleBuildPack_HappyPath_AllThree` — registers an asset
  with all three intermediates on disk plus a fake original GLB
  (so `BuildPackMetaFromBake` can read its bbox), `POST`s, asserts
  200, JSON body fields, file existence in `distDir`.
- `TestHandleBuildPack_TiltedOnly` — omit volumetric, expect 200.
- `TestHandleBuildPack_VolumetricOnly` — omit tilted, expect 200.
- `TestHandleBuildPack_MissingSide` — only tilted+volumetric on
  disk, expect 400 with the
  "missing intermediate" message.
- `TestHandleBuildPack_UnknownID` — empty store, expect 404.
- `TestHandleBuildPack_OversizePack` — pad one intermediate's
  embedded image so the merged result crosses 5 MiB; expect 413
  with the "5 MB" message.
- `TestHandleBuildPack_MethodNotAllowed` — GET ⇒ 405.

Test helpers reuse `writeGLB` from combine.go. The `writeGLB` /
`gltfDoc` types are unexported but in the same `package main`, so
the test file can call them directly.

- **Verification:** `go test ./...` passes; specifically
  `go test -run TestHandleBuildPack ./...` is green.

### Step 4 — Frontend: button + click + enable state

- Edit `static/index.html`: insert the new `<button id="buildPackBtn">`
  inside `.advanced-panel`, after `generateProductionBtn`.
- Edit `static/app.js`:
  1. Add the `buildPackBtn` constant near the other element handles
     (after `generateProductionBtn`).
  2. Add the `buildAssetPack(id)` async function after
     `generateProductionAsset`.
  3. Add the click listener after the
     `generateProductionBtn.addEventListener` block.
  4. Add the `buildPackBtn.disabled = …` line at the bottom of
     `updatePreviewButtons`.
- **Verification:** Open the app, build a hybrid impostor, confirm
  the new button enables, click it, confirm the success line in
  `prepareError` and the file at
  `~/.glb-optimizer/dist/plants/{species}.glb`.

### Step 5 — Manual smoke test

- Start the server (`just run`), upload a small GLB, run
  "Build hybrid impostor", click "Build Asset Pack". Verify:
  - `~/.glb-optimizer/dist/plants/{species}.glb` exists and
    `file -b` reports it as binary glTF.
  - Tail the analytics JSONL: a `pack_built` event with
    `{species, size, has_tilted, has_dome}` is appended.
  - Trigger the 413 path by temporarily lowering `packSizeCap`
    in a scratch branch (or via the oversize unit test) and
    confirm the user-facing message renders inside `prepareError`.

The Step 5 oversize manual test is opportunistic — the unit test
in Step 3 is the load-bearing assertion.

## Testing Strategy

- **Unit tests:** `handlers_pack_test.go` covers every status-code
  branch (200, 400, 404, 405, 413) and the two optional-input
  permutations.
- **Integration tests:** none beyond the unit tests. The handler
  is a thin glue layer over `BuildPackMetaFromBake` and
  `CombinePack`, both of which already have full test suites in
  T-011-02 and T-010-02 respectively.
- **Frontend tests:** none. The repo has no JS test runner; we
  rely on manual smoke tests, the same as every other UI change in
  this codebase.

## Verification Criteria

- `go vet ./... && go test ./... && go build ./...` all green.
- `handlers_pack_test.go` covers each AC status code at least once.
- A successful end-to-end click writes a parseable GLB to
  `dist/plants/{species}.glb` and emits a `pack_built` analytics
  event.
- 413 path renders the canonical "Pack exceeds 5 MB" message.

## Step Boundaries / Commit Points

- Commit 1: Steps 1-3 (handler + route + tests). Atomic — leaves
  the tree compilable and tested.
- Commit 2: Steps 4-5 (frontend). Atomic — UI wiring only, no
  backend changes.

This split mirrors the T-010-02 / T-011-02 commits and lets each
half be reviewed independently.
