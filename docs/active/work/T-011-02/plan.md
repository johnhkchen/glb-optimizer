# T-011-02 — Plan

## Step 1 — Create `pack_meta_capture.go` skeleton

Write the file with:

- Package declaration + file-level doc comment.
- Imports: `encoding/json`, `fmt`, `math`, `os`, `path/filepath`,
  `regexp`, `strings`, `time`.
- The `captureOverride` struct.
- A package-level `nonAlnumRe` for the slug derivation
  (`[^a-z0-9_]+`).
- Empty function bodies for the helpers and `BuildPackMetaFromBake`,
  each with the doc comment from structure.md.

Verification: `go build ./...` clean.

## Step 2 — Implement `readSourceFootprint`

Port the GLB JSON-chunk reader from `scene.go:CountTrianglesGLB`.
Walk every primitive's POSITION accessor, validate it has 3-component
min/max, reduce component-wise, derive footprint, error on degenerate.

Verification: write the synthetic GLB test helper +
`TestBuildPackMetaFromBake_HappyPath` first (TDD), then implement
until it passes. `go test -run TestBuildPackMetaFromBake_HappyPath ./...`
green.

## Step 3 — Implement `loadCaptureOverride`

`os.Open` → `os.IsNotExist` returns zero + nil. Otherwise
`json.Unmarshal`. No semantic validation here — final
`PackMeta.Validate` catches malformed species ids.

Verification: `TestBuildPackMetaFromBake_OverrideWins` passes.

## Step 4 — Implement `deriveSpeciesFromName` + `titleCaseSpecies`

Algorithm laid out in design.md:

1. Strip extension (`strings.TrimSuffix` for known `.glb`/`.gltf`
   would be too narrow — use `path.Ext`/`strings.TrimSuffix(name, ext)`).
2. Lowercase.
3. `nonAlnumRe.ReplaceAllString(s, "_")`.
4. Strip leading non-letters: while first byte is not in `[a-z]`, drop it.
5. Strip trailing `_`.
6. Collapse `__+` → `_`.
7. Return result (may be `""`; caller handles).

`titleCaseSpecies`: split on `_`, ASCII-uppercase first byte of each
non-empty word, join with space.

Verification: `TestBuildPackMetaFromBake_LeadingDigitsStripped` and
`TestBuildPackMetaFromBake_DerivationFails` pass.

## Step 5 — Implement `captureFadeFromSettings`

`LoadSettings(id, settingsDir)` → already returns defaults on missing.
Project the three fields into a `FadeBand`. Don't call
`AssetSettings.Validate` — the consumer of `FadeBand` is
`PackMeta.Validate` which independently enforces the [0,1] + ordering
constraints, so a corrupt settings file surfaces as a PackMeta
validation error with the right field names.

Verification: `TestBuildPackMetaFromBake_TunedFadeFlowsThrough` passes.

## Step 6 — Wire `BuildPackMetaFromBake`

Orchestrate per the structure.md flow. Look up `FileRecord.Filename`
from the store; fall back to `id` when missing or equal to
`{id}.glb`. Assemble the PackMeta, run `Validate`, return.

Verification: `TestBuildPackMetaFromBake_MissingSource` passes.

## Step 7 — Add the integration test

Use `assets/rose_julia_child.glb`. Measure the footprint constants
once by running `BuildPackMetaFromBake` against the real fixture in
a throwaway invocation, hard-code the values into the test with a
`±5%` assertion and a comment recording the date and the procedure.

Verification: `go test ./...` green; full suite (already 14 PackMeta
tests + everything else) still green.

## Step 8 — Run `go vet ./...` and `go test ./...`

End-to-end clean. Memory ID 257 confirms the existing suite is
green; keep it that way.

## Step 9 — Write `progress.md`, then `review.md`

Capture deviations, test counts, and follow-ups for T-010-02 (the
combine step that consumes this).

## Commit strategy

Single atomic commit at the end of Step 7 with subject
`T-011-02: BuildPackMetaFromBake + tests`. The work is small enough
that splitting buys nothing — capture and tests evolve together. No
commits during planning artifacts (they're docs and can ride the
implementation commit if the user later decides to bundle them).

## Out-of-plan / explicit non-actions

- Do **not** edit `pack_meta.go`. The schema is frozen.
- Do **not** edit `handlers.go` or `main.go`. Combine wiring is T-010-02.
- Do **not** add a `pack.go` file. Capture lives in
  `pack_meta_capture.go`.
- Do **not** introduce `golang.org/x/text`. Manual title-casing only.
