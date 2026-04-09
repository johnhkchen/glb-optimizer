# T-010-05 Progress — pack-size-cap

Plan executed in five steps as designed. One mid-flight tweak after
running tests.

## Step-by-step

### Step 1 — Types and helper added (combine.go)

- Added `imageBytes int64` to `mergeContext`.
- Added `PackBreakdown` and `PackOversizeError` exported types.
- Added `(*PackOversizeError).Error()` rendering the AC layout.
- Added `humanBytes(int64) string` helper for B / KB / MB display.
- Added `"strings"` to combine.go imports (used by `strings.Builder`
  in `Error()`).
- `go build ./...` clean.

### Step 2 — Image bytes accounting in absorbImage

- Inserted `mc.imageBytes += int64(len(payload))` inside the
  `BufferView != nil` storage branch — *only* on the new-image path
  (after dedup early-return), so deduped images stay at zero and
  URI-only images don't disturb the BIN-derived `MeshBytes`
  accounting.
- `TestCombine_ImageDedup` still passes.

### Step 3 — Cap-check returns *PackOversizeError

- Replaced the `fmt.Errorf(...exceeds 5 MiB cap)` with a populated
  `*PackOversizeError`.
- Re-marshals `mc.out` once for the JSONBytes field — wasted on the
  failure path but avoids signature churn elsewhere.

### Step 4 — Handler switched to errors.As

- `handleBuildPack` (handlers.go) now does
  `errors.As(err, &oversize)` and writes `oversize.Error()`
  verbatim. The `errors` package was already imported.
- The hard-coded "pack exceeds 5 MB — reduce variant count …"
  string is gone; the formatted breakdown takes its place.

### Step 5 — Test rewritten

- `TestCombine_SizeCapRejection` now asserts:
  - `errors.As` succeeds
  - Species, LimitBytes, ActualBytes correct
  - `Breakdown.TextureCount == 0`, `TextureBytes == 0`
  - `Breakdown.MeshBytes >= ballastLen` (6 MiB)
  - `Breakdown.JSONBytes > 0`
  - Rendered `Error()` contains the species, "exceeds 5 MB limit",
    `meshes:`, `metadata:`, and the `hint:` line
- `t.Logf` dumps the rendered message in verbose mode for
  eyeballing.

## Mid-flight correction

The first test run failed because the rendered limit read "5.2 MB"
(`5*1024*1024 = 5,242,880` rounds to 5.2 in decimal MB). The ticket
AC explicitly says "5 MB limit". Two paths:

1. Display `humanBytes(e.LimitBytes)` and accept "5.2 MB"
2. Hardcode "5 MB limit" in the format string

Chose (2): the cap is a fixed constant and the AC wording is fixed.
The structured `LimitBytes` field remains exact for any future
programmatic consumer. Comment in `Error()` documents the
intentional discrepancy.

## Final test run

```
$ go test ./...
ok  	glb-optimizer	2.864s
```

Sample rendered message from test verbose output:

```
pack "achillea_millefolium" exceeds 5 MB limit (actual: 6.3 MB)
  textures:    none
  meshes:      6.3 MB
  metadata:    887 B
hint: reduce billboard texture resolution or variant count and re-bake
```

Layout matches the ticket example (3 indented breakdown rows + hint
trailer). The "textures: none" branch fires here because the test
fixture has no images; production packs will hit the
`N × avg X = Y MB` branch.

## Files modified

- `combine.go` — types, helper, mergeContext field, accounting,
  cap-check error, `strings` import
- `handlers.go` — `handleBuildPack` 413 branch
- `combine_test.go` — `TestCombine_SizeCapRejection` rewritten,
  `errors` import added
