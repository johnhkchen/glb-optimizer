# T-013-04 Progress: Agent Bake-Pack Pipeline

## Step 1: Add jsonMode and setJsonMode to headless-bake.ts
**Status:** Done

Added module-level `jsonMode` boolean, `setJsonMode()` export, and modified `log()` to route to stderr when json mode is active. Added `--json` to ParsedArgs and parseArgs().

## Step 2: Add BakeError class to headless-bake.ts
**Status:** Done

Defined `BakeError extends Error` with `step` and `screenshot` fields. Exported for batch-bake.ts to use in type checks.

## Step 3: Add step tracking to bakeOne()
**Status:** Done

Added `currentStep` variable tracking through: upload → select → pipeline → (billboard/classify/lod/optimize) → pack. Wrapped entire bakeOne body in try/catch that converts errors to BakeError with step and screenshot info. Screenshot is captured inside bakeOne's catch before re-throwing.

## Step 4: Add JSON emission to headless-bake.ts main()
**Status:** Done

On success + json: emits `{"source","species","pack","size","status":"ok"}` to stdout.
On error + json: emits `{"source","error","step","screenshot?","status":"error"}` to stdout.
Without json: behavior unchanged.

## Step 5: Add --json to batch-bake.ts
**Status:** Done

Added `--json` flag parsing, `batchJsonMode` variable, `setJsonMode(true)` call. Per-file JSON emission in the loop (both success and error). Summary table suppressed in json mode.

## Step 6: Update justfile recipes
**Status:** Done

Both `bake` and `bake-all` recipes now accept `json=""` parameter. Bash log helper routes echo lines to stderr when json flag is set. `{{json}}` appended to TS script invocations.

## Step 7: Create docs/agent-pack-workflow.md
**Status:** Done

Created comprehensive agent documentation with: prerequisites, quick start, JSON schema, worked example with dahlia_blush.glb, troubleshooting table, exit codes, non-interactive guarantees.

## Step 8: Test CLI parsing and build
**Status:** Done

- Go build: clean
- Go tests: pass (cached, no Go changes)
- `npx tsx headless-bake.ts` usage: shows `--json` flag
- `just --list`: shows updated recipe signatures

## Deviations from Plan

- **BakeError screenshot capture moved into bakeOne:** The plan had screenshot capture in the callers, but since bakeOne already has page access and the try/catch wraps the whole body, it's cleaner to capture there. This means batch-bake.ts no longer needs to call `captureErrorScreenshot()` in its catch block — the BakeError already carries the screenshot path.

- **No separate CLI validation test file added:** The existing test infrastructure validates arg parsing via the usage output check. Adding a separate test file for --json parsing would be low-value given the args are parsed by the same loop.
