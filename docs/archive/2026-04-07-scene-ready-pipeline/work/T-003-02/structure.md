# Structure — T-003-02: session-capture-and-auto-instrumentation

## Files touched

| File                                  | Action  | Net Δ (est) |
|---------------------------------------|---------|-------------|
| `analytics.go`                        | modify  | +60         |
| `analytics_test.go`                   | modify  | +90         |
| `handlers.go`                         | modify  | +25         |
| `main.go`                             | modify  | +1          |
| `static/app.js`                       | modify  | +75 / -25   |
| `docs/knowledge/analytics-schema.md`  | modify  | +20         |
| `docs/active/work/T-003-02/*.md`      | create  | RDSPI       |

No new source files. No deletions.

## `analytics.go` — additions

Add to `AnalyticsLogger`:

```go
type AnalyticsLogger struct {
    mu         sync.Mutex
    tuningDir  string
    assetIndex map[string]string // assetID -> sessionID, lazy
}
```

The map is initialized in `NewAnalyticsLogger` (`make(map[string]string)`).
Reads/writes happen under `mu`.

New method:

```go
// LookupOrStartSession returns the session id for the given asset.
// If a JSONL file with a session_start envelope for that asset_id
// already exists in the tuning dir, its session id is returned with
// resumed=true. Otherwise a new session is minted via StartSession
// and resumed=false.
func (a *AnalyticsLogger) LookupOrStartSession(assetID string) (id string, resumed bool, err error)
```

Algorithm:

1. Lock `mu`.
2. If `assetIndex[assetID]` is set, return it with `resumed=true`.
3. `os.ReadDir(tuningDir)`. For each `*.jsonl` entry, sorted by
   `ModTime` descending then by name:
   a. Open, read first line via `bufio.Scanner` with a generous buffer.
   b. `json.Unmarshal` into a local struct `{EventType, AssetID,
      SessionID string}`. On any error, continue.
   c. If `EventType == "session_start" && AssetID == assetID`, set
      `assetIndex[assetID] = SessionID`, return with `resumed=true`.
4. Not found. Unlock; call `a.StartSession(assetID)` (which re-locks
   for its own append). Re-lock; cache the result; return with
   `resumed=false`.

Helper: a small private `firstEnvelope(path) (envHead, error)` to
keep `LookupOrStartSession` readable. `envHead` is `struct {
EventType string; AssetID string; SessionID string }` matching only
the fields we care about.

## `handlers.go` — additions

```go
// handleAnalyticsStartSession handles POST /api/analytics/start-session.
// Body: {"asset_id":"..."}.
// Response: {"session_id":"...","resumed":true|false}.
func handleAnalyticsStartSession(logger *AnalyticsLogger) http.HandlerFunc
```

- Method check: POST only, else 405.
- Decode `{ AssetID string \`json:"asset_id"\` }`. Reject empty
  asset_id with 400.
- Call `logger.LookupOrStartSession(assetID)`. On error, 500.
- Respond with `{session_id, resumed}` JSON.

## `main.go` — one line

Add `mux.HandleFunc("/api/analytics/start-session",
handleAnalyticsStartSession(analyticsLogger))` next to the existing
`/api/analytics/event` registration.

## `static/app.js` — modifications

### Module state (additions near the analytics block, ~line 145)

```js
let analyticsSessionId = null;
let analyticsAssetId = null;     // NEW: which asset the current session belongs to
let lastSettingChangeTs = null;  // NEW: performance.now() of last setting_changed
```

### `startAnalyticsSession(assetId)` — rewritten

Becomes async. POSTs to `/api/analytics/start-session` with
`{asset_id}`. On success sets `analyticsSessionId`, `analyticsAssetId`,
resets `lastSettingChangeTs = null`. Returns the session id. Falls
back to client-mint UUID + best-effort `session_start` post on
network failure (so the UI never breaks if the backend is down).

### `endAnalyticsSession(outcome)` — simplified signature

