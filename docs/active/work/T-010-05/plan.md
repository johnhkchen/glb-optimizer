# T-010-05 Plan — pack-size-cap

Five sequential edits, each verified by `go build ./... && go vet
./...` and the relevant test runs.

## Step 1 — Add types and helper to combine.go

In `combine.go`, after the `mergeContext` definition, append:

- `PackBreakdown` struct (4 fields: TextureCount int, TextureBytes
  int64, MeshBytes int64, JSONBytes int64)
- `PackOversizeError` struct (Species, ActualBytes, LimitBytes,
  Breakdown)
- `(*PackOversizeError).Error() string` — implements the layout
  documented in `design.md`
- `humanBytes(n int64) string` — picks B / KB / MB

Add `imageBytes int64` field to `mergeContext`.

Verify: `go build ./...` succeeds (types compile, no caller yet).

## Step 2 — Wire breakdown accounting into absorbImage

In `combine.go`'s `absorbImage`, inside the `if img.BufferView != nil`
storage branch (just before / after `mc.appendBytes(payload)`), add:

```go
mc.imageBytes += int64(len(payload))
```

Place it where it executes only on the *new* image path — after the
hash dedup early-return, inside the bufferView branch (URI images
contribute to JSON, not BIN, so we exclude them from the BIN-derived
breakdown).

Verify: `go test ./... -run TestCombine_ImageDedup` still green
(deduped images don't double-count).

## Step 3 — Replace cap-check error in CombinePack

In `combine.go`, at the end of `CombinePack`, replace the
`fmt.Errorf("combine: pack size %d exceeds 5 MiB cap", …)` with the
`*PackOversizeError` construction documented in `structure.md`. Use
`json.Marshal(mc.out)` for the JSONBytes field (the marshal cost is
acceptable on a terminal failure path).

Verify: `go build ./...`. Existing
`TestCombine_SizeCapRejection` will fail at this point because the
string changed — that's expected and fixed in Step 5.

## Step 4 — Switch handler to errors.As

In `handlers.go`:

1. Add `"errors"` to the import block (it may already be present —
   verify first).
2. In `handleBuildPack`, replace the
   `if strings.Contains(err.Error(), "5 MiB cap")` block with the
   `errors.As(err, &oversize)` construction from `structure.md`,
   passing `oversize.Error()` to `jsonError`.

Verify: `go build ./...` succeeds.

## Step 5 — Update TestCombine_SizeCapRejection

In `combine_test.go`:

1. Add `"errors"` to imports.
2. Replace the single-line `strings.Contains` assertion with the
   typed-error assertion block from `structure.md`: `errors.As`,
   field checks (Species, LimitBytes, ActualBytes, Breakdown), and
   substring checks against the rendered `poe.Error()`.

Verify: `go test ./... -run TestCombine_SizeCapRejection -v` passes.
Then `go test ./...` to make sure nothing else regressed.

## Final validation

- `go build ./...`
- `go vet ./...`
- `go test ./...` — all suites green
- Manual sanity: print `poe.Error()` from the test in verbose mode
  (`t.Logf`) once to eyeball the formatted layout matches the AC.

## Out of scope (do not touch)

- handler tests in `handlers_*_test.go` — none currently exercise
  the 413 path; adding one is its own follow-up ticket.
- The `dist/plants/` directory creation behavior in handler.
- The `humanBytes` helper's behavior on negative or zero inputs —
  caller never produces them.
