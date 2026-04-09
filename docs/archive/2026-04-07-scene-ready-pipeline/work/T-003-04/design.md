# Design — T-003-04: accepted-tag-and-export

## Decisions summary (TL;DR)

| Question | Decision |
|---|---|
| Accepted snapshot shape | Wrapper struct embedding `*AssetSettings`, mirroring `Profile`. |
| File location | `~/.glb-optimizer/accepted/{id}.json`, thumbs in `accepted/thumbs/{id}.jpg`. |
| Versioning policy | Re-accept replaces (v1, no history). |
| HTTP shape | `POST /api/accept/:id` accepts JSON `{comment, thumbnail_b64}`; server reads current saved settings from disk and snapshots them. |
| Thumbnail upload | Base64 inside the same JSON body. Small payload (~15 KB), one round-trip, no multipart parser. |
| Where the button lives | New third `.settings-section#acceptedSection` after profiles. Visual separation matches the section's distinct purpose. |
| File-list ✓ marker | New `is_accepted` boolean on `FileRecord`, derived in `scanExistingFiles` and flipped on accept. |
| Export script language | Python, stdlib only, lives at `scripts/export_tuning_data.py`. |
| Export schema unit | One JSONL line per accepted asset (the canonical training record), with the *raw* event stream included as a nested array. Profiles get their own dedicated stream. |
| Thumbnail path in export | Relative to `~/.glb-optimizer` (e.g. `accepted/thumbs/{id}.jpg`). Documented. |
| Tests | Go unit tests for the accepted-snapshot model + handler-level test for `POST /api/accept/:id`. Python script gets a smoke `--self-test` invocation we run in CI-style by hand. |

The rest of this document expands the "why" behind each.

## Accepted snapshot shape

```go
// accepted.go
type AcceptedSettings struct {
    SchemaVersion int            `json:"schema_version"`
    AssetID       string         `json:"asset_id"`
    AcceptedAt    string         `json:"accepted_at"`     // RFC 3339 nano UTC
    Comment       string         `json:"comment"`         // optional, ≤1024 bytes
    ThumbnailPath string         `json:"thumbnail_path"`  // relative to workdir, e.g. "accepted/thumbs/{id}.jpg"
    Settings      *AssetSettings `json:"settings"`
}
```

Pinned to `SettingsSchemaVersion` (same trick `Profile` uses —
the embedded settings *are* the bulk of the on-disk shape, so
adding accepted-level fields stays additive without bumping).

**Rejected alternative — flat layout** (settings fields spread
into the top-level object). It would compress the file but
breaks the symmetry with `Profile` and `AssetSettings` and makes
schema-version inheritance murky. Wrapper wins on consistency.

## HTTP endpoint

```
POST /api/accept/{id}
Body: {"comment": "...", "thumbnail_b64": "..."}
```

**Server-side flow**:
1. Look up `id` in `FileStore`. 404 if missing.
2. `LoadSettings(id, settingsDir)` — the snapshot is the
   *currently saved* settings, not whatever the client sends.
   This avoids client/server drift and keeps the ground truth
   on disk.
3. Decode `thumbnail_b64` (data URL prefix tolerated). Cap at
   ~2 MB pre-decode to defend against pathological inputs.
   Write atomically to `accepted/thumbs/{id}.jpg`. Allow empty
   string → no thumbnail (logged but not an error; the manual
   acceptance flow may legitimately fire from a console without
   a canvas).
4. Build the `AcceptedSettings`, stamp `AcceptedAt`, atomically
   write `accepted/{id}.json`.
5. Mark `FileRecord.IsAccepted = true`.
6. Append an `accept` analytics event with the same settings
   payload, *plus* `thumbnail_path` so the export script can
   join events to thumbnails by reading the JSONL alone.
7. Return the `AcceptedSettings` JSON.

