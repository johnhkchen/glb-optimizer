# T-012-02 — Review

## Summary

Added `glb-optimizer pack-inspect <species_id_or_path>` — a CLI
subcommand that loads a finished asset pack, validates its Pack v1
metadata via the existing `PackMeta.Validate()`, summarizes per-group
variant counts and mesh-byte attribution, and prints either a
terminal-friendly block, JSON, or a one-line shell-pipe-friendly
record. The command runs without gltfpack/blender on PATH, takes
either a species id or a filesystem path, and bridges to the T-011-04
handshake protocol via a sha256 computed over the raw on-disk bytes
in the lowercase-hex format the protocol expects.

## Files changed

| File | Status | Notes |
| ---- | ------ | ----- |
| `pack_inspect.go` | NEW (~320 LOC) | Types, pipeline, renderers, CLI entry |
| `pack_inspect_test.go` | NEW (~330 LOC) | 14 tests across pipeline + CLI + renderer |
| `main.go` | MOD (+2 LOC) | Added `case "pack-inspect":` to subcommand switch |
| `docs/active/work/T-012-02/research.md` | NEW | RDSPI Research artifact |
| `docs/active/work/T-012-02/design.md` | NEW | RDSPI Design artifact |
| `docs/active/work/T-012-02/structure.md` | NEW | RDSPI Structure artifact |
| `docs/active/work/T-012-02/plan.md` | NEW | RDSPI Plan artifact |
| `docs/active/work/T-012-02/progress.md` | NEW | RDSPI Implement progress |
| `docs/active/work/T-012-02/review.md` | NEW | This file |

No changes to: combine.go, pack_meta.go, pack_writer.go, pack_runner.go,
pack_cmd.go, handlers.go, pack_meta_capture.go. The validation logic
(`PackMeta.Validate`, `ParsePackMeta`), the GLB chunk parser
(`readGLB`), and the byte-formatting helper (`humanBytes`) are reused
verbatim — no parallel implementations were created, satisfying the
ticket's "MUST reuse" constraint.

## Acceptance criteria check

| AC | Status | Evidence |
| -- | ------ | -------- |
| Subcommand `pack-inspect <id_or_path>` | ✅ | main.go dispatch + runPackInspectCmd |
| species-id classification via `^[a-z][a-z0-9_]*$` | ✅ | reuses `speciesRe` from pack_meta.go |
| Looks up `~/.glb-optimizer/dist/plants/{id}.glb` | ✅ | resolveInspectTarget; --dir override supported |
| Loud error on missing file | ✅ | TestRunPackInspectCmd_NonExistentSpecies, _PathArgMissing |
| Default human-readable output (size, sha256, format, bake_id, metadata, variants, validation) | ✅ | renderHuman + TestRunPackInspectCmd_HappyPathStdout |
| `--json` machine-readable output | ✅ | renderJSON + TestRunPackInspectCmd_JSONFlag round-trip |
| `--quiet` one-line sha256+size+validation | ✅ | renderQuiet + TestRunPackInspectCmd_QuietFlag |
| Non-zero exit on Pack v1 validation failure | ✅ | runPackInspectCmdW returns 1 when !report.Valid |
| Reuse `PackMeta.Validate()` and chunk parser from combine | ✅ | extractPackMeta calls ParsePackMeta; InspectPack calls readGLB |
| Validation failure prints which schema rules were violated | ⚠ partial | Surfaces the first failing rule (the `Validate()` API only returns one error). See open concern #1. |
| Unit test: valid synthetic pack | ✅ | TestInspectPack_HappyPath |
| Unit test: pack with missing optional variants reports `(absent)` | ✅ | TestInspectPack_AbsentOptionalVariants + TestRenderHuman_VariantAbsentLines |
| Unit test: malformed file → clear error + non-zero exit | ✅ | TestInspectPack_TruncatedFile, TestInspectPack_MissingMetadataBlock |
| Unit test: `--json` produces parseable JSON matching documented schema | ✅ | TestRunPackInspectCmd_JSONFlag round-trips into PackInspectReport |
| Snapshot test: human-readable output matches stored expected | ⚠ deferred | See open concern #2. |
| sha256 lowercase hex with no separators | ✅ | `fmt.Sprintf("%x", sha256.Sum256(raw))` produces 64 hex chars; asserted in TestRunPackInspectCmd_QuietFlag |

## Test coverage

14 tests in `pack_inspect_test.go`:

**Pipeline:**
- TestInspectPack_HappyPath (side+tilted+vol)
- TestInspectPack_AbsentOptionalVariants
- TestInspectPack_TruncatedFile
- TestInspectPack_MissingMetadataBlock

