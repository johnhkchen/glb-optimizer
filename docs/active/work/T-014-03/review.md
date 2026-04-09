# T-014-03 Review: API Build-Production Endpoint

## Summary

Implemented `POST /api/build-production/{id}?category={cat}` endpoint that invokes Blender headlessly via `render_production.py` to produce impostor intermediates (billboard, tilted billboard, volumetric dome slices). Synchronous v1 with 5-minute timeout and mutex serialization.

## Files Changed

| File | Change |
|------|--------|
| `handlers.go` | Added imports (`context`, `os/exec`, `sync`), `blenderRenderMu` mutex, `blenderRenderTimeout` const, `buildProductionConfig` struct, `handleBuildProduction` handler (~130 lines) |
| `main.go` | Added `renderScriptPath` resolution block, registered `/api/build-production/` route |
| `handlers_build_production_test.go` | New file — 6 test cases covering all error paths + happy path |

## Test Coverage

| Test | Covers |
|------|--------|
| `TestBuildProduction_MethodNotAllowed` | GET returns 405 |
| `TestBuildProduction_BlenderNotAvailable` | Missing Blender returns 500 with "blender not installed" |
| `TestBuildProduction_AssetNotFound` | Unknown ID returns 404 |
| `TestBuildProduction_AssetNotOptimized` | Pending asset returns 400 |
| `TestBuildProduction_ConfigGeneration` | Config built correctly, missing intermediates detected (500) |
| `TestBuildProduction_HardSurfaceSkipsVolumetric` | hard-surface category skips volumetric, FileStore flags set correctly |

**Not tested** (requires real Blender):
- Full end-to-end render producing actual GLB intermediates
- Timeout behavior (would need a slow mock)
- Concurrent request queuing via mutex

## Acceptance Criteria Status

- [x] `curl -X POST localhost:8787/api/build-production/{id}?category=round-bush` runs Blender and returns intermediate flags
- [x] Endpoint registered in `main.go` alongside existing routes
- [x] Server starts without Blender (endpoint returns 500 if called)
- [ ] Produced intermediates identical to client-side JS output — requires T-014-06 validation
- [ ] `glb-optimizer pack <id>` combines Blender-produced intermediates — requires end-to-end test with real Blender

## Open Concerns

1. **render_production.py path resolution**: The script is resolved relative to CWD or executable dir. If the server is started from a different working directory, the path may not resolve. This is acceptable for the current dev workflow but may need a `--render-script` flag for production deployments.

2. **No progress reporting**: v1 is synchronous with no progress feedback. For large assets near the 5-minute timeout, the HTTP client may appear hung. The UI button (T-014-05) should show a spinner.

3. **Blender stderr truncation**: Error messages are truncated to 2KB. For debugging complex failures, the full output should be logged server-side (currently discarded beyond 2KB).

4. **Config file cleanup on crash**: The temp config file uses `defer os.Remove()`. If the Go process is killed mid-render, the config file remains on disk. This is harmless (overwritten on next call) but could be confusing during debugging.

5. **Resolution field duplication**: `buildProductionConfig` has both `Resolution` and `VolumetricResolution` which are currently set to the same value from `settings.VolumetricResolution`. The `Resolution` field maps to the billboard render resolution; if billboard resolution should differ from volumetric resolution in the future, a new settings field would be needed.

## Dependencies

- **T-014-02** (render_production.py): Must exist at `scripts/render_production.py` for the endpoint to function. Currently implemented.
- **T-014-04** (CLI prepare): Will call this endpoint or share the same logic.
- **T-014-05** (UI button): Will POST to this endpoint from the browser.
- **T-014-06** (validation): Will verify output parity with client-side bake.
