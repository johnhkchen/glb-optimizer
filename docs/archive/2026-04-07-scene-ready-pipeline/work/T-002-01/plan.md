    # Plan — T-002-01: Settings Schema and Persistence

## Step Sequence

Each step is committable on its own. After each, `go build ./...` and (where
relevant) `go test ./...` must pass.

### Step 1 — Create `settings.go`

**Goal**: Land the data layer with no consumers.

**Actions**:

- Create `settings.go` in repo root.
- Add `SettingsSchemaVersion = 1` constant.
- Define `AssetSettings` struct with all 12 fields (including
  `schema_version`) in declaration order matching the schema doc.
- Implement `DefaultSettings()` returning a pointer to a struct with the
  canonical defaults from the ticket.
- Implement `Validate()`:
  - `SchemaVersion != 1` → error "unsupported schema_version: %d".
  - Each numeric field outside its range → error naming the field.
  - `LightingPreset` not in `{"default"}` → error.
- Implement `SettingsFilePath(id, dir)` and `SettingsExist(id, dir)`.
- Implement `LoadSettings`:
  - If file missing (`os.IsNotExist`), return `DefaultSettings(), nil`.
  - Otherwise read, `json.Unmarshal` into a fresh struct, return it.
  - Do **not** call `Validate()` here — load is permissive; validation
    happens at the HTTP boundary.
- Implement `SaveSettings`:
  - `os.MkdirAll(dir, 0755)`.
  - Marshal with `json.MarshalIndent(s, "", "  ")` for human readability.
  - `writeAtomic(path, data)`.
- Implement `writeAtomic`: `os.CreateTemp` in same dir, write, close,
  `os.Rename`. Clean up the temp file on any failure.

**Verify**: `go build ./...`.

**Commit**: `Add AssetSettings schema, defaults, load/save (T-002-01)`.

### Step 2 — Add `settings_test.go`

**Goal**: Lock the data-layer contract with tests.

**Actions**: Create `settings_test.go` with the six tests listed in
structure.md.

**Verify**: `go test ./...` passes.

**Commit**: `Add tests for AssetSettings round-trip and validation (T-002-01)`.

### Step 3 — Add `HasSavedSettings` to `FileRecord`

**Goal**: One-line model change with no behavioral effect yet.

**Actions**: Add field to `models.go` with `omitempty`.

**Verify**: `go build ./...`.

**Commit**: `Add HasSavedSettings field to FileRecord (T-002-01)`.

### Step 4 — Add HTTP handler

**Goal**: Land `handleSettings` in `handlers.go`.

**Actions**:

- Add `handleSettings(store *FileStore, settingsDir string) http.HandlerFunc`.
- GET branch:
  - Trim prefix to extract id.
  - Empty id → 400.
  - Look up the record (404 if not in store).
  - `LoadSettings(id, settingsDir)` → 200 + JSON.
- PUT branch:
  - Trim/lookup as above.
  - Decode body into `AssetSettings`. On decode error → 400.
  - Call `Validate()`. On error → 400 with the message.
  - `SaveSettings(id, settingsDir, &s)`. On error → 500.
  - `store.Update(id, func(r *FileRecord) { r.HasSavedSettings = true })`.
  - Return 200 with the canonical settings.
- Other methods → 405.

**Verify**: `go build ./...`.

**Commit**: `Add GET/PUT /api/settings/{id} handler (T-002-01)`.

### Step 5 — Wire `settingsDir` in `main.go`

**Goal**: Create the directory at startup, populate `HasSavedSettings`,
register the route.

**Actions**:

- Add `settingsDir := filepath.Join(workDir, "settings")` next to the
  existing dirs.
- Include it in the `MkdirAll` slice.
- Update `scanExistingFiles` signature to take `settingsDir`; inside,
  populate `HasSavedSettings` via `SettingsExist`.
- Register `mux.HandleFunc("/api/settings/", handleSettings(store, settingsDir))`.

**Verify**: `go build ./...`, then run the server and exercise the endpoint
manually (see Verification Recipe below).

**Commit**: `Wire settings dir and route in main (T-002-01)`.

### Step 6 — Document the schema

**Goal**: Land `docs/knowledge/settings-schema.md`.

