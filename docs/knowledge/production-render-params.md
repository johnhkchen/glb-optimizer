# Production Render Parameters

Authoritative reference for all rendering parameters used by the client-side impostor
bake pipeline in `static/app.js`. Extracted by reading the JS source directly (T-014-01).
Intended consumers: T-014-02 (Blender script), T-014-06 (validation).

---

## 1. Common Renderer Settings

Every offscreen render (billboard, tilted, volumetric) creates a `THREE.WebGLRenderer`
with these settings:

| Parameter | Value | Source |
|-----------|-------|--------|
| antialias | `true` | app.js:L1400, `renderBillboardAngle()` |
| alpha | `true` | app.js:L1400 |
| preserveDrawingBuffer | `true` | app.js:L1400 |
| outputColorSpace | `THREE.SRGBColorSpace` | app.js:L1404 |
| toneMapping | `THREE.ACESFilmicToneMapping` | app.js:L1405 |
| toneMappingExposure | `currentSettings.bake_exposure` (default: `1.0`) | app.js:L1406, makeDefaults() L165 |
| clearColor | `0x000000` with alpha `0` (transparent) | app.js:L1403 |
| resolution | `512 x 512` pixels (billboards); `currentSettings.volumetric_resolution` for volumetric (default: `512`) | app.js:L1401/L1808/L159 |

**Blender equivalent**: Color Management > View Transform = "ACES" or "Filmic" with
exposure = 1.0. Film > Transparent = true. Output resolution 512x512.

---

## 2. Lighting Pipeline

### 2.1 Environment Map — `createBakeEnvironment(renderer)` (L1713)

Priority chain:
1. **Reference palette** (if loaded): 3-stop vertical gradient env (top=bright, mid=mid, bottom=dark)
2. **Active lighting preset** (if not 'default' and has `env_gradient`): preset's gradient
3. **RoomEnvironment fallback**: Three.js built-in neutral room environment

Gradient env texture construction (`buildGradientEnvTexture`, L1689):
- Canvas: 256w x 128h
- Linear gradient: top stop at 0, mid at 0.5, bottom at 1.0
- Mapping: `EquirectangularReflectionMapping`
- Color space: `SRGBColorSpace`
- Processed through `PMREMGenerator` for IBL

### 2.2 Direct Lights — `setupBakeLights(offScene)` (L1656)

Four lights added to every bake scene:

| Light Type | Color Source | Intensity Setting | Default | Position |
|------------|-------------|-------------------|---------|----------|
| AmbientLight | palette.bright | `ambient_intensity` | `0.5` | omnidirectional |
| HemisphereLight | sky=palette.bright, ground=palette.dark | `hemisphere_intensity` | `1.0` | omnidirectional |
| DirectionalLight (top key) | palette.bright | `key_light_intensity` | `1.4` | `(0, 10, 0)` |
| DirectionalLight (bottom fill) | palette.mid | `bottom_fill_intensity` | `0.4` | `(0, -10, 0)` |

### 2.3 Material Cloning — `cloneModelForBake(model)` (L1743)

- Deep-clones all mesh materials
- Sets `envMap = null` (Three.js auto-binds `scene.environment`)
- Sets `envMapIntensity = currentSettings.env_map_intensity` (default: `1.2`)

---

## 3. Side Billboards

**Functions**: `renderMultiAngleBillboardGLB(model, numAngles)` (L1802),
`renderBillboardAngle(model, angleRad, resolution, elevationRad=0)` (L1392)

### 3.1 Camera

| Parameter | Value / Formula | Source |
|-----------|----------------|--------|
| Type | Orthographic | L1413 |
| Angles | `BILLBOARD_ANGLES = 6`, evenly spaced: `angle = (i/6) * 2*PI` | L1311, L1809 |
| halfW | `max(size.x, size.z) * 0.55` | L1412 |
| halfH | `size.y * 0.55` (elevationRad=0: cosE=1, sinE=0) | L1411 |
| Near plane | `0.01` | L1413 |
| Far plane | `max(size.x, size.y, size.z) * 10` | L1413 |
| Distance | `maxDim * 2` from model center | L1418 |
| Position | `(center.x + sin(angle)*dist, center.y, center.z + cos(angle)*dist)` | L1419-1422 |
| LookAt | model bounding box center | L1423 |

