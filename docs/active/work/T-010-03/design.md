# T-010-03 Design — Pack Button & Endpoint

## Decisions

### Endpoint shape

```
POST /api/pack/:id
→ 200 { "pack_path": "...", "size": N, "species": "..." }
→ 400 if {id}_billboard.glb missing or PackMeta build fails
→ 413 if combine returns "5 MiB cap" error
→ 404 if no FileRecord for id
→ 500 on any other combine / write failure
```

The status-code partition is the part the UI cares about: 413 maps
to a friendly "reduce variant count or texture resolution" message,
400 maps to "you have not built the intermediates yet", and 500 is
the generic fallback.

### Where the file lands

`workDir/dist/plants/{species}.glb`. Created by `main.go` in the
existing mkdir loop alongside `originals/`, `outputs/`, etc. The
handler does **not** create the directory itself — it relies on the
startup mkdir, identical to every other handler.

`{species}` comes from `BuildPackMetaFromBake`'s output, never from
the request URL or the asset id.

### Reading intermediates

```go
sideBytes, err := os.ReadFile(filepath.Join(outputsDir, id+"_billboard.glb"))
```

`os.IsNotExist(err)` ⇒ 400 with message
`"missing intermediate: build the hybrid impostor first"`. Other
read errors ⇒ 500.

For `tilted` and `volumetric`, missing files map to `nil` byte slices
(not an error). This mirrors how `CombinePack` itself models the
optional inputs.

### Translating CombinePack errors

The size-cap check is the only error worth distinguishing from a
generic 500. We detect it by string match on the existing error:

```go
if err != nil {
    if strings.Contains(err.Error(), "exceeds 5 MiB cap") {
        jsonError(w, http.StatusRequestEntityTooLarge, ...)
        return
    }
    jsonError(w, http.StatusInternalServerError, "combine failed: "+err.Error())
    return
}
```

A sentinel error variable in `combine.go` would be cleaner, but
modifying combine.go reopens a file from a sibling ticket
(T-010-02) that just landed. The string check is one line, and the
sibling ticket's error message is fully tested.

### Writing the result

```go
distPath := filepath.Join(distDir, meta.Species+".glb")
if err := os.WriteFile(distPath, packBytes, 0644); err != nil {
    jsonError(w, http.StatusInternalServerError, ...)
    return
}
```

We write the full byte slice in one shot — `CombinePack`'s 5 MiB
cap means we can never need streaming.

### Frontend button placement

A new `<button id="buildPackBtn">` immediately after
`generateProductionBtn` inside the existing `<div class="advanced-panel">`.
That keeps it "near the existing 'Build hybrid impostor' trigger,
not in a new panel" per the ticket. It is the same `toolbar-btn`
class as the rest of the advanced row, so no CSS is needed.

Visibility / enabled state lives entirely inside
`updatePreviewButtons`:

```js
buildPackBtn.disabled = !(file && file.has_billboard
                          && (file.has_billboard_tilted || file.has_volumetric));
```

This matches the acceptance criterion verbatim.

### Frontend click handler

A new `buildAssetPack(id)` async function modeled on
`generateBillboard`. It:

1. Disables the button, sets text to "Packing…".
2. `POST /api/pack/{id}` (no body).
3. On 200, clears `prepareError` and writes a success line:
   `"Pack built: {species}.glb ({size} bytes)"`.
4. On 413, writes the canonical message:
   `"Pack exceeds 5 MB — reduce variant count or texture resolution
    and re-bake"`.
5. On any other non-2xx, writes the server's error string into
   `prepareError`.
6. In all cases, fires
   `logEvent('pack_built', { species, size, has_tilted, has_dome }, id)`
   where `success: false` is encoded as `size: 0`. (We always emit
   the event so the analytics file shows attempts as well as
   successes — consistent with how `regenerate` events are emitted
   from the existing functions.)
7. Re-enables the button and restores its label.

### Reuse of `prepareError`

There is no existing toast widget. `prepareError` is the only
in-toolbar text surface and is already used for prepare-for-scene
failures. Reusing it keeps the toolbar visual footprint constant
and means we don't introduce a parallel error-display path. It is
also the natural neighbor of the new button (both live inside
`.toolbar-actions`).

## Alternatives Considered

### Alternative A: a new combined endpoint that builds intermediates *and* packs

Rejected. The intermediates are built client-side (Three.js
renders, then the JS uploads them via `/api/upload-billboard`,
`/api/upload-volumetric`, etc.). Moving the bake server-side is a
much larger change and explicitly out of scope for the demo.

### Alternative B: HTTP route under `/api/combine/:id` instead of `/api/pack/:id`

Rejected. The ticket's acceptance criteria pin the endpoint to
`POST /api/pack/:id`. "Pack" also matches the user-facing button
label and the dist directory name; "combine" is the
implementation-internal step.

### Alternative C: surface success/error via a new toast component

Rejected. The codebase has zero toast infrastructure today
(`grep -n "showToast\|toast(" static/app.js → no matches`). Adding
a toast is a feature unto itself and the ticket says "the existing
toast/log area" — the closest existing surface is `prepareError`.

### Alternative D: write to `outputsDir` instead of a new `distDir`

Rejected. The ticket explicitly mandates `dist/plants/{species}.glb`,
and the sibling T-010-04 walks `dist/plants/` for the morning-of
USB copy. Mixing finished packs into `outputs/` (which holds
intermediates and reference images) makes both the demo recipe and
human inspection harder.

### Alternative E: derive species in the handler instead of via `BuildPackMetaFromBake`

Rejected. T-011-02 exists precisely so there is *one* species
derivation path. Having the pack handler call `deriveSpeciesFromName`
directly would create a divergence risk: a future fix to the
species rules would have to be applied in two places.

## Risks

- **`BuildPackMetaFromBake` may fail with a non-deterministic error**
  if e.g. the original GLB no longer exists. Mapped to 400 with the
  underlying error, since this is a "user state is wrong" condition,
  not a server bug.
- **Concurrent pack requests for the same id** could race on the
  output write. Unlikely in practice (single user, single browser
  tab), and the worst case is "the second writer wins" — acceptable
  for the demo. No locking added.
