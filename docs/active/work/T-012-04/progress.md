# T-012-04 Progress

## Status: implemented, all tests green

### Step 1 — `upload_manifest.go` skeleton + scan helper ✅

Created `upload_manifest.go` with:
- `UploadManifestEntry` struct (ticket-spec fields)
- `ErrManifestNotFound`
- `AppendUploadRecord(path, entry)` — `O_APPEND|O_CREATE|O_WRONLY`,
  mode 0644, marshal+newline+Sync.
- `LookupUploadFilename(path, hash)` — mtime-keyed cache singleton.
- `scanManifest(path)` — bufio scan, last-write-wins, skips malformed
  lines. 1MB scanner buffer matches species_resolver.go's existing
  pattern.

### Step 2 — Resolver schema rename ✅

`species_resolver.go::uploadManifestEntry` renamed:
`ID → Hash`, `Filename → OriginalFilename`. Reader logic and doc
comments updated. Two existing resolver tests
(`TestResolver_UploadManifestTier`,
`TestResolver_UploadManifestLastWins`) updated to the new keys.

Resolver tier order is unchanged (Tier 5 still `SourceUploadManifest`).
The decision in design.md (D6) holds: T-012-01's tier already exists,
so this ticket's "Resolver integration" AC was satisfied by the schema
rename plus the writer wiring in step 3.

### Step 3 — `handleUpload` writer ✅

`handleUpload` signature gained `uploadsManifestPath string` after
`settingsDir`. Inside the per-file loop, after `store.Add(record)`
and before `autoClassify`, it appends an `UploadManifestEntry` and
logs to `os.Stderr` on error (matching the existing
`fmt.Fprintf(os.Stderr, ...)` style used by autoClassify and the
analytics events). The append is gated on a non-empty manifest path
so call sites that don't care (none today, but defensive) skip it.

`main.go` derives `uploadsManifestPath := filepath.Join(workDir,
"uploads.jsonl")` next to the other workDir paths and threads it
into the `handleUpload` registration.

### Step 4 — `upload_manifest_test.go` ✅

Six unit tests, all green:
- `TestAppendUploadRecord_RoundTrip` — also asserts the on-disk JSON
  has all four fields (`hash`, `original_filename`, `uploaded_at`,
  `size`).
- `TestAppendUploadRecord_LastWriteWins` — uses `bumpMtime` helper
  (advances mtime by 2s) to defeat 1-second filesystem mtime
  granularity.
- `TestLookupUploadFilename_NotFoundFile` — missing file returns
  `ErrManifestNotFound`.
- `TestLookupUploadFilename_NotFoundHash` — present file, missing
  hash returns `ErrManifestNotFound`.
- `TestLookupUploadFilename_CacheInvalidatedByMtime` — covers the
  cache reload contract.
- `TestScanManifest_SkipsMalformedLines` — covers the partial-write
  recovery contract (malformed first line + valid line + truncated
  final line).
- `TestAppendUploadRecord_RestartScenario` — wipes the cache via
  `resetManifestCache()` to simulate a restart.

A `resetManifestCache()` helper at the top of the file is invoked by
every test that touches `LookupUploadFilename` so cases don't bleed
across runs through the package-level cache singleton.

**Deviation from plan:** the original AC test list called for an
"append with a write error logs warning but doesn't propagate" case
inside the manifest tests. The same coverage is now provided by
`TestHandleUpload_ManifestWriteFailureIsNonFatal` in the integration
file (the failing append happens through the real handler), which is
a strictly stronger test. Dropping the lower-level case avoided
mocking the os.OpenFile failure surface.

### Step 5 — `handlers_upload_test.go` ✅

Three integration tests:
- `TestHandleUpload_AppendsManifest` — multipart upload of one fake
  GLB, asserts both the response record and the on-disk manifest.
- `TestHandleUpload_ManifestWriteFailureIsNonFatal` — points the
  manifest path at a directory so OpenFile fails, asserts the upload
  still returns 200 and the file still lands in originalsDir.
- `TestHandleUpload_RestartScenario` — uploads, calls
  `resetManifestCache()`, looks up the hash from a fresh cache and
  recovers the original filename. This is the "restart recovery"
  integration AC.

A `buildUploadBody` helper constructs a `multipart.Writer` request
body matching the `files` form field handleUpload reads.

The 4-byte fake-GLB payload causes `autoClassify` to error, which is
intentionally swallowed by the existing handler (its contract:
"classifier failure is logged and swallowed; the upload still
succeeds"). Tests pass `logger=nil` and rely on that behaviour.

### Step 6 — `scripts/backfill-uploads-manifest.go` ✅

Self-contained `package main` under `//go:build ignore` so
`go build ./...` does not pick it up. Inlines its own `entry` type,
`loadSeenHashes`, and `appendOne` helpers (cannot import from the
main package because of the build tag).

Verified manually:
```
$ go run scripts/backfill-uploads-manifest.go -dir /tmp/t12_04_backfill
backfill: scanned=2 skipped=0 appended=2 manifest=/tmp/t12_04_backfill/uploads.jsonl
$ go run scripts/backfill-uploads-manifest.go -dir /tmp/t12_04_backfill
backfill: scanned=2 skipped=2 appended=0 manifest=/tmp/t12_04_backfill/uploads.jsonl
```
Idempotency confirmed: line count stays at 2 across two runs.

### Step 7 — Full test suite ✅

```
$ go vet ./...   # clean
$ go test -count=1 ./...
ok  glb-optimizer  2.990s
```

## Files touched

| Action | Path |
|---|---|
| Created | `upload_manifest.go` |
| Created | `upload_manifest_test.go` |
| Created | `handlers_upload_test.go` |
| Created | `scripts/backfill-uploads-manifest.go` |
| Modified | `species_resolver.go` (struct rename + comments) |
| Modified | `species_resolver_test.go` (fixture key rename) |
| Modified | `handlers.go` (handleUpload signature + manifest write) |
| Modified | `main.go` (derive + thread `uploadsManifestPath`) |
| Created | `docs/active/work/T-012-04/{research,design,structure,plan,progress,review}.md` |

## Open follow-ups (deferred per ticket scope)

- Garbage collection of old manifest entries (ticket out-of-scope; will
  bite at ~100k uploads ≈ 15 MB).
- CLI subcommand to query the manifest (out of scope; resolver is the
  only consumer in v1).
- Schema versioning (`format_version` field) — not added; ticket says
  "keep it simple."
- Backfill script note for S-012's story doc — needs the operator step
  documented there per the ticket. Not done in this commit; flagging
  as a follow-up because it lives in a different ticket's notes.