**Blender equivalent**: Orthographic camera, `ortho_scale = max(halfW, halfH) * 2`.
Position on a circular orbit at distance `maxDim * 2`, elevation 0.

### 3.2 Geometry & Naming

| Parameter | Value | Source |
|-----------|-------|--------|
| Quad type | `PlaneGeometry(quadWidth, quadHeight)` | L1816 |
| Pivot | Bottom-edge: `translate(0, quadHeight/2, 0)` | L1817 |
| quadWidth | `halfW * 2` (returned from renderBillboardAngle) | L1452 |
| quadHeight | `halfH * 2` | L1452 |
| Naming | `billboard_0` through `billboard_5` | L1825 |
| Count | 6 side quads + 1 `billboard_top` = 7 total meshes | L1825, L1848 |

### 3.3 Material

| Parameter | Value | Source |
|-----------|-------|--------|
| Type | `MeshBasicMaterial` | L1818 |
| transparent | `true` | L1820 |
| side | `THREE.DoubleSide` | L1821 |
| alphaTest | `currentSettings.alpha_test` (default: `0.10`) | L1822 |
| map | `CanvasTexture` from rendered canvas, `SRGBColorSpace` | L1814-1815 |

---

## 4. Top-Down Billboard (`billboard_top`)

**Function**: `renderBillboardTopDown(model, resolution)` (L1759)

Called from within `renderMultiAngleBillboardGLB` — part of the side billboard GLB file.

### 4.1 Camera

| Parameter | Value / Formula | Source |
|-----------|----------------|--------|
| Type | Orthographic | L1777 |
| halfW | `size.x * 0.55` | L1774 |
| halfD | `size.z * 0.55` | L1775 |
| half | `max(halfW, halfD)` | L1776 |
| Frustum | `(-half, half, half, -half)` | L1777 |
| Near/Far | `0.01` / `size.y * 10` | L1777 |
| Position | `(center.x, center.y + size.y * 2, center.z)` | L1778 |
| LookAt | `center` | L1779 |

### 4.2 Geometry

| Parameter | Value | Source |
|-----------|-------|--------|
| Quad type | `PlaneGeometry(quadSize, quadSize)` | L1835 |
| Orientation | Flat on XZ plane: `rotateX(-Math.PI/2)` | L1836 |
| quadSize | `half * 2` (from camera sizing) | L1798 |
| Name | `billboard_top` | L1848 |
| Position | After last side variant (preview only; instancing ignores position) | L1849-1851 |

---

## 5. Tilted Billboards

**Function**: `renderTiltedBillboardGLB(model, numAngles, elevationRad, resolution)` (L1871)

Uses the same `renderBillboardAngle` as side billboards but with non-zero elevation.

### 5.1 Constants

| Constant | Value | Source |
|----------|-------|--------|
| `TILTED_BILLBOARD_ANGLES` | `6` | L1316 |
| `TILTED_BILLBOARD_ELEVATION_RAD` | `Math.PI / 6` = 30 degrees | L1317 |
| `TILTED_BILLBOARD_RESOLUTION` | `512` | L1318 |

### 5.2 Camera (differences from side)

The `renderBillboardAngle` formula with `elevationRad = PI/6`:

| Parameter | Formula | Computed Value (30deg) |
|-----------|---------|----------------------|
| cosE | `cos(PI/6)` | `0.866` |
| sinE | `sin(PI/6)` | `0.5` |
| halfH | `(size.y * 0.866 + maxHoriz * 0.5) * 0.55` | depends on model |
| halfW | `maxHoriz * 0.55` | same as side |
| cam.x | `center.x + sin(angle) * dist * 0.866` | reduced horizontal radius |
| cam.y | `center.y + dist * 0.5` | elevated above center |
| cam.z | `center.z + cos(angle) * dist * 0.866` | reduced horizontal radius |

### 5.3 Geometry & Naming

- Same `PlaneGeometry` with bottom-edge pivot as side billboards
- **No `billboard_top` quad** — side variants only (L1871-1908)
- Naming: `billboard_0` through `billboard_5` (L1893)
- Count: 6 quads total (no top)
- Uploaded to separate endpoint: `/api/upload-billboard-tilted/:id`

