# T-011-03 — Design

## Decision

**Add a new endpoint `POST /api/bake-complete/:id` that writes
`{outputsDir}/{id}_bake.json`.** The JS bake driver
(`generateProductionAsset`) calls it once after all three uploads
succeed. `BuildPackMetaFromBake` reads the file if present and falls
back to `time.Now()` with a warning otherwise.

The bake-id payload is fixed by the AC:

```json
{
  "bake_id": "2026-04-08T19:14:07Z",
  "completed_at": "2026-04-08T19:14:07Z"
}
```

Both fields hold the same RFC3339 UTC timestamp at write time. They
are *separate fields* even though they currently coincide, because
"the id" and "when the bake finished" are distinct facts and the
consumer/asset-server may eventually want to display the latter.

## Options considered

### Option A — New `/api/bake-complete/:id` endpoint *(chosen)*

JS calls it once at the end of `generateProductionAsset` after all
three uploads return ok. Handler writes `{id}_bake.json` atomically.
`BuildPackMetaFromBake` reads it.

**Pros:**
- The endpoint name maps 1:1 to the AC ("when 'Build hybrid impostor'
  finishes, the bake driver writes ...").
- Single, unambiguous trigger point. Devtools tilted/volumetric
  rebakes do *not* fire it, so the id is not invalidated by ad-hoc
  re-renders.
- Mirrors the existing upload-handler shape (Method check, store
  lookup, write, JSON response). No new architectural surface.
- The handler is idempotent: re-clicking "Build hybrid impostor"
  rewrites both fields, which is exactly the desired semantics.
- Easy to test in isolation (`httptest.NewRequest`).

**Cons:**
- One extra round-trip per bake. Negligible (single small JSON,
  localhost) and worth it for the clarity.

### Option B — Stamp from `handleUploadVolumetric`

Volumetric is the last upload phase, so writing `{id}_bake.json` from
that handler would also catch the production case.

**Rejected** because:
- The same handler is reachable from devtools and from any future
  code path that re-uploads volumetric independently. Each such call
  would mint a new `bake_id` even though the side and tilted
  intermediates are unchanged. That breaks the exact stability
  property this ticket exists to defend.
- Couples a generic upload endpoint to bake-driver semantics it has
  no business knowing about.

### Option C — Stamp from all three upload handlers (last writer wins)

Each upload handler writes the file. Since they execute in order,
the volumetric one writes last and "the bake just finished" is the
implicit semantics.

**Rejected** for the same reason as B (devtools rebakes mint new
ids), plus the additional cost of three writes per bake and the
ambiguity about what an interleaved upload sequence does.

### Option D — Generate `bake_id` lazily in combine, but persist on first observation

`BuildPackMetaFromBake` mints `time.Now()` when it doesn't find
`{id}_bake.json`, *and* writes the file so subsequent combines reuse
the value.

**Rejected** because the semantics are wrong: the id then represents
"when combine first ran on this asset," not "when the bake finished."
Two demo machines combining the same intermediates would mint
different ids. Worse, the side effect makes `BuildPackMetaFromBake`
non-pure, which complicates testing.

### Option E — Hash the intermediates

Compute `bake_id = sha256(billboard ++ tilted ++ volumetric)[:16]`.
True content-addressed identity, no clock at all.

**Rejected — explicitly out of scope** in the ticket. Worth
revisiting post-demo when the asset server lands.

## Why option A wins on the AC checklist

| AC | How A satisfies it |
|---|---|
| `bake_id` set when the bake completes | The endpoint is called *after* the third upload returns ok. |
| `outputs/{id}_bake.json` written with bake_id + completed_at | The handler marshals exactly that shape. |
| `BuildPackMetaFromBake` reads `bake_id` from the file | Reader added; capture stops minting `time.Now()` directly. |
| Fallback when file is absent + warning log | Reader returns `(zero, nil)` on `os.IsNotExist`; capture logs a warning and uses `time.Now()`. |
| Combining same intermediates twice → same bake_id | Verified by a unit test that calls `BuildPackMetaFromBake` twice with the same on-disk `{id}_bake.json`. |

## On-disk and wire shapes

**`{outputsDir}/{id}_bake.json`** — single source of truth:

```json
{
  "bake_id": "2026-04-08T19:14:07Z",
  "completed_at": "2026-04-08T19:14:07Z"
}
```

**`POST /api/bake-complete/:id`** — empty request body, JSON response
mirroring the upload handlers:

```json
{ "status": "ok", "bake_id": "2026-04-08T19:14:07Z" }
```

The handler returns the id it wrote so the JS layer (and curl-based
manual tests) can confirm it without re-reading the file.

The Go-side struct is a single internal type `bakeStamp`:

```go
type bakeStamp struct {
    BakeID      string `json:"bake_id"`
    CompletedAt string `json:"completed_at"`
}
```

## Time handling

All timestamps are `time.Now().UTC().Format(time.RFC3339)` — second
precision, Z-suffixed, lexicographically sortable. Matches the
example in `pack_meta_test.go:17` (`"2026-04-08T11:32:00Z"`) and the
existing `pack_meta_capture.go:102` format.

The handler captures the timestamp **once** and writes it to both
fields in the same call. There is no millisecond drift between
`bake_id` and `completed_at` because there is only one `time.Now()`.

## Test injection seam for time

Two unit tests need to lock down "same input → same id":

1. The fallback test must verify that when `{id}_bake.json` is
   absent, capture mints a non-empty id and logs a warning.
2. The stability test must verify that when the file *is* present,
   two consecutive `BuildPackMetaFromBake` calls return the same id
   regardless of wall clock.

Both can be done without injecting a fake clock — the file-present
case is already deterministic, and the absent case just asserts the
field is non-empty (the warning is observed via `log.SetOutput` to a
buffer).

## Failure modes

| Scenario | Behavior |
|---|---|
| `{id}_bake.json` missing | capture logs warning, falls back to `time.Now()`. Combine succeeds. |
| `{id}_bake.json` malformed JSON | capture wraps and returns the error. Combine fails loudly. |
| `{id}_bake.json` present but `bake_id` empty string | capture treats as absent → fallback path, warning logged. (Defensive: an empty string would otherwise crash `PackMeta.Validate`.) |
| `bake-complete` POST for unknown id | 404, mirrors the upload handlers' `store.Get(id)` check. |
| `bake-complete` POST with wrong method | 405, mirrors upload handlers. |
| Concurrent combine racing the bake-complete write | Out of scope for a single-user demo. The atomic temp+rename used by the handler means readers see either the old file or the complete new one, never a partial. |

## What this ticket explicitly does NOT change

- Does not modify `PackMeta` or its `Validate` rules. `bake_id` is
  still a free-form non-empty string at the contract level.
- Does not modify `combine.go` (T-010-02 territory). `BuildPackMeta`
  is the only seam that needs touching.
- Does not modify `FileRecord` or add `HasBake` flags — the on-disk
  presence of `{id}_bake.json` is the source of truth.
- Does not touch `prepareForScene` or any other JS path. The bake-id
  stamp is wired only to the production button driver because that
  is where the AC explicitly points.
