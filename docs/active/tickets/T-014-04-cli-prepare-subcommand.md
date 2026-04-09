---
id: T-014-04
story: S-014
title: cli-prepare-subcommand
type: task
status: open
priority: critical
phase: done
depends_on: [T-014-03]
---

## Context

One command from source GLB to finished Pack v1 file. No browser, no HTTP, no manual steps. This is the entry point for agents and batch processing.

## Usage

```sh
glb-optimizer prepare inbox/dahlia_blush.glb --category round-bush
```

Or batch:
```sh
glb-optimizer prepare-all inbox/ --category round-bush
```

## Pipeline (sequential, one asset)

1. **Copy** source GLB to `{workdir}/originals/{hash}.glb`, compute content hash
2. **Register** in FileStore (or just write the file — server picks it up via `scanExistingFiles` on next start)
3. **Optimize** — run gltfpack with default settings (compression cc, no aggressive simplify). Produces `{workdir}/outputs/{hash}.glb`
4. **Classify** — stamp category from `--category` flag (or run `classify_shape.py` if no flag given). Write to settings file.
5. **LODs** — run gltfpack at 4 simplification levels (0.50, 0.20, 0.05, 0.01) producing `_lod0..3.glb`
6. **Render** — call Blender via `render_production.py` with the category's STRATEGY_TABLE params. Produces `_billboard.glb`, `_billboard_tilted.glb`, `_volumetric.glb`
7. **Pack** — run `CombinePack` to produce `{workdir}/dist/plants/{species}.glb`
8. **Verify** — run the pack verifier (T-012-03) on the output. Non-zero → fail.

Print a summary at the end:
```
✓ dahlia_blush
  source:     inbox/dahlia_blush.glb (28 MB)
  optimized:  outputs/a7c19366...glb (6.8 MB)
  billboard:  outputs/a7c19366..._billboard.glb (1.8 MB)
  tilted:     outputs/a7c19366..._billboard_tilted.glb (0.3 MB)
  volumetric: outputs/a7c19366..._volumetric.glb (0.7 MB)
  pack:       dist/plants/dahlia_blush.glb (2.4 MB) ✓ verified
  duration:   47s
```

## Flags

- `--category <cat>` — shape category (default: auto-classify)
- `--resolution <px>` — billboard render resolution (default: 512)
- `--dir <workdir>` — working directory (default: `~/.glb-optimizer`)
- `--json` — structured JSON output for agent consumption
- `--skip-lods` — skip LOD generation (just optimize + render + pack)
- `--skip-verify` — skip the post-pack verification step

## Acceptance Criteria

- `glb-optimizer prepare inbox/dahlia_blush.glb --category round-bush` produces `dist/plants/dahlia_blush.glb`
- The pack passes `verify-pack` (T-012-03)
- The pack loads in plantastic's `loadPlantPack` (verify by copying to `__fixtures__/packs/` and running `pnpm test:unit plant-pack-real`)
- `--json` output is parseable and contains all fields (source, hash, sizes, duration, status)
- `prepare-all inbox/` processes every `.glb` in the directory, moves completed files to `inbox/done/`
- Non-zero exit on any failure, with clear error message identifying which step failed
- Works from a clean state (no prior server run needed, no FileStore dependency)

## Out of Scope

- Starting an HTTP server (this is purely CLI)
- Interactive prompts
- Parallel processing (sequential is fine — Blender is the bottleneck)
- Automatic species-id detection beyond the hash resolver (T-012-01)

## Notes

- Steps 1-5 are pure Go (gltfpack + file ops). Step 6 shells out to Blender. Step 7 is the existing Go combine code. The only new dependency is the Blender script from T-014-02.
- The `prepare-all` variant reuses a single Blender process if possible (Blender supports batch mode via `--python` with multiple scenes). If not, one Blender invocation per asset is acceptable.
- Use `inbox/dahlia_blush.glb` (28 MB) as the integration test model. Expected total duration: ~60 seconds (gltfpack ~10s, LODs ~30s, Blender ~15s, pack ~2s).
