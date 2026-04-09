# T-008-01 Structure

## Files touched

| File                                            | Action  | Reason                                                       |
|-------------------------------------------------|---------|--------------------------------------------------------------|
| `static/index.html`                             | modify  | Add `#prepareForSceneBtn`, `#prepareProgress` block          |
| `static/app.js`                                 | modify  | Add orchestrator + handlers + button-state wiring            |
| `static/style.css`                              | modify  | Style new primary toolbar button + progress list             |
| `analytics.go`                                  | modify  | Allow-list `prepare_for_scene` event type                    |
| `docs/knowledge/analytics-schema.md`            | modify  | Document the new event type and payload                      |

No new files. No new Go endpoints. No deleted files.

## `static/index.html`

Inside `.toolbar-actions` (currently lines 53–64), prepend:

```html
<button class="toolbar-btn toolbar-btn-primary" id="prepareForSceneBtn"
        disabled title="Run the full pipeline: optimize, classify, LOD,
        production asset">Prepare for scene</button>
```

Immediately after the closing `</div>` of `.toolbar-actions` (and before
`.stress-controls`), add:

```html
<div class="prepare-progress" id="prepareProgress" style="display:none">
    <ul class="prepare-stages" id="prepareStages"></ul>
    <div class="prepare-error" id="prepareError"></div>
    <button class="toolbar-btn" id="viewInSceneBtn" style="display:none">
        View in scene</button>
</div>
```

The progress block is hidden until the first click and stays visible
between clicks so the user can see the most recent run's outcome.

## `static/app.js`

### New constants (near the existing `const generate*Btn = ...` block, ~line 66)

```js
const prepareForSceneBtn = document.getElementById('prepareForSceneBtn');
const prepareProgress    = document.getElementById('prepareProgress');
const prepareStages      = document.getElementById('prepareStages');
const prepareError       = document.getElementById('prepareError');
const viewInSceneBtn     = document.getElementById('viewInSceneBtn');
```

### New section: `// ── Prepare for Scene ──` (placed just after `// ── Production Asset (Hybrid) ──`, ~line 2236)

Public surface:

```js
async function prepareForScene(id) { … }   // the orchestrator
function setPrepareStages(stages) { … }    // render the stage list
function markPrepareStage(idx, status, msg) { … } // status: 'pending'|'running'|'ok'|'error'
```

Internal helpers (closure-scoped, not exported):

- `runStageGltfpack(file)` — calls `processFile(id)` if `file.status !== 'done'`,
  re-reads the store, returns `{ ok, ranIt }`.
- `runStageClassify(file)` — calls `fetchClassification(id)` if
  `currentSettings.shape_confidence === 0`; updates `currentSettings`
  and `populateTuningUI()` on success.
- `runStageLods(file)` — calls `generateLODs(id)`, returns `{ ok }` based
  on the post-call file record's `lods` array.
- `runStageProduction(file)` — calls `generateProductionAsset(id)`,
  returns `{ ok }` based on `has_billboard && has_volumetric`.

The orchestrator loops `[gltfpack, classify, lods, production]`, calls
each adapter, marks the stage in the UI, accumulates `stages_run`, and
breaks on the first `ok === false`. Then emits one `prepare_for_scene`
event and shows the `View in scene` button on success.

### Wiring (event-listener block, ~line 4108)

Add after the `generateProductionBtn` listener:

```js
prepareForSceneBtn.addEventListener('click', () => {
    if (selectedFileId) prepareForScene(selectedFileId);
});

viewInSceneBtn.addEventListener('click', () => {
    stressBtn.click();
});
```

### `updatePreviewButtons()` (~line 3981)

Add one line after `generateProductionBtn.disabled = ...`:

```js
prepareForSceneBtn.disabled = !file || !currentModel;
```

## `static/style.css`

Two new rule blocks, appended near the existing `.toolbar-btn` rules
(~line 386):

```css
.toolbar-btn-primary {
    background: var(--accent);
    color: #fff;
    border-color: var(--accent);
    font-weight: 600;
}
.toolbar-btn-primary:hover:not(:disabled) {
    background: var(--accent-hover);
    color: #fff;
}
.toolbar-btn-primary:disabled {
    background: #333;
    color: #666;
    border-color: #333;
}

.prepare-progress {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 4px 8px;
    border-left: 1px solid var(--panel-border);
    color: var(--text-muted);
    font-size: 11px;
}
.prepare-stages { list-style: none; display: flex; gap: 6px; }
.prepare-stages li { white-space: nowrap; }
.prepare-stages li.running { color: var(--accent); }
.prepare-stages li.ok      { color: var(--success); }
.prepare-stages li.error   { color: var(--error); }
.prepare-error { color: var(--error); font-size: 11px; }
```

## `analytics.go`

In the `validEventTypes` map (line 25), add:

```go
"prepare_for_scene": true, // T-008-01
```

No struct changes — the payload is opaque per the existing envelope rules.

## `docs/knowledge/analytics-schema.md`

Add a new `### prepare_for_scene` section between `scene_template_selected`
and `strategy_selected` (preserving alphabetical-ish ordering used in the
existing doc would put it later, but the existing order is roughly
chronological by ticket — so insert after `scene_template_selected`).
Document:

- when it fires (one click of the new button)
- payload fields: `stages_run` (string array), `total_duration_ms` (int),
  `success` (bool), `failed_stage` (string, optional), `error` (string,
  optional)
- additive guarantee per the doc's existing versioning policy

## Ordering of changes

The order matters only for the analytics piece — if we ship the JS event
emit before allow-listing the type server-side, the very first Prepare
click would log a 400 to the console. So:

1. `analytics.go` allow-list + `analytics-schema.md` doc.
2. `static/index.html` markup (button + progress block).
3. `static/style.css` rules.
4. `static/app.js` orchestrator + handlers + `updatePreviewButtons` line.

Each step compiles/runs independently. The orchestrator is the only step
where the user-visible behavior changes.

## Public interfaces and module boundaries

No new exports beyond `window.prepareForScene = prepareForScene` (added
for parity with the existing `window._processFile`, `window.testLighting`,
etc., so the function is callable from devtools during manual verification).

Nothing else in the app.js global namespace shifts. The new functions
share the existing module-private state (`currentSettings`, `files`,
`selectedFileId`, `currentModel`) read-only — they only mutate the new
DOM elements and the analytics stream.
