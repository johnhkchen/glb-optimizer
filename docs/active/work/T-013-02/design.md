# T-013-02 Design: just bake recipe

## Overview

Two deliverables: (1) a full-lifecycle `just bake` recipe and (2) a `just bake-status` Go subcommand.

## Design: `just bake <source.glb>`

### Architecture

The recipe is a shell script in the justfile that orchestrates three phases:

```
[server-ensure] → [headless-bake] → [summary]
```

The server-ensure phase conditionally starts a background Go server. The headless-bake phase runs the existing Playwright script. The summary phase is handled by headless-bake.ts itself (it already prints pack path + size).

### Server Lifecycle

```
┌─────────────────────────────────┐
│  curl /api/status reachable?    │
│         │                       │
│    yes ─┤── skip start          │
│    no  ─┤── go run . &          │
│         │   SERVER_PID=$!       │
│         │   poll /api/status    │
│         │   (max 15s, 0.5s     │
│         │    interval)          │
│         └───────────────────────│
│                                 │
│  [run headless-bake.ts]         │
│                                 │
│  if SERVER_PID set:             │
│    kill $SERVER_PID             │
└─────────────────────────────────┘
```

**Key decisions**:
- Use `go run .` (not `just run` or `./glb-optimizer`) to avoid needing a separate build step and to avoid spawning a second process whose PID we don't control.
- Use `kill $SERVER_PID` (not `pkill`) for precision — only kill what we started.
- `trap` ensures cleanup on script error/exit.
- 15-second timeout for server readiness is generous for local dev (typically <3s).
- Port defaults to 8787, consistent with headless-bake.ts DEFAULT_PORT.

### Recipe Signature

```just
# Full bake pipeline: start server if needed, run headless bake + pack, stop server.
# Example: just bake inbox/dahlia_blush.glb
bake source:
```

The existing `bake` recipe from T-013-01 is replaced. The `bake-debug` recipe remains unchanged (thin wrapper for headed debugging with a running server).

### Error Handling

- If server fails to start within timeout → exit 1 with clear message.
- If headless-bake.ts fails → its own error handling captures screenshots; recipe propagates exit code.
- Server cleanup happens in `trap` regardless of success/failure.

## Design: `just bake-status`

### Architecture

New Go subcommand: `glb-optimizer bake-status`. Justfile recipe is a thin wrapper:

```just
bake-status: build
    ./glb-optimizer bake-status
```

### Go Subcommand Behavior

1. Walk `~/.glb-optimizer/outputs/` to discover all unique asset IDs.
2. For each ID, check existence of intermediate files:
   - `{id}_billboard.glb` → has_billboard
   - `{id}_billboard_tilted.glb` → has_tilted  
   - `{id}_volumetric.glb` → has_dome
3. Check if a pack exists at `~/.glb-optimizer/dist/plants/{species}.glb`.
4. Resolve species name from the existing resolver (same as pack-all).
5. Print tabwriter table with columns: SPECIES, BILLBOARD, TILTED, DOME, PACK

### ID Discovery

Unlike `discoverPackableIDs` (which requires `_billboard.glb`), bake-status should show ALL assets — including those mid-bake that only have a `.glb` or `_bake.json`. Discovery algorithm:

```
for each file in outputs/:
    strip known suffixes (_billboard.glb, _billboard_tilted.glb, 
                          _volumetric.glb, _lod{0..3}.glb, 
                          _bake.json, _reference.png, .glb)
    collect unique base IDs
```

This is essentially: find all unique content-hash prefixes.

### Output Format

```
SPECIES             BILLBOARD  TILTED  DOME  PACK
dahlia_blush        yes        yes     yes   yes
unknown_abc123      yes        no      no    no
TOTAL: 2 assets, 1 packed
```

If species cannot be resolved, fall back to truncated content hash.

## Decisions

| Decision | Rationale |
|----------|-----------|
| Replace existing `bake` recipe | T-013-02 AC supersedes T-013-01's thin wrapper |
| Keep `bake-debug` unchanged | Headed debugging doesn't need server lifecycle |
| Go subcommand for bake-status | Consistent with pack-all, clean-stale-packs; needs species resolver |
| `go run .` not `./glb-optimizer` | Single process, known PID, no separate build step |
| 15s server startup timeout | Generous for local; typically <3s |
| Discover all IDs, not just packable | bake-status should show mid-bake assets too |
