# T-010-03 Progress — Pack Button & Endpoint

## Status

Implementation complete. All five plan steps done in a single session,
no deviations from the plan.

## Step Log

### Step 1 — `handleBuildPack` in `handlers.go` ✓

Appended a single new handler at the bottom of `handlers.go` (after
`handleAccept`). Implementation matches the plan exactly:

- Method check (405 on non-POST)
- Path parse + 404 on empty / unknown id
- Required side read (400 on `os.IsNotExist`, 500 otherwise)
- Optional tilted + volumetric reads via local `readOptional` closure
- `BuildPackMetaFromBake(...)` (400 on error)
- `CombinePack(...)` with size-cap string match → 413, else 500
- `os.WriteFile` to `distDir/{species}.glb`
- 200 JSON response `{ pack_path, size, species }`

No new imports were needed — `os`, `filepath`, `strings`,
`net/http`, and the JSON helpers are all already imported. The full
build (`go build ./...`) was clean immediately after this step.

### Step 2 — Route + dist dir in `main.go` ✓

Three additions, all within five lines of existing code:

1. `distPlantsDir := filepath.Join(workDir, "dist", "plants")` next
   to the other dir vars.
2. Appended `distPlantsDir` to the startup mkdir slice so the dir
   exists before any handler runs.
3. `mux.HandleFunc("/api/pack/", handleBuildPack(store, originalsDir, settingsDir, outputsDir, distPlantsDir))`
   registered alongside the other `/api/...` routes (placed right
   after the new `bake-complete` route from T-011-02).

### Step 3 — `handlers_pack_test.go` ✓

New test file, ~245 lines, mirroring the structure of
`handlers_billboard_test.go`. A `packTestEnv` helper bundles
fixture dirs and the file store so each test can stand up a
hermetic tree in two lines. Synthetic GLBs are generated with
`makeMinimalGLB` from `combine_test.go` (same package, so the
helper is reachable directly).

Tests added (all green):

| Test                                          | Asserts        |
|-----------------------------------------------|----------------|
| `TestHandleBuildPack_HappyPath_AllThree`      | 200, body shape, on-disk file matches `size`, species derived correctly |
| `TestHandleBuildPack_TiltedOnly`              | 200 with no volumetric on disk                                          |
| `TestHandleBuildPack_VolumetricOnly`          | 200 with no tilted on disk                                              |
| `TestHandleBuildPack_MissingSide`             | 400 + "missing intermediate" message                                    |
| `TestHandleBuildPack_UnknownID`               | 404 (empty store)                                                       |
| `TestHandleBuildPack_MethodNotAllowed`        | 405 on GET                                                              |
| `TestHandleBuildPack_OversizePack`            | 413 + "5 MB" message                                                    |

The 413 test reuses the 6 MiB-ballast pattern from
`TestCombine_SizeCapRejection` (combine_test.go:391). The helper
function `makeOversizeGLB` is local to `handlers_pack_test.go` so
it does not pollute the shared `combine_test.go` symbol space.

`go test -run TestHandleBuildPack ./...` → all 7 tests pass in
0.27 s. Full `go test ./...` is also green.

### Step 4 — Frontend wiring ✓

`static/index.html`:

- Added one `<button id="buildPackBtn">` immediately after
  `generateProductionBtn` inside `.advanced-panel`. Same
  `toolbar-btn` class — no CSS needed.

`static/app.js`:

- Added `const buildPackBtn = document.getElementById('buildPackBtn');`
  next to the other element handles.
- Added `buildAssetPack(id)` async function (~70 lines) modeled on
  the existing `generateBillboard` shape but tailored to the
  HTTP-only flow and the three-status-code response space. It:
  - disables the button and sets text to "Packing…"
  - clears `prepareError`
  - decodes the JSON body once, then branches on `res.ok` /
    `res.status === 413` / generic non-2xx
  - writes a one-line outcome into `prepareError` for each branch
  - always emits `pack_built` with `{species, size, has_tilted, has_dome}`
  - re-enables the button in `finally`
- Added the click listener next to the other generate-* listeners.
- Added the enable-state line inside `updatePreviewButtons` —
  matches the AC verbatim:
  `has_billboard && (has_billboard_tilted || has_volumetric)`.

### Step 5 — Smoke test (deferred to manual)

The unit-test 413 path is the load-bearing assertion. End-to-end
smoke (running the binary, uploading, clicking) is deferred to a
manual checklist; nothing in the implementation requires the JS
side to run during this work session.

## Verification

```
$ go vet ./...
(clean)

$ go test ./...
ok  	glb-optimizer	2.782s

$ go test -run TestHandleBuildPack ./...
ok  	glb-optimizer	0.269s

$ go build ./...
(clean)
```

## Deviations from Plan

None.

## Files Touched

| File                            | Net change       |
|---------------------------------|------------------|
| `handlers.go`                   | +106 lines       |
| `main.go`                       | +4 lines         |
| `static/index.html`             | +1 line          |
| `static/app.js`                 | +85 lines        |
| `handlers_pack_test.go` (new)   | +245 lines       |
| `docs/active/work/T-010-03/*`   | 6 RDSPI artifacts|

No file owned by an in-flight sibling ticket was modified — the
T-010-02 (`combine.go`) and T-011-02 (`pack_meta_capture.go`,
`handleBakeComplete`) surfaces are only consumed.

## Pending Commit

The work is staged in the working tree but not yet committed —
deferred to whoever runs the next `/commit` slash command, per the
project's "only commit when explicitly asked" rule.
