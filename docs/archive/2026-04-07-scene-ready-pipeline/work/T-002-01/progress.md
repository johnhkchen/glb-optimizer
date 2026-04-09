    # Progress — T-002-01: Settings Schema and Persistence

## Status: implementation complete

All six plan steps executed in order. `go build ./...` and `go test ./...`
pass at every checkpoint.

## Step Log

### Step 1 — `settings.go` — DONE

Created `settings.go` with:

- `SettingsSchemaVersion = 1` constant.
- `AssetSettings` struct with all 12 fields in declaration order matching
  the schema doc.
- `DefaultSettings()` returning the canonical defaults.
- `Validate()` checking schema version, numeric ranges, NaN/Inf, resolution
  enum, and lighting preset enum.
- `LoadSettings(id, dir)` returning defaults on `os.IsNotExist`.
- `SaveSettings(id, dir, *AssetSettings)` using indented JSON + atomic
  write.
- `SettingsFilePath`, `SettingsExist` helpers.
- Private `writeAtomic` and `checkRange` helpers.

`go build ./...` clean.

### Step 2 — `settings_test.go` — DONE

Six test functions covering:
- defaults validate cleanly
- save/load round-trip preserves all fields
- missing file → defaults
- bad schema_version (2 and 0)
- out-of-range subtests for each numeric edge + bad resolution + unknown preset
- `SettingsExist` flips after save

All 7 top-level tests + 6 subtests pass.

### Step 3 — `models.go` — DONE

Added `HasSavedSettings bool` with `omitempty` tag to `FileRecord`.

### Step 4 — `handlers.go` — DONE

Added `handleSettings(store, settingsDir) http.HandlerFunc`. Single
function dispatches on `r.Method`:

- Common: trim id, 400 if empty, 404 if not in store.
- `GET`: `LoadSettings` → 200.
- `PUT`: decode → `Validate()` → `SaveSettings` → mark
  `HasSavedSettings = true` on the store record → 200.
- Other methods: 405.

Reuses existing `jsonResponse` / `jsonError` helpers and the standard
`strings.TrimPrefix` id-extraction pattern.

### Step 5 — `main.go` — DONE

Three changes:

1. `settingsDir := filepath.Join(workDir, "settings")` and added to the
   `MkdirAll` slice so it's created on startup.
2. `scanExistingFiles` signature widened to take `settingsDir`; inside,
   `record.HasSavedSettings = SettingsExist(id, settingsDir)` populates
   the flag for files restored after restart.
3. Registered route `mux.HandleFunc("/api/settings/", handleSettings(store, settingsDir))`.

### Step 6 — `docs/knowledge/settings-schema.md` — DONE

Schema doc landed with:

- Versioning section.
- Field table (name, type, default, range, description) for all 12 fields.
- JSON example showing a default document.
- Storage section (path, atomicity, encoding, concurrency note).
- Endpoints table.
- Migration policy with the five-rule decision tree, additive-field
  guidance, and "when to bump" criteria.
- Out-of-scope reminder.

## Deviations from Plan

None. The plan was followed step-for-step. The first test file in the repo
landed without friction; `go test ./...` discovered it automatically with no
build-tag or vendor changes needed, so the manual-recipe fallback in plan.md
was not exercised.

## Files Touched

- **CREATE**: `settings.go` (~180 lines)
- **CREATE**: `settings_test.go` (~95 lines)
- **CREATE**: `docs/knowledge/settings-schema.md` (~110 lines)
- **MODIFY**: `models.go` (+1 line — new `HasSavedSettings` field)
- **MODIFY**: `handlers.go` (+~55 lines — `handleSettings`)
- **MODIFY**: `main.go` (+4 lines / 2 lines changed — `settingsDir`,
  scan signature, route registration)

## Verification

- `go build ./...` — clean.
- `go test ./...` — 7 tests + 6 subtests pass; runtime ~0.3s.
- Manual smoke test recipe is documented in plan.md but was not executed
  in this session (no running server). Recipe is straightforward and the
  acceptance criteria are covered by the automated tests + the structure
  of the wiring (route registered, dir created, scan populates the flag).
