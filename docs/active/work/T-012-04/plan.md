# T-012-04 Plan

## Step 1 — `upload_manifest.go` skeleton + scan helper

Write `upload_manifest.go` with:
- imports: `encoding/json`, `errors`, `os`, `bufio`, `sync`, `time`
- `UploadManifestEntry` struct with the four ticket-spec fields
- `ErrManifestNotFound` sentinel
- `scanManifest(path)` private helper that returns
  `(map[string]string, error)`. Uses bufio.Scanner with the same 1MB
  buffer pattern as species_resolver.go's lookupUploadManifest.
- `AppendUploadRecord(path, entry)` — opens with
  `O_APPEND|O_CREATE|O_WRONLY`, mode 0644, marshals one line, writes
  with trailing newline, calls Sync, closes. Returns the first error.
- `LookupUploadFilename(path, hash)` — uses the package-level
  `manifestCache` with mtime invalidation. Returns
  `ErrManifestNotFound` for both missing-file and missing-hash cases.

**Verify:** `go build ./...` (compiles in isolation).

## Step 2 — Switch resolver schema to `hash`/`original_filename`

In `species_resolver.go`:
- Rename struct fields on `uploadManifestEntry`.
- Update `lookupUploadManifest` to read the new fields.
- Update the doc comments.

In `species_resolver_test.go`:
- Update the JSONL fixture bodies in `TestResolver_UploadManifestTier`
  and `TestResolver_UploadManifestLastWins` to the new keys.

**Verify:** `go test ./... -run Resolver` green.

## Step 3 — Wire `handleUpload` to the manifest writer

In `handlers.go`:
- Add `uploadsManifestPath string` parameter to `handleUpload`.
- In the per-file loop, after `store.Add(record)` and before
  `autoClassify`, call `AppendUploadRecord(...)`. Log a WARN on error;
  do not fail the upload.

In `main.go`:
- Compute `uploadsManifestPath := filepath.Join(workDir,
  "uploads.jsonl")` near the other dir derivations.
- Pass it to `handleUpload(...)` at the registration site.

**Verify:** `go build ./...` (full binary compiles).

## Step 4 — Unit tests for `upload_manifest.go`

Write `upload_manifest_test.go` with the six cases listed in
structure.md. Each uses `t.TempDir()` for the manifest path.

Notable test plumbing:

```go
func resetCache() {
    manifestCache.Lock()
    manifestCache.path = ""
    manifestCache.mtime = time.Time{}
    manifestCache.index = nil
    manifestCache.Unlock()
}
```

Call `resetCache()` at the top of every test that uses
`LookupUploadFilename` so cases don't bleed across runs.

For the cache-invalidation test, set the file mtime forward by 1
second via `os.Chtimes` to defeat 1-second filesystem mtime
granularity.

**Verify:** `go test ./... -run UploadManifest` green.

## Step 5 — Integration test for `handleUpload`

Write `handlers_upload_test.go`. Build a multipart body with one fake
`.glb` payload (just 4 bytes — the upload pipeline doesn't validate
glb content). Call the handler with `httptest.NewRecorder` and assert:

- HTTP 200, response body has one record with `Filename:
  "achillea_millefolium.glb"`.
- The manifest file exists, has one line, decodes to a record with
  matching hash + original_filename.
- A second test points the manifest path at a directory (write fails)
  and asserts the upload still returns 200.
- A third test simulates restart by calling `resetCache()` then
  `LookupUploadFilename`, verifies the original filename is recovered.

**Caveat:** `autoClassify` opens the on-disk file with the GLB
classifier, which would fail on a 4-byte payload. The test passes
`logger=nil, settingsDir=t.TempDir()` and accepts the swallowed
classifier error per the upload handler's existing contract
("classifier failure is logged and swallowed").

**Verify:** `go test ./... -run Upload` green.

## Step 6 — Backfill script

Create `scripts/backfill-uploads-manifest.go` with `//go:build ignore`
header. It is a self-contained `package main` that:

1. Parses `-dir` flag (default `~/.glb-optimizer`).
2. Inlines a minimal `scanSeen(path) map[string]bool` (just hash set).
3. Walks `<dir>/originals` for `*.glb`.
4. For each unseen hash, opens the file to stat its size + mtime,
   appends a JSONL line directly (no shared helper — script is
   isolated).
5. Prints `scanned=N skipped=N appended=N`.

**Verify:** `go run scripts/backfill-uploads-manifest.go -dir <tmp>`
in a tempdir with a fake `originals/abc.glb`, then re-run, second run
reports `appended=0`.

## Step 7 — Full test suite + commit

`go test ./...` green. Stage and commit each step's files together
under one ticket commit (`T-012-04: persist upload filenames across
restarts`) — but only if explicitly asked; default behaviour is leave
unstaged.

## Verification matrix

| AC | Verified by |
|---|---|
| Manifest format `hash`/`original_filename`/`uploaded_at`/`size` | step 1 + step 4 round-trip test |
| Append-only, last-wins | step 4 case 2 |
| Server-side write on upload | step 5 case 1 |
| Append failure does not fail upload | step 5 case 2 |
| `LookupUploadFilename` with mtime cache | step 1 + step 4 case 5 |
| `ErrNotFound` on missing file | step 4 case 3 |
| Resolver Tier 5 reads new schema | step 2, existing resolver tests still green |
| Backfill script idempotent | step 6 manual verification |
| Restart recovery integration test | step 5 case 3 |

## Risk / rollback

- All changes are additive except the rename in
  `uploadManifestEntry`. If a downstream consumer relies on the old
  `id`/`filename` keys, the resolver test failures will catch it
  immediately. No on-disk migration is needed because no
  `uploads.jsonl` exists yet on any deployment.
- Rollback: revert the four files; nothing else depends on the new
  helpers in production.
