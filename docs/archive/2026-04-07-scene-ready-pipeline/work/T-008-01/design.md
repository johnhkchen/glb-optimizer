# T-008-01 Design

## What we are deciding

How to wire a new `Prepare for scene` button so that it runs the existing
pipeline stages in order against the selected asset, shows live progress,
stops cleanly on error, and emits one `prepare_for_scene` analytics event
summarizing the run.

## Options considered

### Option A — Thin JS orchestrator that calls the existing functions

Add an `async function prepareForScene(id)` to `static/app.js` that calls,
in order:

1. `processFile(id)` if the file's `status !== 'done'`.
2. `fetchClassification(id)` if `currentSettings.shape_confidence === 0`
   (never classified). Skip otherwise.
3. `generateLODs(id)` (always — gltfpack LOD chain).
4. `generateProductionAsset(id)` (billboard + volumetric, in-browser).
5. (Optional / deferred) thumbnail capture.

Tracks `t0`, `stages_run`, success per stage; emits one
`prepare_for_scene` event at the end. Renders progress through a small
DOM element (text + maybe a stripe) attached to the button. The new
button lives in the toolbar next to `.toolbar-actions`.

**Pros**

- Minimal new surface — every stage already has a working entry point and
  its own DOM-state-restore logic in `finally`. The orchestrator does not
  need to know how to render a billboard or talk to gltfpack.
- Per-stage `regenerate` analytics events keep firing as a side effect,
  giving us both the high-level event and the existing per-stage stream.
- Easy to back out: deleting the function and the button reverts to the
  current state.

**Cons**

- The orchestrator inherits whatever quirks the existing functions have
  (e.g. `generateProductionAsset` swallows errors with a `console.error`
  and only signals failure via `success = false` inside the `finally`).
  We need a way to *detect* failure from the caller's vantage point. Two
  realistic answers:
  1. Refactor each existing function to throw on failure and handle the
     button-state restore at the call site. Bigger blast radius.
  2. Wrap each call in a try/catch in the orchestrator and snapshot the
     before/after state of the file record (e.g. `f.has_billboard` flips
     to true on success). Smaller blast radius but more brittle.
- The "is this stage already done?" decision is duplicated between the
  orchestrator and the underlying functions.

### Option B — New Go endpoint that runs every stage server-side

Add `POST /api/prepare-for-scene/:id` that runs gltfpack, classify, and
LOD generation server-side and streams progress back via SSE or a polling
endpoint.

**Pros**

- Single source of truth for sequencing. Easy to test in Go.
- The progress channel would naturally surface stage-by-stage status.

**Cons**

- The billboard and volumetric stages are *in-browser* WebGL bakes.
  They cannot move to the server without a headless renderer (deferred
  to S-009 at earliest). So the server would still hand control back to
  the client for stages 4–5, and we'd end up coordinating across two
  different orchestrators.
- "First-pass scope" in the ticket explicitly says "no need for elaborate
  state machines or rollback".
- Adds a brand-new HTTP route, request/response shape, handler test, and
  cancellation story for what is fundamentally a UI affordance.

**Rejected.** The cost is real and the only benefit is symmetry with the
other server-side `/api/generate-*` endpoints, which themselves are about
to become "advanced" plumbing.

### Option C — Refactor existing functions into a stage interface, then write the orchestrator on top

Define a `Stage` shape `{ id, label, isApplicable(file), shouldSkip(file), run() }`,
implement one per existing technique button, and run them through a single
`runStages([…])` driver.

**Pros**

- Cleaner long-term. Plays nicely with the Advanced disclosure in T-008-02
  if that ticket needs to render the same stages individually.

**Cons**

- Premature abstraction for a five-step pipeline whose stages are unlikely
  to grow in number. Three of the five stages already share a near-identical
  shape (`textContent = '...'`; `classList.add('generating')`; try/catch;
  `finally` restore) and the duplication has not actively hurt anyone.
