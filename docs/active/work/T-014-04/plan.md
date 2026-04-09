# T-014-04 Plan: CLI Prepare Subcommand

## Step 1: Create prepare_cmd.go with hashFile and speciesFromFilename helpers

Write the utility functions first since they have no dependencies on the rest of the file:
- `hashFile(path) (string, error)` — SHA-256, return first 16 bytes as hex
- `speciesFromFilename(name) string` — strip extension, lowercase, sanitize

**Verify:** Unit test both functions in prepare_cmd_test.go.

## Step 2: Implement runPrepare (core pipeline)

The 8-step pipeline function. Takes `sourcePath` and `prepareOptions`, returns `prepareResult`. Each step:

1. **Copy+Hash:** `hashFile(sourcePath)` → id. Check if `originals/{id}.glb` exists (skip copy if so). Otherwise `copyFile(src, dst)`.
2. **Register:** Create FileStore, `scanExistingFiles()`, `AppendUploadRecord()` for species resolver.
3. **Optimize:** If `outputs/{id}.glb` doesn't exist, run gltfpack with default cc settings via `BuildCommand` + `RunGltfpack`. Update FileStore status.
4. **Classify:** If `--category` given, use it directly. Otherwise `RunClassifier(originals/{id}.glb)`. Call `applyClassificationToSettings` + `applyShapeStrategyToSettings` + `SaveSettings`.
5. **LODs:** Unless `--skip-lods`, iterate `lodConfigs`, run gltfpack per level. Skip existing outputs.
6. **Render:** `DetectBlender()` — fail if unavailable. Build `buildProductionConfig` from settings + strategy. Write temp config JSON. `exec.Command(blender, "-b", "--python", scriptPath, "--", "--config", configPath)`. Verify intermediates exist.
7. **Pack:** `RunPack(id, ...)` with `ResolverOptions{CLISpecies, CLICommonName}` derived from source filename.
8. **Verify:** Unless `--skip-verify`, `InspectPack(packPath)`. Fail if report shows errors.

Each step sets `result.FailedStep` and `result.Error` on failure, returns immediately.

**Risk:** lodConfigs is declared in handlers.go — it's package-level, so accessible from prepare_cmd.go. Verify this compiles.

## Step 3: Implement runPrepareCmd

Flag parsing wrapper:
- `flag.NewFlagSet("prepare", flag.ContinueOnError)`
- Flags: `--category`, `--resolution` (default 512), `--dir`, `--json`, `--skip-lods`, `--skip-verify`
- Validate: exactly 1 positional arg (source GLB path), file must exist
- Call `runPrepare()`, print result via `printPrepareSummary` or `printPrepareJSON`
- Return 0 on success, 1 on failure

## Step 4: Implement runPrepareAllCmd

- Same flags as prepare (minus source path — takes a directory)
- Glob `{dir}/*.glb`, process each sequentially
- On success: move source to `{dir}/done/` (create `done/` if needed)
- Print per-file summary, then overall summary
- Return 0 if all succeeded, 1 if any failed

## Step 5: Implement output formatters

- `printPrepareSummary(w, result)` — human-readable table matching ticket example
- `printPrepareJSON(w, result)` — JSON with all fields

## Step 6: Wire into main.go

Add `"prepare"` and `"prepare-all"` cases to the switch in `main.go:25-37`.

## Step 7: Write tests

- `TestHashFile` — known input, known hash
- `TestSpeciesFromFilename` — various filename formats
- `TestRunPrepare_MissingSource` — error path
- `TestPrintPrepareSummary` — output format validation
- `TestPrintPrepareJSON` — JSON parseable, contains required fields

## Step 8: Compile check

Run `go build ./...` and `go vet ./...` to verify everything compiles.

## Step 9: Run tests

Run `go test ./...` to verify all tests pass (including existing tests).

## Risks & Mitigations

1. **Blender not available in CI/test:** Tests that need Blender should be skipped with `t.Skip("blender not available")`. The `runPrepare` function itself checks `DetectBlender()` and returns a clear error.
2. **render_production.py might not be complete:** The script exists at `scripts/render_production.py` (T-014-02). If it has issues, the render step will fail with Blender's stderr — which is surfaced to the user.
3. **lodConfigs accessibility:** It's a package-level var in handlers.go — accessible from prepare_cmd.go since they're in the same package.
