# Plan — T-003-02: session-capture-and-auto-instrumentation

## Step 1 — Backend: `LookupOrStartSession`

**Files:** `analytics.go`

- Add `assetIndex map[string]string` field to `AnalyticsLogger`.
- Initialize in `NewAnalyticsLogger`.
- Add private helper `firstEnvelope(path string) (envHead, error)`
  that opens the file, scans the first line with a 1 MiB buffer,
  unmarshals into a small struct.
- Add `LookupOrStartSession(assetID string) (string, bool, error)`.
  Cache hit → return. Otherwise scan dir sorted by mtime desc, read
  first line of each, return first match. On miss → release lock,
  call `StartSession`, re-acquire, cache, return.
- Be careful about lock ordering: `StartSession` calls `AppendEvent`
  which takes `mu`. So `LookupOrStartSession` must release `mu`
  before calling `StartSession`, then re-acquire to update the cache.

**Verify:** `go build ./...` clean.

## Step 2 — Backend: tests for `LookupOrStartSession`

**Files:** `analytics_test.go`

- `TestLookupOrStartSession_NewAsset` — empty `t.TempDir()`,
  lookup, expect `resumed=false`, JSONL created, valid `session_start`.
- `TestLookupOrStartSession_ResumesExisting` — write a JSONL with
  one valid `session_start` for asset "abc", lookup, expect
  `resumed=true` and matching session id, no new file created.
- `TestLookupOrStartSession_PicksMostRecent` — two valid jsonl
  files, `os.Chtimes` to set distinct mtimes, expect newer wins.
- `TestLookupOrStartSession_SkipsCorrupt` — write `not-json\n` as
  first line of one file, write a valid one for the same asset in
  another file with older mtime; lookup must still return the
  valid one. Optional but cheap.

**Verify:** `go test ./...` green.

## Step 3 — Backend: HTTP endpoint

**Files:** `handlers.go`, `main.go`

- Add `handleAnalyticsStartSession(logger *AnalyticsLogger)
  http.HandlerFunc` next to `handleAnalyticsEvent`.
- Method check, JSON decode `{asset_id}`, reject empty.
- Call `LookupOrStartSession`. Marshal `{session_id, resumed}`.
- Register route `/api/analytics/start-session` in `main.go`.

**Verify:** `go build ./...`, `go test ./...`. Optional: a manual
`curl -X POST -d '{"asset_id":"foo"}' http://localhost:8787/...`
sanity check is documented in `progress.md` once Step 5 lands.

## Step 4 — Frontend: rewire session helpers

**Files:** `static/app.js` lines 135–199

- Add module state: `analyticsAssetId = null`,
  `lastSettingChangeTs = null`.
- Replace `startAnalyticsSession` with async version that POSTs to
  `/api/analytics/start-session`. On success: store id and asset
  id, reset `lastSettingChangeTs`. On failure: fall back to
  client-mint UUID + best-effort `session_start` post (so the UI
  works even if backend is down).
- Simplify `endAnalyticsSession(outcome)` — read state from module
  vars, build `session_end` envelope, post via `fetch`, clear state.
- Add `endAnalyticsSessionBeacon(outcome)` — same envelope, but
  ships via `navigator.sendBeacon`. Used only by `beforeunload`.

**Verify:** Smoke load the page; `await window.startAnalyticsSession
('foo')` in devtools must hit the backend and return an id; the
file `~/.glb-optimizer/tuning/{id}.jsonl` must exist with one
`session_start` line.

## Step 5 — Frontend: wire `selectFile` + `beforeunload`

**Files:** `static/app.js`

- Top of `selectFile(id)`: if there is an active session and the
  asset is changing, call `endAnalyticsSession('switched')` first.
- Inside the existing `loadEnv.then(async () => { ... })` block,
  after `await loadSettings(id)` and before `populateTuningUI()`,
  add `await startAnalyticsSession(id)`.
