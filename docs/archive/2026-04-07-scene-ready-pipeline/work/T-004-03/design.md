# T-004-03 — Design

## Problem

The classifier (T-004-02) tags assets with a shape category. Without a
router that maps that category onto concrete bake parameters, the
classification is just a label — the bake still runs with whichever
defaults the user happened to leave on. T-004-03 makes the
classification *load-bearing* by translating "this is a directional
asset" into "slice perpendicular to the long horizontal axis".

## Decision

A small Go-side lookup-table router applied during classification.
Strategy defaults are stamped onto `AssetSettings` at classification
time and round-trip to the browser through the existing settings JSON.
The bake reads them from settings as before.

Concretely:

1. **`strategy.go` (new)** — pure-Go module. `ShapeStrategy` struct +
   `getStrategyForCategory(category) ShapeStrategy` lookup. No I/O.
2. **Persistence** — add `SliceAxis` to `AssetSettings`. Reuse the
   existing `VolumetricLayers` for `slice_count` and
   `SliceDistributionMode` for `slice_distribution_mode`.
3. **Stamping** — extend `applyClassificationToSettings` so that on a
   classification, the strategy is materialised onto the settings
   document **only when each target field is currently at its default
   value.** This is the "user can override per-setting" semantics
   from the ticket: the user's existing customizations are sacred.
4. **Analytics** — emit `strategy_selected{category, strategy}` from
   both `autoClassify` and `handleClassify`, alongside the existing
   `classification` event.
5. **Bake wiring** — `static/app.js` `renderHorizontalLayerGLB`
   reads `currentSettings.slice_axis`. When `slice_axis !== 'y'`, the
   model is rotated so the chosen axis aligns with Y, sliced as
   today, then the export scene's root rotation is set to the inverse
   so the exported GLB is in the original frame.

## Options considered

### A. Lookup table in Go, applied during classification (chosen)

**Mechanics.** A pure function `getStrategyForCategory(category)` in
`strategy.go`. Wired into `applyClassificationToSettings` so the
strategy fields are stamped onto the persisted settings, but only
where the user has not already diverged from defaults.

**Pros.**
- The router is dependency-free, fully unit-testable, no I/O.
- A single seam (`applyClassificationToSettings`) handles both the
  upload-time auto-classify and the explicit re-classify endpoint.
- The bake function reads settings as it does today; no new wire
  format, no new API call from the browser.
- Strategy lookup is also reachable directly from Go for tests and
  for future server-side code (S-006 scene preview, T-004-04
  comparison UI).
- "User overrides win" falls out of comparing against
  `DefaultSettings()` field-by-field at stamp time.

**Cons.**
- The router must live next to settings semantics — tighter
  coupling than a standalone microservice would have. Acceptable:
  this codebase is one process.
- New persisted field (`slice_axis`) means a `LoadSettings`
  forward-compat branch and a settings-schema doc update.

### B. JS-only router

Move the lookup table to `static/app.js` and apply it on the frontend
when settings are loaded.

**Pros.** No new persisted field; the router is closest to its
consumer (the bake).

**Cons.**
- Strategy state is non-canonical — if a future code path on the Go
  side wants to know "what slice axis applies to this asset" (e.g.
  for batch export), it has to re-derive from category. Two sources
  of truth.
- Harder to unit-test without a JS test runner (the project has
  none).
- Analytics emission would have to move to the frontend, which is
  the wrong layer for "this is what the system decided" events —
  contrast with `classification` which is also Go-emitted.
- "User override wins" semantics require frontend dirty-tracking
  against the default snapshot, which is fiddly.

Rejected: it puts the canonical mapping in the wrong tier.

### C. New endpoint `GET /api/strategy/:category`

A pure read endpoint that returns a `ShapeStrategy` JSON for the
given category. The frontend fetches it after classification and
applies it locally to the settings.

**Pros.** Cleanest separation between classification and strategy.

**Cons.**
- Two round-trips per upload (classify, then strategy lookup), and
  the strategy is a literal lookup table — gratuitous network
  hop for an in-process answer.
- Same dirty-tracking complications as B.
- Spreads emission of `strategy_selected` across two boundaries
  (browser computes which strategy was selected, but Go has to
  log it because that's where the analytics writer lives).

Rejected as over-engineered for "the router is a lookup table".

### D. Stamp strategy *unconditionally* during classification

Same as A but skip the "only stamp when at default" check —
classification always overwrites the four strategy-shaped fields.

**Pros.** Simpler code path; no `DefaultSettings()` comparison
needed.

**Cons.** Directly contradicts the ticket: "(user can override
per-setting)". A user who set `volumetric_layers=8` last session
would have it silently reset every time the asset is re-classified.

Rejected on contract grounds.

## Why A wins

- Single canonical source of truth (Go router); reachable from
  every relevant call site.
- Reuses three existing seams (`applyClassificationToSettings`,
  `emitClassificationEvent`, `LoadSettings` forward-compat) so the
  net diff is small.
- Preserves the contract that user customizations are sticky.
- Works the same way for both `autoClassify` (upload-time) and
  `handleClassify` (re-classify endpoint) without duplication.

## Strategy table (decided)

| Category      | slice_axis | slice_count | slice_distribution_mode | instance_orientation_rule | default_budget_priority |
|---------------|------------|-------------|-------------------------|---------------------------|-------------------------|
| `round-bush`  | `y`        | 4           | `visual-density`        | `random-y`                | `mid`                   |
| `directional` | `auto-horizontal` | 4    | `equal-height`          | `fixed`                   | `mid`                   |
| `tall-narrow` | `y`        | 6           | `equal-height`          | `random-y`                | `mid`                   |
| `planar`      | `auto-thin`| 3           | `equal-height`          | `aligned-to-row`          | `low`                   |
| `hard-surface`| `n/a`      | 0           | `n/a`                   | `fixed`                   | `high`                  |
| `unknown`     | `y`        | 4           | `visual-density`        | `random-y`                | `mid`                   |

Notes:
- `round-bush` and `unknown` resolve to the same defaults as
  `DefaultSettings()` so existing rose-asset behavior is preserved.
- `auto-horizontal` and `auto-thin` are sentinel values resolved by
  the bake at slice time against the actual model bounding box.
  The router does not know the geometry; the bake does.
- `hard-surface` returns the sentinel `n/a` for slice fields. The
  stamping step simply does not touch slice fields when the strategy
  marks them `n/a`, because that category routes to the parametric
  pipeline (S-001) instead.

## Override semantics (precise)

Stamping rule, applied per field inside
`applyClassificationToSettings`:

> If `s.<Field> == DefaultSettings().<Field>` AND the strategy
> provides a non-`n/a` value, set `s.<Field> = strategy.<Field>`.

This means:
- A fresh, never-tuned asset gets the category's strategy in full.
- A user who tuned `slice_distribution_mode` to something custom
  keeps their value across re-classifications.
- A user who tuned `slice_distribution_mode` to its default value
  is indistinguishable from "never tuned" — the strategy will
  re-stamp it. Acceptable: that user has no preference recorded.

## Risks

- **Bake axis rotation correctness.** Rotating the model into a Y-aligned frame, slicing, and applying an inverse transform to the export root is the smallest surgical change to the existing bake, but a sign error here exports a tilted GLB. Mitigation: a manual verification step in the plan against `assets/wood_raised_bed.glb` (which is `directional` per the spike data).
- **Round-trip churn on existing assets.** Existing settings files have no `slice_axis` key. `LoadSettings` must default it to `"y"` so they don't fail validation, and `SettingsDifferFromDefaults` must include it.
