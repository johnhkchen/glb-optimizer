# Plan — T-003-01: analytics-event-schema-and-storage

## Step sequence

Each step is sized to commit atomically. Steps 1–2 are pure backend with
no external surface; steps 3–4 wire the HTTP layer; step 5 lands the
frontend; step 6 documents the schema.

### Step 1 — `analytics.go`

Create the file with:

- `AnalyticsSchemaVersion` const.
- `Event` struct (envelope shape from design.md).
- `validEventTypes` map.
- `(*Event).Validate()` — checks `schema_version`, `event_type`,
  non-empty `timestamp`, non-empty `session_id`, asset_id may be empty.
- `newSessionID()` — RFC 4122 v4 from `crypto/rand`.
- `AnalyticsLogger` struct with `sync.Mutex` + `tuningDir`.
- `NewAnalyticsLogger(tuningDir string) *AnalyticsLogger`.
- `AppendEvent(sessionID string, ev Event) error` — locks, opens with
  `O_APPEND|O_CREATE|O_WRONLY`, writes one JSON line, closes.
- `StartSession(assetID string) (string, error)` — mints UUID, calls
  `AppendEvent` with a `session_start` envelope, returns the new session id.

**Verification:** `go build ./...` passes.

### Step 2 — `analytics_test.go`

Standard `testing` package, `t.TempDir()` for the tuning dir.

Cases:

1. `TestNewSessionID_Format` — output matches the v4 regex
   `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`.
2. `TestEventValidate_OK` — a valid envelope passes.
3. `TestEventValidate_RejectsBadVersion` — schema_version=2 → error.
4. `TestEventValidate_RejectsUnknownType` — unknown event_type → error.
5. `TestEventValidate_RejectsEmptySession` — empty session_id → error.
6. `TestAppendEvent_WritesJSONLine` — append one event, read file,
   single line, `json.Unmarshal` round-trips equal.
7. `TestAppendEvent_AppendsMultiple` — three events, three lines, in order.
8. `TestAppendEvent_ConcurrentWrites` — 50 goroutines × 20 events,
   final line count = 1000, every line valid JSON, no duplicates.
9. `TestStartSession_EmitsSessionStart` — call StartSession, verify the
   session file exists with one `session_start` event whose payload says
   `trigger=open_asset` and asset_id matches the input.

**Verification:** `go test ./... -run Analytics` (or just `go test ./...`)
passes.

### Step 3 — `main.go` wiring

1. Add `tuningDir := filepath.Join(workDir, "tuning")` next to the other
   subdirs.
2. Add `tuningDir` to the `MkdirAll` loop.
3. After `store := NewFileStore()`, add
   `analyticsLogger := NewAnalyticsLogger(tuningDir)`.
4. Register `mux.HandleFunc("/api/analytics/event", handleAnalyticsEvent(analyticsLogger))`
   alongside the other `/api/...` routes.

**Verification:** `go build ./...` passes; running the binary creates
`~/.glb-optimizer/tuning/` (will be confirmed in step 4 manual test).

### Step 4 — `handleAnalyticsEvent` in `handlers.go`

Append the handler at the bottom of the file. It:

- Rejects non-POST with 405.
- Decodes the body into `Event`.
- Calls `ev.Validate()` and rejects on error with 400.
- Calls `logger.AppendEvent(ev.SessionID, ev)` and rejects on error with 500.
- Returns 200 `{"status":"ok"}`.

**Verification:**

```sh
go build ./... && ./glb-optimizer -port 18787 &
PID=$!
sleep 1
curl -s -X POST http://localhost:18787/api/analytics/event \
  -H 'Content-Type: application/json' \
  -d '{"schema_version":1,"event_type":"setting_changed","timestamp":"2026-04-07T00:00:00Z","session_id":"00000000-0000-4000-8000-000000000000","asset_id":"abc","payload":{"key":"foo","old_value":1,"new_value":2}}'
kill $PID
cat ~/.glb-optimizer/tuning/00000000-0000-4000-8000-000000000000.jsonl
```

