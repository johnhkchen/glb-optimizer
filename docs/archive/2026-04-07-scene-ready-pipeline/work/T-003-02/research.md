# Research — T-003-02: session-capture-and-auto-instrumentation

## Ticket recap

Wrap the existing settings + regenerate flows from S-002 with automatic
analytics capture. T-003-01 already shipped the storage layer (envelope,
JSONL writer, HTTP endpoint, and frontend `logEvent` helper). T-003-02
must wire those helpers into the actual UI lifecycle so events appear
on disk without each control knowing about analytics.

## Map of what already exists

### Backend (T-003-01)

- `analytics.go` — `Event` envelope, `Validate()`, `AnalyticsLogger`
  with `AppendEvent(sessionID, ev)` and `StartSession(assetID) → id`.
  `StartSession` mints a new UUID, writes a `session_start` envelope
  with `payload.trigger="open_asset"`, returns the id.
  - Process-wide mutex + `O_APPEND|O_CREATE|O_WRONLY` per write. JSONL
    files live at `tuningDir/{session_id}.jsonl`.
- `analytics_test.go` — 12 tests covering envelope validation, single
  + concurrent appends, and `StartSession`. The concurrency test runs
  50 goroutines × 20 events.
- `handlers.go:976` — `handleAnalyticsEvent` decodes the envelope,
  validates, appends. Returns `{"status":"ok"}` or 4xx/5xx.
- `main.go:59` — `tuningDir := filepath.Join(workDir, "tuning")`,
  `MkdirAll`'d at startup, then passed to `NewAnalyticsLogger`. Route
  `/api/analytics/event` registered at `main.go:127`.

### Frontend (T-003-01)

`static/app.js` lines 135–199 already define:

- `let analyticsSessionId = null;` — module-level current session.
- `startAnalyticsSession(assetId)` — mints UUID **client-side** via
  `crypto.randomUUID()` (or `fallbackUUID()`), writes `session_start`,
  returns the id. **Synchronous; does not call the backend to mint.**
- `endAnalyticsSession(outcome, finalSettings, assetId)` — writes
  `session_end` and clears `analyticsSessionId`.
- `logEvent(type, payload, assetId)` — POSTs an envelope to
  `/api/analytics/event`. No-ops if `analyticsSessionId` is null.
- `fallbackUUID()` for environments without `crypto.randomUUID`.
- `window.startAnalyticsSession`, `window.endAnalyticsSession`,
  `window.logEvent` exposed for devtools-driven manual checks.

These helpers are inert in v1 — nothing in the rest of `app.js`
calls them. The T-003-01 review explicitly handed off the wiring as
"T-003-02 just needs to wire it."

### Settings flow (T-002-03, the surface to instrument)

- `app.js:29` — `let currentSettings = null;` — per-asset bake/tuning
  settings, populated by `loadSettings(id)`.
- `app.js:81` `loadSettings(id)` — `GET /api/settings/{id}` → assigns
  to `currentSettings`. Falls back to `applyDefaults()` on failure.
- `app.js:93` `saveSettings(id)` — debounced (300ms) `PUT
  /api/settings/{id}` with the full `currentSettings` body. **This
  is the single chokepoint for persisting tuning changes.**
- `app.js:205` `TUNING_SPEC` — declarative list of 11 fields, each
  with id, parser, formatter.
- `app.js:231` `wireTuningUI()` — runs once at module init. For each
  spec, attaches an `'input'` listener that:
  1. parses value, mutates `currentSettings[spec.field]`,
  2. updates the displayed value + dirty dot,
  3. calls `saveSettings(selectedFileId)` (debounced PUT).
  Also wires the reset button.
- `app.js:219` `populateTuningUI()` — runs after every `loadSettings`
  to push values into the DOM controls.

The "input" handler is the *exact* place where a `setting_changed`
event must fire. The handler currently does not see the *previous*
value once it has parsed the new one, but the previous value is
still in `currentSettings[spec.field]` *until* the assignment on
line 238. Capturing `oldValue` before that line costs one local var.

### Selection lifecycle (T-001-04 + T-002-03)

- `app.js:2275` `selectFile(id)` — sets `selectedFileId`, resets
  preview, then `loadEnv.then(async () => { await loadSettings(id);
  populateTuningUI(); loadModel(...); })`. There is **no** existing
  hook for "switching away from a file". Switching is just a second
  `selectFile(otherId)` call.
- `app.js:1526` — `div.onclick = () => selectFile(f.id)` is the only
  caller in normal flow. The first `selectFile` after page load also
  goes through this.
- There is no `beforeunload` listener anywhere in `app.js` — `grep`
  for `beforeunload`/`visibilitychange` returns nothing.

### Regenerate buttons (T-001-03 / T-001-05 / S-002)

Four async functions, each tied to a button:

| function                       | trigger label  | location           |
|--------------------------------|----------------|--------------------|
| `generateBillboard(id)`        | `billboard`    | `app.js:463`       |
| `generateVolumetric(id)`       | `volumetric`   | `app.js:744`       |
| `generateVolumetricLODs(id)`   | `volumetric_lods` | `app.js:972`    |
| `generateProductionAsset(id)` | `production`   | `app.js:1000`      |

Each follows the same shape: disable button → `try { ... } catch (err)
{ console.error(...); }` → re-enable button. None track success/failure
explicitly — failure is "we logged and the file did not change."
There is currently no shared post-generation hook.

## Constraints and assumptions

1. **First-pass scope.** Ticket §"First-Pass Scope" says: do not
   refactor the settings system. Add the capture calls inline. Any
   abstraction (a `withRegenerate(label, fn)` wrapper, a settings
   diff helper) is out of scope.
2. **Schema v1 is locked.** No envelope changes. New payload fields
   are additive and documented in `analytics-schema.md`.
3. **Single-user local tool.** No multi-tab coordination, no XHR
   reliability heroics. `sendBeacon` for tab close is the one place
   this matters because `fetch` is cancelled on unload.
4. **Backend session minting required.** AC §1 explicitly says
   "calls the backend to create a session ID". T-003-01 frontend
   currently mints client-side. The T-003-01 review §"Open concerns"
   #1 anticipated this: a server-mint endpoint would be "purely
   additive on top of v1 — no on-disk schema impact".
5. **Resumable sessions.** AC §"selectFile loads any previous
   session for the asset (or starts a new one if none exists)"
   implies a per-asset persistent session: switching files writes
   `session_end` (outcome=switched), but coming back resumes the
   same `session_id` and appends to the same JSONL. The export
   pipeline (T-003-04) is what finally closes them.
6. **Timestamp delta.** AC §"include the timestamp delta from the
   previous setting change in the same session" → tracked client-side
   in a module variable, written into the `setting_changed` payload
   as `ms_since_prev` (null on first change of session).
7. **Dependencies.** Only T-003-01. No third-party libs.

## Open questions for Design

- How to look up "the previous session for an asset" on the backend.
  Options: scan tuning dir + read first line of each `.jsonl`; keep
  an in-memory map; persist an index file. Tradeoffs in `design.md`.
- Whether `endAnalyticsSession('switched')` should be fired *before*
  or *after* `startAnalyticsSession(newId)`. Ordering matters for
  the JSONL: switched-end of asset A must be in A's file, not B's.
- Whether `regenerate.success=false` should also include an `error`
  string. Out of scope per AC payload spec; defer.
