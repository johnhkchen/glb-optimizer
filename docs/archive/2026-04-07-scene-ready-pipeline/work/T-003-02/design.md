# Design — T-003-02: session-capture-and-auto-instrumentation

## Decision summary

Wire capture into four touchpoints with as little new code as possible:

1. **Backend gains a `LookupOrStartSession(assetID)` method** plus a
   `POST /api/analytics/start-session` endpoint. Lookup is by directory
   scan of `tuningDir/*.jsonl`, reading the first line of each to
   match `asset_id`. If found, the existing session id is returned;
   otherwise `StartSession` mints a new one. An in-memory cache keyed
   by `assetID` avoids re-scanning on every selection within one
   process.
2. **Frontend `startAnalyticsSession` becomes async** and calls the
   new endpoint. Module state (`analyticsSessionId`,
   `analyticsAssetId`, `lastSettingChangeTs`) tracks the current
   session. `endAnalyticsSession(outcome)` reads from module state.
3. **`selectFile`** ends the current session with `outcome="switched"`
   (if any) before awaiting `startAnalyticsSession(newAssetId)`. A
   `beforeunload` listener fires `endAnalyticsSession("closed")` via
   `navigator.sendBeacon` for reliable tab-close capture.
4. **`wireTuningUI`** captures `oldValue` before mutation, computes
   `ms_since_prev`, fires `setting_changed`. Each `generate*`
   function wraps its existing try/catch with a `success` flag and
   fires `regenerate` once at the end.

No refactor of the settings system. No new module. No abstraction
helper for the regenerate calls — three-line inline pattern is fine
at four call sites per first-pass scope.

## Alternatives considered

### A. Where does session minting live? (chosen: backend)

**A1. Keep client-side minting (status quo from T-003-01).**
Pros: zero backend change; no network round-trip on file selection.
Cons: AC explicitly says "calls the backend to create a session ID";
no place for server-side resume logic; T-003-01 review flagged
this as a deferred design call. **Rejected** — AC is unambiguous.

**A2. Server endpoint that always mints a new session.**
Pros: minimal backend change (just expose existing `StartSession`).
Cons: violates the "loads any previous session for the asset"
requirement. **Rejected.**

**A3. Server endpoint that resumes if a prior session exists for
the asset, otherwise mints.** (chosen)
Pros: matches AC, isolates resume logic to one place, on-disk
format is unchanged. Cons: adds a directory scan on first lookup.
At human selection rates and tens of sessions, the scan is sub-ms.

### B. How to find "the previous session for an asset"?

**B1. Scan `tuningDir/*.jsonl` and read the first line of each.**
The first line is always a `session_start` envelope with `asset_id`
populated. Cost: O(N) opens for N session files. (chosen, with cache)

**B2. Maintain a JSON index file `tuningDir/asset-sessions.json`.**
Pros: O(1) lookup. Cons: another file, another schema, another
write/lock, drift risk between index and JSONL. Out of proportion
to the read frequency. **Rejected.**

**B3. Encode `asset_id` into the JSONL filename.**
Pros: trivial lookup via `Glob`. Cons: changes the on-disk format
(T-003-01 schema doc commits to `{session_id}.jsonl`); breaks the
T-003-04 export glob assumption. **Rejected.**

Chosen: **B1 + an in-memory map** (`map[string]string`,
guarded by the existing logger mutex). The map is populated lazily
on first lookup of an asset and updated whenever a new session is
minted. On process restart we re-scan, which costs one `ReadDir` +
N `Open`+`ReadLine` ops the first time each asset is opened — fine.
We do *not* eagerly scan on startup because most asset selections
won't happen at all.

### C. Selecting the latest session when an asset has multiple

A user could end up with multiple sessions per asset over time (e.g.
if T-003-04 archives or expires sessions and a new one is started).
For first-pass we pick the **most recently modified** `.jsonl` file
whose first line matches. Tiebreak by lex order (deterministic).
This is the single line in `LookupOrStartSession` that does the
sort. Documenting it here so a reviewer doesn't have to guess.

### D. Where in `selectFile` should `endAnalyticsSession` fire?

**D1. After `loadSettings` completes.** Risk: if `loadSettings`
throws (it shouldn't — it has its own try/catch), end is skipped.
Also, end-of-A would be written *after* start-of-B in wall-clock
order, which is confusing in chronological exports. **Rejected.**

**D2. Synchronously at the very top of `selectFile`, before
`selectedFileId = id`.** (chosen) The end of asset A goes to A's
JSONL with the timestamp of the click; then the new session is
started for B. Order is correct.

### E. `setting_changed` payload shape

AC: `{key, old_value, new_value}` plus the timestamp delta.
Choices for delta name: `ms_since_prev`, `delta_ms`, `dt_ms`.
Picked **`ms_since_prev`** because it is unambiguous (vs. "delta
since what?") and reads naturally in the JSONL. Type: number for
deltas, `null` on the first setting_changed of a session. The
frontend tracks `lastSettingChangeTs` (a `performance.now()`
reading) module-side; resets to `null` when the session changes.

### F. `regenerate` success detection

AC: `{trigger, success}`. Each `generate*` function already has
`try { ... } catch (err) { console.error }`. The minimal change is:

```js
let success = false;
try { ... ; success = true; } catch (err) { ... }
finally { logEvent('regenerate', { trigger, success }, assetId); }
```

`success = true` is set after the upload fetch resolves and the
local store is updated; if any earlier step throws, success stays
false. This matches the user-visible "did the asset actually
change?" interpretation.

### G. Tab-close capture

`fetch()` started in a `beforeunload` handler is cancelled by the
browser before the request leaves the network stack. The reliable
primitive is `navigator.sendBeacon(url, blob)` — fire-and-forget,
queued by the browser, survives unload.

`endAnalyticsSession` is therefore split: the normal path posts via
`fetch`, and a `beforeunload` listener calls a separate small
helper that constructs the same envelope but ships it via
`sendBeacon`. The cost is ~12 lines.

## Why not refactor

A clean abstraction would be `instrument(name, fn)` wrapping each
generate function and `instrumentSetting(spec)` wrapping each
slider. Both are tempting. Both are explicitly out of scope per
the ticket: "Don't refactor the settings system to make
instrumentation cleaner — just add the capture calls where they're
needed. We'll iterate on the abstractions in a follow-up if it
gets noisy."

We will revisit when (a) a fifth `generate*` function lands, or
(b) `setting_changed` payloads need shared enrichment (e.g.
profile id from T-003-03).

## Risk register

| Risk                                              | Mitigation                                  |
|---------------------------------------------------|---------------------------------------------|
| Lost `session_end` on tab close                   | `sendBeacon` path                           |
| Resume picks the wrong session for an asset       | Sort by mtime, deterministic tiebreak       |
| `LookupOrStartSession` reads a corrupted JSONL    | Skip files whose first line is not valid JSON; log and continue |
| Large tuning dirs slow first lookup               | In-memory cache after first scan; bounded by N sessions per process |
| Frontend `startAnalyticsSession` becoming async breaks an existing caller | Only existing callers are devtools ad-hoc and the new `selectFile` wiring |
