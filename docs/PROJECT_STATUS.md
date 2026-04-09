# Project Status — glb-optimizer + plantastic integration

*Last updated: 2026-04-09*

## Overview

Two repos form a plant-asset pipeline for a landscaping demo:

**glb-optimizer** (`/Volumes/ext1/swe/repos/glb-optimizer`) — Go server + Blender Python scripts that take raw 3D plant models (TRELLIS GLBs) and produce mobile-friendly "Pack v1" impostor assets. The pipeline is: source GLB → gltfpack mesh optimization → PCA shape classification → LOD generation → Blender headless billboard/tilted/volumetric rendering → pack combine + verify.

**plantastic** (`/Users/johnchen/swe/repos/plantastic`) — Rust + SvelteKit landscaping platform. The `web/` directory contains a Three.js scene viewer showing a LIDAR-scanned Powell & Market demo scene with raised beds populated by plant impostors. The rendering uses a four-variant hybrid impostor scheme (side billboards → tilted billboards → volumetric dome slices, crossfaded by camera elevation angle).

## Current state

### glb-optimizer — fully done (24/24 tickets)

| Epic/Story | Status | What it delivered |
|---|---|---|
| E-002 S-010 | done | Pack v1 format spec, GLB combine step |
| E-002 S-011 | done | Pack distribution, bake-time metadata capture, integration handshake gate |
| E-002 S-012 | done | Hash-to-species resolver, pack-inspect CLI, standalone verifier, filename persistence, stale cleanup |
| E-002 S-013 | done (superseded) | Playwright headless bake — replaced by S-014 |
| E-002 S-014 | done | Server-side Blender rendering, `POST /api/build-production/{id}`, `prepare` CLI, UI button wiring |

**CLI entry point**: `glb-optimizer prepare <source.glb> -category round-bush -resolution 256`

Three species baked and verified in `~/.glb-optimizer/dist/plants/`:
- `achillea_millefolium.glb` (2.5 MB, 10s bake)
- `coffeeberry.glb` (2.8 MB, 11s bake)
- `dahlia_blush.glb` (2.9 MB, 15s bake)

Source GLBs live in `inbox/` at the repo root.

### plantastic — 55/60 tickets done

| Epic/Story | Status | What it delivered |
|---|---|---|
| E-028 S-080 | done | Pack loader, registry interface, prebuild script, terrain manifest |
| E-028 S-081 | done | SpeciesInstancer (4-variant hybrid), PlantingSystem, scale jitter, engine wiring |
| E-028 S-082 | done | Mixed-species beds, stripe-band assignment, per-species sizing, tier escalation |
| E-028 S-083 | done | Replace clone loop, asset cleanup, USB procedure, smoke test, integration gate, plant list overlay |
| E-028 S-084 | done | TS triage, real-pack test fixture, mock builder, headless engine, demo:check CLI, schema drift detector |
| E-028 S-085 | done | Code hygiene — all tests pass, TS clean, prettier, packs deployed |
| E-028 S-086 | done | Offline laptop demo — static bundle, slim clone, raw scan bloat removed, transfer verification |
| **E-029 S-087** | **in progress** | Deploy real packs + verify rendering — T-087-01 done, T-087-02 in implement |
| E-029 S-088 | blocked | Demo bundle refresh — waiting on S-087 |

## What's happening right now

T-087-02 is in `implement` — an agent is verifying whether the SpeciesInstancer correctly renders the real Pack v1 files in the demo scene. This is the first time real billboard textures flow through the full pipeline into the scene.

**Likely issues the agent will hit**:
- Plant sizing: real bbox values (canopy radius ~0.38m) differ from the white-quad fallback (0.18m). Plants may be correctly spaced but look sparser.
- Plant scale: real `bbox.height` (~1.0m from the model) vs the fallback's tuned `TARGET_HEIGHT_M / naturalHeight` ratio. May need adjustment.
- Crossfade band tuning: the `low_start/low_end/high_start` values were calibrated against client-side JS output; Blender renders might look different at transition angles.
- Material properties: Blender-exported PBR materials may interact differently with the instancer's opacity driving.