---

## 6. Volumetric Dome Slices

**Functions**: `renderHorizontalLayerGLB(model, numLayers, resolution)` (L2244),
`renderLayerTopDown(model, resolution, floorY, ceilingY)` (L1956)

### 6.1 Slice Axis Rotation — `resolveSliceAxisRotation(model, mode)` (L2211)

Before slicing, the model may be rotated so the chosen slice axis aligns with +Y:

| Mode | Behavior | Used By |
|------|----------|---------|
| `'y'` | Identity (no rotation) | round-bush, tall-narrow, unknown |
| `'auto-horizontal'` | Longer of X/Z becomes the slice axis | directional |
| `'auto-thin'` | Shortest of X/Y/Z becomes the slice axis | planar |

The inverse rotation is applied to the export scene root so the final GLB sits in
the original world frame.

### 6.2 Adaptive Layer Count — `pickAdaptiveLayerCount(model, baseLayers)` (L2189)

```
heightToWidth = size.y / max(size.x, size.z)
if heightToWidth > 2.5: return baseLayers + 2
if heightToWidth > 1.5: return baseLayers + 1
return baseLayers
```

### 6.3 Boundary Computation

Three modes, selected by `currentSettings.slice_distribution_mode`:

**`equal-height`** (L2052): Linear interpolation of bounding box.
```
for i in 0..numLayers:
    boundary[i] = minY + (i / numLayers) * (maxY - minY)
```

**`visual-density`** (L2073): Trunk-filtered, radially-weighted quantile.
```
1. Collect all vertex Y positions with world transform
2. Discard vertices below trunkY = minY + 0.10 * (maxY - minY)
3. Weight each vertex by radial distance / maxRadius (clamped to [0.05, 1.0])
4. Sort by Y, compute weighted cumulative distribution
5. Place interior boundaries at weighted quantile positions
6. Outer boundaries (0 and N) from unfiltered bounding box
```

**`vertex-quantile`** (L2151): Unweighted vertex Y quantile (legacy fallback).
```
1. Collect all vertex Y positions, sort ascending
2. boundary[i] = ys[floor(i/numLayers * count)]
3. boundary[0] = min, boundary[N] = max
```

### 6.4 Per-Slice Camera — `renderLayerTopDown(model, resolution, floorY, ceilingY)` (L1956)

| Parameter | Value / Formula | Source |
|-----------|----------------|--------|
| Type | Orthographic | L1971 |
| halfExtent | `max(size.x, size.z) * 0.55` | L1961 |
| Frustum | `(-halfExtent, halfExtent, halfExtent, -halfExtent)` | L1971-1972 |
| camHeight | `ceilingY + size.y * 2` | L1970 |
| Near/Far | `0.01` / `camHeight - floorY + 0.01` | L1972 |
| Position | `(center.x, camHeight, center.z)` | L1974 |
| LookAt | `(center.x, floorY, center.z)` | L1975 |
| Clipping | `Plane(Vector3(0, -1, 0), ceilingY)` — clips everything above ceiling | L1978 |
| localClippingEnabled | `true` | L1969 |

**Lighting**: Same palette and rig as `setupBakeLights`, but constructed inline with
the key light positioned at `(center.x, ceilingY + 20, center.z)` (L2003). No bottom
fill light in volumetric renders.

### 6.5 Dome Geometry — `createDomeGeometry(size, domeHeight, segments=6)` (L2037)

Builds a subdivided plane with parabolic Y bulge:

```
1. PlaneGeometry(size, size, segments, segments)
2. rotateX(-PI/2) — lay flat on XZ plane
3. For each vertex:
   dist = min(1, sqrt(x^2 + z^2) / (size/2))
   y = (1 - dist^2) * domeHeight
4. computeVertexNormals()
```

| Parameter | Value / Formula | Source |
|-----------|----------------|--------|
| size | `quadSize` = `halfExtent * 2` (from renderLayerTopDown) | L1961, L2030 |
| domeHeight | `layerThickness * currentSettings.dome_height_factor` (default: `0.5`) | L2306 |
| segments | `6` (hardcoded) | L2308 |
| layerThickness | `max(ceilingY - floorY, 0.001)` | L2299 |

