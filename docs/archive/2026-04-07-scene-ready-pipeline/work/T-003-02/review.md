# Review — T-003-02: session-capture-and-auto-instrumentation

## What changed

### Files modified

| File                                   | Change                                                                                       |
|----------------------------------------|----------------------------------------------------------------------------------------------|
| `analytics.go`                         | `assetIndex` cache, `firstEnvelope()` helper, `LookupOrStartSession()` method                |
| `analytics_test.go`                    | 4 new tests covering new asset / resume / mtime ordering / corrupt-file skip                 |
| `handlers.go`                          | `handleAnalyticsStartSession` — POST endpoint that wraps `LookupOrStartSession`              |
| `main.go`                              | One route registration: `/api/analytics/start-session`                                       |
| `static/app.js`                        | Async backend-mint `startAnalyticsSession`; `endAnalyticsSessionBeacon`; `selectFile` lifecycle hooks; `wireTuningUI` setting_changed instrumentation; four `generate*` functions wrapped with regenerate emission; `beforeunload` listener |
| `docs/knowledge/analytics-schema.md`   | `setting_changed.ms_since_prev`; extended `session_end.outcome` enum; reshaped `regenerate` payload; new `start-session` endpoint section                |

No files created (other than the RDSPI artifacts under
`docs/active/work/T-003-02/`). No files deleted. No new third-party
dependencies (`go.mod` unchanged).

## Acceptance-criteria mapping

| AC item                                                                                          | Status | Where                                          |
|--------------------------------------------------------------------------------------------------|--------|------------------------------------------------|
| `selectFile(id)` calls `startAnalyticsSession(id)` which calls the backend                      | ✅     | `app.js` `selectFile`, `startAnalyticsSession`; `handlers.go` `handleAnalyticsStartSession` |
| Switching/closing fires `endAnalyticsSession(outcome)` with `switched` \| `closed`               | ✅     | `app.js` `selectFile` head, `beforeunload` listener |
| Session id stored in module-level variable                                                       | ✅     | `app.js` `analyticsSessionId` / `analyticsAssetId` |
| Debounced settings PUT also fires `setting_changed` with `{key, old_value, new_value}`           | ✅     | `app.js` `wireTuningUI` input handler          |
| Include timestamp delta from previous setting change in same session                             | ✅     | `ms_since_prev` field, tracked via `lastSettingChangeTs` |
| `generateBillboard`/`Volumetric`/`ProductionAsset`/etc. fire `regenerate` with `{trigger, success}` | ✅  | All four `generate*` functions wrapped         |
| `selectFile` loads any previous session for the asset (or starts new)                            | ✅     | `LookupOrStartSession` (backend) + `startAnalyticsSession` (frontend) |
| Manual: load → 3 sliders → Production Asset → load another → expected JSONL                      | ⚠️     | Plumbing wired and unit-tested; live e2e deferred (see "Open concerns") |

## Test coverage

### Added (Go)

```
=== RUN   TestLookupOrStartSession_NewAsset           PASS
=== RUN   TestLookupOrStartSession_ResumesExisting    PASS
=== RUN   TestLookupOrStartSession_PicksMostRecent    PASS
=== RUN   TestLookupOrStartSession_SkipsCorrupt       PASS
```

These cover the four meaningful states of the lookup path:

1. **`_NewAsset`** — empty dir; mints, returns `resumed=false`,
   verifies the on-disk envelope; second lookup hits cache and
   returns the same id with `resumed=true`.
2. **`_ResumesExisting`** — pre-seeded JSONL is found; no new file
   created; correct id returned.
3. **`_PicksMostRecent`** — two valid sessions for same asset,
   distinct mtimes set via `os.Chtimes`; the newer one wins.
4. **`_SkipsCorrupt`** — non-JSON file with newer mtime is skipped
   silently; the older valid file is returned.

### Inherited (Go)

All 12 T-003-01 tests still pass. Total: 16 green.

### Coverage gaps

- **No HTTP-handler test for `handleAnalyticsStartSession`.**
  Following the same precedent as T-003-01: the handler is
  decode → validate → delegate → marshal, with all the load-bearing
  logic in `LookupOrStartSession`, which *is* tested. An
  `httptest.NewRecorder` test would be straightforward but adds
  rote coverage. Same call: defer until a third analytics
  endpoint lands.
