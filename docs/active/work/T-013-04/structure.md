# T-013-04 Structure: Agent Bake-Pack Pipeline

## Files Modified

### `scripts/headless-bake.ts`

- Add `json` field to `ParsedArgs` interface
- Add `--json` parsing to `parseArgs()`
- Export a module-level `jsonMode` boolean (set by parseArgs or by batch-bake.ts before calling bakeOne)
- Modify `log()` to write to `process.stderr` when `jsonMode` is true
- Add `currentStep` tracking inside `bakeOne()` — set before each major stage
- Add JSON result emitter in `main()`:
  - On success: `JSON.stringify({ source, species, pack, size, status: "ok" })`
  - On error: `JSON.stringify({ source, error, step, screenshot, status: "error" })`
- Export `setJsonMode(flag: boolean)` for batch-bake.ts to call

### `scripts/batch-bake.ts`

- Add `json` field to `ParsedArgs` interface
- Add `--json` parsing to `parseArgs()`
- Call `setJsonMode(true)` on imported headless-bake module when json flag is active
- Modify `batchLog()` to write to `process.stderr` when json mode is active
- Replace summary table with NDJSON output when `--json`:
  - One `JSON.stringify(...)` per completed file, written to stdout
  - Suppress table rendering
- Keep human table output when `--json` is not set (no behavior change)

### `justfile`

- `bake` recipe: add `json=""` parameter, append `{{json}}` to headless-bake.ts invocation, redirect bash echo lines to stderr when json is set
- `bake-all` recipe: add `json=""` parameter, append `{{json}}` to batch-bake.ts invocation, redirect bash echo lines to stderr when json is set

## Files Created

### `docs/agent-pack-workflow.md`

Agent-facing documentation. Sections:
1. **Prerequisites** — Go, Node.js, Playwright, gltfpack
2. **Quick Start** — single bake and batch bake with `--json`
3. **JSON Output Schema** — success and error shapes
4. **Worked Example** — `inbox/dahlia_blush.glb` → pack, with expected output
5. **Troubleshooting** — table of common errors and fixes
6. **Verification** — `just verify-pack <species>` after baking

## Files NOT Modified

- **Go source files** — no changes needed; all output formatting is in TypeScript
- **pack_inspect.go** — existing `--json` is a reference pattern, not modified
- **static/** — no UI changes
- **bake_status.go** — out of scope (no agent JSON needed for status)

## Module Boundaries

```
justfile bake/bake-all
  └─ passes --json to TS scripts
  └─ redirects own echo to stderr when json mode

headless-bake.ts
  └─ exports: bakeOne(), log(), captureErrorScreenshot(), formatBytes(),
              setJsonMode(), BakeResult
  └─ jsonMode: module-level boolean, controls log routing + result emission
  └─ bakeOne() adds currentStep tracking, throws BakeError with step field

batch-bake.ts
  └─ imports from headless-bake.ts
  └─ calls setJsonMode() before bakeOne loop
  └─ emits NDJSON or table based on flag
```

## Interface Changes

### New: `BakeError` class (headless-bake.ts)

```typescript
class BakeError extends Error {
  step: string;
  screenshot?: string;
  constructor(message: string, step: string, screenshot?: string) {
    super(message);
    this.step = step;
    this.screenshot = screenshot;
  }
}
```

Thrown by `bakeOne()` when a stage fails. The `step` field identifies which bake stage (upload, prepare, billboard, tilted, dome, pack). The `screenshot` field is populated by `captureErrorScreenshot()` before throwing.

### New: `setJsonMode(flag: boolean)` (headless-bake.ts)

Exported function that sets the module-level `jsonMode` boolean. Called by batch-bake.ts to ensure `log()` routes to stderr during JSON mode.

### Modified: `bakeOne()` return type

No change to `BakeResult` interface. The function now internally tracks `currentStep` and wraps stage failures in `BakeError`.

## Change Ordering

1. headless-bake.ts — add jsonMode, BakeError, step tracking, JSON emission
2. batch-bake.ts — add --json flag, NDJSON output, setJsonMode() call
3. justfile — add json parameter to bake and bake-all recipes
4. docs/agent-pack-workflow.md — create documentation
