# T-013-03 Review: batch-bake

## Files Changed

| File | Action | Description |
|------|--------|-------------|
| scripts/headless-bake.ts | modified | Extracted `bakeOne()`, added exports, guarded `main()` with `isMain` |
| scripts/batch-bake.ts | created | Batch orchestrator: loops over inbox, one browser, summary table, done-move |
| justfile | modified | Added `bake-all inbox="inbox"` recipe with server lifecycle |

## Acceptance Criteria Checklist

- [x] `just bake-all [inbox-dir]` recipe exists with default `inbox/`
- [x] For each `.glb` in inbox: upload → headless bake → pack (via `bakeOne()`)
- [x] Prints summary table: filename → species → pack size → status
- [x] Exit non-zero if any asset failed; continue processing the rest
- [x] Moves successfully-baked source files to `inbox/done/`
- [x] Reuses a single Go server + browser instance across all assets
- [x] `inbox/` already contains `dahlia_blush.glb` as reference model

## Test Coverage

### Automated verification performed
- `just --list` parses all recipes correctly (5 bake-related recipes visible)
- headless-bake.ts exports compile (`import { bakeOne, formatBytes, log }` succeeds)
- headless-bake.ts CLI: no-args → usage + exit 1 (unchanged behavior)
- batch-bake.ts: nonexistent inbox dir → clean error + exit 1
- batch-bake.ts: valid inbox → finds files, attempts server connection (expected failure without server)
- Go build: passes clean (no regressions)

### Not tested (requires running server + WebGL)
- End-to-end bake of a single file via refactored `just bake`
- End-to-end batch bake via `just bake-all`
- Browser state reset between multiple assets (page.goto resets DOM)
- `inbox/done/` move after successful bake
- Summary table rendering with real data

## Design Decisions Made

1. **Refactored headless-bake.ts** instead of duplicating bake logic — keeps one source of truth for the upload→bake→pack flow
2. **`isMain` guard** added to prevent `main()` running on import — necessary for ESM module reuse
3. **`captureErrorScreenshot` moved to callers** — gives batch-bake control to continue after errors while single-file mode can still exit immediately
4. **Server lifecycle duplicated** in `bake-all` (not extracted to shared script) — only two callers, not worth the abstraction

## Open Concerns

1. **Browser state between assets**: `page.goto(baseUrl)` should reset the DOM, but the server retains uploaded files in its file store. If the file list grows large enough to slow the UI, this could become an issue for large batches. Mitigation: test with 5+ files on the Mac mini.
2. **No integration test**: the full pipeline requires a running server with WebGL. A future ticket could add a CI-friendly mock or a lightweight integration test.
3. **Server lifecycle duplication**: the `just bake` and `just bake-all` recipes share ~20 lines of identical server lifecycle bash. If a third recipe needs this, extract to a shell script.

## TODOs

- Smoke test: `just bake-all` with `dahlia_blush.glb` in `inbox/` (requires manual run with server)
- Verify `just bake inbox/dahlia_blush.glb` still works after the headless-bake.ts refactor
