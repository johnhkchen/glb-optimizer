# Structure — T-003-03: profiles-save-load-comment

## Files touched

| File                                  | Action  | Net Δ (est) |
|---------------------------------------|---------|-------------|
| `profiles.go`                         | create  | +180        |
| `profiles_test.go`                    | create  | +200        |
| `analytics.go`                        | modify  | +2          |
| `handlers.go`                         | modify  | +130        |
| `main.go`                             | modify  | +5          |
| `static/index.html`                   | modify  | +35         |
| `static/app.js`                       | modify  | +130        |
| `static/style.css`                    | modify  | +20         |
| `docs/knowledge/analytics-schema.md`  | modify  | +25         |
| `docs/active/work/T-003-03/*.md`      | create  | RDSPI       |

Two new source files (`profiles.go`, `profiles_test.go`). No
deletions. No new third-party dependencies (`go.mod` unchanged).

## `profiles.go` (new)

Package `main`. Imports: `encoding/json`, `errors`, `fmt`, `io`,
`io/fs`, `os`, `path/filepath`, `regexp`, `sort`, `strings`,
`time`. Reuses `writeAtomic` from `settings.go` (same package).

```go
const ProfilesSchemaVersion = SettingsSchemaVersion // 1

// profileNameRe enforces kebab-case: lowercase alnum segments
// joined by single dashes, no leading/trailing/double dashes.
var profileNameRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

const (
    profileNameMaxLen = 64
    profileCommentMaxLen = 1024
)

type Profile struct {
    SchemaVersion int            `json:"schema_version"`
    Name          string         `json:"name"`
    Comment       string         `json:"comment"`
    CreatedAt     string         `json:"created_at"`
    SourceAssetID string         `json:"source_asset_id"`
    Settings      *AssetSettings `json:"settings"`
}

type ProfileMetadata struct {
    Name          string `json:"name"`
    Comment       string `json:"comment"`
    CreatedAt     string `json:"created_at"`
    SourceAssetID string `json:"source_asset_id"`
}

func ValidateProfileName(name string) error
func (p *Profile) Validate() error
func ProfilesFilePath(name, dir string) string
func LoadProfile(name, dir string) (*Profile, error)
func SaveProfile(p *Profile, dir string) error
func ListProfiles(dir string) ([]ProfileMetadata, error)
func DeleteProfile(name, dir string) error
```

Behavior notes:

- `ValidateProfileName` is the single source of truth for the name
  rule. Called by `Validate()`, `LoadProfile`, `DeleteProfile`, and
  `ProfilesFilePath`. Returns a friendly error string —
  `"profile name %q must be kebab-case (a-z0-9 with single dashes), 1-64 chars"`.
- `Validate()`: name → schema version → comment length → settings
  non-nil → `settings.Validate()`. Returns the first failure.
- `LoadProfile`: validates name, builds path, opens file, decodes
  into `Profile`. Wraps `os.IsNotExist` errors as `fs.ErrNotExist`
  so callers can use `errors.Is`.
- `SaveProfile`: validates the profile, ensures `CreatedAt` is set
  (stamps `time.Now().UTC().Format(time.RFC3339Nano)` if empty),
  `MkdirAll(dir)`, marshal with 2-space indent + trailing newline,
  `writeAtomic`.
- `ListProfiles`: `os.ReadDir`, filter `*.json`, decode each into
  the local `profileMetadataOnly` struct (lowercase fields matching
  `ProfileMetadata` JSON tags). On per-file decode errors, log via
  `fmt.Fprintln(os.Stderr, ...)` and skip — same posture as
  `LookupOrStartSession` in `analytics.go`. Result sorted by `Name`
  ascending.
- `DeleteProfile`: validates name, `os.Remove`. Wraps not-exist
  errors as `fs.ErrNotExist`.

## `profiles_test.go` (new)

Mirror `settings_test.go`. Tests:

1. `TestDefaultProfile_Valid` — fixture profile passes `Validate()`.
2. `TestSaveLoad_Roundtrip` — save then load round-trips identity.
3. `TestLoadMissing_ReturnsNotFoundError` — `errors.Is(err, fs.ErrNotExist)`.
4. `TestValidate_RejectsBadName` — table-driven: empty, uppercase,
   leading dash, trailing dash, double dash, too long, slash, dot.
5. `TestValidate_RejectsBadSettings` — embeds an `AssetSettings`
   that fails `Validate()` (e.g. resolution=333); expect error.
6. `TestSaveProfile_StampsCreatedAtIfEmpty` — pass profile with
   empty `CreatedAt`, save, load, assert non-empty + parses as RFC 3339.
7. `TestListProfiles_SortedByName` — save three profiles in
   reverse-alphabetical order; list returns them ascending; each
   metadata has the right name+comment but `Settings` field is
   absent from `ProfileMetadata` (compile-time check).
