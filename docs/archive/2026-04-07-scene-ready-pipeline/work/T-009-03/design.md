# Design — T-009-03

## Decision summary

- **One unified visibility function** `updateHybridVisibility()` that
  computes all three opacities from a single `lookDownAmount` reading
  and applies them to `billboardInstances` + `billboardTopInstances` +
  `tiltedBillboardInstances` + `volumetricInstances`. Replaces both
  `updateBillboardVisibility` and `updateVolumetricVisibility` *in the
  hybrid code path*. The legacy 2-state functions stay intact for the
  standalone "Billboard" / "Volumetric" / "Tilted" preview buttons.
- **A `productionHybridFade` flag** (mirror of `volumetricHybridFade`)
  set true only when `runProductionStressTest` instantiates the three
  layers. `animate()` dispatches to the unified function when
  `productionHybridFade === true`, otherwise it falls back to the
  existing per-mode visibility calls.
- **Three new `AssetSettings` fields** —
  `tilted_fade_low_start` (0.30), `tilted_fade_low_end` (0.55),
  `tilted_fade_high_start` (0.75) — wired through `settings.go`,
  `makeDefaults()`, `TUNING_SPEC`, `index.html` sliders, and
  `help_text.js`. Auto-instrumentation gives `setting_changed` events
  for free.
- **`generateProductionAsset` runs all three bakes sequentially.** The
  tilted bake reuses the existing `renderTiltedBillboardGLB` +
  `/api/upload-billboard-tilted/:id` path. `prepareForScene`'s
  "production" stage updates its label as each sub-bake runs and the
  success check requires `has_billboard && has_billboard_tilted &&
  has_volumetric`.
- **`runProductionStressTest` fetches three GLBs in parallel** and
  instantiates all three layers at the same positions, with
  `productionHybridFade = true`.

## Options considered

### Option A (chosen): unified `updateHybridVisibility()` gated by `productionHybridFade`

The 3-state math lives in one place. The four arrays are read in one
loop and the four opacities are applied together. The legacy
2-state functions remain untouched so non-hybrid previews behave
identically (no risk of regression in the standalone Billboard /
Volumetric buttons that ship today). One new module flag
(`productionHybridFade`) plus one new function (`updateHybridVisibility`).

Pros:
- Math symmetry: all three opacities computed from one
  `lookDownAmount`, with the band thresholds adjacent in source.
- No regression risk to standalone preview modes — the existing
  visibility functions are not modified.
- Clean ownership: the hybrid path owns the new flag, the new
  settings, and the new visibility function.
- Easy to extend if we ever add a fourth state (e.g. straight-down
  pancake).

Cons:
- Two visibility code paths to keep mentally aligned. Mitigated by
  the standalone path being legacy and frozen — we don't intend to
  evolve it further.

### Option B (rejected): rewrite `updateBillboardVisibility` and `updateVolumetricVisibility` in place

Replaces the two existing functions with the 3-state version. Saves
one function definition but forces every preview mode through the
3-state math whether it has tilted instances or not. The "no tilted
present" branch becomes a frequent special case, and the standalone
preview buttons (which today do not load a tilted bake) need
defensive guards everywhere. Unnecessary blast radius for the
standalone modes.

### Option C (rejected): substages in `PREPARE_STAGES`

Spec allows it. Implementation cost is high — `setPrepareStages` /
`markPrepareStage` would need a parent/child model, and the analytics
payload's `stages_run` would need to grow to record sub-stage
identifiers. The same information is conveyed by updating the
running label in place via `markPrepareStage('production', 'running', 'tilted…')`.
Simpler, lower-risk, identical user-visible signal.

## Crossfade math

```
const lookDown = Math.abs(camDir.dot(0,-1,0));
const lowStart  = currentSettings.tilted_fade_low_start;   // 0.30
const lowEnd    = currentSettings.tilted_fade_low_end;     // 0.55
const highStart = currentSettings.tilted_fade_high_start;  // 0.75
const highEnd   = 1.0;                                     // fixed

// Smoothstep helper inlined as t*t*(3-2t).
const sLow  = smoothstep01((lookDown - lowStart)  / (lowEnd  - lowStart));
const sHigh = smoothstep01((lookDown - highStart) / (highEnd - highStart));

const horizontalOpacity = 1 - sLow;          // 1 → 0 across low band
const tiltedOpacity     = sLow * (1 - sHigh); // 0 → 1 across low, 1 → 0 across high
const domeOpacity       = sHigh;             // 0 → 1 across high band
```

`smoothstep01(x) = clamp(x,0,1)*clamp(x,0,1)*(3-2*clamp(x,0,1))`.

