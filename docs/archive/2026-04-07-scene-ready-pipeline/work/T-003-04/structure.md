# Structure — T-003-04: accepted-tag-and-export

## Files created

| Path | Purpose |
|---|---|
| `accepted.go` | `AcceptedSettings` struct, validation, atomic save/load, thumbnail write helper. |
| `accepted_test.go` | Unit tests for the model + handler test for `POST /api/accept/:id`. |
| `scripts/export_tuning_data.py` | Python stdlib-only exporter producing the JSONL bundle. |

## Files modified

| Path | Change |
|---|---|
| `models.go` | Add `IsAccepted bool` field to `FileRecord` (omitempty json tag). |
| `main.go` | Create `acceptedDir` + `acceptedThumbsDir`; `MkdirAll`; pass `acceptedDir` into `scanExistingFiles`; register `POST /api/accept/` route. |
| `handlers.go` | New `handleAccept` function. New imports (`encoding/base64`) if not already present. Update `scanExistingFiles` signature (in `main.go`, but the helper to derive `IsAccepted` lives next to `SettingsExist` in `accepted.go`). |
| `static/index.html` | Add `#acceptedSection` block after `#profilesSection`. |
| `static/app.js` | Add DOM refs, `capturePreviewThumbnail`, `markAccepted`, `populateAcceptedUI`; wire `acceptBtn` listener; call `populateAcceptedUI` from `selectFile`; render ✓ marker in `renderFileList`. |
| `static/style.css` | Style `.accept-mark`, `.accept-status`, `#acceptedSection` button row. |
| `docs/knowledge/analytics-schema.md` | Add "Export format (v1)" section; add `thumbnail_path` row to the `accept` payload table; strike export-script bullet from "Out of scope". |

No file deletions.

## `accepted.go` interface

```go
package main

const AcceptedSchemaVersion = SettingsSchemaVersion // pinned, see profiles.go precedent

const acceptedCommentMaxLen = 1024

type AcceptedSettings struct {
    SchemaVersion int            `json:"schema_version"`
    AssetID       string         `json:"asset_id"`
    AcceptedAt    string         `json:"accepted_at"`
    Comment       string         `json:"comment"`
    ThumbnailPath string         `json:"thumbnail_path"`
    Settings      *AssetSettings `json:"settings"`
}

func (a *AcceptedSettings) Validate() error
func AcceptedFilePath(id, dir string) string
func AcceptedThumbPath(id, dir string) string
func AcceptedExists(id, dir string) bool
func LoadAccepted(id, dir string) (*AcceptedSettings, error)
func SaveAccepted(a *AcceptedSettings, dir string) error
func WriteThumbnail(id, thumbsDir string, jpegBytes []byte) error
```

`Validate()`:
- `SchemaVersion == AcceptedSchemaVersion`
- `AssetID != ""`
- `len(Comment) <= acceptedCommentMaxLen`
- `Settings != nil` and `Settings.Validate()` returns nil

`SaveAccepted` stamps `AcceptedAt` if empty (mirrors `SaveProfile`),
calls `Validate()`, then `writeAtomic` (defined in `settings.go`).

`WriteThumbnail` writes via temp+rename in the thumbs dir. The caller
(handler) computes the relative thumbnail path string for the
JSON snapshot.

## `handleAccept` flow

```
POST /api/accept/{id}
Content-Type: application/json
Body: { "comment": string, "thumbnail_b64": string }

200 → AcceptedSettings JSON
400 → bad body / decode error / oversize thumbnail
404 → unknown asset id
500 → disk write failure
```

Steps inside the handler:

1. Method check.
2. Strip prefix → `id`. If empty → 400.
3. `store.Get(id)` → 404 if missing.
4. JSON decode `{comment, thumbnail_b64}`.
5. `LoadSettings(id, settingsDir)` → fail → 500.
6. Validate decoded settings (defensive; LoadSettings doesn't
   validate per its existing contract).
7. Decode base64 thumbnail (tolerate `data:image/jpeg;base64,`
   prefix). If non-empty: enforce ≤2 MB raw, write via
   `WriteThumbnail`. Capture `relThumbPath`.
8. Build `AcceptedSettings` and `SaveAccepted`.
9. `store.Update` → `IsAccepted = true`.
10. `analyticsLogger.AppendEvent` with an `accept` envelope.
    Use the existing `LookupOrStartSession(id)` so the event
    lands in the right session JSONL even if the request
    arrives outside an active client session.
11. Respond 200 with the saved snapshot.

The handler signature mirrors the others:

