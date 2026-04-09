# Design — T-003-03: profiles-save-load-comment

## Decision summary

Build a `profiles.go` Go module that mirrors `settings.go` almost
field-for-field: `Profile` struct with explicit JSON tag order, a
`Validate()` method, `LoadProfile/SaveProfile/ListProfiles/DeleteProfile`
file ops keyed by sanitized name, and `writeAtomic()` reused from
`settings.go` (it is package-private and already in scope). Wire four
HTTP endpoints in `handlers.go` and one route registration block in
`main.go`. On the frontend, add a small "Profiles" subsection inside the
existing tuning panel (no modal), wire it to the four endpoints, fire
`profile_saved` / `profile_applied` analytics events through the
existing `logEvent` helper, and add the two new event types to the
`validEventTypes` allow-list and to `analytics-schema.md`.

## Approaches considered

### A. Mirror `settings.go` (chosen)

Lift the entire shape of `settings.go`: a struct with JSON tags, a
`Validate()` method, `Load/Save` functions that take a directory
argument, atomic writes via the existing helper, defaults via a
package-level constructor.

**Pros**

- Zero conceptual overhead. Anyone reading `settings.go` already
  knows the pattern.
- Reuses `writeAtomic` directly (same package, no export needed).
- Mirrors the existing test idiom in `settings_test.go` 1:1, so the
  test file basically writes itself.
- Adding `name` validation is local to one method.

**Cons**

- Slight duplication of the file-IO scaffolding (open / decode /
  marshal / write). At ~150 lines this is well below the threshold
  where extracting a helper would save more than it costs.

### B. Generic `keyed_store.go` abstraction

Extract a generic `KeyedJSONStore[T]` that both settings and profiles
use. Removes the duplication noted in A's "cons".

**Pros**

- DRY-er. One bug fix or atomic-write change covers both stores.

**Cons**

- Adds an abstraction the codebase doesn't have anywhere else and
  doesn't ask for. The project's idiom so far is "small flat
  packages, one file per concern." Settings and profiles share
  *some* mechanics but diverge on validation, defaults, listing
  semantics (settings has no list method), and the "metadata
  projection" pattern that profiles needs and settings does not.
- Generics for two callsites is a bad trade — the saved lines are
  about equal to the lines spent on the abstraction itself.
- T-002 reviews explicitly favored "small flat" over "premature
  abstraction." Reusing settings as a model rather than a helper is
  the more honest match.

**Verdict**: rejected. Reach for this only if a third store appears.

### C. Single combined file `~/.glb-optimizer/profiles.json`

One JSON file containing a map of `name → Profile`. Each save
rewrites the whole file under a lock.

**Pros**

- Atomicity is one file rename.
- Single read for `ListProfiles`.

**Cons**

- Concurrent saves from two browser tabs would need explicit
  locking (settings has the same problem and ignores it because
  it's per-asset; profiles is global so the collision surface is
  larger).
- Larger blast radius if the file is corrupted — losing one bad
  profile takes down the whole list.
- Doesn't compose with the ticket's "Profile sharing" out-of-scope
  note ("use the file directly for now") — that note implies one
  file per profile, drag-n-drop friendly.

**Verdict**: rejected. The per-file layout is what the ticket
already presumes.

## Profile data model

```go
type Profile struct {
    SchemaVersion  int            `json:"schema_version"`
    Name           string         `json:"name"`
    Comment        string         `json:"comment"`
    CreatedAt      string         `json:"created_at"`        // RFC 3339 UTC
    SourceAssetID  string         `json:"source_asset_id"`
    Settings       *AssetSettings `json:"settings"`
}
```

Notes:

- `SchemaVersion` mirrors `SettingsSchemaVersion`. Profiles do not
  introduce a second versioning axis — the version reflects the
  embedded `AssetSettings` shape and is bumped at the same time.
  Profile-level metadata fields (name, comment) can be added
  additively without bumping.
- `CreatedAt` is a string, not `time.Time`, to match the project's
  existing pattern for timestamp fields in the analytics envelope
  (`analytics.go:46` uses `string`). Format: `time.RFC3339Nano`.
