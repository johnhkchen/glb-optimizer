# T-013-02 Research: just bake recipe

## Ticket Summary

Wrap T-013-01's Playwright headless-bake script into a single `just bake <source.glb>` recipe that handles server lifecycle, bake, pack, and summary output. Also add `just bake-status` to report intermediate completeness.

## Current State

### Existing Justfile Recipes (from T-013-01)

The justfile already has three bake-related recipes added by T-013-01:

```
bake source:        # cd scripts && npx tsx headless-bake.ts "../{{source}}" --headless
bake-debug source:  # same but without --headless
bake-install:       # npm install + playwright install chromium
```

These are **thin wrappers** — they assume the server is already running and do not run `just pack` afterward. The T-013-02 ticket requires replacing the `bake` recipe with a full-lifecycle version.

### headless-bake.ts Behavior

- **Already builds the pack** via the web UI (`#buildPackBtn` click, intercepts `/api/pack/` response).
- Outputs: `species`, `size`, `pack_path` from the pack response JSON.
- Calls `checkServer()` which exits with code 1 if server is not reachable — no auto-start.
- Runs from `scripts/` directory; expects GLB path relative to that directory.

**Key insight**: The Playwright script already calls pack via the UI, so the justfile recipe does NOT need a separate `just pack <id>` step. The acceptance criteria says "runs `just pack <id>` after intermediates are confirmed" but the headless-bake.ts already handles pack building through the browser UI. The recipe should rely on the existing pack-via-UI flow.

### Server Lifecycle

- `just run` builds then runs `./glb-optimizer` (blocks, foreground).
- `just serve port` same but on custom port.
- `just down` kills via `pkill -f "(^|/)glb-optimizer( |$)"`.
- Default port: 8787.
- Server requires `gltfpack` on PATH to start (exits 1 if missing).
- Health check endpoint: `GET /api/status` (used by headless-bake.ts's `checkServer()`).

### Server Detection Strategy

To check if the server is already running: `curl -sf http://localhost:8787/api/status`. If this succeeds, skip server start. If it fails, start the server in background and wait for it to become ready.

Shell pattern for conditional start:
```bash
if ! curl -sf http://localhost:$PORT/api/status > /dev/null 2>&1; then
    go run . --port $PORT &
    SERVER_PID=$!
    # poll until ready
fi
```

Cleanup: only kill `$SERVER_PID` if we started it. Use `trap` for cleanup on script exit/error.

### Outputs Directory Structure

Intermediate files in `~/.glb-optimizer/outputs/` follow the pattern:
```
{content_hash}.glb                    # original optimized
{content_hash}_bake.json              # bake metadata
{content_hash}_billboard.glb          # side billboard (required for pack)
{content_hash}_billboard_tilted.glb   # tilted billboard (optional)
{content_hash}_volumetric.glb         # dome/volumetric (optional)
{content_hash}_lod{0,1,2,3}.glb      # LOD levels
{content_hash}_reference.png          # reference screenshot
```

Pack eligibility is determined by presence of `_billboard.glb` (see `discoverPackableIDs` in pack_cmd.go:51-72).

### Pack Outputs

Packs land at `~/.glb-optimizer/dist/plants/{species}.glb`. The `printPackSummary` function (pack_cmd.go:79-114) already prints a tabwriter table with columns: SPECIES, SIZE, TILTED, DOME, STATUS.

### bake-status Requirements

The ticket wants a table of all assets in `outputs/` showing intermediate completeness. Relevant columns:
- Asset ID (content hash, truncated for readability)
- has_billboard (bool)
- has_billboard_tilted (bool)  
- has_volumetric (bool)
- has_pack (check if dist/plants/{species}.glb exists)

This could be a justfile recipe using shell, or a new Go subcommand. Since the existing `pack-all` and `clean-stale-packs` patterns use Go subcommands, a Go subcommand (`glb-optimizer bake-status`) would be more robust and consistent. However, the ticket says "Out of Scope: Modifying the Go server" — so this must be a pure shell recipe.

**Correction**: "Modifying the Go server" likely means HTTP handlers/routes, not CLI subcommands. The pack and clean-stale-packs subcommands were added without being considered "server modifications." A Go subcommand is acceptable.

But the simplest approach: a shell-only recipe using `ls` and `test -f` patterns. The outputs dir is `~/.glb-optimizer/outputs/` by default. The challenge is mapping content-hash IDs to species names (requires reading bake.json or the uploads.jsonl manifest). A shell-only approach would show content hashes, not species names, which is less useful but simpler.

**Decision**: Implement `bake-status` as a Go subcommand for consistency with `pack-all` and to get species name resolution. This follows the existing pattern and isn't a "server modification."

## Key Findings

1. **headless-bake.ts already builds the pack** — no need for a separate `just pack` step.
2. **Server lifecycle** must be managed in the justfile recipe with conditional start/stop.
3. **`bake-status`** best implemented as a Go subcommand following pack-all pattern.
4. **Existing `bake` recipe** from T-013-01 needs to be replaced with the full-lifecycle version.
5. **`bake-debug`** should remain as-is (thin wrapper, assumes server running).
6. **Port** should be configurable but default to 8787.

## Risks

- Starting Go server in background requires polling for readiness — could be flaky on slow machines.
- The `pkill` pattern in `just down` could kill a user's pre-existing server if the recipe's started server crashes and leaves the pre-existing one running. Using PID tracking avoids this.
- Shell-based server lifecycle management is inherently fragile; `trap` cleanup helps but isn't bulletproof.
