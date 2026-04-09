    # Design — T-002-01: Settings Schema and Persistence

## Decision Summary

- **Schema document**: `docs/knowledge/settings-schema.md`, version `1`, with
  one table per logical group and a migration-policy section.
- **Storage**: One JSON file per asset at `~/.glb-optimizer/settings/{id}.json`.
  No subdirectories. Atomic write via `os.CreateTemp` + `os.Rename` in the
  same directory.
- **API surface**: `GET /api/settings/{id}` and `PUT /api/settings/{id}` on a
  single registered handler `/api/settings/` that dispatches on `r.Method`,
  matching the codebase's existing path-trim routing pattern.
- **Validation**: Hand-rolled, in `settings.go`. Strict on `schema_version`
  and numeric ranges; lenient on unknown fields (forward-compatible).
- **`HasSavedSettings`** is recomputed from disk presence, never persisted in
  the JSON. Populated in `scanExistingFiles` and after each upload/save.
- **Verification**: A `settings_test.go` covering defaults round-trip and
  validation rejection. This is the first test file in the repo, but
  `go test ./...` will pick it up with no extra wiring.

## Schema Decisions

### Naming

- Struct: `AssetSettings` (avoids the existing `Settings` collision in
  `models.go`).
- File: `settings.go` at repo root.
- JSON tags: `snake_case` matching the rest of `models.go`.

### Field set (v1)

Mirrors the ticket's acceptance criteria. Each field carries a default that
matches *current observed behavior*, even when current behavior is hardcoded
and inconsistent (the volumetric renderer uses key=1.6 in one place and 1.4
in another — the schema canonicalizes on `1.4` per the ticket).

```
schema_version       int     = 1
volumetric_layers    int     = 4    range [1,16]
volumetric_resolution int    = 512  enum {128,256,512,1024,2048}
dome_height_factor   float64 = 0.5  range [0.0, 2.0]
bake_exposure        float64 = 1.0  range [0.0, 4.0]
ambient_intensity    float64 = 0.5  range [0.0, 4.0]
hemisphere_intensity float64 = 1.0  range [0.0, 4.0]
key_light_intensity  float64 = 1.4  range [0.0, 8.0]
bottom_fill_intensity float64 = 0.4 range [0.0, 4.0]
env_map_intensity    float64 = 1.2  range [0.0, 4.0]
alpha_test           float64 = 0.15 range [0.0, 1.0]
lighting_preset      string  = "default" enum {"default"} (extends in S-007)
```

Ranges are deliberately permissive — they exist to catch typos (negative
values, NaN, absurd magnitudes), not to encode taste. UI sliders in T-002-03
will set tighter clamps for the *recommended* range.

### Migration policy

Documented in the schema doc:

1. The server reads the file, inspects `schema_version`.
2. If `schema_version == current`, validate and use.
3. If `schema_version < current`, run a registered migrator function chain
   `migrateV1toV2`, `migrateV2toV3`, etc., then re-validate.
