# T-012-04 Research

## Goal recap

Persist a hash → original-filename mapping across server restarts so the
species resolver (T-012-01) has provenance to fall back on without
hand-authored sidecars.

## Current upload pipeline

`handleUpload` (handlers.go:45) is the only HTTP entry point that writes
into `originalsDir`. Per request:

1. `r.ParseMultipartForm(100 << 20)` — 100MB cap.
2. For each `*.glb` part:
   - `id := generateID()` — random 16-byte hex (handlers.go:24). The
     ticket calls this a "content hash" but it is actually a random ID;
     the field semantics still match (opaque, unique, lowercase hex).
   - File is written to `filepath.Join(originalsDir, id+".glb")`.
   - A `FileRecord{ID, Filename: fh.Filename, OriginalSize, Status}` is
     pushed into the in-memory `FileStore` (models.go:67).
   - `autoClassify(...)` is called best-effort.
3. JSON response returns the records.

`fh.Filename` (the multipart `filename=` parameter, the only place the
human-readable name lives) is written **only** into the in-memory
`FileStore`. It is **not persisted to disk**. This is the root cause.

## What survives a restart today

`scanExistingFiles` (main.go:186) walks `originalsDir` on startup and
rebuilds the FileStore from `{id}.glb` filenames:

```go
record := &FileRecord{
    ID:           id,
    Filename:     e.Name(), // "<id>.glb" — original filename lost
    ...
}
```

The comment `// We lose original filename on restart` (main.go:203)
already flags this as a known issue. No sidecar, manifest, or alternate
store currently captures the upload-time name.

## How T-012-01's resolver consumes this gap

`species_resolver.go` already implements the read side of the manifest
this ticket has to write. Tier 5 (`SourceUploadManifest`) calls
`lookupUploadManifest(path, id)` which:

- Defaults `path` to `~/.glb-optimizer/uploads.jsonl` when empty.
- Opens the file (silently returns `("", false)` on missing/error).
- Scans line-by-line, decoding into `uploadManifestEntry`:

  ```go
  type uploadManifestEntry struct {
      ID       string `json:"id"`
      Filename string `json:"filename"`
  }
  ```

- Returns the **last** matching entry (re-uploads shadow earlier ones).

**Schema mismatch with ticket spec.** The ticket's example record uses
`hash`, `original_filename`, `uploaded_at`, `size`. The resolver uses
`id`, `filename`. Either the writer adopts the resolver's keys or both
sides switch to the ticket schema. Decision belongs in design.md.

## ResolverOptions surface

`ResolverOptions.UploadManifestPath` (species_resolver.go:91) already
exists as an explicit override "Tests use this to point at a tempdir."
That hook is exactly what the new write side will need for unit tests
without touching `~/.glb-optimizer`.

## Where the workDir lives

`main.go:61` resolves `workDir` from `-dir` flag, falling back to
`$HOME/.glb-optimizer`. All other persistent dirs (`originals`,
`outputs`, `settings`, `tuning`, `accepted`, `dist/plants`, ...) are
created under `workDir` by `os.MkdirAll` at main.go:82.

The uploads manifest naturally belongs at `filepath.Join(workDir,
"uploads.jsonl")`. With the default workDir this is exactly the path
the resolver hardcodes; with a custom `-dir` it diverges. Threading the
chosen path through both writer and resolver removes the risk.

## Originals directory contents (for backfill)

Per the previous T-012-01 discovery (memory 459): the on-disk originals
directory currently stores **only** content-hash filenames; no sidecar
records the upload-time name. Therefore the backfill script cannot
recover real human-readable names — its best effort is to write one
manifest line per existing `{id}.glb` with `original_filename = id +
".glb"` so the manifest is at least dense and the resolver can skip
re-scanning. The backfill is documented as "do this once" and is not
expected to recover lost data, only to seed the file.

## Existing tests

There is **no** `handlers_test.go` covering the main `handleUpload`
path. Tests for upload-style handlers exist for billboards
(`handlers_billboard_test.go`) and bake-complete (`handlers_bake_complete_test.go`)
and follow the pattern: build a multipart body in-memory, call the
handler with `httptest.NewRecorder`, assert on disk + response. The
new manifest tests can follow the same shape.

## Constraints / assumptions

- JSONL append must be crash-safe to **partial writes** but does not
  need to be crash-safe to **lost writes** — the upload itself is the
  primary record; the manifest is a derived index.
- Append failure must NOT fail the upload (ticket AC).
- File mode 0644, parent dir already created during workDir bootstrap.
- mtime-based cache invalidation per the ticket — single-process
  server, so no cross-process write coordination is required.
- `scripts/backfill-uploads-manifest.go` is idempotent: re-running adds
  no duplicates beyond the first run. Easiest implementation is to read
  the existing manifest first, build a hash-set of already-recorded
  ids, then append only the missing ones.

## Out-of-scope reminders

The ticket explicitly excludes: query API/CLI, garbage collection,
database migration, schema versioning, cross-machine sync. Keep the
implementation surface tight.
