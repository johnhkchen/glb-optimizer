# T-013-01 Structure: Playwright Headless Bake

## Files Created

### `scripts/headless-bake.ts`

Main script file. Single-file design — no module splitting needed for a ~200-line CLI tool.

```
scripts/headless-bake.ts
├── imports (playwright, node:fs, node:path, node:url)
├── constants
│   ├── DEFAULT_PORT = 8787
│   ├── BAKE_TIMEOUT_MS = 300_000 (5 minutes)
│   ├── POLL_INTERVAL_MS = 2_000
│   └── LOG_PREFIX = "[headless-bake]"
├── log(msg: string) → void
│   └── Prefixed console.log
├── parseArgs(argv: string[]) → { glbPath, port, headless }
│   └── Minimal CLI arg parsing (no deps)
├── checkServer(baseUrl: string) → Promise<void>
│   └── GET /api/status, throw if unreachable
├── waitForFileInList(page, filename) → Promise<string>
│   └── Poll /api/files until filename appears, return id
├── waitForStageCompletion(page) → Promise<void>
│   └── Watch #prepareStages for all stages [ok] or any [err]
├── captureErrorScreenshot(page, filename) → Promise<string>
│   └── Screenshot → dist/bake-errors/{filename}-{timestamp}.png
├── main() → Promise<void>
│   ├── Parse args
│   ├── Validate glb file exists
│   ├── Check server reachable
│   ├── Launch browser (headed/headless)
│   ├── Navigate to baseUrl
│   ├── Upload via setInputFiles('#fileInput')
│   ├── Wait for file in list
│   ├── Click file card to select
│   ├── Click #prepareForSceneBtn
│   ├── Wait for stage completion (with timeout)
│   ├── Click #buildPackBtn
│   ├── Wait for pack result in #prepareError
│   ├── Extract species + size from result text
│   ├── Verify pack file on disk
│   ├── Print summary (path, size, species)
│   └── Close browser
└── main().catch() → process.exit(1)
```

### Interface / Type Notes

No exported types. Internal-only types:

```typescript
interface ParsedArgs {
  glbPath: string;
  port: number;
  headless: boolean;
}
```

## Files Modified

### `scripts/package.json`

Add dependencies:
```json
{
  "devDependencies": {
    "playwright": "^1.x",
    "tsx": "^4.x"
  }
}
```

Note: `playwright` rather than `@playwright/test` — we need the library API, not the test runner.

### `justfile`

Add recipe:
```just
bake source:
    cd scripts && npx tsx headless-bake.ts ../{{source}} --headless
```

This wraps the script for convenient use: `just bake inbox/dahlia_blush.glb`

### `.gitignore`

Add entry:
```
dist/bake-errors/
```

## Files NOT Modified

- `static/app.js` — no UI changes
- `static/index.html` — no UI changes
- `handlers.go` — no server changes
- `main.go` — no server changes

## Directory Structure After Implementation

```
scripts/
├── headless-bake.ts          ← NEW
├── package.json              ← MODIFIED (add playwright, tsx)
├── package-lock.json         ← REGENERATED
├── verify-pack.mjs           ← unchanged
├── build-fixtures.mjs        ← unchanged
└── test-verify-pack.sh       ← unchanged

dist/
└── bake-errors/              ← NEW (created on first error, gitignored)
    └── {filename}-{timestamp}.png

justfile                      ← MODIFIED (add bake recipe)
.gitignore                    ← MODIFIED (add dist/bake-errors/)
```

## Module Boundaries

The script is intentionally self-contained in a single file. Rationale:
- It's a CLI tool, not a library
- ~200 lines is manageable in one file
- No other scripts need to import from it
- Single-file makes `npx tsx scripts/headless-bake.ts` work without module resolution complexity

## Key Architectural Decisions

### Playwright Library vs Test Runner

Use `playwright` (the library) not `@playwright/test` (the test framework). We're writing a CLI automation tool, not a test suite. The library gives us `chromium.launch()` directly.

### Arg Parsing

Hand-rolled minimal parser. The script takes 1 positional arg and 2 optional flags. Adding a dependency like `yargs` or `commander` for this would be over-engineering.

### Error Screenshot Directory

`dist/bake-errors/` follows the existing `dist/` convention (the pack output is `dist/plants/`). Gitignored because screenshots are debugging artifacts.

### Working Directory Assumption

The script must be run from the repo root (or the `just bake` recipe handles the path). It resolves the GLB path relative to `process.cwd()`, not relative to the script file.

### Server Working Directory for Pack Verification

The pack output path returned by `/api/pack/:id` is absolute (e.g., `~/.glb-optimizer/dist/plants/dahlia_blush.glb`). The script uses the returned `pack_path` directly to verify the file exists, rather than guessing the path.

## Selector Stability

All selectors used are `#id`-based, which are stable:
- `#fileInput` — hidden file input
- `#fileList` — file list container
- `#prepareForSceneBtn` — full pipeline button
- `#prepareStages` — stage progress list
- `#prepareError` — result/error text
- `#buildPackBtn` — pack build button

No CSS class selectors or positional selectors are used, reducing fragility.
