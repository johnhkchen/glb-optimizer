# Plan — T-009-02 tilted-billboard-loader-and-instances

## Step list

Each step is independently committable. Steps 1–6 modify `static/app.js`;
step 7 modifies `static/index.html`. Step 8 is verification.

### Step 1 — Add module-level state

Edit `static/app.js`. Insert `let tiltedBillboardInstances = [];` (with
its T-009-02 comment) immediately after the existing
`billboardTopInstances` declaration.

**Verify**: file parses (open in browser, no console errors). The
loader page should render unchanged.

### Step 2 — Add `createTiltedBillboardInstances` and
`updateTiltedBillboardFacing`

Edit `static/app.js`. Insert the two new functions immediately after
the existing `updateBillboardVisibility` block. Bodies are spelled out
in `design.md`.

**Verify**: file parses. The functions are not yet called from anywhere,
so behaviour is unchanged.

### Step 3 — Wire `clearStressInstances`

Edit `static/app.js`. Add `tiltedBillboardInstances = [];` to the reset
block.

**Verify**: file parses. Stress test (existing) still cleans up
correctly.

### Step 4 — Wire animate loop

Edit `static/app.js`. Add the new
`if (stressActive && tiltedBillboardInstances.length > 0)` block
immediately after the existing horizontal billboard guard.

**Verify**: existing horizontal stress test still works (camera-facing
behaviour unchanged because the new branch is gated on a still-empty
array).

### Step 5 — Wire `updatePreviewButtons`

Edit `static/app.js`. Two changes in the same function: the
`lodToggle.style.display` predicate and the per-button `disabled`
switch.

**Verify**: Existing files still render their toggle bars correctly.
For an asset with no LODs, no volumetric, and no billboard, the bar
stays hidden. For an asset whose `has_billboard_tilted` flag is true,
the bar appears and the new button is enabled.

### Step 6 — Wire LOD click handler `fileSize` predicate

Edit `static/app.js`. Add `'billboard-tilted'` to the existing
or-list in the click handler.

**Verify**: clicking any other LOD button still works as before.

### Step 7 — Add the HTML button

Edit `static/index.html`. Insert the
`<button data-lod="billboard-tilted">Tilted</button>` line.

**Verify**: button appears in the correct position in the toggle bar
and inherits its sibling's styling.

### Step 8 — Manual verification on a real asset

This is the acceptance-criteria smoke check from the ticket. Owner-run
in a browser session.

1. Pick or upload an asset that does not yet have a tilted billboard.
2. Open the asset, then run from the devtools console:
   ```js
   await generateTiltedBillboard(selectedFileId)
   ```
   (T-009-01 exposes both names on `window`.)
3. Verify the toggle bar now shows a "Tilted" button (this exercises
   `updatePreviewButtons`).
4. Click "Tilted". The previewed model swaps to the tilted bake — the
   side-by-side row of elevated quads from the export.
5. Refresh the page (browser reload). The toggle bar should still show
   the "Tilted" button (T-009-01's `scanExistingFiles` rehydrated the
   flag from disk; this ticket's `updatePreviewButtons` now honours
   it).
6. Open another asset that has no tilted bake. Confirm the "Tilted"
   button is disabled (greyed) for that asset.
7. Delete the asset from step 1 via the file-list UI. Confirm no
   console errors and that the deletion removes the
   `_billboard_tilted.glb` file from `outputs/` (T-009-01 wired this
   into `handleDeleteFile`).

The "creates instances at the asset's position" wording in the ticket
is satisfied by the existing `loadModel` flow — see `research.md`
section "Where instances get created on the preview path" for the
reasoning. The new `createTiltedBillboardInstances` is dead-code-but-
callable in this ticket and will be wired into the stress-test path
in T-009-03.

## Testing strategy

| Layer | Strategy | Why |
|---|---|---|
| Module load | Step-by-step browser refresh during implementation | Catches syntax errors immediately. The repo has no JS test runner. |
| Behaviour regression on horizontal billboard / volumetric paths | Run an existing stress test before and after the edits | The new code is additive — every change is gated behind a fresh state array or a new `data-lod` value, so the horizontal path's call sites are not touched. The stress test is the broadest behavioural smoke. |
| `createTiltedBillboardInstances` body correctness | Mirror argument vs. `createBillboardInstances` (chosen approach in design.md) | The function is a near-clone with two well-understood deltas: no `billboard_top` carve-out, and a different seed offset. Both are commented inline. |
| `updateTiltedBillboardFacing` correctness | Algebraic mirror of `updateBillboardFacing` | Identical body except for the array it iterates. If horizontal facing works, tilted facing works. |
| Backend touchpoints | `go test ./...` smoke before merge | Should pass clean — this ticket changes no Go. |

## Open deviation budget

If the manual verification surfaces a problem with the **bake** itself
(quad name, missing texture, file path), that is a T-009-01 bug, not
T-009-02. Document in `progress.md` and either patch T-009-01's code
in this PR (with a one-line note in the commit message) or open a
follow-up ticket — whichever the owner prefers in the moment.

If the manual verification surfaces a problem with the **toggle bar**
or the **preview swap**, that is a T-009-02 bug. Fix in this PR.

## What this plan deliberately does not include

- Wiring `createTiltedBillboardInstances` into `runStressTest` /
  `runProductionStressTest`. Owned by T-009-03.
- Three-way crossfade. Owned by T-009-03.
- Per-instance tilt jitter. Out of scope.
- Settings UI for elevation. Out of scope.
- Analytics events for the new button (no `regenerate` or
  `lod_selected` events fire from the existing LOD toggle either —
  this is a viewer toggle, not a generation trigger).