- `Settings` is a pointer to `AssetSettings` so we can detect a
  missing block as `nil` rather than as a zero-value struct that
  passes `Validate()` only by accident.
- `SourceAssetID` is best-effort breadcrumb data — the asset id the
  user happened to be tuning when they saved. It's not used for
  anything in v1 and may be empty if we ever add a "save from
  defaults" path. Carrying it now is cheap and unlocks T-003-04
  follow-up analysis ("which assets generated which profiles").

## Validation rules

`(*Profile).Validate()`:

1. `SchemaVersion == SettingsSchemaVersion` — same enforcement as
   `AssetSettings`. Reject otherwise.
2. `Name` matches `^[a-z0-9]+(-[a-z0-9]+)*$` and `1 <= len(Name) <= 64`.
3. `Comment` length `<= 1024` (silent guard against pathological
   input — the textarea has no client-side cap).
4. `Settings != nil` and `Settings.Validate() == nil`. Delegate
   field-level checks to the existing settings validator.
5. `CreatedAt` non-empty (server fills it on save if the client
   omits it; see Save flow below).
6. `SourceAssetID` is **not** validated against the file store —
   profiles outlive the assets they were derived from, and stale
   ids are valid history.

The kebab-case regex is the load-bearing rule because it also
doubles as the on-disk filename safety check: by definition the
regex disallows `..`, `/`, leading dots, and any character that
needs shell quoting.

## Storage layout

```
~/.glb-optimizer/
    profiles/
        round-bushes-warm.json
        cool-dome.json
        ...
```

- One file per profile, name = `{Name}.json`.
- Created at startup by `main.go` (same `MkdirAll` block as
  `tuning/`).
- File contents are the marshaled `Profile` struct, 2-space indent,
  trailing newline.
- The on-disk filename is derived from the validated `Name` field,
  which means there is exactly one canonical filename per profile
  and the name validation also covers the filesystem safety check
  (no `..`, no `/`).

## Function-level interface

```go
// profiles.go (package main)
const ProfilesSchemaVersion = SettingsSchemaVersion

func DefaultProfile(name string) *Profile          // skeleton with defaults
func (p *Profile) Validate() error
func ProfilesFilePath(name, dir string) string
func LoadProfile(name, dir string) (*Profile, error)        // 404 → fs.ErrNotExist
func SaveProfile(p *Profile, dir string) error              // overwrite OK
func ListProfiles(dir string) ([]ProfileMetadata, error)    // sorted by Name
func DeleteProfile(name, dir string) error
```

```go
// ProfileMetadata is the stripped projection used by ListProfiles.
// Excludes Settings to keep the list payload small.
type ProfileMetadata struct {
    Name          string `json:"name"`
    Comment       string `json:"comment"`
    CreatedAt     string `json:"created_at"`
    SourceAssetID string `json:"source_asset_id"`
}
```

`LoadProfile` validates the *name* before touching disk (so we never
do an `os.Open` with attacker-controlled paths even when callers
forget). It does *not* call `Validate()` on the loaded body — the
caller decides.

`SaveProfile` calls `Validate()`, then `MkdirAll(dir)`, then
`writeAtomic`. If `CreatedAt` is empty it stamps `time.Now().UTC()`.
If a profile with the same name exists, the rename overwrites it
silently — this is the "overwrite is fine for v1" rule from the
ticket.

`ListProfiles` reads the dir, decodes only the metadata fields per
file (using a small private `profileMetadataOnly` struct), skips
files that fail to decode (corrupt or partially-written), and sorts
the result by `Name` ascending.

## HTTP contract

| Verb   | Path                | Body                            | 200 response                              |
|--------|---------------------|---------------------------------|-------------------------------------------|
| GET    | `/api/profiles`     | —                               | `[]ProfileMetadata` (may be empty)        |
| GET    | `/api/profiles/:name` | —                             | full `Profile`                            |
| POST   | `/api/profiles`     | `{name, comment, settings, source_asset_id?}` | full `Profile` (with stamped CreatedAt) |
| DELETE | `/api/profiles/:name` | —                             | `{"status":"deleted"}`                    |

