# T-010-03 Structure — Pack Button & Endpoint

## Files Modified

### `handlers.go`

Adds a single new top-level handler constructor at the end of the
file (just after `handleAccept`):

```go
// handleBuildPack handles POST /api/pack/:id. Reads the three
// intermediates from outputsDir for the asset, calls
// BuildPackMetaFromBake, runs CombinePack, and writes the result
// to distDir/{species}.glb.
//
// Status codes:
//   200 — success, JSON body { pack_path, size, species }
//   400 — required intermediate missing or PackMeta build failed
//   404 — no FileRecord for id
//   413 — combine returned the 5 MiB cap error
//   500 — any other read / combine / write failure
func handleBuildPack(
    store *FileStore,
    originalsDir, settingsDir, outputsDir, distDir string,
) http.HandlerFunc
```

No other handler in `handlers.go` is touched. No imports change
(`os`, `filepath`, `strings`, `encoding/json`, `net/http` are all
already imported).

### `main.go`

Three additions:

1. New variable in the working-dir block:
   ```go
   distPlantsDir := filepath.Join(workDir, "dist", "plants")
   ```
2. Append `distPlantsDir` to the `for _, d := range []string{...}`
   mkdir loop so it is created at startup (`0755`).
3. New mux registration alongside the other `/api/...` lines:
   ```go
   mux.HandleFunc("/api/pack/", handleBuildPack(
       store, originalsDir, settingsDir, outputsDir, distPlantsDir))
   ```

### `static/index.html`

Single new `<button>` inserted into the `.advanced-panel` div,
immediately after `generateProductionBtn`:

```html
<button class="toolbar-btn" id="buildPackBtn" disabled
        title="Combine the three intermediates into a single dist/plants/{species}.glb pack">
    Build Asset Pack
</button>
```

### `static/app.js`

Three additions, all small:

1. Element handle near the existing `generateProductionBtn`
   declaration (~line 70):
   ```js
   const buildPackBtn = document.getElementById('buildPackBtn');
   ```
2. New `buildAssetPack(id)` async function placed immediately after
   `generateProductionAsset` (around line 2473). ~50 lines including
   the analytics emit and `prepareError` writes.
3. Click listener and enable-state branch:
   - Add `buildPackBtn.addEventListener('click', () => { if (selectedFileId) buildAssetPack(selectedFileId); });`
     next to the other generate-button listeners (~line 4838).
   - Add one line inside `updatePreviewButtons`:
     ```js
     buildPackBtn.disabled = !(file && file.has_billboard
                                && (file.has_billboard_tilted || file.has_volumetric));
     ```

## Files Created

### `handlers_pack_test.go` (new, ~180 lines)

Companion test file mirroring `handlers_billboard_test.go`. Builds
synthetic minimal GLBs in-memory using the same `writeGLB` helper
the production code uses, runs them through `handleBuildPack`, and
asserts on:

- Happy path: 200, file lands in `distDir`, JSON body shape
  contains `pack_path`, `size`, `species`.
- Missing side intermediate ⇒ 400.
- Unknown id ⇒ 404.
- Tilted-only optional path: omit volumetric, expect success.
- Volumetric-only optional path: omit tilted, expect success.
- 5 MiB cap exceeded ⇒ 413. We trigger this by stuffing a large
  embedded image into one intermediate via `writeGLB` so the
  combine output crosses the cap.

The 413 test is the load-bearing one — it is the only assertion
that exercises the error-string match in the handler.

### `docs/active/work/T-010-03/{research,design,structure,plan,progress,review}.md`

The six RDSPI artifacts. (`research.md`, `design.md`, this file,
and the three remaining are produced in the same session.)

## Files Deleted

None.

## Public Surface Added

- `handleBuildPack` — Go-level constructor returning an
  `http.HandlerFunc`. Same shape as every other handler in
  `handlers.go`; not exported because nothing in the codebase
  exports handlers.
- `POST /api/pack/:id` — HTTP endpoint.
- `buildAssetPack(id)` — JS function (module-private, like
  `generateProductionAsset`).
- `buildPackBtn` — DOM element id.
- A new `pack_built` analytics event type carried by the existing
  `Event` envelope. Payload schema:
  `{ species: string, size: int, has_tilted: bool, has_dome: bool }`.
  No new event-validation rules are needed because `analytics.go`'s
  `Event.Validate` only checks the envelope (timestamp,
  schema_version, session_id, event_type, asset_id) — payload
  contents are opaque.

## Touch Surface Summary

| File                     | Lines added | Lines removed |
|--------------------------|-------------|---------------|
| handlers.go              | ~80         | 0             |
| main.go                  | ~3          | 0             |
| static/index.html        | 1           | 0             |
| static/app.js            | ~60         | 0             |
| handlers_pack_test.go    | ~180 (new)  | 0             |

No file required by another in-flight T-010-* or T-011-* ticket is
touched in a way that would conflict (T-010-02 creates `combine.go`
which we only call; T-011-02 creates `pack_meta_capture.go` which
we only call).
