# T-014-01 Research: Extract Rendering Parameters from app.js

## Overview

This ticket requires extracting every rendering parameter from `static/app.js` that
controls how the four impostor artifact types are generated. The output is a
reference document (`docs/knowledge/production-render-params.md`) that will serve
as the spec for the Blender script (T-014-02) and the validation baseline (T-014-06).

## Source Files

- **`static/app.js`** (5091 lines) — all rendering logic lives here, client-side
- **`settings.go`** — server-side mirror of `makeDefaults()`, stores per-asset settings

## Rendering Functions Identified

### 1. Side Billboards

**Entry**: `generateBillboard(id)` (L1322)
**Render**: `renderMultiAngleBillboardGLB(model, numAngles)` (L1802)
**Per-angle**: `renderBillboardAngle(model, angleRad, resolution, elevationRad=0)` (L1392)
**Top-down**: `renderBillboardTopDown(model, resolution)` (L1759) — called from within renderMultiAngleBillboardGLB

Key parameters:
- `BILLBOARD_ANGLES = 6` (L1311) — 6 evenly-spaced azimuth angles
- `resolution = 512` (L1808, hardcoded inside renderMultiAngleBillboardGLB)
- Camera: **orthographic**, sized to fit model bounding box
  - `halfW = maxHoriz * 0.55` where `maxHoriz = max(size.x, size.z)` (L1412)
  - `halfH = size.y * 0.55` (when elevationRad=0, cosE=1, sinE=0) (L1411)
  - Near/far: `0.01` / `maxDim * 10` (L1413)
  - Distance: `maxDim * 2` from center (L1418)
  - Position: orbits at `angleRad` around center at Y=center.y (L1419-1422)
- Quad construction: `PlaneGeometry(quadWidth, quadHeight)` with bottom-edge pivot (`translate(0, quadHeight/2, 0)`) (L1816-1817)
- Naming: `billboard_0` through `billboard_5` (L1825)
- Material: `MeshBasicMaterial`, transparent, DoubleSide, `alphaTest = currentSettings.alpha_test` (default 0.10) (L1818-1823)
- Background: transparent RGBA (`setClearColor(0x000000, 0)`) (L1403)
- Renderer: `THREE.SRGBColorSpace`, `ACESFilmicToneMapping`, exposure from `currentSettings.bake_exposure` (default 1.0) (L1404-1407)
- **Output includes `billboard_top`** quad from `renderBillboardTopDown` (L1830-1855)

### 2. Top-Down Billboard (`billboard_top`)

**Function**: `renderBillboardTopDown(model, resolution)` (L1759)

Key parameters:
- Camera: **orthographic**, straight down
  - `halfW = size.x * 0.55`, `halfD = size.z * 0.55`, `half = max(halfW, halfD)` (L1774-1776)
  - Position: `(center.x, center.y + size.y * 2, center.z)` (L1777)
  - Near/far: `0.01` / `size.y * 10` (L1777)
  - `lookAt(center)` (L1778)
- Same renderer settings (SRGB, ACES, bake_exposure)
- Same lighting: `createBakeEnvironment` + `setupBakeLights` (L1782-1783)
- Quad: `PlaneGeometry(quadSize, quadSize)` rotated flat (`rotateX(-Math.PI/2)`) (L1835-1836)
- Name: `billboard_top` (L1848)
- Returns `{ canvas, quadSize: half * 2 }` — quad sized to encompass model footprint

### 3. Tilted Billboards

**Entry**: `generateTiltedBillboard(id)` (L1353)
**Render**: `renderTiltedBillboardGLB(model, numAngles, elevationRad, resolution)` (L1871)
**Per-angle**: `renderBillboardAngle(model, angleRad, resolution, elevationRad)` (L1392) — same function, non-zero elevation

Key parameters:
- `TILTED_BILLBOARD_ANGLES = 6` (L1316)
- `TILTED_BILLBOARD_ELEVATION_RAD = Math.PI / 6` (30 degrees) (L1317)
- `TILTED_BILLBOARD_RESOLUTION = 512` (L1318)
- Camera: orthographic, same formula as side but with elevation:
  - `halfH = (size.y * cos(30deg) + maxHoriz * sin(30deg)) * 0.55` (L1411)
  - `halfW = maxHoriz * 0.55` (L1412)
  - Distance: `maxDim * 2`, elevated by `dist * sin(30deg)` above center.y (L1419-1422)
- **No `billboard_top` quad** — side variants only (confirmed by code at L1871-1908)
- Naming: `billboard_0` through `billboard_5` (L1893)
- Same material, background, renderer settings as side billboards

### 4. Volumetric Dome Slices

**Entry**: `generateVolumetric(id)` (L1924)
**Render**: `renderHorizontalLayerGLB(model, numLayers, resolution)` (L2244)
**Per-slice**: `renderLayerTopDown(model, resolution, floorY, ceilingY)` (L1956)

Key parameters:
- Layer count: `currentSettings.volumetric_layers` (default 4, per STRATEGY_TABLE) (L158)
- Resolution: `currentSettings.volumetric_resolution` (default 512) (L159)
- Adaptive layer count: `pickAdaptiveLayerCount` adds +1 if aspect > 1.5, +2 if > 2.5 (L2189-2198)
- Slice axis: `currentSettings.slice_axis` (default 'y'), resolved by `resolveSliceAxisRotation` (L2211)
  - `'y'`: identity
  - `'auto-horizontal'`: longer of X/Z becomes vertical
  - `'auto-thin'`: shortest axis becomes vertical
