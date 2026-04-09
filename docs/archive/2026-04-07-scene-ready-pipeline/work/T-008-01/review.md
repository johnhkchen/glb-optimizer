# T-008-01 Review

## What changed

A new `Prepare for scene` primary action runs the existing pipeline
end-to-end against the selected asset, with a per-stage progress block
and a post-run `View in scene` shortcut. One `prepare_for_scene`
analytics event summarizes each click. The existing technique buttons
remain wired and unchanged — hiding them is T-008-02.

### Files modified

- `analytics.go` — added `"prepare_for_scene": true` to
  `validEventTypes`. One-line additive change.
- `docs/knowledge/analytics-schema.md` — new `### prepare_for_scene`
  section between `scene_template_selected` and `strategy_selected`.
  Documents `stages_run`, `total_duration_ms`, `success`, optional
  `failed_stage`, optional `error`. Notes that per-stage `regenerate`
  events still fire.
- `static/index.html` — prepended `#prepareForSceneBtn` to
  `.toolbar-actions`; inserted `#prepareProgress` block (stage `<ul>`,
  error `<div>`, `#viewInSceneBtn`) immediately after.
- `static/style.css` — new `.toolbar-btn-primary` modifier and a small
  `.prepare-progress` / `.prepare-stages` / `.prepare-error` ruleset.
- `static/app.js` — five new `getElementById` constants; new
  `// ── Prepare for Scene (T-008-01) ──` section with `PREPARE_STAGES`,
  `setPrepareStages`, `markPrepareStage`, `prepareForScene`, and
  `window.prepareForScene` for devtools access; one new line in
  `updatePreviewButtons` for the disabled state; two new event-listener
  bindings (`prepareForSceneBtn`, `viewInSceneBtn`).

### Files created / deleted

None. No new Go endpoints.

## How it maps to the acceptance criteria

| AC                                                                                  | Status                                                                                              |
|-------------------------------------------------------------------------------------|-----------------------------------------------------------------------------------------------------|
| New primary `Prepare for scene` button                                              | ✓ — first child of `.toolbar-actions`, styled with `.toolbar-btn-primary`.                          |
| Click runs full pipeline in order using current settings                            | ✓ — gltfpack → classify → LODs → production asset.                                                   |
| Stage 1: gltfpack cleanup if not already done                                       | ✓ — skipped when `f.status === 'done'`.                                                              |
| Stage 2: classify shape (S-004)                                                     | ✓ — skipped when `currentSettings.shape_confidence > 0`.                                             |
| Stage 3: generate LODs                                                              | ✓ — calls `generateLODs(id)`, success-checks the returned `lods` array.                              |
| Stage 4: production asset (billboard + volumetric per shape category)               | ✓ — calls `generateProductionAsset(id)`, which already routes via the asset's classified strategy. |
| Stage 5: optionally generate scene preview thumbnail                                | ⚠ deferred — AC marks it optional, no helper exists; see "Open concerns" below.                     |
| Progress indicator showing current stage                                            | ✓ — `.prepare-stages` list with `[•]/[✓]/[✗]` glyphs and color-coded statuses.                       |
| On completion, surface the result and a "View in scene" action                      | ✓ — `View in scene` button appears post-success and delegates to the existing `#stressBtn`.          |
| If any stage fails, show error clearly and stop                                     | ✓ — failing stage gets `[✗] <error>`, the error message is mirrored in `.prepare-error`, the orchestrator throws and the `finally` re-enables the button without proceeding to later stages. |
| Emit `prepare_for_scene` analytics event with `{stages_run, total_duration_ms, success}` | ✓ — emitted from the orchestrator's `finally`, with `failed_stage` + `error` added on failure.    |
| Existing technique buttons still work                                               | ✓ — they were not touched; the orchestrator only *calls* the existing functions.                     |
| Manual verification: upload a fresh rose, click Prepare, end up with a production asset ready to view in a scene template | Documented as the manual checklist in `progress.md`. Not yet executed against a live workdir in this pass. |

## Test coverage

- **Go:** `go build ./...` and `go test ./...` both pass. The
  `analytics.go` change is exercised by the existing envelope-validation
  tests (the new key is reachable through the same `Validate()` path).
  No new Go test was added because the change is a one-line allow-list
  entry; a future "exact set of valid event types" assertion is the
  natural place to update if it's added.
- **JS:** the project has no JS test harness today. Verification is
  manual per the AC's bullet 8. The orchestrator was syntax-checked
  with `node --check static/app.js`. The full manual checklist is in
  `progress.md` and should be run before merging — items 1–7 cover the
  happy path, item 8 the failure path, and item 9 the regression check
  on the existing technique buttons.

## Open concerns and limitations

1. **Scene preview thumbnail (AC stage 5) is deferred.** AC marks it
   optional and no thumbnail-capture helper exists today. Adding one
   would mean a new offscreen render path; out of scope for "first-pass
   scope" per the ticket. T-008-03 or a follow-up can add it.

2. **Failure detection sniffs the file store.** Each stage's success
   check inspects mutations the underlying function makes
   (`f.status === 'done'`, `f.lods` length, `has_billboard`,
   `has_volumetric`) rather than relying on a thrown error. This is
   because the existing `generateBillboard` / `generateVolumetric` /
   `generateProductionAsset` functions swallow errors with
   `console.error` and only signal via DOM/store state. If a future
   change to those functions stops mutating the field we sniff, the
   orchestrator will silently report success. The per-stage
   `regenerate` events still carry the authoritative `success` boolean
   from inside each function, so divergence will be visible in the
   analytics stream even if it slips past the orchestrator UI.

3. **Re-running gltfpack after `status === 'done'` is a no-op skip.**
   If the user changes gltfpack settings between runs, they must use
   the existing `Process All` button to re-run the optimizer. Not a
   bug — documented in `design.md` — but worth flagging because the
   button label "Prepare for scene" may imply a from-scratch rebuild.

4. **`stages_run` does not list skipped stages.** The schema doc is
   explicit about this. If a downstream consumer wants to distinguish
   "ran fast because skipped" from "ran fast because the asset was
   already prepared", the `total_duration_ms` plus the absence of the
   stage id in `stages_run` is enough; we do not emit `skipped_stages`
   separately. Worth confirming this matches what S-008's analytics
   consumers expect.

5. **`#viewInSceneBtn` delegates to the existing stress button.** That
   couples the affordance to whatever the toolbar's count / template /
   ground inputs say at click time, not at Prepare time. In practice
   this is the right behavior (the user just tuned the scene template
   in the toolbar) but it's worth surfacing because it differs from a
   strict "use the same template that was active when Prepare ran"
   interpretation of AC bullet 4.

6. **Manual verification has not been executed against a live
   workdir** in this implementation pass. The manual checklist in
   `progress.md` should be run before merging.

## Critical issues

None known. All Go tests pass; the JS file parses cleanly; the change
is additive — no existing button or analytics event was modified.

## Suggested reviewer focus

- Does the failure-detection sniff (concern #2) feel acceptable, or
  would you rather refactor the underlying generate functions to
  throw? The refactor is bigger but cleaner and would also tighten
  the existing per-stage `regenerate` events.
- Is the "skip gltfpack when already done" behavior (concern #3) the
  right call, or should Prepare always re-run gltfpack with the
  current right-panel settings?
- Run the manual checklist on a real asset and confirm that step 6
  (`View in scene`) actually surfaces the production asset in a
  recognizable form.
