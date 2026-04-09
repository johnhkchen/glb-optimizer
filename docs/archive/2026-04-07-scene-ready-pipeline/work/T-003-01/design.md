# Design — T-003-01: analytics-event-schema-and-storage

## Decision summary

1. **Envelope-first schema.** A single `Event` struct with a fixed envelope
   and an opaque `payload` map. Server validates `schema_version` and
   `event_type`; payload shape is documented in markdown but not enforced
   at the wire layer.
2. **One JSONL file per session**, append-only, opened-for-each-write
   (no long-lived file handles).
3. **Process-wide `sync.Mutex`** serializes appends. No per-session lock
   pool — premature.
4. **Hand-rolled UUID v4** for session IDs. No new dependency.
5. **`POST /api/analytics/event`** accepts a complete envelope; the server
   stamps nothing. Client controls timestamp, session_id, asset_id.
6. **Frontend helper inline in `app.js`** — no new file.

## Schema

### Envelope (v1)

| Field            | Type     | Required | Notes                                       |
|------------------|----------|----------|---------------------------------------------|
| `schema_version` | int      | yes      | `1` for the v1 schema                       |
| `event_type`     | string   | yes      | One of the enum below                       |
| `timestamp`      | string   | yes      | RFC 3339 nano UTC, e.g. `2026-04-07T12:34:56.789Z` |
| `session_id`     | string   | yes      | UUID v4 from `StartSession` (or empty for `session_start` itself — see below) |
| `asset_id`       | string   | yes      | The file ID the event refers to. Empty allowed only for non-asset events; v1 has none |
| `payload`        | object   | yes      | Type-specific. Must be a JSON object even when empty (`{}`) |

### Event types (v1)

- `session_start` — emitted exactly once per session.
- `session_end`   — emitted exactly once per session.
- `setting_changed` — one per slider/input commit.
- `regenerate` — user triggered a bake/optimize action.
- `accept` — user marked the current settings as the canonical accepted profile.
- `discard` — user discarded the current session (revert).

### Per-type payloads

```jsonc
// session_start
{ "trigger": "open_asset" }    // freeform reason; "open_asset" for v1

// session_end
{
  "outcome": "accept" | "discard" | "leave",
  "duration_ms": 12345,
  "final_settings": { ... full AssetSettings ... }
}

// setting_changed
{
  "key": "bake_exposure",
  "old_value": 1.0,
  "new_value": 1.25
}

// regenerate
{
  "trigger": "tune_bake_exposure" | "manual" | ...,
  "output_glb": "outputs/abc.glb",   // optional, may be ""
  "thumbnail_path": ""               // reserved for S-003 thumbnail work; v1 always ""
}

// accept
{ "settings": { ... full AssetSettings ... } }

// discard
{ "reason": "" }   // freeform, optional
```

These payload schemas are *documented* in `analytics-schema.md` but
**not** validated server-side. The envelope is the contract; payload
evolution is permitted within v1 as long as new fields are additive and
optional.

### Why this envelope shape

The S-003 acceptance criteria list six fields' worth of data on
`setting_changed` (`timestamp, session, asset_id, key, old_value, new_value`).
Splitting them into envelope + payload keeps the envelope generic enough
to also carry `regenerate` (which has a totally different shape) without
union-typing the top level. Every future event type costs zero envelope
churn.

## Storage layout

```
~/.glb-optimizer/
    tuning/
        {session_id}.jsonl     # one file per session
```

`tuning/` is created at startup alongside `originals/`, `outputs/`,
`settings/`. One file per session means:

- Append-only writes never grow a single hot file unbounded.
- Crash in the middle of a session loses at most the in-flight event
  (the prior bytes are durable on disk).
- T-003-04 export script can `glob *.jsonl` and stream events session by
  session.
- Deleting an asset doesn't pollute analytics; deleting a session is a
  single `unlink` (not in this ticket, but easy if it ever matters).

## Concurrency model

```go
type AnalyticsLogger struct {
    mu       sync.Mutex
    tuningDir string
}

func (a *AnalyticsLogger) AppendEvent(sessionID string, ev Event) error {
    a.mu.Lock()
    defer a.mu.Unlock()
    f, err := os.OpenFile(
        filepath.Join(a.tuningDir, sessionID+".jsonl"),
        os.O_APPEND|os.O_CREATE|os.O_WRONLY,
        0644,
    )
    if err != nil { return err }
    defer f.Close()
    data, err := json.Marshal(ev)
    if err != nil { return err }
    data = append(data, '\n')
    _, err = f.Write(data)
    return err
}
```

A single global mutex is fine: the human-rate of events (~1/sec peak per
user, single-user tool) is six orders of magnitude below the cost of a
mutex acquisition. Re-opening the file every call is also fine — the cost
is dominated by the syscall floor either way, and never holding a long-
lived file handle means we don't have to think about reopen-on-rotate or
fsync semantics.

## Session ID format

**Decision: RFC 4122 v4 UUID.** Hand-rolled (~12 lines). The ticket asks
for a UUID by name, and a UUID is visually distinguishable from the
existing 32-char hex `generateID()` file IDs in the same `~/.glb-optimizer/`
tree, which avoids confusion when grepping logs.

