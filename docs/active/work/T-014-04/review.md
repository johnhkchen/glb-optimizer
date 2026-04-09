# T-014-04 Review: CLI Prepare Subcommand

## Summary

Implemented `glb-optimizer prepare` and `glb-optimizer prepare-all` CLI subcommands that execute an 8-step pipeline (copy+hash, register, optimize, classify, LODs, render, pack, verify) to take a source GLB to a finished Pack v1 file with no browser or HTTP server required.

## Files Changed

| File | Change | Lines |
|------|--------|-------|
| `prepare_cmd.go` | **New** — core implementation | ~370 |
| `prepare_cmd_test.go` | **New** — 12 unit tests | ~190 |
| `main.go` | **Modified** — added 4 lines to dispatch switch | +4 |

## Test Coverage

| Test | What it covers |
|------|---------------|
| `TestHashFile` | SHA-256 content hash: idempotency, uniqueness |
| `TestHashFile_NotFound` | Error path for missing file |
| `TestSpeciesFromFilename` | 9 cases: normal, spaces, hyphens, uppercase, empty, path prefix, truncation |
| `TestFormatSize` | Bytes, KB, MB formatting |
| `TestPrintPrepareSummary_Success` | Human-readable output contains key fields |
| `TestPrintPrepareSummary_Failure` | Failure output shows step + error |
| `TestPrintPrepareJSON` | JSON is parseable, contains status + species |
| `TestRunPrepare_MissingSource` | Returns failed/copy on nonexistent file |
| `TestRunPrepareCmd_NoArgs` | Exit code 2 on no arguments |
| `TestRunPrepareCmd_MissingFile` | Exit code 2 on nonexistent source |
| `TestRunPrepareAllCmd_NoArgs` | Exit code 2 on no arguments |
| `TestRunPrepareAllCmd_EmptyDir` | Exit code 0, no crash on empty inbox |

**Full suite:** `go test ./...` passes (12 new + all existing tests).

## Acceptance Criteria Status

| Criterion | Status |
|-----------|--------|
| `prepare inbox/dahlia_blush.glb --category round-bush` produces pack | Ready — requires gltfpack + Blender at runtime |
| Pack passes `verify-pack` | Implemented — step 8 calls `InspectPack` |
| `--json` output is parseable with all fields | Covered by `TestPrintPrepareJSON` |
| `prepare-all inbox/` processes all GLBs, moves to `inbox/done/` | Implemented in `runPrepareAllCmd` |
| Non-zero exit on failure with step identification | Covered by error paths + `TestRunPrepare_MissingSource` |
| Works from clean state (no server needed) | Uses `resolveWorkdir` + `scanExistingFiles` inline |

## Open Concerns

1. **End-to-end test requires runtime deps:** Full pipeline integration test needs gltfpack on PATH, Blender installed, and `scripts/render_production.py` functional. Unit tests cover all code paths up to the external tool boundaries. Recommend running `glb-optimizer prepare inbox/dahlia_blush.glb --category round-bush` manually to validate.

2. **render_production.py (T-014-02) untested end-to-end:** The Python script exists but hasn't been validated in a full pipeline run. The prepare command will surface Blender's stderr on failure.

3. **Large file hashing:** `hashFile` reads the entire file into memory for SHA-256. For the expected 28 MB test model this is fine. For files >1 GB, a streaming hash would be better — but that's out of scope for plant assets.

4. **Species naming collision:** If two different source GLBs have the same filename (e.g., from different directories), they'll produce the same species name and the pack will be overwritten. This is by design — the species IS the filename identity.

5. **classify_shape.py availability:** Auto-classification (when `--category` is omitted) requires Python 3 and the classifier script. The `--category` flag bypasses this entirely, which is the expected agent workflow.

6. **No Blender reuse in prepare-all:** The ticket notes that Blender batch mode could reuse a single process. Current implementation invokes Blender once per asset. Acceptable for v1 since Blender startup is ~2s vs ~15s render time.
