# Progress — T-003-04

All plan steps executed in one session. Build green, full test
suite green.

## Steps completed

- [x] Step 1 — `accepted.go` created (model + I/O).
- [x] Step 2 — `accepted_test.go` created with model tests.
  - Two test names (`TestValidate_RejectsBadSettings`,
    `TestValidate_RejectsNilSettings`) collided with names already
    declared in `profiles_test.go`. Renamed to
    `TestAcceptedValidate_*` to disambiguate. Same prefix applied
    to the schema-version, oversized-comment, and empty-asset-id
    cases for consistency.
- [x] Step 3 — `models.go` `IsAccepted` field added; `main.go`
  wired (acceptedDir, acceptedThumbsDir, MkdirAll loop entry,
  `scanExistingFiles` signature + body, route registration).
- [x] Step 4 — `handleAccept` added to `handlers.go` with both
  GET and POST branches. New imports: `encoding/base64`, `path`,
  `time`. Uses `LookupOrStartSession` to land the analytics
  event in the right session JSONL even for non-browser callers
  (curl, tests).
- [x] Step 5 — Handler tests written using `httptest`. All seven
  cases pass: missing-GET-404, post-happy-path (asserts on-disk
  files, in-memory `IsAccepted` flag, AND the analytics event in
  the session JSONL), unknown-id-404, bad-JSON-400, empty-thumb-OK,
  oversized-thumb-400, get-after-post.
- [x] Step 6 — `static/index.html` `#acceptedSection` added after
  `#profilesSection`. `static/style.css` got a `T-003-04 Accepted`
  block at the bottom (`#acceptedSection textarea`, `.accept-row`,
  `.accept-status[.ok|.err]`, `.accept-mark`).
- [x] Step 7 — `static/app.js` got the Accepted module
  (`capturePreviewThumbnail`, `populateAcceptedUI`, `markAccepted`,
  status helper). Wired into `selectFile` (after `populateTuningUI`),
  `renderFileList` (the ✓ marker), and the init block at the
  bottom (`acceptBtn` click listener).
- [x] Step 8 — `scripts/export_tuning_data.py` created. Stdlib
  only. `--self-test` invocation passes locally. Notably the
  self-test fixture intentionally includes a malformed JSONL line
  to exercise the per-line tolerance — the exporter logs the skip
  to stderr and continues, the meta record reports the correct
  count of valid events (2).
- [x] Step 9 — `docs/knowledge/analytics-schema.md` updated:
  added `thumbnail_path` row to the `accept` payload table,
  added a new "Export format (v1)" section, struck the export
  script bullet from the "Out of scope" list.
- [ ] Step 10 — Manual end-to-end browser run not performed in
  this session. The handler test exercises the full server-side
  surface (snapshot file, thumbnail file, FileRecord flag, and
  the analytics event in the session JSONL all in one happy
  path), so the residual gap is purely visual: does the button
  exist, does the ✓ render in the file list, does the comment
  textarea prefill correctly across reselects? See review.md
  "Manual verification gap".

## Deviations from the plan

1. **Test name collision** — described above. Renaming was the
   minimum-impact fix and keeps the precedent that profile vs.
   accepted tests are distinguished by the `Profile`/`Accepted`
   prefix.

2. **`AnalyticsLogger.LookupOrStartSession` mints a session if
   none exists** — this means the handler test happy-path
   correctly observes a freshly-created session JSONL even
   though no client `start-session` was issued. The plan
   anticipated this; flagging it here so future readers know
   the handler does not depend on a pre-existing session.

3. **Thumbnail path is forward-slash joined via `path.Join` in
   the handler**, not `filepath.Join`. The path stored on disk
   in the snapshot JSON and emitted in the `accept` event needs
   to be portable across OSes (the export script reads it on
   any platform); using `path` instead of `filepath` keeps
   forward slashes on Windows-style systems. Documented inline
   in the handler comment.

4. **Test for the GET-after-POST case** was added (not in the
   original plan list) to cover the prefill round-trip the
   frontend depends on. Cheap, useful, kept it.

5. **Oversized-thumbnail check is double-gated** — pre-decode
   length check based on `len(b64)*3/4`, then a post-decode
   check on the actual bytes. The pre-check rejects before the
   `base64.Decode` allocation; the post-check is a safety net.
   Saves us from allocating a 5 GB buffer if a hostile client
   sends a 6.7 GB string.

6. **Frontend `populateAcceptedUI` does its own `fetch` instead
   of trusting the file list's `is_accepted` flag** — this is
   slightly more bytes on the wire but means the comment
   textarea prefills with the actual stored comment, not just
   "yes/no accepted." Net positive.

7. **The export script `--self-test` does not assert on the
   stderr "skip" warning** — the malformed line is intentional
   in the fixture but the assertion only counts valid events.
   That keeps the test resilient against future changes to the
   warning string.

## Test results

```
go test ./...
ok  	glb-optimizer	0.373s
```

```
go test ./... -run Accept -v
... 17 PASS lines ...
PASS
ok  	glb-optimizer	0.340s
```

```
python3 scripts/export_tuning_data.py --self-test
export: skip <tmp>/tuning/<session>.jsonl:3: Expecting value: line 1 column 1 (char 0)
self_test: PASS
```
