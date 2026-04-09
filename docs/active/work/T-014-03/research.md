# T-014-03 Research: API Build-Production Endpoint

## Ticket Summary

Wire the Blender `render_production.py` script into the Go server as `POST /api/build-production/{id}?category={category}`. Both the CLI `prepare` subcommand (T-014-04) and the UI button (T-014-05) funnel through this endpoint. Synchronous v1 — blocks until Blender finishes (up to 5 min timeout).

## Codebase Mapping

### Route Registration (main.go)

Routes are registered on `http.NewServeMux` at L129-174. Each route calls a handler factory that returns `http.HandlerFunc`. Key variables available at registration time:

- `store *FileStore` — in-memory file state
- `originalsDir`, `outputsDir`, `settingsDir` — disk paths
- `blenderInfo BlenderInfo` — detected Blender (Available, Path, Version, ResourceDir)
- `blenderScriptPath string` — path to embedded remesh_lod.py

New endpoint will be registered here alongside existing routes.

### Existing Blender Integration Pattern (blender.go + handlers.go:1329-1375)

`handleGenerateBlenderLODs` is the reference pattern:
1. Check `blender.Available` — return 503 if not found
2. Extract ID from URL path
3. Look up asset in FileStore
4. Build `exec.Command` with Blender args
5. Run synchronously, capture combined output
6. Update FileStore with results

`RunBlenderLOD` (blender.go:102-126) builds the exec.Command:
- Uses `blender.Path` with `-b --python <script> -- <args>`
- Sets `BLENDER_SYSTEM_RESOURCES` env var if `blender.ResourceDir` is set
- Returns combined stdout/stderr and error

### render_production.py CLI Interface

The script accepts:
- `--source <path>` — source GLB
- `--output-dir <path>` — where to write intermediates
- `--id <id>` — asset ID for filenames
- `--config <path>` — JSON config file (all params)
- `--category <cat>` — shape category
- Individual overrides: `--resolution`, `--billboard-angles`, `--tilted-elevation`, `--volumetric-layers`, `--volumetric-resolution`, `--slice-distribution-mode`, `--slice-axis`, `--dome-height-factor`, `--alpha-test`, `--ground-align`, `--bake-exposure`, `--ambient-intensity`, `--hemisphere-intensity`, `--key-light-intensity`, `--bottom-fill-intensity`, `--env-map-intensity`
- Skip flags: `--skip-billboard`, `--skip-tilted`, `--skip-volumetric`

Output files:
- `{output-dir}/{id}_billboard.glb`
- `{output-dir}/{id}_billboard_tilted.glb`
- `{output-dir}/{id}_volumetric.glb`

### FileRecord Flags (models.go:45-65)

Three boolean flags already exist on `FileRecord`:
- `HasBillboard bool` — set by `handleUploadBillboard`
- `HasBillboardTilted bool` — set by `handleUploadBillboardTilted`
- `HasVolumetric bool` — set by `handleUploadVolumetric`

The new endpoint updates these same flags after Blender completes.

### AssetSettings (settings.go:21-72)

Per-asset settings include all bake parameters needed for render_production.py:
- `VolumetricLayers`, `VolumetricResolution`, `DomeHeightFactor`
- `BakeExposure`, `AmbientIntensity`, `HemisphereIntensity`, `KeyLightIntensity`, `BottomFillIntensity`, `EnvMapIntensity`
- `AlphaTest`, `SliceDistributionMode`, `SliceAxis`, `GroundAlign`
- `ShapeCategory` — used to look up `ShapeStrategy`

`LoadSettings(id, dir)` reads from disk; returns defaults if file missing.

### ShapeStrategy (strategy.go)

`getStrategyForCategory(category)` returns `ShapeStrategy` with:
- `SliceAxis`, `SliceCount`, `SliceDistributionMode`
- Falls back to "unknown" entry for unrecognized categories

The endpoint needs to merge AssetSettings + ShapeStrategy when building the config JSON.

### Status Endpoint (handlers.go:1076-1082)

`handleStatus` already returns `blenderInfo` as JSON — clients can check capabilities before calling build-production.

## Asset State Precondition

The ticket requires `status=done` (asset already optimized). The existing upload-billboard handlers don't check this — they accept any asset. The new endpoint is stricter: it validates status before launching Blender.

Relevant `FileStatus` values: `StatusPending`, `StatusProcessing`, `StatusDone`, `StatusError`.

## Concurrency Model

Ticket spec: Go mutex serializes renders, one at a time. The existing `handleProcessAll` doesn't use a mutex — it processes sequentially in a loop. A new `sync.Mutex` dedicated to Blender renders is needed.

## Timeout Pattern

Go's `context.WithTimeout` + `exec.CommandContext` provides clean timeout support. The existing `RunBlenderLOD` doesn't use timeouts. The new endpoint needs 5-minute (300s) timeout with process kill on expiry.

## Open Questions

1. **Config file vs CLI args**: The ticket says "temp JSON config file." The script supports both `--config` and individual CLI args. Using `--config` is cleaner for passing many parameters.
2. **hard-surface category**: Strategy has `SliceAxis="n/a"` and `SliceCount=0` — the endpoint should still render billboards but skip volumetric (pass `--skip-volumetric`).
3. **Script path**: render_production.py lives at `scripts/render_production.py` in the repo, not embedded like remesh_lod.py. The endpoint needs the repo-relative path or an absolute path.
