# T-011-03 — Research

## Ticket recap

`bake_id` is a forward-looking handle for the future asset server, which
will serve packs at immutable URLs like `/{species}/{bake_id}.glb` and
rely on cache headers for invalidation. For the demo it is *only* a
string in the metadata that the consumer roundtrips. The crucial
property is **stability across combine runs of the same intermediates**:
two consecutive packs of the same bake must carry the same `bake_id`,
or future cache-busting will treat them as different assets.

The current code (T-011-02) violates this. `pack_meta_capture.go:102`
sets `BakeID` from `time.Now().UTC().Format(time.RFC3339)` *inside*
`BuildPackMetaFromBake`, so every combine run mints a fresh id even
when nothing else has changed.

## Where the bake happens today

The "bake" is the work that runs when the user clicks **Build hybrid
impostor** on the toolbar (`static/index.html:69`). The driver function
is JS — `generateProductionAsset` in `static/app.js:2412`. Its three
phases run in fixed order:

1. Side billboard pass: `renderMultiAngleBillboardGLB` → POST to
   `/api/upload-billboard/:id`.
2. Tilted billboard pass: `renderTiltedBillboardGLB` → POST to
   `/api/upload-billboard-tilted/:id`.
3. Volumetric dome pass: `renderHorizontalLayerGLB` → POST to
   `/api/upload-volumetric/:id`.

Each upload lands in a Go handler in `handlers.go`:

| Endpoint | Handler | File written |
|---|---|---|
| `/api/upload-billboard/:id` | `handleUploadBillboard` (424) | `{outputsDir}/{id}_billboard.glb` |
| `/api/upload-billboard-tilted/:id` | `handleUploadBillboardTilted` (470) | `{outputsDir}/{id}_billboard_tilted.glb` |
| `/api/upload-volumetric/:id` | `handleUploadVolumetric` (512) | `{outputsDir}/{id}_volumetric.glb` |

The handlers each save the bytes, set a `Has*` flag on the
`FileRecord`, and respond `{"status":"ok","size":...}`. None of them
write any provenance metadata today.

The volumetric upload is the *last* phase of the bake driver, so its
arrival on the server marks "bake complete" — but **only** when the
driver was the production button. The same volumetric endpoint is
also reachable from devtools entry points and `prepareForScene`, so
the handler itself cannot assume volumetric == bake done.

## Where `bake_id` is consumed

`pack_meta.go:43` declares `BakeID string`; validation rejects an
empty value. Every embed/parse path goes through it:

- `PackMeta.ToExtras` (pack_meta.go:115) — combine writes this into
  `scene.extras.plantastic` of the output pack.
- `ParsePackMeta` (pack_meta.go:130) — round-trips on the consumer side.
- Tests in `pack_meta_test.go` use a fixed string `"2026-04-08T11:32:00Z"`.

`pack_meta_capture.go:48–112` (`BuildPackMetaFromBake`) is the **only**
producer of `BakeID` in the Go code. T-010-02's combine ticket calls
`BuildPackMetaFromBake` from the combine endpoint to mint the metadata
that goes into the pack — that is the call site whose stability we are
fixing.

## Existing on-disk layout for asset state

`outputsDir` (default `outputs/`) already holds per-asset bake state
keyed by id: `{id}_billboard.glb`, `{id}_billboard_tilted.glb`,
`{id}_volumetric.glb`, `{id}_volumetric_lod*.glb`, plus the optional
`{id}_meta.json` species/common-name override read by
`loadCaptureOverride` in pack_meta_capture.go:118. Adding one more
sibling file `{id}_bake.json` follows the established convention:
small JSON, namespaced by id, lives next to the bake outputs.

Pack writes go to a separate `dist/plants/{species}.glb` tree
(T-011-01), so `outputs/` and `dist/` do not overlap.

## Existing JSON-on-disk patterns to mirror

- `loadCaptureOverride` (pack_meta_capture.go:118) is the canonical
  read pattern: `os.ReadFile`, `os.IsNotExist` → zero value + nil
  error, malformed → wrapped error. T-011-03's reader should mirror
  this so a missing `{id}_bake.json` does not block combine.
- `SaveSettings` / `LoadSettings` in `settings.go` are the canonical
  write pattern for per-asset JSON: marshal indented, write atomically
  via temp + rename. The bake-complete writer should follow the same
  shape so concurrent reads never see a half-written file.

## How JS already talks to Go

`static/app.js:2425–2460` shows the established pattern: `fetch(url, {
method: 'POST', headers, body })`. Adding one more POST at the end of
`generateProductionAsset` is mechanically identical to the three calls
already there, with no architectural surface area expanded.

`store.Get(id)` and `store.Update(id, ...)` are how the existing
upload handlers verify the asset exists and stamp `Has*` flags.
`FileRecord` lives in `models.go`. Adding a `BakeCompletedAt` flag is
optional for the demo but trivially possible if observability ever
needs it.

## Constraints surfaced by the ticket

1. **Stability:** repeated combine runs against the same intermediates
   MUST yield identical `bake_id`. The unit test for this is the
   acceptance lock.
2. **Set-once semantics:** the id is written when the **bake**
   completes, not on each combine. Re-running the bake mints a new
   id. This is correct behavior — if the operator re-bakes, the
   intermediates are different, and the consumer cache should miss.
3. **Backward fallback:** if `{id}_bake.json` is absent (e.g., the
   bake ran before this ticket merged), `BuildPackMetaFromBake` falls
   back to `time.Now()` and **logs a warning**. The fallback exists
   so the demo flow does not hard-fail on stale intermediates.
4. **Format:** the JSON shape is fixed by the AC:
   `{ "bake_id": "<RFC3339 UTC>", "completed_at": "..." }`. Both
   fields are populated even though the consumer only reads `bake_id`
   today; `completed_at` is for human/operator forensics.
5. **No hashing:** out of scope. Timestamp ids are fine for the demo.
   Avoid the temptation to add a content hash.

## Risks and gotchas

- **The wrong upload handler.** Stamping `{id}_bake.json` inside
  `handleUploadVolumetric` would also fire for non-production
  volumetric uploads (devtools, future single-pass rebakes). The id
  must be written from a code path that is unambiguously "the
  full bake just finished."
- **Driver-side race:** if the JS driver writes the id between
  upload-volumetric and a combine that races in, there is a tiny
  window where combine sees the old id (or none). For a single-user
  demo this is fine; the test only locks down deterministic stability
  given a stable on-disk file.
- **`time.Now()` in tests.** The current happy-path test asserts
  `BakeID != ""`. After this change the test must seed
  `{id}_bake.json` and assert the value flows through, plus add a
  fallback test that asserts the warning fires when the file is
  absent.
- **Fallback log channel.** The codebase uses the stdlib `log`
  package via the analytics pipeline; capture currently has no logs.
  A single `log.Printf("pack_meta_capture: ...")` line is consistent
  with the rest of the file's "fail loudly" tone.
