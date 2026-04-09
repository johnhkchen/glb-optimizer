# T-013-04 Plan: Agent Bake-Pack Pipeline

## Step 1: Add jsonMode and setJsonMode to headless-bake.ts

- Add `let jsonMode = false;` module-level variable
- Export `setJsonMode(flag: boolean)` function
- Modify `log()` to use `process.stderr.write()` when `jsonMode` is true
- Add `--json` to ParsedArgs and parseArgs()
- Set `jsonMode` from parsed args in `main()`

**Verify:** Existing tests still pass (CLI validation tests from T-013-01).

## Step 2: Add BakeError class to headless-bake.ts

- Define `BakeError extends Error` with `step` and `screenshot` fields
- Export the class for batch-bake.ts to use in type checks

**Verify:** No behavior change yet — class defined but not thrown.

## Step 3: Add step tracking to bakeOne()

- Add `let currentStep = "upload"` at top of bakeOne()
- Set `currentStep` before each major stage: upload, prepare, billboard, tilted, dome, pack
- Wrap the final catch (or add a wrapper) that converts errors to BakeError with the current step
- Before throwing BakeError, call `captureErrorScreenshot()` and attach the path

**Verify:** Existing bake behavior unchanged — errors now carry step metadata.

## Step 4: Add JSON emission to headless-bake.ts main()

- On success + jsonMode: `console.log(JSON.stringify({ source, species, pack, size, status: "ok" }))`
- On error + jsonMode: `console.log(JSON.stringify({ source, error, step, screenshot, status: "error" }))`
- On success without jsonMode: existing behavior (log "done")
- On error without jsonMode: existing behavior (stderr message)

**Verify:** Run `npx tsx headless-bake.ts --help` or invalid args to check --json is parsed. Full bake test deferred to manual smoke test.

## Step 5: Add --json to batch-bake.ts

- Add `json` to ParsedArgs and parseArgs()
- After parsing, call `setJsonMode(json)` from imported headless-bake
- Modify `batchLog()` to use `process.stderr.write()` when json mode active
- In the per-file loop, when json mode:
  - On success: `console.log(JSON.stringify({ source, species, pack, size, status: "ok" }))`
  - On error: `console.log(JSON.stringify({ source, error, step, screenshot, status: "error" }))`
- After loop, skip summary table when json mode active
- Keep `FileResult` for internal tracking (exit code depends on it)

**Verify:** Parse test with `--json` flag.

## Step 6: Update justfile recipes

- `bake` recipe: change signature to `bake source json="":`
  - When `{{json}}` is non-empty, redirect bash echo lines to `>&2`
  - Append `{{json}}` to the npx tsx invocation
- `bake-all` recipe: change signature to `bake-all inbox="inbox" json="":`
  - Same stderr redirect pattern
  - Append `{{json}}` to the npx tsx invocation

**Verify:** `just --list` shows updated recipes. `just bake --json` passes flag through.

## Step 7: Create docs/agent-pack-workflow.md

Write the agent documentation:
- Prerequisites section
- Quick start with single + batch examples
- JSON schema documentation (success + error)
- Worked example using inbox/dahlia_blush.glb
- Troubleshooting table
- Verification via just verify-pack

**Verify:** File exists and renders correctly.

## Step 8: Test headless-bake.ts CLI parsing with --json

- Update or add to existing CLI validation tests to cover --json flag parsing
- Verify parseArgs handles --json correctly
- Verify parseArgs still works without --json

**Verify:** `cd scripts && npx tsx headless-bake.ts 2>&1` shows usage includes --json.

## Testing Strategy

- **Unit tests:** CLI arg parsing for --json in both scripts
- **Manual smoke test:** `just bake inbox/dahlia_blush.glb --json` — verify stdout is valid JSON, stderr has progress logs
- **Manual smoke test:** `just bake-all --json` — verify NDJSON on stdout
- **Regression:** Existing non-json bake behavior unchanged
- **Edge case:** Error path produces valid JSON (can test with a nonexistent file if server is running)

## Gotchas

1. `console.log()` adds a newline; `JSON.stringify()` does not. Using `console.log(JSON.stringify(...))` gives exactly one JSON line per result.
2. The justfile `echo` lines in the server lifecycle block also go to stdout. Must redirect to stderr in json mode or the agent gets garbage mixed with JSON.
3. `bakeOne()` is called by both headless-bake main() and batch-bake main(). Step tracking must be inside bakeOne(), not in the callers.
4. The `captureErrorScreenshot()` function is async and can itself throw. The error JSON emission must handle this gracefully.
