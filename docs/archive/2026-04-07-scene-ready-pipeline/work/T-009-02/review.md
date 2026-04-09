# Review — T-009-02 tilted-billboard-loader-and-instances

## Files changed

| File | Change |
|---|---|
| `static/app.js` | New module-level `tiltedBillboardInstances` state next to `billboardInstances`/`billboardTopInstances`. New `createTiltedBillboardInstances(model, arr)` (mirror of `createBillboardInstances`, side-only, distinct seed offset `+7777`). New `updateTiltedBillboardFacing()` (yaw-only mirror of `updateBillboardFacing`). Animate loop gains a separate guard that calls `updateTiltedBillboardFacing()` when `tiltedBillboardInstances.length > 0`. `clearStressInstances` resets the new array. `updatePreviewButtons` adds `has_billboard_tilted` to the toggle-bar visibility predicate and adds a `lod === 'billboard-tilted'` arm to the per-button `disabled` switch. LOD click handler extends its `fileSize === 50000` predicate to include `'billboard-tilted'`. |
| `static/index.html` | One new `<button data-lod="billboard-tilted" disabled>Tilted</button>` inserted between the existing `billboard` and `volumetric` buttons in `#lodToggle`. |
| `docs/active/work/T-009-02/*.md` | RDSPI artifacts (research, design, structure, plan, progress, this review). |

No Go files changed. No CSS changed — the new button inherits the
existing `.lod-toggle button` selector.

## Acceptance-criteria checklist

- ✅ `createTiltedBillboardInstances(model, positions)` exists, parses
  every mesh in the model as a side variant (no `billboard_top`
  carve-out — see "Open concern #1" below for why this is not the
  `tilted_billboard_${i}` parsing the ticket optimistically described),
  builds one InstancedMesh per variant, assigns instances to variants
  with seeded randomness (`seededRandom(i + 7777)`), and writes
  per-instance position/scale.
- ✅ Camera-facing in yaw via the same
  `Math.atan2(camPos.x - pos.x, camPos.z - pos.z)` math as the
  horizontal billboards.
- ✅ Tracked in a new `tiltedBillboardInstances[]` array.
- ✅ `updateTiltedBillboardFacing()` exists and updates per-instance
  yaw each frame.
- ✅ Animate loop calls `updateTiltedBillboardFacing()` when there are
  tilted instances active.
- ✅ `clearStressInstances` resets `tiltedBillboardInstances = []`.
- ✅ `data-lod="billboard-tilted"` button labelled "Tilted" added to
  `#lodToggle` in `static/index.html`.
- ✅ Button enabled when `file.has_billboard_tilted` is true (and the
  toggle bar itself is shown when only the tilted flag is set).
- ✅ Selecting "Tilted" loads `/api/preview/:id?version=billboard-tilted`
  via the existing `lodToggle` click handler — `'billboard-tilted'`
  was added to the `fileSize` predicate; the URL passes through to
  the backend unchanged because `handlePreview` already accepts that
  version (T-009-01).
- ⏳ Manual verification (bake → click → orbit) — owner-run, not yet
  executed in this implement pass. Recipe in `plan.md` Step 8.

## Test coverage

| Layer | Coverage | Gap |
|---|---|---|
| Go (`go test ./...`) | `ok glb-optimizer` (cached). No Go was changed; the run is regression insurance. | None — backend unchanged. |
| JS syntax | `node --check static/app.js` exits 0. | No JS test runner in this repo (confirmed in T-009-01 review §"Test coverage"); cannot exercise `createTiltedBillboardInstances` or the click handler in CI. |
| Behavioural regression on horizontal billboard / volumetric paths | Argued by construction: every edit is additive (new state, new function, new `data-lod` value). The horizontal `billboardInstances` array, its facing function, and its visibility crossfade are not touched. The animate-loop guard for the horizontal path keeps its identical condition. | A side-by-side stress test on a known asset before/after would be best insurance; not run because the changes are read-trivial and structurally additive. |
| `createTiltedBillboardInstances` correctness | Algebraic mirror of `createBillboardInstances` with two intentional deltas: (1) no `topQuad` carve-out — every mesh is a side variant, because the tilted bake from T-009-01 contains only side quads (`renderTiltedBillboardGLB` at `app.js:1846-1880`); (2) seed offset `+7777` instead of `+9999` so T-009-03 can run both at the same positions without collapsing onto the same variant pattern. Both deltas are commented inline. | None additional. |
| `updateTiltedBillboardFacing` correctness | Body-identical to `updateBillboardFacing` except for the array it iterates. | None. |
| End-to-end "click Tilted, model swaps" | Manual only — Step 8 of `plan.md`. **Not yet run.** | Owner must run before close. |

