# Analytics Event Schema (v1)

This document is the canonical reference for the tuning analytics event
format produced by the glb-optimizer frontend and backend. Events are
written one per line as JSON to `~/.glb-optimizer/tuning/{session_id}.jsonl`.

The schema is **versioned**. Once events land on disk under a given
`schema_version`, that version's envelope shape is frozen forever — readers
in the export pipeline (T-003-04) and any downstream ML training code
depend on it being stable. New event types and new payload fields can be
added within v1 as long as the addition is backwards-compatible (additive
only, never renames or removes).

The current version is **v1**, defined by the constant
`AnalyticsSchemaVersion = 1` in `analytics.go`.

## Envelope

Every event is a JSON object with exactly these top-level fields:

| Field            | Type     | Required | Description                                       |
|------------------|----------|----------|---------------------------------------------------|
| `schema_version` | integer  | yes      | Always `1` for the v1 envelope.                   |
| `event_type`     | string   | yes      | One of the enum below. Unknown values are rejected. |
| `timestamp`      | string   | yes      | RFC 3339 UTC, e.g. `2026-04-07T12:34:56.789Z`. The client (browser or backend) stamps this at the moment the underlying user action occurred — **not** the server's receive time. |
| `session_id`     | string   | yes      | UUID v4. All events for one tuning session share the same id. |
| `asset_id`       | string   | yes      | The file id the event refers to. May be empty for events that don't pertain to a specific asset (none in v1). |
| `payload`        | object   | yes      | Event-specific contents. Must always be a JSON object — use `{}` for empty payloads. |

The envelope is enforced server-side: `(*Event).Validate()` rejects
mismatched `schema_version`, unknown `event_type`, empty `timestamp`, empty
`session_id`, and `nil` payloads with HTTP 400. Payload contents are
**not** introspected at the wire layer — payload schemas below are
documentation, not validation.

## Event types (v1)

### `session_start`

Emitted exactly once at the beginning of a tuning session. The frontend
emits this when the user opens an asset for tuning; the backend's
`StartSession` helper emits this when minting a session id Go-side.

```json
{
  "trigger": "open_asset"
}
```

| Field     | Type   | Required | Notes                                        |
|-----------|--------|----------|----------------------------------------------|
| `trigger` | string | yes      | Free-form. v1 emitters use `"open_asset"`.   |

### `session_end`

Emitted exactly once at the end of a tuning session — when the user
accepts, discards, or leaves the asset.

```json
{
  "outcome": "accept",
  "duration_ms": 12345,
  "final_settings": { "schema_version": 1, "...": "..." }
}
```

| Field            | Type    | Required | Notes                                                      |
|------------------|---------|----------|------------------------------------------------------------|
| `outcome`        | string  | yes      | `accept` \| `discard` \| `leave` \| `switched` \| `closed`. `switched` = user opened a different asset (T-003-02). `closed` = browser tab unload, sent via `navigator.sendBeacon`. |
| `duration_ms`    | integer | no       | Wall-clock duration since `session_start`.                 |
| `final_settings` | object  | no       | Snapshot of `AssetSettings` at the moment the session ended. |

### `setting_changed`

Emitted every time the user commits a single tunable parameter — slider
release, select change, etc. Each event covers exactly one field.

```json
{
  "key": "bake_exposure",
  "old_value": 1.0,
  "new_value": 1.25,
  "ms_since_prev": 842
}
```

| Field           | Type            | Required | Notes                                    |
|-----------------|-----------------|----------|------------------------------------------|
| `key`           | string          | yes      | Field name from `AssetSettings`.         |
| `old_value`     | number/string   | yes      | Previous value (matches the field type). |
| `new_value`     | number/string   | yes      | New value.                               |
| `ms_since_prev` | number \| null  | no       | Milliseconds since the previous `setting_changed` in this session, rounded. `null` for the first change of a session. T-003-04 uses this to detect "fast revert" patterns. |

### `regenerate`

Emitted whenever the user triggers a bake / optimize action.

```json
{
  "trigger": "production",
  "success": true
}
```

| Field            | Type    | Required | Notes                                                                     |
|------------------|---------|----------|---------------------------------------------------------------------------|
| `trigger`        | string  | yes      | Which generate action fired this. T-003-02 emits `billboard`, `volumetric`, `volumetric_lods`, `production`. T-009-01 adds `billboard_tilted` (devtools-only entry point — no toolbar button in v1). |
| `success`        | boolean | yes      | `true` if the generate function ran to completion (upload accepted, store updated); `false` if any step threw. |
| `output_glb`     | string  | no       | Reserved. Path to the produced GLB, relative to the workdir.              |
| `thumbnail_path` | string  | no       | Reserved for the S-003 thumbnail-linking work (T-003-04).                 |

