---
id: T-012-04
story: S-012
title: persist-original-filename
type: task
status: open
priority: medium
phase: done
depends_on: []
---

## Context

T-010-04's review noted Open Concern #1: "filename loss across server restarts — CLI always derives species from id, not original filename; fix belongs in upload pipeline, not this ticket." That fix lives here.

When the glb-optimizer server processes an upload, it stores the file under a content-hash filename (`{hash}.glb`) and forgets the original name (`achillea_millefolium.glb`). After a server restart, there's no way to recover the human-readable name from the hash. This is the root cause of T-012-01's friction — without persisted provenance, the resolver has nothing to fall back on except hand-authored sidecars.

This ticket adds a persistent upload manifest so that every uploaded source GLB carries its original filename forward, surviving server restarts.

## Acceptance Criteria

### Upload manifest format

- New file: `~/.glb-optimizer/uploads.jsonl` — append-only JSONL log
- Each line is one upload record:
  ```json
  {"hash":"0b5820c3aaf51ee5cff6373ef9565935","original_filename":"achillea_millefolium.glb","uploaded_at":"2026-04-08T19:30:00Z","size":14266164}
  ```
- Append-only: never rewritten in place. Duplicates (same hash uploaded multiple times) are appended again with the new timestamp; readers take the most recent entry per hash.

### Server-side write

- The existing upload handler (find via grep — likely `handleUpload` or similar in handlers.go) appends a record to `uploads.jsonl` after successfully storing the file
- Append failure logs a warning but does NOT fail the upload (backwards compatibility — uploads work even if the manifest write hits an error)
- File is created with mode 0644 if absent

### Server-side read

- New function `LookupUploadFilename(hash string) (string, error)` in a new `upload_manifest.go`
- Reads `uploads.jsonl` line by line, returns the most recent `original_filename` for the given hash
- Returns `(_, ErrNotFound)` if hash not present
- Caches the parsed manifest in memory with mtime-based invalidation (re-read when file changes)

### Resolver integration

- T-012-01's `ResolveSpeciesIdentity` resolution chain gains a new tier: **upload manifest** between "original filename" (filesystem-based) and "content hash fallback" (last resort). If T-012-01 has shipped before this ticket, edit the resolver to add the new tier; if not, T-012-01's implementation should anticipate this tier per its acceptance criteria.

### Backfill (one-shot script)

- New file: `scripts/backfill-uploads-manifest.go` — walks `~/.glb-optimizer/originals/` (or wherever existing original filenames are kept) and writes one manifest line per existing file
- Idempotent: re-running adds no duplicates beyond the first run
- Documented in S-012's story notes as the "do this once for the existing intermediates" step

### Tests

- Unit test: append a record, read it back, verify field round-trips
- Unit test: append two records for the same hash, lookup returns the most recent
- Unit test: read from non-existent file returns `ErrNotFound`, not a parse error
- Unit test: append with a write error logs warning but doesn't propagate
- Unit test: cache invalidation — modify file mtime, next lookup re-reads
- Integration test: upload a file via the existing upload handler, restart the server (simulated by re-instantiating the manifest reader), verify the filename is recoverable

## Out of Scope

- A query API for the manifest (CLI subcommand to list uploads, etc.) — the resolver is the only consumer for v1
- Garbage collection of old entries
- Switching to a real database (JSONL is fine at this scale)
- UI for browsing uploads
- Cross-machine sync of the manifest
- Schema versioning of the JSONL format (add a `format_version` field if you anticipate change; otherwise keep it simple)

## Notes

- This ticket's value compounds with T-012-01: alone, it's invisible. Together, the producer agent never has to author a sidecar by hand.
- The append-only design is intentional. JSONL files survive partial-write crashes (the worst case is a truncated final line, which a careful reader skips). Compare with a JSON object file that requires atomic rewrites.
- If the existing upload handler stores the original filename anywhere already (e.g., as a `.meta.json` next to the hash file), this ticket reduces to "centralize that data into uploads.jsonl and migrate." Discover during research phase.