- Boundary modes (`currentSettings.slice_distribution_mode`, default 'visual-density'):
  - `'equal-height'`: linear interpolation of bbox (L2052)
  - `'visual-density'`: trunk-filtered weighted quantile (L2073) — discards bottom 10%, weights by radial distance
  - `'vertex-quantile'`: unweighted vertex Y quantile (L2151)
- Per-slice camera: orthographic, straight down
  - `halfExtent = max(size.x, size.z) * 0.55` (L1961)
  - Position: `(center.x, ceilingY + size.y * 2, center.z)` (L1970-1974)
  - Near/far: `0.01` / `camHeight - floorY + 0.01` (L1972)
  - `lookAt(center.x, floorY, center.z)` (L1975)
  - **Clipping plane**: `Plane(Vector3(0, -1, 0), ceilingY)` — removes everything above ceiling (L1978)
- Quad geometry: `createDomeGeometry(quadSize, domeHeight, 6)` — parabolic dome, 6 subdivisions (L2037)
  - `domeHeight = layerThickness * currentSettings.dome_height_factor` (default 0.5) (L2306)
- Naming: `vol_layer_{i}_h{baseMm}` where baseMm = `Math.round(floorY * 1000)` (L2312)
- Ground alignment: `exportScene.position.y = -boundaries[0]` when `currentSettings.ground_align` is true (default true) (L2322)
- Renderer: same settings (SRGB, ACES, bake_exposure), plus `localClippingEnabled = true` (L1969)
- Lighting: inline palette setup (same as setupBakeLights but positioned at clipping ceiling)

### 5. Shared Constants & Settings

**STRATEGY_TABLE** (L428-434):
| Category      | slice_axis       | slice_distribution_mode | volumetric_layers | instance_orientation_rule |
|---------------|------------------|-------------------------|-------------------|--------------------------|
| round-bush    | y                | visual-density          | 4                 | random-y                 |
| directional   | auto-horizontal  | equal-height            | 4                 | fixed                    |
| tall-narrow   | y                | equal-height            | 6                 | random-y                 |
| planar        | auto-thin        | equal-height            | 3                 | aligned-to-row           |
| hard-surface  | n/a              | n/a                     | 0                 | fixed                    |
| unknown       | y                | visual-density          | 4                 | random-y                 |

**Default settings** (`makeDefaults()`, L155-178):
- `volumetric_layers: 4`, `volumetric_resolution: 512`
- `dome_height_factor: 0.5`, `bake_exposure: 1.0`
- `ambient_intensity: 0.5`, `hemisphere_intensity: 1.0`
- `key_light_intensity: 1.4`, `bottom_fill_intensity: 0.4`
- `env_map_intensity: 1.2`, `alpha_test: 0.10`
- `slice_distribution_mode: 'visual-density'`, `slice_axis: 'y'`
- `ground_align: true`, `lighting_preset: 'default'`
- Tilted fade thresholds: `tilted_fade_low_start: 0.30`, `tilted_fade_low_end: 0.55`, `tilted_fade_high_start: 0.75`

**Lighting pipeline** (shared across all render types):
- `createBakeEnvironment(renderer)` (L1713): PMREM env map — reference palette > preset > RoomEnvironment fallback
- `setupBakeLights(offScene)` (L1656): AmbientLight + HemisphereLight + top DirectionalLight + bottom fill DirectionalLight
- `cloneModelForBake(model)` (L1743): clones materials, nulls envMap, boosts envMapIntensity

**Volumetric LOD chain** (L2373-2378):
| Level | Layers | Resolution | Label |
|-------|--------|------------|-------|
| 0     | 4      | 512        | vlod0 |
| 1     | 3      | 256        | vlod1 |
| 2     | 2      | 256        | vlod2 |
| 3     | 1      | 128        | vlod3 |

## Key Observations

1. **All renders use orthographic cameras** — no perspective anywhere in the bake pipeline.
2. **The 0.55 padding factor** appears in every camera sizing calculation (halfW, halfH, halfExtent).
3. **Billboard resolution is hardcoded at 512** in `renderMultiAngleBillboardGLB`, while volumetric resolution is tunable via settings.
4. **Tilted and side billboards use the same `renderBillboardAngle` function** — the only difference is the `elevationRad` parameter.
5. **Dome geometry is parabolic**, not flat — `createDomeGeometry` raises center vertices by `(1 - dist^2) * domeHeight`.
6. **Lighting is setting-driven** — all 5 intensity values and the lighting preset are per-asset tunables.
7. **Slice axis rotation** is a pre-transform + inverse pattern — the slicing always works in Y-up space.

## Constraints for Blender Translation

- Blender's orthographic camera uses a different parameterization (ortho_scale vs left/right/top/bottom).
- The ACES filmic tone mapping and SRGB output color space must be replicated in Blender's color management.
- The PMREM environment / RoomEnvironment fallback has no direct Blender equivalent; the gradient env texture approach is reproducible.
- Clipping planes for volumetric slices map to Blender's camera clip start/end + material shader nodes.
- The adaptive layer count and boundary algorithms operate on vertex positions — same logic in Python.
