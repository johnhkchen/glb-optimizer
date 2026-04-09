# T-004-04 — Design

## Decisions

### D1. Candidate source: Python ranking inside `features.candidates`

**Picked.** `classify_shape.py` already computes the full distance
ranking over the four geometric centroids and discards it
(`_ranking` at line 334). We extend `classify_points` to surface
that ranking inside the existing opaque `features` dict as
`features.candidates = [{category, score}, ...]`, sorted descending
by softmax score, and we append `hard-surface` (with the overlay
score, mapped onto a normalized 0–1 number) when
`is_hard_surface=true`.

Rejected:

- **Hard-coded frontend list.** Wastes the classifier's actual
  ranking — would always show the same three candidates regardless
  of the asset's geometry. Defeats the training-data motivation:
  the most informative human picks are exactly the ones near a
  decision boundary, and the boundary differs per asset.
- **New Go endpoint that returns the ranking.** More wiring (route,
  handler, struct, tests) for the same payload. The ranking is
  just metadata about *one* classification result; bolting it onto
  the existing classify response is the natural place.
- **Append to settings.** Settings is the canonical "what to bake";
  the ranking is ephemeral diagnostic data that loses meaning the
  moment the user picks. It does not belong on disk in
  `AssetSettings`.

The `features` map is opaque (per analytics-schema.md) and Go's
`map[string]interface{}` decoding round-trips it unchanged. **No Go
code change is required for the ranking itself** — the change is
isolated to one Python function and one new analytics-schema bullet.

### D2. `/api/classify/:id` response shape: `{settings, candidates}`

**Picked.** The handler today returns just `AssetSettings`. We change
the response body to:

```json
{
  "settings": { ...AssetSettings... },
  "candidates": [
    { "category": "directional", "score": 0.41 },
    { "category": "tall-narrow", "score": 0.33 },
    { "category": "round-bush",  "score": 0.18 }
  ]
}
```

Top-N is bounded at 3 in the handler (matches the modal's slot
count). Backwards compatibility is not a concern because there is
no consumer of this endpoint today other than the about-to-be-built
modal — but `autoClassify` (upload-time) does **not** change shape;
it still writes settings to disk and emits the classification +
strategy_selected events as before. The candidates array exists
only on the synchronous JSON response.

### D3. Auto-open trigger: `selectFile` with `shape_confidence < 0.7`

**Picked.** When the user selects an asset whose persisted
`shape_confidence` is in `(0, 0.7)`, `selectFile` calls
`POST /api/classify/:id` once (idempotent) and opens the comparison
modal with the returned candidates.

Sentinels:

- `shape_confidence == 0`: never been classified (legacy file or
  classifier outage on upload). Don't auto-open — there is nothing
  to compare against.
- `shape_confidence == 1.0`: human-confirmed via this ticket's
  override flow. Don't auto-open; the user already resolved the
  ambiguity.
- `shape_confidence ∈ (0, 0.7)`: low-confidence classifier output.
  Auto-open.
- `shape_confidence ∈ [0.7, 1.0)`: confident enough; the user can
  still trigger the modal manually via "Reclassify…".

Re-running the classifier on every low-confidence selection is the
right cost: it costs ~1s, and once the user picks, confidence
flips to 1.0 and the modal stops opening.

Rejected:

- **Auto-open from /api/upload directly.** Upload doesn't return
  to the front of the model preview pipeline; the asset isn't
  loaded into Three.js yet, so there's no `currentModel` to bake.
  Selection is the natural moment.
- **Auto-open whenever shape_confidence < 0.7, regardless of
  human-confirmed flag.** Would re-prompt the user every selection
  even after they resolved it. Annoying.

### D4. Thumbnails: full bake roundtrip per candidate

**Picked.** For each candidate, we (a) clone `currentSettings` with
the candidate strategy's `slice_axis`, `slice_distribution_mode`, and
`volumetric_layers` patched in, (b) call `renderHorizontalLayerGLB`
to get a GLB byte buffer, (c) load that GLB via GLTFLoader into a
small offscreen Three.js scene, (d) render to a 256px canvas with a
fixed perspective camera, (e) `toDataURL()` and stuff into an
`<img>` in the modal.

This is exactly the existing `runPipelineRoundtrip` pattern (line
1990). The cost is ~1–3s per candidate × 3 candidates = up to 10s
to fully populate the modal. The ticket explicitly says "expect
this to take a few seconds", and the modal renders thumbnails
sequentially with a "Rendering…" placeholder so the user sees
progress.

Rejected:

- **Cheap axis-overlay render.** A render of the original model
  with arrows showing the chosen slice axis is fast but not what
  the ticket asks for ("asset processed with that strategy"). The
  whole point is to see the bake output that the strategy
  produces, because that's what the user is judging.
- **Single full bake + crop.** Strategies differ in slice axis and
  layer count, not just framing — you cannot derive the directional
  bake from the round-bush bake.
