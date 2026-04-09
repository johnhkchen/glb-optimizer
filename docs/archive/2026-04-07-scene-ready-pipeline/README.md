# Scene-Ready Pipeline — Archived 2026-04-07

This collection is the foundational arc of glb-optimizer: from "TRELLIS
mesh too heavy to render" to "scene-ready hybrid impostor with
analytics-instrumented tunable bake."

## Contents

- **1 epic** — `E-001` tunable-pipeline-with-analytics
- **9 stories** — `S-001` through `S-009`
- **31 tickets** — `T-001-01` through `T-009-03`
- **31 work directories** — RDSPI artifacts (research, design, structure, plan, progress, review) per ticket

## What landed

| Story | Title | Anchor |
|-------|-------|--------|
| S-001 | asset-distillation-pipeline | Original distillation work — raised bed parametric reconstruction, rose volumetric slicing, scene budget system |
| S-002 | settings-and-tuning-ui | Per-asset settings schema + persistence + tuning panel skeleton |
| S-003 | analytics-foundation | Versioned event schema, JSONL storage, profiles, accepted-tag, export script |
| S-004 | geometry-aware-classification | PCA shape classifier (round-bush / directional / tall-narrow / planar / hard-surface), strategy router, multi-strategy comparison UI, trellis end-to-end validation |
| S-005 | bake-tuning-controls | Visual-density slice mode, ground alignment, dome curvature restoration, bake quality control surface |
| S-006 | scene-preview-realism | Scene templates (hedge-row, mixed-bed, rock-garden, container, grid), per-instance variation, ground plane |
| S-007 | lighting-weather-presets | Preset enum (midday-sun, overcast, golden-hour, dusk, indoor, from-reference-image), bake-preview consistency |
| S-008 | workflow-consolidation | "Prepare for scene" primary action, advanced disclosure, label rename, inline help, first-run hint |
| S-009 | tilted-billboard-transition-band | Third impostor type at 30° elevation, three-stage crossfade through the ~45° tilt band |

## Anchor quote

> "Inedible muffins at the right price, but not the right flavor."

The thesis of E-001. The bake worked but needed human tuning + analytics
+ shape awareness to deliver designer-quality output. By the close of
S-009 the muffins are flavored.

## Notable carries-forward (not blockers, follow-up candidates)

- `scanExistingFiles` only detects `_billboard_tilted.glb` on restart;
  the older `_billboard.glb` and `_volumetric.glb` flags don't survive a
  server restart. One-line follow-up to walk all known suffixes.
- `handleDeleteFile` is now a 10-line flat list of `os.Remove` calls.
  Refactor to a `[]string{...}` slice loop is the obvious cleanup.
- `makeDefaults()` in `static/app.js` is hand-synced with
  `DefaultSettings()` in `settings.go`. A `/api/settings/defaults`
  endpoint would eliminate the duplication.
- No JS test runner anywhere in the project. Adding a minimal harness
  is the highest-leverage testing investment.
- Default crossfade thresholds for the tilted band (0.30 / 0.55 / 0.75)
  are best-guesses, not visually tuned against the rose. Sliders exist
  in the tuning panel for hand-tuning by eye.

## Layout

```
2026-04-07-scene-ready-pipeline/
├── README.md           (this file)
├── epics/              (E-001)
├── stories/            (S-001 through S-009)
├── tickets/            (T-001-01 through T-009-03)
└── work/               (per-ticket RDSPI artifacts)
```
