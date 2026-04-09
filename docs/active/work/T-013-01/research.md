# T-013-01 Research: Playwright Headless Bake

## Objective

Map the codebase surface relevant to automating the browser-based bake pipeline via Playwright. Understand every API endpoint, UI element, and data flow the script must interact with.

## Server Architecture

The Go server (`main.go`) serves a static HTML UI and exposes a REST API. Default port is 8787, configurable via `--port` flag. Working directory defaults to `~/.glb-optimizer` with subdirectories: `originals/`, `outputs/`, `settings/`, `tuning/`, `profiles/`, `accepted/`.

Pack output goes to `dist/plants/` (constant `DistPlantsDir` in `pack_writer.go` L12), resolved relative to the working directory.

## API Endpoints Relevant to Bake Pipeline

### Upload
- `POST /api/upload` — multipart/form-data, field name `files`, accepts `.glb` only, 100 MB max
- Returns array of `FileRecord` JSON objects with generated UUIDs
- Auto-classifies shape on upload

### File Listing / Polling
- `GET /api/files` — returns all `FileRecord` objects
- Key fields for bake polling:
  - `id` (UUID string)
  - `filename` (original name)
  - `status` ("pending" | "processing" | "done" | "error")
  - `has_billboard` (bool) — multi-angle camera-facing impostor rendered
  - `has_billboard_tilted` (bool) — elevated-camera billboard rendered
  - `has_volumetric` (bool) — horizontal dome slice stack rendered

### Optimization
- `POST /api/process/:id` — runs gltfpack on single file
- `POST /api/classify/:id` — classify shape category
- `POST /api/generate-lods/:id` — generate LOD chain

### Bake Intermediates (client-side three.js renders, uploaded back)
- `POST /api/upload-billboard/:id` — binary GLB body
- `POST /api/upload-billboard-tilted/:id` — binary GLB body
- `POST /api/upload-volumetric/:id` — binary GLB body
- `POST /api/bake-complete/:id` — stamps `{outputsDir}/{id}_bake.json` with RFC3339 UTC timestamp, returns `{ status: "ok", bake_id: "..." }`

### Pack Build
- `POST /api/pack/:id` — assembles final pack from intermediates
- Response (200): `{ pack_path, size, species }`
- Error responses: 400 (missing intermediate), 404 (no file), 413 (>5 MiB), 500 (internal)

## Static UI Elements

### File: `static/index.html`

| Selector | Element | Purpose |
|----------|---------|---------|
| `#dropZone` | div | Drag-drop upload area |
| `#browseBtn` | button | Opens file picker |
| `#fileInput` | input[type=file] | Hidden, accept=".glb" |
| `#fileList` | div | Rendered file list |
| `#prepareForSceneBtn` | button | One-click full pipeline |
| `#generateProductionBtn` | button | "Build hybrid impostor" (3-phase bake) |
| `#buildPackBtn` | button | "Build Asset Pack" |
| `#prepareProgress` | div | Pipeline progress container |
| `#prepareStages` | ul | Stage status list |
| `#prepareError` | div | Error/result text |

### File: `static/app.js`

**Upload flow** (L1208-1216):
- `uploadFiles(fileObjects)` creates `FormData`, appends files, POSTs to `/api/upload`
- Drag-drop handler at L4837-4841 calls `uploadFiles`

**Full pipeline** — `prepareForScene()` (L2590-2698):
1. Optimize via `POST /api/process/:id` (skips if already done)
2. Classify via `POST /api/classify/:id` (skips if confidence > 0)
3. Generate LODs via `POST /api/generate-lods/:id`
4. Bake production via `generateProductionAsset()` (three impostor uploads)

**Bake sequence** — `generateProductionAsset()` (L2414-2479):
1. Render billboard from `BILLBOARD_ANGLES` → `POST /api/upload-billboard/:id`
2. Render tilted billboard from `TILTED_BILLBOARD_ANGLES` → `POST /api/upload-billboard-tilted/:id`
3. Render volumetric dome slices → `POST /api/upload-volumetric/:id`
4. Stamp completion → `POST /api/bake-complete/:id`

**Pack build** — `buildAssetPack()` (L2494-2545):
- `POST /api/pack/:id`
- On success: displays species + size in `#prepareError`
- Pack button enabled only when all three intermediates exist

**Progress display**: stage glyphs `[*]` running, `[ok]` done, `[err]` failed — rendered in `#prepareStages` children.

## Existing Node.js Tooling

### `scripts/package.json`
- Name: `glb-optimizer-scripts`, private, ESM (`"type": "module"`)
- Dependencies: `@gltf-transform/core@4.1.1`
- Scripts: `verify-pack` → `node verify-pack.mjs`
- Lock file exists: `scripts/package-lock.json`

### Existing scripts
- `verify-pack.mjs` — validates pack schema + scene graph
- `build-fixtures.mjs` — build test fixtures
- `test-verify-pack.sh` — shell test wrapper

No Playwright or TypeScript tooling currently installed.

## Justfile Recipes

| Recipe | Purpose |
|--------|---------|
| `run` / `serve [port]` | Build + run server |
| `down` | Kill server |
| `pack [id]` | CLI pack single asset |
| `pack-all` | CLI pack all baked assets |
| `verify-pack [arg]` | Validate pack output |
| `clean-packs` | Wipe dist/plants/ |

## Inbox Convention

`inbox/` at repo root is the standard drop location for source GLBs awaiting bake. Currently contains:
- `dahlia_blush.glb` (~28 MB, renamed from dahlia_magenta)

Not used by the web UI — serves as the staging area for CLI/automated workflows.

## Key Constraints and Observations

1. **Bake is client-side**: The three.js rendering happens in the browser, not the server. The browser renders impostor textures and uploads the GLB results back to the server. This means Playwright must actually run the full browser pipeline — there's no server-side shortcut.

2. **No existing bake API**: There's no single "bake this file" endpoint. The script must orchestrate the same UI flow a human would: upload → optimize → classify → LODs → production bake → pack.

3. **`#prepareForSceneBtn` is the one-click path**: This button orchestrates the full pipeline. The Playwright script could click this single button and monitor stage progress rather than driving each step individually.

4. **Completion detection**: The `#prepareStages` element shows per-stage status. After all stages complete, the pack button becomes enabled. The `/api/files` endpoint provides the authoritative state.

5. **File selection**: After upload, the script must click on the file in `#fileList` to select it before any bake buttons become active. The file list renders dynamically.

6. **Pack output path**: `~/.glb-optimizer/dist/plants/{species}.glb` — the species is derived from the original filename.

7. **Timeout budget**: Bake takes 30-120 seconds per asset. The 5-minute timeout in the ticket is generous but appropriate.

8. **TypeScript execution**: The ticket specifies `npx tsx` for running `.ts` files. This requires adding `tsx` as a devDependency or relying on npx auto-install.

9. **Playwright installation**: `npx playwright install chromium` must be run once to download the browser binary. This is a one-time setup step.

10. **The script does NOT start the server**: Per ticket scope, the Go server must be running separately. The script should verify connectivity first.