### `accept`

Emitted when the user marks the current settings as the canonical
"accepted profile" for an asset. This is the most valuable training signal
captured by the system — see S-003 §"Tagged final settings".

```json
{
  "settings": { "schema_version": 1, "...": "..." }
}
```

| Field            | Type   | Required | Notes                                                       |
|------------------|--------|----------|-------------------------------------------------------------|
| `settings`       | object | yes      | Full `AssetSettings` snapshot at accept time.               |
| `thumbnail_path` | string | no       | Workdir-relative path to the captured 256px JPEG, e.g. `accepted/thumbs/{asset_id}.jpg`. Empty when no thumbnail was captured. Added in T-003-04. |

### `discard`

Emitted when the user discards the current session and reverts.

```json
{ "reason": "" }
```

| Field    | Type   | Required | Notes                                     |
|----------|--------|----------|-------------------------------------------|
| `reason` | string | no       | Optional free-form note. Empty in v1.     |

### `profile_saved`

Emitted when the user saves the current `AssetSettings` as a named
profile via the Profiles section of the tuning panel (T-003-03).

```json
{ "profile_name": "round-bushes-warm" }
```

| Field          | Type   | Required | Notes                                                |
|----------------|--------|----------|------------------------------------------------------|
| `profile_name` | string | yes      | Kebab-case, 1–64 chars. Server-validated on save.    |

### `classification`

Emitted whenever the shape classifier (T-004-02) runs against an
asset — both on upload (auto-classify) and on explicit
`POST /api/classify/:id` requests. This event is critical training
data for the S-004 strategy router (T-004-03).

```json
{
  "category": "round-bush",
  "confidence": 0.83,
  "features": {
    "n_points": 12345,
    "dimensions": [1.0, 0.9, 1.1],
    "ratios": {"r2": 0.92, "r3": 0.47},
    "axis_alignment": 0.93,
    "mean_peakiness": 2.17,
    "...": "..."
  }
}
```

| Field        | Type   | Required | Notes                                                                            |
|--------------|--------|----------|----------------------------------------------------------------------------------|
| `category`   | string | yes      | One of the `shape_category` enum values (see settings-schema.md).                |
| `confidence` | number | yes      | Softmax confidence in `[0,1]`.                                                   |
| `features`   | object | yes      | Opaque feature dump from the classifier — includes PCA eigenvalues / eigenvectors, dimensions, axis alignment, peakiness. Schema is intentionally not validated server-side; consumers should treat unknown fields gracefully. T-004-04 added `features.candidates`: a top-N list of `{category, score}` entries sorted descending by softmax score, surfaced for the comparison-UI modal. The hard-surface overlay always wins (score `1.0`) when set. The field is additive and optional — older event lines may not carry it. |

### `classification_override`

Emitted by the Go side when the user picks a strategy from the
multi-strategy comparison UI (T-004-04). The split between this
event and the plain `classification` event is intentional:
downstream training treats `classification_override` as a
high-value labeled training example linking
`(asset features → human-preferred strategy)`, while a solo
`classification` event with a high confidence is a low-value
confirmation.

The override path re-runs the classifier so `features` carries the
*current* geometry, not whatever was on disk. The
`original_category` and `original_confidence` fields preserve the
classifier's pre-override pick for downstream "would the model
have agreed?" analysis.

```json
{
  "original_category": "planar",
  "original_confidence": 0.42,
  "candidates": [
    {"category": "planar",       "score": 0.42},
    {"category": "directional",  "score": 0.31},
    {"category": "tall-narrow",  "score": 0.20}
  ],
  "chosen_category": "directional",
  "features": {
    "ratios": {"r2": 0.46, "r3": 0.05},
    "axis_alignment": 0.92,
    "...": "..."
  }
}
```

| Field                 | Type   | Required | Notes                                                                                |
|-----------------------|--------|----------|--------------------------------------------------------------------------------------|
| `original_category`   | string | yes      | The category the classifier picked before the override.                              |
| `original_confidence` | number | yes      | The classifier's softmax confidence in `original_category`, in `[0,1]`.              |
| `candidates`          | array  | yes      | The candidate ranking the user picked from. Same shape as `classification.features.candidates`: `[{category, score}, ...]`, sorted descending. May be `null` if the classifier feature dump did not include a ranking (e.g. legacy classifier output). |
| `chosen_category`     | string | yes      | One of the `shape_category` enum values. Persisted to `AssetSettings.shape_category` and stamped through the strategy router. |
| `features`            | object | yes      | Same opaque feature dump as in `classification` events.                              |

### `scene_template_selected`

Emitted by the JS side when the user changes the active scene
preview template via the toolbar picker (T-006-02). Only fired on
actual change (`from !== to`); the count input and ground plane
toggle do NOT emit their own events — they snapshot into the
payload here, and the next `session_end` carries the resting
values via `final_settings`.