- At end of file (next to existing init calls), add a
  `beforeunload` listener that fires `endAnalyticsSessionBeacon
  ('closed')` if a session is active.

**Verify:** Devtools check — load asset → JSONL contains
`session_start`. Load second asset → first JSONL gains
`session_end{outcome:"switched"}`, second JSONL gains
`session_start`. Close tab → second JSONL gains
`session_end{outcome:"closed"}` (look for sendBeacon in Network
tab; the response is fire-and-forget).

## Step 6 — Frontend: instrument tuning sliders

**Files:** `static/app.js` `wireTuningUI()` ~line 235

- Inside the `'input'` handler, capture `oldValue =
  currentSettings[spec.field]` *before* mutation.
- After existing dirty/save calls, compute `ms_since_prev`
  (rounded), update `lastSettingChangeTs`, call `logEvent
  ('setting_changed', {...}, selectedFileId)`.
- Reset button: leave alone. Out of scope.

**Verify:** Devtools — drag a slider, look in JSONL for
`setting_changed` line with correct `key`, `old_value`,
`new_value`, and `ms_since_prev` (null on the first one).

## Step 7 — Frontend: instrument regenerate buttons

**Files:** `static/app.js` `generateBillboard`,
`generateVolumetric`, `generateVolumetricLODs`,
`generateProductionAsset`

- Wrap each function body with `let success = false`, set
  `success = true` at the end of the existing `try` block, add
  a `finally` that calls `logEvent('regenerate', {trigger,
  success}, id)`.
- Trigger labels: `billboard`, `volumetric`, `volumetric_lods`,
  `production`.
- Leave button-restore (textContent / disabled) outside the
  try/catch as it already is.

**Verify:** Click each button (or just one in the manual e2e),
JSONL line for `regenerate` appears with `success:true`.

## Step 8 — Schema doc update

**Files:** `docs/knowledge/analytics-schema.md`

- §"Event types (v1)" → `setting_changed` payload table:
  add `ms_since_prev: number|null` row with description
  "milliseconds since the previous setting_changed event in this
  session, or null for the first one".
- §"Event types (v1)" → `session_end` payload table:
  extend `outcome` enum with `switched`, `closed`.
- §"Versioning and migration policy" → add a one-line note that
  these are additive, no schema bump.

## Step 9 — Manual end-to-end

Per AC: load asset → change three sliders → click Production
Asset → load another asset.

Expected JSONL contents in the first asset's file:

```
{...session_start, payload:{trigger:"open_asset"}}
{...setting_changed, payload:{key:..., old:..., new:..., ms_since_prev:null}}
{...setting_changed, payload:{...,ms_since_prev:N1}}
{...setting_changed, payload:{...,ms_since_prev:N2}}
{...regenerate,      payload:{trigger:"production", success:true}}
{...session_end,     payload:{outcome:"switched", final_settings:{...}}}
```

Document the actual `ls`/`cat` output in `progress.md`.

## Testing strategy

| Layer        | Coverage                                          |
|--------------|---------------------------------------------------|
| Go unit      | `LookupOrStartSession` × 4 cases (Step 2)         |
| Go HTTP      | None added — handler is decode/validate/append; covered manually + by existing endpoint patterns |
| JS unit      | None — project has zero JS test infra (per T-002-02 and T-003-01 reviews) |
| Manual e2e   | Step 9 — the AC verification sequence            |

The Go tests are the load-bearing automated coverage. The frontend
wiring is verified the same way the rest of `app.js` is: devtools
+ manual interaction. This is consistent with how T-003-01 was
landed and accepted.

## Out of scope reminders

- No `setting_changed` events from the reset button.
- No retry/backoff on `logEvent` failures (warn-only is fine).
- No `error` field in `regenerate` payload (success bool only).
- No accept/discard wiring (T-003-04).
- No profile id in payloads (T-003-03).