- **Pre-bake on upload.** Costs CPU on every upload even when the
  user never opens the modal; auto-classify already does the
  cheapest part (the classifier itself). Defer.

The render uses `currentModel` directly (the Three.js scene's
loaded asset) — same input that the live bake button uses — so
there is no extra GLB fetch or decode.

### D5. Pick action: re-use `applyClassificationToSettings` semantics, set confidence=1.0

**Picked.** The override "Pick" handler does **not** mutate settings
on the frontend and PUT them. Instead it calls
`POST /api/classify/:id?override=<category>`, a new query mode on
the existing handler. Server-side that path:

1. Synthesizes a `ClassificationResult` with the user's chosen
   category, `Confidence: 1.0`, and the *features that the most
   recent classifier run produced* (re-run the classifier so the
   features are current; cheap and consistent).
2. Calls `applyClassificationToSettings` with that result, which
   stamps the strategy router's defaults exactly the same way as a
   genuine classification — preserving the "user override survives"
   invariant tested in `strategy_handlers_test.go`.
3. Emits a `classification_override` analytics event (the *new*
   training-data event from this ticket) instead of a plain
   `classification` event. The override event payload carries
   everything the export pipeline needs to recover the decision:
   classifier-original category + confidence, full candidates list,
   chosen category, asset features.
4. Returns `{settings, candidates}` (same shape as the non-override
   path) so the modal can close cleanly with the new state.

Rejected:

- **Pure-frontend override (PUT settings).** Bypasses the
  strategy-stamping helper and the dirty-tracking, and would
  duplicate the classifier feature dump in JS. The whole reason
  `applyClassificationToSettings` exists is to be the single
  point of truth for "category + features → settings".
- **Override is a separate endpoint
  (`POST /api/classify/:id/override`).** A second route with the
  same handler skeleton but a different payload assembler. The
  query-param variant is one branch in the existing handler and
  shares all the validation / store-update plumbing.

### D6. New analytics event: `classification_override`

**Picked.** Emitted server-side from the override branch of
`handleClassify`. Payload:

```json
{
  "original_category": "planar",
  "original_confidence": 0.42,
  "candidates": [
    {"category": "planar",       "score": 0.42},
    {"category": "directional",  "score": 0.31},
    {"category": "tall-narrow",  "score": 0.20}
  ],
  "chosen_category": "directional",
  "features": { ... full feature dump ... }
}
```

Registered in `validEventTypes` (analytics.go:25) and documented in
`docs/knowledge/analytics-schema.md` alongside `classification` and
`strategy_selected`. The export pipeline already inlines all events
per asset; no exporter change is needed.

The split between `classification` (the system's decision) and
`classification_override` (the human's correction) is intentional:
downstream training treats them as different label types. A pair
`(classification confidence=0.42, classification_override
chosen=directional)` is a high-value training example. A solo
`classification confidence=0.95` is a low-value confirmation.

### D7. Modal styling: minimal, lives in `index.html` + `style.css`

**Picked.** Single new `<div id="comparisonModal">` at the bottom of
`<body>`, hidden by default with `display:none`. Three slot
sub-divs, each with an `<img>`, the category label, the confidence
score, and a "Pick {category}" button. CSS reuses existing
`--panel-bg` / `--panel-border` / `--accent` variables. No
animation library, no positioning library — `position:fixed; inset:
0;` + a flex centering wrapper.

Closing the modal without picking is allowed (`Cancel` button +
backdrop click): the asset keeps its existing classifier-derived
category and confidence, and the auto-open will fire again next
time the user selects this asset (which is correct — the user
hasn't resolved it yet). No analytics event for cancel — `cancel`
isn't a labeled training signal.

## Where the changes land (high level)

| File                              | Change                                                            |
|-----------------------------------|-------------------------------------------------------------------|
| `scripts/classify_shape.py`       | `classify_points` returns `features.candidates`                    |
| `scripts/classify_shape_test.py`  | One new test asserting `features.candidates` shape and ordering    |
| `classify.go`                     | (no change — opaque features map)                                  |
| `handlers.go`                     | `handleClassify` returns `{settings, candidates}`; override branch via `?override=` query; `emitClassificationOverrideEvent` helper |
| `analytics.go`                    | Register `classification_override` event type                      |
| `analytics_test.go`               | One new test for the new envelope type                             |
| `strategy_handlers_test.go`       | One new end-to-end test of the override path                       |
| `docs/knowledge/analytics-schema.md` | New `classification_override` event section                     |
| `static/index.html`               | New modal markup + new "Reclassify…" button in tuning panel        |
| `static/style.css`                | Modal styles                                                       |
| `static/app.js`                   | `openComparisonModal`, `renderCandidateThumbnail`, `pickCandidate`, `selectFile` low-confidence trigger, Reclassify button wiring |
