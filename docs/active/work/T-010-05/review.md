# T-010-05 Review — pack-size-cap

## Summary

Replaced the flat string error from `CombinePack`'s 5 MiB cap with a
typed `*PackOversizeError` carrying species, actual/limit bytes, and
a per-budget breakdown (texture count + bytes, mesh bytes, JSON
bytes). The HTTP layer now detects this error via `errors.As` and
echoes its formatted message verbatim in the 413 response.

## Files changed

| File | Lines | Change |
|---|---|---|
| `combine.go` | +~70 | Added `PackBreakdown`, `PackOversizeError`, `humanBytes`, `imageBytes` field on `mergeContext`, `strings` import. Replaced cap-check `fmt.Errorf` with structured error. Image-bytes accounting in `absorbImage`. |
| `handlers.go` | -5 / +4 | `handleBuildPack` 413 branch switched from `strings.Contains` to `errors.As(&PackOversizeError{})`; surfaces `oversize.Error()` verbatim. |
| `combine_test.go` | +35 / -3 | `TestCombine_SizeCapRejection` rewritten to assert error type, all fields, and rendered message contents. Added `errors` import. |

No new files. No public API removals. `CombinePack`'s signature is
unchanged.

## Acceptance criteria check

- ✅ `CombinePack` returns `*PackOversizeError` (not plain
  `errors.New`) with all four fields (`Species`, `ActualBytes`,
  `LimitBytes`, `Breakdown`).
- ✅ `Breakdown` has texture count, total texture bytes, mesh bytes,
  JSON bytes (named `TextureCount`, `TextureBytes`, `MeshBytes`,
  `JSONBytes`).
- ✅ `Error()` formatted layout matches the AC example: header with
  species and actual size, indented breakdown rows for textures /
  meshes / metadata, fixed `hint:` trailing line.
- ✅ HTTP endpoint (`handleBuildPack` in `handlers.go`) returns the
  message verbatim — `jsonError(w, 413, oversize.Error())`.
- ✅ Unit test verifies the typed error is returned, all fields are
  populated correctly, and the rendered message contains the
  species name, "5 MB limit", `meshes:`, `metadata:`, and the hint
  line.

## Test coverage

`go test ./...` — all packages green (`ok glb-optimizer 2.864s`).

`TestCombine_SizeCapRejection` exercises:
- `errors.As` detection
- Species propagation from `meta.Species`
- Exact `LimitBytes == 5*1024*1024`
- `ActualBytes > LimitBytes`
- Zero-texture branch (`TextureCount == 0`, "textures: none" in
  rendered message)
- `MeshBytes` covers the 6 MiB ballast bufferView
- `JSONBytes > 0`
- All required substrings in rendered `Error()`

`t.Logf` dumps the rendered message in verbose mode so future
contributors can eyeball the layout.

## Test gaps / open concerns

- *No handler-level test for the 413 path.* `handlers_*_test.go`
  files don't currently exercise `handleBuildPack` end-to-end at
  all (T-010-03 shipped without HTTP-layer tests). Adding one
  would require synthesizing oversized intermediates on disk, which
  is heavy; recommended as a follow-up under handler-test
  hardening rather than this ticket.
- *No test for the populated-textures branch* of `Error()`. A pack
  with `TextureCount > 0` would render
  `textures: N × avg X = Y MB` instead of `textures: none`. The
  branch is exercised in production but not under test. Cheap
  follow-up: a unit test that constructs a `PackOversizeError`
  literal with `TextureCount: 18, TextureBytes: 5_700_000` and
  string-checks the formatted output.
- *URI images contribute to `TextureCount` but not `TextureBytes`.*
  Documented in the design and the inline comment, but worth
  flagging: if a future bake stage starts emitting URI-only images,
  the breakdown becomes slightly misleading (the count and bytes
  diverge). No current pipeline does this.

## Intentional deviations from spec

- *Cap wording.* The cap constant is `5 * 1024 * 1024 = 5,242,880`
  (binary 5 MiB). Rendering this with `humanBytes` produces "5.2
  MB", not the AC's "5 MB". Hardcoded the literal "5 MB limit" in
  `Error()` to match the AC verbatim; the structured `LimitBytes`
  field remains exact for any programmatic consumer. Inline
  comment in `Error()` documents the discrepancy.
- *`textures:` row uses image count, not texture count.* In glTF,
  textures = (image, sampler) pairs. After SHA256 dedup the actual
  storage cost lives in `mc.out.Images`. We render
  `len(mc.out.Images)` as the "textures" number because that's
  what users care about (physical payloads, not refs). Documented
  in design.md.

## Risks / things to watch

- The `json.Marshal(mc.out)` re-marshal on the cap-failure path
  is wasted work, but the failure path is terminal and rare. If a
  profile ever shows it as hot, plumbing the JSON length out of
  `writeGLB` is the obvious next step.
- Anyone adding new BIN payload types (audio? animation?) needs to
  decide whether they fall under "meshes" or warrant a new
  breakdown bucket. Today they'd transparently roll into
  `MeshBytes`, which is acceptable.

## Out of scope (per ticket)

- Auto-shrinking textures
- Soft warnings under the limit
- Per-environment configurable limit
- The "5 MB" vs "5 MiB" reconciliation in code (separate concern;
  the constant stays binary, the user-facing message stays
  decimal)

## Recommendation

Ready to ship. No critical issues. The two test gaps above are
worthwhile follow-ups but not blockers — the code path is correct,
the typed-error contract is enforced by the existing test, and the
handler integration is a one-liner that's hard to get wrong.
