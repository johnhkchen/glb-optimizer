# T-013-04 Research: Agent Bake-Pack Pipeline

## Scope

Make `just bake` and `just bake-all` agent-friendly: structured JSON output via `--json` flag, agent documentation, and non-interactive guarantees.

## Current Architecture

### Execution Flow

```
just bake <file>
  → bash: start server if needed (go run . --port 8787)
  → cd scripts && npx tsx headless-bake.ts "../<file>" --headless --port 8787
    → Playwright: upload GLB, click bake, wait for pack
    → returns BakeResult { filename, species, size, packPath }
  → bash: cleanup server

just bake-all [inbox]
  → bash: start server if needed
  → cd scripts && npx tsx batch-bake.ts "../<inbox>" --headless --port 8787
    → Playwright: single browser, loop over .glb files
    → per file: bakeOne() → FileResult { filename, species, size, status, error? }
    → prints summary table, moves done files to inbox/done/
  → bash: cleanup server
```

### Key Files

| File | Role | Lines |
|------|------|-------|
| `justfile` L54-91 | `bake` recipe — server lifecycle + headless-bake.ts | bash |
| `justfile` L97-133 | `bake-all` recipe — server lifecycle + batch-bake.ts | bash |
| `scripts/headless-bake.ts` | Single-file bake via Playwright UI | 260 |
| `scripts/batch-bake.ts` | Batch orchestrator, imports bakeOne from headless-bake | 177 |
| `pack_inspect.go` | Existing `--json` pattern to follow | 418 |
| `bake_status.go` | CLI subcommand for intermediate completeness | 144 |
| `pack_cmd.go` | pack / pack-all CLI subcommands | 264 |

### Output Interfaces (TypeScript)

**headless-bake.ts** — `BakeResult` (L86-91):
```typescript
export interface BakeResult {
  filename: string;  // source GLB name
  species: string;   // resolved species id
  size: number;      // pack size in bytes
  packPath: string;  // path to output pack
}
```

**batch-bake.ts** — `FileResult` (L56-62):
```typescript
interface FileResult {
  filename: string;
  species: string;
  size: number;
  status: "ok" | "error";
  error?: string;
}
```

### Current Output Format

**headless-bake.ts main()** (L220-250):
- Logs debug lines via `log()` to stdout with `[headless-bake]` prefix
- On error: prints `[headless-bake] ERROR: <msg>` to stderr, captures screenshot, exits 1
- On success: just logs "done" — no structured result printed

**batch-bake.ts main()** (L64-174):
- Logs progress lines with `[batch-bake]` prefix
- Prints summary table with columns: FILENAME, SPECIES, SIZE, STATUS
- Already has `FileResult` with status/error fields — close to ticket's JSON spec
- Exits 1 if any file failed

### Existing JSON Pattern (pack-inspect)

`pack_inspect.go` L364-417 establishes the convention:
- `--json` boolean flag on the flag set
- Three output modes: `renderHuman()`, `renderJSON()`, `renderQuiet()`
- JSON uses `json.NewEncoder(w)` with 2-space indent
- All structs have `json:"field_name"` tags

### Where --json Flag Gets Threaded

The justfile recipes pass CLI args through to the TypeScript scripts:
```
cd scripts && npx tsx headless-bake.ts "../{{source}}" --headless --port "$PORT"
cd scripts && npx tsx batch-bake.ts "../{{inbox}}" --headless --port "$PORT"
```

The justfile `bake` recipe doesn't support extra args today. Options:
1. Add a `json` parameter to the justfile recipe, thread `--json` to the TS script
2. Use an env var (e.g., `JSON=1 just bake foo.glb`)

### Non-Interactive Guarantees

Already met:
- `--headless` flag launches Playwright without visible browser
- Server auto-starts and auto-stops via justfile trap
- Species derived via resolver chain (no manual _meta.json)
- File paths use `~/.glb-optimizer/` working directory by default

Remaining gaps:
- No `--json` flag on bake/bake-all
- Debug log lines go to stdout, would pollute JSON output
- No agent documentation exists

### Error Screenshot Path

`captureErrorScreenshot()` (headless-bake.ts L66-78) saves to `dist/bake-errors/{filename}_error.png` and returns the path. This path should appear in JSON error output.

### Agent Documentation Gap

No `docs/agent-pack-workflow.md` exists. The ticket requires:
- Prerequisites section
- Worked example with inbox/dahlia_blush.glb
- Troubleshooting section
- Verification via `just verify-pack`

### Species Resolver

Species are resolved automatically by the server's resolver chain. The headless bake flow reads the species from the UI after bake completion. No manual mapping needed for single files. The batch flow inherits this.

## Constraints

- The TypeScript scripts own all output formatting — no Go changes needed for `--json`
- Debug logs must go to stderr when `--json` is active (stdout reserved for JSON)
- The justfile recipes need a way to pass `--json` through
- batch-bake.ts already has `FileResult` with the right shape; headless-bake.ts needs a result emitter
- pack-inspect's `--json` flag convention should be followed for consistency
