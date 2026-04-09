# T-008-01 Progress

All four implementation steps from `plan.md` complete in a single pass.

## Step 1 — Allow-list `prepare_for_scene` analytics event ✓

- `analytics.go`: added `"prepare_for_scene": true, // T-008-01` to
  `validEventTypes` (line 38-ish).
- `docs/knowledge/analytics-schema.md`: added a `### prepare_for_scene`
  section before the `strategy_selected` section. Documents
  `stages_run`, `total_duration_ms`, `success`, optional `failed_stage`,
  optional `error`. Notes that per-stage `regenerate` events still fire
  in addition to this summary event.
- `go build ./...` and `go test ./...` both pass.

## Step 2 — Markup ✓

- `static/index.html`: prepended `#prepareForSceneBtn` to
  `.toolbar-actions` with class `toolbar-btn toolbar-btn-primary`.
- Inserted `#prepareProgress` block (stage list `<ul>`, error `<div>`,
  `#viewInSceneBtn`) immediately after `.toolbar-actions`.

## Step 3 — Styles ✓

- `static/style.css`: added `.toolbar-btn-primary`, `.prepare-progress`,
  `.prepare-stages`, `.prepare-stages li` status variants, and
  `.prepare-error` rules.

## Step 4 — Orchestrator + wiring ✓

- `static/app.js`:
  - Added the five new `getElementById` constants near the existing
    `generate*Btn` block.
  - Added the `// ── Prepare for Scene (T-008-01) ──` section with
    `PREPARE_STAGES`, `setPrepareStages`, `markPrepareStage`,
    `prepareForScene`, and `window.prepareForScene = prepareForScene`.
  - Added `prepareForSceneBtn.disabled = !file || !currentModel;` to
    `updatePreviewButtons`.
  - Wired the `prepareForSceneBtn` click and `viewInSceneBtn` click
    listeners next to the existing technique-button listeners.
- `node --check static/app.js` passes (no syntax errors).
- `go build ./... && go test ./...` still passes — Go side untouched
  apart from the analytics allow-list addition in step 1.

## Deviations from plan

None of substance.

- The plan suggested `setPrepareStages` and `markPrepareStage` accept an
  index; the implementation accepts the stage `id` instead because the
  orchestrator already knows the id and looking up by id is more robust
  if PREPARE_STAGES is reordered.
- The plan listed four `runStage*` adapters; the orchestrator inlines
  the four blocks instead because each block is short and extracting
  them would have created four functions used exactly once each.
  Matches CLAUDE.md "don't create helpers ... for one-time operations".

## Manual verification checklist (per plan.md step 4)

The repo has no JS test harness, so verification is manual. To run:

1. `go run .` → open `http://localhost:8787`.
2. Drop a fresh `.glb` file (e.g. a rose), wait for it to appear.
3. Click the file. Confirm `Prepare for scene` becomes enabled once
   the model loads (it gates on `currentModel`).
4. Click `Prepare for scene`. Watch the stage list cycle:
   `[•] Optimize` → `[✓] Optimize` → `[•] Classify` → `[✓] Classify` →
   `[•] LOD` → `[✓] LOD` → `[•] Production asset` → `[✓] Production asset`.
5. Confirm `View in scene` appears.
6. Click `View in scene` — confirm the stress-test scene runs with the
   asset's current scene template (it just delegates to the existing
   `#stressBtn`).
7. `tail -1 ~/.glb-optimizer/tuning/<latest>.jsonl` — confirm the last
   event is `prepare_for_scene` with `payload.success === true` and
   `stages_run` listing the stages that actually ran.
8. **Failure path:** temporarily edit `prepareForScene` to throw inside
   the LOD stage, click Prepare again. Confirm the LOD row shows `[✗]`
   with the error, the production stage does NOT run, the emitted
   event has `success: false`, `failed_stage: "lods"`, and `error`
   populated. Revert the synthetic throw before committing.
9. **Regression check:** with the file now fully prepared, click each
   of the original technique buttons (`LODs (gltfpack)`, `Billboard`,
   `Volumetric`, `Vol LODs`, `Production Asset`). Confirm they each
   still run and update the preview as before — the new orchestrator
   only *calls* them, never mutates their internals.

## Files modified

- `analytics.go`
- `docs/knowledge/analytics-schema.md`
- `static/index.html`
- `static/style.css`
- `static/app.js`

No new files. No deleted files. No new Go endpoints.