Errors:

- `400` — name fails kebab-case/length validation, body decode
  error, settings sub-validation error.
- `404` — `LoadProfile` / `DeleteProfile` returns `fs.ErrNotExist`.
- `405` — wrong method.
- `500` — disk error.

The `/api/profiles` and `/api/profiles/:name` split is handled the
same way `/api/files` does it in `main.go:103-111`: one
`mux.HandleFunc("/api/profiles", ...)` for the collection and one
`mux.HandleFunc("/api/profiles/", ...)` for the trailing-slash
single-resource path.

## Frontend integration

A new "Profiles" subsection is appended to the existing tuning
section in `index.html` (after the Reset button at line 302).
Layout, top to bottom:

1. Heading "Profiles".
2. `<select id="profileSelect">` populated from `GET /api/profiles`,
   with a sticky first option `(none selected)`.
3. Two buttons side by side: `Apply` and `Delete`. Both disabled
   until the select has a non-placeholder value.
4. Below them, a "Save current as profile…" button that toggles
   visibility on a small form: a kebab-case name input, a
   `<textarea>` for comments (3 rows, no rich text), and `Save` /
   `Cancel`. Inline validation on the name field shows a red
   border + helper text when the input fails the regex.
5. The whole subsection is hidden until `selectedFileId` is set,
   matching how the tuning sliders behave.

JS additions in `app.js`:

- `profilesState = { list: [], visible: false }` module-level.
- `loadProfileList()` — `GET /api/profiles`, repopulates the
  `<select>`. Called at init and after every save/delete.
- `applyProfile(name)` — `GET /api/profiles/:name`, then PUT to
  `/api/settings/:selectedFileId` with the profile's `settings`,
  then call the existing `loadSettings(selectedFileId)` and
  `populateTuningUI()` to redraw, then
  `logEvent('profile_applied', {profile_name: name}, selectedFileId)`.
- `saveCurrentAsProfile(name, comment)` — POST
  `{name, comment, settings: currentSettings, source_asset_id: selectedFileId}`,
  then `loadProfileList()`, then
  `logEvent('profile_saved', {profile_name: name}, selectedFileId)`.
- `deleteProfile(name)` — `confirm()` dialog, then DELETE, then
  `loadProfileList()`. No analytics event (per ticket).

The kebab-case regex lives in **both** the JS (for inline UX
feedback) and the Go validator (for safety). Drift is acceptable:
the Go side is the source of truth, the JS is a courtesy.

## Analytics extension

Add to `analytics.go`:

```go
var validEventTypes = map[string]bool{
    "session_start":   true,
    "session_end":     true,
    "setting_changed": true,
    "regenerate":      true,
    "accept":          true,
    "discard":         true,
    "profile_saved":   true,  // T-003-03
    "profile_applied": true,  // T-003-03
}
```

Add to `docs/knowledge/analytics-schema.md`:

- Two new entries under "Event types (v1)" with the
  `{profile_name: string}` payload shape and a one-line description.
- Note in "Versioning and migration policy" that this is an
  additive change and `schema_version` stays at 1.
- Update the "Out of scope (deferred)" footer to strike the
  T-003-03 line, matching how T-003-02 struck its own line.

## Why this design wins on the criteria

- **Faithful to existing patterns**: every choice has a precedent
  one file over (`settings.go` for the data layer, `handleSettings`
  for the HTTP layer, `wireTuningUI` for the JS).
- **Minimal blast radius**: adds files and lines, doesn't refactor
  anything. The only existing-file mutations are the
  `validEventTypes` map, the `main.go` MkdirAll loop, the route
  registrations, the schema doc, and the small UI insertion.
- **Testable**: mirroring `settings_test.go` gives 6+ tests for free
  with high confidence.
- **Reversible**: dropping `profiles/` and removing the four routes
  reverts the feature without affecting anything else.
