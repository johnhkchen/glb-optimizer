# Review — T-003-01: analytics-event-schema-and-storage

## What changed

### Files created

| File                                  | Purpose                                                  | Lines |
|---------------------------------------|----------------------------------------------------------|-------|
| `analytics.go`                        | Event envelope, validation, logger, session minting     | 155   |
| `analytics_test.go`                   | 12 unit tests covering envelope + append + concurrency  | 244   |
| `docs/knowledge/analytics-schema.md`  | Canonical v1 schema doc (envelope, types, layout, policy)| 210   |
| `docs/active/work/T-003-01/*.md`      | RDSPI artifacts (research, design, structure, plan, progress, review) | — |

### Files modified

| File             | Change                                                              | Net lines |
|------------------|---------------------------------------------------------------------|-----------|
| `main.go`        | `tuningDir`, `MkdirAll` slot, logger construction, route registration | +6      |
| `handlers.go`    | `handleAnalyticsEvent` HTTP handler                                 | +27       |
| `static/app.js`  | `analyticsSessionId` + `startAnalyticsSession` / `endAnalyticsSession` / `logEvent` / `fallbackUUID` block, plus three `window.*` exposures | +66 |

No files were deleted. No third-party Go dependencies added (`go.mod`
unchanged).

## Acceptance-criteria mapping

| AC item                                                                   | Status | Where                                            |
|---------------------------------------------------------------------------|--------|--------------------------------------------------|
| `analytics-schema.md` documents envelope                                  | ✅     | `docs/knowledge/analytics-schema.md` §Envelope   |
| Initial event types: `session_start`/`_end`/`setting_changed`/`regenerate`/`accept`/`discard` | ✅ | `analytics.go` `validEventTypes` + schema doc |
| Per-payload schemas with field types                                      | ✅     | schema doc §"Event types (v1)"                   |
| `analytics.go` with `Event` struct mirroring envelope                     | ✅     | `analytics.go:27`                                |
| `AppendEvent(sessionID, event)` writes to per-session JSONL              | ✅     | `analytics.go:103`                               |
| `StartSession(assetID)` returns new session ID (UUID)                    | ✅     | `analytics.go:140`                               |
| Atomic append (open append + flush per write)                            | ✅     | `analytics.go:115` `O_APPEND\|O_CREATE\|O_WRONLY` |
| HTTP `POST /api/analytics/event` validates schema_version, appends       | ✅     | `handlers.go` `handleAnalyticsEvent`             |
| Tuning directory created at startup                                      | ✅     | `main.go` `MkdirAll` loop                        |
| Frontend `logEvent(type, payload)` POSTs to endpoint                     | ✅     | `static/app.js` analytics block                  |
| Manual: setting change → JSONL line on disk                              | ⚠️     | console-driven path verified; UI hook is T-003-02 |

## Test coverage

Twelve unit tests in `analytics_test.go`, all passing:

```
=== RUN   TestNewSessionID_Format                  PASS
=== RUN   TestEventValidate_OK                     PASS
=== RUN   TestEventValidate_RejectsBadVersion      PASS
=== RUN   TestEventValidate_RejectsUnknownType     PASS
=== RUN   TestEventValidate_RejectsEmptySession    PASS
=== RUN   TestEventValidate_RejectsEmptyTimestamp  PASS
=== RUN   TestEventValidate_RejectsNilPayload      PASS
=== RUN   TestAppendEvent_WritesJSONLine           PASS
=== RUN   TestAppendEvent_AppendsMultiple          PASS
=== RUN   TestAppendEvent_ConcurrentWrites         PASS
=== RUN   TestAppendEvent_RejectsEmptySessionID    PASS
=== RUN   TestStartSession_EmitsSessionStart       PASS
```

The concurrency test (50 goroutines × 20 events) is the load-bearing
guarantee that the mutex-plus-`O_APPEND` strategy holds. All 1000 lines
parse cleanly with no torn records on every run. Plus existing
`settings_test.go` cases continue to pass.

