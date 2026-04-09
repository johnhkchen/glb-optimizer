# T-010-01 Research — Pack Metadata Schema

## Goal of this ticket

Define the Go types, JSON encoding, and validation for **Pack Format v1**
metadata as specified in `docs/active/epics/E-002-asset-pack-format.md` §"Pack
Format v1 — The Contract". This is the Wave-0 unblocker for E-002 (combine
step) and plantastic E-028 (loader). No combine logic, no HTTP, no footprint
computation lives here — only the contract.

## Source of truth

The schema is frozen in `E-002-asset-pack-format.md` lines 92–124. The Go
types must round-trip the exact JSON shape:

```json
{
  "format_version": 1,
  "bake_id": "2026-04-08T11:32:00Z",
  "species": "achillea_millefolium",
  "common_name": "Common Yarrow",
  "footprint": { "canopy_radius_m": 0.45, "height_m": 0.62 },
  "fade":      { "low_start": 0.30, "low_end": 0.55, "high_start": 0.75 }
}
```

All keys are `lowercase_with_underscores`. Every field is required for v1.

## Codebase shape — relevant facts

- **Package layout.** Flat `package main` at the repo root. No subpackages.
  All `.go` files (`models.go`, `settings.go`, `handlers.go`, etc.) sit in the
  root directory. New types belong in a new top-level `pack_meta.go`.
- **Validation idiom.** `settings.go:170` defines
  `func (s *AssetSettings) Validate() error`. Pattern is: return the *first*
  failing field as a `fmt.Errorf` with the field name and the offending
  value. Numeric range checks go through a small `checkRange(name, v, lo, hi)`
  helper at `settings.go:238` that also rejects NaN/Inf. We will reuse the
  same helper rather than duplicate it — `checkRange` is already in
  `package main` and is unexported but visible.
- **Schema version constant.** `settings.go:15` declares
  `const SettingsSchemaVersion = 1`. Mirror that pattern with
  `const PackFormatVersion = 1`.
- **JSON tag style.** Lowercase-underscore keys throughout
  (`AssetSettings`, `FileRecord`). No `omitempty` on required fields — that
  matters here because every Pack v1 field is required and we must reject
  zero values, not silently elide them.
- **Test idiom.** `settings_test.go` uses table-driven tests for validation
  rejection (`TestValidate_RejectsOutOfRange`, lines 64–95) plus dedicated
  round-trip tests (`TestSaveLoad_Roundtrip`). Tests live in the same
  package, no external testing frameworks. We will follow the same shape in
  `pack_meta_test.go`.
- **No existing pack* code.** `Glob("pack*")` returns nothing under the repo
  root. This file is greenfield, with no pre-existing types or callers to
  preserve compatibility with.

## Schema constraints worth re-stating

From the AC and the epic spec:

1. `format_version` must equal `PackFormatVersion` (1). Anything else is
   rejected — this is the breaking-change escape hatch.
2. `species` must match `^[a-z][a-z0-9_]*$`. Latin-name slug, lowercased,
   underscore-separated, must start with a letter. The epic also constrains
   `species` to match the filename minus `.glb`, but **filename verification
   is the combine step's job, not this schema's** — out of scope here.
3. `common_name` is required, free-form non-empty string.
4. `bake_id` is required ISO-8601-ish text. The epic says "ISO 8601 timestamp
   of the bake" but does not mandate strict parsing. For v1 we require it to
   be non-empty; downstream consumers may parse it. (Strict
   `time.RFC3339` parsing is a reasonable v1.1 tightening; flagged in
   review.)
5. `footprint.canopy_radius_m` and `footprint.height_m` must both be
   strictly positive and finite. Zero canopy radius or zero height is
   physically meaningless and would crash the consumer's per-instance scale
   math.
6. `fade.low_start < fade.low_end < fade.high_start <= 1.0`. All three in
   `[0, 1]`. Strict ordering — equal values would collapse the crossfade
   band and produce a discontinuity. The upper bound is `<= 1.0` because
   `high_start == 1.0` is the degenerate "no fade-to-dome" case which we
   permit.

## ToExtras() — the embed format

The combine step (T-010-02) will need to write this struct into a glTF
node's `extras` field as `extras.plantastic = {…}`. glTF `extras` is
`map[string]any` in Go's JSON model, so `ToExtras()` returns a
`map[string]any` shaped exactly like the JSON above. We build it field by
field rather than via `json.Marshal` → `json.Unmarshal` round-trip so the
output is deterministic and obviously correct under code review.

## ParsePackMeta — round-trip

`ParsePackMeta(raw json.RawMessage) (PackMeta, error)` decodes a JSON blob
(typically what the loader pulls out of `extras.plantastic`) into a
`PackMeta` and immediately calls `Validate()` so callers cannot accidentally
get a half-populated struct.

## Risks / things to watch

- **Tautological tests.** Ticket Notes line 40 explicitly warns against
  serialize-then-compare-against-self. Round-trip tests must seed a
  *hand-written* JSON byte sequence and assert that decoding produces the
  expected struct, then re-encoding produces a byte-for-byte match against
  the same hand-written sequence (modulo key ordering — Go's `encoding/json`
  emits map keys in declared struct order, so this is stable).
- **Map key ordering in `ToExtras()`.** Go maps are unordered when
  marshaled by `encoding/json`, which sorts keys alphabetically. That means
  the on-disk extras blob will have alphabetical keys
  (`bake_id, common_name, fade, footprint, format_version, species`) — fine
  for the consumer (it's keyed lookup) but worth noting so a reviewer
  doesn't expect declaration order.
- **Regex compilation.** Compile the species regex once at package init via
  `var speciesRe = regexp.MustCompile(...)` — cheap, idiomatic, no per-call
  cost.
- **No file I/O in this ticket.** Validation is pure. No disk reads, no
  network. Easy to test.

## Out of scope (re-confirming)

- Combine logic that opens a `.glb` and writes `extras` (T-010-02).
- HTTP endpoint exposing pack metadata (T-010-03).
- Footprint computation from a mesh bounding box (T-011-02).
- `bake_id` generation / embedding (T-011-03).
- 5 MB pack size cap (T-010-05).
