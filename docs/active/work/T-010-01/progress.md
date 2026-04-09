# T-010-01 Progress — Pack Metadata Schema

## Status: implementation complete, all tests green

## Steps executed

### Step 1 — `pack_meta.go` ✅
Created. Contents per `structure.md`:
- `const PackFormatVersion = 1`
- `var speciesRe` (compiled once)
- `Footprint`, `FadeBand`, `PackMeta` structs with json tags
- `PackMeta.Validate() error` — 13 ordered checks
- `checkPositive(name, v)` helper (strict-positive variant of
  `checkRange`; cannot reuse `checkRange` because it's an inclusive
  bound)
- `PackMeta.ToExtras() map[string]any` — built field-by-field
- `ParsePackMeta(json.RawMessage) (PackMeta, error)` — decode then
  validate, errors prefixed `pack_meta:`

`go build ./...` clean.
`go vet ./...` clean.

### Step 2 — `pack_meta_test.go` ✅
Created. 14 tests, all passing:

```
PASS  TestPackMeta_DefaultIsValid
PASS  TestPackMeta_RoundTrip                    (hand-typed JSON literal)
PASS  TestPackMeta_RejectsBadVersion
PASS  TestPackMeta_RejectsBadSpecies            (8 cases)
PASS  TestPackMeta_RejectsMissingCommonName
PASS  TestPackMeta_RejectsMissingBakeID
PASS  TestPackMeta_RejectsBadFootprint          (7 cases)
PASS  TestPackMeta_RejectsFadeOutOfRange        (4 cases)
PASS  TestPackMeta_RejectsFadeOutOfOrder        (5 cases)
PASS  TestPackMeta_AllowsHighStartEqualOne
PASS  TestPackMeta_ToExtras
PASS  TestParsePackMeta_RoundTrip
PASS  TestParsePackMeta_RejectsBadJSON
PASS  TestParsePackMeta_RejectsBadValues
```

### Step 3 — full test sweep ✅
`go test ./...` → `ok glb-optimizer 0.658s`. No regressions in any
pre-existing test (settings, classify, accepted, profiles, strategy,
analytics, handlers).

### Step 4 — commit
**Not yet executed.** Awaiting Lisa's commit step or human go-ahead.
The two new files are staged-ready; nothing else in the working tree
was touched by this ticket.

## Deviations from plan

**One deviation:** Plan said reuse `checkRange` for footprint dims.
Implementation introduced a small `checkPositive` helper instead.

Reason: `checkRange("footprint.canopy_radius_m", v, 0, math.MaxFloat64)`
would *accept* zero (because `checkRange` uses `v < lo`, an inclusive
lower bound), but the contract requires strict positivity. Inlining
the strict check ~3 places would be ugly; one tiny named helper
preserves readability and is reusable for any future strict-positive
field. Helper is 9 LOC, sits next to `Validate` in `pack_meta.go`,
mirrors `checkRange`'s shape exactly. This was already flagged as a
risk in `plan.md` ("inline ~6 LOC"); I chose a named helper instead
of inline duplication.

## Verification against AC

| AC bullet | Status |
|---|---|
| `pack_meta.go` defines `PackMeta`, `Footprint`, `FadeBand` | ✅ |
| JSON tags lowercase_with_underscores | ✅ verified by `TestPackMeta_RoundTrip` decoding hand-typed JSON |
| `Validate()` checks `format_version == 1` | ✅ |
| `Validate()` checks species regex `^[a-z][a-z0-9_]*$` | ✅ |
| `Validate()` checks fade ordering | ✅ |
| `Validate()` checks required fields | ✅ |
| `ToExtras()` returns embed shape | ✅ |
| `ParsePackMeta` round-trips | ✅ |
| Unit tests for valid round-trip | ✅ |
| Unit tests for invalid species | ✅ |
| Unit tests for fade out of order | ✅ |
| Unit tests for missing required field | ✅ |
| `const PackFormatVersion = 1` | ✅ |

All 13 AC bullets covered.

## Files touched

- **created** `pack_meta.go` (137 lines)
- **created** `pack_meta_test.go` (260 lines)
- **created** `docs/active/work/T-010-01/{research,design,structure,plan,progress}.md`

No edits to any existing source file. Zero blast radius outside the
new files.
