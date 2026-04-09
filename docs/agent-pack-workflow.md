# Agent Pack Workflow

How to produce asset packs from source GLB files without human interaction.

## Prerequisites

| Tool | Install | Verify |
|------|---------|--------|
| Go 1.21+ | [go.dev/dl](https://go.dev/dl/) | `go version` |
| Node.js 18+ | [nodejs.org](https://nodejs.org/) | `node --version` |
| gltfpack | [meshoptimizer releases](https://github.com/zeux/meshoptimizer/releases) | `gltfpack -v` |
| Playwright | `just bake-install` | (installs Chromium automatically) |

Run `just check` to verify Go, gltfpack, and Blender (optional) are available.

## Quick Start

### Single file

```bash
just bake inbox/dahlia_blush.glb --json
```

Output (stdout, one JSON line):
```json
{"source":"dahlia_blush.glb","species":"dahlia_blush","pack":"dist/plants/dahlia_blush.glb","size":1842311,"status":"ok"}
```

### Batch (all files in inbox/)

```bash
just bake-all --json
```

Output (stdout, one JSON line per file — NDJSON):
```json
{"source":"dahlia_blush.glb","species":"dahlia_blush","pack":"dist/plants/dahlia_blush.glb","size":1842311,"status":"ok"}
{"source":"achillea.glb","species":"achillea_millefolium","pack":"dist/plants/achillea_millefolium.glb","size":923411,"status":"ok"}
```

### Without --json

Without the flag, human-readable progress logs and a summary table are printed to stdout. This is the default interactive mode.

## JSON Output Schema

### Success

```json
{
  "source": "dahlia_blush.glb",
  "species": "dahlia_blush",
  "pack": "dist/plants/dahlia_blush.glb",
  "size": 1842311,
  "status": "ok"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `source` | string | Input GLB filename |
| `species` | string | Resolved species identifier |
| `pack` | string | Path to output pack file |
| `size` | number | Pack file size in bytes |
| `status` | `"ok"` | Success indicator |

### Error

```json
{
  "source": "bad.glb",
  "error": "bake timeout after 300s",
  "step": "billboard",
  "screenshot": "dist/bake-errors/bad-2026-04-09T12-00-00-000Z.png",
  "status": "error"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `source` | string | Input GLB filename |
| `error` | string | Error message |
| `step` | string | Pipeline stage that failed: `upload`, `select`, `pipeline`, `billboard`, `classify`, `lod`, `optimize`, `pack`, or `unknown` |
| `screenshot` | string? | Path to error screenshot (omitted if capture failed) |
| `status` | `"error"` | Failure indicator |

## Log Routing

When `--json` is active:
- **stdout** contains only JSON (one line per result)
- **stderr** contains progress logs (prefixed `[headless-bake]` / `[batch-bake]` / `[just bake]`)

Parse stdout for results. Ignore or log stderr for debugging.

```bash
# Parse results, suppress logs
result=$(just bake inbox/dahlia_blush.glb --json 2>/dev/null)
echo "$result" | jq .species
```

## Worked Example

Given `inbox/dahlia_blush.glb` (the reference model already in the repo):

```bash
# 1. Bake and pack
just bake inbox/dahlia_blush.glb --json
# stdout: {"source":"dahlia_blush.glb","species":"dahlia_blush","pack":"/Users/you/.glb-optimizer/dist/plants/dahlia_blush.glb","size":1842311,"status":"ok"}

# 2. Verify the pack
just verify-pack dahlia_blush
# Checks Pack v1 schema compliance and scene graph structure

# 3. Inspect pack details (optional)
./glb-optimizer pack-inspect --json dahlia_blush
# Returns detailed pack metadata as JSON
```

## Troubleshooting

| Error | Cause | Fix |
|-------|-------|-----|
| `server not reachable` | Server failed to start or port in use | Check `lsof -i :8787`, kill stale processes |
| `file not found` | GLB path doesn't exist | Use absolute path or path relative to repo root |
| `upload failed: 413` | File too large for default limits | Check server upload limits |
| `pipeline stage failed: Optimize` | gltfpack not installed or not on PATH | `which gltfpack` — install from meshoptimizer releases |
| `pipeline stage failed: Classify` | Shape classification error | Check server logs (stderr) for details |
| `bake timeout after 300s` | Asset too complex or server hung | Restart server, try simpler model |
| `pack file not found` | Pack build succeeded but file missing | Check disk space, check `~/.glb-optimizer/dist/plants/` |
| `inbox directory not found` | Wrong inbox path | Default is `inbox/` at repo root |

## Exit Codes

| Code | Meaning |
|------|---------|
| 0 | All files baked and packed successfully |
| 1 | One or more files failed (check JSON output for details) |

## Non-Interactive Guarantees

- No prompts or confirmations
- Browser runs headless (no visible window)
- Server auto-starts and auto-stops
- Species resolved automatically via the resolver chain — no manual `_meta.json` needed
- All paths default to `~/.glb-optimizer/` working directory
