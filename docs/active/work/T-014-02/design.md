# T-014-02 Design: Blender render_production.py Script

## 1. Decision: Script Architecture

### Option A: Monolithic Script (Single File)

One `scripts/render_production.py` file containing all rendering logic — argument
parsing, scene setup, lighting, camera positioning, billboard rendering, volumetric
slicing, dome geometry construction, and GLB export.

**Pros:**
- Follows the established pattern (`bake_textures.py` is 619 lines, `remesh_lod.py`
  is 277 lines — both monolithic)
- Single `//go:embed` directive if we embed it later
- Easy to invoke: one script path, no module dependencies
- Blender scripts run in a special Python environment — imports from external modules
  require `sys.path` manipulation; keeping everything in one file avoids this entirely

**Cons:**
- Will be ~600-800 lines (comparable to `bake_textures.py`)
- Three rendering modes in one file is dense

### Option B: Multi-File Package

Split into `scripts/render_production/` package with `__main__.py`, `billboards.py`,
`volumetric.py`, `lighting.py`, `geometry.py`.

**Pros:** Cleaner separation of concerns
**Cons:**
- Breaks the `blender -b --python script.py` convention
- Can't `//go:embed` a directory (only single files)
- Requires `sys.path` hacks for Blender's Python environment
- No existing precedent in this codebase

### Decision: **Option A — Monolithic Script**

The codebase convention is monolithic Blender scripts. The expected size (~700 lines)
is within the range of `bake_textures.py` (619). A single file is simpler to embed,
invoke, and debug.

---

## 2. Decision: Rendering Engine

### Option A: EEVEE (Rasterization)

- ~1 second per 512px frame
- Sufficient quality for billboard textures (clean silhouettes, correct color)
- `film_transparent` works out of the box
- The ticket explicitly recommends EEVEE

### Option B: Cycles (Ray-tracing)

- 10-30 seconds per 512px frame
- Higher quality lighting/shadows
- Overkill for billboard textures that get displayed at <100px in the final scene

### Decision: **Option A — EEVEE**

Per ticket guidance. Billboard textures need clean silhouettes, not photorealistic
lighting. EEVEE is ~10x faster and the quality is more than sufficient. Total render
time for one asset (6+1+6+4 = 17 renders at ~1s each) ≈ 20 seconds vs ~5 minutes
with Cycles.

---

## 3. Decision: Lighting Approach

### Option A: Direct Translation of Three.js Lights

Recreate each Three.js light type in Blender:
- AmbientLight → Blender doesn't have true ambient; fake with a dim area light or
  increase World shader ambient
- HemisphereLight → Two sun lamps (sky color from top, ground color from bottom)
- DirectionalLight → Sun lamp at the same position

**Pros:** Closest to JS pipeline output
**Cons:** Blender's light intensity scaling differs from Three.js; would need manual
tuning to approximate visual parity

### Option B: Simplified Studio Rig

Two sun lamps (key + fill) plus a world environment that provides ambient.
Tune intensities by eye against the JS pipeline output.

**Pros:** Simpler, easier to tune, fewer Blender API calls
**Cons:** Less systematic correspondence to JS settings

### Option C: Emission-Based Unlit Approach

Since billboard materials use `MeshBasicMaterial` (unlit) in Three.js, make the
imported model's materials emit their base color and render with no lights. This
captures the model's albedo directly.

**Pros:** Sidesteps the entire lighting mismatch problem
**Cons:** Doesn't match the JS pipeline (which does use lighting to modulate the
billboard renders even though the final billboards are unlit MeshBasicMaterial)

### Decision: **Option A — Direct Translation**

The JS pipeline applies real lighting during the offscreen render — the rendered
texture bakes in lighting. We need to approximate the same lighting to get similar
textures. Direct translation gives us the most systematic control. The intensity
values from T-014-01 (ambient=0.5, hemisphere=1.0, key=1.4, fill=0.4) will be
used as starting points, with a note that manual tuning may be needed for T-014-06.

---

## 4. Decision: Clipping Implementation for Volumetric Slices

### Option A: Blender Camera Clip Planes

Use Blender's camera `clip_start`/`clip_end` to limit what the camera sees.

**Pros:** Simple
**Cons:** Clips in camera space (Z-depth), not world Y. For a top-down orthographic
camera this should be equivalent — near plane at ceiling, far plane at floor.

### Option B: Boolean Modifier / Intersection

Use a cube boolean modifier to slice the model geometrically.

**Pros:** Clean geometric slicing
**Cons:** Modifies geometry, slow, may fail on non-manifold TRELLIS meshes

### Option C: Shader-Based Clipping

Use a material node that discards fragments above/below Y thresholds.

