# Structure — T-003-01: analytics-event-schema-and-storage

## Files touched

| File                                  | Action  | Approx. lines |
|---------------------------------------|---------|---------------|
| `analytics.go`                        | CREATE  | ~140          |
| `analytics_test.go`                   | CREATE  | ~180          |
| `docs/knowledge/analytics-schema.md`  | CREATE  | ~150          |
| `main.go`                             | MODIFY  | +10           |
| `handlers.go`                         | MODIFY  | +55           |
| `static/app.js`                       | MODIFY  | +45           |

No deletions. No changes to `models.go`, `settings.go`, `processor.go`,
`scene.go`, `blender.go`, `static/index.html`, or `static/style.css`.

## `analytics.go` (new)

Package `main`. Public surface:

```go
const AnalyticsSchemaVersion = 1

// Event is the canonical envelope written to disk.
type Event struct {
    SchemaVersion int                    `json:"schema_version"`
    EventType     string                 `json:"event_type"`
    Timestamp     string                 `json:"timestamp"`
    SessionID     string                 `json:"session_id"`
    AssetID       string                 `json:"asset_id"`
    Payload       map[string]interface{} `json:"payload"`
}

// validEventTypes enumerates the v1 event_type set.
var validEventTypes = map[string]bool{
    "session_start":   true,
    "session_end":     true,
    "setting_changed": true,
    "regenerate":      true,
    "accept":          true,
    "discard":         true,
}

func (e *Event) Validate() error { ... }

// AnalyticsLogger owns the tuning directory and serializes appends.
type AnalyticsLogger struct {
    mu        sync.Mutex
    tuningDir string
}

func NewAnalyticsLogger(tuningDir string) *AnalyticsLogger
func (a *AnalyticsLogger) AppendEvent(sessionID string, ev Event) error
func (a *AnalyticsLogger) StartSession(assetID string) (string, error)

// newSessionID returns an RFC 4122 v4 UUID.
func newSessionID() string
```

Internal organization:

1. Constants and the `validEventTypes` map.
2. `Event` struct + `Validate()`.
3. `newSessionID()` helper.
4. `AnalyticsLogger` struct + constructor.
5. `AppendEvent` (locking, open-append-write).
6. `StartSession` (mints UUID, calls `AppendEvent` with a `session_start`
   envelope, returns the session id).

`StartSession` is implemented even though the v1 frontend mints session
ids client-side; it exists for symmetry, for unit tests, and for the
eventual case where the backend logs on its own behalf. Marked with a
godoc comment to that effect.

## `analytics_test.go` (new)

Mirrors the `settings_test.go` style: standard library only, `t.TempDir()`
for isolation, table-free unit tests with explicit names. See
design.md §"Test strategy" for the case list.

The concurrency test uses `sync.WaitGroup` and asserts the line count
plus that every line round-trips through `json.Unmarshal` (no torn
records). 50 goroutines × 20 events = 1000 lines is enough to flush out
naive non-locking implementations without dragging out `go test`.

## `main.go` (modify)

Three additions, all in `main()`:

1. Add `tuningDir := filepath.Join(workDir, "tuning")` to the dir-list.
2. Add `tuningDir` to the `for _, d := range []string{...}` `MkdirAll` loop.
3. Construct `analyticsLogger := NewAnalyticsLogger(tuningDir)` after the
   file store, before route registration.
4. Register the route:
   `mux.HandleFunc("/api/analytics/event", handleAnalyticsEvent(analyticsLogger))`

Total: ~10 lines added, no lines removed.

## `handlers.go` (modify)

One new handler appended at the end of the file (after
`handleGenerateBlenderLODs`):

```go
// handleAnalyticsEvent handles POST /api/analytics/event.
func handleAnalyticsEvent(logger *AnalyticsLogger) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        if r.Method != http.MethodPost { ... }
        var ev Event
        if err := json.NewDecoder(r.Body).Decode(&ev); err != nil { ... }
        if err := ev.Validate(); err != nil { ... }
        if ev.SessionID == "" { ... }
        if err := logger.AppendEvent(ev.SessionID, ev); err != nil { ... }
        jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
    }
}
```

No changes to existing handlers, helpers, or the routing structure for
`handleFiles`/`handleDeleteFile`. The new endpoint has a fixed path
(no `:id`), so no special-case dispatch is needed in `main.go`.