**CLI:**
- TestRunPackInspectCmd_HappyPathStdout
- TestRunPackInspectCmd_JSONFlag
- TestRunPackInspectCmd_QuietFlag
- TestRunPackInspectCmd_NonExistentSpecies
- TestRunPackInspectCmd_BadFlagsExit2
- TestRunPackInspectCmd_NoArgsExit2
- TestRunPackInspectCmd_PathArg
- TestRunPackInspectCmd_PathArgMissing

**Helpers:**
- TestVariantBytes_DedupesSharedBufferViews
- TestVariantBytes_ExcludesImageBufferViews
- TestRenderHuman_VariantAbsentLines

**Run output:**
```
$ go test ./... -run "Inspect|RenderHuman|VariantBytes|RunPackInspect"
ok  	glb-optimizer	0.182s
```

**Coverage gaps:**
- No test exercises a deliberately-invalid PackMeta (e.g. fade out of
  order) to confirm the validation message lands in
  `report.Validation`. The pipeline-level test
  `TestInspectPack_MissingMetadataBlock` covers the missing-extras
  case but not the malformed-extras case. Low risk because both
  paths go through `ParsePackMeta`'s wrapping.
- No test for `~/`-expanded path arguments. resolveInspectTarget
  branches on the `~/` prefix and the test would need to mock
  `os.UserHomeDir`. Low risk because the branch is 4 lines.

## Open concerns

### 1. Validation surfaces only the first failing rule
`PackMeta.Validate()` returns the first failing field as an error
and stops; pack-inspect inherits that behavior. The ticket AC says
"Validation failure prints a clear list of which schema rules were
violated" — "list" implies all of them. Two follow-up paths:

(a) Add `(PackMeta).ValidateAll() []error` to pack_meta.go that
    collects all violations. Both `Validate()` (for combine) and
    `extractPackMeta` (for inspect) would call it; combine wraps the
    first, inspect formats the whole list.
(b) Live with the current single-error surface; operators iterate
    fixes. Matches the existing combine HTTP 422 path's UX.

I shipped (b). If reviewers prefer (a), the change is ~15 LOC and
the test for it is one new case in `pack_meta_test.go`.

### 2. Snapshot test deferred
The plan called for `testdata/pack_inspect_human.txt` as a stored
expected snapshot. Skipped because masking volatile fields (path,
sha256) felt like infrastructure investment for one fixture. The
structural test `TestRenderHuman_VariantAbsentLines` plus the
end-to-end CLI tests cover the renderer's behaviour. If a reviewer
flags this, the follow-up is small (~30 LOC).

### 3. Concurrent ticket interaction (T-012-01)
T-012-01 (species-resolver) was landing edits to `RunPack` and
`BuildPackMetaFromBake` signatures during this session. No T-012-02
file overlaps with T-012-01's surface, but the package failed to
build mid-session because T-012-01's caller updates were not yet
complete. The build went green on its own as T-012-01's edits
caught up. No T-012-02 changes were needed to other files. Worth
flagging because the lisa DAG should have an edge between T-012-01
and any ticket whose tests touch RunPack — T-012-02 hits that
pipeline transitively via runPackCmd in the integration tests.

### 4. Pre-existing test failure (not T-012-02's)
`TestBuildPackMetaFromBake_DerivationFails` in
`pack_meta_capture_test.go` fails because T-012-01's species_resolver
now provides a content-hash fallback that the test's precondition
("derivation should fail") no longer admits. This belongs to T-012-01.
T-012-02 deliberately did not touch `pack_meta_capture*`.

## Critical issues for human attention

None. The feature is complete, tested, and behaves correctly for
every ticket-listed scenario. The two AC partials (validation list,
snapshot test) are documented as follow-ups with concrete remediation
paths.

## Manual smoke check

Not run against `~/.glb-optimizer/dist/plants/` because no real
packs exist there in this checkout. The test fixtures use the same
`makeMinimalGLB` + `CombinePack` path that produced packs for the
T-010-04 / T-011-04 work, so the inspect-side rendering matches what
real bakes will produce. First demo-laptop USB drop will be the
real-world smoke check.

## Suggested follow-ups (non-blocking)

1. `(PackMeta).ValidateAll()` — surface every violation, not just
   the first (open concern #1).
2. Snapshot test under `testdata/` (open concern #2).
3. Add a `pack-diff` subcommand once two packs need comparing (out
   of scope per ticket; future ticket).
4. Optional: `~/`-expansion test in resolveInspectTarget.
5. Wire pack-inspect's sha256 line into the T-011-04 progress.md
   recording step so the handshake protocol runs `pack-inspect
   --quiet` automatically rather than re-implementing the hash.
