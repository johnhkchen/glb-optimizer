# T-014-01 Design: Extract Rendering Parameters

## Problem Statement

We need to produce `docs/knowledge/production-render-params.md` — a single authoritative
document containing every numeric parameter and algorithm the Blender script (T-014-02)
must replicate. The document must be extracted from actual JS code, not from comments or
docs.

## Design Options

### Option A: Flat Parameter Table

A single markdown file with one table per render type listing parameter name, value,
and source line. Compact, easy to scan.

**Pros**: Simple, grep-friendly, minimal structure.
**Cons**: Loses algorithmic context (e.g., how boundaries are computed, how adaptive
layer count works). A table can't capture "halfH depends on elevationRad which changes
per render type."

### Option B: Structured Reference Document with Sections

Organized by render type with subsections for camera, geometry, material, lighting,
and algorithms. Each parameter includes its value, the formula if computed, and the
source line reference. Algorithmic parameters (slice boundaries, adaptive layers) get
pseudocode blocks.

**Pros**: Complete enough for a developer to implement without referring back to app.js.
Algorithms are explicit. Validation section maps naturally.
**Cons**: Longer (~300 lines). More effort to maintain if app.js changes.

### Option C: JSON Schema + Markdown Wrapper

Define parameters as a JSON schema, wrap in markdown for readability. Blender script
could potentially parse the JSON directly.

**Pros**: Machine-readable.
**Cons**: Over-engineered for this ticket. The Blender script will hardcode values, not
parse a schema at runtime. JSON doesn't capture algorithms well.

## Decision: Option B — Structured Reference Document

**Rationale**: The document's primary consumer is the T-014-02 Blender script author.
They need to understand not just "what value" but "how the value is derived" for computed
parameters (camera framing, slice boundaries, adaptive layer count). Option B provides
this without the overhead of Option C.

The ~300 line length is acceptable — this is a knowledge document, not a phase artifact
with a soft 200-line target.

## Document Structure

```
# Production Render Parameters
## 1. Common Renderer Settings
## 2. Lighting Pipeline
## 3. Side Billboards (renderBillboardGLB)
   ### 3.1 Camera
   ### 3.2 Geometry & Naming
   ### 3.3 Material
## 4. Top-Down Billboard (billboard_top)
   ### 4.1 Camera
   ### 4.2 Geometry
## 5. Tilted Billboards (renderTiltedBillboardGLB)
   ### 5.1 Camera (differences from side)
   ### 5.2 Geometry & Naming
## 6. Volumetric Dome Slices (renderHorizontalLayerGLB)
   ### 6.1 Slice Axis Rotation
   ### 6.2 Boundary Computation
   ### 6.3 Per-Slice Camera
   ### 6.4 Dome Geometry
   ### 6.5 Naming & Ground Alignment
## 7. STRATEGY_TABLE Reference
## 8. Volumetric LOD Chain
## 9. Validation Plan
```

## Line Reference Format

Each parameter will include `(app.js:L{N}, {functionName})` — e.g.:
`resolution = 512 (app.js:L1808, renderMultiAngleBillboardGLB)`

This satisfies the acceptance criterion of line references for every parameter.

## Validation Plan Design

The validation section will specify:
1. **Per-type checks**: quad count, texture dimensions, naming conventions
2. **Size ranges**: expected file size bounds for the known-good `1e562361...` asset
3. **Visual comparison**: render both JS and Blender at same settings, diff textures
4. **Structural comparison**: parse both GLBs, compare mesh names, vertex counts, UV layouts

This section is forward-looking (consumed by T-014-06) but establishes the criteria now
while all the parameters are fresh.

## What Was Rejected

- **Option A** was rejected because it doesn't capture computed parameters (camera
  sizing formulas, slice boundary algorithms). A flat table would require the Blender
  author to reverse-engineer relationships.
- **Option C** was rejected because there's no runtime consumer of machine-readable
  parameter data. The Blender script will embed values directly.
- **Generating params automatically from AST parsing** was considered but rejected —
  the JS functions use runtime state (`currentSettings`, `currentModel` bounding box)
  that can't be statically extracted. Human reading is required.

## Risk

- **Stale line numbers**: If app.js changes between this ticket and T-014-02, line
  references will drift. Mitigation: include function names alongside line numbers so
  the reader can locate parameters even if lines shift.
- **Settings dependency**: Many parameters come from `currentSettings` which is per-asset.
  The document will list defaults but must note which values are overridable.
