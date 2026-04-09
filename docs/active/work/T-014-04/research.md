# T-014-04 Research: CLI Prepare Subcommand

## 1. CLI Entry Point & Subcommand Pattern

**File:** `main.go:18-40`

Manual `os.Args` dispatch — no CLI framework. Each subcommand calls a `run*Cmd(os.Args[2:])` function returning an int exit code (0=success, 1=error, 2=usage). Subcommands dispatch *before* `flag.Parse()` for the server, so the server's gltfpack/blender checks are skipped.

Pattern (`clean_cmd.go`, `pack_cmd.go`):
1. `flag.NewFlagSet("subcommand", flag.ContinueOnError)`
2. Parse flags, return 2 on error
3. Validate positional arg count, return 2 on mismatch
4. `resolveWorkdir()` — creates `~/.glb-optimizer/{originals,outputs,settings,tuning,profiles,accepted,accepted/thumbs,dist/plants}`
5. `NewFileStore()` + `scanExistingFiles()`
6. Perform work
7. Return 0 or 1

## 2. Gltfpack Integration

**File:** `processor.go`

- `BuildCommand(inputPath, outputPath string, s Settings) []string` — constructs args from Settings struct
- `RunGltfpack(args []string) (string, error)` — `exec.Command("gltfpack", args...)` with CombinedOutput
- Settings struct (models.go:16): Simplification, Compression, TextureCompression, TextureQuality, TextureSize, KeepNodes, KeepMaterials, FloatPositions, AggressiveSimplify, PermissiveSimplify, LockBorders

Default optimization: `compression: "cc"` — no aggressive simplify for the base optimize pass.

## 3. File Hashing & ID

The server uses `generateID()` → random 16-byte hex (32 chars) as asset IDs, NOT content hashing. Files stored as `originals/{id}.glb`. The `prepare` command should use content-hash (SHA-256 truncated to 32 hex chars) for idempotency: hashing the source GLB and using the hex prefix as the asset ID.

`upload_manifest.go` provides `AppendUploadRecord()` to persist the original filename → hash mapping for the species resolver.

## 4. LOD Generation (gltfpack path)

**File:** `handlers.go:373-382`

```go
var lodConfigs = []struct {
    Label          string
    Simplification float64
    Aggressive     bool
    Permissive     bool
}{
    {"lod0", 0.5, false, false},
    {"lod1", 0.2, true, false},
    {"lod2", 0.05, true, true},
    {"lod3", 0.01, true, true},
}
```

Inputs from `originalsDir/{id}.glb`, outputs to `outputsDir/{id}_lod0..3.glb`. Uses `BuildCommand` + `RunGltfpack` per level.

## 5. Classification

**File:** `classify.go`

- `RunClassifier(glbPath string) (*ClassificationResult, error)` — shells out to `python3 scripts/classify_shape.py <glb>`
- Returns Category, Confidence, IsHardSurface, Features
- Valid categories: round-bush, directional, tall-narrow, planar, hard-surface, unknown

`handlers.go:applyClassificationToSettings()` merges result into AssetSettings. `applyShapeStrategyToSettings()` stamps strategy defaults (fade bands, budget priority) from `shapeStrategyTable`.

## 6. Blender Integration

**File:** `blender.go`

- `DetectBlender() BlenderInfo` — probes macOS .app paths, then PATH
- `RunBlenderLOD(info, scriptPath, inputPath, outputPath, cfg)` — `exec.Command(info.Path, "-b", "--python", scriptPath, "--", args...)`
- Sets `BLENDER_SYSTEM_RESOURCES` env var when ResourceDir != ""

**Production render (T-014-03):** `handlers.go:1766-1933`
- Writes a `buildProductionConfig` JSON to `{outputsDir}/{id}_render_config.json`
- Calls `blender -b --python scripts/render_production.py -- --config <configPath>`
- Config includes: Source, OutputDir, ID, Category, Resolution, BillboardAngles(6), TiltedElevation(30), VolumetricLayers, SliceDistributionMode, SliceAxis, DomeHeightFactor, AlphaTest, lighting params, skip flags
- Produces: `{id}_billboard.glb`, `{id}_billboard_tilted.glb`, `{id}_volumetric.glb`
- Uses `blenderRenderMu sync.Mutex` for serialization

## 7. CombinePack + RunPack

**File:** `combine.go:698-806`, `pack_runner.go`

- `CombinePack(side, tilted, volumetric []byte, meta PackMeta) ([]byte, error)` — merges 3 intermediates, 5 MiB cap
- `RunPack(id, originalsDir, settingsDir, outputsDir, distDir string, store *FileStore, opts ResolverOptions) PackResult` — full orchestration: read intermediates → BuildPackMetaFromBake → CombinePack → WritePack
- `WritePack(distDir, species string, data []byte) error` — writes `{distDir}/{species}.glb`
- `BuildPackMetaFromBake()` — resolves species (6-tier), reads footprint, captures fade bands

## 8. Pack Verification

**File:** `pack_inspect.go:55-417`

- `InspectPack(path string) (*PackInspectReport, error)` — reads pack, validates structure
- Report includes: PackMeta, variant summary, has_billboard/tilted/dome, SHA256, size breakdown
- `renderHuman()`, `renderJSON()`, `renderQuiet()` output formats
- CLI: `runPackInspectCmd(args)` — `pack-inspect [--json|--quiet] [--dir] <id-or-path>`

## 9. Settings

**File:** `settings.go`

- `AssetSettings` struct: shape category, budget priority, simplification params, compression, texture, fade bands, render params (DomeHeightFactor, AlphaTest, BakeExposure, AmbientIntensity, etc.)
- `DefaultSettings()` returns canonical defaults (Resolution=512, DomeHeightFactor=0.5, etc.)
- `LoadSettings(id, dir)` / `SaveSettings(id, dir, s)` — read/write `{dir}/{id}.json`

## 10. Strategy Table

**File:** `strategy.go:27-120`

- `shapeStrategyTable` maps category → `ShapeStrategy` (SliceAxis, SliceCount, SliceDistributionMode, etc.)
- `getStrategyForCategory(category)` — returns entry or "unknown" fallback
- `applyShapeStrategyToSettings()` stamps defaults onto AssetSettings

## 11. Key Observations for prepare Command

1. **All building blocks exist** — the prepare command is pure orchestration
2. **ID strategy**: Use content-hash (SHA-256 prefix) for CLI idempotency, matching the existing `originals/{hash}.glb` convention
3. **FileStore is needed** by RunPack/BuildPackMetaFromBake — must create and populate
4. **Settings must exist on disk** before pack metadata can be built (fade bands read from settings)
5. **render_production.py exists** at `scripts/render_production.py` and uses JSON config
6. **LOD generation** can reuse the `lodConfigs` + `BuildCommand` + `RunGltfpack` pattern directly
7. **Species resolution** for CLI: the --category flag provides the category, but species name comes from the filename (e.g., `dahlia_blush.glb` → `dahlia_blush`)
8. **No content-hash function exists** in Go code — need to add SHA-256 file hashing
9. **Blender mutex not needed** for CLI (single-threaded sequential execution)
10. **prepare-all** should reuse the same `prepare` logic per file, with `inbox/done/` move on success
