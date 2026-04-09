# T-013-03 Progress: batch-bake

## Step 1: Refactor headless-bake.ts — extract bakeOne() ✅

- Extracted `bakeOne(page, glbPath, baseUrl): Promise<BakeResult>` from main()
- Added `export` to: `log`, `captureErrorScreenshot`, `formatBytes`, `BakeResult`, `bakeOne`
- Guarded `main()` with `isMain` check using `import.meta.url` so imports don't trigger CLI execution
- `main()` now delegates to `bakeOne()` — CLI behavior preserved

## Step 2: Verify headless-bake.ts single-file mode ✅

- `npx tsx headless-bake.ts` (no args) → shows usage, exits 1
- Exports compile cleanly: `import { bakeOne, formatBytes, log } from './headless-bake.ts'` → "imports ok"

## Step 3: Create scripts/batch-bake.ts ✅

- 155 lines: inbox scan → server check → browser launch → per-file bakeOne loop → summary table
- Default inbox: `../inbox` (relative to scripts/ dir, resolves to repo root `inbox/`)
- Moves successful files to `inbox/done/` via `renameSync`
- Per-file error handling: catch, screenshot, record, continue
- Summary table: FILENAME / SPECIES / SIZE / STATUS columns

## Step 4: Add bake-all recipe to justfile ✅

- `just bake-all inbox="inbox"` with bash shebang and server lifecycle
- Server lifecycle duplicated from `just bake` (prefixed `[just bake-all]`)
- Calls: `cd scripts && npx tsx batch-bake.ts "../{{inbox}}" --headless --port "$PORT"`

## Step 5: Verify compilation and CLI validation ✅

- `just --list` shows all 5 bake recipes correctly
- batch-bake.ts compiles and runs (finds inbox files, fails on no server — expected)
- batch-bake.ts with nonexistent dir → clean error, exit 1
- headless-bake.ts no-args → usage message, exit 1 (unchanged)
- Go build passes clean

## Deviations from Plan

- Added `isMain` guard to headless-bake.ts (not in plan). Required because ESM `import` of headless-bake.ts was triggering its `main()` call, causing batch-bake.ts to exit before its own code ran.
- Moved `captureErrorScreenshot` call out of `bakeOne` into callers (main and batch-bake). This gives callers control over error handling: batch-bake catches, screenshots, and continues; single-file main catches, screenshots, and exits.