### 6.6 Naming & Ground Alignment

| Parameter | Value | Source |
|-----------|-------|--------|
| Mesh name | `vol_layer_{i}_h{baseMm}` where `baseMm = Math.round(floorY * 1000)` | L2312 |
| Mesh position | `(0, floorY, 0)` | L2313 |
| Ground align | When `currentSettings.ground_align` (default: `true`): `exportScene.position.y = -boundaries[0]` | L2322 |

### 6.7 Material (per-slice)

Same as billboard material:

| Parameter | Value | Source |
|-----------|-------|--------|
| Type | `MeshBasicMaterial` | L2309 |
| transparent | `true` | L2311 |
| side | `THREE.DoubleSide` | L2311 |
| alphaTest | `currentSettings.alpha_test` (default: `0.10`) | L2311 |

---

## 7. STRATEGY_TABLE Reference

Source: `app.js:L428-434`

| Category | slice_axis | slice_distribution_mode | volumetric_layers | instance_orientation_rule |
|----------|-----------|------------------------|-------------------|--------------------------|
| round-bush | `y` | `visual-density` | `4` | `random-y` |
| directional | `auto-horizontal` | `equal-height` | `4` | `fixed` |
| tall-narrow | `y` | `equal-height` | `6` | `random-y` |
| planar | `auto-thin` | `equal-height` | `3` | `aligned-to-row` |
| hard-surface | `n/a` | `n/a` | `0` | `fixed` |
| unknown | `y` | `visual-density` | `4` | `random-y` |

Note: `hard-surface` has 0 volumetric layers — no dome slices are generated.

---

## 8. Volumetric LOD Chain

Source: `VOLUMETRIC_LOD_CONFIGS`, `app.js:L2373-2378`

| Level | Layers | Resolution | Label |
|-------|--------|------------|-------|
| 0 | 4 | 512 | vlod0 |
| 1 | 3 | 256 | vlod1 |
| 2 | 2 | 256 | vlod2 |
| 3 | 1 | 128 | vlod3 |

Each LOD calls `renderHorizontalLayerGLB` with reduced layer count and resolution.
Uploaded to `/api/upload-volumetric-lod/:id?level={N}`.

---

## 9. Validation Plan

For comparing Blender output against the known-good `1e562361...` asset intermediates:

### 9.1 Structural Checks

- **Side billboard GLB**: 7 meshes (`billboard_0`..`billboard_5` + `billboard_top`)
- **Tilted billboard GLB**: 6 meshes (`billboard_0`..`billboard_5`, no top)
- **Volumetric GLB**: N meshes named `vol_layer_{i}_h{mm}` where N = adaptive layer count
- All meshes use `MeshBasicMaterial` with transparent textures

### 9.2 Dimensional Checks

- Billboard textures: 512x512 pixels
- Volumetric textures: 512x512 (LOD0), decreasing per LOD table
- Quad world-space dimensions derived from model bounding box with 0.55 padding factor
- Dome geometry: 6x6 subdivision grid with parabolic height profile

### 9.3 Visual Comparison

1. Render the known-good asset in both JS (browser) and Blender with identical settings
2. Export textures from both GLBs
3. Per-texture RMSE comparison — threshold TBD based on tone mapping differences
4. Alpha channel comparison — binary mask should match within 1-2px border tolerance

### 9.4 File Size Ranges

Expected ranges for the known-good asset (to be populated during T-014-06 execution):
- Side billboard GLB: TBD KB
- Tilted billboard GLB: TBD KB
- Volumetric GLB (4 layers, 512px): TBD KB

### 9.5 Per-Asset Settings Sensitivity

Parameters that change per asset (and must be forwarded to Blender):
- `bake_exposure`, `alpha_test`, `dome_height_factor`
- `volumetric_layers`, `volumetric_resolution`
- `slice_distribution_mode`, `slice_axis`
- `ground_align`
- All 5 lighting intensities + `env_map_intensity`
- `lighting_preset`
- Shape category (drives STRATEGY_TABLE lookup)
