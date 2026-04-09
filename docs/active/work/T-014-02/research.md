# T-014-02 Research: Blender render_production.py Script

## 1. Ticket Summary

Write `scripts/render_production.py` — a headless Blender Python script that renders
the four impostor variants (side billboard, top-down billboard, tilted billboard,
volumetric dome slices) for one GLB asset. Must produce the same three intermediate
GLB files that the client-side JS pipeline in `static/app.js` currently creates.

Depends on T-014-01 (production render params doc, completed).

---

## 2. Existing Blender Script Patterns

### 2.1 scripts/bake_textures.py (619 lines)

- **Guard pattern**: `try: import bpy` with fallback error message + `sys.exit(1)`
- **CLI args**: `parse_args()` extracts args after `--` separator from `sys.argv`
- **Scene lifecycle**: `clear_scene()` removes all objects/materials/images before work
- **Import**: `bpy.ops.import_scene.gltf(filepath=...)`
- **Export**: `bpy.ops.export_scene.gltf(filepath=..., export_format='GLB', ...)`
- Uses `argparse` with typed args, choices, and defaults

### 2.2 scripts/remesh_lod.py (277 lines)

- **No guard**: assumes Blender context directly (`import bpy` at top level)
- Same `parse_args()` pattern with `--` separator
- Same `clear_scene()` pattern
- Both scripts are invoked via `blender -b --python script.py -- [args]`

### 2.3 blender.go (136 lines) — Go-side invocation

- `DetectBlender()` searches macOS `.app` paths then PATH
- `RunBlenderLOD()` builds arg list and calls `exec.Command(info.Path, args...)`
- Sets `BLENDER_SYSTEM_RESOURCES` env var for macOS app bundles
- `WriteEmbeddedScript()` writes `//go:embed` script to temp dir
- Pattern: Go builds CLI arg list, passes to Blender via `--` separator

### 2.4 Invocation convention

```
blender -b --python scripts/render_production.py -- \
    --source path.glb --output-dir dir/ --id assetId \
    --category round-bush --resolution 512 ...
```

Alternative: `--config params.json` for Go server integration (all params in one file).

---

## 3. Client-Side Rendering Pipeline (static/app.js, ~5091 lines)

The JS pipeline renders four impostor types via WebGL (THREE.js offscreen renderer).
All parameters are documented in `docs/knowledge/production-render-params.md` (T-014-01).

### 3.1 Side Billboards — renderMultiAngleBillboardGLB (L1802)

- Renders `BILLBOARD_ANGLES=6` side views at evenly-spaced azimuths
- Each view: orthographic camera at `dist = maxDim * 2`, halfW/halfH from bbox
- Plus one top-down view (`billboard_top`)
- Output: 7 named quads (`billboard_0`..`billboard_5` + `billboard_top`)
- Each quad: `PlaneGeometry`, bottom-edge pivot (translate up by halfHeight)
- Material: `MeshBasicMaterial`, transparent, DoubleSide, alphaTest=0.10
- Exported as single GLB via `GLTFExporter`

### 3.2 Tilted Billboards — renderTiltedBillboardGLB (L1871)

- Same as side but with `elevationRad = PI/6` (30 degrees)
- Camera position elevated: `y = center.y + dist * sin(elevationRad)`
- Camera horizontal radius reduced by `cos(elevationRad)`
- **No top-down quad** — 6 side quads only
- Naming: `billboard_0`..`billboard_5`

### 3.3 Top-Down Billboard — renderBillboardTopDown (L1759)

- Camera at `center.y + size.y * 2`, looking straight down
- halfW from `size.x * 0.55`, halfD from `size.z * 0.55`, half = max
- Square quad, rotated flat on XZ plane (`rotateX(-PI/2)`)
- Included in the side billboard GLB (not a separate file)

### 3.4 Volumetric Dome Slices — renderHorizontalLayerGLB (L2244)

- Slice axis rotation based on category (`y`, `auto-horizontal`, `auto-thin`)
- Adaptive layer count: base layers +0/+1/+2 depending on aspect ratio
- Three boundary computation modes: `equal-height`, `visual-density`, `vertex-quantile`
- Per-slice: orthographic camera from above, clipping plane at ceiling height
- Dome geometry: `PlaneGeometry` with parabolic Y bulge (6x6 segments)
- Naming: `vol_layer_{i}_h{baseMm}`
- Ground alignment: shift scene.position.y = -boundaries[0]

---

## 4. Downstream Consumer: CombinePack (combine.go)

`CombinePack(side, tilted, volumetric []byte, meta PackMeta) ([]byte, error)`

- Expects three GLB byte slices (tilted and volumetric may be nil)
- Routes meshes by name pattern:
  - `billboard_0`..`billboard_N` + `billboard_top` -> view_side + view_top
  - `billboard_0`..`billboard_N` (from tilted) -> view_tilted
  - `vol_layer_*` -> view_dome (Y-sorted by POSITION accessor min)