The `tiltedOpacity` formula keeps the tilted layer at full opacity
across the entire flat zone 0.55–0.75 because both `sLow=1` and
`sHigh=0` there. Outside the bands the opacity decays smoothly to 0.

`horizontalOpacity` is split between side / top via the existing
side/top crossfade. To preserve the "side rotates to top as you tilt"
behavior, the unified function multiplies `horizontalOpacity` by the
existing 0.55–0.75 side/top weighting — but inside the new low band
(0.30–0.55) the side/top split stays at sideOpacity=1, since the user
hasn't tilted past the side fade yet. To keep the math simple and the
behavior continuous, we drive the side/top split off the *low band*:
`topOpacity = sLow`, `sideOpacity = 1 - sLow`. When `horizontalOpacity`
is multiplied in, the side billboards fade out across the low band
just like they should.

```
const topWeight  = sLow;
const sideOpacity = horizontalOpacity * (1 - topWeight);
const topOpacity  = horizontalOpacity * topWeight;
```

This collapses to the table:
- 0.00–0.30: horizontal=1, side=1, top=0, tilted=0, dome=0 ✓
- 0.30–0.55: horizontal fades out, top fades in (within horizontal),
  tilted fades in, dome=0 ✓
- 0.55–0.75: horizontal=0, tilted=1, dome=0 ✓
- 0.75–1.00: tilted fades out, dome fades in ✓

(The "horizontal split" still respects the tilt — at the 0.55 crossover
point side=0 and top=horizontalOpacity=0, so the user sees the tilted
billboard taking over from the *top* impostor, which is visually closer
than from the side.)

## Settings schema deltas

```go
// settings.go
type AssetSettings struct {
    ...
    TiltedFadeLowStart  float64 `json:"tilted_fade_low_start,omitempty"`
    TiltedFadeLowEnd    float64 `json:"tilted_fade_low_end,omitempty"`
    TiltedFadeHighStart float64 `json:"tilted_fade_high_start,omitempty"`
    ...
}
```

Defaults: 0.30, 0.55, 0.75 in `DefaultSettings()`. Validation:
`checkRange("tilted_fade_low_start", v, 0, 1)` ×3.

`omitempty` is intentional — legacy on-disk settings written before
T-009-03 will load with all three fields zero, which fails the
"start < end" intuition but still passes [0,1] validation. The JS
loader fills in defaults if any field is zero (mirror the existing
`SceneInstanceCount` zero-normalize pattern).

Ordering invariants (`low_start < low_end < high_start < 1.0`) are
*not* enforced server-side — the user is allowed to drag sliders to
any combination, and the JS visibility function uses
`THREE.MathUtils.clamp` so a degenerate ordering produces a hard cut
rather than a NaN. This matches how `volumetric_layers` and
`alpha_test` are validated only on range, not on relationship.

## `prepareForScene` UX delta

Single "Production asset" row, label updated in place:

1. `markPrepareStage('production', 'running')` → `[•] Production asset`
2. After billboard upload: `markPrepareStage('production', 'running', 'tilted bake…')` → `[•] Production asset — tilted bake…`
3. After tilted upload: `markPrepareStage('production', 'running', 'volumetric bake…')` → `[•] Production asset — volumetric bake…`
4. After volumetric upload: `markPrepareStage('production', 'ok')` → `[✓] Production asset`

Failure check at the end of stage 4:
```js
if (!after || !after.has_billboard || !after.has_billboard_tilted || !after.has_volumetric) {
    throw new Error('production asset failed');
}
```

## Risks & mitigations

| Risk | Mitigation |
|---|---|
| Tilted bake call inside `generateProductionAsset` reuses `renderTiltedBillboardGLB`'s reliance on `currentSettings.alpha_test` — same as the horizontal billboard path; safe. | None needed. |
| The new settings fields are missing from a legacy on-disk JSON; loader sees zeros, math clamps to a hard cut at lookDown=0. | JS loader normalizes any zero in the trio to its `makeDefaults()` value. Backend Validate accepts zeros. |
| The standalone "Tilted" preview button (T-009-02) calls `createTiltedBillboardInstances` without setting `productionHybridFade`. The new gating in `animate()` correctly leaves the tilted instances fully visible in this case. | Verify by reading `animate()` after edit — `productionHybridFade` only set in `runProductionStressTest`. |
| Bundle size on rose increases >500 KB. | Out-of-band manual verification per ticket. |
| `updateHybridVisibility` runs every frame on four arrays — perf regression at high instance count. | Only runs in stress test (`stressActive`); the inner loops are O(n) over instance buckets, identical cost to today's two functions combined. |
