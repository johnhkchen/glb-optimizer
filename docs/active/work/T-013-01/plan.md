# T-013-01 Plan: Playwright Headless Bake

## Step 1: Add Dependencies to `scripts/package.json`

Add `playwright` and `tsx` as devDependencies. Run `npm install` in `scripts/`.

**Verification:** `ls scripts/node_modules/.package-lock.json` exists, `npx playwright --version` works from `scripts/`.

**Commit:** "T-013-01: add playwright and tsx dependencies"

## Step 2: Install Playwright Chromium Browser

Run `cd scripts && npx playwright install chromium` to download the Chromium binary.

**Verification:** `npx playwright install --dry-run` shows chromium installed.

**Note:** This is a local-only step — Chromium binary is not committed. CI environments would need this in their setup.

## Step 3: Create `scripts/headless-bake.ts` — Scaffolding + Arg Parsing

Write the initial script with:
- Imports (playwright, node:fs, node:path)
- Constants (DEFAULT_PORT, BAKE_TIMEOUT_MS, LOG_PREFIX)
- `log()` helper
- `parseArgs()` function — parse positional GLB path, `--port`, `--headless`
- `main()` entry point that parses args, validates the GLB file exists, prints usage on bad input
- `main().catch()` exit handler

**Verification:** `cd scripts && npx tsx headless-bake.ts` prints usage. `npx tsx headless-bake.ts /nonexistent.glb` prints error about missing file.

## Step 4: Add Server Check + Browser Launch

Add to `main()`:
- `checkServer(baseUrl)` — fetch `/api/status`, fail with "start the server" message if unreachable
- Launch Playwright Chromium (headed by default, headless if `--headless` flag)
- Navigate to `baseUrl`
- Wrap in try/finally to ensure browser closes

**Verification:** Run without server → get clear error message. Run with server → browser opens and navigates to UI.

## Step 5: Implement Upload Flow

Add to `main()` after navigation:
- Use `page.setInputFiles('#fileInput', absoluteGlbPath)` to trigger upload
- Wait for file to appear in the file list by polling the DOM or watching network
- Extract the file's server-assigned ID

Strategy for getting the file ID:
- After `setInputFiles`, intercept the `/api/upload` response via `page.waitForResponse`
- Parse the response JSON to get the `id` field

**Verification:** Run with server + valid GLB → file appears in UI file list, ID printed to console.

## Step 6: Implement File Selection + Pipeline Trigger

Add to `main()`:
- Click the file card in `#fileList` to select it (use the ID or filename to find the right element)
- Wait for `#prepareForSceneBtn` to be visible/enabled
- Click `#prepareForSceneBtn` to start the full pipeline
- Log "starting pipeline..."

**Verification:** Run full flow → UI shows pipeline progress starting.

## Step 7: Implement Stage Monitoring

Add `waitForStageCompletion()`:
- Watch `#prepareStages` children for stage status text
- Parse stage status: look for completion indicators (checkmark glyphs or "ok" text)
- Log each stage as it completes: "stage 1/4: optimize... done"
- If any stage shows error: capture screenshot, throw with stage name
- Overall timeout: BAKE_TIMEOUT_MS (5 minutes)

**Verification:** Run full flow → stages progress logged, all 4 stages complete.

## Step 8: Implement Pack Build

Add to `main()` after pipeline completion:
- Click `#buildPackBtn`
- Wait for result text in `#prepareError` (success or failure message)
- Parse the result to extract species and size
- On failure: capture screenshot, throw

**Verification:** Run full flow → pack built message displayed.

## Step 9: Implement Pack Verification + Summary

Add to `main()` after pack build:
- Poll `/api/files` to get the file record with pack metadata
- Use the pack response info to verify the file exists on disk
- Print summary: pack path, size (human-readable), species ID
- Exit 0 on success

**Verification:** Run full flow → summary printed, pack file exists on disk, exit code 0.

## Step 10: Implement Error Screenshot Capture

Add `captureErrorScreenshot()`:
- Create `dist/bake-errors/` directory if it doesn't exist
- Take full-page screenshot
- Save as `dist/bake-errors/{filename}-{ISO timestamp}.png`
- Return the screenshot path
- Wire into all error paths (stage failure, pack failure, timeout)

**Verification:** Simulate an error (e.g., stop server mid-bake) → screenshot saved to expected path.

## Step 11: Add Justfile Recipe + Gitignore

- Add `bake` recipe to `justfile`: `cd scripts && npx tsx headless-bake.ts ../{{source}} --headless`
- Add `dist/bake-errors/` to `.gitignore`

**Verification:** `just bake inbox/dahlia_blush.glb` runs the full pipeline.

**Commit:** "T-013-01: add just bake recipe and gitignore bake-errors"

## Step 12: End-to-End Smoke Test

Full manual verification:
1. Start server: `just run`
2. Run: `just bake inbox/dahlia_blush.glb`
3. Verify: pack file exists at `~/.glb-optimizer/dist/plants/dahlia_blush.glb`
4. Verify: pack size is reasonable (< 5 MB)
5. Verify: exit code is 0
6. Clean up: delete test outputs

## Testing Strategy

### Manual Integration Test (primary)
The smoke test in Step 12 is the primary verification. This is an integration tool that requires a running Go server and a real GLB file — unit testing the Playwright interactions would require mocking the entire browser, which defeats the purpose.

### What's Testable Without Server
- Arg parsing: valid/invalid args produce correct results
- File validation: missing GLB file produces correct error
- Server check: unreachable server produces correct error message

### What Requires Full Integration
- Upload flow (needs server + file)
- Bake pipeline (needs server + browser + GLB rendering)
- Pack build (needs server + baked intermediates)
- Error screenshots (needs failure scenario)

### Verification Criteria
- [ ] Script runs headless with `--headless` flag
- [ ] Script runs headed by default (browser window visible)
- [ ] Upload succeeds and file appears in list
- [ ] All 4 pipeline stages complete
- [ ] Pack file is written to disk
- [ ] Progress messages printed to stdout
- [ ] Timeout after 5 minutes produces error + screenshot
- [ ] Missing server produces clear error message
- [ ] Missing GLB file produces clear error message
- [ ] Non-zero exit code on any failure
