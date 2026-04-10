# Project Status — glb-optimizer

*Last updated: 2026-04-10*

## What this is

Go server + Blender Python scripts that take raw 3D plant models (TRELLIS
GLBs) and produce lightweight "Pack v1" impostor assets with billboard
textures for real-time rendering.

Part of a four-repo system. See also:
- [plant-catalog PROJECT_STATUS](https://github.com/johnhkchen/plant-catalog/blob/main/docs/PROJECT_STATUS.md) — catalog, storefront, design tools
- [plant-catalog PRODUCT.md](https://github.com/johnhkchen/plant-catalog/blob/main/docs/PRODUCT.md) — full product vision
- [plant-catalog NORTH_STAR.md](https://github.com/johnhkchen/plant-catalog/blob/main/docs/NORTH_STAR.md) — Laura & Marco scenario

| Repo | Role | Status |
|------|------|--------|
| **plant-model-studio** | Upstream: research → image gen → TRELLIS 2 | Complete (23/23) |
| **glb-optimizer** (this) | Midstream: GLB → bake → Pack v1 | Complete (24/24) + S-015 open |
| **plant-catalog** | Downstream: catalog + storefront + design tools + API | 89/89 + S-033 open |
| **plantastic** | Consumer: SvelteKit + Three.js scene viewer | Complete (60/60) |

## Current state

**24/24 original tickets complete.** Full pipeline operational. One new
story (S-015) filed for a rendering bug.

### What works

- **CLI**: `glb-optimizer prepare <source.glb> -category round-bush -resolution 256`
- **Pipeline**: source GLB → gltfpack optimize → PCA classify → LOD generate → Blender headless render (side/tilted/volumetric billboards) → pack combine + verify
- **Pack v1 format**: single GLB with `view_side` (N yaw variants), `view_tilted` (N tilted variants), `view_dome` (M height slices), plus metadata at `scene.extras.plantastic`
- **Blender 5.x compat**: engine name probing, use_bloom guard, temp-PNG pixel readback, calc_normals guard
- **Server**: web UI for upload, classify, bake, tune, accept + preview
- **Additional CLIs**: pack, pack-all, pack-inspect, clean-stale-packs, bake-status, prepare-all
- **Tests**: 33 test files, all passing (3.3s)

### Species baked

Three species in `~/.glb-optimizer/dist/plants/`:

| Species | Pack size | Source size |
|---------|-----------|-------------|
| achillea_millefolium | 2.5 MB | 14 MB |
| coffeeberry | 2.9 MB | 12 MB |
| dahlia_blush | 3.0 MB | 30 MB |

Both packs AND source models are uploaded to plant-catalog's R2 bucket:
- `packs/{species}.glb` — Pack v1 billboard quads
- `sources/{species}.glb` — TRELLIS 3D source mesh

### Open issue: S-015 — Top-down render incorrect for round-bush

**T-015-01**: The dome/volumetric slice camera in `render_production.py` is
positioned incorrectly for the round-bush strategy. Instead of capturing
overhead canopy views, it produces side-profile views that look flat when
viewed top-down. The side and tilted billboards render correctly.

Investigation needed in `scripts/render_production.py` — the dome slice
camera rig likely reuses the side billboard orbital camera instead of
positioning directly overhead.

## Pipeline architecture

```
Source GLB (TRELLIS output, 12-30 MB)
    │
    ▼
gltfpack mesh optimization → outputs/{id}.glb
    │
    ▼
PCA shape classification → settings/{id}.json (category, strategy)
    │
    ▼
LOD generation → outputs/{id}_lod0.glb ... _lod3.glb
    │
    ▼
Blender headless render (render_production.py)
├── Side billboards (N angles)    → outputs/{id}_billboard.glb
├── Tilted billboards (N angles)  → outputs/{id}_billboard_tilted.glb
└── Volumetric dome slices (M)    → outputs/{id}_volumetric.glb
    │
    ▼
Pack combine + verify → dist/plants/{species}.glb (Pack v1)
    │
    ▼
Upload to R2 → plant-catalog serves via /api/packs/?preview
```

## Pack v1 scene graph

```
pack_root
├── view_side (7 children)      ← yaw-variant billboard quads
│   ├── variant_0 [4 verts, own texture]
│   ├── variant_1
│   └── ...
├── view_tilted (6 children)    ← 30° elevation billboard quads
│   ├── variant_0
│   └── ...
└── view_dome (4 children)      ← horizontal dome cross-sections
    ├── slice_0 [144 verts]
    ├── slice_1
    └── ...
```

Coordinate convention: billboard quads lie in the XZ plane (Blender Y-up
exported as glTF Y-up). Side/tilted quads have Y=0 with X/Z extent.
Dome slices are in XY plane with Z=0.

Pack metadata at `scene.extras.plantastic`:
```json
{
  "format_version": 1,
  "bake_id": "2026-04-09T18:30:00Z",
  "species": "achillea_millefolium",
  "footprint": { "canopy_radius_m": 0.3, "height_m": 0.6 },
  "fade": { "low_start": 0.30, "low_end": 0.55, "high_start": 0.75 }
}
```

## Key files

| File | Purpose |
|------|---------|
| `main.go` | Server + CLI subcommand dispatch |
| `prepare_cmd.go` | End-to-end prepare pipeline |
| `combine.go` | Pack v1 GLB assembly |
| `pack_writer.go` | Pack binary writing |
| `classify.go` | PCA shape classification |
| `scripts/render_production.py` | Blender headless impostor renderer |
| `strategy.go` | Per-category render strategy table |
| `settings.go` | Per-asset settings (category, fade bands) |
| `justfile` | Task runner recipes |

## Key commands

```sh
# Bake one species
glb-optimizer prepare inbox/<file>.glb -category round-bush -resolution 256

# Bake all inbox items
glb-optimizer prepare-all -category round-bush -resolution 256

# Inspect a pack
glb-optimizer pack-inspect <species>

# Run tests
go test ./...

# Check project status
lisa status
```

## How to add a new species to the catalog

```sh
# 1. Drop source GLB in inbox
cp ~/Downloads/new_plant.glb inbox/

# 2. Bake (15s per species)
glb-optimizer prepare inbox/new_plant.glb -category round-bush -resolution 256

# 3. Upload pack + source to R2
cd /Volumes/ext1/swe/repos/plant-catalog
pnpm exec wrangler r2 object put plant-assets/packs/new_plant.glb \
  --file ~/.glb-optimizer/dist/plants/new_plant.glb \
  --content-type model/gltf-binary --remote
pnpm exec wrangler r2 object put plant-assets/sources/new_plant.glb \
  --file ~/.glb-optimizer/originals/<hash>.glb \
  --content-type model/gltf-binary --remote

# 4. Extract and upload thumbnail
node scripts/extract-thumbnail.mjs ~/.glb-optimizer/dist/plants/new_plant.glb
pnpm exec wrangler r2 object put plant-assets/thumbnails/new_plant.webp \
  --file new_plant_thumb.webp --content-type image/webp --remote

# 5. Create species entry in EmDash (via admin UI or API)

# 6. Verify in catalog: https://plant-catalog.john-hk-chen.workers.dev
```
