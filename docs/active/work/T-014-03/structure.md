# T-014-03 Structure: API Build-Production Endpoint

## Files Modified

### handlers.go (append)

New handler function + mutex + config builder:

```go
// blenderRenderMu serializes Blender production renders (one at a time).
var blenderRenderMu sync.Mutex

// buildProductionConfig is the JSON shape written to the temp config file
// for render_production.py's --config flag.
type buildProductionConfig struct {
    Source               string  `json:"source"`
    OutputDir            string  `json:"output_dir"`
    ID                   string  `json:"id"`
    Category             string  `json:"category"`
    Resolution           int     `json:"resolution"`
    BillboardAngles      int     `json:"billboard_angles"`
    TiltedElevation      float64 `json:"tilted_elevation"`
    VolumetricLayers     int     `json:"volumetric_layers"`
    VolumetricResolution int     `json:"volumetric_resolution"`
    SliceDistributionMode string `json:"slice_distribution_mode"`
    SliceAxis            string  `json:"slice_axis"`
    DomeHeightFactor     float64 `json:"dome_height_factor"`
    AlphaTest            float64 `json:"alpha_test"`
    GroundAlign          bool    `json:"ground_align"`
    BakeExposure         float64 `json:"bake_exposure"`
    AmbientIntensity     float64 `json:"ambient_intensity"`
    HemisphereIntensity  float64 `json:"hemisphere_intensity"`
    KeyLightIntensity    float64 `json:"key_light_intensity"`
    BottomFillIntensity  float64 `json:"bottom_fill_intensity"`
    EnvMapIntensity      float64 `json:"env_map_intensity"`
    SkipBillboard        bool    `json:"skip_billboard,omitempty"`
    SkipTilted           bool    `json:"skip_tilted,omitempty"`
    SkipVolumetric       bool    `json:"skip_volumetric,omitempty"`
}

// buildProductionResponse is the JSON response for POST /api/build-production/{id}.
type buildProductionResponse struct {
    ID         string `json:"id"`
    Billboard  bool   `json:"billboard"`
    Tilted     bool   `json:"tilted"`
    Volumetric bool   `json:"volumetric"`
    DurationMs int64  `json:"duration_ms"`
}

// handleBuildProduction handles POST /api/build-production/{id}?category={cat}.
// Invokes Blender headlessly to render production impostor intermediates.
func handleBuildProduction(
    store *FileStore,
    settingsDir, outputsDir string,
    blender BlenderInfo,
    renderScriptPath string,
) http.HandlerFunc
```

Handler body flow:
1. Method check (POST only)
2. Extract ID from URL, validate asset exists with status=done
3. Resolve category (query param > saved settings > "unknown")
4. Load AssetSettings, get ShapeStrategy
5. Build `buildProductionConfig` merging settings + strategy
6. Write temp JSON config file
7. Acquire `blenderRenderMu`
8. `exec.CommandContext` with 300s timeout
9. Wait for completion, release mutex
10. Delete temp config file
11. Verify intermediate files exist
12. Update FileStore flags
13. Return `buildProductionResponse`

### main.go (modify route registration block)

Add at L99-116 (after Blender detection):
```go
// Resolve render_production.py script path
renderScriptPath := filepath.Join("scripts", "render_production.py")
if _, err := os.Stat(renderScriptPath); err != nil {
    // Try relative to executable
    if exePath, err2 := os.Executable(); err2 == nil {
        alt := filepath.Join(filepath.Dir(exePath), "scripts", "render_production.py")
        if _, err3 := os.Stat(alt); err3 == nil {
            renderScriptPath = alt
        }
    }
}
```

Add route registration after existing routes (~L174):
```go
mux.HandleFunc("/api/build-production/", handleBuildProduction(store, settingsDir, outputsDir, blenderInfo, renderScriptPath))
```

## Files NOT Modified

- `blender.go` — No new Blender abstraction needed; the handler builds exec.Command directly (render_production.py has a different interface than remesh_lod.py).
- `models.go` — FileRecord already has `HasBillboard`, `HasBillboardTilted`, `HasVolumetric`.
- `settings.go` — No schema changes needed.
- `strategy.go` — Used as-is via `getStrategyForCategory`.

## Constants

- `blenderRenderTimeout = 300 * time.Second` (5 minutes)
- `maxBlenderStderr = 2048` (truncation limit for error messages)

## Test File

New file: `handlers_build_production_test.go`

Test cases:
1. `TestBuildProduction_BlenderNotAvailable` — returns 500
2. `TestBuildProduction_AssetNotFound` — returns 404
3. `TestBuildProduction_AssetNotOptimized` — returns 400
4. `TestBuildProduction_MethodNotAllowed` — GET returns 405
5. `TestBuildProduction_ConfigGeneration` — verify config JSON shape
6. `TestBuildProduction_HardSurfaceSkipsVolumetric` — verify skip flag
