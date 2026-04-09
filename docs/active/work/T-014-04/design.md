# T-014-04 Design: CLI Prepare Subcommand

## Decision 1: File Location

**Choice:** New file `prepare_cmd.go` (+ `prepare_cmd_test.go`)

**Why:** Follows existing pattern — `pack_cmd.go`, `clean_cmd.go`, `bake_status.go` each own their subcommand. The prepare command is ~300 lines, warranting its own file.

## Decision 2: Asset ID Strategy

**Choice:** SHA-256 content hash, truncated to 32 hex characters (16 bytes)

**Why:** Matches `generateID()` output length (32 hex chars) so all downstream code (FileStore, settings, pack runner) works unchanged. Content-addressing gives idempotency — re-running `prepare` on the same file is a no-op if outputs already exist.

**Implementation:** `hashFile(path string) (string, error)` — reads file, computes `sha256.Sum256`, returns `hex.EncodeToString(sum[:16])`.

## Decision 3: Species Name Resolution

**Choice:** Derive species from the source filename stem (`dahlia_blush.glb` → `dahlia_blush`), passed as CLISpecies + CLICommonName to `ResolverOptions`.

**Why:** The prepare command knows the original filename (unlike the server, which loses it). Using it directly gives clean species names without requiring `--species` flags. The filename IS the species identity for CLI workflows.

**Format:** Strip `.glb` extension, lowercase, replace spaces/hyphens with underscores, truncate to 64 chars.

## Decision 4: Settings Lifecycle

**Choice:** Create default settings → apply category + strategy → save to disk, all inline before the render step.

**Why:** `BuildPackMetaFromBake` reads fade bands from settings on disk. The settings must exist and reflect the correct category before pack metadata can be built. No separate "settings creation" step — it's part of the classify step.

## Decision 5: Gltfpack Optimization Settings

**Choice:** Use default settings with `compression: "cc"` for the base optimization pass. LODs use the existing `lodConfigs` table.

**Why:** Matches what the server does in `handleProcess` — Draco compression, no aggressive simplification on the base pass. LOD configs are already defined and proven.

## Decision 6: Error Handling Strategy

**Choice:** Fail fast with step identification. Each pipeline step wraps errors with context (`"optimize: ..."`, `"render: ..."`). Non-zero exit on any failure. `--json` mode emits a JSON object with `status: "failed"`, `step`, and `error` fields.

**Why:** CLI users and agent consumers need to know WHERE the pipeline broke, not just that it broke. The ticket's acceptance criteria require "clear error message identifying which step failed."

## Decision 7: Blender Invocation

**Choice:** Reuse the `buildProductionConfig` pattern from `handleBuildProduction` — write JSON config to temp file, invoke `blender -b --python scripts/render_production.py -- --config <path>`, clean up config on exit.

**Why:** The render script already supports JSON config mode. This is exactly what the HTTP handler does. No mutex needed for CLI (single process).

## Decision 8: prepare-all Implementation

**Choice:** Sequential processing in a loop. Glob `inbox/*.glb`, process each with the same `runPrepare()` function, move successes to `inbox/done/`.

**Why:** Ticket explicitly says "sequential is fine — Blender is the bottleneck." Moving to `done/` prevents re-processing on subsequent runs.

## Decision 9: Resolution Flag

**Choice:** `--resolution` overrides `VolumetricResolution` in AssetSettings before render. Default 512.

**Why:** Matches the ticket spec. Applied to settings before the Blender config is built so all render params stay consistent.

## Decision 10: Skip Flags

**Choice:** `--skip-lods` skips step 5 only. `--skip-verify` skips step 8 only. Both are boolean flags.

**Why:** These are the two most time-consuming optional steps. Skip-lods is useful when iterating on render params. Skip-verify is useful when you know the pack format is stable.

## Decision 11: Output Summary Format

**Choice:** Human-readable table by default (matching the ticket's example output). `--json` emits a single JSON object with all sizes, durations, and status.

**Why:** Human readability for interactive use, machine readability for agents. The `--json` flag is the agent consumption mode per ticket spec.