**Why server-side snapshotting** (vs. client-supplied
settings): the client already PUTs settings whenever the user
moves a slider (via `saveSettings`'s 300ms debounce). By the
time the user clicks "Mark as Accepted," disk is the
authoritative state. Reading from disk inside the handler
removes any "did the debounce flush?" race.

**Why base64 in JSON**: avoids a multipart form parser, keeps
the wire shape inspectable from `curl`, and matches the
analytics endpoint style. The size budget (256px JPEG @ 0.85
quality is ~10-20 KB raw → ~15-27 KB base64) fits well under
typical request limits.

## Frontend

**`#acceptedSection`** sits between `#profilesSection` and the
end of `.panel-right`. Contents:

```html
<div class="settings-section" id="acceptedSection">
    <h3>Accepted</h3>
    <div class="setting-row">
        <textarea id="acceptCommentInput" rows="2" placeholder="Why is this good?"></textarea>
    </div>
    <div class="setting-row">
        <button class="preset-btn" id="acceptBtn">Mark as Accepted</button>
        <span id="acceptStatus" class="accept-status"></span>
    </div>
</div>
```

`acceptBtn`'s click handler in `app.js`:

1. If no `selectedFileId`, no-op.
2. `renderer.render(scene, camera)` then capture the canvas via
   an off-screen 256px-longest-edge JPEG (`toDataURL('image/jpeg', 0.85)`).
   The fresh `render()` immediately before the read sidesteps
   the missing `preserveDrawingBuffer` flag — the buffer is
   guaranteed to be valid for that one frame.
3. POST to `/api/accept/{id}` with comment + base64 string.
4. On success: mark the file record locally as accepted,
   re-render the file list (✓ marker appears),
   show "Accepted ✓" in `#acceptStatus`,
   fire `logEvent('accept', {settings, thumbnail_path}, id)`.
5. On failure: surface the error inline in `#acceptStatus`.

**File list ✓**: `renderFileList` already builds `metaHTML`
from `f.status`, etc. We add ` <span class="accept-mark"
title="Accepted">✓</span>` when `f.is_accepted` is true. Done.

`selectFile` already calls `loadSettings` + `populateTuningUI`
on click; we add `populateAcceptedUI(id)` to the same chain to
prefill the comment textarea from `accepted/{id}.json` when one
exists (so re-accepting starts from the previous comment).

**Rejected alternative — top-of-tuning button**: makes
"Accepted" look like a tuning control. The whole point is that
"accepted" is a *state transition* on the asset, not a
parameter. Visual separation reinforces that.

## Thumbnail capture details

```js
function capturePreviewThumbnail() {
    if (!currentModel) return null; // no scene, no thumb
    renderer.render(scene, camera); // ensure buffer is fresh
    const src = previewCanvas;
    const longest = Math.max(src.width, src.height);
    const scale = Math.min(1, 256 / longest);
    const w = Math.round(src.width * scale);
    const h = Math.round(src.height * scale);
    const off = document.createElement('canvas');
    off.width = w; off.height = h;
    off.getContext('2d').drawImage(src, 0, 0, w, h);
    return off.toDataURL('image/jpeg', 0.85);
}
```

The 2D-context downsample is the standard browser-native path.
No third-party imaging libraries.

## Export script (`scripts/export_tuning_data.py`)

```
usage: export_tuning_data.py [--dir ~/.glb-optimizer] [--out -|FILE]
```

**Output is JSONL**, one record per line, in three sections
distinguished by an `kind` field:

```json
{"kind":"asset","asset_id":"abc...","filename":null,"current_settings":{...},"accepted":{"comment":"...","accepted_at":"...","thumbnail_path":"accepted/thumbs/abc.jpg","settings":{...}},"events":[...]}
{"kind":"profile","name":"round-bushes-warm","comment":"...","created_at":"...","source_asset_id":"...","settings":{...}}
{"kind":"meta","schema_version":1,"exported_at":"...","workdir":"~/.glb-optimizer","record_counts":{"assets":N,"profiles":M,"events":K}}
```

The `meta` line is emitted **last** so a streaming reader can
verify completeness from EOF. Order of asset/profile records is
sorted (asset id ascending, profile name ascending) for
deterministic diffs.

**Asset record assembly**:
1. Read `settings/{id}.json` if present (optional).
2. Read `accepted/{id}.json` if present (optional).
3. Group `tuning/*.jsonl` events by `asset_id`. Inline them
   into the asset record's `events` array, sorted by
   `timestamp` ascending.
4. Skip assets with no settings, no accepted file, AND no
   events — they have nothing to contribute.

**Why one record per asset** (vs. one per event): the ticket
explicitly frames this as ML training data
(`(asset features → accepted settings)` pairs). The natural
join key is `asset_id`. Doing the join in the export script
means the consumer can `for line in f: rec = json.loads(line)`
and immediately have `(rec["accepted"]["settings"], rec["events"])`
in hand. The raw stream is preserved inside `events` for any
analysis that needs it.

**Why also-emit profiles**: profiles are user-curated settings
the user trusted enough to name. They are potentially more
valuable than per-asset accepted snapshots because they
generalize. Cheap to include.

**Thumbnail path representation**: relative to `--dir`. The
script logs (to stderr) the absolute workdir so users can
reconstruct full paths if needed. Documented in the JSONL's
`meta` line as `workdir: "<expanded path>"`.

**Stdlib only**: `argparse`, `json`, `os`, `pathlib`, `sys`,
`datetime`. No `pandas`, no `numpy`. The script is well under
200 lines.

## Tests

### Go tests (`accepted_test.go`)

- `TestDefaultAcceptedRoundtrip` — save → load yields the same
  struct (catches field-tag drift).
- `TestSaveAccepted_StampsAcceptedAt` — empty `AcceptedAt`
  gets filled in.
- `TestAccepted_RejectsBadSettings` — embedded settings
  validation propagates.
- `TestLoadAccepted_MissingReturnsNotExist` — `fs.ErrNotExist`
  on missing file.
- `TestSaveAccepted_Overwrite` — re-save replaces in place.
- `TestThumbnailWrite_AtomicPath` — bytes land at the expected
  path with the right extension.
- `TestHandleAccept_HappyPath` — `httptest`-based: PUT settings
  first, then POST accept with a small base64 JPEG, assert
  response shape, files on disk, and analytics event written.
- `TestHandleAccept_NotFound` — unknown id → 404.
- `TestHandleAccept_BadBody` — malformed body → 400.

This is a slightly heavier test pass than T-003-03 (which
deferred handler tests entirely) because the accept endpoint
has more moving parts: it touches three on-disk locations and
emits an analytics event. A handler test is the cheapest way to
catch wiring breakage across all four touch points at once.

### Python script

A `--self-test` flag that:
1. Creates a tempdir mimicking the workdir layout.
2. Drops one settings file, one accepted file, one fake JSONL
   tuning file with two events, and one profile.
3. Runs the exporter against it.
4. Asserts the resulting JSONL has the expected number of
   asset/profile/meta records and that the asset record's
   `events` array is non-empty.

Manual run from a shell during the implement phase. Not wired
into a CI pipeline because the project has none.

## Migration to analytics-schema.md

Add a new "## Export format (v1)" section near the bottom,
documenting the JSONL shape above. Strike the
"Export script — T-003-04" bullet from the "Out of scope"
list. Append a `thumbnail_path` field row to the `accept`
event payload table (additive — no schema bump needed because
both producer and consumer change in the same ticket).

## Risks and mitigations

| Risk | Mitigation |
|---|---|
| `previewCanvas.toDataURL` returns blank because the GL backbuffer was already flipped | Always call `renderer.render(scene, camera)` immediately before reading. |
| Base64 thumbnail blows past server body limit | Cap pre-decode at 2 MB. 256px JPEG should be ~15 KB; the cap is 100x headroom. |
| User accepts an asset before settings have been PUT (the 300 ms debounce is mid-flight) | Server reads from disk → may see stale state. Acceptable: the user just clicked Accept *intentionally*; another click re-accepts. Not worth a forced flush. |
| Export script reads a partially-written JSONL | Tuning JSONL writes are O_APPEND single-write; a partial line is a torn write that the script must tolerate. Handle with try/except per line, log skips to stderr. |
| Export script crashes on malformed JSON | Per-line `try/except`, skip and warn. Same defensive posture as `ListProfiles`. |
| Thumbnail directory doesn't exist on first accept | `os.MkdirAll(acceptedDir/thumbs, 0755)` at server startup, alongside the other dirs. |

## What this design rejects

- **No history of accepts.** The ticket says "replacement is
  fine for v1." Honoring that.
- **No client-supplied settings.** The server reads from disk;
  any drift is a bug elsewhere.
- **No multipart upload.** Base64 in JSON is simpler.
- **No structured `*ValidationError` refactor.** That's
  T-003-03's flagged debt; out of scope here.
- **No CI hookup for the Python script.** Same precedent as
  other scripts in the repo.
- **No "share/upload" of the export bundle.** Out of scope per
  ticket.