- Larger diff, more places to be wrong, harder to review.
- Violates CLAUDE.md "don't create helpers, utilities, or abstractions for
  one-time operations".

**Rejected.** Worth revisiting only if a third caller (T-008-02 advanced
mode? a future scripting API?) needs to enumerate the same stages.

## Decision: Option A, with these specifics

1. **Failure detection.** Each stage gets a small adapter inside the
   orchestrator. The adapters wrap the existing functions and return
   `{ ok: boolean, label: string }`. They do **not** modify the existing
   functions. Detection rules:
   - `processFile`: success when `files[i].status === 'done'` after the
     call, error when it is `'error'`.
   - `fetchClassification`: success when the call resolves without
     throwing — it already throws on non-2xx.
   - `generateLODs`: success when the resolved file record has
     `lods.length > 0` and no `lods[i].error`.
   - `generateProductionAsset`: success when both `f.has_billboard` and
     `f.has_volumetric` are true after the call.
   This sniff-the-store approach is admittedly fragile, but it is the
   smallest change that lets us detect failure without touching the
   existing functions, which all have other callers (the toolbar buttons
   we are not removing in this ticket).

2. **Skip rules.**
   - gltfpack: skip when `f.status === 'done'`.
   - classify: skip when `currentSettings.shape_confidence > 0`.
   - LOD: always run (cheap on small assets, fresh settings can change
     output).
   - Production asset: always run.
   - Thumbnail: not implemented; the AC marks it optional.

3. **Progress UI.** A new `<div id="prepareProgress">` lives next to the
   button and renders one line per stage as `[ ] Optimizing…` →
   `[✓] Optimizing` / `[✗] Optimizing — <error>`. Clicking the button
   disables it and replaces its label with `Preparing…`. On completion
   (success or failure) the label restores; on error, the failing stage's
   error message stays visible until the next click.

4. **Button placement.** Inside `.toolbar-actions` in `#previewToolbar`,
   as the first child, so it sits visually distinct from the existing
   technique buttons. Style: a new `.toolbar-btn-primary` modifier
   (filled accent color) so it reads as the primary action. We do not
   touch `.panel-actions` in the left panel — `processAllBtn` is the
   batch path and Prepare is the per-asset path; mixing them would
   conflate two different mental models.

5. **"View in scene" affordance.** After a successful run, the progress
   block grows a `View in scene` button that calls `stressBtn.click()`.
   That button already reads the active template id, instance count, and
   ground/LOD toggles from the toolbar — exactly what AC bullet 4 means
   by "the user's last scene template".

6. **Analytics.** Add `"prepare_for_scene": true` to `validEventTypes` in
   `analytics.go` and a documentation block in
   `docs/knowledge/analytics-schema.md`. Payload shape:
   `{ stages_run: ["gltfpack", "classify", "lods", "production"],
      total_duration_ms: <int>, success: <bool>, failed_stage?: <string>,
      error?: <string> }`.
   `failed_stage` and `error` are present only when `success === false`.
   The event fires once per click, regardless of skip-vs-run, and
   `stages_run` lists every stage that actually executed (skipped stages
   are omitted).

7. **Disabling rules.** Mirror `generateProductionBtn`: enabled iff
   `selectedFileId && currentModel`. Disabled while a Prepare run is in
   flight. Re-enabled in `finally`.

## Risks left after this design

- **Sniff-the-store failure detection** is brittle. If a future change to
  one of the existing stages stops mutating the field we sniff, the
  orchestrator will silently report success. Mitigated by keeping the
  per-stage `regenerate` events firing — those carry the authoritative
  `success` boolean from inside each function, so the analytics stream
  will still show divergence if it ever happens.
- **Re-running gltfpack** after the file is `done` is a no-op skip in
  this design. If a user changes gltfpack settings between runs, they
  must hit `Process All` (the existing left-panel button) first or
  delete and re-upload. Documented as an explicit limitation, not a bug.