4. If `schema_version > current`, log a warning and return defaults — never
   silently downgrade a forward-version file (a newer client may have written
   semantics this server can't honor).
5. Migration happens in `LoadSettings` so callers always see the current
   shape.
6. v0 → v1 migration is **not** in scope: prior to this ticket no settings
   files exist on disk anywhere.

## Storage Design

### Path layout

```
~/.glb-optimizer/settings/{id}.json
```

Single flat directory. `id` is the same hex string used by `originals/` and
`outputs/`, so settings move with the asset and `handleDeleteFile` can clean
them up symmetrically (left to a follow-up — the ticket only cares about
read/write/persistence here).

### Atomic write

```go
func SaveSettings(id, dir string, s *AssetSettings) error {
    if err := os.MkdirAll(dir, 0755); err != nil { ... }
    final := filepath.Join(dir, id+".json")
    tmp, err := os.CreateTemp(dir, id+".*.json.tmp")
    ...
    enc := json.NewEncoder(tmp); enc.SetIndent("", "  ")
    if err := enc.Encode(s); err != nil { os.Remove(tmp.Name()); ... }
    tmp.Close()
    return os.Rename(tmp.Name(), final)
}
```

Same-directory `CreateTemp` guarantees `Rename` is atomic on the same
filesystem (POSIX) and on macOS APFS. We do not fsync — settings are not
crash-critical and the cost isn't worth it for a local dev tool.

### Stable field order

`encoding/json` marshals struct fields in declaration order. We rely on that
rather than a custom marshaler. Declaration order in `AssetSettings` will
match the ticket's enumeration order so the on-disk file is human-readable.

## API Design

### Routes

Register one handler at `/api/settings/`. Inside, switch on `r.Method`:

- `GET`  — read settings, return defaults if file is missing.
- `PUT`  — decode body, validate, write, return 200 with the canonical
  (post-validation) settings.
- Anything else → 405.

The id is extracted via `strings.TrimPrefix(r.URL.Path, "/api/settings/")`
matching `handleProcess`/`handleDownload` style. An empty id → 400.

### Validation strategy

1. Decode into `AssetSettings`. Use `json.NewDecoder(r.Body).Decode(&s)`
   (lenient on unknown fields by default — desirable for forward
   compatibility).
2. Call `s.Validate()` which returns `error` listing the *first* failing
   field. Cheap and good enough for a local tool.
3. `Validate()` checks:
   - `schema_version == 1` (reject v2+; reject 0/missing as well).
   - All numeric ranges.
   - `lighting_preset` is in the enum set.
4. On failure → 400 with the error message in the standard `{"error":...}`
   shape.

### Why one handler instead of two

`http.ServeMux` will not let us register the same path twice with different
methods, and the codebase already has handler functions that switch on
`r.Method` internally (see `handleProcess`). One handler keeps the routing
table simple.

## `HasSavedSettings` Computation

Add `HasSavedSettings bool` to `FileRecord` with `json:"has_saved_settings,omitempty"`.

Populated in two places:

1. **`scanExistingFiles`** in `main.go`: stat the settings file alongside the
   original and set the bool. New parameter `settingsDir`.
2. **After successful PUT**: `store.Update(id, func(r *FileRecord) { r.HasSavedSettings = true })`.

We do **not** mutate the bool inside `LoadSettings` — that function just
reads from disk and is called both by `GET /api/settings/:id` and (in later
tickets) by the bake pipeline. The store is the source of truth for the
flag's value as observed by `/api/files`.

## Rejected Alternatives

1. **Single `settings.json` keyed by id** — simpler than per-file but
   requires read-modify-write for every save, makes deletion fiddly, and
   doesn't compose with the existing per-id file layout. Rejected.
2. **SQLite** — overkill, adds a CGO dependency, and the working directory
   is meant to be inspectable with `cat`. Rejected.
3. **Embedding settings in `FileRecord` and persisting `FileStore`** — would
   couple settings to the in-memory store's lifecycle and defeat the
   ticket's "load defaults if missing" semantic. Rejected.
4. **JSON-Schema library + generated validator** — overkill for a 12-field
   struct in a single-developer tool. Hand-rolled `Validate()` is ~30 lines.
   Rejected.
5. **Strict unknown-field rejection** (`Decoder.DisallowUnknownFields`) —
   would break forward compatibility the moment T-002-02 or S-005 add a
   field. Rejected; we accept unknowns and let them fall on the floor.
6. **Per-field PATCH endpoint** — out of scope; T-002-02 will simply re-PUT
   the full document. Rejected.

## What This Design Does Not Address

- Wiring the JS bake code to fetch/use these settings (T-002-02).
- UI sliders (T-002-03).
- Cleanup of `settings/{id}.json` on delete (small follow-up; not in
  acceptance criteria).
- Profile/preset storage (S-007).
- Analytics events on settings change (S-003).
