# T-013-03 Research: batch-bake

## Ticket Summary

Add a `just bake-all [inbox-dir]` recipe that processes every `.glb` in an inbox directory sequentially — uploading, baking (headless), and packing each one via the existing `headless-bake.ts` script — then prints a summary table and moves successful sources to `inbox/done/`.

## Existing Infrastructure

### headless-bake.ts (T-013-01)

- 233-line TypeScript CLI using Playwright chromium
- Accepts: `<source.glb> [--port N] [--headless]`
- Flow: upload → select file card → wait for model load → click #prepareForSceneBtn → wait all stages → click #buildPackBtn → verify pack on disk
- Returns: logs species, size, pack_path to stdout; exits 0 on success, 1 on failure
- Error screenshots saved to `dist/bake-errors/`
- **Key limitation**: processes exactly one file per invocation. Browser launches and closes each time.

### just bake (T-013-02)

- Full server lifecycle wrapper in justfile (bash shebang, `set -euo pipefail`)
- Checks if server already running via `curl /api/status`; starts `go run . --port $PORT &` if not
- PID-tracked with `trap cleanup EXIT` for graceful shutdown
- Calls: `cd scripts && npx tsx headless-bake.ts "../{source}" --headless --port "$PORT"`
- **One file at a time**: starts/stops server per invocation

### just bake-status (T-013-02)

- Go subcommand showing intermediate completeness table
- Useful for verifying batch results after bake-all completes

### inbox/ directory

- Already exists at repo root with `dahlia_blush.glb` as reference model
- Already in `.gitignore` (via `inbox/` pattern — confirmed by untracked status in git)
- No `inbox/done/` subdirectory exists yet

## Key Design Questions

### Q1: Where does the batch loop live?

**Option A: justfile recipe (bash)** — Loop over `*.glb` in justfile, call headless-bake.ts per file. Simple, consistent with existing `just bake` pattern.

**Option B: New TypeScript wrapper** — batch-bake.ts that imports/wraps headless-bake.ts logic, reusing one browser instance.

**Option C: New Go subcommand** — Go binary manages the loop, shells out to headless-bake.ts.

**Recommendation: Option A** — The ticket says "reuses a single Go server + browser instance across all assets." However, reusing the browser requires Option B (modifying headless-bake.ts or wrapping it). The justfile already handles server lifecycle; the browser-reuse requirement means we need a TypeScript-level batch wrapper.

**Revised recommendation: Option B** — Create a new `scripts/batch-bake.ts` that:
1. Accepts an inbox directory (default `inbox/`)
2. Launches one browser instance
3. Loops over GLB files, driving the same page actions as headless-bake.ts
4. Tracks results per file for the summary table
5. Moves successful sources to `inbox/done/`

The justfile `bake-all` recipe handles server lifecycle (reuse the pattern from `just bake`) and calls batch-bake.ts.

### Q2: Browser reuse between assets

headless-bake.ts launches and closes the browser per file. For batch mode, we need to:
- Launch browser once
- For each GLB: navigate to baseUrl (fresh page state), upload, bake, pack
- Close browser after all files processed

The simplest approach: extract the bake-one-file logic into a shared function, then batch-bake.ts calls it in a loop with the same browser/page.

**Alternative**: Refactor headless-bake.ts to export `bakeOne(page, glbPath)`, then batch-bake.ts imports it. This avoids code duplication but requires restructuring headless-bake.ts.

**Decision**: Refactor headless-bake.ts to export `bakeOne()`, keeping the single-file CLI entry point intact. batch-bake.ts imports and loops.

### Q3: Summary table format

Ticket specifies: `filename → species → pack size → status`

Can be printed to stdout as a simple aligned table after all files are processed.

### Q4: Error handling

- "Exit non-zero if any asset failed; continue processing the rest"
- Must catch errors per-file, not let one failure abort the batch
- Track failed files for final exit code and summary

### Q5: Moving to inbox/done/

- `rename()` (or `fs.rename`) moves the source file after successful bake+pack
- Create `inbox/done/` if it doesn't exist
- Only move on success — failed files stay in inbox for retry

## Constraints and Risks

1. **Browser state between assets**: After baking one file, the UI may retain state (file list, model viewer). Navigating to baseUrl fresh (`page.goto`) should reset everything.
2. **Server file accumulation**: Each upload adds files to the server's originals/outputs dirs. This is fine — pack-all already handles multiple assets.
3. **Existing headless-bake.ts callers**: `just bake` and `just bake-debug` both call headless-bake.ts directly. Refactoring must preserve the CLI entry point.
4. **tsx module resolution**: batch-bake.ts importing from headless-bake.ts works with tsx since both are in the same directory and package.json has `"type": "module"`.