```json
{
  "from": "grid",
  "to": "mixed-bed",
  "instance_count": 50,
  "ground_plane": true
}
```

| Field            | Type   | Required | Notes                                                                            |
|------------------|--------|----------|----------------------------------------------------------------------------------|
| `from`           | string | yes      | The previous template id. One of the keys in `validSceneTemplates` (settings.go). |
| `to`             | string | yes      | The new template id. Different from `from`.                                      |
| `instance_count` | number | yes      | Snapshot of the count input at change time, integer in `[1, 500]`.               |
| `ground_plane`   | bool   | yes      | Snapshot of the ground plane toggle at change time.                              |

### `prepare_for_scene`

Emitted by the JS side when the user clicks the **Prepare for scene**
primary action (T-008-01) and the orchestrator finishes — either after
all stages succeed, or after the first stage failure stops the pipeline.
Exactly one event per click. Per-stage `regenerate` events continue to
fire from inside the underlying generate functions; this event is the
higher-level summary on top.

```json
{
  "stages_run": ["gltfpack", "classify", "lods", "production"],
  "total_duration_ms": 18432,
  "success": true
}
```

| Field               | Type             | Required | Notes                                                                                              |
|---------------------|------------------|----------|----------------------------------------------------------------------------------------------------|
| `stages_run`        | array of string  | yes      | Stage ids that actually ran, in order. Skipped stages (e.g. `gltfpack` when the file is already optimized, `classify` when `shape_confidence > 0`) are omitted. v1 stage ids: `gltfpack`, `classify`, `lods`, `production`. |
| `total_duration_ms` | integer          | yes      | Wall-clock duration of the orchestrator run, including skipped-stage no-ops.                       |
| `success`           | boolean          | yes      | `true` if every executed stage reported success; `false` if any stage failed and the pipeline stopped. |
| `failed_stage`      | string           | no       | Present only when `success === false`. The stage id that failed.                                   |
| `error`             | string           | no       | Present only when `success === false`. Short human-readable error message.                         |

### `strategy_selected`

Emitted by the Go side immediately after every `classification` event,
both on upload-time auto-classify and on explicit
`POST /api/classify/:id` requests. Captures which bake / orientation
strategy the S-004 router (T-004-03) picked for the classified
category — paired with the `classification` event, this is the
audit trail for "what did the system decide and what did the user
do about it" downstream training analysis.

```json
{
  "category": "directional",
  "strategy": {
    "category": "directional",
    "slice_axis": "auto-horizontal",
    "slice_count": 4,
    "slice_distribution_mode": "equal-height",
    "instance_orientation_rule": "fixed",
    "default_budget_priority": "mid"
  }
}
```

| Field      | Type   | Required | Notes                                                              |
|------------|--------|----------|--------------------------------------------------------------------|
| `category` | string | yes      | One of the `shape_category` enum values (see settings-schema.md).  |
| `strategy` | object | yes      | The full `ShapeStrategy` returned by `getStrategyForCategory`. Field set is fixed at v1: `category`, `slice_axis`, `slice_count`, `slice_distribution_mode`, `instance_orientation_rule`, `default_budget_priority`. New fields may be added additively without a schema bump. |

### `profile_applied`

Emitted when the user applies a saved profile to the current asset
(T-003-03). The profile's settings overwrite the asset's per-asset
settings before the event fires.

```json
{ "profile_name": "round-bushes-warm" }
```

| Field          | Type   | Required | Notes                                                |
|----------------|--------|----------|------------------------------------------------------|
| `profile_name` | string | yes      | Kebab-case, 1–64 chars.                              |

## Storage layout

```
~/.glb-optimizer/
    tuning/
        00000000-0000-4000-8000-000000000000.jsonl
        ...
```

One file per session. The directory is created at startup by `main.go`
alongside `originals/`, `outputs/`, and `settings/`. Each file is
append-only and contains one JSON envelope per line, terminated by `\n`,
with no trailing comma, no header, and no footer.

Readers should treat the file as a stream of newline-delimited JSON
objects (`for line in f: json.loads(line)`).

## Concurrency and durability

Appends are serialized in-process by a single `sync.Mutex` and use
`os.O_APPEND|O_CREATE|O_WRONLY` against a freshly opened handle on every
write. POSIX guarantees atomicity for `O_APPEND` writes up to `PIPE_BUF`
(4096 bytes); a tuning envelope is far smaller than that, so partial /
torn lines are not possible from in-process writers. The handle is closed
after each write — no long-lived file descriptors, no rotate semantics to
reason about.

If the process crashes mid-write, the worst case is that the in-flight
event's bytes are not written; all prior bytes on disk are intact.