```go
func handleAccept(
    store *FileStore,
    settingsDir, acceptedDir, acceptedThumbsDir string,
    logger *AnalyticsLogger,
) http.HandlerFunc
```

## `main.go` wiring

```go
acceptedDir := filepath.Join(workDir, "accepted")
acceptedThumbsDir := filepath.Join(acceptedDir, "thumbs")

for _, d := range []string{originalsDir, outputsDir, settingsDir, tuningDir, profilesDir, acceptedDir, acceptedThumbsDir} {
    ...
}

scanExistingFiles(store, originalsDir, outputsDir, settingsDir, acceptedDir)

mux.HandleFunc("/api/accept/", handleAccept(store, settingsDir, acceptedDir, acceptedThumbsDir, analyticsLogger))
```

`scanExistingFiles` gains a parameter and a single `record.IsAccepted = AcceptedExists(id, acceptedDir)` line.

## Frontend module boundaries

The right panel is the only UI surface. The new code lives in a
single contiguous block in `app.js` for grep-ability:

```js
// ── Accepted (T-003-04) ──
const acceptCommentInput = document.getElementById('acceptCommentInput');
const acceptBtn          = document.getElementById('acceptBtn');
const acceptStatus       = document.getElementById('acceptStatus');

function capturePreviewThumbnail() { ... }
async function populateAcceptedUI(id) { ... }
async function markAccepted() { ... }

acceptBtn.addEventListener('click', markAccepted);
```

`populateAcceptedUI` is invoked from `selectFile` immediately
after `populateTuningUI()`. It does a `GET /api/accept/{id}`
(or, more frugally, reads `f.is_accepted` first and only fetches
when true) to prefill the comment textarea and the status label.

Adding a `GET /api/accept/{id}` is cheap and symmetric with other
endpoints; the structure decision: yes, also add `GET` so the
frontend can prefill without parsing the file list. The `GET`
branch is a 5-line addition in `handleAccept`.

So `handleAccept` becomes a switch on method:

```go
case http.MethodGet:
    a, err := LoadAccepted(id, acceptedDir)
    if errors.Is(err, fs.ErrNotExist) { 404 }
    else if err != nil { 500 }
    else { 200 + json }
case http.MethodPost:
    // existing flow
```

## File-list ✓ marker

In `renderFileList` (`static/app.js`), inside the per-file loop,
right after the existing `metaHTML` assembly:

```js
if (f.is_accepted) {
    metaHTML += ` <span class="accept-mark" title="Accepted">✓</span>`;
}
```

CSS:

```css
.accept-mark {
    color: var(--accent-good, #6c6);
    font-weight: bold;
    margin-left: 4px;
}
```

## Export script structure

```python
#!/usr/bin/env python3
"""Aggregate ~/.glb-optimizer tuning artifacts into a single JSONL bundle."""

import argparse, json, os, sys
from datetime import datetime, timezone
from pathlib import Path

EXPORT_SCHEMA_VERSION = 1

def expand_workdir(s: str) -> Path: ...
def load_json(p: Path) -> dict | None: ...
def iter_jsonl(p: Path): ...

def collect_assets(workdir: Path) -> list[dict]: ...
def collect_profiles(workdir: Path) -> list[dict]: ...
def build_meta(workdir: Path, assets, profiles, total_events) -> dict: ...

def main(argv): ...
def self_test(): ...

if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
```

`collect_assets`:
- Walks `originals/*.glb` to enumerate ids.
- For each id: load `settings/{id}.json`, `accepted/{id}.json`.
- After the walk, scans `tuning/*.jsonl` once and bucket-sorts
  events by `asset_id`. Single pass over the directory.
- Yields `{kind: "asset", asset_id, current_settings, accepted, events}`.

The "events first / assets after" pass means we never hold every
event in memory twice. For the expected single-user volumes (a
few hundred sessions, a few thousand events) this is a millisecond
job; the structure exists for clarity, not performance.

Output is written to `--out FILE` (default stdout). `--self-test`
runs the smoke check and exits 0/1.

## Ordering of changes

The implementation phase will execute these in this order
(detailed in `plan.md`):

1. `accepted.go` + tests pass on its own (model only).
2. `models.go` field, `main.go` wiring, `handlers.go` handler.
3. Handler test passes.
4. `static/index.html` + `static/app.js` + `static/style.css`.
5. `scripts/export_tuning_data.py` + self-test.
6. `docs/knowledge/analytics-schema.md` updates.

Each step is independently committable. The Go layers are wired
top-to-bottom so the build is green at every commit boundary.
