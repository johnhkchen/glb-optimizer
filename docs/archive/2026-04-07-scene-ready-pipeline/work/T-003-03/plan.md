# Plan — T-003-03: profiles-save-load-comment

## Step-by-step

### Step 1 — Create `profiles.go`

- Add the package-level constants: `ProfilesSchemaVersion`,
  `profileNameMaxLen`, `profileCommentMaxLen`, and the
  `profileNameRe` regex.
- Define `Profile` and `ProfileMetadata` structs with the JSON
  tag order specified in design.md.
- Implement `ValidateProfileName(name)` first — used by every
  other function.
- Implement `(*Profile).Validate()`. Order: name → schema version
  → comment length → settings non-nil → `Settings.Validate()`.
- Implement `ProfilesFilePath(name, dir)` as plain string join.
- Implement `LoadProfile`: validate name, open file, return
  `fs.ErrNotExist` for missing, decode otherwise.
- Implement `SaveProfile`: validate, stamp `CreatedAt` if empty,
  `MkdirAll`, marshal indent, `writeAtomic`.
- Implement `ListProfiles`: `os.ReadDir`, decode-only-metadata
  per file, skip-on-error, sort by name ascending.
- Implement `DeleteProfile`: validate name, `os.Remove`, wrap
  `os.IsNotExist` as `fs.ErrNotExist`.

**Verification**: `go build ./...` is green.

### Step 2 — Create `profiles_test.go`

Write the 11 test cases listed in structure.md. Group `t.Run`
tables for the bad-name and bad-settings cases.

**Verification**: `go test -run TestProfile ./...` and
`go test -run TestSaveLoad ./...` pass; `go test ./...` is green.
**Commit**: "Add profiles.go data layer with tests".

### Step 3 — Wire `analytics.go` allow-list

Add the two new event types to `validEventTypes`. No other
analytics changes; the existing `Event` envelope already supports
arbitrary string→interface{} payloads.

**Verification**: `go test ./...` (existing analytics tests still
pass).

### Step 4 — Add HTTP handlers in `handlers.go`

In order:

- `handleProfilesList(profilesDir)` — GET-only, calls
  `ListProfiles`, on error 500, on success
  `jsonResponse(w, 200, list)` (empty list returns `[]`, not
  `null` — initialize the slice as `[]ProfileMetadata{}` if
  `ListProfiles` returns nil).
- `handleProfileSave(profilesDir)` — POST-only, decode into a
  `profileSaveRequest{Name, Comment, Settings, SourceAssetID}`
  struct (omitting `CreatedAt`), build a `Profile`, call
  `SaveProfile`. Return 400 on validate failure, 500 on disk
  failure, 200 with the saved profile on success.
- `handleProfile(profilesDir)` — GET / DELETE on
  `/api/profiles/:name`. Parse the name segment via
  `strings.TrimPrefix`, validate it (400 on failure — also catches
  embedded slashes), dispatch on method.

**Verification**: `go build ./...` is green. Hand-run via curl
once main.go is wired (Step 5).

### Step 5 — Wire routes and dir in `main.go`

- Add `profilesDir := filepath.Join(workDir, "profiles")` near the
  other dir vars.
- Add `profilesDir` to the `MkdirAll` loop.
- Register the two routes after the `/api/settings/` registration:

```go
mux.HandleFunc("/api/profiles", func(w http.ResponseWriter, r *http.Request) {
    switch r.Method {
    case http.MethodGet:  handleProfilesList(profilesDir)(w, r)
    case http.MethodPost: handleProfileSave(profilesDir)(w, r)
    default:              jsonError(w, http.StatusMethodNotAllowed, "method not allowed")
    }
})
mux.HandleFunc("/api/profiles/", handleProfile(profilesDir))
```

**Verification**:
- `go build ./...` is green.
- `go test ./...` is green.
- Manual curl roundtrip:
  - `curl -X POST localhost:8787/api/profiles -d '{"name":"test","comment":"hi","settings":{...defaults...}}'`
  - `curl localhost:8787/api/profiles`
  - `curl localhost:8787/api/profiles/test`
  - `curl -X DELETE localhost:8787/api/profiles/test`
  - bad name `curl ... -d '{"name":"BadName",...}'` → 400.
- Inspect `~/.glb-optimizer/profiles/test.json` after the save.

**Commit**: "Add profiles HTTP endpoints + main wiring".

### Step 6 — Add the Profiles section in `static/index.html`

Insert the new `.settings-section` block after the existing
`tuningSection` (after line 304). Use the skeleton from
structure.md verbatim.

**Verification**: open the page, confirm the section renders
beneath Tuning. Buttons present and disabled (no JS yet).

### Step 7 — Add JS module state, helpers, and DOM refs

In `app.js`, near the existing tuning helpers:

- Add the `profilesList` module variable and the
  `PROFILE_NAME_RE` constant.