**Pros:** Per-pixel precision
**Cons:** Requires custom shader nodes per material, complex for multi-material models

### Decision: **Option A — Camera Clip Planes**

For a top-down orthographic camera, near/far clip maps directly to Y range. Set
`clip_start` so the near plane sits at `camHeight - ceilingY` and `clip_end` so the
far plane sits at `camHeight - floorY`. This is exactly analogous to the Three.js
approach (clipping plane at ceiling height, camera far plane at floor). Simple and
no geometry modification needed.

---

## 5. Decision: Slice Boundary Algorithms

### Option A: Full Reimplementation of All Three Modes

Reimplement `equal-height`, `visual-density`, and `vertex-quantile` in Python.

**Pros:** Feature-complete, handles all categories
**Cons:** `visual-density` is complex (~30 lines of vertex iteration, weighting,
and quantile computation)

### Option B: Only `equal-height` + `visual-density`

Skip `vertex-quantile` (legacy fallback, not used by any STRATEGY_TABLE category).

**Pros:** Less code
**Cons:** Technically incomplete if someone sets the mode manually

### Decision: **Option A — All Three Modes**

The `visual-density` algorithm is the most important (used by `round-bush` and
`unknown`, the two most common categories). `equal-height` is trivial. `vertex-quantile`
is simple too (just numpy-free sorted percentiles). Implementing all three is ~40
extra lines and ensures the script handles any configuration.

---

## 6. Decision: Config Input Format

### Option A: CLI Args Only

All parameters as `--arg value` flags.

### Option B: JSON Config Only

Single `--config params.json` argument.

### Option C: Both (CLI Args with JSON Override)

Support both `--config` and individual CLI args. JSON provides defaults; CLI args
override.

### Decision: **Option C — Both**

The ticket explicitly requires both modes. CLI args for manual/debug use, JSON config
for Go server integration. Implementation: if `--config` is provided, load JSON first,
then let any explicit CLI args override. This matches the pattern other tools use.

---

## 7. Decision: Dome Geometry Construction

### Option A: bmesh API

Build geometry vertex-by-vertex using `bmesh`, apply the parabolic height formula,
create faces manually.

**Pros:** Full control over vertex placement
**Cons:** More code, manual face construction

### Option B: Create PlaneGeometry via bpy.ops, Then Deform

Create a subdivided plane via `bpy.ops.mesh.primitive_plane_add()`, then iterate
vertices to apply the dome height formula.

**Pros:** Mirrors the Three.js approach (PlaneGeometry then vertex deformation)
**Cons:** Blender's `primitive_plane_add` subdivision parameter differs from Three.js

### Decision: **Option B — Primitive Plane + Deformation**

This mirrors the Three.js code path most closely. Create a `(segments+1)^2` grid
plane, rotate to XZ orientation, then apply `y = (1 - dist^2) * domeHeight` per
vertex. Recompute normals after deformation.

---

## 8. Decision: GLB Export Strategy

### Option A: One Export Call Per Variant Type

Build all quads for one variant (e.g., 7 side billboard quads) in a single scene,
export as one GLB.

### Option B: One Export Call Per Quad, Then Merge

Render and export each quad separately, merge in a post-processing step.

### Decision: **Option A — Scene-Level Export**

Matches the Three.js approach: all quads for a variant are children of a single
export scene. Simpler, fewer files, one GLB per variant type.

---

## 9. Output File Naming

Per ticket spec:
- `{output-dir}/{id}_billboard.glb` — side + top (7 meshes)
- `{output-dir}/{id}_billboard_tilted.glb` — tilted (6 meshes)
- `{output-dir}/{id}_volumetric.glb` — dome slices (N meshes)

These match the naming convention expected by the Go server's upload handlers.

---

## 10. Error Handling Strategy

- **Import failure**: If `bpy.ops.import_scene.gltf()` fails, exit with code 1 and
  a clear error message (this is the TRELLIS GLB import blocker)
- **Empty model**: If bounding box is zero, exit with error
- **Render failure**: EEVEE render failures are rare but possible; catch and report
- **Export failure**: Catch `export_scene.gltf()` errors
- **Progress output**: Print structured progress to stderr (matching `bake_textures.py`
  pattern) so the Go server can parse status

---

## 11. What Was Rejected

1. **Multi-file package** — breaks conventions and Go embed
2. **Cycles renderer** — too slow, no quality benefit for this use case
3. **Emission-based unlit** — doesn't match the lit JS pipeline rendering
4. **Boolean modifier clipping** — fragile on TRELLIS meshes
5. **CLI-only args** — ticket requires JSON config for server integration
6. **bmesh manual geometry** — unnecessary when plane primitive + deformation works
