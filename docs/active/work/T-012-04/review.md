# T-012-04 Review

## What changed

T-012-04 closes the "filename loss across server restarts" gap that
T-010-04's review flagged. Uploaded GLBs now have their original
filename persisted to a `<workDir>/uploads.jsonl` log so the species
resolver (T-012-01) can recover provenance after a restart without
hand-authored sidecars.

### Files created

- **`upload_manifest.go`** (~170 lines) — public API:
  `UploadManifestEntry`, `ErrManifestNotFound`, `AppendUploadRecord`,
  `LookupUploadFilename`. Private: `scanManifest`,
  `loadManifestCached`, `manifestCache` singleton (mtime-keyed).
- **`upload_manifest_test.go`** — 7 unit tests covering round-trip,
  last-write-wins, missing-file, missing-hash, mtime cache
  invalidation, malformed-line skip, and a restart scenario.
- **`handlers_upload_test.go`** — 3 integration tests against
  `handleUpload` covering happy-path manifest writing, append-failure
  non-fatal behaviour, and end-to-end restart recovery.
- **`scripts/backfill-uploads-manifest.go`** — `//go:build ignore`
  one-shot migration script. Self-contained `package main`,
  idempotent, prints `scanned/skipped/appended` counters.
- **`docs/active/work/T-012-04/{research,design,structure,plan,progress,review}.md`**

### Files modified

- **`species_resolver.go`** — `uploadManifestEntry` fields renamed
  (`ID/Filename` → `Hash/OriginalFilename`) to match the ticket's
  canonical schema. Reader logic + doc comments updated. No tier
  reorder.
- **`species_resolver_test.go`** — two fixture JSONL bodies updated
  to the new key names.
- **`handlers.go`** — `handleUpload` gained `uploadsManifestPath
  string` parameter. Inside the per-file loop, after `store.Add`, it
  appends an `UploadManifestEntry`. Errors logged via the existing
  `fmt.Fprintf(os.Stderr, ...)` style and swallowed per the
  upload-must-not-fail contract.
- **`main.go`** — derives `uploadsManifestPath := filepath.Join(workDir,
  "uploads.jsonl")` and threads it into the handler.

## Schema

```json
{"hash":"<id>","original_filename":"<name>.glb","uploaded_at":"<rfc3339>","size":<bytes>}
```

The `hash` field name keeps the ticket's vocabulary even though the
underlying value is currently a random 16-byte hex from `generateID()`
rather than a content hash; the field is opaque to consumers and the
rename to a real content-addressed scheme is a future-proof option.

## Acceptance criteria — verification

| AC | Status | Verified by |
|---|---|---|
| Manifest file at `uploads.jsonl` with the four-field schema | ✅ | `TestAppendUploadRecord_RoundTrip` asserts all four field names appear in the on-disk JSON |
| Append-only, never rewritten | ✅ | Implementation uses `O_APPEND`; no rename/truncate path exists |
| Duplicates allowed, last-wins on read | ✅ | `TestAppendUploadRecord_LastWriteWins`, `TestResolver_UploadManifestLastWins` |
| Upload handler appends after successful write | ✅ | `TestHandleUpload_AppendsManifest` |
| Append failure does NOT fail upload | ✅ | `TestHandleUpload_ManifestWriteFailureIsNonFatal` |
| File created with mode 0644 if absent | ✅ | `os.OpenFile(..., 0644)` in `AppendUploadRecord` |
| `LookupUploadFilename(hash) (string, error)` | ✅ | `upload_manifest.go` |
| Most-recent entry per hash | ✅ | `scanManifest` overwrites map on each match |
| `ErrNotFound` (`ErrManifestNotFound`) for missing file | ✅ | `TestLookupUploadFilename_NotFoundFile` |
| `ErrNotFound` for missing hash | ✅ | `TestLookupUploadFilename_NotFoundHash` |
| Cached with mtime invalidation | ✅ | `TestLookupUploadFilename_CacheInvalidatedByMtime` |
| Resolver Tier 5 reads the new schema | ✅ | Existing resolver tests pass after struct rename |
| Backfill script (one-shot, idempotent) | ✅ | Manual run: `scanned=2 skipped=0 appended=2`, re-run `scanned=2 skipped=2 appended=0` |
| Restart-recovery integration test | ✅ | `TestHandleUpload_RestartScenario` |

## Test coverage

- **Unit (upload_manifest_test.go):** 7 tests, exercise the writer,
  reader, cache, and crash-safety contract.
- **Integration (handlers_upload_test.go):** 3 tests, exercise the
  full HTTP path including the failure mode and restart simulation.
- **Resolver regression (species_resolver_test.go):** existing tests
  re-validated against the new schema.
- **Full suite:** `go vet ./...` clean, `go test -count=1 ./...`
  green (~3.0s).

### Coverage gaps (acceptable)

- The 4-byte fake-GLB used in handler tests triggers an autoClassify
  failure that the handler intentionally swallows. This is documented
  in the existing handler contract; not a regression. A "real GLB
  payload" test would also exercise the classifier path, but that
  belongs in a classifier test, not an upload-manifest test.
- The backfill script has no Go test (it lives under `//go:build
  ignore` and cannot be imported). Verified manually with two runs
  against a tempdir; that is the appropriate level for a one-shot
  operator tool.
- Concurrent-append behaviour is not tested. POSIX guarantees atomic
  `O_APPEND` writes ≤ PIPE_BUF (typically 4 KB) and our records are
  ~150 bytes, so theoretical safety holds. A stress test would not
  add insight beyond the kernel guarantee.

## Open concerns

1. **Manifest path divergence with `-dir` flag.** The resolver's
   `lookupUploadManifest` still falls back to
   `~/.glb-optimizer/uploads.jsonl` when `ResolverOptions.UploadManifestPath`
   is empty. Server callers explicitly pass the path so this is fine
   in practice, but a CLI invocation that uses `-dir <other>` and
   doesn't thread the manifest path would silently miss the manifest.
   Not a bug per se — design.md acknowledges this — but worth a
   follow-up if a CLI consumer ever forgets the threading.

2. **Backfill cannot recover real names.** The backfill script writes
   `original_filename = <hash>.glb` for pre-T-012-04 intermediates
   because the upload-time name was never persisted anywhere. The
   resolver will treat these as the post-restart sentinel and fall
   through to its hash-fallback tier. Operators with old assets they
   care about should re-upload them through the UI to get a real
   record; documenting that step in S-012's story is the suggested
   follow-up listed in progress.md.

3. **No schema versioning.** Per the ticket ("keep it simple"), no
   `format_version` field. Future field additions are
   forwards-compatible because both the resolver and the cache reader
   decode a narrow subset of fields and ignore unknown keys. Removals
   would require a migration and a version bump if/when they happen.

4. **Single-slot cache.** `manifestCache` only holds one path at a
   time. If a future caller alternates between two manifest paths in
   a hot loop the cache becomes a thrash; not a current concern (one
   path per process) but an O(1) → O(n) invariant change to keep in
   mind.

## Risk assessment

Low. The change is additive (new file, new helper, one new parameter
threaded to one handler). The single rename in
`uploadManifestEntry` was caught by existing resolver tests. No
on-disk migration needed because no `uploads.jsonl` existed pre-this
ticket on any deployment. Rollback is "revert four files."

## Suggested follow-ups (not in scope)

- Document the backfill operator step in S-012's story notes.
- Add a `glb-optimizer manifest list` subcommand (out of scope per
  ticket).
- Consider switching the upload `id` to a real content hash so the
  `hash` field becomes content-addressed in fact as well as name.
  Would unlock dedup at the upload layer.
