# T-014-03 Design: API Build-Production Endpoint

## Decision 1: Handler Placement

**Decision**: New handler `handleBuildProduction` in `handlers.go`, following the existing pattern.

**Rationale**: All API handlers live in handlers.go. The function follows the same factory pattern: `func handleBuildProduction(...) http.HandlerFunc`. No new file needed — the handler is ~100 lines, consistent with peers like `handleGenerateBlenderLODs` and `handleBuildPack`.

## Decision 2: Config Passing — JSON Temp File

**Decision**: Write a temp JSON config file and pass it via `--config <path>`. Delete the temp file after Blender completes.

**Rationale**: The script accepts 15+ parameters. Passing them all as CLI args is error-prone and hard to debug. A JSON config file:
- Is self-documenting (can be inspected if Blender crashes)
- Matches the script's primary interface (`--config`)
- Avoids shell escaping issues
- Can be logged for debugging

The config struct will be a simple `map[string]interface{}` marshalled to JSON. The temp file lives in `outputsDir` (guaranteed writable) with a predictable name like `{id}_render_config.json`, deleted on completion.

## Decision 3: Blender Invocation — CommandContext with Timeout

**Decision**: Use `exec.CommandContext` with a 5-minute (300s) `context.WithTimeout` for process lifecycle management.

**Rationale**: Unlike `RunBlenderLOD` (which has no timeout), production renders can take minutes for complex assets. `CommandContext` automatically sends SIGKILL on timeout, preventing runaway processes. The context-based approach integrates cleanly with Go's cancellation model.

## Decision 4: Concurrency — Package-Level Mutex

**Decision**: A `sync.Mutex` declared at package level in handlers.go, acquired at the start of the Blender invocation and released when it finishes.

**Rationale**: Only one Blender render at a time (ticket spec). A package-level mutex is simple and sufficient for v1. It serializes all build-production requests — concurrent callers block. This prevents OOM from parallel Blender processes. The mutex is declared in handlers.go alongside the handler, not in a separate file.

## Decision 5: Script Path Discovery

**Decision**: Pass `scripts/render_production.py` as a path relative to the executable's directory, resolved at startup in main.go.

**Rationale**: Unlike remesh_lod.py (which is `//go:embed`-ed), render_production.py is too large and complex to embed — it imports Blender-internal modules. The script lives in the repo's `scripts/` directory. At startup, main.go resolves the path relative to the executable (or working directory) and passes it to the handler factory. If the script is missing, the endpoint returns 500 with a clear message, similar to the "Blender not found" behavior.

## Decision 6: Category Parameter Resolution

**Decision**: Category flows through three levels with fallback:
1. Query param `?category=X` (explicit override)
2. Asset's `ShapeCategory` from saved settings
3. Default `"unknown"` (falls through to strategy table)

The resolved category feeds into `getStrategyForCategory()` to get the strategy, which determines `--skip-volumetric` for hard-surface and the slice parameters.

## Decision 7: Response Shape

**Decision**: Synchronous JSON response matching the ticket spec:
```json
{
  "id": "abc123",
  "billboard": true,
  "tilted": true,
  "volumetric": true,
  "duration_ms": 12345
}
```

**Rationale**: v1 is synchronous. The response confirms which intermediates were produced. `volumetric` will be `false` for hard-surface assets (correctly skipped). `duration_ms` aids performance profiling.

## Decision 8: Error Codes

Five error cases as specified in the ticket:
| Condition | Status | Message |
|-----------|--------|---------|
| Blender not found | 500 | `"blender not installed"` |
| Asset not optimized | 400 | `"asset must be optimized first (status=done)"` |
| Blender non-zero exit | 500 | Blender's stderr (truncated to 2KB) |
| Timeout after 300s | 500 | `"render timed out after 300s"` |
| Missing intermediates | 500 | `"blender completed but intermediates missing: <list>"` |

## Decision 9: hard-surface Handling

**Decision**: For hard-surface category (`SliceCount=0`, `SliceAxis="n/a"`), pass `--skip-volumetric` to the script. Billboards and tilted billboards are still rendered. The response will have `volumetric: false`.

## Rejected Alternatives

- **Async job queue**: Deferred per ticket — v1 is synchronous.
- **Embedding render_production.py**: Too large (~1000 lines), imports bpy — must run inside Blender's Python.
- **Separate blender_production.go file**: Handler is ~100 lines and follows the same pattern as all other handlers in handlers.go. No new file needed.
- **Per-request mutex in FileStore**: Overengineered — a simple package-level mutex is clearest.
