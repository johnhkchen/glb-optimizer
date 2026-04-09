# T-012-04 Structure

## Files created

### `upload_manifest.go` (new, package main)

Public surface:

```go
// UploadManifestEntry is one record in uploads.jsonl.
type UploadManifestEntry struct {
    Hash             string    `json:"hash"`
    OriginalFilename string    `json:"original_filename"`
    UploadedAt       time.Time `json:"uploaded_at"`
    Size             int64     `json:"size"`
}

// ErrManifestNotFound — returned by LookupUploadFilename when the
// hash is not present in the manifest (or the file is missing).
var ErrManifestNotFound = errors.New("upload manifest: hash not found")

// AppendUploadRecord appends one entry to path. Creates the file
// (mode 0644) if absent. Calls Sync() before close.
func AppendUploadRecord(path string, entry UploadManifestEntry) error

// LookupUploadFilename returns the most recent original_filename
// recorded for hash. Uses an mtime-keyed in-process cache.
func LookupUploadFilename(path, hash string) (string, error)
```

Private helper:

```go
// scanManifest reads path line-by-line, returning a hash → most-recent
// original_filename map. Missing file → (nil, os.ErrNotExist). Malformed
// lines are skipped (crash-safety contract).
func scanManifest(path string) (map[string]string, error)
```

Cache singleton:

```go
var manifestCache struct {
    sync.RWMutex
    path  string
    mtime time.Time
    index map[string]string
}
```

## Files modified

### `species_resolver.go`

- Rename `uploadManifestEntry` struct fields:
  - `ID string \`json:"id"\`` → `Hash string \`json:"hash"\``
  - `Filename string \`json:"filename"\`` → `OriginalFilename string \`json:"original_filename"\``
- Update the comparison in `lookupUploadManifest` to use `entry.Hash`
  and `entry.OriginalFilename`.
- Update file/struct comments to reference T-012-04's schema.
- No tier reorder, no new public API.

### `species_resolver_test.go`

- Update fixture JSONL bodies in `TestResolver_UploadManifestTier` and
  `TestResolver_UploadManifestLastWins` to use the new key names.

### `handlers.go`

- `handleUpload` gains a new positional parameter
  `uploadsManifestPath string` after `originalsDir`. Inside the loop,
  after `store.Add(record)` succeeds, call:

  ```go
  err := AppendUploadRecord(uploadsManifestPath, UploadManifestEntry{
      Hash:             id,
      OriginalFilename: fh.Filename,
      UploadedAt:       time.Now().UTC(),
      Size:             written,
  })
  if err != nil {
      log.Printf("upload manifest: append failed for %s: %v", id, err)
  }
  ```

- `time` is already imported via the `path` group at the top of the
  file; `log` is already imported elsewhere in handlers.go (verified
  during research). If not, add it.

### `main.go`

- Add `uploadsManifestPath := filepath.Join(workDir, "uploads.jsonl")`
  after the workDir setup block (~line 82).
- Pass `uploadsManifestPath` to `handleUpload(...)` at the existing
  registration site (~line 123).

## Files created (test)

### `upload_manifest_test.go`

Cases (one Test func each):

1. `TestAppendUploadRecord_RoundTrip` — write one record, read with
   scanManifest, assert all four fields decode equal.
2. `TestAppendUploadRecord_LastWriteWins` — write two records with the
   same hash, lookup returns the second's filename.
3. `TestLookupUploadFilename_NotFoundFile` — call against a nonexistent
   path, returns `ErrManifestNotFound` (not a parse error).
4. `TestLookupUploadFilename_NotFoundHash` — append one record, look up
   a different hash, returns `ErrManifestNotFound`.
5. `TestLookupUploadFilename_CacheInvalidatedByMtime` — append, look
   up (cache populates), append a second record, advance mtime via
   `os.Chtimes`, look up second hash → succeeds without restart.
6. `TestAppendUploadRecord_AppendOnlyDurability` — write a malformed
   line directly, then append a valid record, scan: malformed line is
   skipped, valid line is returned. Proves the partial-write recovery
   contract.

### `handlers_upload_test.go` (new)

7. `TestHandleUpload_AppendsManifest` — build a multipart body with one
   `.glb` file, dispatch through `handleUpload`, then read the manifest
   from the tempdir and assert the hash + original filename round-trip.
8. `TestHandleUpload_ManifestWriteFailureDoesNotFailUpload` — point
   `uploadsManifestPath` at an unwritable path (a directory), upload
   succeeds with HTTP 200, response body still includes the record.
9. `TestHandleUpload_RestartScenario` — write a manifest via append,
   simulate restart by clearing the in-memory cache (set `manifestCache.path
   = ""`), call `LookupUploadFilename` with the same path, recovers
   the original filename.

### `scripts/backfill-uploads-manifest.go`

Standalone `package main` with `//go:build ignore` (modern syntax) so
`go build ./...` does not pick it up.

```go
//go:build ignore

package main

func main() {
    var dir string
    flag.StringVar(&dir, "dir", "", "workDir (default ~/.glb-optimizer)")
    flag.Parse()
    // resolve dir
    // path := dir + "/uploads.jsonl"
    // existing := scanManifest(path) (or empty)
    // walk dir/originals/*.glb, append missing
    // print scanned/skipped/appended
}
```

Because the script lives in `package main` with build-ignore, it
**cannot** import `scanManifest` from the main binary. The script
will inline a minimal scanner (just enough to populate the `seen`
set). This is the cost of avoiding a separate Go package; acceptable
because the script is one-shot and ~80 lines total.

## Public/private interface table

| Symbol | Visibility | File |
|---|---|---|
| `UploadManifestEntry` | exported | upload_manifest.go |
| `ErrManifestNotFound` | exported | upload_manifest.go |
| `AppendUploadRecord` | exported | upload_manifest.go |
| `LookupUploadFilename` | exported | upload_manifest.go |
| `scanManifest` | unexported | upload_manifest.go |
| `manifestCache` | package-private | upload_manifest.go |
| `lookupUploadManifest` | unchanged (resolver-internal) | species_resolver.go |
| `uploadManifestEntry` | unchanged name, renamed fields | species_resolver.go |

## Ordering

The implementation order matters for compile-cleanness:

1. Add `upload_manifest.go` first (no dependencies).
2. Update `species_resolver.go` struct fields + the resolver test
   fixture bodies (these compile together).
3. Update `handleUpload` signature + main.go call site.
4. Add new test files.
5. `go test ./...` green.
6. Add the backfill script last (build-ignored, never breaks build).
