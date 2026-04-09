# T-013-03 Design: batch-bake

## Architecture

Two-layer design matching the existing `just bake` pattern:

1. **justfile `bake-all` recipe** — Server lifecycle (start/stop), delegates to TypeScript
2. **scripts/batch-bake.ts** — Batch orchestrator: one browser, N files, summary table, done-move

## Component Design

### 1. Refactored headless-bake.ts

Extract the single-file bake logic into an exported function:

```typescript
export interface BakeResult {
  filename: string;
  species: string;
  size: number;
  packPath: string;
}

export async function bakeOne(
  page: Page,
  glbPath: string,
  baseUrl: string
): Promise<BakeResult>
```

- Navigates to `baseUrl` (resets UI state)
- Performs the full upload → select → prepare → pack flow
- Returns structured result on success, throws on failure
- The existing `main()` function calls `bakeOne()` internally — CLI behavior unchanged

### 2. scripts/batch-bake.ts

New file, ~120 lines:

```
CLI: npx tsx batch-bake.ts [inbox-dir] [--port N] [--headless]
```

Flow:
1. Parse args: inbox directory (default `../inbox`), port, headless flag
2. Scan inbox for `*.glb` files (exclude `done/` subdirectory)
3. If no files found, log and exit 0
4. Launch browser (single instance)
5. For each `.glb`:
   a. Call `bakeOne(page, glbPath, baseUrl)`
   b. On success: record result, move source to `inbox/done/`
   c. On error: record failure (with error message), continue
6. Close browser
7. Print summary table: FILENAME → SPECIES → SIZE → STATUS
8. Exit with code = number of failures (capped at 1 for simplicity, or just non-zero)

### 3. justfile `bake-all` recipe

Bash shebang recipe (same pattern as `just bake`):

```
bake-all inbox="inbox":
    #!/usr/bin/env bash
    set -euo pipefail
    # ... server lifecycle (identical to just bake) ...
    cd scripts && npx tsx batch-bake.ts "../{{inbox}}" --headless --port "$PORT"
```

The server lifecycle block is duplicated from `just bake` — extracting it would require a separate shell script, which adds complexity for little gain given there are only two callers.

## Key Decisions

### D1: Navigate per asset (not create new page)

Reuse the same `page` object but call `page.goto(baseUrl)` at the start of each `bakeOne()` call. This resets all DOM state without the overhead of creating a new browser context. If we encounter stale-state bugs, we can upgrade to `browser.newPage()` + `page.close()` per asset.

### D2: Summary table in TypeScript

The summary table is printed by batch-bake.ts after all assets complete. This keeps the table format close to the data source and avoids parsing stdout from the subprocess.

### D3: inbox/done/ created on first success

`mkdirSync(doneDir, { recursive: true })` called before the first `rename()`. No-op if already exists.

### D4: Exit code semantics

Exit 0 if all succeed, exit 1 if any failed. The summary table shows per-file status so the operator can identify failures.

### D5: File glob ignores done/ subdirectory

`readdirSync(inboxDir)` filtered to `.glb` extension only, skipping directories. Since `done/` is a directory, it's naturally excluded.

## Error Handling

- Per-file try/catch: errors are captured, logged, and recorded; processing continues
- Error screenshots: `captureErrorScreenshot` still fires per failed file (from within `bakeOne`)
- Server crash: if the server dies mid-batch, subsequent `bakeOne` calls will fail on upload/navigate; the summary table will show all remaining as failed
- Browser crash: entire batch aborts (no recovery) — this is acceptable given the sequential nature

## Data Flow

```
inbox/
  ├── dahlia_blush.glb    ─── bakeOne() ──► outputs/*, dist/plants/dahlia_blush.glb
  ├── fern_boston.glb       ─── bakeOne() ──► outputs/*, dist/plants/fern_boston.glb
  └── done/
       └── (moved after success)
```