8. `TestListProfiles_SkipsCorrupt` — pre-write a `.json` file with
   garbage; list silently ignores it and still returns the valid one.
9. `TestDeleteProfile_RemovesFile` — save, delete, load returns
   not-exist.
10. `TestProfilesFilePath_RejectsBadName` — direct call returns the
    same kind of safe path or panics for invalid name? Decision:
    `ProfilesFilePath` does **not** validate (callers do); the test
    verifies it composes the path verbatim. The name validation is
    the gate at every public entry point.
11. `TestSaveProfile_Overwrite` — save a profile, save again with a
    different comment, load returns the new comment.

All tests use `t.TempDir()`. No HTTP, no global state.

## `analytics.go` — additions

Two new entries in `validEventTypes`:

```go
"profile_saved":   true,
"profile_applied": true,
```

Order matches the existing pattern: append at the bottom with a
trailing comment `// T-003-03`. No other changes.

## `handlers.go` — additions

Four new handlers + a small URL parsing helper for the trailing
slash split. All use `jsonResponse`/`jsonError`.

```go
// handleProfilesList handles GET /api/profiles. Returns
// []ProfileMetadata sorted by name.
func handleProfilesList(profilesDir string) http.HandlerFunc

// handleProfile handles GET / DELETE / POST on /api/profiles/:name.
// (POST also accepted at /api/profiles for the "create" verb; see
// the dispatch wrapper below.)
func handleProfile(profilesDir string) http.HandlerFunc

// handleProfileSave handles POST /api/profiles. Decodes a Profile,
// stamps CreatedAt if missing, validates, saves, returns the saved
// profile with stamped fields.
func handleProfileSave(profilesDir string) http.HandlerFunc
```

Dispatch wrapper in `main.go` (mirrors the `/api/files` split):

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

`handleProfile` parses the name segment via
`strings.TrimPrefix(r.URL.Path, "/api/profiles/")`, calls
`ValidateProfileName(name)` (returns 400 if it fails — this also
catches embedded slashes), then dispatches:

- `GET` → `LoadProfile` → on `errors.Is(err, fs.ErrNotExist)` 404,
  on other error 500, on success `jsonResponse(w, 200, profile)`.
- `DELETE` → `DeleteProfile` → same error mapping →
  `{"status":"deleted"}`.
- Anything else → 405.

`handleProfileSave` decodes into a transient struct
`{Name, Comment, Settings, SourceAssetID}` (no `CreatedAt` from the
client — server-stamped), constructs a `Profile`, stamps
`CreatedAt`, calls `SaveProfile`. On `Validate()` failure return
400 with the validator's error string. On disk failure return 500.
On success return the full `Profile`.

## `main.go` — modifications

Two changes:

1. Add `profilesDir := filepath.Join(workDir, "profiles")` to the
   directory variable block (line ~58).
2. Add `profilesDir` to the `for _, d := range []string{...}`
   MkdirAll loop (line ~61).
3. Register the two routes immediately after `/api/settings/`
   registration (after line 125).

## `static/index.html` — additions

Insert a new `.settings-section` block right after the existing
`tuningSection` div (after line 304), still inside
`.settings-panel`. Skeleton:

```html
<div class="settings-section" id="profilesSection">
    <h3>Profiles</h3>
    <div class="setting-row">
        <select id="profileSelect">
            <option value="">(select a profile)</option>
        </select>
    </div>
    <div class="setting-row profile-actions">
        <button id="profileApplyBtn" disabled>Apply</button>
        <button id="profileDeleteBtn" disabled>Delete</button>
    </div>
    <div class="setting-row">
        <button class="preset-btn" id="profileSaveOpenBtn">Save current as profile…</button>
    </div>
    <div class="setting-row" id="profileSaveForm" style="display:none">
        <input type="text" id="profileNameInput" placeholder="profile-name (kebab-case)">
        <div class="profile-name-error" id="profileNameError"></div>
        <textarea id="profileCommentInput" rows="3" placeholder="What worked here?"></textarea>
        <div class="profile-form-actions">
            <button id="profileSaveSubmitBtn">Save</button>
            <button id="profileSaveCancelBtn">Cancel</button>
        </div>
    </div>
</div>
```

## `static/app.js` — additions

New module-level state:

```js
let profilesList = [];          // ProfileMetadata[] from server
const PROFILE_NAME_RE = /^[a-z0-9]+(-[a-z0-9]+)*$/;
```

New DOM refs (near the existing tuning refs):

```js
const profileSelect       = document.getElementById('profileSelect');
const profileApplyBtn     = document.getElementById('profileApplyBtn');
const profileDeleteBtn    = document.getElementById('profileDeleteBtn');
const profileSaveOpenBtn  = document.getElementById('profileSaveOpenBtn');
const profileSaveForm     = document.getElementById('profileSaveForm');
const profileNameInput    = document.getElementById('profileNameInput');
const profileNameError    = document.getElementById('profileNameError');
const profileCommentInput = document.getElementById('profileCommentInput');
const profileSaveSubmitBtn= document.getElementById('profileSaveSubmitBtn');
const profileSaveCancelBtn= document.getElementById('profileSaveCancelBtn');
```

