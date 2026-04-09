    # Structure — T-002-01: Settings Schema and Persistence

## File Changes Overview

| File                                  | Action  | Purpose                                  |
|---------------------------------------|---------|------------------------------------------|
| `docs/knowledge/settings-schema.md`   | CREATE  | Authoritative schema spec + migration policy |
| `settings.go`                         | CREATE  | `AssetSettings`, defaults, load/save     |
| `settings_test.go`                    | CREATE  | Round-trip + validation tests            |
| `handlers.go`                         | MODIFY  | Add `handleGetSettings` / `handlePutSettings` (one func, dispatch on method) |
| `main.go`                             | MODIFY  | Create `settings/` dir at startup; pass to handlers and `scanExistingFiles`; register route |
| `models.go`                           | MODIFY  | Add `HasSavedSettings bool` to `FileRecord` |

No file deletions. No changes to `static/`.

## Module Boundaries

### `settings.go` (new) — Pure data layer

Public API:

```go
const SettingsSchemaVersion = 1

type AssetSettings struct {
    SchemaVersion        int     `json:"schema_version"`
    VolumetricLayers     int     `json:"volumetric_layers"`
    VolumetricResolution int     `json:"volumetric_resolution"`
    DomeHeightFactor     float64 `json:"dome_height_factor"`
    BakeExposure         float64 `json:"bake_exposure"`
    AmbientIntensity     float64 `json:"ambient_intensity"`
    HemisphereIntensity  float64 `json:"hemisphere_intensity"`
    KeyLightIntensity    float64 `json:"key_light_intensity"`
    BottomFillIntensity  float64 `json:"bottom_fill_intensity"`
    EnvMapIntensity      float64 `json:"env_map_intensity"`
    AlphaTest            float64 `json:"alpha_test"`
    LightingPreset       string  `json:"lighting_preset"`
}

func DefaultSettings() *AssetSettings
func (s *AssetSettings) Validate() error

func LoadSettings(id, dir string) (*AssetSettings, error)
func SaveSettings(id, dir string, s *AssetSettings) error
func SettingsFilePath(id, dir string) string
func SettingsExist(id, dir string) bool
```

Internal helpers:

```go
func writeAtomic(path string, data []byte) error
```

No imports beyond stdlib (`encoding/json`, `errors`, `fmt`, `os`,
`path/filepath`).

### `handlers.go` (modify) — Transport layer

Add one exported handler factory:

```go
func handleSettings(store *FileStore, settingsDir string) http.HandlerFunc
```

Internally dispatches:

- `GET`: `LoadSettings`, return JSON. If file missing, return defaults.
- `PUT`: decode → `Validate()` → `SaveSettings` → mark `HasSavedSettings` on
  the store record → return canonical settings.
- `OPTIONS` / others: 405.

Reuses `jsonResponse` and `jsonError`. ID extraction matches the existing
pattern: `strings.TrimPrefix(r.URL.Path, "/api/settings/")`.

### `main.go` (modify)

Three small changes:

1. After `outputsDir := filepath.Join(workDir, "outputs")`, add:
   ```go
   settingsDir := filepath.Join(workDir, "settings")
   ```
   and include it in the existing `MkdirAll` slice.
2. Update the `scanExistingFiles` call site and signature to take
   `settingsDir`. Inside, after building the `FileRecord`, call
   `SettingsExist(id, settingsDir)` to populate `HasSavedSettings`.
3. Register the new route:
   ```go
   mux.HandleFunc("/api/settings/", handleSettings(store, settingsDir))
   ```

### `models.go` (modify)

Add one field to `FileRecord`:

```go
HasSavedSettings bool `json:"has_saved_settings,omitempty"`
```

That's it. The struct already has the right shape; the new field reuses
existing JSON conventions.

## File-Level Layout

### `settings.go` declaration order

1. Package + imports.
2. `SettingsSchemaVersion` constant.
3. `AssetSettings` struct (fields in the order shown above — that's also
   the on-disk JSON order).
4. `DefaultSettings()` constructor.
5. `Validate()` method.
6. `SettingsFilePath(id, dir string) string` helper.
7. `SettingsExist(id, dir string) bool` helper.
8. `LoadSettings(id, dir string) (*AssetSettings, error)`.
9. `SaveSettings(id, dir string, s *AssetSettings) error`.
10. `writeAtomic(path string, data []byte) error` (private).

### `settings_test.go` cases

1. `TestDefaultSettings_Valid` — `DefaultSettings().Validate()` returns nil.
2. `TestSaveLoad_Roundtrip` — write defaults to a `t.TempDir()`, read back,
   `reflect.DeepEqual` to the original.
3. `TestLoadMissing_ReturnsDefaults` — `LoadSettings("nonexistent", dir)`
   returns defaults with no error.
4. `TestValidate_RejectsBadVersion` — `schema_version = 2` returns error.
5. `TestValidate_RejectsOutOfRange` — `bake_exposure = -1` returns error.
6. `TestSettingsExist` — false before save, true after.

## Public Interface Stability

After this ticket the contract for downstream tickets is:

- **HTTP**: `GET /api/settings/{id}` → 200 + `AssetSettings` JSON;
  `PUT /api/settings/{id}` accepts the same shape, returns 200 + canonical.
- **Go**: `LoadSettings`, `SaveSettings`, `DefaultSettings`, `AssetSettings`
  are the four symbols T-002-02 will import.
- **Schema doc**: `docs/knowledge/settings-schema.md` is the source of truth
  for field names, defaults, ranges.

## Ordering of Changes

The changes must land in this order to keep the build green at every step:

1. `settings.go` (new, no dependencies). Compiles standalone.
2. `settings_test.go` (depends on `settings.go`). `go test ./...` passes.
3. `models.go` (one-line addition; nothing references the new field yet).
4. `handlers.go` (depends on `settings.go` + the new field).
5. `main.go` (depends on the new handler; wires up `settingsDir`).
6. `docs/knowledge/settings-schema.md` (documentation; can land any time
   but kept last so it reflects the as-built code).

Each step is independently buildable and committable, which matches the
"small enough to commit atomically" rule from the workflow doc.

## What This Structure Does NOT Touch

- `static/app.js` — T-002-02.
- `processor.go`, `blender.go`, `scene.go` — unrelated subsystems.
- `handleDeleteFile` — symmetric cleanup of `settings/{id}.json` on delete
  is a *nice-to-have* not in acceptance criteria; deferred.
- Any existing `Settings` (gltfpack) struct or its handlers — naming is
  deliberately kept distinct (`AssetSettings` vs `Settings`).
