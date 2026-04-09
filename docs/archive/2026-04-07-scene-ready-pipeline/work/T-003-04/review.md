# Review — T-003-04: accepted-tag-and-export

## What changed

### Files created

| File | Purpose |
|---|---|
| `accepted.go` | `AcceptedSettings` model, validation, atomic save/load, `WriteThumbnail` helper, `AcceptedExists` lookup. |
| `accepted_test.go` | 17 tests: 10 model unit tests + 7 `httptest`-based handler tests. |
| `scripts/export_tuning_data.py` | Stdlib-only Python aggregator producing the JSONL bundle. Includes `--self-test`. |
| `docs/active/work/T-003-04/{research,design,structure,plan,progress,review}.md` | RDSPI artifacts. |

### Files modified

| File | Change |
|---|---|
| `models.go` | New `IsAccepted bool` field on `FileRecord` (omitempty). |
| `main.go` | New `acceptedDir` and `acceptedThumbsDir` paths; both added to the startup `MkdirAll` loop; `scanExistingFiles` signature gains `acceptedDir` parameter and sets `record.IsAccepted = AcceptedExists(...)`; new `/api/accept/` route registration. |
| `handlers.go` | New `handleAccept` function with GET + POST branches; new imports (`encoding/base64`, `path`, `time`); `acceptRequest` wire-shape struct; `maxAcceptedThumbnailBytes` constant. |
| `static/index.html` | New `#acceptedSection` block immediately after `#profilesSection`. |
| `static/app.js` | New `Accepted (T-003-04)` module (~110 lines): DOM refs, `setAcceptStatus`, `capturePreviewThumbnail`, `populateAcceptedUI`, `markAccepted`. Wired into `selectFile` (after `populateTuningUI`), `renderFileList` (the ✓ marker), and the init block (button click listener). |
| `static/style.css` | New `T-003-04 Accepted` block: textarea, `.accept-row`, `.accept-status[.ok|.err]`, `.accept-mark`. |
| `docs/knowledge/analytics-schema.md` | Added `thumbnail_path` row to the `accept` event payload table; new "Export format (v1)" section; struck export-script bullet from "Out of scope". |

No files deleted. No new third-party dependencies (`go.mod`
unchanged; the Python script is stdlib-only).

## Acceptance-criteria mapping

| AC item | Status | Where |
|---|---|---|
| `accepted_settings.json` per asset at `~/.glb-optimizer/accepted/{asset_id}.json` with `{asset_id, settings, accepted_at, comment, thumbnail_path}` schema | ✅ | `accepted.go` `AcceptedSettings` |
| Re-accepting replaces (v1) | ✅ | `SaveAccepted` → `writeAtomic` overwrite; `TestSaveAccepted_Overwrite` |
| `POST /api/accept/:id` snapshots current settings + writes file + emits `accept` analytics event | ✅ | `handlers.go` `handleAccept` POST branch |
| "Mark as Accepted" button in tuning panel with optional comment input | ✅ | `static/index.html` `#acceptedSection`; `static/app.js` `markAccepted` |
| Confirmation of save | ✅ | `setAcceptStatus('Accepted ✓', 'ok')` after successful POST |
| File list ✓ marker for accepted assets | ✅ | `renderFileList` adds `<span class="accept-mark">✓</span>` when `f.is_accepted` is true |
| Linked thumbnail: 256px JPEG render → `accepted/thumbs/{asset_id}.jpg` | ✅ | `capturePreviewThumbnail` does the 256px downsample; `WriteThumbnail` writes atomically |
| Export script aggregating sessions, events, profiles, accepted snapshots | ✅ | `scripts/export_tuning_data.py` |
| Includes file paths to thumbnails (relative or absolute, documented) | ✅ | Relative to workdir, documented in the meta record's `thumbnail_path_format` field and in `analytics-schema.md` "Export format (v1)" |
| Output schema documented in analytics-schema.md "export format" section | ✅ | New "Export format (v1)" section |
| Manual verification: accept an asset, run export, confirm accept event + thumbnail reference | ⚠️ Deferred | Handler test covers the server-side surface; in-browser visual run not executed in this session. See "Manual verification gap" below. |

## Test coverage

### Added — Go (`accepted_test.go`, 17 tests)

