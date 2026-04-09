# T-010-01 Review — Pack Metadata Schema

## Summary

Implements the Wave-0 unblocker for E-002 (asset-pack-format-and-combine)
and plantastic E-028 (hybrid impostor rendering): a frozen Go contract
for the Pack v1 metadata block that lives at `scene.extras.plantastic`
in every asset pack GLB.

Pure data + validation. Zero file I/O, zero glTF coupling, zero HTTP.
Two new files; no edits to any existing source.

## What changed

### Files created

| File | LOC | Purpose |
|---|---|---|
| `pack_meta.go` | 137 | Types, constant, regex, `Validate`, `ToExtras`, `ParsePackMeta`, `checkPositive` helper |
| `pack_meta_test.go` | 260 | 14 unit tests covering all AC bullets |
| `docs/active/work/T-010-01/research.md` | — | Phase artifact |
| `docs/active/work/T-010-01/design.md` | — | Phase artifact |
| `docs/active/work/T-010-01/structure.md` | — | Phase artifact |
| `docs/active/work/T-010-01/plan.md` | — | Phase artifact |
| `docs/active/work/T-010-01/progress.md` | — | Phase artifact |
| `docs/active/work/T-010-01/review.md` | — | This file |

### Public Go surface added

```go
const PackFormatVersion = 1

type Footprint struct { CanopyRadiusM, HeightM float64 }
type FadeBand  struct { LowStart, LowEnd, HighStart float64 }
type PackMeta  struct {
    FormatVersion int
    BakeID, Species, CommonName string
    Footprint Footprint
    Fade      FadeBand
}

func (PackMeta) Validate() error
func (PackMeta) ToExtras() map[string]any
func ParsePackMeta(json.RawMessage) (PackMeta, error)
```

Unexported additions: `speciesRe` (compiled regex), `checkPositive`
(strict-positive numeric helper).

## Test coverage

**14 tests, all passing.** `go test ./...` is green; no regressions in
any pre-existing test (`settings_test`, `classify_test`,
`accepted_test`, `profiles_test`, `strategy_test`, `analytics_test`,
`strategy_handlers_test`, `handlers_billboard_test`).

| Concern | Test(s) |
|---|---|
| Default-shaped meta validates | `TestPackMeta_DefaultIsValid` |
| JSON tags decode to right fields | `TestPackMeta_RoundTrip` (hand-typed literal) |
| Re-encode round-trip is structurally stable | `TestPackMeta_RoundTrip` (second half) |
| Bad `format_version` rejected | `TestPackMeta_RejectsBadVersion` (4 cases: 0, 2, -1, 99) |
| Bad species rejected | `TestPackMeta_RejectsBadSpecies` (8 cases inc. caps, dashes, dots, digits, whitespace, leading underscore) |
| Missing common_name rejected | `TestPackMeta_RejectsMissingCommonName` (3 cases inc. whitespace) |
| Missing bake_id rejected | `TestPackMeta_RejectsMissingBakeID` |
| Bad footprint dims rejected | `TestPackMeta_RejectsBadFootprint` (7 cases inc. zero, negative, NaN, +Inf) |
| Fade values out of `[0,1]` rejected | `TestPackMeta_RejectsFadeOutOfRange` (4 cases inc. NaN) |
| Fade values out of order rejected | `TestPackMeta_RejectsFadeOutOfOrder` (5 cases inc. equal-pair, all-zero) |
| `high_start == 1.0` accepted (degenerate "no high crossfade") | `TestPackMeta_AllowsHighStartEqualOne` |
| `ToExtras()` returns canonical map | `TestPackMeta_ToExtras` |
| `ParsePackMeta` decode + validate round-trip | `TestParsePackMeta_RoundTrip` |
| `ParsePackMeta` rejects malformed JSON with prefixed error | `TestParsePackMeta_RejectsBadJSON` |
| `ParsePackMeta` rejects valid JSON with bad values | `TestParsePackMeta_RejectsBadValues` |