- Add the new DOM refs near `simplificationSlider` etc.
- Implement `loadProfileList`, `updateProfileButtons`,
  `applySelectedProfile`, `deleteSelectedProfile`,
  `openSaveProfileForm`, `submitSaveProfile`.
- Wire the listeners at the bottom of the file (near the
  `wireTuningUI()` call site).
- Call `loadProfileList()` once at init.

**Verification**: open the page, save a profile via the form,
confirm it appears in the dropdown after save. Apply it after
changing settings, confirm sliders revert. Delete it, confirm it
disappears.

**Commit**: "Add profiles UI and JS wiring".

### Step 8 — Add CSS for the new controls

Add the `.profile-actions`, `.profile-name-error`,
`.profile-form-actions`, `#profileNameInput.invalid`,
`#profileCommentInput` rules. Match the existing dark-panel
palette.

**Verification**: visual check in the browser. Bad name shows red
border + error text.

**Commit**: "Style profiles UI".

### Step 9 — Update `docs/knowledge/analytics-schema.md`

Add the two new event-type sections, update the versioning policy
note, strike the profile-artifacts line in "Out of scope".

**Verification**: docs render in markdown preview without broken
formatting.

**Commit**: "Document profile_saved/profile_applied events".

### Step 10 — Manual end-to-end check

With the server running and an asset uploaded:

1. Select an asset.
2. Tweak two sliders.
3. Click "Save current as profile…", enter name `round-bushes-warm`,
   comment "tested with the dome at 0.6", click Save.
4. Verify the dropdown now has `round-bushes-warm`.
5. Tweak one more slider so it diverges.
6. Select the profile and click Apply. Confirm the sliders snap
   back to the saved values.
7. `cat ~/.glb-optimizer/tuning/{session}.jsonl` and confirm the
   sequence ends with `setting_changed`, then `profile_saved`,
   then `setting_changed`, then `profile_applied`.
8. `cat ~/.glb-optimizer/profiles/round-bushes-warm.json` and
   confirm the on-disk shape matches the design's data model.
9. Click Delete on the dropdown selection, confirm dialog,
   confirm the profile vanishes from the dropdown and the file
   is gone from disk.

## Testing strategy

### Unit (Go)

Covered in Step 2. Total: 11 new tests in `profiles_test.go`. The
critical paths are:

- Round-trip identity (`TestSaveLoad_Roundtrip`): catches any
  drift between `SaveProfile` and `LoadProfile` shape.
- Validation (`TestValidate_RejectsBadName`,
  `TestValidate_RejectsBadSettings`): exercises the validator
  table.
- Listing (`TestListProfiles_SortedByName`,
  `TestListProfiles_SkipsCorrupt`): the bits unique to profiles
  vs. settings.
- Overwrite (`TestSaveProfile_Overwrite`): the "v1 is overwrite-OK"
  rule.

### HTTP (Go)

No new `httptest`-based handler tests for v1, following the
precedent set by T-003-01 and T-003-02 reviews. The handlers are
thin (decode → validate → delegate → marshal), the load-bearing
logic is in `profiles.go` and is tested. The manual curl
roundtrip in Step 5 is the verification gate for the wire layer.

If the four-endpoint surface ever grows or sees non-trivial
branching (auth, batching, etc.), this is the right time to add
`httptest` coverage.

### Frontend

No JS tests — the project still has zero JS test infra. The
manual e2e in Step 10 is the verification gate. The JS is small
(~130 lines) and 1:1 with the backend.

### Regression

`go test ./...` after every step that touches Go. The full
existing suite (T-003-01 + T-003-02 + T-002 settings + earlier)
must stay green.

## Risk and mitigation

| Risk                                                | Mitigation                                            |
|-----------------------------------------------------|-------------------------------------------------------|
| Profile name validation drifts between Go and JS    | Go is the source of truth; JS regex is mirrored verbatim and the server returns 400 with the validator's error string for the JS to surface |
| Overwriting an existing profile silently            | Documented behavior per ticket; no mitigation needed in v1 |
| Apply-profile races with auto-save of current settings | `applySelectedProfile` PUTs the profile's settings via `/api/settings/:id`, then calls `loadSettings(id)` which reads the canonical state back. Any in-flight debounced save is harmless because it carries the *previous* `currentSettings` which is what was on disk anyway |
| Listing crashes if the profiles dir is missing      | `main.go` MkdirAll's it at startup; `ListProfiles` also tolerates missing dir by returning an empty list |
| The session_end snapshot includes a profile name?  | Out of scope. `profile_saved`/`profile_applied` are independent events; `session_end.final_settings` continues to capture only the settings struct |

## Out-of-plan items (intentional non-goals)

- Built-in starter profiles
- Profile diff / preview
- Profile import/export UI
- Profile thumbnails
- Multi-asset apply ("apply this profile to all assets in this
  folder")
- Profile-level analytics like `profile_deleted`

These are deferred per the ticket's "Out of Scope" section.
