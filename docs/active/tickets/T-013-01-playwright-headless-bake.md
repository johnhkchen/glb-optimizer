---
id: T-013-01
story: S-013
title: playwright-headless-bake
type: task
status: open
priority: critical
phase: done
depends_on: []
---

## Context

The bake step (rendering billboard / tilted / volumetric intermediates) runs in the browser because it's client-side three.js code. Automating it requires driving the browser programmatically. Playwright is the simplest path — it automates exactly what a human does, with Chromium running headlessly.

## Acceptance Criteria

### Bake script

- New file: `scripts/headless-bake.ts` (TypeScript, run via `npx tsx`)
- Usage: `npx tsx scripts/headless-bake.ts <source.glb> [--port 8787] [--headless]`
- Steps:
  1. Verify the Go server is running on the specified port (or start it)
  2. Launch Playwright Chromium (headed by default for debugging, `--headless` flag for CI/agent use)
  3. Upload `<source.glb>` via the drag-drop zone (or POST to `/api/upload`)
  4. Wait for processing to complete (poll the file list API until `status: 'done'`)
  5. Click "Build hybrid impostor" button (or call the production bake API if one exists)
  6. Wait for all three intermediates to appear (`has_billboard`, `has_billboard_tilted`, `has_volumetric` all true)
  7. Click "Build Asset Pack" button (or POST to `/api/pack/:id`)
  8. Wait for pack to appear in `dist/plants/`
  9. Print pack path + size + species id
  10. Close browser

### Error handling

- Timeout after 5 minutes per asset (bake is CPU-intensive)
- If any step fails: print the step that failed, take a screenshot, save it to `dist/bake-errors/`, exit non-zero
- If the Go server isn't reachable: clear error message with "start the server with `go run .` first"

### Dependencies

- Add `playwright` as a devDependency (or use `npx playwright`)
- The script does NOT need the web frontend (SvelteKit) — it drives the Go server's static HTML UI
- The Go server must be running separately

### Tests

- Smoke test: upload `inbox/dahlia_blush.glb` (28 MB dahlia model), verify intermediates appear, verify pack is written as `dist/plants/dahlia_blush.glb`
- The test should clean up after itself (delete the test asset from outputs/)
- The `inbox/` directory at the repo root is the standard drop location for source GLBs awaiting bake

## Out of Scope

- Starting the Go server automatically (the script assumes it's running)
- Blender-based baking (future ticket)
- Modifying the existing UI code
- Running without a display server (headless Chromium works, but some CI environments may need xvfb)

## Notes

- The "Build hybrid impostor" button in the UI triggers a multi-step client-side bake that takes 30-120 seconds per asset depending on complexity. The Playwright script should show progress (e.g., "waiting for billboard... done. waiting for tilted... done. waiting for volumetric... done.").
- Use Playwright's `page.waitForSelector` / `page.waitForResponse` for polling rather than sleep loops.
- The Go server's `/api/files` endpoint returns `has_billboard`, `has_billboard_tilted`, `has_volumetric` flags — poll this for completion.
