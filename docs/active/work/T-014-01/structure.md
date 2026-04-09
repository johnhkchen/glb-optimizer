# T-014-01 Structure: File-Level Changes

## Files Created

### `docs/knowledge/production-render-params.md` (new, ~300 lines)

The sole deliverable. A structured markdown document organized into 9 sections
as defined in the Design phase. No code changes — this is pure documentation.

**Section breakdown with estimated line counts:**

| Section | Title | Est. Lines |
|---------|-------|-----------|
| 1 | Common Renderer Settings | 25 |
| 2 | Lighting Pipeline | 40 |
| 3 | Side Billboards | 45 |
| 4 | Top-Down Billboard | 25 |
| 5 | Tilted Billboards | 30 |
| 6 | Volumetric Dome Slices | 80 |
| 7 | STRATEGY_TABLE Reference | 20 |
| 8 | Volumetric LOD Chain | 15 |
| 9 | Validation Plan | 30 |
| — | Header / TOC | 10 |

**Total**: ~320 lines

## Files Modified

None. This ticket produces documentation only.

## Files Deleted

None.

## Module Boundaries

Not applicable — no code modules are created or modified.

## Public Interfaces

The document serves as a reference interface between:
- **T-014-02** (Blender script): consumes all parameter values and algorithms
- **T-014-06** (validation): consumes the validation plan section
- **Future maintenance**: any change to app.js rendering should update this document

## Ordering

Single file, single step. No ordering dependencies within the implementation.

## Cross-References

Each parameter entry follows the format:
```
**parameter_name** = `value` | `formula`
Source: `app.js:L{N}` — `functionName()`
Overridable: yes/no (if yes, via `currentSettings.field_name`, default: `value`)
```

This format ensures:
1. The Blender author can find the exact source location
2. Per-asset tunability is explicit
3. Default values are captured for the common case

## Conventions

- Use fenced code blocks for pseudocode (algorithms like slice boundaries)
- Use inline code for numeric values and function names
- Use tables for tabular data (STRATEGY_TABLE, LOD configs)
- No diagrams or images — text-only for grep-ability and git-diff friendliness
