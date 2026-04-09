# T-013-01 Design: Playwright Headless Bake

## Design Question

How should the Playwright script drive the bake pipeline? Two fundamental approaches exist: drive the UI buttons, or bypass the UI and call APIs directly.

## Option A: Drive the Full UI via `#prepareForSceneBtn`

Click the one-click "Prepare for Scene" button, which orchestrates optimize → classify → LODs → production bake. Then click `#buildPackBtn` for the pack step.

**Pros:**
- Exercises the exact same code path a human uses — catches UI regressions
- Single button click triggers the 4-stage pipeline with built-in progress tracking
- `#prepareStages` provides visual progress indication
- No need to understand internal API sequencing

**Cons:**
- Must select the file in the file list first (click the rendered file card)
- Must wait for dynamic UI updates (file list render, button enable states)
- More fragile to CSS/layout changes
- Harder to report which specific sub-step failed
- The "Prepare for Scene" flow includes optimize + classify + LODs which may not be needed if the goal is just bake + pack

**Assessment:** This is the cleanest approach and matches the ticket's intent ("automates exactly what a human does"). The extra steps (optimize, classify, LODs) are prerequisites for baking anyway, so including them is correct. The fragility concern is mitigated by using semantic selectors (`#id` selectors) rather than CSS classes.

## Option B: Hybrid — Upload via API, Bake via UI

Upload via `POST /api/upload` (skip the drag-drop), then click buttons for the bake steps.

**Pros:**
- Reliable upload (no drag-drop simulation needed)
- Still tests the UI bake path

**Cons:**
- Loses the upload UI test coverage
- Mixed paradigm is harder to maintain

**Assessment:** Reasonable but the mixing of API calls and UI interaction creates a confusing script. The upload via drag-drop can be reliably simulated with Playwright's `setInputFiles`.

## Option C: Pure API Orchestration

Skip the UI entirely. POST to each endpoint in sequence, poll `/api/files` between steps.

**Pros:**
- Most reliable — no DOM dependencies
- Fastest execution (no rendering overhead)

**Cons:**
- **Cannot work.** The bake step (generating billboard/tilted/volumetric intermediates) happens client-side in three.js. The browser must render the 3D scene and capture textures. There is no server-side bake endpoint. The `/api/upload-billboard/:id` etc. endpoints receive the already-rendered GLBs from the browser.

**Assessment:** Eliminated. The core bake step requires a real browser.

## Decision: Option A — Full UI Drive via `#prepareForSceneBtn`

The script will:
1. Upload via `page.setInputFiles('#fileInput', filePath)` (reliable, no drag-drop simulation)
2. Wait for file to appear in `#fileList`
3. Click the file card to select it
4. Click `#prepareForSceneBtn` to run the full pipeline
5. Monitor `#prepareStages` for stage completion/failure
6. After bake completes, click `#buildPackBtn`
7. Wait for pack success message in `#prepareError`
8. Verify pack file exists on disk

### Rationale
- Matches the ticket's stated intent: "automates exactly what a human does"
- Uses the battle-tested `prepareForScene()` orchestration
- `#prepareForSceneBtn` handles optimize + classify + LODs + bake — all prerequisites
- Upload via `setInputFiles` on the hidden `#fileInput` is the standard Playwright pattern — avoids brittle drag-drop simulation while still testing the upload handler
- Stage monitoring via `#prepareStages` children gives per-step visibility

## Upload Strategy

Playwright's `page.setInputFiles()` can set files on a hidden input element. The `#fileInput` element (hidden, accept=".glb") is connected to the upload handler. This avoids needing to simulate drag-and-drop while still triggering the same `uploadFiles()` code path.

Alternatively, we could `POST /api/upload` directly via `page.request` — but using `setInputFiles` tests the actual UI upload path.

**Decision:** Use `setInputFiles` on `#fileInput` to trigger the upload.

## File Selection Strategy

After upload, the file appears in `#fileList`. Each file card is rendered dynamically. The script needs to click on the newly uploaded file to select it. Strategy:

1. After upload, poll `/api/files` to get the file's `id`
2. Wait for the file card element to appear in the DOM
3. Click it to select it

The file cards are rendered with the filename visible. We can use `page.getByText(filename)` or look for the file card containing the filename.

## Completion Detection Strategy

Two complementary signals:

1. **Stage progress** — `#prepareStages` children show `[ok]` or `[err]` per stage. When all 4 stages show `[ok]`, bake is complete.
2. **API polling** — `GET /api/files` returns `has_billboard`, `has_billboard_tilted`, `has_volumetric` flags.

**Decision:** Primary detection via watching the `#prepareStages` DOM for stage completion text. Fall back to `/api/files` polling for authoritative state. This way the script monitors what the user sees while also having a reliable backend signal.

## Error Handling Strategy

- **Server unreachable**: Attempt `fetch(baseUrl + '/api/status')` before launching Playwright. Fail fast with clear message.
- **Upload failure**: Check response status from the upload.
- **Bake stage failure**: Watch for `[err]` in stage progress. On failure, capture screenshot → `dist/bake-errors/{filename}-{timestamp}.png`.
- **Timeout**: 5-minute overall timeout. Use Playwright's built-in timeout on `waitForSelector` / `waitForFunction`.
- **Pack failure**: Check `#prepareError` text and `/api/pack` response.

## Progress Reporting

The script should print progress to stdout:
```
[headless-bake] uploading dahlia_blush.glb...
[headless-bake] file uploaded: id=abc123
[headless-bake] starting pipeline...
[headless-bake] stage 1/4: optimize... done
[headless-bake] stage 2/4: classify... done
[headless-bake] stage 3/4: lods... done
[headless-bake] stage 4/4: production bake... done
[headless-bake] building pack...
[headless-bake] pack built: dist/plants/dahlia_blush.glb (1.2 MB)
```

## Technology Choices

| Choice | Decision | Rationale |
|--------|----------|-----------|
| Language | TypeScript (.ts) | Ticket specifies `scripts/headless-bake.ts` |
| Runner | `npx tsx` | Ticket specifies; tsx runs TS directly without compile step |
| Browser | Chromium via Playwright | Standard, headless-capable |
| Package location | `scripts/package.json` | Existing Node.js tooling lives here |
| Headed by default | Yes | Ticket says "headed by default for debugging" |
| `--headless` flag | Opt-in headless | For CI/agent use |

## Rejected Alternatives

- **Puppeteer**: Playwright is specified in the ticket and has better API ergonomics.
- **Selenium**: Over-engineered for this use case.
- **Server-side bake endpoint**: Doesn't exist; bake is client-side three.js rendering.
- **Direct API orchestration**: Impossible — bake requires browser rendering.
- **Starting the Go server from the script**: Out of scope per ticket.
