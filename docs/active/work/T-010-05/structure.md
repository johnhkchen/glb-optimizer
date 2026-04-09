# T-010-05 Structure — pack-size-cap

## Files touched

| File | Change |
|---|---|
| `combine.go` | Add `PackOversizeError` + `PackBreakdown` types, `humanBytes` helper, `imageBytes` field on `mergeContext`, return new error type from `CombinePack`. |
| `combine_test.go` | Replace string-only assertion in `TestCombine_SizeCapRejection` with full type / fields / formatted-message check. |
| `handlers.go` | Switch `handleBuildPack`'s 413 branch from `strings.Contains` to `errors.As(err, &PackOversizeError{})`; respond with `oversize.Error()` verbatim. |

No new files. No package boundary changes. No new imports beyond
`errors` in `handlers.go` (combine.go already uses `fmt`,
`encoding/json`).

## Type surface (combine.go)

```go
type PackBreakdown struct {
    TextureCount int
    TextureBytes int64
    MeshBytes    int64
    JSONBytes    int64
}

type PackOversizeError struct {
    Species     string
    ActualBytes int64
    LimitBytes  int64
    Breakdown   PackBreakdown
}

func (e *PackOversizeError) Error() string
```

`humanBytes(n int64) string` — package-private helper, picks units
(B / KB / MB) and renders with one fractional digit for MB, zero
otherwise.

## Mutation points

### combine.go: `mergeContext`

```go
type mergeContext struct {
    out         *gltfDoc
    outBin      *bytes.Buffer
    imageHashes map[[32]byte]int
    imageBytes  int64 // NEW
}
```

`newMergeContext` does not need to set this — zero value is correct.

### combine.go: `absorbImage`

After the dedup branch returns early (existing image found), the new
counter is *not* incremented. In the new-image branch, after the
`payload` is resolved but before the bufferView/URI store decision:

```go
mc.imageBytes += int64(len(payload))
```

Place this *inside* the `if img.BufferView != nil` branch only —
URI-only images don't add bytes to BIN, only to the JSON manifest.
The breakdown's MeshBytes accounting depends on this: only payloads
that *land in BIN* contribute to imageBytes, so
`MeshBytes = BIN.len() - imageBytes` stays accurate when URI images
exist.

### combine.go: cap-check site (end of CombinePack)

Replace:

```go
if len(raw) > packSizeCap {
    return nil, fmt.Errorf("combine: pack size %d exceeds 5 MiB cap", len(raw))
}
```

with:

```go
if len(raw) > packSizeCap {
    jsonRaw, _ := json.Marshal(mc.out)
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

### handlers.go: `handleBuildPack`

Add `"errors"` to the import block. Replace the existing
string-match block (handlers.go:1788) with:

```go
var oversize *PackOversizeError
if errors.As(err, &oversize) {
    jsonError(w, http.StatusRequestEntityTooLarge, oversize.Error())
    return
}
```

### combine_test.go: `TestCombine_SizeCapRejection`

Replace the existing single-line string assertion with:

```go
var poe *PackOversizeError
if !errors.As(err, &poe) {
    t.Fatalf("want *PackOversizeError, got %T: %v", err, err)
}
if poe.Species != "achillea_millefolium" { … }
if poe.LimitBytes != 5*1024*1024 { … }
if poe.ActualBytes <= poe.LimitBytes { … }
if poe.Breakdown.TextureCount != 0 { … }
if poe.Breakdown.MeshBytes < 6*1024*1024 { … }
msg := poe.Error()
for _, want := range []string{
    `pack "achillea_millefolium"`, "exceeds 5 MB limit",
    "meshes:", "metadata:", "hint:",
} {
    if !strings.Contains(msg, want) { … }
}
```

Add `"errors"` to combine_test.go imports.

## Risk surface

- *Existing test compatibility:* `TestCombine_SizeCapRejection`
  currently asserts the string "exceeds 5 MiB cap". After the
  change, the new error's `Error()` says "exceeds 5 MB limit"
  (decimal MB, matching the AC). The test must be updated; no other
  test depends on the old string. Confirmed via grep — no other
  callers in handlers/etc. assert against `5 MiB cap` (the old
  handlers.go path checked for `5 MiB cap` and we're replacing that
  path simultaneously).
- *Breakdown rounding:* `humanBytes` rounds for display only. The
  underlying `int64` fields stay exact for any future structured
  consumer.
- *URI images:* counted in `TextureCount` but not `TextureBytes`.
  Acceptable — no current pipeline produces URI-only images, and the
  breakdown is informational.