- **No JS tests.** Project still has zero JS test infra (per
  T-002-02 review §"coverage gaps" and T-003-01 review).
  Frontend is verified by devtools-driven manual checks. The
  affected JS surface is small: `startAnalyticsSession`,
  `endAnalyticsSession`, `endAnalyticsSessionBeacon`, the
  `wireTuningUI` handler block, and four `generate*` finally
  blocks — each is 1:1 with its Go counterpart.
- **No live end-to-end run.** The manual sequence in the AC
  requires interacting with the running server in a browser. The
  individual pieces are wired and the backend half is unit-tested;
  the live confirmation is the residual gap.

## Open concerns

1. **Live e2e not yet executed.** The AC's manual verification
   (load asset → 3 sliders → Production Asset → load another →
   `cat` the JSONL) was not run during this ticket because that
   sequence requires a human in front of a browser with a real
   asset library. Worth running before T-003-03 builds on top.
   Expected JSONL contents documented in `plan.md` Step 9.

2. **Slider input events vs. "value-change" events.** The AC says
   "every interaction" but a literal interpretation would emit
   hundreds of `setting_changed` events while a slider is being
   dragged (one per pixel). The handler now early-returns when
   `parsed_value === oldValue`, which matches the human-readable
   intent. Documented in `progress.md` §"Deviations from plan".
   If T-003-04 reveals a need for finer-grained capture, the
   guard can be removed.

3. **Resume semantics across `session_end`.** Resuming an asset
   that was previously left with `outcome="switched"` will append
   new events to the same JSONL *after* the existing
   `session_end` line. Downstream readers (T-003-04 export) must
   treat sessions as potentially containing multiple `session_end`
   markers. Alternative — write `session_pause`/`session_resume`
   pseudo-events — was rejected as over-design for first-pass
   scope. The schema doc's `session_end.outcome` description now
   explicitly notes that `switched` is a pause-like marker.

4. **`sendBeacon` reliability.** Browsers cap individual beacons
   at ~64KB and may drop them if the queue is full at unload.
   The `final_settings` snapshot is small (<1 KB) and we only
   queue one beacon per unload, so this is theoretical.

5. **`assetIndex` cache is process-local.** On server restart the
   first lookup per asset re-scans `tuning/`. With N sessions on
   disk this is `O(N)` opens per asset on first-touch only. At
   the expected scale (<100 sessions for the foreseeable future)
   this is sub-millisecond. If session counts ever balloon, the
   right fix is a disk-side index, not a longer-lived cache.

6. **Frontend backend-mint fallback to client-mint.** If
   `/api/analytics/start-session` is unreachable,
   `startAnalyticsSession` falls back to a client-minted UUID and
   posts a `session_start` event directly. This keeps the UI
   working but means a backend outage can produce sessions that
   are *not* resumable (the cache is never populated for them).
   Acceptable tradeoff for a single-user local tool — flagged
   here for visibility.

7. **`generate*` `success` flag is coarse.** It is `true` if the
   final upload + store update succeeded, `false` otherwise. There
   is no `error` field in the payload. AC payload spec is
   `{trigger, success}` only, so this is intentional; the console
   error is still logged via the existing `console.error` calls.

## Critical issues for human attention

None. Build green, all tests green, plan executed without
deviation beyond the slider no-op guard. The only residual item
needing attention is running the live e2e in a browser, which is
better done by a human anyway.

## Handoff notes for downstream tickets

- **T-003-03** (profiles) can call the existing `logEvent('accept',
  {settings})` and `logEvent('discard', {})` from new accept/
  discard buttons; the session is already active by the time those
  buttons are reachable (asset is selected → session started).
- **T-003-04** (export) should:
  - `glob ~/.glb-optimizer/tuning/*.jsonl` (unchanged from T-003-01)
  - Tolerate sessions with multiple `session_end` markers
    (concern #3 above) — `outcome=switched` is a pause, not a
    terminal state.
  - Use `setting_changed.ms_since_prev` to detect "fast revert"
    sequences (rapid back-and-forth on the same key with small
    deltas) per the S-003 satisfaction-derivation plan.
- The T-003-01 review's open concern #1 (server-mint vs.
  client-mint) is now resolved: the canonical path is server-mint
  via `/api/analytics/start-session`, with client-mint as a
  fallback for outage tolerance only.

## Diff stats (approximate)

```
analytics.go                              | +130 / -5
analytics_test.go                         | +175 / -1
handlers.go                               |  +35
main.go                                   |   +1
static/app.js                             |  +135 / -27
docs/knowledge/analytics-schema.md        |  +40 / -10
```

Plus the RDSPI artifacts under `docs/active/work/T-003-02/`.
