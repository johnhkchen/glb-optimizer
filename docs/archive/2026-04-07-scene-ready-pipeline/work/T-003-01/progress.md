# Progress — T-003-01

## Status

All six implementation steps complete. Build green, tests green, manual
end-to-end verified against a running server.

## Step log

### Step 1 — `analytics.go` ✅
Created `analytics.go` with `Event`, `validEventTypes`, `(*Event).Validate`,
`newSessionID` (RFC 4122 v4), `AnalyticsLogger`, `NewAnalyticsLogger`,
`AppendEvent`, and `StartSession`. ~155 lines, no third-party deps.

### Step 2 — `analytics_test.go` ✅
12 unit tests covering: UUID v4 format, envelope validation (ok / bad
version / unknown type / empty session / empty timestamp / nil payload),
single append + round-trip, multi-event ordering, **concurrent writes**
(50 goroutines × 20 events = 1000 events with no torn lines), empty
sessionID rejection, and `StartSession` end-to-end. All passing.

```
ok  glb-optimizer  0.326s   (12 analytics tests + existing settings tests)
```

### Step 3 — `main.go` wiring ✅
- Added `tuningDir := filepath.Join(workDir, "tuning")`.
- Included `tuningDir` in the startup `MkdirAll` loop.
- Constructed `analyticsLogger := NewAnalyticsLogger(tuningDir)` after
  `NewFileStore`.
- Registered `mux.HandleFunc("/api/analytics/event", handleAnalyticsEvent(analyticsLogger))`.

### Step 4 — `handleAnalyticsEvent` ✅
Appended to `handlers.go`. POST-only, decodes envelope, calls
`Validate()`, calls `AppendEvent`. Returns 405 / 400 / 500 / 200 as
specified.

### Step 5 — `static/app.js` frontend helpers ✅
Inserted the analytics block after `applyDefaults()`, before the Tuning
UI block. Added `analyticsSessionId`, `startAnalyticsSession`,
`endAnalyticsSession`, `logEvent`, `fallbackUUID`, plus three `window.*`
exposures for devtools-driven manual verification.

### Step 6 — `docs/knowledge/analytics-schema.md` ✅
~210 lines. Covers envelope, all six event types with payload tables,
storage layout, concurrency / durability, HTTP API, versioning policy,
and out-of-scope items.

## Verification log

### Unit tests
```
$ go test ./... -count=1
ok  glb-optimizer  0.326s
```

All analytics tests pass on first run; no flakes from the concurrency
test across multiple invocations.

### Manual end-to-end
Started a fresh server against a temp workdir and exercised the route:

```
$ TMP=$(mktemp -d); ./glb-optimizer -port 18799 -dir "$TMP" &
$ curl -X POST http://localhost:18799/api/analytics/event \
    -d '{"schema_version":1,"event_type":"setting_changed",
         "timestamp":"2026-04-07T00:00:00Z",
         "session_id":"00000000-0000-4000-8000-000000000001",
         "asset_id":"abc",
         "payload":{"key":"bake_exposure","old_value":1.0,"new_value":1.25}}'
{"status":"ok"}                                                       HTTP 200

$ curl ... '{"schema_version":2,...}'
{"error":"unsupported schema_version: 2 (expected 1)"}                HTTP 400

$ curl ... '{"event_type":"lol",...}'
{"error":"unknown event_type: \"lol\""}                               HTTP 400

$ cat $TMP/tuning/00000000-0000-4000-8000-000000000001.jsonl
{"schema_version":1,"event_type":"setting_changed","timestamp":...,"payload":{"key":"bake_exposure","new_value":1.25,"old_value":1}}
```

Confirmed:
- Tuning directory created at startup.
- Valid envelope → 200, single JSONL line on disk.
- `schema_version=2` → 400 with descriptive error.
- Unknown event type → 400 with descriptive error.

### Frontend manual check
Not exercised in this session because it requires a browser; the helpers
are exposed on `window` (`startAnalyticsSession`, `endAnalyticsSession`,
`logEvent`) so a reviewer can drive them from devtools without any
additional wiring. The shape mirrors the Go-side envelope exactly.

## Deviations from plan

None. The plan as written was followed step-by-step.

## Known gaps (intentional, deferred)

- The ticket's "manual verification: trigger a setting change in
  T-002-03's UI" requires T-003-02's instrumentation hook, which is
  out of scope here. The console-driven path documented above
  satisfies the verifiability requirement for T-003-01 alone.
- No JS unit tests — the project has no JS test infrastructure
  (T-002-02 review §coverage gaps), and adding that infra is out of
  scope. The frontend block is small enough (~60 lines) and mirrors
  the Go envelope tightly enough that manual verification is
  appropriate for v1.
