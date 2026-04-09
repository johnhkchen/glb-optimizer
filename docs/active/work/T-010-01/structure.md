# T-010-01 Structure — Pack Metadata Schema

## File-level changes

| File | Action | Purpose |
|---|---|---|
| `pack_meta.go` | **create** | Types, constant, validation, ToExtras, ParsePackMeta |
| `pack_meta_test.go` | **create** | Unit tests for validation, round-trip, parse |

No other files are touched. No existing functions are renamed, moved, or
modified. This is fully additive — zero blast radius outside the two new
files.

## `pack_meta.go` — outline

```go
package main

import (
    "encoding/json"
    "fmt"
    "regexp"
    "strings"
)

// PackFormatVersion is the on-disk schema version for Pack v1 metadata
// embedded in the root scene's extras.plantastic block of every asset
// pack GLB. Bumped on breaking schema changes. See
// docs/active/epics/E-002-asset-pack-format.md for the contract.
const PackFormatVersion = 1

// speciesRe enforces the lowercase-latin-name slug rule for species ids:
// must start with [a-z] and contain only [a-z0-9_] thereafter. Compiled
// once at package init.
var speciesRe = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// Footprint records the per-instance physical dimensions the consumer
// uses to scale unit-reference geometry and to lay out no-walk zones.
type Footprint struct {
    CanopyRadiusM float64 `json:"canopy_radius_m"`
    HeightM       float64 `json:"height_m"`
}

// FadeBand records the three crossfade thresholds (band 1 start/end,
// band 2 start) used by the consumer's hybrid-impostor renderer.
// Band 2 end is implicit 1.0. All values are in [0,1] of |dot(camDir,-Y)|
// and must satisfy low_start < low_end < high_start <= 1.0.
type FadeBand struct {
    LowStart  float64 `json:"low_start"`
    LowEnd    float64 `json:"low_end"`
    HighStart float64 `json:"high_start"`
}

// PackMeta is the v1 contract for the metadata embedded at
// scene.extras.plantastic of every asset pack GLB. Field declaration
// order matches the canonical example in E-002.
type PackMeta struct {
    FormatVersion int       `json:"format_version"`
    BakeID        string    `json:"bake_id"`
    Species       string    `json:"species"`
    CommonName    string    `json:"common_name"`
    Footprint     Footprint `json:"footprint"`
    Fade          FadeBand  `json:"fade"`
}

// Validate returns the first failing field as an error, or nil if the
// metadata satisfies the v1 contract. Validation order is:
// version → required strings → footprint dims → fade range → fade order.
func (m PackMeta) Validate() error { /* ... */ }

// ToExtras returns the canonical map shape for embedding under
// scene.extras["plantastic"] in a glTF asset pack. Built field-by-field
// for determinism and review-friendliness.
func (m PackMeta) ToExtras() map[string]any { /* ... */ }

// ParsePackMeta decodes a JSON blob (typically the bytes pulled from
// extras.plantastic on the consumer side) into a validated PackMeta.
// Errors from decode and validate are wrapped with a "pack_meta:" prefix.
func ParsePackMeta(raw json.RawMessage) (PackMeta, error) { /* ... */ }
```

## Validation order (one statement per check)

```text
1. m.FormatVersion == PackFormatVersion              → version error
2. strings.TrimSpace(m.Species) != ""                → required field
3. speciesRe.MatchString(m.Species)                  → species format
4. strings.TrimSpace(m.CommonName) != ""             → required field
5. strings.TrimSpace(m.BakeID) != ""                 → required field
6. checkRange("footprint.canopy_radius_m", v, ε, ∞)  → finite + positive
7. checkRange("footprint.height_m",        v, ε, ∞)  → finite + positive
8. checkRange("fade.low_start",  v, 0, 1)            → range
9. checkRange("fade.low_end",    v, 0, 1)            → range
10. checkRange("fade.high_start", v, 0, 1)           → range
11. low_start < low_end                              → ordering
12. low_end   < high_start                           → ordering
13. high_start <= 1.0                                → upper bound
```