New functions (added near `populateTuningUI`):

- `async function loadProfileList()` — `GET /api/profiles`,
  populates `profilesList`, redraws the `<select>` (preserving the
  current selection if it still exists), calls
  `updateProfileButtons()`.
- `function updateProfileButtons()` — disables apply/delete when
  the select has the placeholder value.
- `async function applySelectedProfile()` — fetches
  `/api/profiles/:name`, PUTs `profile.settings` to
  `/api/settings/:selectedFileId`, refreshes `currentSettings`
  via `loadSettings`, calls `populateTuningUI()`, fires
  `logEvent('profile_applied', {profile_name: name}, selectedFileId)`.
- `async function deleteSelectedProfile()` — `confirm()`, then
  `fetch DELETE`, then `loadProfileList()`.
- `function openSaveProfileForm()` — toggles the form visible,
  resets inputs, focuses the name input.
- `async function submitSaveProfile()` — validates the name client
  side, POSTs `{name, comment, settings, source_asset_id}`, on
  success closes the form and `loadProfileList()` and fires
  `logEvent('profile_saved', {profile_name: name}, selectedFileId)`.
  On 400 from the server, surfaces the response error in
  `profileNameError`.

Wiring (added near the existing settings event listeners block at
the bottom of the file, after `wireTuningUI()` is called):

```js
profileSelect.addEventListener('change', updateProfileButtons);
profileApplyBtn.addEventListener('click', applySelectedProfile);
profileDeleteBtn.addEventListener('click', deleteSelectedProfile);
profileSaveOpenBtn.addEventListener('click', openSaveProfileForm);
profileSaveCancelBtn.addEventListener('click', () => {
    profileSaveForm.style.display = 'none';
});
profileSaveSubmitBtn.addEventListener('click', submitSaveProfile);
profileNameInput.addEventListener('input', () => {
    const ok = PROFILE_NAME_RE.test(profileNameInput.value)
        && profileNameInput.value.length <= 64;
    profileNameInput.classList.toggle('invalid', !ok && profileNameInput.value !== '');
});

// Initial population
loadProfileList();
```

The profile section's apply/delete buttons depend on
`selectedFileId` only indirectly: applying with no asset selected
is a no-op guarded inside `applySelectedProfile`. The save button
is similarly guarded.

## `static/style.css` — additions

Small additions for the new section's controls. Reuse existing
`.settings-section`, `.setting-row`, `.preset-btn` styles. New
classes:

- `.profile-actions` — flex row, gap 8px, two buttons fill width.
- `#profileNameInput.invalid` — red border (`var(--error)`).
- `.profile-name-error` — red text, 11px, hidden when empty.
- `.profile-form-actions` — flex row, right-aligned, gap 8px.
- `#profileCommentInput` — full width, monospace, dark bg matching
  the panel.

## `docs/knowledge/analytics-schema.md` — additions

In §"Event types (v1)", append two subsections after `discard`:

```markdown
### `profile_saved`

Emitted when the user saves the current `AssetSettings` as a named
profile.

```json
{ "profile_name": "round-bushes-warm" }
```

| Field          | Type   | Required | Notes                                   |
|----------------|--------|----------|-----------------------------------------|
| `profile_name` | string | yes      | Kebab-case, 1-64 chars. Server-validated. |

### `profile_applied`

Emitted when the user applies a saved profile to the current asset.

```json
{ "profile_name": "round-bushes-warm" }
```

| Field          | Type   | Required | Notes                                   |
|----------------|--------|----------|-----------------------------------------|
| `profile_name` | string | yes      | Kebab-case, 1-64 chars.                 |
```

In §"Versioning and migration policy", note that this addition is
backwards-compatible and `schema_version` stays at 1.

In §"Out of scope (deferred)", strike the "Profile artifacts" line
the way T-003-02 struck the auto-instrumentation line.

## Ordering of changes (commit boundaries)

1. **Backend data layer**: `profiles.go` + `profiles_test.go`.
   Compiles and tests pass; no behavior change to anything else.
2. **Backend HTTP layer**: `analytics.go` (allow-list), `handlers.go`
   (four handlers), `main.go` (dir + routes). Backend is fully
   functional via curl at this point.
3. **Frontend**: `static/index.html` + `static/app.js` +
   `static/style.css`. UI is reachable.
4. **Schema doc**: `analytics-schema.md` updates.

Each step is independently committable and revertable. Steps 1
and 2 can be merged at the same commit if it makes the diff
cleaner; the boundary is suggested for review ergonomics, not
forced by dependencies.