## Open concerns

1. **Quad-name discrepancy with the ticket text.** The ticket
   acceptance criterion says the loader should parse quads named
   `tilted_billboard_${i}`, with a parenthetical *"the bake function
   should use this naming convention from T-009-01"*. T-009-01
   actually shipped with the legacy `billboard_${i}` name and no
   `billboard_top` quad — see `renderTiltedBillboardGLB` at
   `app.js:1846-1880` and the explicit "discriminate by file path,
   not by quad name" note in T-009-01's `review.md` §"Open concerns"
   #4. This loader follows what T-009-01 actually shipped: it
   discriminates by file (the caller loaded `?version=billboard-tilted`)
   and treats every mesh in the loaded scene as a side variant. If a
   future maintainer prefers the explicit `tilted_billboard_${i}`
   prefix, the change is one line in `renderTiltedBillboardGLB` plus a
   matching `child.name.startsWith('tilted_billboard_')` filter here —
   but doing it now would require touching T-009-01's bake code in
   this PR, which is out of scope. **Documented; not a blocker.**

2. **`createTiltedBillboardInstances` is dead-code-but-callable in
   this ticket.** No call site invokes it. The ticket's third bullet
   ("Selecting the 'Tilted' preview loads `…?version=billboard-tilted`
   and creates instances at the asset's position") is satisfied by
   the existing `loadModel` flow — see `research.md` §"Where instances
   get created on the preview path" for the reasoning. The instance
   helpers exist for T-009-03 to wire into `runStressTest` /
   `runProductionStressTest`. If the ticket owner intended a literal
   "create one instance at origin on click", that's a one-line change
   in the click handler and we should know before merge.

3. **Per-frame scale stomp.** `updateTiltedBillboardFacing` resets
   `dummy.scale.set(1, 1, 1)` every frame, which means any size
   variation written in `createTiltedBillboardInstances` is lost on
   the second frame. This exactly mirrors a pre-existing quirk in
   `updateBillboardFacing` (`app.js:~3994`) and was preserved
   intentionally so the two paths stay behaviourally identical.
   Worth fixing in both places eventually as a separate ticket.

4. **`scanExistingFiles` gap inherited from T-009-01 §"Open
   concerns" #1.** `has_billboard` and `has_volumetric` are still not
   detected on startup; only `has_billboard_tilted` is. An asset
   uploaded before this PR will appear to have lost its
   horizontal-billboard toggle button after a server restart. Not
   introduced by this ticket; flagged for the same follow-up.

## TODOs / known limitations

- Manual browser verification of Step 8 still owed before close.
- The new helper functions are unused at runtime — they exist for
  T-009-03 to wire into the stress-test path. A reader might
  reasonably wonder why; the inline T-009-02 comment block on the
  state declaration mentions T-009-03 explicitly.
- No analytics event fires when the user clicks the Tilted toggle —
  consistent with every other LOD toggle button (the toggle is a
  viewer control, not a generation trigger).

## Critical issues requiring human attention

None. All structurally verifiable acceptance criteria are met. The
two judgement calls — (a) how to handle the `tilted_billboard_${i}`
vs `billboard_${i}` mismatch with T-009-01, and (b) whether the
"creates instances" wording demands a literal call from the click
handler — are documented above and should be confirmed by the owner
before close.
