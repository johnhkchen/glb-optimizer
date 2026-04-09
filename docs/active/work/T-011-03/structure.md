# T-011-03 — Structure

## Files touched

| File | Op | Purpose |
|------|----|---------|
| `bake_stamp.go` | **create** | `bakeStamp` type, `WriteBakeStamp`, `ReadBakeStamp` |
| `bake_stamp_test.go` | **create** | unit tests for write/read/round-trip |
| `pack_meta_capture.go` | **modify** | swap `time.Now()` for `ReadBakeStamp` + fallback log |
| `pack_meta_capture_test.go` | **modify** | new stability test, fallback test, fix happy-path |
| `handlers.go` | **modify** | add `handleBakeComplete` |
| `main.go` | **modify** | register `/api/bake-complete/` route |
| `static/app.js` | **modify** | call new endpoint at end of `generateProductionAsset` |

No edits to `pack_meta.go`, `combine.go`, `models.go`, `settings.go`,
or any test outside `pack_meta_capture_test.go` and `bake_stamp_test.go`.

## `bake_stamp.go` — public surface

```go
package main

// bake_stamp.go owns the on-disk {id}_bake.json file written when
// "Build hybrid impostor" finishes. The file is the single source of
// truth for PackMeta.BakeID — the stable handle the future asset
// server will use as a cache-busting URL component. See T-011-03.

import (
    "encoding/json"
    "fmt"
    "os"
    "path/filepath"
    "time"
)

// bakeStamp is the on-disk shape of {outputsDir}/{id}_bake.json.
// Both fields hold an RFC3339 UTC timestamp captured at bake
// completion. They are separate keys because "the id" and
// "when the bake finished" are conceptually distinct, even if they
// coincide in v1.
type bakeStamp struct {
    BakeID      string `json:"bake_id"`
    CompletedAt string `json:"completed_at"`
}

// bakeStampPath returns {outputsDir}/{id}_bake.json — the canonical
// file location used by both the writer and the reader.
func bakeStampPath(outputsDir, id string) string

// WriteBakeStamp creates/overwrites {outputsDir}/{id}_bake.json with
// a fresh RFC3339 UTC timestamp in BOTH fields, captured exactly
// once. Returns the bake_id it wrote so callers can echo it. Uses
// atomic temp+rename so concurrent readers never see a partial file.
func WriteBakeStamp(outputsDir, id string) (string, error)

// ReadBakeStamp reads the bake stamp for an asset. Missing file →
// (zero value, nil) so callers can fall back. Malformed JSON →
// wrapped error.
func ReadBakeStamp(outputsDir, id string) (bakeStamp, error)
```

Helper conventions follow `pack_meta_capture.go`'s "fail loudly" tone
and the `pack_meta:` / `pack_meta_capture:` error-prefix discipline.
This file uses `bake_stamp:` as its prefix.

## `pack_meta_capture.go` — diff shape

Inside `BuildPackMetaFromBake`, replace this:

```go
meta := PackMeta{
    FormatVersion: PackFormatVersion,
    BakeID:        time.Now().UTC().Format(time.RFC3339),
    ...
}
```

with:

```go
bakeID, err := resolveBakeID(id, outputsDir)
if err != nil {
    return PackMeta{}, fmt.Errorf("pack_meta_capture: bake_id: %w", err)
}
meta := PackMeta{
    FormatVersion: PackFormatVersion,
    BakeID:        bakeID,
    ...
}
```

`resolveBakeID` is a new unexported helper, also in
`pack_meta_capture.go`:

```go
// resolveBakeID returns a stable bake_id for the asset. It first
// looks for {outputsDir}/{id}_bake.json (written by WriteBakeStamp
// when the bake driver completes). If absent or empty, it logs a
// one-line warning and falls back to a fresh time.Now() UTC stamp.
// A malformed file is propagated as an error — silently masking it
// would let combine ship a pack with a bogus id.
func resolveBakeID(id, outputsDir string) (string, error) {
    stamp, err := ReadBakeStamp(outputsDir, id)
    if err != nil {
        return "", err
    }
    if stamp.BakeID != "" {
        return stamp.BakeID, nil
    }
    log.Printf("pack_meta_capture: %s: no bake stamp at %s, using current time as bake_id; "+
        "rebake to get a stable id", id, bakeStampPath(outputsDir, id))
    return time.Now().UTC().Format(time.RFC3339), nil
}
```

`pack_meta_capture.go` already imports `time` and `os`. The diff
adds `log` to the import block. The existing `time.Now()` line in
`BuildPackMetaFromBake` is removed.

## `handlers.go` — diff shape