Drops `finalSettings`/`assetId` params; reads from module state.
Builds and POSTs the `session_end` envelope with payload
`{outcome, final_settings: currentSettings || null}`. Clears
`analyticsSessionId`, `analyticsAssetId`, `lastSettingChangeTs`.

### `endAnalyticsSessionBeacon(outcome)` — new helper

Uses `navigator.sendBeacon('/api/analytics/event', new Blob([JSON],
{type:'application/json'}))`. Used only by `beforeunload`. Same
envelope shape as the fetch path.

### `wireTuningUI()` — modify each input handler

Inside the existing handler (post `parse`, before
`currentSettings[spec.field] = v`):

```js
const oldValue = currentSettings[spec.field];
currentSettings[spec.field] = v;
// ...existing dirty + saveSettings calls...
const now = performance.now();
const msSincePrev = lastSettingChangeTs == null ? null
    : Math.round(now - lastSettingChangeTs);
lastSettingChangeTs = now;
logEvent('setting_changed', {
    key: spec.field,
    old_value: oldValue,
    new_value: v,
    ms_since_prev: msSincePrev,
}, selectedFileId);
```

Reset button: also fires `setting_changed` for each field that
changed? No — out of scope. The reset button currently calls
`saveSettings(selectedFileId)`; we leave it untouched. (A `reset`
event type is not in the v1 schema, and rapid-fire per-field
events would be misleading anyway.)

### `selectFile(id)` — modify

At the very top, before `selectedFileId = id`:

```js
if (analyticsSessionId && analyticsAssetId !== id) {
    endAnalyticsSession('switched');
}
```

After `await loadSettings(id)` (so we have a valid asset to attach
the new session to):

```js
await startAnalyticsSession(id);
```

This sits inside the existing `loadEnv.then(async () => { ... })`
chain right before `populateTuningUI()`.

### `beforeunload` listener — new (near `wireTuningUI()` call site, end of file)

```js
window.addEventListener('beforeunload', () => {
    if (analyticsSessionId) endAnalyticsSessionBeacon('closed');
});
```

### `generate*` functions — wrap each in `success`/`finally`

For each of `generateBillboard`, `generateVolumetric`,
`generateVolumetricLODs`, `generateProductionAsset`:

```js
let success = false;
try {
    // ...existing body...
    success = true;
} catch (err) { console.error('...', err); }
finally {
    logEvent('regenerate', { trigger: '<label>', success }, id);
}
// existing button-restore code stays after the try/catch
```

Trigger labels: `billboard`, `volumetric`, `volumetric_lods`,
`production`.

## `analytics_test.go` — additions

Three new test functions:

1. `TestLookupOrStartSession_NewAsset` — empty dir; lookup mints a
   new session, file appears, returns `resumed=false`.
2. `TestLookupOrStartSession_ResumesExisting` — pre-create a JSONL
   with a `session_start` line for asset "abc"; lookup returns the
   existing session id with `resumed=true`; no new file is created.
3. `TestLookupOrStartSession_PicksMostRecent` — two existing
   sessions for same asset, different mtimes; the newer one wins.

Optional fourth: `TestLookupOrStartSession_SkipsCorrupt` — a file
with a non-JSON first line is silently ignored.

All tests use `t.TempDir()` and a fresh `NewAnalyticsLogger`.

## `analytics-schema.md` — additions

Two small additions in §"Event types (v1)":

- `setting_changed` payload gains `ms_since_prev: number|null`.
- `session_end` payload `outcome` enum extended with `switched` and
  `closed` (alongside the existing values).

Note in §"Versioning and migration policy" that these are additive,
so `schema_version` stays at 1.

## Ordering of changes (commit boundaries)

1. Backend: `analytics.go` + tests + `handlers.go` + `main.go`
   route. Compiles, tests pass, no UI behavior change.
2. Frontend: rewire `startAnalyticsSession` to call backend; wire
   `selectFile` + `beforeunload`; instrument `wireTuningUI` and
   `generate*`. Manual e2e check.
3. Schema doc update.

Each step is independently committable and revertable. The frontend
step is the only one that produces user-visible behavior changes.
