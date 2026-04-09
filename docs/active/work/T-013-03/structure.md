# T-013-03 Structure: batch-bake

## New Files

### scripts/batch-bake.ts (~120 lines)
Batch orchestrator. Imports `bakeOne` from headless-bake.ts.

```
Exports: none (CLI entry point)
Imports: { bakeOne, BakeResult, captureErrorScreenshot, formatBytes } from "./headless-bake.ts"
         { chromium } from "playwright"
         { readdirSync, mkdirSync, renameSync, existsSync } from "node:fs"
         { resolve, join, basename } from "node:path"

Functions:
  parseArgs(argv: string[]): { inboxDir: string; port: number; headless: boolean }
  main(): Promise<void>

Types:
  interface FileResult { filename: string; species: string; size: number; status: "ok" | "error"; error?: string }
```

## Modified Files

### scripts/headless-bake.ts
- Extract `bakeOne(page, glbPath, baseUrl): Promise<BakeResult>` from the body of `main()`
- Export: `bakeOne`, `BakeResult`, `captureErrorScreenshot`, `formatBytes`, `log`
- `main()` remains and calls `bakeOne()` — CLI entry point unchanged
- Add `export` keyword to `interface BakeResult` (new), `captureErrorScreenshot`, `formatBytes`, `log`

### justfile
- Add `bake-all` recipe after existing `bake` recipe (line ~91)
- Parameters: `inbox="inbox"` (default value)
- Server lifecycle block: duplicated from `just bake`
- Script call: `cd scripts && npx tsx batch-bake.ts "../{{inbox}}" --headless --port "$PORT"`

## File Change Summary

| File | Action | Lines ±  |
|------|--------|----------|
| scripts/headless-bake.ts | modify | +25 / -10 (extract bakeOne, add exports) |
| scripts/batch-bake.ts | create | ~120 |
| justfile | modify | +30 (new bake-all recipe) |

## No Changes Required

- `bake_status.go` — no changes; bake-all doesn't interact with it
- `package.json` — no new dependencies needed; playwright and tsx already present
- `.gitignore` — `inbox/` already covered; `inbox/done/` is a subdirectory, also covered
