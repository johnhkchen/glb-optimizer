# T-013-04 Design: Agent Bake-Pack Pipeline

## Decision Summary

Add `--json` flag to headless-bake.ts and batch-bake.ts. Thread it through justfile via recipe parameters. Redirect debug logs to stderr when `--json` is active. Create agent documentation at `docs/agent-pack-workflow.md`.

## Option A: TypeScript-Only JSON (Chosen)

All output formatting lives in the TypeScript scripts. The justfile recipes gain a `json=""` parameter that, when set to `--json`, gets appended to the script invocation.

**Pros:**
- Minimal change surface — only 2 TS files + justfile + 1 new doc
- Consistent with existing architecture (TS owns output)
- No Go changes needed
- JSON output matches ticket spec exactly

**Cons:**
- justfile parameter syntax is slightly awkward (`just bake foo.glb --json`)

## Option B: Go Wrapper Subcommand (Rejected)

Add a `bake` subcommand to main.go that wraps the TypeScript scripts and adds JSON formatting.

**Rejected because:**
- Duplicates the server lifecycle logic already in justfile
- Adds Go↔TS coordination complexity
- The TS scripts already have the data; adding a Go intermediary is overhead

## Option C: Environment Variable for JSON Mode (Rejected)

Use `JSON=1 just bake foo.glb` instead of `--json` flag.

**Rejected because:**
- Inconsistent with pack-inspect's `--json` flag convention
- Less discoverable for agents parsing `--help` output
- Ticket explicitly says "accept a `--json` flag"

## Detailed Design

### 1. `--json` Flag in TypeScript Scripts

**headless-bake.ts:**
- Add `--json` to `ParsedArgs` interface and `parseArgs()`
- When `--json` is active, `log()` writes to stderr instead of stdout
- On success: emit one JSON line to stdout matching ticket spec:
  ```json
  {"source":"achillea.glb","species":"achillea_millefolium","pack":"dist/plants/achillea_millefolium.glb","size":1842311,"status":"ok"}
  ```
- On error: emit one JSON line to stdout:
  ```json
  {"source":"bad.glb","error":"bake timeout after 300s","step":"billboard","screenshot":"dist/bake-errors/bad_billboard.png"}
  ```
- When `--json` is NOT active: behavior unchanged (human-readable logs)

**batch-bake.ts:**
- Add `--json` to `ParsedArgs` and `parseArgs()`
- Thread `jsonMode` to log functions → stderr when active
- Instead of summary table, emit one JSON line per file (NDJSON)
- `FileResult` already has the right shape; just serialize it
- Export `jsonMode` or pass as parameter to imported `log()` / `bakeOne()`

### 2. Justfile Recipe Changes

```just
bake source json="":
    ...
    cd scripts && npx tsx headless-bake.ts "../{{source}}" --headless --port "$PORT" {{json}}

bake-all inbox="inbox" json="":
    ...
    cd scripts && npx tsx batch-bake.ts "../{{inbox}}" --headless --port "$PORT" {{json}}
```

Usage: `just bake inbox/dahlia_blush.glb --json` or `just bake-all --json`

The server lifecycle bash output goes to stderr naturally (via `echo` to fd1, but the TS script's stdout is the only thing an agent should parse). Actually — the bash `echo` lines also go to stdout. We need to redirect them to stderr when json mode is active.

**Fix:** When `json` param is non-empty, redirect all bash echo lines to `>&2`.

### 3. Log Routing

The key insight: when `--json` is active, ALL human-readable output must go to stderr. Only JSON goes to stdout. This lets an agent do:

```bash
result=$(just bake inbox/foo.glb --json 2>/dev/null)
```

Implementation:
- `log()` in headless-bake.ts checks a module-level `jsonMode` flag
- `batchLog()` in batch-bake.ts does the same
- justfile recipe bash `echo` lines redirect to stderr when json param is set

### 4. Error Step Detection

The ticket wants `"step":"billboard"` in error JSON. The bakeOne() function progresses through stages: upload → prepare → billboard → tilted → dome → pack. We can detect which step failed by wrapping each stage in a try/catch or by checking which stage was last entered before the error.

Simpler approach: bakeOne() already throws on failure. The error message often contains the stage name. For structured output, we add a `step` field to the error catch in main(), derived from the last stage bakeOne() was executing. We can expose this by having bakeOne() set a `currentStep` variable that the catch block reads.

### 5. Agent Documentation

`docs/agent-pack-workflow.md` — standalone file, not inside work artifacts:
- Prerequisites: Go, Node.js 18+, Playwright (`just bake-install`)
- Worked example: single file and batch
- JSON output parsing examples
- Troubleshooting table: common errors → fixes
- Verification: `just verify-pack <species>`

### 6. Non-Interactive Guarantees

Already met by T-013-01 through T-013-03. This ticket documents them and ensures `--json` doesn't introduce any interactive behavior.

## Output Contract

### Success (single file)
```json
{"source":"dahlia_blush.glb","species":"dahlia_blush","pack":"dist/plants/dahlia_blush.glb","size":1842311,"status":"ok"}
```

### Error (single file)
```json
{"source":"bad.glb","error":"bake timeout after 300s","step":"billboard","screenshot":"dist/bake-errors/bad_error.png","status":"error"}
```

### Batch (NDJSON — one line per file)
```json
{"source":"dahlia_blush.glb","species":"dahlia_blush","pack":"dist/plants/dahlia_blush.glb","size":1842311,"status":"ok"}
{"source":"bad.glb","error":"timeout","step":"pack","screenshot":"dist/bake-errors/bad_error.png","status":"error"}
```

## Risk

- The justfile `echo` lines and the TS `log()` lines share stdout. Routing to stderr is essential for clean JSON. Missing a single `console.log` in the bake flow would corrupt the output.
- Step detection in errors is best-effort — some errors (server crash, Playwright timeout) don't map cleanly to a bake step.