```go
func newSessionID() string {
    var b [16]byte
    _, _ = rand.Read(b[:])
    b[6] = (b[6] & 0x0f) | 0x40 // version 4
    b[8] = (b[8] & 0x3f) | 0x80 // variant 10
    return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
```

## HTTP API

### `POST /api/analytics/event`

Request body: a single JSON envelope. The server:

1. Decodes into `Event`.
2. Rejects `schema_version != 1` with 400.
3. Rejects unknown `event_type` with 400.
4. Rejects empty `session_id` with 400 (clients must call `StartSession` first).
5. Calls `logger.AppendEvent(ev.SessionID, ev)`.
6. Returns `200 {"status":"ok"}`.

Validation is intentionally minimal — payload contents are not introspected.
No rate limiting, no batching, no authentication, no CORS (same-origin only).

### Why no `StartSession` HTTP endpoint?

The Go `StartSession(assetID)` function the ticket requires is exposed as
*Go API* — it's the canonical way for the backend to mint a session id. But
the **frontend** is the producer of events in v1, so the frontend mints its
own session ids client-side using `crypto.randomUUID()` (browser built-in,
all evergreen browsers). The Go `StartSession` is still implemented and
unit-tested for completeness and for the eventual case where the backend
needs to log events on its own behalf (e.g., a future processing pipeline).

This split is the only design choice that deviates from a strict reading of
the ticket. The ticket lists `StartSession(assetID)` under "New Go module
analytics.go" without saying it must be reachable by the frontend — and the
*frontend helper* requirement is listed separately. Reading both together,
mints-on-both-sides is the simplest correct shape.

## Frontend helper

Inline in `app.js`, near the existing `loadSettings` / `saveSettings`
block. Surface:

```js
let analyticsSessionId = null;

function startAnalyticsSession(assetId) {
    analyticsSessionId = crypto.randomUUID();
    logEvent('session_start', { trigger: 'open_asset' }, assetId);
    return analyticsSessionId;
}

async function logEvent(type, payload, assetId) {
    if (!analyticsSessionId) return; // no-op if no active session
    const envelope = {
        schema_version: 1,
        event_type: type,
        timestamp: new Date().toISOString(),
        session_id: analyticsSessionId,
        asset_id: assetId || '',
        payload: payload || {},
    };
    try {
        await fetch('/api/analytics/event', {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify(envelope),
        });
    } catch (err) {
        console.warn('logEvent failed:', err);
    }
}
```

T-003-01 does **not** wire `startAnalyticsSession` into `selectFile` or any
other lifecycle hook — that is T-003-02. T-003-01 only ships the helpers
plus a window-exposed binding so the manual verification step in the
ticket AC ("trigger a setting change in T-002-03's UI, confirm a JSONL
line lands on disk") can be performed by typing
`startAnalyticsSession('...'); logEvent('setting_changed', {key:'foo'}, '...')`
into the devtools console.

## Alternatives considered and rejected

### A. Single global `events.jsonl`

Rejected. Mixing sessions in one file makes per-session deletion hard,
forces every reader to filter, and turns one hot file into the only hot
file. Per-session files cost nothing and align with the per-session
storage model the parent story already prescribes.

### B. SQLite via `mattn/go-sqlite3`

Rejected. Adds a CGO dependency to a project that currently has *zero*
third-party deps. JSONL is sufficient for the ML export pipeline (T-003-04
will trivially convert it). Sticking with the file system honors the
"local-only, single-user, all local" constraint from S-003.

### C. Strongly-typed payload structs per event

Rejected for v1. It would force a tagged-union shape in Go (via interface
or json.RawMessage) and a parallel set of TypeScript types we don't have
the infra for. The v1 envelope makes evolution cheap; adding strict
payload validation later is purely additive and can happen in T-003-02
once the field set has stabilized.

### D. Long-lived file handle per session

Rejected. Saves microseconds, costs us a session-handle map, eviction
policy, and crash semantics for unflushed writes. Open-write-close per
event is the obvious correct shape at this throughput.

### E. Server-stamped timestamps

Rejected. The client knows when the user clicked; the server knows when
the request arrived. For training data, the click time is what matters,
and the human reaction time gap (~10–500 ms) is the dominant noise. We
let the client stamp and trust it. The server can sanity-check skew if
that ever becomes a problem.

## Test strategy

`analytics_test.go`:

- `TestNewSessionID_Format` — UUID v4 shape, version/variant bits.
- `TestAppendEvent_Roundtrip` — write three events, read the file back,
  assert one JSON object per line in order.
- `TestAppendEvent_ConcurrentWrites` — N goroutines × M events each, no
  data loss, no torn lines.
- `TestEventValidate_RejectsBadVersion` — schema_version=2 is rejected.
- `TestEventValidate_RejectsUnknownType` — `event_type="lol"` is rejected.
- `TestStartSession_CreatesUUID` — returns a non-empty UUID and the
  session id parses as v4.
- `TestHandleAnalyticsEvent_OK` — POST a valid envelope, assert 200 and
  one JSONL line on disk.
- `TestHandleAnalyticsEvent_BadVersion` — POST schema_version=2, assert 400.
