# T-013-03 Plan: batch-bake

## Implementation Sequence

### Step 1: Refactor headless-bake.ts — extract bakeOne()

- Define and export `BakeResult` interface: `{ filename, species, size, packPath }`
- Extract the body of `main()` (lines 101-216, the try block contents) into `export async function bakeOne(page, glbPath, baseUrl): Promise<BakeResult>`
- `bakeOne` starts with `page.goto(baseUrl, { waitUntil: "networkidle" })` and ends by returning `{ filename, species, size, packPath }`
- `bakeOne` throws on error (caller handles screenshots and recovery)
- Add `export` to `captureErrorScreenshot`, `formatBytes`, `log`
- Rewrite `main()` to: parse args, check server, launch browser, create page, call `bakeOne(page, glbPath, baseUrl)`, log results, close browser
- Error handling in `main()`: catch from `bakeOne`, capture screenshot, exit 1

**Gotcha**: `bakeOne` must NOT close the browser — that's the caller's responsibility (main or batch).

### Step 2: Verify headless-bake.ts single-file mode still works

- Run the existing CLI validation tests: `cd scripts && npx tsx headless-bake.ts` (no args) should show usage and exit 1
- Confirm exports don't break the CLI entry point (tsx handles top-level `main()` call fine even with exports)

### Step 3: Create scripts/batch-bake.ts

- Import `bakeOne`, `BakeResult`, `captureErrorScreenshot`, `formatBytes`, `log` from `"./headless-bake.ts"`
- `parseArgs()`: inbox dir (positional, default `"../inbox"`), `--port`, `--headless`
- `main()` flow:
  1. Parse args, resolve inbox dir to absolute path
  2. Check inbox exists, scan for `*.glb` files (filter: not directory, ends with `.glb`)
  3. Sort files alphabetically for deterministic order
  4. Log count: `"found N .glb files in {inboxDir}"`
  5. If N=0, log "no files to process" and exit 0
  6. Check server reachable (reuse fetch pattern from headless-bake.ts)
  7. Launch browser
  8. For each file:
     a. Log `"[N/M] processing {filename}..."`
     b. Try: `const result = await bakeOne(page, filePath, baseUrl)`
     c. Success: record result, move to done/, log success
     d. Catch: capture screenshot, record failure, log error, continue
  9. Close browser
  10. Print summary table
  11. Exit with failures > 0 ? 1 : 0

### Step 4: Summary table format

```
FILENAME              SPECIES         SIZE      STATUS
dahlia_blush.glb      dahlia_blush    245 KB    ok
fern_boston.glb        fern_boston      312 KB    ok
broken_model.glb      —               —         ERROR: pipeline stage failed
```

Use console.log with padEnd() for alignment, or a simple tab-separated format.

### Step 5: Add bake-all recipe to justfile

- Insert after line 91 (after existing `bake` recipe, before `bake-debug`)
- Recipe signature: `bake-all inbox="inbox":`
- Bash shebang with server lifecycle (copy from `just bake`)
- Call: `cd scripts && npx tsx batch-bake.ts "../{{inbox}}" --headless --port "$PORT"`
- Add comment header explaining the recipe

### Step 6: Test batch-bake.ts CLI validation

- Run with no inbox dir (should use default)
- Run with nonexistent dir (should error clearly)
- Run with `--help` or bad flags (should show usage)

### Step 7: End-to-end verification plan

Cannot fully test without a running server + WebGL, but verify:
- `just bake-all` recipe parses correctly (`just --list` shows it)
- batch-bake.ts compiles without errors (`npx tsx --eval "import './batch-bake.ts'"` from scripts/)
- headless-bake.ts CLI still works (no-args exits with usage)

## Risk Mitigation

- **Browser state leaks**: `page.goto(baseUrl)` at start of each `bakeOne()` resets DOM. If issues arise, escalate to new page per asset.
- **Large inbox**: No parallelism — sequential is fine per ticket. Memory shouldn't be an issue since each bake cleans up via page navigation.
- **Partial runs**: Successfully baked files move to `done/`, so re-running processes only remaining files.
