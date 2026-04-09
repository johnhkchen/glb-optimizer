# T-011-03 — Review

## What changed

| File | Op | Lines | Purpose |
|---|---|---|---|
| `bake_stamp.go` | new | ~70 | `bakeStamp` type, atomic writer, missing-tolerant reader |
| `bake_stamp_test.go` | new | ~110 | round-trip, format, missing, malformed, overwrite |
| `pack_meta_capture.go` | edit | +20 | `resolveBakeID`, swap inline `time.Now()` for it, add `log` import |
| `pack_meta_capture_test.go` | edit | +85 | stability test, fallback warning test, `silenceLog` helper, `silenceLog` calls in 5 existing tests |
| `handlers.go` | edit | +30 | `handleBakeComplete` |
| `main.go` | edit | +1 | route registration |
| `handlers_bake_complete_test.go` | new | ~120 | happy / 404 / 405 / overwrite-on-rebake |
| `static/app.js` | edit | +5 | `await fetch('/api/bake-complete/${id}')` after volumetric upload |

No edits to `pack_meta.go`, `combine.go`, `models.go`, `settings.go`,
or any other unrelated source.

## How the AC is satisfied

| Acceptance criterion | Implementation |
|---|---|
| `bake_id` is set when the **bake** completes, NOT when combine runs | `static/app.js generateProductionAsset` POSTs to `/api/bake-complete/:id` after the third (volumetric) upload returns ok. The combine path is now read-only with respect to `bake_id`. |
| `outputs/{id}_bake.json` contains `{ "bake_id": "<RFC3339 UTC>", "completed_at": "..." }` | `WriteBakeStamp` captures one `time.Now().UTC().Format(time.RFC3339)` value, populates both fields, marshals indented, writes via `os.WriteFile` to a `.tmp` file, then `os.Rename` to the final path. |
| `BuildPackMetaFromBake` reads `bake_id` from this file | `resolveBakeID(id, outputsDir)` calls `ReadBakeStamp`. If the stamp's `BakeID` is non-empty it is returned verbatim — no clock involved. |
| Missing file → fallback to `time.Now()` + warning | `ReadBakeStamp` returns `(zero, nil)` on `os.IsNotExist`. `resolveBakeID` checks the empty string, calls `log.Printf("pack_meta_capture: %s: no bake stamp at %s, falling back to current time as bake_id; rebake to get a stable id", ...)`, and falls back to `time.Now().UTC().Format(time.RFC3339)`. |
| Unit test: combining the same intermediates twice produces the same `bake_id` | `TestBuildPackMetaFromBake_StableBakeID` stages a fixed `_bake.json`, calls `BuildPackMetaFromBake` twice, asserts both `meta.BakeID` values equal the staged string. |

## Test coverage

```
go test ./...   →   ok (full suite)
```

Tests added or touched in this ticket:

- `bake_stamp_test.go`: 5 tests covering write, read, missing,
  malformed, overwrite.
- `pack_meta_capture_test.go`:
  - 1 new test for stability (`StableBakeID`) — the AC lock.
  - 1 new test for the fallback warning (`MissingStampLogsWarning`).
  - 5 existing tests updated to call `silenceLog(t)` so the
    intentional fallback warning does not pollute test output.
- `handlers_bake_complete_test.go`: 4 tests covering 200 / 404 /
  405 / overwrite-on-rebake.

**Coverage gaps deliberately left open:**

- No JS unit test for the `generateProductionAsset` fetch call.
  This file is exercised manually in the dev server and has no
  existing JS test harness; adding one would be out of scope.
  Manual smoke procedure documented in `plan.md` Step 5.
- No end-to-end test that takes a real bake → real combine → reads
  the resulting `dist/plants/*.glb` and asserts the embedded
  `bake_id` matches the on-disk stamp. The combine step itself is
  T-010-02 territory and not yet wired up; the AC unit test
  exercises the contract at the `BuildPackMetaFromBake` boundary,
  which is the right seam for this ticket.

## Behaviour and design notes for the reviewer

1. **Set-once-per-bake.** The stamp is rewritten only when
   `generateProductionAsset` runs end to end. Standalone devtools
   rebakes of the side, tilted, or volumetric layers do **not**
   touch `_bake.json`. This is intentional: the asset server cares
   about the *full bake's* identity, not partial layers. If the
   user re-runs only one layer, the stamp is stale relative to that
   layer but stable across combines, which is the property the
   ticket asked for.
2. **Atomic writes.** `WriteBakeStamp` uses temp+rename so a
   concurrent reader (e.g., a combine racing the bake driver) sees
   either the old file or the complete new one, never a partial.
3. **Fallback behaviour.** If the stamp file is missing
   (pre-existing intermediates baked before this ticket), capture
   logs a warning **and** mints a fresh `time.Now()` id. The demo
   does not hard-fail on stale state. Operators see the warning and
   can choose to rebake.
4. **Malformed stamp = hard error.** A non-JSON stamp is propagated
   as an error from `ReadBakeStamp` → `resolveBakeID` →
   `BuildPackMetaFromBake`. Combine fails loudly. Silently masking
   it would let a typo in the file ship a pack with a clock-derived
   id even though the operator thought they had a stable one.
5. **Time field semantics.** `bake_id` and `completed_at` hold the
   *same* RFC3339 UTC value at write time, captured by a single
   `time.Now()` call. They are separate JSON keys because they
   represent conceptually distinct facts ("identity" vs "moment");
   the consumer or asset server may eventually display
   `completed_at` even though `bake_id` is the cache key.

## Open concerns

None blocking. Two minor notes for follow-up:

- **Hash-based ids (E-002 follow-up).** Per the ticket's Out of
  Scope section, this implementation uses a wall-clock string. When
  the asset server lands and the cache-busting story matures, the
  natural upgrade is `bake_id = sha256(billboard ++ tilted ++
  volumetric)[:N]`. The contract change is trivial — the field is
  already a free-form string. The bake driver call site is the
  only place that needs to change, which is exactly the point of
  centralising the stamp through `WriteBakeStamp`.
- **JS error handling.** The new `await fetch(...)` is inside the
  existing `try/catch`. If the POST 500s the bake is reported as
  failed (`success = false`) and the operator sees a console error.
  The intermediates on disk remain valid; the next bake re-runs the
  whole flow. Acceptable for the demo.

## Files reviewed for accidental scope creep

- `pack_meta.go` — untouched. ✅
- `combine.go` — untouched. ✅
- `models.go`, `handlers.go` (other handlers), `main.go` (other
  routes) — only the targeted additions, nothing else moved. ✅
- `static/app.js` — single 5-line addition inside
  `generateProductionAsset`; no other JS code changed. ✅

## Handoff

Ticket is complete and ready for human review. The ticket frontmatter
has not been touched — Lisa will detect the artifacts and advance the
phase automatically.
