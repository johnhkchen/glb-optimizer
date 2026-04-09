# T-014-05 Design: UI Button Calls API

## Decision 1: How to Replace the Function Body

### Option A: Rewrite `generateProductionAsset` in place
Replace the body of the existing function with a `fetch()` to the server endpoint. The function signature stays the same (`id, onSubstage`), the button management bookkeeping stays, and the try/catch/finally structure stays.

### Option B: New function, old function renamed
Create `generateProductionAssetViaAPI()` and rename the old one to `generateProductionAssetClientSide()`. Wire the button and prepare flow to the new function.

### Decision: Option A

**Why:** The ticket says to replace the handler, not create a parallel code path. Option B adds complexity (two functions doing the same thing, routing logic to pick one) for no stated benefit. The old render functions remain individually callable — the ticket explicitly says to keep them — so there's no loss of debuggability. Option A is a clean replacement with zero new abstractions.

---

## Decision 2: Bake-Complete Stamping

### Option A: Client calls bake-complete after API success
Keep the `POST /api/bake-complete/{id}` call in the client after the build-production response succeeds.

### Option B: Server endpoint should be updated to stamp bake-complete
Add bake-complete stamping inside `handleBuildProduction`.

### Decision: Option A

**Why:** The server endpoint (T-014-03) is already implemented and potentially reviewed. Adding bake-complete logic to it changes a different ticket's scope. The client-side call is a single `fetch()` that already exists and works. This is the minimal-change approach. If someone later wants the server to own bake-complete, that's a separate ticket.

---

## Decision 3: Progress Text

### Option A: Static "Rendering via Blender..." text
Replace the current "Building..." with "Rendering via Blender..." for the entire duration of the server call. The `onSubstage` callback is called once at the start.

### Option B: Poll for progress
Add a polling mechanism to check render progress.

### Decision: Option A

**Why:** Ticket explicitly says "just a spinner for v1" and "out of scope: progress bar with per-stage updates." The `onSubstage` callback gets a single 'rendering' call so the prepare flow shows something useful.

---

## Decision 4: Error Handling

### Option A: Show raw server error
Parse the JSON error from the server and show it to the user.

### Option B: Generic "Server rendering failed" message
Wrap all errors in a user-friendly message.

### Decision: Option A with truncation

**Why:** The server already truncates Blender stderr to 2KB. Showing the actual error helps debugging. We'll log the full error to console and show a shorter message in the UI if the error text is very long, but for v1 the server error is descriptive enough.

---

## Decision 5: Response Handling

The server returns `{id, billboard, tilted, volumetric, duration_ms}`. The client should:

1. Use the boolean flags to update `store_update` (same as before, but driven by server response).
2. Call `refreshFiles()` to sync the full file list from server.
3. Call `updatePreviewButtons()` and `setBakeStale(false)`.
4. Call `POST /api/bake-complete/{id}` to stamp bake metadata.

This preserves the exact same post-render behavior as the client-side path.

---

## Decision 6: Category Parameter

The server endpoint accepts `?category=` query param but falls back to saved settings if omitted. The client has `currentSettings.shape_category`.

### Decision: Pass category explicitly

**Why:** The client already knows the category from the current UI state. Passing it explicitly ensures the server uses the same category the user sees, avoiding any race condition between classification and rendering.

---

## Rejected Alternatives

- **Feature flag to toggle client/server rendering**: Not needed. The ticket says to replace, not to toggle. Old render functions remain callable individually.
- **WebSocket for progress**: Overkill for v1. The server endpoint is synchronous (one HTTP call).
- **Removing the `onSubstage` parameter**: Keep it for API compatibility with the prepare flow caller. Just call it once.

## Summary of Changes

The `generateProductionAsset` function body is replaced to:
1. Call `onSubstage('rendering')` once
2. `POST /api/build-production/{id}?category={currentSettings.shape_category}`
3. Parse JSON response
4. Update store flags from response
5. Call `POST /api/bake-complete/{id}`
6. `refreshFiles()`, `updatePreviewButtons()`, `setBakeStale(false)`
7. Log analytics event

All bookkeeping (button state, try/catch/finally) stays identical.
