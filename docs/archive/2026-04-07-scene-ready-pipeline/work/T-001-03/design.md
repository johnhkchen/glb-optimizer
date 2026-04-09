# Design: T-001-03 Rose Volumetric Distillation

## Decision: Cross-Plane Impostor with Configurable Slice Count

The implementation uses intersecting textured planes through the model center (the "cross-tree"
pattern standard in game vegetation), generated client-side using the same offscreen rendering
pipeline as the existing billboard system.

## Approaches Evaluated

### Option A: True Depth Slicing (Rejected)

Render thin cross-sections of the model using clipping planes, producing quads that show only
the geometry at each depth layer.

- **Pro**: Most faithful to the "MRI slicing" metaphor in the ticket description.
- **Con**: Thin slices produce sparse, disconnected fragments — a rose bush at 6-8 slices has
  most pixels transparent. The visual result is ghostly stripes rather than a recognizable plant.
- **Con**: Requires custom shader or clipping plane manipulation not available in standard
  Three.js material pipeline without `renderer.clippingPlanes` setup.
- **Con**: The stacked result only looks correct from the slicing axis direction.
- **Verdict**: Technically interesting but produces poor visual quality for organic shapes.

### Option B: Cross-Plane Impostor (Selected)

Render full orthographic views from N angles, place the resulting textured quads as planes
intersecting through the model's center at the same angles.

- **Pro**: Industry-proven technique for vegetation (used in Unity SpeedTree, Unreal foliage).
- **Pro**: Looks reasonable from any viewing angle — always at least one plane is near-frontal.
- **Pro**: Maps directly to existing billboard rendering code — same camera setup, same
  material creation, same GLTFExporter pipeline.
- **Pro**: Simple geometry (intersecting planes) with no complex shader requirements.
- **Pro**: `alphaTest` cutout avoids transparency sorting issues at intersections.
- **Con**: Not truly volumetric — the "depth" is faked by intersecting planes.
- **Verdict**: Best balance of visual quality, implementation simplicity, and budget compliance.

### Option C: Layered Parallel Planes (Rejected)

Render from one direction, capture multiple depth layers via depth peeling, stack parallel
planes at increasing distances.

- **Pro**: True depth layering — each plane shows what's visible at that depth.
- **Con**: Depth peeling requires multiple render passes with custom framebuffer management.
- **Con**: Only looks correct from the rendering direction — severe parallax from other angles.
- **Con**: More complex than cross-planes with worse multi-angle visual quality.
- **Verdict**: More complex, worse results for vegetation.

## Architecture

### Slice Geometry Layout

For 6 slices (default):
- 6 vertical planes, each rotated `i * (180/6)` degrees around Y axis (0, 30, 60, 90, 120, 150)
- Note: 180 degrees coverage, not 360 — each plane is double-sided, covering both directions
- 1 horizontal plane at ~70% of model height for canopy cap
- 1 horizontal plane at ~20% of model height for stem/base fill (optional, count-dependent)

Total: 6-8 quads = 12-16 triangles. Meets acceptance criteria.

### Rendering Pipeline

Reuse the billboard rendering approach with modifications:

1. **Camera setup**: Orthographic camera at each slice angle, same as `renderBillboardAngle`
2. **Quad placement**: Instead of spacing quads apart for billboard preview, position each
   quad at the model center, rotated to match its capture angle
3. **Horizontal quads**: Render top-down (reuse `renderBillboardTopDown`), place at
   configurable Y heights within the model bounds
4. **Texture resolution**: 256x256 per quad (tunable). At this resolution, 7 PNG textures
   with alpha fit within ~80-90KB when exported via GLTFExporter.

### Key Differences from Billboard

| Aspect | Billboard | Volumetric |
|--------|-----------|------------|
| Quad placement | Side by side, outside model | Intersecting at center |
| Rotation | All face same direction initially | Each rotated to capture angle |
| Camera-facing | Yes (runtime) | No (static placement) |
| Horizontal quad | 1, at model top | 1-2, within canopy volume |
| Use case | LOD5+ / far distance | LOD4 / medium distance |
| Quad naming | `billboard_N` | `volumetric_N`, `volumetric_top` |

### Integration Design

**New endpoint**: Not needed. Reuse the billboard upload pattern with a new filename suffix.
The server already routes by version string in `handlePreview`.

**Server changes** (minimal):
- `models.go`: Add `HasVolumetric bool` field to `FileRecord`
- `handlers.go`: Add `volumetric` case to `handlePreview` switch, add upload endpoint
  `handleUploadVolumetric`, add cleanup in `handleDeleteFile`
- `main.go`: Register the new upload route

**Frontend changes**:
- New function `generateVolumetric(id)` — similar to `generateBillboard` but with
  cross-plane placement
- New function `renderVolumetricGLB(model, numSlices)` — renders slices and creates
  intersecting-plane GLB
- Toolbar: Add "Volumetric" button next to "Billboard" button
- LOD toggle: Add "volumetric" option alongside "billboard"
- Stress test: Volumetric instances are static (no camera-facing update needed)
- Preview stats: Show slice count and per-slice resolution

### Configurable Parameters

- **Slice count**: 4-8 (default 6). Exposed as parameter in generation function.
- **Texture resolution**: 128/256/512 (default 256). Affects quality vs file size.
- **Horizontal slices**: 0-2 (default 1 at 70% height). Top-down canopy cap.
- **Alpha test threshold**: 0.1-0.5 (default 0.1). Higher = harder cutout, less bleed.

These are hardcoded defaults initially. A settings UI can be added later if needed.

### File Size Budget

With 6 vertical slices + 1 horizontal at 256x256:
- Geometry: ~2KB (7 quads, 14 triangles, 28 vertices)
- Textures: 7 x ~12KB (PNG with alpha, foliage content compresses well) = ~84KB
- GLB overhead: ~3KB
- **Total: ~89KB** — within the <100KB target

If over budget, reduce to 128x128 (~4KB per texture) for ~31KB total, or reduce slice count.

### Depth Sorting Strategy

Intersecting transparent planes are a well-known rendering challenge. The approach:
- Use `alphaTest: 0.1` (cutout) as primary — avoids all sorting issues
- Set `depthWrite: true` (default for cutout materials)
- `side: THREE.DoubleSide` so planes are visible from both directions
- `MeshBasicMaterial` — no lighting calculation needed, baked appearance

This matches the existing billboard material setup and avoids the transparency sorting
artifacts mentioned in the acceptance criteria.

### Preview Integration

When viewing volumetric version:
- Load the volumetric GLB normally via GLTFLoader
- The quads are pre-positioned at their correct intersecting angles
- No special camera-facing logic needed (unlike billboards)
- Wireframe mode works naturally on the quad geometry
- Stats show: 14 triangles, 28 vertices, file size

For stress testing with LOD distribution:
- Volumetric instances can be placed directly (no per-frame rotation)
- Use `InstancedMesh` for each quad group (or clone the model group per instance)
- Simpler than billboard instancing since no camera-facing updates

## Rejected Enhancement: Texture Atlas

Packing all slice textures into a single atlas would reduce GLB overhead and potentially
improve rendering (fewer draw calls, single texture bind). Rejected for now because:
- GLTFExporter doesn't support atlas packing natively
- Would require manual UV manipulation on exported geometry
- Individual textures are simpler and the file size budget is achievable without it
- Can be added as an optimization later if needed
