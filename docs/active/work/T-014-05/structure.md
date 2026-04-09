# T-014-05 Structure: UI Button Calls API

## Files Modified

### `static/app.js`

One function modified, no new functions, no deleted functions.

#### `generateProductionAsset(id, onSubstage)` — L2414-L2478

**Before**: 60-line function with three render+upload sequences, bake-complete call, and state management.

**After**: ~40-line function with:

```
async function generateProductionAsset(id, onSubstage = () => {}) {
    if (!currentModel || !threeReady) return;

    // Button state management — unchanged
    generateProductionBtn.textContent = 'Rendering via Blender…';
    generateProductionBtn.classList.add('generating');
    generateProductionBtn.disabled = true;

    // Single substage notification for prepare flow
    onSubstage('rendering');

    let success = false;
    try {
        // Build category query param from current settings
        const cat = currentSettings && currentSettings.shape_category;
        const qs = cat ? `?category=${encodeURIComponent(cat)}` : '';

        // Single API call replaces three render+upload sequences
        const resp = await fetch(`/api/build-production/${id}${qs}`, {
            method: 'POST',
        });

        if (!resp.ok) {
            const body = await resp.json().catch(() => ({}));
            throw new Error(body.error || `server error ${resp.status}`);
        }

        const result = await resp.json();

        // Update local store from server response flags
        store_update(id, f => {
            f.has_billboard = result.billboard;
            f.has_billboard_tilted = result.tilted;
            f.has_volumetric = result.volumetric;
        });

        // Stamp bake metadata (server doesn't do this)
        await fetch(`/api/bake-complete/${id}`, { method: 'POST' });

        await refreshFiles();
        updatePreviewButtons();
        success = true;
        setBakeStale(false);
    } catch (err) {
        console.error('Production asset generation failed:', err);
    } finally {
        logEvent('regenerate', { trigger: 'production', success }, id);
    }

    // Restore button — unchanged
    generateProductionBtn.textContent = 'Build hybrid impostor';
    generateProductionBtn.classList.remove('generating');
    generateProductionBtn.disabled = false;
}
```

### Key Structural Decisions

1. **Button text**: Changes from `'Building…'` to `'Rendering via Blender…'` to communicate that the server (Blender) is doing the work, not the browser.

2. **`onSubstage` call**: Single `onSubstage('rendering')` at the start. The prepare flow at L2660 will show "rendering bake..." instead of per-pass labels. Acceptable for v1.

3. **Error handling**: `resp.json().catch(() => ({}))` handles non-JSON error bodies gracefully. The error is thrown and caught by the existing catch block which logs to console.

4. **`store_update` batching**: All three flags updated in one call instead of three separate calls. The flags come from the server response JSON.

5. **`bake-complete` remains client-side**: Single `POST /api/bake-complete/{id}` call preserved after successful render, same as before.

## Files NOT Modified

| File | Why |
|------|-----|
| `static/index.html` | Button element unchanged |
| `handlers.go` | Server endpoint already implemented (T-014-03) |
| `static/style.css` | `.generating` class already exists and works |
| Other render functions | Explicitly kept per ticket |

## No New Files

No new files created. This is a function body replacement in an existing file.

## Module Boundaries

- The change is entirely within `static/app.js`.
- No new imports, no new globals, no new DOM elements.
- The function signature is unchanged — all callers continue to work.
