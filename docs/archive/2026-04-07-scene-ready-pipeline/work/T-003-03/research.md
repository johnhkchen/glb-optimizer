# Research — T-003-03: profiles-save-load-comment

## Ticket essentials

Profiles are named, commented snapshots of `AssetSettings` that the user
can save once and apply across many assets. They live in
`~/.glb-optimizer/profiles/{name}.json`. The acceptance criteria call
for a Go module (`profiles.go`), four HTTP endpoints, a small UI section
in the tuning panel, two new analytics events (`profile_saved`,
`profile_applied`), and kebab-case server-side name validation capped at
64 chars.

This ticket is the third in the S-003 chain. T-003-01 shipped the
analytics envelope + `AnalyticsLogger`, T-003-02 wired
auto-instrumentation and per-asset session resume. Both are landed and
green.

## Existing pieces this builds on

### `settings.go` (T-002-01, T-002-03)

The whole `AssetSettings` story is a near-perfect template for the
profile module:

- `AssetSettings` struct (`settings.go:22`) with `SchemaVersion: 1`,
  11 typed fields, JSON tag order = field declaration order.
- `DefaultSettings()` returns the canonical v1 defaults
  (`settings.go:39`). The on-disk JSON shape is also documented in
  `docs/knowledge/settings-schema.md`.
- `Validate()` enforces ranges + enum membership (`settings.go:68`),
  delegating numeric checks to `checkRange` (NaN/Inf-aware).
- `LoadSettings(id, dir)` returns `DefaultSettings()` on missing
  file, decode-only otherwise (no validate-on-load — the comment
  there says callers should validate themselves).
- `SaveSettings(id, dir, s)` mkdir-p's the directory, marshals
  with 2-space indent, then `writeAtomic()` (temp file in same dir
  + `os.Rename`). All file work for profiles can lift this verbatim.
- `writeAtomic(path, data)` is private but reusable in-package; it
  guarantees the partial-write window is impossible because the
  rename is atomic on POSIX.

### `handlers.go` analytics + settings handlers

- `handleSettings` (`handlers.go:624`) is the closest precedent for
  what the four profile endpoints look like. It dispatches on
  `r.Method`, decodes into a struct, calls `Validate()`, then
  `SaveSettings`. Errors funnel through `jsonError(w, status, msg)`.
  Success uses `jsonResponse(w, 200, payload)`.
- `handleAnalyticsEvent` (`handlers.go:976`) shows the pattern for
  POST-only endpoints with method-check + decode + validate +
  delegate.
- `jsonResponse`/`jsonError` (`handlers.go:25-33`) are the only
  response helpers in the project — there is no router framework.
- All routes are wired by hand in `main.go` via
  `mux.HandleFunc`. The `/api/files/` and `/api/files` split (lines
  103–111) shows the project's idiom for handling "list" vs.
  "single resource" on the same prefix: a wrapper handler examines
  the path/method and dispatches.

### `analytics.go` + frontend `logEvent`

- `validEventTypes` (`analytics.go:25`) is a hard allow-list. Any
  new event type — `profile_saved`, `profile_applied` — must be
  added there or it will be rejected with HTTP 400 by
  `(*Event).Validate()`.
- The matching client side is `logEvent(type, payload, assetId)`
  in `app.js:218`. It is a no-op if `analyticsSessionId` is unset,
  which is the right behavior for "save profile while no asset is
  selected" (we can choose to require selection or fire with empty
  asset_id; see Design).
- `analytics-schema.md` is the canonical doc; new event types
  must land there or downstream readers (T-003-04) won't know
  what to expect.

### Frontend tuning panel (T-002-03)

- `TUNING_SPEC` array + `populateTuningUI()` + `wireTuningUI()`
  in `app.js:260-328` is the entire tuning UI. It is built around
  one DOM element per `AssetSettings` field, plus a "reset to
  defaults" button.
- `currentSettings` (module-level, `app.js:29`) is the single
  source of truth for the tuning panel. Anything that mutates it
  must call `populateTuningUI()` to redraw and `saveSettings(id)`
  to debounce-PUT.
- `selectFile(id)` (`app.js:2369`) drives the per-asset lifecycle:
  ends the previous analytics session, loads settings, starts the
  new analytics session, populates the tuning UI, loads the model.
- The Tuning section in `index.html` is at line 219 and ends at
  line 304. There is no modal infrastructure — sections just
  stack inside `.settings-panel`. CSS classes available include
  `.settings-section`, `.setting-row`, `.preset-btn`, and a
  small `.dirty-dot` indicator.

### Storage layout

`main.go:56-67` MkdirAll's `originals/`, `outputs/`, `settings/`,
`tuning/`. Profiles will need a fifth directory, `profiles/`,
created the same way at startup so the logger never has to deal
with a missing dir.

## Constraints from the ticket

- **Profile name validation**: kebab-case, max 64 chars. Must be
  enforced server-side (not just in the UI). Concretely, the
  regex `^[a-z0-9]+(-[a-z0-9]+)*$` matches kebab-case as commonly
  understood (no leading/trailing dashes, no double dashes, no
  uppercase). Length cap is independent.
- **Atomic writes**: same `writeAtomic` story as settings; profiles
  are small (single-digit KB) so atomicity is cheap.
- **`ListProfiles` returns metadata only** (name + comment, no full
  settings). Implementation: read each `*.json`, decode just the
  metadata fields, skip the rest. The settings field is the bulk of
  the file (~12 numeric fields) so this is mainly about list-page
  payload size, not perf.
- **Profile applied → overwrites current asset settings**. This is
  a frontend-side operation: PUT the profile's settings to
  `/api/settings/:assetId`, then `loadSettings()` and
  `populateTuningUI()` to refresh.
- **Analytics**: `profile_saved {profile_name}`,
  `profile_applied {profile_name}`. Both ride the existing
  `logEvent` helper. They are session-scoped — i.e. they fire only
  when an analytics session is live (asset is selected). For the
  "save" path that is fine because the user must have an asset
  open to have settings worth saving. For the "apply" path that is
  also fine because applying a profile only makes sense to an
  open asset.

## Out of scope (per ticket)

- Profile sharing / export — punts to "use the file directly".
- Built-in starter profiles — empty list at install time.
- Profile versioning — overwrite is fine for v1.
- Search / filter / tag — flat dropdown.
- Profile diff / preview / thumbnail.

## Open questions to resolve in Design

1. Do profiles store the full `AssetSettings` struct verbatim
   (including `schema_version`), or only the user-tunable subset?
2. What happens when a profile is applied to an asset whose own
   `AssetSettings` schema version is older/newer than the
   profile's? (Current schema is v1 across the board, so this is
   theoretical for now, but the `Validate()` call will catch it
   either way.)
3. Where in the tuning panel does the "Profiles" section sit —
   above the per-field controls (so it reads as "load a profile
   first, then tweak") or below (so it reads as "tweak, then save
   what you have")? Below mirrors the existing "Reset to defaults"
   button placement.
4. Should `ListProfiles` sort by name, by `created_at`, or by
   most-recently-modified? The dropdown UX argues for stable
   alphabetical.
5. What does the "delete profile" flow look like — confirm dialog
   or no? Given the local-single-user context and the simplicity
   of file recovery (profiles are tiny JSON), a `confirm()` is
   the lightest reasonable guard.
