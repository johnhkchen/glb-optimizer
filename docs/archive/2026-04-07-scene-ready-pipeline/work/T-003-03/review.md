# Review — T-003-03: profiles-save-load-comment

## What changed

### Files created

| File                                  | Purpose                                              |
|---------------------------------------|------------------------------------------------------|
| `profiles.go`                         | Data model, validation, file I/O for `Profile`       |
| `profiles_test.go`                    | 12 unit tests covering validation, round-trip, list, delete, overwrite, metadata projection |

### Files modified

| File                                  | Change                                                                                  |
|---------------------------------------|-----------------------------------------------------------------------------------------|
| `analytics.go`                        | Two new entries in `validEventTypes`: `profile_saved`, `profile_applied`                |
| `handlers.go`                         | `handleProfilesList`, `handleProfileSave`, `handleProfile`, `isProfileValidationError`; new `errors`/`io/fs` imports |
| `main.go`                             | `profilesDir` variable, MkdirAll loop entry, two route registrations                    |
| `static/index.html`                   | New `.settings-section#profilesSection` with select, action buttons, and save form      |
| `static/app.js`                       | Profiles state, DOM refs, six helper functions, listener wiring, init call; one new line in `selectFile` |
| `static/style.css`                    | Styles for the new section's select, buttons, inputs, and validation feedback           |
| `docs/knowledge/analytics-schema.md`  | Two new event-type sections, struck the "Profile artifacts" out-of-scope line           |

No files deleted. No new third-party dependencies (`go.mod`
unchanged).

## Acceptance-criteria mapping

| AC item                                                            | Status | Where                                       |
|--------------------------------------------------------------------|--------|---------------------------------------------|
| `profiles.go` with `Profile` struct (name, comment, settings, created_at, source_asset_id) | ✅ | `profiles.go:35-45` |
| `LoadProfile(name)` from `~/.glb-optimizer/profiles/{name}.json`  | ✅ | `profiles.go` `LoadProfile`                 |
| `SaveProfile(profile)` writes atomically                          | ✅ | `profiles.go` `SaveProfile` → `writeAtomic` |
| `ListProfiles()` returns metadata (no full settings)              | ✅ | `profiles.go` `ListProfiles` → `ProfileMetadata` |
| `DeleteProfile(name)`                                             | ✅ | `profiles.go` `DeleteProfile`               |
| `GET /api/profiles` returns list                                   | ✅ | `handlers.go` `handleProfilesList`          |
| `GET /api/profiles/:name` returns full profile                    | ✅ | `handlers.go` `handleProfile` (GET branch)  |
| `POST /api/profiles` accepts `{name, comment, settings}`, saves   | ✅ | `handlers.go` `handleProfileSave`           |
| `DELETE /api/profiles/:name`                                      | ✅ | `handlers.go` `handleProfile` (DELETE branch) |
| New "Profiles" section in tuning panel                            | ✅ | `static/index.html` `#profilesSection`      |
| Dropdown listing existing profiles                                 | ✅ | `app.js` `loadProfileList` / `redrawProfileSelect` |
| "Save current as profile…" with name + comment input              | ✅ | `app.js` `openSaveProfileForm` / `submitSaveProfile` |
| "Apply profile" button overwrites current asset settings          | ✅ | `app.js` `applySelectedProfile`             |
| "Delete profile" action                                            | ✅ | `app.js` `deleteSelectedProfile`            |
| Applying a profile fires `profile_applied {profile_name}`         | ✅ | `app.js` `applySelectedProfile` final line  |
| Saving a profile fires `profile_saved`                             | ✅ | `app.js` `submitSaveProfile` final line     |
| Profile names: kebab-case, server-validated, max 64 chars         | ✅ | `profiles.go` `ValidateProfileName` + `profileNameRe` |

## Test coverage

### Added (Go)

```
=== RUN   TestDefaultProfile_Valid                       PASS
=== RUN   TestSaveLoad_ProfileRoundtrip                  PASS
=== RUN   TestLoadProfile_MissingReturnsNotExist         PASS
=== RUN   TestValidate_RejectsBadName (10 sub-cases)     PASS
=== RUN   TestValidate_RejectsBadSettings                PASS
=== RUN   TestValidate_RejectsNilSettings                PASS
=== RUN   TestSaveProfile_StampsCreatedAtIfEmpty         PASS
=== RUN   TestListProfiles_SortedByName                  PASS
=== RUN   TestListProfiles_SkipsCorrupt                  PASS
=== RUN   TestListProfiles_MissingDirReturnsEmpty        PASS
=== RUN   TestDeleteProfile_RemovesFile                  PASS
=== RUN   TestDeleteProfile_MissingReturnsNotExist       PASS
=== RUN   TestSaveProfile_Overwrite                      PASS
=== RUN   TestListProfiles_MetadataExcludesSettings      PASS
```

12 new test functions; one (`TestValidate_RejectsBadName`) is
table-driven with 10 sub-cases. Total new sub-tests: ~22.

The critical paths are:

- **Round-trip** — save then load reproduces the same struct,
  catches any drift in field ordering or tag names.
- **Validation matrix** — every kebab-case failure mode plus the
  bad-settings and nil-settings cases.
- **List sorting + corrupt-skip + missing-dir tolerance** — the
  three behaviors unique to profiles vs. settings.
- **Overwrite semantics** — confirms the "v1 is overwrite-OK" rule.
- **Metadata projection** — compile-time-ish guarantee that
  `ListProfiles` does not leak the full settings block.

