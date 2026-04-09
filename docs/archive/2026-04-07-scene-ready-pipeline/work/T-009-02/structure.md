# Structure — T-009-02 tilted-billboard-loader-and-instances

## Files touched

| File | Change | Approx LOC delta |
|---|---|---|
| `static/app.js` | Add `tiltedBillboardInstances` state, `createTiltedBillboardInstances`, `updateTiltedBillboardFacing`. Extend `clearStressInstances`, animate loop, `updatePreviewButtons`, LOD click handler `fileSize` predicate. | +75 / -2 |
| `static/index.html` | Insert one new `<button data-lod="billboard-tilted">` between the existing `billboard` and `volumetric` buttons. | +1 / -0 |
| `docs/active/work/T-009-02/*.md` | RDSPI artifacts. | new |

No Go files change. No new files in `static/`. No CSS changes — the
new button inherits the `.lod-toggle button` selector that already
styles its siblings.

## In-file structure of `static/app.js` changes

The new code lives in five distinct, non-overlapping insertions. Each
is a focused edit so a reviewer can verify them one at a time.

### Insertion 1 — animate-loop hook (~line 3343)

After the existing horizontal billboard guard:

```js
if (stressActive && (billboardInstances.length > 0 || billboardTopInstances.length > 0)) {
    updateBillboardFacing();
    updateBillboardVisibility();
}
+if (stressActive && tiltedBillboardInstances.length > 0) {
+    updateTiltedBillboardFacing();
+}
```

### Insertion 2 — `clearStressInstances` reset (~line 3375)

```js
stressInstances = [];
billboardInstances = [];
+tiltedBillboardInstances = [];
billboardTopInstances = [];
volumetricInstances = [];
```

### Insertion 3 — state declaration (~line 3789)

Adjacent to the existing `billboardInstances` / `billboardTopInstances`
declarations so all three live in one block:

```js
let billboardInstances = []; // ...existing comment
let billboardTopInstances = []; // ...existing comment
+// Tilted-camera billboard instances (T-009-02). Side-only — no
+// `billboard_top` quad in the tilted bake. Camera-facing in yaw,
+// same math as `billboardInstances`; the tilt is baked into the
+// texture, not the runtime transform.
+let tiltedBillboardInstances = [];
```

### Insertion 4 — `createTiltedBillboardInstances` and
`updateTiltedBillboardFacing` (after `updateBillboardVisibility`,
~line 4040)

Two new functions, ~70 lines together. Bodies are spelled out in
`design.md`. Placed adjacent to the horizontal helpers so future
maintainers can diff the pair at a glance. Public surface (i.e. what
the rest of `app.js` calls):

- `createTiltedBillboardInstances(model, arr): InstancedMesh[]` —
  same return contract as `createBillboardInstances` so a future
  caller can `stressInstances.push(...result)`.
- `updateTiltedBillboardFacing(): void` — called from the animate
  loop. No-op if `tiltedBillboardInstances` is empty.

### Insertion 5 — `updatePreviewButtons` predicates (~line 4464, ~4470)

Two small edits in the same function:

```js
lodToggle.style.display = hasLods || hasVlods ||
-    (file && (file.has_billboard || file.has_volumetric))
+    (file && (file.has_billboard || file.has_billboard_tilted || file.has_volumetric))
    ? 'flex' : 'none';
```

```js
} else if (lod === 'billboard') {
    btn.disabled = !(file && file.has_billboard);
+} else if (lod === 'billboard-tilted') {
+    btn.disabled = !(file && file.has_billboard_tilted);
} else if (lod === 'volumetric') {
    ...
```

### Insertion 6 — LOD click handler `fileSize` predicate (~line 4549)

```js
-if (version === 'billboard' || version === 'volumetric' || version === 'production') {
+if (version === 'billboard' || version === 'billboard-tilted' || version === 'volumetric' || version === 'production') {
    fileSize = 50000;
}
```

No change needed to the `previewVer` mapping; `billboard-tilted`
passes through to the backend as-is.

## In-file structure of `static/index.html` changes

Single line insertion at line 46, between the existing `billboard`
and `volumetric` buttons:

```html
<button data-lod="billboard"        disabled>Camera-facing</button>
+<button data-lod="billboard-tilted" disabled>Tilted</button>
<button data-lod="volumetric"       disabled>Dome slices</button>
```

## Module boundaries / public interfaces

- **Module-level state.** `tiltedBillboardInstances` is a `let` in the
  same scope as `billboardInstances`. Not exported. Accessed only by
  `createTiltedBillboardInstances`, `updateTiltedBillboardFacing`,
  and `clearStressInstances`.
- **Window globals.** None new. The existing `window.selectedFileId`
  getter from T-009-01 is sufficient for devtools verification.
- **DOM globals.** None new. The new button is reached through the
  existing `lodToggle.querySelectorAll('button')` loop in both
  `updatePreviewButtons` and the click-handler block.

## Ordering of changes

The insertions are commutative — none of them depend on another being
applied first to compile or to preserve behaviour. They will be made
in the order listed (1 through 6 in app.js, then the index.html line)
purely for review readability.

## Test surface

There is no JS test harness in this repo (confirmed in T-009-01 review,
section "Test coverage"), so verification is manual. See `plan.md` for
the verification recipe. The Go side is unchanged in this ticket so
existing `go test ./...` coverage is sufficient and re-running it is
just smoke insurance.
