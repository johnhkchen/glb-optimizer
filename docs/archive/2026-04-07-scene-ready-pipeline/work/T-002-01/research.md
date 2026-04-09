    # Research — T-002-01: Settings Schema and Persistence

## Ticket Scope Recap

Foundation work for E-001. Today every parameter that drives the volumetric
bake (slice count, dome height, exposure, ambient/key/hemisphere intensities,
env map intensity, alpha test thresholds, color calibration mode) is a
hardcoded literal scattered across `static/app.js`. This ticket lands the
**schema, the storage layer, and the read/write HTTP endpoints**. No client
wiring (T-002-02) and no UI (T-002-03).

## Existing Code Inventory

### Server (Go)

- `main.go` — `main()` initializes the work directory at `~/.glb-optimizer/`
  with `originals/` and `outputs/` subdirs (lines 56–64). Routes are
  registered on a `http.ServeMux` (lines 95–120). `scanExistingFiles()`
  rebuilds the in-memory store on startup (lines 141–172).
- `handlers.go` — All HTTP handlers. Helpers `jsonResponse` and `jsonError`
  exist (lines 25–33). Path-suffix routing pattern: handlers do
  `strings.TrimPrefix(r.URL.Path, "/api/foo/")` to extract the id. The
  existing `Settings` struct (a *gltfpack* settings bag) is decoded from
  request bodies in `handleProcess` (line 131) and `handleProcessAll`.
- `models.go` — Defines:
  - `Settings` (gltfpack CLI flags — `simplification`, `compression`,
    `texture_*`, etc.). **Important**: this is **not** the per-asset bake
    settings the ticket is about. Naming collision risk; the new struct must
    be named differently (`AssetSettings`, per acceptance criteria).
  - `FileRecord` (line 45) — needs a `HasSavedSettings bool` field.
  - `FileStore` is a thread-safe in-memory map; persistence elsewhere is
    file-system based, never DB.
- `processor.go`, `blender.go`, `scene.go` — Processing pipelines. Not touched
  by this ticket.

### Client (`static/app.js`) — Hardcoded Bake Constants

Located via grep. The following literals will become fields in the schema:

| Constant                          | Location              | Today's value             |
|-----------------------------------|-----------------------|---------------------------|
| `VOLUMETRIC_LAYERS`               | line 542              | `4` (adaptive base)       |
| `VOLUMETRIC_RESOLUTION`           | line 543              | `512`                     |
| dome height factor                | line 738              | `layerThickness * 0.5`    |
| `toneMappingExposure`             | line 580              | `1.0`                     |
| `AmbientLight` intensity          | line 614              | `0.5`                     |
| `HemisphereLight` intensity       | line 615              | `1.0`                     |
| Key (top) `DirectionalLight`      | lines 363, 616        | `1.4` / `1.6`             |
| Bottom fill `DirectionalLight`    | line 367              | `0.4`                     |
| `envMapIntensity` (clones)        | lines 422, 1584       | `1.2` / `2.0`             |
| `alphaTest` (volumetric/billboard) | lines 491, 745, 978   | `0.1` / `0.15`            |
| Lighting preset                   | (none — implicit)     | `"default"`               |

`pickAdaptiveLayerCount` (line 709) bumps the base layer count by +1 or +2 for
tall/thin models. The schema's `volumetric_layers` is the **base** value, not
the adapted one. The adaptive multiplier stays in JS.

Note that several constants appear with multiple values (e.g. key light is
`1.4` in the billboard renderer and `1.6` in the volumetric renderer). The
ticket calls out `key_light_intensity: 1.4` as the canonical default; the
volumetric renderer's `1.6` is a discrepancy that will resolve to `1.4` once
T-002-02 wires up the values.

### Tests

`go test ./...` is the standard, but no `*_test.go` files exist in this repo
today. The acceptance criteria allow either a Go test file *or* a documented
manual verification recipe. Adding a single `settings_test.go` is the lower-
friction path and gives future tickets a foothold for more tests.

## Filesystem Layout (Current)

```
~/.glb-optimizer/
├── originals/{id}.glb
└── outputs/
    ├── {id}.glb
    ├── {id}_lod{0..3}.glb
    ├── {id}_billboard.glb
    ├── {id}_volumetric.glb
    └── {id}_vlod{0..3}.glb
```

The ticket prescribes a third subdirectory: `~/.glb-optimizer/settings/{id}.json`.
`main.go` creates `originals/` and `outputs/` at startup using a slice +
`MkdirAll` loop — adding `settings/` is a one-line change.

## Routing Pattern

`http.ServeMux` is used directly (not chi/gorilla). Path parameters are
extracted by string trimming. The `/api/files` vs `/api/files/:id` split is
handled by registering both `/api/files` and `/api/files/` and dispatching
inside the handler based on path-segment count (main.go:99–106). The new
`/api/settings/:id` route can register on `/api/settings/` and switch on
`r.Method` (GET vs PUT) inside one handler — that matches existing
conventions.

## Existing JSON Conventions

- Field names: `snake_case` via struct tags (`json:"file_id"`).
- `omitempty` is used widely for optional fields.
- `json.NewDecoder(r.Body).Decode(&x)` for request bodies; `jsonResponse`
  helper for responses.
- No JSON-Schema or validator library — validation is hand-rolled
  (e.g. `handleOptimizeScene` enum checks at lines 723–748). Numeric range
  validation will follow that style.

## Atomic Write Pattern

The codebase doesn't have an existing atomic-write helper. `os.WriteFile`
is used directly in several places (e.g. `handleUploadBillboard` line 433).
The ticket requires temp-file + `os.Rename` for `SaveSettings`. This will be
a new helper local to `settings.go`. `os.CreateTemp` in the same directory
as the destination guarantees `Rename` is atomic on the same filesystem.

## Constraints and Risks

1. **Name collision** between existing `Settings` (gltfpack) and the new
   per-asset settings. Resolved by naming the new struct `AssetSettings` and
   the file `settings.go` (gltfpack settings live in `models.go`).
2. **Adaptive layers in JS**: schema stores the base; the +1/+2 adaptation
   happens later in the renderer. Documented so future readers don't expect
   the saved value to match what the renderer actually uses.
3. **No migration target yet** (`schema_version: 1` only). The migration
   *policy* still needs to be documented so v2 has a clear path.
4. **`HasSavedSettings` is computed from disk presence** — this is a derived
   field, not persisted in the JSON. It must be populated whenever a record
   is read out (in `scanExistingFiles` and after upload).
5. **No existing test infrastructure**. Adding a Go test file is feasible
   but introduces the first `*_test.go` to the repo; the manual recipe is
   the safer fallback if test wiring causes friction.

## Open Questions

- Should the JSON file include the asset ID inside the document, or rely
  solely on the filename? (Design phase — leaning toward filename only,
  matching how originals/outputs work.)
- Should validation reject *unknown* fields (strict) or ignore them
  (forward-compatible)? (Design phase — leaning toward ignore for forward
  compatibility, since v2 may add fields that v1 servers must tolerate.)
