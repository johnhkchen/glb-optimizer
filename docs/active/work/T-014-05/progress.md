# T-014-05 Progress: UI Button Calls API

## Completed

### Step 1: Replace `generateProductionAsset` function body
- Replaced the three client-side render+upload sequences with a single `POST /api/build-production/{id}` call
- Button text changed from "Building..." to "Rendering via Blender..."
- Single `onSubstage('rendering')` call replaces three per-pass calls
- Response JSON flags drive `store_update` (billboard, tilted, volumetric)
- `POST /api/bake-complete/{id}` preserved after successful render
- All post-render state management preserved: `refreshFiles()`, `updatePreviewButtons()`, `setBakeStale(false)`
- Analytics event preserved in finally block
- Error handling: parses server JSON error, falls back to status code

### Step 2: Verify prepare-for-scene integration
- Confirmed: prepare flow at L2660 calls `generateProductionAsset(id, (substage) => {...})`
- With new implementation, `substage` = `'rendering'` → prepare UI shows "rendering bake..."
- Post-condition check at L2670 (`has_billboard && has_billboard_tilted && has_volumetric`) still works because `refreshFiles()` re-fetches from server after the API call updates the FileStore

### Step 3: Verify post-conditions
- `setBakeStale(false)` preserved
- Analytics event shape unchanged: `{trigger: 'production', success}`
- Button re-enable in finally path unchanged

## Deviations from Plan

None. Implementation followed the plan exactly.

## Remaining

None. Implementation complete.
