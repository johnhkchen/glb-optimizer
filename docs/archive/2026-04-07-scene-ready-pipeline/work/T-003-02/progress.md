# Progress — T-003-02: session-capture-and-auto-instrumentation

## Steps completed

- [x] Step 1 — `analytics.go`: added `assetIndex` cache,
      `firstEnvelope()` helper, `LookupOrStartSession()`. Lock
      discipline: hold mu for the dir scan, release before
      `StartSession` (which re-takes mu via `AppendEvent`),
      re-acquire to update the cache.
- [x] Step 2 — `analytics_test.go`: 4 new tests
      (`_NewAsset`, `_ResumesExisting`, `_PicksMostRecent`,
      `_SkipsCorrupt`). All green. Imports gained `time`.
- [x] Step 3 — `handlers.go`: added
      `handleAnalyticsStartSession`. `main.go`: registered
      `/api/analytics/start-session` route.
- [x] Step 4 — `static/app.js`: rewrote `startAnalyticsSession`
      to be async and call the backend. Added `analyticsAssetId`,
      `lastSettingChangeTs` module state. Added
      `endAnalyticsSessionBeacon` for tab-close. Network failure
      falls back to client-mint UUID + best-effort post.
- [x] Step 5 — `static/app.js`: `selectFile` now ends the prior
      session with `outcome="switched"` (when asset id changes)
      and awaits `startAnalyticsSession(id)` after `loadSettings`.
      Added `beforeunload` listener at end of file.
- [x] Step 6 — `static/app.js`: `wireTuningUI` input handler
      captures `oldValue` before mutation, computes
      `ms_since_prev`, fires `setting_changed`. Added a
      no-op-if-equal early-return so identical re-emits from
      slider input events don't generate noise.
- [x] Step 7 — `static/app.js`: `generateBillboard`,
      `generateVolumetric`, `generateVolumetricLODs`,
      `generateProductionAsset` each gained
      `let success = false` / `try ... success = true ... finally
      logEvent('regenerate', {trigger, success}, id)`.
      Trigger labels: `billboard`, `volumetric`, `volumetric_lods`,
      `production`.
- [x] Step 8 — `docs/knowledge/analytics-schema.md`:
      `setting_changed` payload table gained `ms_since_prev`.
      `session_end` `outcome` enum extended with `switched` and
      `closed`. `regenerate` payload table reshaped around the
      new `{trigger, success}` AC. New §"`POST /api/analytics/
      start-session`" section. Out-of-scope bullet for T-003-02
      struck through.

## Verification

### Go build / test

```
$ go build ./...
$ go test -run LookupOrStartSession -v ./...
=== RUN   TestLookupOrStartSession_NewAsset
--- PASS: TestLookupOrStartSession_NewAsset (0.00s)
=== RUN   TestLookupOrStartSession_ResumesExisting
--- PASS: TestLookupOrStartSession_ResumesExisting (0.00s)
=== RUN   TestLookupOrStartSession_PicksMostRecent
--- PASS: TestLookupOrStartSession_PicksMostRecent (0.00s)
=== RUN   TestLookupOrStartSession_SkipsCorrupt
--- PASS: TestLookupOrStartSession_SkipsCorrupt (0.00s)
PASS
ok  	glb-optimizer	0.319s

$ go test ./...
ok  	glb-optimizer
```

Full suite (all 16 tests: 12 from T-003-01 + 4 new) green.

### Manual end-to-end (deferred)

The interactive AC verification — load asset → change three sliders
→ click Production Asset → load another asset → confirm JSONL
contents — requires a running server and a browser session. The
plumbing is wired and unit-tested at the API level; flagging this
as the one residual confirmation step in `review.md`.

## Deviations from plan

None. Implementation followed `plan.md` step by step. The only
mid-implementation refinement was the no-op-if-equal early-return
in the slider handler (Step 6) — added because slider `'input'`
events fire on every pixel of drag and would otherwise emit
hundreds of identical setting_changed events with `ms_since_prev=0`.
The AC says "every interaction in the tuning UI fires a
`setting_changed` event" but interpreted as "every value change",
which matches the human-readable intent of the per-event payload.

## Files modified

```
analytics.go                              | +130 / -5
analytics_test.go                         | +175 / -1
handlers.go                               |  +35
main.go                                   |   +1
static/app.js                             |  +135 / -27
docs/knowledge/analytics-schema.md        |  +40 / -10
docs/active/work/T-003-02/*.md            | RDSPI artifacts
```