### Anti-tautology guardrail honored

The ticket Notes warned: "do NOT serialize-then-compare-against-self".
Round-trip tests use a hand-typed `canonicalJSON` constant, decode it,
and assert each Go field equals the literal value the human typed in
the JSON. This is what catches a wrong tag name (e.g.
`json:"canopy_radius"` instead of `json:"canopy_radius_m"`) — a bug
that a serialize-then-decode test would never see because both
directions would use the same wrong tag.

## Coverage gaps / things deliberately not tested

- **Strict ISO-8601 parsing of `bake_id`.** Today we only check
  non-empty. Tightening to `time.RFC3339` is documented as a deferred
  follow-up (see `design.md` "Open question") — the producer
  (T-011-03) controls bake_id generation, so a malformed value cannot
  appear in practice until/unless someone hand-edits a pack.
- **Filename ↔ species cross-check.** The epic says `species` must
  match the pack filename minus `.glb`. That's a property of the
  combine step (T-010-02), not of the schema, and is correctly out of
  scope per the ticket.
- **Property/fuzz testing for the species regex.** Considered, judged
  overkill for an 8-case table given the regex is one line.
- **Concurrency.** `PackMeta` is value-type and immutable through its
  methods (`Validate` and `ToExtras` use a value receiver), so there
  is nothing to test.

## Open concerns

### 1. `checkPositive` helper — small deviation from plan

`plan.md` said inline the strict-positive check; I extracted a 9-LOC
`checkPositive(name, v)` helper instead, mirroring `checkRange`'s
shape. Reason in `progress.md`. Reviewer should sanity-check that
this micro-helper is preferred over inlining; reverting is a 5-line
change if not.

### 2. Float `0.3` in `TestPackMeta_RoundTrip` exact-equality

The hand-typed JSON contains `"low_start": 0.3` and the assertion is
`m.Fade.LowStart != 0.3`. Float exact-equality is generally hazardous,
but here both sides go through `strconv.ParseFloat` from the same
string literal `"0.3"`, so the bit pattern is identical. This is safe
*for this test* but a future refactor that introduces any arithmetic
on the value would need to switch to a tolerance compare. Flagged
because the pattern looks like a smell at first glance.

### 3. `bake_id` non-empty only

Already noted under coverage gaps. Worth reviewer agreement that
deferring strict RFC3339 parsing to a follow-up is acceptable for
demo-day.

### 4. JSON marshal key order in `ToExtras()`

Go's `encoding/json` sorts `map[string]any` keys alphabetically when
marshaling. The on-disk extras blob therefore has keys in
`bake_id, common_name, fade, footprint, format_version, species`
order — *not* the declaration order shown in E-002. The plantastic
loader is keyed access, so this is functionally fine, but anyone
visually diffing a baked pack against the spec example will see
reordered keys. Documented in `design.md`. No action needed.

## What this unblocks

Per the E-002 wave plan:

- **Wave 1 starts now.** T-010-02 (combine) and T-011-02 (bake-time
  capture) can develop in parallel against this contract.
- **plantastic E-028.** T-080-01 (plant pack types and loader, already
  filed in the plantastic repo) builds against the same JSON shape on
  the consumer side. The two sides will integrate via the canonical
  example in E-002 §"Metadata".

## How to verify

```bash
go build ./...                              # clean
go vet ./...                                # clean
go test ./...                               # all green
go test -run TestPackMeta -v ./...          # 11 PackMeta tests
go test -run TestParsePackMeta -v ./...     # 3 ParsePackMeta tests
```

## Sign-off checklist

- [x] All AC bullets covered (mapped in `progress.md`)
- [x] `go build ./...` clean
- [x] `go vet ./...` clean
- [x] `go test ./...` green (no regressions)
- [x] No edits outside `pack_meta.go` and `pack_meta_test.go`
- [x] No serialize-then-compare-against-self tautology in tests
- [x] Public surface matches `structure.md`
- [x] Phase artifacts written for all six RDSPI phases