## `static/app.js` (modify)

Insert a new `// ── Analytics ──` block immediately after the existing
`// ── Asset Settings ──` block (which currently ends near `loadSettings`
/ `saveSettings`). Contents:

```js
let analyticsSessionId = null;

function startAnalyticsSession(assetId) {
    analyticsSessionId = (crypto && crypto.randomUUID)
        ? crypto.randomUUID()
        : fallbackUUID();
    logEvent('session_start', { trigger: 'open_asset' }, assetId);
    return analyticsSessionId;
}

function endAnalyticsSession(outcome, finalSettings, assetId) {
    if (!analyticsSessionId) return;
    logEvent('session_end', {
        outcome: outcome || 'leave',
        final_settings: finalSettings || null,
    }, assetId);
    analyticsSessionId = null;
}

async function logEvent(type, payload, assetId) {
    if (!analyticsSessionId) return;
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

function fallbackUUID() {
    // RFC 4122 v4 fallback for environments without crypto.randomUUID
    const b = new Uint8Array(16);
    crypto.getRandomValues(b);
    b[6] = (b[6] & 0x0f) | 0x40;
    b[8] = (b[8] & 0x3f) | 0x80;
    const h = [...b].map(x => x.toString(16).padStart(2, '0'));
    return `${h.slice(0,4).join('')}-${h.slice(4,6).join('')}-${h.slice(6,8).join('')}-${h.slice(8,10).join('')}-${h.slice(10,16).join('')}`;
}

// Expose for manual verification from devtools console.
window.startAnalyticsSession = startAnalyticsSession;
window.endAnalyticsSession = endAnalyticsSession;
window.logEvent = logEvent;
```

No `selectFile`, no `wireTuningUI`, no `saveSettings` changes. T-003-01
intentionally stops at "the helpers exist and can be called from the
console". T-003-02 will wire the lifecycle.

## `docs/knowledge/analytics-schema.md` (new)

Documents the v1 schema for human readers: envelope table, event_type
enum, per-payload schema, on-disk layout, and the migration policy
(parallel to `settings-schema.md`'s section). ~150 lines.

## Public interfaces summary

| Identifier                 | Where           | Kind    | Purpose                   |
|----------------------------|-----------------|---------|---------------------------|
| `AnalyticsSchemaVersion`   | `analytics.go`  | const   | On-disk envelope version  |
| `Event`                    | `analytics.go`  | struct  | Envelope                  |
| `(*Event).Validate`        | `analytics.go`  | method  | Server-side guard         |
| `AnalyticsLogger`          | `analytics.go`  | struct  | Owner of tuning dir + lock|
| `NewAnalyticsLogger`       | `analytics.go`  | func    | Constructor               |
| `AppendEvent`              | `analytics.go`  | method  | Append-one-line write     |
| `StartSession`             | `analytics.go`  | method  | Mint session id (Go side) |
| `handleAnalyticsEvent`     | `handlers.go`   | func    | HTTP handler              |
| `startAnalyticsSession`    | `app.js`        | fn (JS) | Mint session id (frontend)|
| `endAnalyticsSession`      | `app.js`        | fn (JS) | Emit `session_end`        |
| `logEvent`                 | `app.js`        | fn (JS) | POST envelope             |

## Ordering / dependencies

1. Create `analytics.go` first (types, validate, logger).
2. Add `analytics_test.go` and run `go test ./...`. This unblocks
   confidence before touching the wider HTTP surface.
3. Wire `main.go` (dir creation, logger construction, route).
4. Add `handleAnalyticsEvent` in `handlers.go`.
5. Add the frontend block in `app.js`.
6. Write `docs/knowledge/analytics-schema.md`.

Each step is an atomic commit. Step 2 must pass tests before step 3.

## Things this does NOT change

- Existing routes, handlers, store, settings layer.
- The on-disk layout for originals/outputs/settings (`tuning/` is a
  *new sibling*, not a reorganization).
- Any UI element. No HTML, no CSS, no listener wiring.
- The `Settings` struct (gltfpack flags) or `AssetSettings` (per-asset
  bake config). Analytics observes them; it does not own them.
- `go.mod` — no new dependencies.
