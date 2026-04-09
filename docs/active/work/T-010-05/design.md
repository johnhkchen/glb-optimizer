# T-010-05 Design — pack-size-cap

## Decision

Introduce a typed error `*PackOversizeError` in `combine.go`, populate
it at the cap-check site inside `CombinePack`, and consume it in
`handleBuildPack` via `errors.As`. The error renders itself with a
fixed layout that matches the ticket's example. No new files; no
public-API changes beyond the new exported type.

## Type

```go
// PackOversizeError is returned by CombinePack when the assembled pack
// exceeds the 5 MiB hard cap. It carries enough breakdown for an
// operator to decide what to shrink (texture variants vs mesh density)
// without having to re-instrument the bake.
type PackOversizeError struct {
    Species     string
    ActualBytes int64
    LimitBytes  int64
    Breakdown   PackBreakdown
}

// PackBreakdown decomposes the assembled pack's size into the three
// budgets a baker can act on: texture payload, mesh / vertex payload,
// and the glTF JSON manifest.
type PackBreakdown struct {
    TextureCount int   // number of physical image payloads (post-dedup)
    TextureBytes int64 // sum of image payload bytes in BIN
    MeshBytes    int64 // BIN bytes that aren't textures (verts, indices, etc.)
    JSONBytes    int64 // length of the gltf JSON chunk
}

func (e *PackOversizeError) Error() string { … }
```

`PackOversizeError` implements `error`. Callers detect it with
`errors.As`. We deliberately do *not* implement `Unwrap()` — there is
no underlying cause to expose.

## Error message layout

The `Error()` method renders the message verbatim per AC:

```
pack "achillea_millefolium" exceeds 5 MB limit (actual: 6.2 MB)
  textures:    18 × avg 320 KB = 5.7 MB
  meshes:      147 KB
  metadata:    2 KB
hint: reduce billboard texture resolution or variant count and re-bake
```

Formatting rules:

- *Sizes* render via a small helper `humanBytes` that picks B / KB /
  MB based on magnitude (`>= 1_000_000` ⇒ MB, `>= 1000` ⇒ KB, else B).
  Decimal rendering uses one fractional digit for MB ("6.2 MB"), zero
  for KB ("147 KB"). Values are decimal MB (1_000_000), matching the
  ticket's "5 MB" wording even though the cap constant is binary MiB.
- The `actual:` value comes from `ActualBytes`.
- *Textures row* — only renders the `× avg = ` portion when
  `TextureCount > 0`. When zero, prints `textures:    none`.
- *Meshes row* — always present (always nonzero in practice; even an
  empty pack carries header bytes).
- *Metadata row* — always present.
- The `hint:` trailing line is fixed copy from the AC and never
  customized — it's the same advice in all cases.

## Computing the breakdown inside CombinePack

We thread an `imageBytes int64` field on `mergeContext`, incremented
once per *new* (non-deduped) image inside `absorbImage`:

```go
mc.imageBytes += int64(len(payload))
```

At the cap-check site, after `writeGLB`:

```go
if len(raw) > packSizeCap {
    jsonRaw, _ := json.Marshal(mc.out) // re-marshal for length only
    return nil, &PackOversizeError{
        Species:     meta.Species,
        ActualBytes: int64(len(raw)),
        LimitBytes:  packSizeCap,
        Breakdown: PackBreakdown{
            TextureCount: len(mc.out.Images),
            TextureBytes: mc.imageBytes,
            MeshBytes:    int64(mc.outBin.Len()) - mc.imageBytes,
            JSONBytes:    int64(len(jsonRaw)),
        },
    }
}
```

The re-marshal is wasted work in the failure case, but it only fires
on the cap path (which already aborts the pipeline) and avoids
plumbing JSON length back from `writeGLB`.

`MeshBytes = outBin.Len() - imageBytes` — every byte in `outBin`
that isn't an image payload is, by definition, a vertex / index /
attribute. This is the right grouping for the user-facing breakdown.

## Handler integration

In `handleBuildPack` (handlers.go:1788–1798) replace:

```go
if strings.Contains(err.Error(), "5 MiB cap") {
    jsonError(w, http.StatusRequestEntityTooLarge,
        "pack exceeds 5 MB — …")
    return
}
```

with:

```go
var oversize *PackOversizeError
if errors.As(err, &oversize) {
    jsonError(w, http.StatusRequestEntityTooLarge, oversize.Error())
    return
}
```

This satisfies AC #3 ("returns this message verbatim"). The JSON
response shape is unchanged: `{"error": "…"}`. We pick up the
`errors` import — handlers.go already uses `fmt`, so no other change.

## Trade-offs

- *Re-marshal on the failure path* vs *plumbing JSON length through
  writeGLB*. We chose re-marshal: simpler, no signature churn, and
  the failure path is rare and already terminal.
- *`textureCount` from images vs textures.* Images is the right
  count because the storage cost is in the image payload, and SHA256
  dedup means `len(out.Images) <= len(out.Textures)`. We label it
  "textures" in the message because that's the term operators use.
- *`humanBytes` as a free function vs method on the error.* Free
  function in combine.go, package-private. No reason to export it;
  no other caller needs it today.

## Alternatives rejected

- *Embed the breakdown into the existing string error.* Loses the
  structured fields; callers can't programmatically branch on
  oversize vs other failures. The whole point of this ticket is
  type, not text.
- *Make the breakdown computation a separate exported function*
  (`PackBreakdownOf(out *gltfDoc, bin []byte) PackBreakdown`).
  Speculative — nothing else needs it. YAGNI.
- *Soft-warn at 80% of cap.* Explicitly out of scope per ticket.
