# Research — T-003-01: analytics-event-schema-and-storage

## Ticket scope

First brick of S-003 (analytics-foundation). The goal is to land a versioned
event envelope, a JSONL append-only storage layer, an HTTP capture endpoint,
and a frontend helper. No instrumentation, no profiles, no UI surface — those
are downstream tickets (T-003-02, T-003-03, T-003-04).

The schema decision is **permanent**: events landing on disk under v1 must
remain readable forever, or every future change has to migrate.

## Codebase shape (relevant slice)

Single Go binary, no third-party deps (`go.mod` declares only `module
glb-optimizer` / `go 1.22`). One package `main`. Files of interest:

| File              | Role                                                       |
|-------------------|------------------------------------------------------------|
| `main.go`         | CLI flags, working-dir bootstrap, route registration       |
| `handlers.go`     | All HTTP handlers, plus `generateID()` and JSON helpers    |
| `models.go`       | `FileStore`, `FileRecord`, `Settings`, `SceneRequest` etc. |
| `settings.go`     | Persistent per-asset settings (T-002-03 work product)      |
| `settings_test.go`| Unit tests using `testing` + `t.TempDir()`                 |
| `static/app.js`   | Single ESM module, ~2480 lines, no bundler                 |
| `static/index.html`| Hand-authored DOM, no template engine                     |

## Working-directory convention

`main.go` already establishes the layout under `~/.glb-optimizer/`:

```go
workDir   = ~/.glb-optimizer (overridable via -dir)
originals = workDir/originals
outputs   = workDir/outputs
settings  = workDir/settings
```

Each subdir is `os.MkdirAll`-ed at startup with mode `0755`. The ticket asks
for a peer `tuning/` directory created at startup with the same lifecycle.

## HTTP routing

`net/http.ServeMux` with explicit `mux.HandleFunc` calls in `main.go`. There
is no router library, no middleware stack. Each handler is a closure that
captures the dependencies it needs (store, paths, etc.). The pattern for new
endpoints is straightforward: register one line in `main.go`, define the
handler in `handlers.go`.

`handlers.go` already exposes two helpers we will reuse:

- `jsonResponse(w, status, data)` and `jsonError(w, status, msg)`
- `generateID()` — 16 random bytes hex-encoded (32 chars). This is **not** a
  UUID by RFC standards; the ticket explicitly says "session ID (UUID)". We
  need to decide whether to reuse `generateID()` (32-char hex), reuse it
  rebranded, or emit a true RFC 4122 v4 UUID. No external deps means we'd
  hand-roll the v4 byte twiddling — trivial (~10 lines).

## Persistence patterns already in the codebase

`settings.go` is the freshest example (T-002-03) and sets the precedent:

- `SchemaVersion` is a top-level constant; the struct embeds the value as
  the first field; `Validate()` rejects mismatches.
- Atomic writes use `writeAtomic()` — temp file + `os.Rename` — for
  persistence files that must not be half-written on crash.
- `LoadSettings` returns defaults on `os.IsNotExist`, surfaces other errors.
- File-per-asset, JSON-encoded with 2-space indent, trailing newline.

**Key divergence for analytics:** the analytics layer is *append-only*, not
read-modify-write. Atomic-rename is the wrong primitive for an append log:
it would be O(n) per event and would race with other appends. The right
primitive is `os.OpenFile(path, O_APPEND|O_CREATE|O_WRONLY, 0644)` followed
by a single `Write` of one JSON object plus `\n`. POSIX guarantees that
writes to a file opened with `O_APPEND` are atomic up to `PIPE_BUF` (4096
bytes on Linux/macOS) — and an analytics envelope will be far smaller than
that. Concurrency safety inside the process additionally needs a
`sync.Mutex` (or per-session mutex) because Go's `os.File.Write` is not
serialized across goroutines. The ticket's "atomic append (open append +
flush per write)" wording confirms this is the expected approach.

## Existing concurrency model

`FileStore` uses `sync.RWMutex`. `handleProcess` runs gltfpack synchronously
on the request goroutine — there is no worker pool, no event loop. For the
analytics endpoint, the natural shape is: handler goroutine takes a process-
wide lock, opens the per-session file in append mode, writes one line,
closes, releases lock, returns 200. Throughput is bounded by user clicks
(human-rate, not machine-rate); a single mutex is fine for v1.

## Frontend conventions

`static/app.js` is plain ESM (imports from `three/addons/...`). No bundler,
no test infrastructure (T-002-02 review §coverage gaps confirms this). State
is module-level top-of-file. Network calls are `fetch()` with `await`. There
is no central API client — handlers each call `fetch` directly. Functions
are top-level, not exported, not namespaced.

The ticket allows either a new `analytics.js` file or a section in
`app.js`. The codebase precedent (T-002-03 added Tuning UI inline rather
than splitting) leans inline. The function the ticket names is
`logEvent(type, payload)`.

## What `setting_changed` means in the current UI

T-002-03 wired eleven tuning controls (`tuneVolumetricLayers`, etc.) into
`saveSettings(id)` via `populateTuningUI()` / `wireTuningUI()`. Each
`input` listener mutates `currentSettings[key]` and schedules a debounced
PUT to `/api/settings/:id`. T-003-02 will hook these listeners to also emit
`setting_changed` events; T-003-01 only needs `logEvent` to exist.

## `generateID` vs UUID

`handlers.go:19` defines `generateID()` as 16 random bytes → 32-char hex.
A UUID v4 is also 16 random bytes, but with two version/variant bits set
and the canonical 8-4-4-4-12 hyphenation. Either is acceptable as a
session id; the ticket explicitly says "UUID". Lowest-friction path:
hand-roll a `newUUIDv4()` helper inside `analytics.go` so that v1 events on
disk are unambiguous (`8400ce0e-...`) rather than ambiguous hex blobs that
collide with file IDs in the same workdir.

## Out-of-scope reminders (S-003 acceptance criteria not in this ticket)

These exist in the parent story but T-003-01 explicitly defers them:

- Profiles (`~/.glb-optimizer/profiles/`)
- Accepted-tag snapshots
- Thumbnail linking on `regenerate`
- Export script (`scripts/export_tuning_data.py`)
- Auto-instrumentation hooks (T-003-02)

The schema **must** leave room for them — `regenerate` payload should
already have a `thumbnail_path` field even though nothing populates it
yet — but no code needs to touch them.

## Constraints / assumptions

- Go 1.22, no third-party deps. Hand-roll UUID; use `crypto/rand`.
- Single-user, local. No auth, no rate limiting.
- The JSONL file format is "one JSON object per line, UTF-8, no BOM".
  Readers (export script in T-003-04) will `for line in file: json.loads`.
- Timestamps are RFC 3339 UTC strings (`time.Now().UTC().Format(time.RFC3339Nano)`).
  Numeric epochs are smaller but harder to eyeball during dev; the ML
  pipeline can convert later.
- `schema_version` is an integer, mirroring `SettingsSchemaVersion`.
- The HTTP endpoint accepts the *envelope* — not "type + payload" — so
  clients are explicit about what they're sending and the server validates
  `schema_version` directly.
