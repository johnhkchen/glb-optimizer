# T-008-01 Plan

Implementation steps in commit-sized chunks. Each step compiles/runs and
is independently revertible.

## Step 1 — Allow-list `prepare_for_scene` analytics event

Files: `analytics.go`, `docs/knowledge/analytics-schema.md`.

- Add `"prepare_for_scene": true,` to `validEventTypes` map.
- Add a new section to `analytics-schema.md` between
  `scene_template_selected` and `strategy_selected` describing payload
  fields (`stages_run`, `total_duration_ms`, `success`, optional
  `failed_stage`, optional `error`) and the trigger (one click of the
  new primary action).

Verification:

- `go build ./...` passes.
- `go test ./...` passes (no analytics tests assert the exact map; the
  envelope-validation test should still pass since we are only adding a
  key).
- Spot-check the doc markdown rendering in the editor.

Commit message: `T-008-01: allow prepare_for_scene analytics event`

## Step 2 — Markup for the new button and progress block

Files: `static/index.html`.

- Prepend `#prepareForSceneBtn` to `.toolbar-actions`.
- Insert `#prepareProgress` block (stage list + error + view-in-scene
  button) right after `.toolbar-actions`.

Verification:

- Reload the app, select an asset, confirm the new button renders
  disabled (no click handler yet → it stays disabled because there's
  no JS-side reference yet — it will throw a `ReferenceError` in step 4
  but step 3 lands first to keep visuals coherent).

Commit message: `T-008-01: add Prepare for scene button + progress markup`

## Step 3 — Styles

Files: `static/style.css`.

- Add `.toolbar-btn-primary` modifier (filled accent).
- Add `.prepare-progress`, `.prepare-stages`, `.prepare-error` rules.

Verification: visual only — confirm the button reads as primary.

Commit message: `T-008-01: style Prepare for scene button + progress`

## Step 4 — Orchestrator and wiring

Files: `static/app.js`.

- Add the five new `getElementById` constants near the existing
  `generate*Btn` block.
- Add the `// ── Prepare for Scene ──` section: `prepareForScene`,
  `setPrepareStages`, `markPrepareStage`, and the four `runStage*`
  adapters described in `structure.md`.
- Wire `prepareForSceneBtn.addEventListener('click', …)` and
  `viewInSceneBtn.addEventListener('click', …)` in the event-listener
  block.
- Add `prepareForSceneBtn.disabled = !file || !currentModel;` inside
  `updatePreviewButtons`.
- Expose `window.prepareForScene = prepareForScene;` for devtools-driven
  manual verification.

Verification (manual, per AC bullet 8):

1. `go run .` and open the app.
2. Drop a fresh `.glb` (a rose if available, otherwise any test asset).
3. Wait for the file to appear in the list, click it. Confirm the
   preview loads and `Prepare for scene` becomes enabled (it gates on
   `currentModel`).
4. Click `Prepare for scene`. Watch the stage list:
   - `[•] Optimize` → `[✓] Optimize`
   - `[•] Classify` → `[✓] Classify` (or skipped if already classified)
   - `[•] LOD` → `[✓] LOD`
   - `[•] Production asset` → `[✓] Production asset`
5. Confirm the `View in scene` button appears.
6. Click `View in scene`; confirm the stress-test scene runs with the
   asset's current scene template.
7. Look at the latest `~/.glb-optimizer/tuning/*.jsonl`. The last few
   events should include one `prepare_for_scene` envelope with
   `payload.success === true` and `stages_run` listing every stage that
   actually ran.
8. Failure path: edit one of the `runStage*` helpers to throw, click
   Prepare again, confirm:
   - The failing stage shows `[✗]` with the error message.
   - Subsequent stages are not run.
   - The emitted `prepare_for_scene` event has `success: false`,
     `failed_stage`, and `error` populated.
   - Revert the synthetic throw before committing.

Commit message: `T-008-01: orchestrate prepare-for-scene pipeline`

## Test strategy

- **Go:** existing `analytics_test.go` covers envelope validation. The
  one-line allow-list addition is exercised by the existing happy-path
  tests; no new test is added because the assertion would just be "this
  string is in the map", which the implementation already is. If we
  later add a test that asserts the exact set of valid event types,
  this is the place to update it.
- **JS:** the project has no JS test harness today. Verification is
  manual per the AC. Document the manual checklist (steps 1–8 above) in
  `progress.md` so the next reviewer can re-run it.
- **Regression risk:** the existing technique buttons remain wired and
  unchanged. The orchestrator only *calls* them; it does not mutate
  their DOM state directly (their own `finally` blocks handle that).
  After a Prepare run, clicking e.g. the standalone `Production Asset`
  button should still work exactly as before. Add this to the manual
  checklist as step 9.

## What success looks like at the end of this ticket

- A single primary button on the preview toolbar runs the full pipeline
  end-to-end against the selected asset.
- Progress is visible per stage; errors are clearly attributed.
- One `prepare_for_scene` event is emitted per click with accurate
  `stages_run`, `total_duration_ms`, `success`, and (on failure)
  `failed_stage` + `error`.
- After a successful run, `View in scene` triggers `runStressTest` with
  the asset's current scene template settings.
- The existing technique buttons and `Process All` still work.

## What is explicitly NOT in this ticket

- Hiding the technique buttons (T-008-02).
- Renaming any existing label (T-008-02).
- Inline help text (T-008-03).
- Pipeline retry / resume.
- Background pipeline execution.
- Scene preview thumbnail capture (no helper exists; AC marks it
  optional; deferring keeps the diff small).
