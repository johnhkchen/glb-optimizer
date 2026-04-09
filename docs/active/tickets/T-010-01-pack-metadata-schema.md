---
id: T-010-01
story: S-010
title: pack-metadata-schema
type: task
status: open
priority: critical
phase: done
depends_on: []
---

## Context

Define and freeze the Pack v1 metadata schema in Go. This is the contract that both glb-optimizer (producer) and plantastic (consumer) build against. Once this lands, downstream tickets in both repos can develop in parallel.

The schema lives in E-002 — this ticket implements it as Go types and a validation function.

## Acceptance Criteria

- New file `pack_meta.go` (or section in `pack.go`) defining:
  - `type PackMeta struct` matching E-002 §"Metadata"
  - `type Footprint struct { CanopyRadiusM, HeightM float64 }`
  - `type FadeBand struct { LowStart, LowEnd, HighStart float64 }`
  - JSON tags producing lowercase_with_underscores keys
- `func (PackMeta) Validate() error` — checks `format_version == 1`, all required fields populated, `species` matches `^[a-z][a-z0-9_]*$`, fade values are sorted (`low_start < low_end < high_start <= 1.0`)
- `func (PackMeta) ToExtras() map[string]any` — returns the value to embed at `scene.extras.plantastic`
- `func ParsePackMeta(raw json.RawMessage) (PackMeta, error)` — round-trips
- Unit tests for: valid meta round-trips, invalid species name rejected, fade out of order rejected, missing required field rejected
- Schema version constant: `const PackFormatVersion = 1`

## Out of Scope

- Combine logic (T-010-02)
- HTTP endpoint (T-010-03)
- Footprint computation from bbox (T-011-02)

## Notes

- This ticket must merge before any other E-002 or plantastic E-028 ticket. It's the unblocker.
- Coding agent: when writing tests, derive expected JSON byte sequences by hand, do NOT serialize-then-compare-against-self (tautology).
