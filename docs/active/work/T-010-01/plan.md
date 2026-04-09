# T-010-01 Plan — Pack Metadata Schema

## Step sequence

### Step 1 — write `pack_meta.go`

- Package `main`, imports: `encoding/json`, `fmt`, `math`, `regexp`,
  `strings`.
- `const PackFormatVersion = 1`.
- `var speciesRe = regexp.MustCompile(\`^[a-z][a-z0-9_]*$\`)`.
- Three structs (`Footprint`, `FadeBand`, `PackMeta`) with JSON tags
  exactly matching E-002 §"Metadata".
- `Validate() error` — 13 ordered checks per `structure.md`.
- `ToExtras() map[string]any` — built field-by-field.
- `ParsePackMeta(json.RawMessage) (PackMeta, error)` — decode then
  validate, errors wrapped with `pack_meta:` prefix.

**Verification:** `go build ./...` succeeds.

### Step 2 — write `pack_meta_test.go`

Tests listed in `structure.md`. Specifically:

- `validMeta()` helper returns the canonical Achillea instance.
- `TestPackMeta_DefaultIsValid` — `validMeta().Validate()` returns nil.
- `TestPackMeta_RoundTrip` — uses the hand-typed `canonicalJSON`
  literal; asserts decoded fields equal the literal values; re-encodes
  and asserts the round-trip decodes back to the same struct.
- `TestPackMeta_RejectsBadVersion` — version 0 and 2.
- `TestPackMeta_RejectsBadSpecies` — table:
  `""`, `"1leading"`, `"with-dash"`, `"WithCaps"`, `"with.dot"`,
  `"with space"`.
- `TestPackMeta_RejectsMissingCommonName` — empty and whitespace-only.
- `TestPackMeta_RejectsMissingBakeID` — empty.
- `TestPackMeta_RejectsBadFootprint` — table: zero radius, negative
  height, NaN, +Inf.
- `TestPackMeta_RejectsFadeOutOfRange` — table: low_start = -0.1,
  high_start = 1.5.
- `TestPackMeta_RejectsFadeOutOfOrder` — table: low_start == low_end,
  low_end > high_start, all-equal-zero.
- `TestPackMeta_AllowsHighStartEqualOne` — degenerate but legal.
- `TestPackMeta_ToExtras` — builds the expected `map[string]any` by
  hand and `reflect.DeepEqual`s against `m.ToExtras()`.
- `TestParsePackMeta_RoundTrip` — `canonicalJSON` → struct, then
  feed `ToExtras()` through `json.Marshal` and decode again.
- `TestParsePackMeta_RejectsBadJSON` — `[]byte("{not json")`.
- `TestParsePackMeta_RejectsBadValues` — valid JSON, fade out of order.

**Verification:** `go test -run TestPackMeta -v ./...` and
`go test -run TestParsePackMeta -v ./...` both pass.

### Step 3 — full test sweep

`go test ./...` — must pass entirely. Confirms no inadvertent collision
with existing symbols (e.g. an existing `Footprint` or `Validate` clash
in `package main`).

### Step 4 — commit

Single commit, message:

```
T-010-01: Pack v1 metadata schema (types + Validate + ToExtras)
```

## Testing strategy

- **Unit only.** No integration tests. The schema has zero file I/O,
  zero network, zero glTF coupling — pure data validation.
- **Table-driven** for every "rejects bad X" case so future schema
  tightening (e.g. RFC3339 bake_id) lands as a one-line table addition.
- **Hand-written JSON literals** for round-trip — explicit anti-pattern
  guard from the ticket Notes. The literal lives as a `const` at the
  top of the test file; subsequent tests can re-use it via
  `[]byte(canonicalJSON)`.
- **Acceptance Criteria coverage matrix:**

  | AC bullet | Covered by |
  |---|---|
  | `PackMeta`/`Footprint`/`FadeBand` types with json tags | `TestPackMeta_RoundTrip` (verifies tag names by decoding hand-typed JSON) |
  | `Validate()` checks `format_version == 1` | `TestPackMeta_RejectsBadVersion` |
  | `Validate()` checks species regex | `TestPackMeta_RejectsBadSpecies` |
  | `Validate()` checks fade ordering | `TestPackMeta_RejectsFadeOutOfOrder` |
  | `Validate()` checks required fields | `TestPackMeta_RejectsMissing*` |
  | `ToExtras()` returns the embed shape | `TestPackMeta_ToExtras` |
  | `ParsePackMeta` round-trips | `TestParsePackMeta_RoundTrip` |
  | `PackFormatVersion` constant exists | `TestPackMeta_RejectsBadVersion` (uses it) |

## Verification criteria (DoD)

1. `go build ./...` — clean.
2. `go test ./...` — all pre-existing tests still pass; new tests
   pass.
3. `go vet ./...` — clean.
4. The exported surface listed in `structure.md` exists with the
   declared signatures.
5. No edits outside `pack_meta.go` and `pack_meta_test.go`.

## Risk register

| Risk | Mitigation |
|---|---|
| Symbol collision in `package main` | `Glob("**Pack**")` and `Grep("Footprint\|FadeBand")` before writing — confirm clean. |
| Tautological round-trip test | Hand-written JSON literal; assert against literal values, not against re-marshaled struct. |
| `checkRange` cannot express "strictly positive" | Inline `math.IsNaN`/`Inf`/`<=0` check for footprint dims. ~6 LOC, no helper extraction. |
| `regexp.MustCompile` cost on every Validate | Compiled once at package init via package-level `var`. |
| Future producer emits malformed `bake_id` | Documented as deferred tightening in design.md "Open question". |

## Deferred to follow-up tickets

- Strict `time.RFC3339` parsing for `bake_id` (revisit after T-011-03).
- Consumer-side default backfill (`fade = {0.30,0.55,0.75}` if absent)
  — that lives in plantastic, not here.