Append after `handleUploadVolumetric` (around line 550):

```go
// handleBakeComplete handles POST /api/bake-complete/:id. The bake
// driver (static/app.js generateProductionAsset) calls this once
// after all three impostor uploads succeed. The handler stamps
// {outputsDir}/{id}_bake.json with a fresh RFC3339 UTC timestamp;
// pack_meta_capture later reads that stamp so combine runs of the
// same intermediates produce identical bake_ids. See T-011-03.
func handleBakeComplete(store *FileStore, outputsDir string) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodPost {
            jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
            return
        }
        id := strings.TrimPrefix(r.URL.Path, "/api/bake-complete/")
        if _, ok := store.Get(id); !ok {
            jsonError(w, http.StatusNotFound, "file not found")
            return
        }
        bakeID, err := WriteBakeStamp(outputsDir, id)
        if err != nil {
            jsonError(w, http.StatusInternalServerError, "failed to write bake stamp")
            return
        }
        jsonResponse(w, http.StatusOK, map[string]interface{}{
            "status":  "ok",
            "bake_id": bakeID,
        })
    }
}
```

## `main.go` — diff shape

Add one route registration next to the existing upload handlers
(around line 124):

```go
mux.HandleFunc("/api/bake-complete/", handleBakeComplete(store, outputsDir))
```

## `static/app.js` — diff shape

In `generateProductionAsset`, after the third upload (volumetric)
succeeds and before `await refreshFiles()`, add:

```js
// T-011-03: stamp {id}_bake.json so the next combine sees a stable
// bake_id that survives rebuild rounds of the same intermediates.
await fetch(`/api/bake-complete/${id}`, { method: 'POST' });
```

The fetch is fire-and-forget at the call site (no error swallowing
beyond what the surrounding `try/catch` already does). If it fails,
the existing catch handler logs `'Production asset generation
failed'` and `success` stays false. That is the right behavior:
without a bake stamp the demo would still ship, but the operator
wants to know.

## `bake_stamp_test.go` — layout

| # | Name | What it locks down |
|---|------|--------------------|
| 1 | `TestWriteBakeStamp_RoundTrip` | write then read returns same `bake_id` and `completed_at` |
| 2 | `TestWriteBakeStamp_Format` | both fields parse as RFC3339 UTC, equal to each other |
| 3 | `TestReadBakeStamp_Missing` | absent file → zero value + nil error |
| 4 | `TestReadBakeStamp_Malformed` | non-JSON file → error containing path + "decode" |
| 5 | `TestWriteBakeStamp_Overwrite` | second write replaces first; reader sees new id |

All tests use `t.TempDir()` for the outputs dir; no fixtures needed.

## `pack_meta_capture_test.go` — diff shape

Three changes:

1. **Fix the existing `TestBuildPackMetaFromBake_HappyPath`.** It
   currently asserts `meta.BakeID != ""`. After this ticket the
   happy path takes the fallback branch (no `{id}_bake.json` is
   staged), so the assertion still holds — no change needed there,
   but suppress the fallback log via `log.SetOutput(io.Discard)`
   inside a small helper to keep test output clean.

2. **Add `TestBuildPackMetaFromBake_StableBakeID`.** Stages a
   `{id}_bake.json` with a fixed id, calls `BuildPackMetaFromBake`
   twice, asserts both returned `BakeID`s equal the staged value.
   This is the AC unit test.

3. **Add `TestBuildPackMetaFromBake_MissingStampLogsWarning`.**
   Captures `log` output via `log.SetOutput(buf)`, runs capture,
   asserts the buffer contains `"no bake stamp"` and `id` and the
   stamp path. Restores the previous log destination in `t.Cleanup`.

A small test helper `silenceLog(t)` is added at the bottom of
`pack_meta_capture_test.go` to avoid duplicating the cleanup code.

## Naming and conventions

- Package `main`, single file per concern (matches the
  `pack_meta.go` / `pack_meta_capture.go` precedent recorded in
  memory ID 244).
- Error prefix `bake_stamp:` for `bake_stamp.go`,
  `pack_meta_capture:` for resolver errors. Matches existing
  discipline.
- Atomic write: marshal → `os.WriteFile(tmp, ...)` → `os.Rename(tmp,
  final)`. The settings package uses the same pattern.
- Stdlib only: `encoding/json`, `fmt`, `os`, `path/filepath`,
  `time`, plus `log` for the warning. No new module deps.
- The HTTP route is registered with a trailing slash to match the
  existing `/api/upload-billboard/`, `/api/upload-volumetric/`,
  etc. patterns in `main.go:121-124`.
