# Plan — T-003-04: accepted-tag-and-export

## Step-by-step

### Step 1 — `accepted.go` (model + I/O)

Create `accepted.go` with:
- `AcceptedSchemaVersion = SettingsSchemaVersion`
- `acceptedCommentMaxLen = 1024`
- `AcceptedSettings` struct
- `Validate`, `AcceptedFilePath`, `AcceptedThumbPath`,
  `AcceptedExists`, `LoadAccepted`, `SaveAccepted`,
  `WriteThumbnail`

**Verify**: `go build ./...` succeeds.

### Step 2 — `accepted_test.go` (model tests)

Tests:
- `TestAcceptedRoundtrip` — save → load yields the same struct.
- `TestSaveAccepted_StampsAcceptedAtIfEmpty`.
- `TestValidate_RejectsBadSettings`.
- `TestValidate_RejectsNilSettings`.
- `TestValidate_RejectsBadSchemaVersion`.
- `TestValidate_RejectsOversizedComment`.
- `TestLoadAccepted_MissingReturnsNotExist`.
- `TestSaveAccepted_Overwrite`.
- `TestWriteThumbnail_LandsAtPath`.
- `TestAcceptedExists_TrueAfterSave`.

**Verify**: `go test ./... -run TestAccepted` green.

### Step 3 — `models.go` field + `main.go` wiring

- Add `IsAccepted bool` to `FileRecord` (omitempty).
- In `main.go`: build `acceptedDir` and `acceptedThumbsDir`,
  `MkdirAll` both, pass `acceptedDir` into `scanExistingFiles`.
- Update `scanExistingFiles` signature + body to set
  `record.IsAccepted = AcceptedExists(id, acceptedDir)`.

**Verify**: `go build ./...` green; `go test ./...` green.

### Step 4 — `handleAccept` in `handlers.go`

Implement the GET/POST switch as described in `structure.md`:
- GET: `LoadAccepted` → 200/404/500.
- POST: decode body, lookup file, load settings from disk,
  validate, decode thumbnail (≤2 MB), `WriteThumbnail`, build
  `AcceptedSettings`, `SaveAccepted`, mark store, append
  analytics `accept` event using `LookupOrStartSession`,
  return 200.

Add the `encoding/base64` import.

Register the route in `main.go`:

```go
mux.HandleFunc("/api/accept/", handleAccept(store, settingsDir, acceptedDir, acceptedThumbsDir, analyticsLogger))
```

**Verify**: `go build ./...` green.

### Step 5 — Handler tests in `accepted_test.go`

Use `httptest`:
- `TestHandleAccept_GetMissingReturns404`
- `TestHandleAccept_PostHappyPath`
  - Setup: temp workdir, `FileStore` with one file, write
    a settings file, build `AnalyticsLogger` against the
    temp tuning dir.
  - POST `{comment, thumbnail_b64}` with a tiny valid base64
    JPEG (a 3-byte placeholder is fine — the handler doesn't
    validate JPEG bytes).
  - Assert: 200, response decodes to `AcceptedSettings`,
    file at `accepted/{id}.json` exists, file at
    `accepted/thumbs/{id}.jpg` exists, the in-memory
    `FileRecord.IsAccepted` is `true`, exactly one `accept`
    line lands in the corresponding session JSONL.
- `TestHandleAccept_PostUnknownIDReturns404`
- `TestHandleAccept_PostBadJSONReturns400`
- `TestHandleAccept_PostOversizedThumbnailReturns400`
- `TestHandleAccept_PostEmptyThumbnailIsOK` — empty `thumbnail_b64`
  is allowed; `ThumbnailPath` ends up empty.

**Verify**: full `go test ./...` green.

### Step 6 — Frontend HTML + CSS

- Add `#acceptedSection` block to `static/index.html`
  immediately after `#profilesSection`.
- Add CSS rules in `static/style.css` for `.accept-mark`,
  `.accept-status`, and the `#acceptedSection` button row.

**Verify**: hand-load the page; section renders without
console errors.

### Step 7 — Frontend JS

In `static/app.js`:
- Add DOM refs near the existing profiles refs.
- Implement `capturePreviewThumbnail` (returns data URL or `''`).
- Implement `populateAcceptedUI(id)` — fetch `/api/accept/{id}`,
  on 200 prefill comment + show "Accepted ✓"; on 404 clear them.
