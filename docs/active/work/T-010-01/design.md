# T-010-01 Design — Pack Metadata Schema

## Decision summary

- **One new file**, `pack_meta.go`, top-level `package main`.
- **Three structs**: `PackMeta`, `Footprint`, `FadeBand`. Plain data, no
  methods beyond `Validate` and `ToExtras` on `PackMeta`.
- **One constant**: `PackFormatVersion = 1`.
- **Validation reuses `checkRange` from settings.go** — no helper
  duplication.
- **Species regex compiled once** at package init.
- **Tests in a new `pack_meta_test.go`** following `settings_test.go`'s
  table-driven idiom; round-trip tests use hand-written JSON byte literals
  to avoid the tautology trap called out in the ticket Notes.

## Options considered

### Option A — separate `pack` subpackage

Create `pack/` as a sibling Go package with its own types, validation,
and tests. Pros: clean import boundary, no risk of polluting `main` with
many pack-related symbols, easier to expose to a future CLI. Cons: this
repo is **flat `package main`** today (`models.go`, `handlers.go`,
`scene.go`, etc. all sit at the root). Introducing a subpackage just for
~5 types breaks the established convention and forces every downstream
ticket (T-010-02, T-010-03) to either also move into the subpackage or
add `import "glb-optimizer/pack"` plumbing. **Rejected** as premature
abstraction: there is no second consumer of these types yet, and the
flat layout has been working fine through ten epics. If a future CLI or
SDK consumer appears, the subpackage promotion is mechanical.

### Option B — single `pack_meta.go` file in `package main` (chosen)

Mirrors `settings.go` exactly: one file per concern, all in the root
package, validation method on the value type, helpers shared via the
existing `checkRange`. Adoption cost for downstream tickets is zero —
they just reference `PackMeta` directly.

### Option C — bundle everything into the existing `models.go`

`models.go` already holds `Settings`, `FileRecord`, `LODLevel`, etc.
Adding `PackMeta` there is one less file. **Rejected** because
`models.go` is already 166 lines of unrelated runtime/state types and
mixing in a frozen wire-format schema would muddy its purpose. A
dedicated file makes the schema's frozen-contract status visually
obvious.

## Validation strategy

`Validate()` returns the **first** failing field as a `fmt.Errorf` —
matching `AssetSettings.Validate`. Order of checks:

1. `format_version == PackFormatVersion`
2. `species` non-empty AND matches `^[a-z][a-z0-9_]*$`
3. `common_name` non-empty (trimmed)
4. `bake_id` non-empty
5. `footprint.canopy_radius_m` finite and `> 0`
6. `footprint.height_m` finite and `> 0`
7. `fade.low_start` in `[0, 1]`
8. `fade.low_end` in `[0, 1]`
9. `fade.high_start` in `[0, 1]`
10. Strict ordering: `low_start < low_end < high_start <= 1.0`

The version check goes first so a future-format pack hits a clear error
immediately rather than failing on some detail check that may have
shifted between versions. Required-field checks come before
range/ordering checks because a missing field would otherwise produce
a confusing `out of range [0,1]: 0` error.

### Why strict `<` for fade ordering

Equal values would collapse the fade band to a step function. The
consumer's hybrid math (`updateHybridVisibility`, `app.js`) divides by
`(low_end - low_start)` and `(1 - high_start)`. A zero denominator
yields `Inf`/`NaN` opacity. Validation must catch this here, not at
runtime in the renderer.

### Why `high_start <= 1.0` (not `< 1.0`)

`high_start == 1.0` is the **degenerate "no high crossfade"** case —
useful for groundcover species that have no tilted view. The consumer
treats `high_start == 1.0` as "stay on tilted forever". Allowed.

### Why `> 0` (not `>= 0`) for footprint dims

Zero canopy radius means "a point" — the consumer's per-instance scale
multiplies a unit-reference geometry by `height_m`, so `height_m == 0`
collapses every plant to a flat sheet. Both must be strictly positive.

## ToExtras() shape

```go
func (m PackMeta) ToExtras() map[string]any {
    return map[string]any{
        "format_version": m.FormatVersion,
        "bake_id":        m.BakeID,
        "species":        m.Species,
        "common_name":    m.CommonName,
        "footprint": map[string]any{
            "canopy_radius_m": m.Footprint.CanopyRadiusM,
            "height_m":        m.Footprint.HeightM,
        },
        "fade": map[string]any{
            "low_start":  m.Fade.LowStart,
            "low_end":    m.Fade.LowEnd,
            "high_start": m.Fade.HighStart,
        },
    }
}
```

Built field-by-field, not via marshal/unmarshal. The result is the
exact JSON-shape that the combine step (T-010-02) will assign to the
glTF root scene's `extras["plantastic"]`. Numeric fields stay as
`float64` / `int` rather than being coerced to `json.Number` because
the consumer (Three.js GLTFLoader) parses them as plain JS numbers
either way.

## ParsePackMeta() shape

```go
func ParsePackMeta(raw json.RawMessage) (PackMeta, error) {
    var m PackMeta
    if err := json.Unmarshal(raw, &m); err != nil {
        return PackMeta{}, fmt.Errorf("pack_meta: decode: %w", err)
    }
    if err := m.Validate(); err != nil {
        return PackMeta{}, fmt.Errorf("pack_meta: validate: %w", err)
    }
    return m, nil
}
```

Always validates before returning. Error messages are prefixed with
`pack_meta:` so callers (consumers parsing untrusted on-disk data) get
unambiguous source attribution in logs.

## Why no `Marshal` helper

`encoding/json` already serializes `PackMeta` correctly via the struct
tags. A wrapper method would just be `json.Marshal(m)` and add zero
value. The combine step that needs to embed metadata in glTF extras
will use `ToExtras()` (which returns the right shape for direct
assignment), not `Marshal`. Skipping the wrapper keeps the surface
area minimal.

## Open question (deferred)

- Should `bake_id` be parsed as strict `time.RFC3339`? Today we only
  check non-empty. Tightening to RFC3339 would let `Validate` catch
  malformed timestamps from a hypothetical hand-edited pack. Deferring
  to a follow-up — the producer (T-011-03) controls bake_id generation
  end-to-end and can be made to emit RFC3339 by construction.