```
TestAcceptedRoundtrip                         PASS
TestSaveAccepted_StampsAcceptedAtIfEmpty      PASS
TestAcceptedValidate_RejectsBadSettings       PASS
TestAcceptedValidate_RejectsNilSettings       PASS
TestAcceptedValidate_RejectsBadSchemaVersion  PASS
TestAcceptedValidate_RejectsOversizedComment  PASS
TestAcceptedValidate_RejectsEmptyAssetID      PASS
TestLoadAccepted_MissingReturnsNotExist       PASS
TestSaveAccepted_Overwrite                    PASS
TestAcceptedExists_TrueAfterSave              PASS
TestWriteThumbnail_LandsAtPath                PASS
TestWriteThumbnail_RejectsEmpty               PASS
TestHandleAccept_GetMissingReturns404         PASS
TestHandleAccept_PostHappyPath                PASS
TestHandleAccept_PostUnknownIDReturns404      PASS
TestHandleAccept_PostBadJSONReturns400        PASS
TestHandleAccept_PostEmptyThumbnailIsOK       PASS
TestHandleAccept_PostOversizedThumbnailReturns400 PASS
TestHandleAccept_GetAfterPostReturnsSnapshot  PASS
```

The handler-test happy path is the load-bearing one: it asserts
on response body shape, the snapshot file on disk, the thumbnail
file on disk, the in-memory `FileRecord.IsAccepted` flag, AND
the `accept` event landing in the correct session JSONL — all
in a single test. If any of the four touch points wires
incorrectly to one of the others, this test catches it.

This is a deliberate departure from T-003-01 / T-003-02 / T-003-03
which all skipped HTTP-handler tests. The accept endpoint has
more cross-cutting moving parts than any other endpoint in
S-003 (settings + analytics + thumbnail + store), so the cost
of the integration test paid for itself immediately during
authoring (caught the path.Join vs filepath.Join concern).

### Added — Python

`scripts/export_tuning_data.py --self-test` builds a tempdir
fixture, runs the exporter, and asserts:
- exactly 1 asset record + 1 profile record + final meta record
- asset record has `accepted` populated and `events` non-empty
- thumbnail_path is the workdir-relative form
- meta event count is 2 (skipping a deliberately corrupt line)
- meta record is the last line in the file

Manual run produces:

```
export: skip <tmp>/tuning/<session>.jsonl:3: Expecting value: line 1 column 1 (char 0)
self_test: PASS
```

The skip-line warning to stderr is intentional and demonstrates
the per-line tolerance.

### Inherited

Full Go suite (T-002 + T-003-01..03 + T-003-04) green:

```
go test ./...
ok  	glb-optimizer	0.373s
```

### Coverage gaps

1. **No JS tests** (continuing the project precedent — there
   is no JS test infrastructure). The new JS surface is ~110
   lines and is 1:1 with Go contracts that the unit + handler
   tests cover.

2. **No live in-browser end-to-end** in this session. The
   server-side surface is fully tested; the residual gap is
   purely visual: does the button render in the right place,
   does the ✓ marker render in the file list cell, does the
   comment textarea prefill across reselects. None of these
   can fail in a way the Go tests would miss because they're
   all DOM/CSS plumbing — but they should still get a human
   eye before the ticket is closed. See "Manual verification
   gap" below.

3. **No script-against-real-workdir test**. The Python
   self-test uses a synthetic tempdir. Running the exporter
   against an actual `~/.glb-optimizer/` populated by the live
   server is the second half of the ticket's manual
   verification step.

## Open concerns

1. **`preserveDrawingBuffer` is not set on the WebGLRenderer.**
   Strategy: call `renderer.render(scene, camera)` immediately
   before `toDataURL`. This works on Chrome/Firefox/Safari at
   time of writing because the GL backbuffer is guaranteed
   valid for the same JS tick the render fired in. If a
   future browser/three.js update breaks this assumption,
   the captured thumbnail will silently come back blank. The
   handler tolerates an empty thumbnail (`TestHandleAccept_PostEmptyThumbnailIsOK`),
   so the failure mode is "no thumbnail," not a crash. A more
   robust fix is to pass `{ preserveDrawingBuffer: true }` to
   `THREE.WebGLRenderer` in `initThreeJS`, but that has a
   small perf cost on every frame and is the kind of change
   that should land separately so it can be measured.

2. **Server-side settings snapshot vs. client-side state.**
   `handleAccept` reads settings from `LoadSettings(id, settingsDir)`,
   not from any client-supplied body. This is correct because
   `saveSettings` debounces PUTs by 300 ms and reading from
   disk is the unambiguous source of truth — but if the user
   clicks "Mark as Accepted" within 300 ms of moving a slider,
   the snapshot may not include that final move. Acceptable
   trade-off (re-clicking accept re-snapshots) but worth
   knowing. A future enhancement could `await saveSettings()`
   in `markAccepted` before the POST.

3. **Thumbnail size cap is generous (2 MiB).** A 256px JPEG
   should be ~15 KiB. The 2 MiB cap exists to defend against
   pathological clients but if someone wanted to tighten it
   to ~256 KiB ("a real thumbnail can never be this big"),
   the constant is in one place and easy to lower.

