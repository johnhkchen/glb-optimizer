# T-014-01 Review: Extract Rendering Parameters

## Summary

Created `docs/knowledge/production-render-params.md` (~310 lines), documenting every
rendering parameter from `static/app.js` used by the four impostor artifact types.

## Files Changed

| File | Action | Lines |
|------|--------|-------|
| `docs/knowledge/production-render-params.md` | Created | ~310 |

No code files were modified. No existing files were changed.

## Acceptance Criteria Verification

| Criterion | Status |
|-----------|--------|
| Covers all four render types (side, top-down, tilted, volumetric) | Pass |
| Numeric values extracted from JS source | Pass |
| Each parameter has line reference + function name | Pass |
| Validation plan section included | Pass |
| Read actual JS code, not docs/comments | Pass — all values verified against function bodies |

## Test Coverage

Not applicable — this ticket produces documentation only. No code was written or
modified. Existing test suites are unaffected.

Verified with `go test ./...` — all tests continue to pass (no changes made to Go code).

## Key Decisions

1. **Structured reference format** (Option B from design) — organized by render type with
   subsections for camera/geometry/material rather than a flat parameter table.
2. **Pseudocode for algorithms** — slice boundary computation and adaptive layer count
   documented as pseudocode blocks rather than prose, for direct translation to Python.
3. **Computed vs static values** — clearly distinguished parameters that are constants
   (BILLBOARD_ANGLES=6) from those derived at runtime (halfH depends on model bbox and
   elevationRad).

## Open Concerns

1. **Line numbers will drift**: `app.js` is actively developed. Line references (e.g.,
   L1392) will become stale as the file changes. Mitigated by including function names
   alongside every line reference, so parameters can be located by searching for the
   function name.

2. **Tone mapping parity**: Three.js ACES Filmic tone mapping may not match Blender's
   implementation exactly. The validation plan (Section 9.3) accounts for this with
   per-texture RMSE comparison rather than pixel-exact matching.

3. **File size ranges TBD**: Validation plan Section 9.4 leaves file size ranges as TBD —
   these must be populated during T-014-06 execution when the known-good asset is
   available for measurement.

4. **Volumetric lighting difference**: `renderLayerTopDown` constructs its lights inline
   rather than calling `setupBakeLights`, with two differences: (a) key light is positioned
   at `ceilingY + 20` instead of `(0, 10, 0)`, and (b) no bottom fill light. This is
   documented in Section 6.4 but worth verifying the Blender script handles correctly.

5. **Per-asset settings forwarding**: The document identifies 12+ settings that vary per
   asset (Section 9.5). T-014-02/T-014-04 must design a mechanism to pass these from the
   Go server to the Blender script — this ticket documents *what* needs to be passed.

## TODOs for Downstream Tickets

- **T-014-02**: Use this document as the Blender script specification. Pay special
  attention to ortho camera parameterization differences (Three.js left/right/top/bottom
  vs Blender ortho_scale).
- **T-014-06**: Populate file size ranges in Section 9.4. Execute the visual comparison
  methodology in Section 9.3.
