# T-010-03 Research — Pack Button & Endpoint

## Goal Recap

Wire the existing `CombinePack` (T-010-02) and `BuildPackMetaFromBake`
(T-011-02) into the production preview UI so that, after building the
three intermediates, a user can click "Build Asset Pack" and have a
finished `dist/plants/{species}.glb` written.

## Codebase Surface Relevant to This Ticket

### Backend — Go HTTP layer

- **`handlers.go` (1683 lines)** holds every HTTP handler. The pattern
  is uniform: each handler is a closure constructor (`handleX(...)
  http.HandlerFunc`) that captures the file store, directory paths,
  and any helpers (analytics logger, blender info). Errors are
  surfaced via the `jsonError(w, status, msg)` helper.
- The closest analogues to the new handler are
  `handleUploadBillboardTilted` (handlers.go:470-509) and
  `handleUploadVolumetric` (handlers.go:512-551). They both
  - parse `id := strings.TrimPrefix(r.URL.Path, "/api/.../")`
  - reject unknown ids with 404 via `store.Get(id)`
  - call `os.WriteFile` against `filepath.Join(outputsDir, id+"_<role>.glb")`
  - flip the matching `FileRecord` flag via `store.Update`
- **`combine.go:631`** — the `CombinePack(side, tilted, volumetric []byte, meta PackMeta) ([]byte, error)`
  function. `side` is required, the other two may be `nil`. The
  function itself enforces the 5 MiB cap (`packSizeCap = 5 * 1024 * 1024`,
  combine.go:20) and returns an error string containing
  `"exceeds 5 MiB cap"` when the pack is too large.
- **`pack_meta_capture.go:48`** — `BuildPackMetaFromBake(id, originalsDir, settingsDir, outputsDir, store) (PackMeta, error)`
  is the canonical way to assemble the pack metadata at bake time.
  It already handles species derivation, override-file lookup, and
  fade-band capture, and returns a fully validated `PackMeta`.
- **`main.go:139`** registers routes on a single `http.ServeMux`.
  The `outputsDir` is `workDir/outputs`; new directories are created
  via `os.MkdirAll` in the loop at main.go:101. There is no existing
  `distDir`.

### Backend — file naming on disk

The intermediates that `CombinePack` consumes live in `outputsDir`
under the following names (asserted by the existing upload handlers):

- `{id}_billboard.glb`         — required (camera-facing side)
- `{id}_billboard_tilted.glb`  — optional (tilted)
- `{id}_volumetric.glb`        — optional (dome slices)

`FileRecord` exposes the matching `HasBillboard`,
`HasBillboardTilted`, and `HasVolumetric` boolean flags
(models.go:55-57). All three are JSON-serialized as
`has_billboard`, `has_billboard_tilted`, `has_volumetric` and are
already consumed by the frontend in `updatePreviewButtons`
(static/app.js:4696-4736).

### Backend — analytics envelope

`handleAnalyticsEvent` (handlers.go:1338) accepts the canonical
`Event` envelope defined in `analytics.go:48`. The frontend already
has a `logEvent(type, payload, assetId)` helper at
static/app.js:366-388 — every existing trigger reuses it. We must
not roll a new analytics path; the ticket explicitly says "Reuse the
existing analytics-event helper; do not roll a new one."

### Frontend — production trigger

- The "Build hybrid impostor" button (`generateProductionBtn`,
  static/index.html:69) sits inside the
  `<details class="toolbar-advanced">` Advanced disclosure of the
  toolbar (static/index.html:60-71). It is the existing trigger
  that produces the three intermediates.
- The click handler at static/app.js:4838-4840 delegates to
  `generateProductionAsset(id)` (static/app.js:2415-2472), which
  uploads the three intermediates one after another and ends with
  `refreshFiles(); updatePreviewButtons()`.
- `updatePreviewButtons` (static/app.js:4696) is the single source
  of truth for enabling/disabling toolbar buttons based on the
  selected file's flags.

### Frontend — error/log surface

There is no toast component. The closest existing surface is the
`prepareError` div (static/index.html:83, static/app.js:81) — a
small text element under the Prepare-for-scene progress block,
already used by `prepareForScene` to show stage failures
(static/app.js:2606). Reusing it for the pack-build outcome keeps
the UI footprint identical to the rest of the toolbar story and
avoids inventing a new component.

## Constraints & Assumptions

- **Five MiB cap is shared.** `CombinePack` already returns an error
  containing `"5 MiB cap"`. The handler must translate that error
  into a `413 Payload Too Large` response so the frontend can
  distinguish it from a generic combine failure.
- **Missing intermediates ⇒ 400.** Acceptance says
  "Returns 400 if required intermediates are missing". `side`
  (`{id}_billboard.glb`) is the only strictly required input;
  the optional pair (`tilted`, `volumetric`) cannot both be missing
  per the UI gate, but the handler will tolerate either being absent
  the same way `CombinePack` does.
- **`dist/plants/` does not yet exist anywhere in the codebase.**
  Searching for `dist/plants`, `distDir`, `distRoot` returned nothing.
  The sibling ticket T-010-04 (justfile-pack-all) makes clear the
  intent: this is a sibling of `outputs/` under the working
  directory, ready to be USB-copied to the demo laptop. We will
  create `workDir/dist/plants/` in `main.go`'s mkdir loop and pass
  it as a new `distDir` parameter to the handler.
- **Filename = `{species}.glb`, not `{id}.glb`.** The species id is
  what the consumer (plantastic) keys on, and packs from different
  uploads of the same species should overwrite cleanly.
- **No batch packing here.** Out-of-scope per ticket.

## Open Questions Resolved

- *Where does the species come from?* From the `PackMeta` we just
  built — `meta.Species`. We do not re-derive it client-side.
- *What does the response shape look like?* The ticket pins it:
  `{ "pack_path", "size", "species" }`.
- *Should the button live next to "Build hybrid impostor" or in the
  Advanced disclosure?* The ticket says "near the existing 'Build
  hybrid impostor' trigger, not in a new panel" — so it goes inside
  the same `advanced-panel` div, immediately after
  `generateProductionBtn`.