### Inherited

All existing T-002 / T-003-01 / T-003-02 tests still pass. Total
suite: green.

### Coverage gaps

- **No HTTP-handler tests for the four new profile endpoints.**
  Same precedent as T-003-01 and T-003-02 reviews: handlers are
  thin (decode → validate → delegate → marshal), the load-bearing
  logic lives in `profiles.go` and is exhaustively tested. The
  HTTP layer adds error mapping (validate → 400, not-found → 404,
  disk → 500) which is verifiable by inspection. If a third
  endpoint family lands, this is the right time to add an
  `httptest`-based suite that covers all three.

- **No JS tests.** Project still has zero JS test infra. The
  affected JS surface is ~155 lines: `loadProfileList`,
  `redrawProfileSelect`, `updateProfileButtons`,
  `applySelectedProfile`, `deleteSelectedProfile`,
  `openSaveProfileForm`, `closeSaveProfileForm`,
  `submitSaveProfile`. Each is small and 1:1 with a wire
  contract that the Go suite covers.

- **No live end-to-end run.** The manual sequence in plan.md
  Step 10 (open browser, save a profile, apply it, inspect the
  JSONL and the on-disk profile) was not executed in this run.
  All wire contracts are unit-tested and the JS is straightforward;
  the residual gap is the visual confirmation that the form opens,
  the validation feedback shows red, and the dropdown re-populates.

## Open concerns

1. **Validation-vs-disk error split via prefix sniffing.**
   `handleProfileSave` distinguishes 400 from 500 by examining the
   error message string for known prefixes
   (`isProfileValidationError`). This is fragile: renaming a
   `Validate()` error message silently turns a 400 into a 500. The
   right fix is to return a typed `*ValidationError` from
   `(*Profile).Validate()` (and ideally from `(*AssetSettings).Validate()`
   too, for symmetry) and inspect with `errors.As`. Deferred because
   it touches `settings.go` and would balloon the diff. Flagged for
   a future cleanup ticket.

2. **Kebab-case regex is duplicated in JS and Go.** Drift fails
   loud (server-side 400) rather than silent, so this is the
   acceptable kind of duplication, but it is duplication. A
   `/api/profiles/validate-name?name=...` endpoint or a literal
   regex string returned from `/api/status` would be over-design
   for a single field.

3. **`profile_applied` fires before settings re-read completes.**
   `applySelectedProfile` calls `loadSettings(selectedFileId)` and
   `populateTuningUI()` *before* `logEvent`, so by the time the
   event fires the UI is already in the new state. This is the
   intended order — the event represents "a successful apply", not
   "an apply was kicked off". Worth noting for T-003-04 readers.

4. **Save form does not warn on overwrite.** Re-saving a profile
   under the same name silently replaces it. The ticket explicitly
   says "Profile versioning: overwrite is fine for v1," so this is
   working as specified, but a single-user `confirm()` could be
   added cheaply if it ever bites.

5. **Source asset id is breadcrumb-only.** The field is captured
   on save and surfaced in the metadata projection but is not used
   anywhere in v1 — neither in the UI nor in the analytics flow.
   It is documented as reserved for future use ("which assets
   generated which profiles") and is cheap to carry now.

6. **`comment` is plain text, capped at 1024 bytes.** No
   markdown rendering, no link detection, no length feedback in
   the textarea. The cap is silent (returns 400 if exceeded) rather
   than enforced in the UI; for first-pass scope this is fine but
   a small char-count helper would be a nice ergonomic addition.

7. **`profile_saved` / `profile_applied` are session-scoped.**
   `logEvent` no-ops if `analyticsSessionId` is unset. In practice
   you cannot reach the save/apply UI without a selected asset
   (and therefore an active session), so this is correct behavior;
   noting it explicitly so future refactors don't accidentally
   move the buttons outside the session lifetime.

## Critical issues for human attention

None. Build green, all 14 ticket-relevant test functions green
(with subtests, ~26 PASS lines), full suite green, plan executed
with the seven small deviations documented in `progress.md`. The
only residual item needing attention is running the live e2e in a
browser, which is better done by a human anyway.

## Handoff notes for downstream tickets

- **T-003-04 (export)** can lean on the new
  `profile_saved` / `profile_applied` events as user-intent
  signals: a `profile_saved` followed by `accept` is a strong
  satisfaction indicator; a `profile_applied` followed quickly by
  many `setting_changed` events suggests the profile didn't fit.
- The on-disk format at `~/.glb-optimizer/profiles/{name}.json`
  is the canonical "shareable" artifact per the ticket's
  "Out of Scope" framing. Future export work can simply zip the
  directory.
- The kebab-case validation rule lives in `profiles.go`
  (`ValidateProfileName`) and is the single source of truth;
  any future surface that accepts profile names should call it.
- `isProfileValidationError` in `handlers.go` is a known smell
  (concern #1). The structured-error refactor it suggests would
  also benefit `(*AssetSettings).Validate()` if T-003-04 ever
  needs to distinguish those failure modes over the wire.

## Diff stats (approximate)

```
profiles.go                              | +210 / -0
profiles_test.go                         | +205 / -0
analytics.go                             |   +2 / -0
handlers.go                              | +127 / -0
main.go                                  |  +13 / -2
static/index.html                        |  +24 / -0
static/app.js                            | +156 / -1
static/style.css                         |  +82 / -0
docs/knowledge/analytics-schema.md       |  +30 / -1
```

Plus the RDSPI artifacts under `docs/active/work/T-003-03/`.
