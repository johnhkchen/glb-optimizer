# T-012-04 Design

## Decisions

### D1 — JSONL schema: adopt the ticket's field names

The ticket spec is the source of truth. Both the new writer **and**
T-012-01's existing reader switch to:

```json
{"hash":"<id>","original_filename":"<name>.glb","uploaded_at":"<rfc3339>","size":<bytes>}
```

Rationale:

- The ticket explicitly enumerates these fields (uploaded_at, size) and
  T-012-01's stub schema was a placeholder choice landed before this
  ticket existed.
- `uploaded_at` and `size` are cheap to write and useful for the
  backfill / GC follow-ups in S-012 even though no consumer reads them
  in v1.
- Field renames in `species_resolver.go::uploadManifestEntry` are a
  one-line struct edit; risk is contained.

**Rejected:** keep `id`/`filename` for resolver compat. Smaller diff
but locks the ticket spec into a debt that future readers would have
to trip over.

**Rejected:** dual-write both names. Hacky, doubles every line,
provides no value.

### D2 — Manifest path: `filepath.Join(workDir, "uploads.jsonl")`

Threaded through `handleUpload` as a new parameter (`uploadsManifestPath
string`) and through `ResolverOptions.UploadManifestPath` so callers
that respect a custom `-dir` flag stay consistent.

The default workDir is `~/.glb-optimizer`, so the existing resolver
default (`~/.glb-optimizer/uploads.jsonl`) keeps working unchanged for
the common case. We **also** update `lookupUploadManifest`'s default to
prefer the explicit path argument when provided, falling back to
`$HOME/.glb-optimizer/uploads.jsonl` only when no override is set.

**Rejected:** hardcoded `~/.glb-optimizer/uploads.jsonl` everywhere.
Breaks the `-dir` flag contract used by integration tests and dev
machines that segregate fixtures.

### D3 — Write semantics: append + fsync, swallow errors

`AppendUploadRecord(path, record) error` opens the file with
`O_APPEND|O_CREATE|O_WRONLY`, mode 0644, writes one marshalled JSON
object + `"\n"`, calls `f.Sync()`, then closes. Errors are returned to
the caller; the upload handler logs at WARN and continues per the AC.

`f.Sync()` is included because the manifest's whole reason to exist is
to survive restart/crash; an unflushed page-cache write defeats it.

**Rejected:** atomic temp-file + rename per record. Too expensive for a
hot upload path; JSONL append is crash-safe enough (worst case is a
truncated final line, which the reader skips via `json.Unmarshal`
returning an error → `continue`).

### D4 — Reader: in-memory cache, mtime invalidation

`LookupUploadFilename(hash) (string, error)` is the public read API
for **non-resolver consumers** (none in v1, but the ticket asks for
it). It maintains a package-level cache:

```go
var manifestCache struct {
    sync.RWMutex
    path  string
    mtime time.Time
    index map[string]string // hash → most-recent original_filename
}
```

On lookup:
1. Stat the file; if `os.IsNotExist`, clear cache, return `ErrNotFound`.
2. If mtime matches cached mtime, serve from cached index.
3. Otherwise re-scan the whole file, rebuild the index (last write
   wins per hash), update mtime, then serve.

The existing `lookupUploadManifest` in species_resolver.go is the
"raw" path-driven lookup used inside the resolver; it stays free of
caching to avoid a stale-during-test footgun. `LookupUploadFilename`
delegates to a shared scan helper but adds the cache layer. Both
share a single private `scanManifest(path string) (map[string]string,
error)` so the JSONL parsing rules live in one place.

**Rejected:** cache inside `lookupUploadManifest` directly. Resolver
tests reset state between cases by writing fresh tempdir manifests; a
package-global cache there would force test plumbing.

**Rejected:** inotify / fsnotify watcher. Vastly overkill, adds a dep,
single-process server doesn't need it.

### D5 — Backfill script: separate `package main` under `scripts/`

`scripts/backfill-uploads-manifest.go` lives as its own `package main`
with a `// +build ignore` build tag (so `go build ./...` does not try
to compile it as a second main). Run via `go run scripts/backfill-uploads-manifest.go`.

It:
1. Opens `~/.glb-optimizer` (or `-dir` flag).
2. Reads existing `uploads.jsonl` if present, builds a `seen` set of
   hashes.
3. Walks `originals/`, for each `*.glb`:
   - `hash := strings.TrimSuffix(name, ".glb")`
   - if `hash` not in `seen`: append a record using `original_filename
     = name`, `uploaded_at = file mtime`, `size = file size`.
4. Reports counts: scanned, skipped, appended.

**Rejected:** integrate into the server's startup path. Backfill is a
one-shot operator action; mixing it with serve-mode startup risks
silent re-runs and turns a debugging flag into a state mutation.

### D6 — Resolver tier already exists

T-012-01 already implements Tier 5 (`SourceUploadManifest`). The
ticket's "Resolver integration" AC is satisfied by the schema rename
(D1) plus updating `ResolverOptions.UploadManifestPath` to flow from
the CLI/server entry points. No tier reordering required.

## Risk register

| Risk | Mitigation |
|---|---|
| Manifest grows unbounded | Out of scope (S-012 follow-up); lines are ~150 bytes so 100k uploads ≈ 15 MB. Acceptable. |
| Concurrent uploads append in parallel | `O_APPEND` is atomic for writes ≤ PIPE_BUF on POSIX (typically 4 KB). Records are < 300 bytes. Safe without explicit lock. |
| Scan re-reads on every lookup pre-cache | `LookupUploadFilename` cache pins mtime; resolver path is one scan per pack which is fine. |
| Schema rename breaks existing test fixtures | The resolver test that uses the old `id`/`filename` shape will break; update those fixtures alongside the struct rename. |
| `go run scripts/...` gets accidentally caught by `go build ./...` | `// +build ignore` header. |