- Implement `markAccepted` — capture thumbnail, POST, on
  success patch `files[i].is_accepted = true`, re-render file
  list, fire `logEvent('accept', {settings, thumbnail_path}, id)`,
  show "Accepted ✓"; on failure show error inline.
- Wire `acceptBtn.addEventListener('click', markAccepted)`.
- Add the `is_accepted` ✓ marker line in `renderFileList`.
- Call `populateAcceptedUI(id)` from `selectFile` after
  `populateTuningUI()`.

**Verify**: hand-test in browser. Click Accept on a selected
asset → ✓ appears, comment persists across reselect.

### Step 8 — Export script

Create `scripts/export_tuning_data.py` per the structure
outlined in `structure.md`. Implementation notes:

- Use `pathlib.Path` throughout.
- Tolerate missing subdirectories (a fresh install has no
  `accepted/` or `tuning/` until something happens).
- Per-line `try/except json.JSONDecodeError` in `iter_jsonl`,
  warn to stderr, continue.
- `--self-test` builds a tempdir, drops fixture files, runs
  the exporter against it, asserts the JSONL has the right
  shape. Print PASS/FAIL and exit accordingly.

**Verify**: `python3 scripts/export_tuning_data.py --self-test`
prints PASS and exits 0.

### Step 9 — Doc updates

Edit `docs/knowledge/analytics-schema.md`:

1. In the `accept` event section, add a `thumbnail_path` row
   to the payload table.
2. Add a new top-level "## Export format (v1)" section
   documenting the JSONL bundle (asset / profile / meta lines,
   thumbnail-path-is-relative rule, ordering, sentinel meta
   line at EOF).
3. Strike the "- Export script — T-003-04" bullet from the
   "Out of scope (deferred)" list.

**Verify**: the section renders cleanly on visual inspection.

### Step 10 — Manual end-to-end (the ticket's verification)

1. `go run .` to launch the server.
2. Upload a `.glb`.
3. Move some sliders.
4. Click "Mark as Accepted" with a short comment.
5. Confirm:
   - File list shows ✓ next to the file.
   - `~/.glb-optimizer/accepted/{id}.json` exists with the
     comment, settings snapshot, and a `thumbnail_path`.
   - `~/.glb-optimizer/accepted/thumbs/{id}.jpg` exists and
     opens as a real image.
   - The session JSONL contains an `accept` event with the
     thumbnail path embedded.
6. Run `python3 scripts/export_tuning_data.py --out /tmp/export.jsonl`.
7. Confirm `/tmp/export.jsonl` contains an asset record for
   that id with `accepted` populated and `events` non-empty,
   plus a final `meta` line.

If the e2e cannot be run interactively in the Implement
session, document the gap in `progress.md` and `review.md`
under "manual verification deferred."

## Testing strategy

| Layer | Strategy | Where |
|---|---|---|
| Accepted model | Go unit tests | `accepted_test.go` |
| HTTP handler | Go `httptest` integration | `accepted_test.go` |
| Analytics integration | Verified inside the happy-path handler test by reading the session JSONL after the POST | `accepted_test.go` |
| Frontend | Manual browser run | `progress.md` notes |
| Export script | Python `--self-test` smoke | runs from CLI |
| End-to-end | Manual sequence in Step 10 | `progress.md` |

JS is uncovered by automated tests, same as every prior S-003
ticket. The new JS surface is ~60 lines and is 1:1 with the
Go contracts the unit/handler tests cover, so the residual
risk is wiring (does the button actually fire?), which the
manual e2e catches.

## Commit boundaries

1. `accepted.go` + `accepted_test.go` (model only).
2. `models.go` `IsAccepted` field + `main.go` wiring.
3. `handleAccept` + handler tests + route registration.
4. Frontend HTML/CSS/JS.
5. Export script + self-test.
6. Doc updates (`analytics-schema.md`).

Each commit leaves the build and tests green. Each commit is
small enough to revert atomically if a downstream commit reveals
a problem in an upstream one.

## Out-of-scope reminders (do not creep)

- No accept history.
- No multipart upload.
- No structured `*ValidationError` refactor.
- No Python script CI hookup.
- No export upload or sharing.
- No "delete accepted" UI.
- No backwards-compat shims for the new `IsAccepted` field
  in the JSON wire format (it is additive and `omitempty`).