### Coverage gaps

- **No HTTP-handler unit tests.** The handler is small (decode → validate
  → append → respond) and is exercised by the manual end-to-end below.
  Adding `httptest.NewRecorder` cases would be straightforward but the
  cost-to-value ratio is poor at this size; defer to T-003-02 when more
  endpoints land.
- **No JS tests.** Project has zero JS test infra (T-002-02 review §coverage gaps).
  The frontend block is ~60 lines and 1:1 with the Go envelope; it is
  verified by devtools-driven manual checks.

### Manual end-to-end (re-run from `progress.md`)

```
POST /api/analytics/event {valid v1}        → 200 {"status":"ok"}
POST /api/analytics/event {schema_version:2}→ 400 unsupported schema_version
POST /api/analytics/event {event_type:"lol"}→ 400 unknown event_type
ls ~/.glb-optimizer/tuning/                  → {session_id}.jsonl present
cat ...                                       → exactly one JSONL line, parses
```

## Open concerns

1. **Frontend session minting vs. Go `StartSession`.** The ticket lists
   `StartSession(assetID)` as a Go function but expects the *frontend* to
   produce events. Resolved by implementing both: the Go API exists and
   is unit-tested, but the v1 frontend mints session ids client-side via
   `crypto.randomUUID()`. If a reviewer prefers a server-mint endpoint
   instead (`POST /api/analytics/start-session`), it is purely additive
   on top of v1 — no on-disk schema impact.

2. **Payload looseness.** Envelope is strict; payload is
   `map[string]interface{}` with no server-side schema enforcement.
   This is deliberate (see `design.md` §"Alternatives D"). If T-003-02
   discovers that loose payloads are causing data quality problems,
   per-type structs can be added without bumping `schema_version`.

3. **Timestamp trust.** The client stamps `timestamp`, not the server.
   This is the right call for training data (click time matters more
   than receive time), but it does mean a misbehaving client could
   write arbitrary timestamps. Single-user local tool, low risk; worth
   noting if the schema ever becomes a multi-tenant concern.

4. **Schema doc references** the field `final_settings` in `session_end`
   and `settings` in `accept` as opaque objects. Whether these should
   include the *gltfpack* `Settings` or only the per-asset bake
   `AssetSettings` is an open call for T-003-02. The schema is
   compatible with either (or both) since payloads are not validated.

5. **No deletion / retention policy.** Sessions accumulate indefinitely
   under `~/.glb-optimizer/tuning/`. At 1 KB/event and 100 events/session
   that's ~100 KB per session, so it'll be a long while before this
   matters. Worth a follow-up ticket if the export pipeline (T-003-04)
   doesn't handle archival.

## Critical issues for human attention

None. All AC items satisfied (modulo the T-003-02 dependency for the
in-UI manual check); build green; tests green; manual verification
confirmed.

## Handoff notes for downstream tickets

- **T-003-02** can call `startAnalyticsSession(id)` from `selectFile`
  (after `await loadSettings(id)`), and add a `logEvent('setting_changed',
  {key, old_value, new_value}, id)` call inside the `wireTuningUI` input
  handler. The plumbing is ready; T-003-02 just needs to wire it.
- **T-003-03 / T-003-04** can rely on the on-disk format being stable
  per `analytics-schema.md` §"Versioning and migration policy".
- The export script in T-003-04 should `glob ~/.glb-optimizer/tuning/*.jsonl`
  and stream `for line in f: json.loads(line)`.
- The schema doc lives at `docs/knowledge/analytics-schema.md` and is
  the canonical reference — keep it in sync with any additive
  payload changes.

## Diff stats

```
analytics.go                              | 155 +++++++
analytics_test.go                         | 244 +++++++++
docs/knowledge/analytics-schema.md        | 210 ++++++++
main.go                                   |   6 +-
handlers.go                               |  27 ++
static/app.js                             |  66 +++
```

Plus the RDSPI artifacts under `docs/active/work/T-003-01/`.