Expected: a single JSONL line containing the envelope.

### Step 5 — `static/app.js` frontend helpers

Insert the analytics block after the `// ── Asset Settings ──` section
(after `loadSettings`/`saveSettings`/`getSettings`/`applyDefaults`). Add:

- `analyticsSessionId` module variable.
- `startAnalyticsSession(assetId)`.
- `endAnalyticsSession(outcome, finalSettings, assetId)`.
- `logEvent(type, payload, assetId)`.
- `fallbackUUID()` for browsers without `crypto.randomUUID`.
- `window.startAnalyticsSession`, `window.endAnalyticsSession`,
  `window.logEvent` exposures.

**Verification:** Reload the page in a browser, open devtools, run
`startAnalyticsSession('manual-test')` then
`logEvent('setting_changed', {key:'bake_exposure', old_value:1, new_value:1.25}, 'manual-test')`,
then check `~/.glb-optimizer/tuning/` for a freshly-created session file
containing two lines.

### Step 6 — `docs/knowledge/analytics-schema.md`

Document the v1 schema with the same depth as `settings-schema.md`:

- Envelope table.
- Event type enum with semantic descriptions.
- Per-event payload schemas with field types.
- Storage layout.
- Versioning / migration policy (mirrors the settings policy).
- Forward-compatibility notes (additive fields OK; renames or
  removals require a schema bump).

**Verification:** `wc -l` ~150; the document covers everything in the
ticket's "Acceptance Criteria" §1.

## Testing strategy

- **Unit tests** live in `analytics_test.go` and cover the Go layer
  exhaustively (envelope validation, append correctness, concurrency,
  session minting).
- **No JS tests** — the project has zero JS test infrastructure
  (T-002-02 review §coverage gaps). The frontend helper is verified by
  manual devtools poking, as the ticket explicitly prescribes.
- **Manual end-to-end** is the ticket's stated acceptance bar:
  trigger a setting change in T-002-03's UI (after T-003-02 wires the
  hook — *out of scope here*) and confirm a JSONL line lands on disk.
  For T-003-01 alone, the manual verification is the curl + devtools
  variant described in steps 4 and 5.

## Verification criteria (acceptance mapping)

| AC item                                              | Verified by                  |
|------------------------------------------------------|------------------------------|
| `analytics-schema.md` documents envelope + types     | Step 6 artifact              |
| `analytics.go` with `Event`, `AppendEvent`, etc.     | Step 1 + Step 2 unit tests   |
| Atomic append, concurrent-safe                       | `TestAppendEvent_ConcurrentWrites` |
| `POST /api/analytics/event` w/ schema validation     | Step 4 + manual curl         |
| Tuning dir created at startup                        | Step 3 (`MkdirAll`) + step 4 manual |
| Frontend `logEvent` POSTs to endpoint                | Step 5 + manual devtools     |
| Manual: setting change → JSONL line on disk          | Step 5 manual (post T-003-02) |

## Risks / open questions

- **Frontend session minting vs. ticket wording.** The ticket lists
  `StartSession` as a Go function but expects the frontend to log
  events. We resolve this by having both sides able to mint, with the
  frontend authoritative in v1 (see design.md). If a reviewer wants the
  frontend to call a `POST /api/analytics/start-session` endpoint
  instead, that's an additive change — easy to bolt on without breaking
  v1 events on disk.
- **Payload looseness.** Envelope is strict; payload is `map[string]interface{}`.
  This is deliberate (see design.md §"Alternatives D"). If T-003-02 finds
  this too loose in practice, we add per-type structs in a follow-up
  without bumping `schema_version`.
- **Manual E2E for the AC's "trigger a setting change" check** depends on
  T-003-02's instrumentation hook, which is out of scope. T-003-01's
  artifact will note this as a known gap; the ticket's other AC items
  are all satisfied independently.
