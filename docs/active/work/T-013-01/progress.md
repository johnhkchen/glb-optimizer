# T-013-01 Progress: Playwright Headless Bake

## Completed

### Step 1: Add Dependencies
- Added `playwright` and `tsx` as devDependencies to `scripts/package.json`
- Ran `npm install` — lock file regenerated

### Step 2: Install Playwright Chromium
- Ran `npx playwright install chromium` — Chromium binary downloaded
- Verified playwright is importable via Node.js

### Step 3: Create Script — Scaffolding + Arg Parsing
- Created `scripts/headless-bake.ts` with full implementation
- `parseArgs()` handles positional GLB path, `--port`, `--headless` flags
- Usage message on no args, error on missing file
- Verified: no-args → usage, nonexistent file → clear error

### Step 4: Server Check + Browser Launch
- `checkServer()` fetches `/api/status`, fails with clear message if unreachable
- Browser launches headed by default, headless with `--headless` flag
- try/finally ensures browser closes on error
- Verified: no server → "Start the server with: go run . (or just run)"

### Step 5: Upload Flow
- Uses `page.setInputFiles('#fileInput', glbPath)` to trigger upload
- Intercepts `/api/upload` response via `page.waitForResponse` to get file ID
- Validates response status and extracted file array

### Step 6: File Selection + Pipeline Trigger
- Clicks file card using `.file-item:has(.filename[title="..."])` selector
- Waits for `#prepareForSceneBtn` to become enabled (model loaded)
- Clicks `#prepareForSceneBtn` to start full pipeline

### Step 7: Stage Monitoring
- Uses `page.waitForFunction` to watch `#prepareStages li` elements
- Detects completion (all `li.ok`) or failure (any `li.error`)
- Logs each stage result after completion
- 5-minute timeout on the wait

### Step 8: Pack Build
- Clicks `#buildPackBtn` after pipeline completes
- Intercepts `/api/pack/:id` response for pack metadata
- Handles non-OK responses with error details from `#prepareError`

### Step 9: Pack Verification + Summary
- Verifies pack file exists on disk using `pack_path` from response
- Prints summary: path, species, size (human-readable via `formatBytes`)
- Exits 0 on success

### Step 10: Error Screenshot Capture
- `captureErrorScreenshot()` saves full-page PNG to `dist/bake-errors/`
- Filename pattern: `{stem}-{ISO timestamp}.png`
- Creates directory if needed
- Wired into all error paths (pipeline failure, pack failure, catch block)

### Step 11: Justfile + Gitignore
- Added `bake`, `bake-debug`, and `bake-install` recipes to justfile
- `dist/bake-errors/` already covered by existing `dist/` gitignore entry — no change needed

## Deviations from Plan

1. **Single commit instead of incremental**: Implemented the full script in one pass since the logic is straightforward and self-contained (~190 lines). The plan suggested incremental steps but each step was too small to commit independently for a single-file script.

2. **Added `bake-debug` recipe**: Not in the original plan but a natural complement — runs headed mode for debugging. Minimal addition.

3. **Added `bake-install` recipe**: Combines `npm install` + `npx playwright install chromium` for one-command setup.

4. **No separate gitignore change**: The existing `dist/` entry already covers `dist/bake-errors/`.

## Remaining

### Step 12: End-to-End Smoke Test
- Requires running Go server + dahlia_blush.glb in inbox/
- Cannot be automated in this session without a running server
- Manual verification deferred to user
