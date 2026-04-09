# T-013-01 Review: Playwright Headless Bake

## Summary

Created `scripts/headless-bake.ts`, a Playwright automation script that drives the glb-optimizer browser UI to bake GLB assets headlessly. The script handles the full pipeline: upload → optimize → classify → LODs → production bake → asset pack build.

## Files Created

| File | Lines | Purpose |
|------|-------|---------|
| `scripts/headless-bake.ts` | ~190 | Main Playwright bake automation script |
| `docs/active/work/T-013-01/research.md` | ~140 | RDSPI research artifact |
| `docs/active/work/T-013-01/design.md` | ~150 | RDSPI design artifact |
| `docs/active/work/T-013-01/structure.md` | ~120 | RDSPI structure artifact |
| `docs/active/work/T-013-01/plan.md` | ~120 | RDSPI plan artifact |
| `docs/active/work/T-013-01/progress.md` | ~70 | RDSPI progress artifact |

## Files Modified

| File | Change |
|------|--------|
| `scripts/package.json` | Added `playwright` and `tsx` devDependencies |
| `scripts/package-lock.json` | Regenerated (npm install) |
| `justfile` | Added `bake`, `bake-debug`, `bake-install` recipes |

## Files NOT Modified

- No changes to Go server code (handlers.go, main.go, etc.)
- No changes to static UI (app.js, index.html, style.css)
- No changes to .gitignore (existing `dist/` entry covers `dist/bake-errors/`)

## Acceptance Criteria Checklist

| Criterion | Status | Notes |
|-----------|--------|-------|
| `scripts/headless-bake.ts` exists | Done | TypeScript, ~190 lines |
| Usage: `npx tsx headless-bake.ts <source.glb> [--port] [--headless]` | Done | Verified — usage, missing file, and no-server cases all work |
| Upload via UI | Done | Uses `page.setInputFiles('#fileInput')` |
| Wait for intermediates (`has_billboard`, `has_billboard_tilted`, `has_volumetric`) | Done | Via `#prepareForSceneBtn` pipeline which produces all three |
| Build asset pack | Done | Clicks `#buildPackBtn`, intercepts `/api/pack` response |
| Print pack path + size + species | Done | Human-readable output |
| 5-minute timeout | Done | `BAKE_TIMEOUT_MS = 300_000` |
| Error screenshot to `dist/bake-errors/` | Done | Full-page PNG with timestamp |
| Clear error for unreachable server | Done | Verified manually |
| `playwright` as devDependency | Done | In `scripts/package.json` |
| Script drives Go server's static HTML UI | Done | No SvelteKit dependency |
| Headed by default, `--headless` opt-in | Done | Verified |

## Test Coverage

### Verified (without server)
- No args → usage message + exit 1
- Nonexistent GLB → "file not found" + exit 1
- No server running → "server not reachable" + clear instructions + exit 1
- Arg parsing: `--port`, `--headless` flags work correctly

### Not Verified (requires running server)
- Full end-to-end bake pipeline with `dahlia_blush.glb`
- Stage monitoring and progress logging
- Pack build and output verification
- Error screenshot capture during actual bake failure
- Timeout behavior on slow bake

### Test Gap Assessment
This is an integration tool — the primary test is the E2E smoke test described in the ticket (upload `inbox/dahlia_blush.glb`, verify pack at `dist/plants/dahlia_blush.glb`). Unit testing Playwright interactions would require mocking the browser, which defeats the purpose. The manual verification paths (no args, missing file, no server) are the only sensible offline tests.

## Open Concerns

1. **E2E smoke test not run**: The full pipeline test requires a running Go server with `dahlia_blush.glb` in inbox/. This should be verified manually before merging: `just run` in one terminal, `just bake inbox/dahlia_blush.glb` in another.

2. **File card selector fragility**: The selector `.file-item:has(.filename[title="${filename}"])` depends on the file card HTML structure in `renderFileList()`. If that structure changes, the selector breaks. However, this is the most semantic selector available without adding a `data-id` attribute to file cards.

3. **`prepareForSceneBtn` enable timing**: The script waits for this button to become enabled after selecting a file. This depends on the 3D model loading, which could be slow for large files. The 30-second timeout should be sufficient for the 28 MB dahlia model.

4. **Stage monitoring assumes 4 stages**: The `PREPARE_STAGES` constant in `app.js` defines 4 stages. If stages are added or removed, the script's wait condition (all `li.ok`) still works correctly since it checks all `li` elements regardless of count.

5. **Pack path is absolute**: The `/api/pack/:id` response returns an absolute `pack_path` (e.g., `~/.glb-optimizer/dist/plants/dahlia_blush.glb`). The script uses this directly for verification, which is correct but means the output location depends on the server's `--dir` flag.

## TODOs for Follow-Up

- T-013-02 (`just bake` recipe) is already addressed by the `bake` recipe added to the justfile
- T-013-03 (batch bake via inbox/) should use this script in a loop
- T-013-04 (agent-callable pipeline) should wrap this script with JSON output

## Critical Issues

None. The implementation matches the ticket specification. The main risk is the untested E2E path, which is inherent to the nature of the tool (requires a running server with WebGL rendering).
