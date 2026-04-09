# T-010-05 Research — pack-size-cap

## Goal

Make the 5 MiB size-cap failure inside `CombinePack` ergonomic. Instead of
a flat `errors.New("…exceeds 5 MiB cap")`, return a structured
`*PackOversizeError` whose fields explain *why* the pack is oversized
(species, actual size, limit, and a breakdown of texture / mesh / JSON
bytes), and surface it verbatim in the HTTP layer's 413 response.

## Current state

### combine.go

The cap is enforced at the very end of `CombinePack`
(combine.go:715):

```go
raw, err := writeGLB(mc.out, mc.outBin.Bytes())
…
if len(raw) > packSizeCap {
    return nil, fmt.Errorf("combine: pack size %d exceeds 5 MiB cap", len(raw))
}
```

`packSizeCap` is `5 * 1024 * 1024` (combine.go:21). Note: the
acceptance criteria document the cap as "5 MB" in the user-facing
message, but the code uses MiB. We will keep the canonical value at
`5 * 1024 * 1024` (the existing constant; T-010-02 ships with this) and
*display* "5 MB" in the formatted error to match the ticket spec
verbatim. The displayed actual size will be rendered in MB
(`bytes / 1_000_000`) so the human-facing numbers remain consistent.

### Image bytes accounting

`mergeContext.absorbImage` (combine.go:357) is the only call site
that copies image payloads into `outBin`. Adding a single
`imageBytes int64` accumulator on `mergeContext` and incrementing it
in the "new image" branch gives us the texture-bytes breakdown for
free, with no second pass over the buffer.

### Mesh + JSON byte accounting

- *Mesh bytes:* every non-image bufferView ends up in `outBin` via
  `absorb`. The simplest decomposition is
  `meshBytes = totalBinBytes - imageBytes`. This sweeps in indices,
  positions, normals, UVs, and any other vertex attributes — anything
  that isn't an image. For the purposes of the error display this is
  the right grouping ("everything in BIN that isn't a texture").
- *JSON bytes:* `writeGLB` already marshals to `jsonRaw`. We can
  compute it inline at the cap-check site by re-marshalling `mc.out`
  (cheap relative to the merge work already done) and using its
  length. Alternatively `writeGLB` could return the JSON length, but
  that bloats its signature for one caller. We'll re-marshal locally.

### Texture *count* vs image *count*

The example in the ticket reads
`textures: 18 × avg 320 KB = 5.7 MB`. In glTF, textures and images
are separate arrays — a texture is a (image, sampler) pair. After
SHA256 dedup the storage cost is in `mc.out.Images`, not
`mc.out.Textures`. We render the count using `len(mc.out.Images)`
(the number of *physical* texture payloads) and label it "textures"
in the message because that's the term users recognize. The avg is
`imageBytes / imageCount`.

### handlers.go

`handleBuildPack` (handlers.go:1724) currently does:

```go
if strings.Contains(err.Error(), "5 MiB cap") {
    jsonError(w, http.StatusRequestEntityTooLarge,
        "pack exceeds 5 MB — reduce variant count … and re-bake")
    return
}
```

That string-match becomes brittle once the wrapping changes. We'll
switch to `errors.As(err, &oversize)` and write `oversize.Error()`
verbatim into the JSON body, satisfying AC #3 ("returns this message
verbatim in its 413 response").

### combine_test.go

`TestCombine_SizeCapRejection` (combine_test.go:444) already
constructs an oversized pack with a 6 MiB ballast bufferView and
asserts the error string contains "exceeds 5 MiB cap". We'll keep
the test (it's the only fixture that builds an oversized pack) and
extend it to assert:

1. `errors.As(err, &poe)` succeeds
2. `poe.Species == "achillea_millefolium"` (from `validCombineMeta`)
3. `poe.ActualBytes > poe.LimitBytes`
4. `poe.LimitBytes == 5 * 1024 * 1024`
5. `poe.Breakdown.TextureBytes == 0` (no images in this fixture)
6. `poe.Breakdown.MeshBytes >= 6 * 1024 * 1024` (the ballast)
7. `poe.Error()` contains the species name and the "hint:" line

## Constraints / non-goals

- *Out of scope* (per ticket): auto-shrink, soft warnings, per-env
  limits. The 5 MiB cap stays hard.
- The text rendering must match the ticket's example layout (3
  indented breakdown rows + a `hint:` trailing line) closely enough
  that operators reading server logs recognize the format.
- We must not change `CombinePack`'s signature — the dist/plants
  handler already calls it with `(side, tilted, volumetric, meta)`.

## Open questions resolved during research

- *Q:* Should `PackOversizeError` wrap a cause? *A:* No — the error
  is generated inside `CombinePack` itself, there's nothing to wrap.
  It does need to satisfy the `error` interface (obviously) and be
  detectable via `errors.As`.
- *Q:* Where does Species come from at the cap-check site? *A:* From
  `meta.Species` — already validated via `meta.Validate()` earlier in
  `CombinePack`.
