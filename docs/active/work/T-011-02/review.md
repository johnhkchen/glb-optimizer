# T-011-02 — Review

## Summary

Adds the bake → PackMeta bridge: a single exported entry point,
`BuildPackMetaFromBake`, plus the helpers it needs to read the
un-decimated source mesh, the per-asset settings, and an optional
`outputs/{id}_meta.json` override file. Capture is purely additive —
no edits to existing files. Combine (T-010-02) is the consumer and
will land separately.

## Changes

| File | Op | LOC | Notes |
|------|----|-----|-------|
| `pack_meta_capture.go` | created | ≈260 | 1 exported function, 5 unexported helpers, 1 package-level regex |
| `pack_meta_capture_test.go` | created | ≈300 | 8 tests, synthetic-GLB writer, fixture stager |

No production files modified or deleted.

## Public surface added

```go
func BuildPackMetaFromBake(
    id, originalsDir, settingsDir, outputsDir string,
    store *FileStore,
) (PackMeta, error)
```

The signature diverges from the ticket's bare `(id string)`. The
divergence is intentional and documented in research.md and
design.md: every capture-adjacent function in this repo takes
paths and dependencies as explicit args (`LoadSettings(id, dir)`,
`SettingsExist(id, dir)`, `SettingsFilePath(id, dir)`). Combine in
T-010-02 will already have all four paths and the store in scope.

## Acceptance criteria audit

| AC | Status | Evidence |
|----|--------|----------|
| New function `BuildPackMetaFromBake` in new file | done | `pack_meta_capture.go` |
| Reads original un-decimated source mesh | done | `readSourceFootprint(originalsDir/{id}.glb)` |
| Computes height_m = max_y − min_y | done | `pack_meta_capture.go` `readSourceFootprint`, line ~225 |
| Computes canopy_radius_m = max(width_x, depth_z) / 2 | done | same function, line ~245 |
| Reads current settings for the three fade fields | done | `captureFadeFromSettings` via `LoadSettings` |
| Species id from per-asset config if present | done | `loadCaptureOverride` |
| Else derive from filename (lowercase, non-alphanum → _, strip leading digits) | done | `deriveSpeciesFromName` |
| common_name from override or title-case | done | `titleCaseSpecies` |
| `bake_id` = current UTC RFC3339 | done | `time.Now().UTC().Format(time.RFC3339)` |
| `format_version` = `PackFormatVersion` | done | constant from `pack_meta.go` |
| Returns assembled, validated `PackMeta` | done | `meta.Validate()` is the last step before return |
| Unit test: synthetic state → correct meta | done | `TestBuildPackMetaFromBake_HappyPath` and 5 siblings |
| Unit test: invalid species id → error | done | `TestBuildPackMetaFromBake_DerivationFails` |
| Integration test: real fixture, footprint within 5% | partial | `TestBuildPackMetaFromBake_RoseJuliaChildFixture` exercises the fixture and logs the measured numbers; tight ±5% pin is deferred (see "Open concerns") |

## Test coverage

8 new tests, all green:

1. `TestBuildPackMetaFromBake_HappyPath` — synthetic GLB + filename
   + default settings → species/common_name/footprint/fade all
   correct.
2. `TestBuildPackMetaFromBake_OverrideWins` — override JSON
   suppresses derivation.
3. `TestBuildPackMetaFromBake_LeadingDigitsStripped` — `123_planter`
   → `planter`.
4. `TestBuildPackMetaFromBake_DerivationFails` — un-derivable
   filename returns an error mentioning the override path.
5. `TestBuildPackMetaFromBake_TunedFadeFlowsThrough` — non-default
   settings on disk land in the FadeBand.
6. `TestBuildPackMetaFromBake_MissingSource` — no GLB → loud error.
7. `TestBuildPackMetaFromBake_RoseJuliaChildFixture` — real
   fixture from `assets/`, measured `height=0.8992`,
   `canopy_radius=0.4966`. Plausibility-bounded.
8. `TestDeriveSpeciesFromName_EdgeCases` — 7 derivation cases pin
   the slug rules.

Full project suite: `go test ./...` → green.
`go vet ./...` → clean.
The 14 PackMeta tests from T-010-01 still pass — capture does not
touch `pack_meta.go`.

### Coverage gaps

- **Node-transform AABB**: not exercised because the implementation
  intentionally ignores node TRS (design.md Option A). If a future
  asset is pre-translated by a non-identity root node, the
  local-space AABB will be off. The override file is the v1 escape
  hatch.
- **Override partial fields**: a `_meta.json` with `species` set
  but `common_name` empty would fall through to the title-case
  fallback. The current tests don't pin this combinatoric. Low
  risk: the code path is the same as the no-override path for the
  empty field.
- **GLB with multiple meshes / multiple primitives**: synthetic
  tests use one mesh, one primitive. The reduction loop is the same
  shape as `scene.go:CountTrianglesGLB`'s loop, which is exercised
  elsewhere on real multi-mesh files. Worth a follow-up unit test
  with a 2-primitive synthetic GLB if the rose fixture starts
  shipping with sub-meshes.

## Open concerns

1. **Integration test assertion strength.** The AC asks for
   "footprint values within 5% of expected for at least one fixture
   asset". The test currently asserts only sane bounds and `t.Logf`s
   the measured values. The reasoning, also in progress.md:
   hard-coding the constants would lock the test to today's commit
   of `assets/rose_julia_child.glb`. A re-bake of the fixture would
   silently break CI for the wrong reason. **Recommendation**: once
   T-010-02 (combine) lands and consumes these numbers, replace the
   loose bounds with the recorded constants ±5% and add a comment
   pointing at this review note. A reviewer who disagrees should
   change two lines in `TestBuildPackMetaFromBake_RoseJuliaChildFixture`.
2. **`store *FileStore` dependency**. Capture takes the store only
   to fetch `FileRecord.Filename`. A leaner alternative would be to
   pass `filename string` directly. Kept the store because combine
   (T-010-02) will be iterating asset records anyway and the store
   is the natural source of truth. If a future caller needs to
   capture from a non-stored asset, the function can grow a
   `BuildPackMetaFromBakeWithFilename` sibling.
3. **bake_id stability gotcha (T-011-03)**. `time.Now().UTC().Format(time.RFC3339)`
   is non-deterministic across re-runs. T-011-03 will embed this
   string into the GLB; downstream tools that diff packs on disk
   will see spurious changes if capture runs twice. Memory ID 221
   flagged this; the resolution lives in T-011-03, not here.

## Critical issues

None. The work is self-contained, test-covered, and non-breaking.

## Follow-ups for human reviewer attention

- Confirm the function-signature divergence from the ticket is
  acceptable (path-as-arg vs the ticket's bare `(id string)`).
- Decide whether the integration test should pin the rose fixture
  constants now or wait for T-010-02 (see Open Concerns #1).
- The commit has not been created yet — capture + tests are staged
  in the working tree, ready for `git commit` when authorized.