**Actions**: Write the schema doc with:

- Frontmatter (none — this is reference material, not a ticket).
- Title + one-paragraph purpose.
- Versioning section (`schema_version: 1`).
- Field table (name, type, default, range, description).
- JSON example showing a default document.
- Migration policy section (the 5 rules from design.md).
- Storage section (path layout, atomic write).

**Verify**: Read it; check field list matches `AssetSettings` struct
declaration order.

**Commit**: `Document settings schema v1 (T-002-01)`.

## Testing Strategy

### Automated (Go tests)

The six tests in step 2 cover:

- **Defaults are valid**: forces the contract that `DefaultSettings()`
  produces a struct that passes its own validator.
- **Round-trip**: write → read returns an equivalent struct (this catches
  field-tag typos and any future custom marshal logic that breaks).
- **Missing file → defaults**: documented behavior of `LoadSettings`.
- **Validation rejects bad version**: catches accidental migration regressions.
- **Validation rejects out-of-range**: spot-check on `bake_exposure`. Not
  exhaustive — one example proves the mechanism works.
- **`SettingsExist` flips after save**: confirms the path-existence helper.

### Manual (smoke test)

After step 5, run the server and curl:

```bash
./glb-optimizer &  # or `go run .`
# upload a glb to get an id (use the UI or curl /api/upload)
ID=...

# 1. GET returns defaults
curl -s http://localhost:8787/api/settings/$ID | jq .

# 2. PUT a modified value
curl -s -X PUT http://localhost:8787/api/settings/$ID \
  -H 'Content-Type: application/json' \
  -d '{"schema_version":1,"volumetric_layers":6,"volumetric_resolution":512,
       "dome_height_factor":0.5,"bake_exposure":1.2,"ambient_intensity":0.5,
       "hemisphere_intensity":1.0,"key_light_intensity":1.4,
       "bottom_fill_intensity":0.4,"env_map_intensity":1.2,"alpha_test":0.15,
       "lighting_preset":"default"}' | jq .

# 3. GET again returns the persisted value
curl -s http://localhost:8787/api/settings/$ID | jq .volumetric_layers  # → 6

# 4. File list shows the flag
curl -s http://localhost:8787/api/files | jq '.[] | select(.id=="'"$ID"'") | .has_saved_settings'  # → true

# 5. Bad version is rejected
curl -s -X PUT http://localhost:8787/api/settings/$ID \
  -H 'Content-Type: application/json' \
  -d '{"schema_version":99,"volumetric_layers":4,...}'  # → 400

# 6. File on disk is human-readable
cat ~/.glb-optimizer/settings/$ID.json
```

### Verification criteria (acceptance check)

- [ ] `go build ./...` and `go test ./...` both pass.
- [ ] `~/.glb-optimizer/settings/` exists after first server start.
- [ ] `GET /api/settings/{id}` returns defaults for an unsaved asset.
- [ ] `PUT /api/settings/{id}` persists and round-trips.
- [ ] Bad `schema_version` and out-of-range values are rejected with 400.
- [ ] `/api/files` reflects `has_saved_settings: true` after a save.
- [ ] `docs/knowledge/settings-schema.md` lists every field with default,
      range, type, description, plus the migration policy.

## Risks and Mitigations

- **Test file is the first in the repo** — if `go test ./...` somehow
  fails to discover `settings_test.go` (build tags, vendor weirdness), fall
  back to documenting the manual recipe in `docs/active/work/T-002-01/manual-verify.md`
  and mark the test step as a follow-up. Mitigation: try the test path
  first; switch only if it actually breaks.
- **`scanExistingFiles` signature change** is the largest cross-file edit.
  Mitigation: update both the call site and the function in the same
  commit (step 5).
- **Race on concurrent PUTs** — `SaveSettings` is atomic per call, but two
  simultaneous PUTs to the same id will race and one will win. Acceptable
  for a single-user local tool; documented as a known limitation in
  review.md, not fixed here.

## Out-of-Scope Reminders

- No JS changes.
- No UI.
- No symmetric delete cleanup.
- No analytics events.
- No new fields beyond the 11 (+ schema_version) listed in the ticket.
