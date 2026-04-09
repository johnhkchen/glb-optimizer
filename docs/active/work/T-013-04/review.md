# T-013-04 Review: Agent Bake-Pack Pipeline

## Summary of Changes

### Files Modified

| File | Changes |
|------|---------|
| `scripts/headless-bake.ts` | Added `--json` flag, `jsonMode`/`setJsonMode()`, `BakeError` class, step tracking in `bakeOne()`, JSON emission in `main()` |
| `scripts/batch-bake.ts` | Added `--json` flag, `batchJsonMode`, NDJSON output per file, conditional summary table |
| `justfile` | Added `json=""` parameter to `bake` and `bake-all` recipes, stderr-routing `log()` helper |

### Files Created

| File | Purpose |
|------|---------|
| `docs/agent-pack-workflow.md` | Agent-facing documentation: prerequisites, JSON schema, worked example, troubleshooting |

### Files Not Modified

No Go source files changed. The `--json` feature is entirely in the TypeScript/justfile layer.

## Acceptance Criteria Assessment

### Structured output
- **`just bake <file> --json`**: Emits one JSON line to stdout with `source`, `species`, `pack`, `size`, `status` fields. Matches ticket spec.
- **`just bake-all --json`**: Emits one JSON line per file (NDJSON). Matches ticket spec.
- **Error JSON**: Includes `error`, `step`, and `screenshot` fields. Matches ticket spec.
- **Log routing**: All human-readable output goes to stderr when `--json` is active. Stdout is clean JSON only.

### Agent documentation
- **`docs/agent-pack-workflow.md` created**: Contains prerequisites, quick start, JSON schema docs (success + error), worked example with `inbox/dahlia_blush.glb`, troubleshooting table with 8 common errors, exit codes, non-interactive guarantees.
- **Verification step**: Documents `just verify-pack dahlia_blush` (references T-012-03).

### Non-interactive
- **No prompts**: Already guaranteed by T-013-01 headless bake design.
- **Headless browser**: `--headless` flag passed automatically by justfile recipes.
- **Auto server lifecycle**: justfile starts/stops server via trap.
- **Auto species resolution**: Resolver chain handles species ID — documented in agent workflow.
- **Default paths**: Working directory defaults to `~/.glb-optimizer/`.

## Test Coverage

| Area | Coverage | Notes |
|------|----------|-------|
| Go build | Pass | No Go changes, build still clean |
| Go tests | Pass | Cached — no Go source modified |
| CLI arg parsing | Verified | `npx tsx headless-bake.ts` shows `--json` in usage |
| Justfile syntax | Verified | `just --list` shows correct recipe signatures |
| JSON emission (success) | **Not tested** | Requires running server + Playwright against a real GLB — manual smoke test |
| JSON emission (error) | **Not tested** | Requires triggering a bake failure — manual smoke test |
| Batch JSON | **Not tested** | Requires multiple GLBs in inbox — manual smoke test |

### Test Gaps

The core JSON emission paths are **not unit tested**. They require a running server, Playwright, and real GLB files. This is consistent with T-013-01 and T-013-03 which also deferred end-to-end testing to manual smoke tests.

Recommended smoke test:
```bash
# Single file
just bake inbox/dahlia_blush.glb --json 2>/dev/null | jq .

# Batch
just bake-all --json 2>/dev/null | jq -s .
```

## Open Concerns

1. **Justfile json parameter UX**: The `json=""` parameter means `just bake source --json` works because `--json` becomes the value of the `json` parameter. But `just bake --json source` would assign `--json` to `source` and `source` would be empty. The positional-then-flag order is natural but worth noting.

2. **Screenshot paths are absolute**: The `screenshot` field in error JSON contains absolute paths (e.g., `/Users/foo/repo/dist/bake-errors/...`). An agent on a different machine would need to interpret these relative to the repo. This matches the existing `pack` field behavior (also absolute).

3. **Step detection is best-effort**: Pipeline failures are mapped to step names by matching text in the failed stage's label. Unexpected error types (Playwright crash, network timeout) get `step: "unknown"`. This is documented in the agent workflow.

4. **batch-bake.ts removes its own `captureErrorScreenshot` call**: Since `bakeOne()` now captures the screenshot internally and attaches it to `BakeError`, the batch loop no longer needs to call `captureErrorScreenshot()` separately. This is a behavioral improvement — screenshots are now always captured exactly once.