- **Critical**: mesh naming must match exactly or routing fails
- 5 MiB hard cap on combined pack size

---

## 5. Blender/Three.js Rendering Equivalences

### 5.1 Renderer

| Three.js | Blender Equivalent |
|----------|-------------------|
| WebGLRenderer + alpha | EEVEE with film_transparent=True |
| ACESFilmicToneMapping | Color Management: "Filmic" view transform |
| toneMappingExposure=1.0 | Exposure=1.0 in Color Management |
| SRGBColorSpace output | Output color space sRGB (default) |
| 512x512 resolution | Render resolution 512x512 |
| clearColor 0x000000 alpha 0 | Transparent background via film_transparent |

### 5.2 Lighting

| Three.js | Blender Equivalent |
|----------|-------------------|
| AmbientLight intensity=0.5 | World shader ambient term or low-power area lights |
| HemisphereLight intensity=1.0 | Sky texture gradient or two-point light rig |
| DirectionalLight top key=1.4 | Sun lamp at (0,10,0), strength ~1.4 |
| DirectionalLight bottom fill=0.4 | Sun lamp at (0,-10,0), strength ~0.4 |
| RoomEnvironment (default preset) | Simple HDRI or gradient environment |

Note: Exact 1:1 lighting match is impossible (different renderers). Goal is visual
parity — clean silhouettes with correct color, not pixel-identical output.

### 5.3 Camera

| Three.js | Blender Equivalent |
|----------|-------------------|
| OrthographicCamera | Camera type=ORTHO, ortho_scale = max(halfW,halfH)*2 |
| near=0.01, far=maxDim*10 | clip_start=0.01, clip_end=maxDim*10 |

### 5.4 Material / Export

| Three.js | Blender Equivalent |
|----------|-------------------|
| MeshBasicMaterial | BSDF with Emission socket (unlit look) |
| GLTFExporter binary:true | bpy.ops.export_scene.gltf(export_format='GLB') |
| alphaTest=0.10 | Alpha clip threshold in material or post-processing |

---

## 6. Key Constraints and Risks

### 6.1 Quad Sizing Parity

The three.js code uses `halfW = max(size.x, size.z) * 0.55` and `halfH = size.y * 0.55`
for side billboards. The Blender script MUST use identical formulas. If quad dimensions
differ, the plantastic instancer's scale math breaks.

### 6.2 TRELLIS GLB Import

Blender must successfully import TRELLIS-format GLB files. Test model: `inbox/dahlia_blush.glb`.
This is flagged as an early blocker — if Blender can't import the model, the entire
approach needs rethinking.

### 6.3 Mesh Naming Convention

CombinePack routes meshes by name. The Blender output MUST use:
- `billboard_0`..`billboard_{N-1}` + `billboard_top` (side)
- `billboard_0`..`billboard_{N-1}` (tilted, no top)
- `vol_layer_{i}_h{baseMm}` (volumetric)

### 6.4 Alpha / Transparency

Both Three.js (`preserveDrawingBuffer` + alpha renderer) and Blender (`film_transparent`)
produce premultiplied RGBA. Should be compatible but needs visual verification.

### 6.5 Dome Geometry

The parabolic dome geometry is not a standard Blender primitive. Must be constructed
programmatically using `bmesh` or direct vertex manipulation — same formula as JS:
`y = (1 - dist^2) * domeHeight` where `dist = sqrt(x^2+z^2) / (size/2)`.

### 6.6 Visual-Density Slice Distribution

The `visual-density` boundary algorithm reads vertex positions, filters by trunk height,
weights by radial distance, and computes weighted quantiles. This must be reimplemented
in Python using the imported model's mesh data.

---

## 7. Existing Test Infrastructure

- Go tests use `makeMinimalGLB()` to synthesize test GLBs from scratch
- `scripts/verify-pack.mjs` validates pack structure
- Smoke test target: `inbox/dahlia_blush.glb` (28MB TRELLIS model)
- No existing Python test framework for Blender scripts
- Acceptance: run `glb-optimizer pack <id>` on Blender output, then `verify-pack.mjs`

---

## 8. Config/JSON Input Mode

The ticket specifies `--config params.json` as an alternative to CLI args. This is
for Go server integration (T-014-03 / T-014-04) where the server writes all parameters
to a JSON file and passes the path. The JSON schema should mirror the CLI arg names.

---

## 9. Files to Create/Modify

| File | Action | Purpose |
|------|--------|---------|
| `scripts/render_production.py` | Create | Main Blender rendering script |
| `blender.go` | Modify (later, T-014-03) | Add `RunBlenderRender()` function |

No Go code changes in this ticket — only the Python script.