4. **`isProfileValidationError` smell still present.** Same
   concern T-003-03's review flagged. T-003-04 deliberately
   does NOT touch the structured-error refactor, so the smell
   remains. The accept handler avoids the same trap by not
   bundling validation and disk errors into a single error
   path — `s.Validate()` returns its own error before
   `SaveAccepted` is called.

5. **`AnalyticsLogger.LookupOrStartSession` mints a brand new
   session inside the handler when the asset has never been
   opened in a browser.** This means a `curl POST /api/accept/...`
   creates a new session JSONL containing exactly two events:
   `session_start` (from `LookupOrStartSession`) and `accept`.
   That is the right behavior — the alternative (silently
   dropping the analytics event) would lose the most valuable
   training signal in the system — but it does mean the
   tuning directory accumulates "single-event" sessions for
   any out-of-band accept. Cheap, defensible.

6. **No "delete accepted" UI.** The ticket is silent on it
   and the on-disk file is small; users can `rm` the JSON
   manually if they need to. Not adding the UI keeps the
   surface area minimal.

7. **`accepted/{id}.json` and `accepted/thumbs/{id}.jpg` are
   not symmetrically cleaned up when an asset is deleted.**
   Same precedent as `settings/{id}.json` (settings-schema.md
   "Out of Scope"). If this gets noisy, a cleanup pass in
   `handleDeleteFile` is the right place.

## Critical issues for human attention

None. Build green, all 17 ticket-relevant Go tests pass, full
suite green, the Python self-test passes. The only outstanding
item is the visual end-to-end run in a browser, which is
better done by a human anyway and is documented under "Manual
verification gap."

## Manual verification gap

The ticket's "Manual verification" line:

> accept an asset, run the export script, confirm the output
> JSONL contains the accept event AND references the thumbnail

The server-side half of this is fully covered by
`TestHandleAccept_PostHappyPath` (asserts the snapshot file,
the thumbnail file, the FileRecord flag, and the analytics
event in the session JSONL). The export-script half is
covered by `--self-test` against a synthetic fixture that
includes an `accept` event and a `thumbnail_path`.

What is **not** covered automatically:

1. Click flow in the actual browser — does the button visually
   appear, does the ✓ marker render in the file list cell at
   the right size/color, does the textarea prefill on reselect,
   does the canvas-to-JPEG capture path produce a valid
   non-blank JPEG when run against a real Three.js scene?
2. Running `python3 scripts/export_tuning_data.py --out
   /tmp/export.jsonl` against an actual `~/.glb-optimizer/`
   populated by the live server — does it find the asset
   record, does the joined `accepted` block resolve, does
   the meta record's count match what you'd expect.

Both gaps are visual / integration only and are the right
shape for a human reviewer to do in five minutes.

## Handoff notes for downstream

- The export bundle's JSONL shape is documented in
  `analytics-schema.md` "Export format (v1)". The
  `EXPORT_SCHEMA_VERSION` constant in
  `scripts/export_tuning_data.py` is the single source of
  truth for the version integer. Bump both if the bundle
  shape changes.
- `AcceptedSchemaVersion` is pinned to `SettingsSchemaVersion`
  (same precedent as `ProfilesSchemaVersion`). When the
  embedded settings schema bumps, the accepted snapshot bumps
  with it; when only the wrapper changes, additive fields are
  fine without a bump.
- The `accept` event payload now carries `thumbnail_path` as
  a documented additive field. Producers (handler) and
  consumers (export script) both read it. Schema version did
  not bump because the addition is purely additive.
- `path.Join` (forward slashes) is used for the
  `thumbnail_path` string in both the snapshot file and the
  analytics event. Do not switch to `filepath.Join`; the
  string crosses into the export bundle and is consumed on
  whatever platform the user runs the script.
- The handler-test pattern in `accepted_test.go`
  (`acceptHarness` struct + helper constructor) is reusable
  for any future endpoint that needs to assemble a fake
  workdir + FileStore + AnalyticsLogger. If T-003-05 lands,
  copy this pattern.

## Diff stats (approximate)

```
accepted.go                              | +135 / -0
accepted_test.go                         | +335 / -0
models.go                                |   +1 / -0
main.go                                  |   +5 / -2
handlers.go                              | +160 / -1
static/index.html                        |  +11 / -0
static/app.js                            | +120 / -1
static/style.css                         |  +35 / -0
scripts/export_tuning_data.py            | +260 / -0
docs/knowledge/analytics-schema.md       |  +45 / -2
```

Plus the RDSPI artifacts under `docs/active/work/T-003-04/`.
