---
id: S-014
epic: E-002
title: Server-Side Production Rendering via Blender
status: open
priority: critical
tickets: [T-014-01, T-014-02, T-014-03, T-014-04, T-014-05, T-014-06]
supersedes: S-013
---

## Goal

Move the Production variant rendering from the browser to the server. The billboard, tilted-billboard, and volumetric dome-slice rendering currently runs as client-side three.js JavaScript — the browser loads the model, positions cameras, renders to canvas, exports quads as GLB, and uploads them back to the Go server. This is an accident of history from when the pipeline was a single-user interactive tool. For a production pipeline that processes hundreds of GLBs, it must be server-side, headless, and callable from a CLI.

Blender is already installed on the system, detected at server startup, and used by `scripts/bake_textures.py` for parametric texture baking. The new script `scripts/render_production.py` uses the same Blender Python API to render the four impostor variants (side billboards, top-down, tilted billboards, volumetric dome slices) with the **exact same parameters** the client-side JS uses today.

## Supersedes S-013

S-013 (Playwright headless bake) was a browser-automation approach to the same problem. It worked for one-off bakes but was fragile (modal popups, WebGL in headless mode, timeouts) and fundamentally wrong — you shouldn't need a browser to run a batch processing pipeline. S-013's code can stay as a fallback or be deleted once S-014 is validated. The new approach replaces the client-side render path entirely.

## Architecture

```
                    CLI                          API
                     │                            │
  glb-optimizer prepare <file>     POST /api/build-production/{id}
                     │                            │
                     └──────────┬─────────────────┘
                                │
                    ┌───────────▼───────────┐
                    │  Go: orchestrator     │
                    │  1. gltfpack optimize │
                    │  2. classify (Go/Py)  │
                    │  3. generate LODs     │
                    │  4. call Blender ─────┼──► blender -b --python render_production.py
                    │  5. combine → pack    │         ├─ {id}_billboard.glb
                    └───────────────────────┘         ├─ {id}_billboard_tilted.glb
                                                      └─ {id}_volumetric.glb
```

The UI's "Build hybrid impostor" button becomes a `POST /api/build-production/{id}` call. The browser polls for completion and shows the result. The interactive three.js preview stays for real-time exploration; it's just no longer the production bake path.

## Critical constraint: parameter parity

The rendering parameters (camera distance, FOV, angle count, elevation, resolution, slice heights, quad sizing) must match the client-side JS **exactly**. The crossfade thresholds in plantastic's `SpeciesInstancer` (`low_start=0.30, low_end=0.55, high_start=0.75`) were calibrated against the client-side output. Different parameters → different visual artifacts → broken crossfade.

T-014-01 extracts these parameters from `app.js`. T-014-06 validates the Blender output against a known-good client-side bake.

## Tickets

| Ticket | Title | Depends on |
|---|---|---|
| T-014-01 | extract-rendering-parameters | — |
| T-014-02 | blender-render-production-script | T-014-01 |
| T-014-03 | api-build-production-endpoint | T-014-02 |
| T-014-04 | cli-prepare-subcommand | T-014-03 |
| T-014-05 | ui-button-calls-api | T-014-03 |
| T-014-06 | validation-against-known-good | T-014-04 |

## Reference assets

- **Source model**: `inbox/dahlia_blush.glb` (28 MB TRELLIS dahlia)
- **Known-good bake**: asset `1e562361be18ea9606222f8dcf81849d` in `~/.glb-optimizer/outputs/` — manually baked via the browser UI, has `_billboard.glb`, `_billboard_tilted.glb`, `_volumetric.glb`
- **Blender**: `/Volumes/ext1/Applications/Blender.app/Contents/MacOS/Blender` — v5.1.0