## Immediate priorities

1. **Land T-087-02** (verify rendering) and **T-087-03** (fix any visual issues)
2. **T-087-04** — make the fallback loud (`console.error` instead of `warn`) so it's obvious if it fires
3. **T-088-01 + T-088-02** — rebuild the offline demo bundle with real packs, test on laptop

## Key commands

```sh
# Producer
cd /Volumes/ext1/swe/repos/glb-optimizer
lisa status                                              # DAG overview
glb-optimizer prepare inbox/<file>.glb -category round-bush -resolution 256  # bake one asset
glb-optimizer pack-inspect <species>                     # inspect a pack
just validate                                            # validate Blender output vs known-good
go test ./...                                            # Go tests (3.3s)

# Consumer
cd /Users/johnchen/swe/repos/plantastic/web
lisa status                                              # DAG overview
pnpm demo:check                                         # single readiness check
pnpm test:unit                                           # all tests (478 pass, ~15s)
pnpm test:unit plant-pack-real                           # real-pack loader test
pnpm dev --host 0.0.0.0                                  # dev server on tailscale
pnpm prebuild                                            # regenerate plants-registry.json
```

## Uncommitted fixes to be aware of

These were made during live debugging and may not be committed yet:

| File | Fix | Why |
|---|---|---|
| `scripts/render_production.py` | EEVEE engine name: try `BLENDER_EEVEE` before `BLENDER_EEVEE_NEXT` | Blender 5.1 renamed it back |
| `scripts/render_production.py` | `use_bloom` guarded with `hasattr` | Removed in Blender 5.x |
| `scripts/render_production.py` | `calc_normals` guarded with `hasattr` | Removed in Blender 5.x |
| `scripts/render_production.py` | `render_to_image` saves to temp file instead of reading Render Result pixels | Blender 5.x returns empty pixels from `bpy.data.images["Render Result"]` |
| `scripts/render_production.py` | Cycles CPU for headless, EEVEE for headed | EEVEE needs GPU context unavailable in `blender -b` |
| `prepare_cmd.go` | Blender source path: `originals/{id}.glb` not `outputs/{id}.glb` | gltfpack's `EXT_meshopt_compression` not supported by Blender's importer |
| `main.go` | Server binds `0.0.0.0` instead of `localhost` | Tailscale access for demo |

## Architecture references

- **Pack v1 spec**: `glb-optimizer/docs/active/epics/E-002-asset-pack-format.md` §"Pack Format v1 — The Contract"
- **Rendering parameters**: `glb-optimizer/docs/knowledge/production-render-params.md`
- **Demo deployment**: `plantastic/docs/laptop-dev-setup.md` + `plantastic/web/README.md` USB section
- **Crossfade math**: `plantastic/web/src/lib/three/species-instancer.ts` — ported verbatim from `glb-optimizer/static/app.js:4197`

## Cross-repo coordination

Agents can read each other's repos and run `lisa status` for coordination. The two integration gates (glb-optimizer T-011-04 and plantastic T-083-05) are both done — the handshake protocol is documented in their ticket bodies for reference if a re-handshake is ever needed.

## How to add a new plant species

```sh
# 1. Drop source GLB in inbox
cp ~/Downloads/new_plant.glb /Volumes/ext1/swe/repos/glb-optimizer/inbox/

# 2. Bake (15s, no browser)
cd /Volumes/ext1/swe/repos/glb-optimizer
glb-optimizer prepare inbox/new_plant.glb -category round-bush -resolution 256

# 3. Deploy pack to plantastic
cp ~/.glb-optimizer/dist/plants/new_plant.glb /Users/johnchen/swe/repos/plantastic/web/static/potree-viewer/assets/plants/

# 4. Add to manifest
echo "new_plant" >> /Users/johnchen/swe/repos/plantastic/web/static/potree-viewer/assets/plants/MANIFEST.txt

# 5. Regenerate registry + verify
cd /Users/johnchen/swe/repos/plantastic/web
pnpm prebuild
pnpm demo:check

# 6. Add to SCENE_BEDS in demo-scene.ts
# Edit the species array in the relevant bed spec
```