## HTTP capture API

### `POST /api/analytics/event`

Body: a single JSON envelope as described above.

Responses:

- `200 OK` `{"status":"ok"}` — accepted and appended.
- `400 Bad Request` — envelope failed `Validate()` (bad version, unknown
  type, missing required field).
- `405 Method Not Allowed` — non-POST.
- `500 Internal Server Error` — disk write failed.

There is no batching, no rate limiting, no authentication. The endpoint
is single-user, local, same-origin only.

### `POST /api/analytics/start-session`

Added in T-003-02. Body: `{"asset_id":"<file id>"}`.

Resumes the most recent session for the given asset if one exists on
disk, otherwise mints a new session (which writes a `session_start`
envelope to disk via the existing `StartSession` path).

Responses:

- `200 OK` `{"session_id":"<uuid>","resumed":true|false}` — `resumed`
  is `true` when a prior `session_start` for this asset was found in
  the tuning dir, `false` when a new session was minted.
- `400 Bad Request` — body missing `asset_id` or not valid JSON.
- `405 Method Not Allowed` — non-POST.
- `500 Internal Server Error` — disk read/write failed.

The lookup scans `tuning/*.jsonl`, reads the first line of each, and
returns the most recently modified file whose first envelope is a
`session_start` matching `asset_id`. Results are cached in memory per
process for O(1) repeat lookups within one run.

## Versioning and migration policy

`schema_version` is bumped only when the **envelope** changes in a way
that breaks readers — renaming a field, removing a field, changing a
field's type, or changing the storage layout. New event types and new
optional payload fields are additive and do **not** require a bump.

When a bump becomes necessary:

1. The new constant `AnalyticsSchemaVersion = 2` (etc.) lands in
   `analytics.go`.
2. The export pipeline gains a migration step that reads v1 events and
   rewrites them as v2. v1 files on disk are never modified in-place.
3. This document gains a v2 section while preserving the v1 section
   verbatim.
4. The HTTP endpoint accepts both versions during a transition window
   and only the latest after.

This mirrors the migration policy documented in `settings-schema.md`.

## Export format (v1)

The export pipeline (`scripts/export_tuning_data.py`, T-003-04)
aggregates every tuning artifact under `~/.glb-optimizer/` into a
single JSONL bundle. One record per line, three `kind`s:

```json
{"kind":"asset","asset_id":"<id>","current_settings":{...AssetSettings...|null},"accepted":{...AcceptedSettings...|null},"events":[{...Event envelopes for this asset_id, sorted by timestamp...}]}
{"kind":"profile","schema_version":1,"name":"...","comment":"...","created_at":"...","source_asset_id":"...","settings":{...AssetSettings...}}
{"kind":"meta","schema_version":1,"exported_at":"...","workdir":"<absolute path>","thumbnail_path_format":"relative-to-workdir","record_counts":{"assets":N,"profiles":M,"events":K}}
```

Rules:

- **Asset records** are emitted in `asset_id` ascending order.
  Profile records are emitted in `name` ascending order. The
  `meta` record is emitted **last** so a streaming reader can
  verify completeness from EOF.
- An asset record is emitted only when at least one of
  `current_settings`, `accepted`, or `events` is non-empty.
- The full event stream for an asset is inlined as
  `asset.events`. The same events are NOT also emitted as
  standalone records — the asset record is the join.
- **Thumbnail paths are relative to the workdir** declared in the
  `meta` record's `workdir` field. Resolve with
  `os.path.join(workdir, accepted.thumbnail_path)`.
- The bundle's own `schema_version` is `1`. It is independent of
  `AnalyticsSchemaVersion` and `SettingsSchemaVersion` because the
  bundle is a derived view, not a source-of-truth document.
- Per-line decoding tolerance: the exporter skips and logs (to
  stderr) any malformed line in the input JSONL files; consumers
  should mirror this defensive posture.

The bundle is intentionally a *flat* JSONL stream rather than a
nested JSON document so that it streams cleanly through
`for line in f: rec = json.loads(line)` in any language and
scales beyond what fits in memory at once.

## Out of scope (deferred)

Items listed in S-003 but **not** implemented in v1 of this schema:

- ~~Profile artifacts (`~/.glb-optimizer/profiles/{name}.json`)~~ — landed in T-003-03.
- Thumbnail linking (`thumbnail_path` is reserved but always empty).
- Implicit satisfaction signals (time-between-actions, revert-within-N).
  These can be derived offline from the event stream as it stands; the
  schema does not need to embed them.
- ~~Auto-instrumentation hooks in `app.js`~~ — landed in T-003-02.
- ~~Export script~~ — landed in T-003-04 (`scripts/export_tuning_data.py`).