`checkRange` lives in `settings.go:238`; we share it directly. For the
"strictly positive" footprint dims we cannot use `checkRange` as-is
(it's an inclusive `>=` check), so the validator does its own
finite-and-positive check inline using `math.IsNaN` / `math.IsInf` /
`v <= 0` — same shape as `checkRange`, ~6 lines, no helper extraction.

## `pack_meta_test.go` — outline

Mirrors `settings_test.go`:

```go
package main

import (
    "encoding/json"
    "strings"
    "testing"
)

// validMeta returns a known-good PackMeta used as the baseline for
// rejection tests; each subtest mutates one field and asserts that
// Validate() now fails.
func validMeta() PackMeta { /* ... */ }

func TestPackMeta_DefaultIsValid(t *testing.T)
func TestPackMeta_RoundTrip(t *testing.T)              // hand-written JSON literal
func TestPackMeta_RejectsBadVersion(t *testing.T)
func TestPackMeta_RejectsBadSpecies(t *testing.T)      // table: empty, leading digit, dash, uppercase, dot
func TestPackMeta_RejectsMissingCommonName(t *testing.T)
func TestPackMeta_RejectsMissingBakeID(t *testing.T)
func TestPackMeta_RejectsBadFootprint(t *testing.T)    // table: zero radius, negative height, NaN, Inf
func TestPackMeta_RejectsFadeOutOfRange(t *testing.T)
func TestPackMeta_RejectsFadeOutOfOrder(t *testing.T)  // table: equal, swapped, high>1
func TestPackMeta_AllowsHighStartEqualOne(t *testing.T)// degenerate "no high crossfade"
func TestPackMeta_ToExtras(t *testing.T)               // shape match against hand-built map
func TestParsePackMeta_RoundTrip(t *testing.T)         // raw bytes → struct → ToExtras
func TestParsePackMeta_RejectsBadJSON(t *testing.T)
func TestParsePackMeta_RejectsBadValues(t *testing.T)  // valid JSON, invalid values
```

### Round-trip test — anti-tautology approach

```go
const canonicalJSON = `{
  "format_version": 1,
  "bake_id": "2026-04-08T11:32:00Z",
  "species": "achillea_millefolium",
  "common_name": "Common Yarrow",
  "footprint": {"canopy_radius_m": 0.45, "height_m": 0.62},
  "fade": {"low_start": 0.3, "low_end": 0.55, "high_start": 0.75}
}`
```

Test:
1. Decodes `canonicalJSON` into a `PackMeta`.
2. Asserts every field equals the literal value the human typed in the
   JSON — this is what catches a wrong tag name, the bug a
   serialize-then-decode test would never find.
3. Re-encodes via `json.Marshal` and asserts the result decodes back to
   the same struct (semantic equivalence — does NOT compare bytes,
   because Go's encoder may differ in whitespace).

## Ordering of changes (commit plan)

1. Create `pack_meta.go` with types, constant, regex, `Validate`,
   `ToExtras`, `ParsePackMeta`. Run `go build ./...` — must compile.
2. Create `pack_meta_test.go`. Run `go test ./...` — every test must
   pass.
3. Single commit. The two files together are one atomic unit; splitting
   them would leave an intermediate state where the schema exists with
   no tests.

## Public surface added

| Symbol | Kind |
|---|---|
| `PackFormatVersion` | const |
| `PackMeta` | struct |
| `Footprint` | struct |
| `FadeBand` | struct |
| `(PackMeta) Validate() error` | method |
| `(PackMeta) ToExtras() map[string]any` | method |
| `ParsePackMeta(json.RawMessage) (PackMeta, error)` | func |
| `speciesRe` | unexported var |

Eight symbols, six exported, one file. Downstream T-010-02 imports
nothing — they're already in the same package.
