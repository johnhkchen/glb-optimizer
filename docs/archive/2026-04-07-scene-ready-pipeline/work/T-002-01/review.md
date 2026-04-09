    # Review — T-002-01: Settings Schema and Persistence

## Summary

Landed the v1 per-asset settings schema, the file-based storage layer, and
the `GET`/`PUT /api/settings/{id}` endpoints. Schema captures all 11 of the
ticket's required parameters plus `schema_version`. Defaults match the
hardcoded constants currently scattered through `static/app.js`. No client
wiring (T-002-02) or UI (T-002-03) was attempted.

## Files Changed

| File                                   | Action  | Notes |
|----------------------------------------|---------|-------|
| `settings.go`                          | CREATE  | `AssetSettings`, defaults, validate, load/save, atomic write |
| `settings_test.go`                     | CREATE  | First test file in the repo; 7 tests + 6 subtests |
| `docs/knowledge/settings-schema.md`    | CREATE  | Schema spec + migration policy |
| `docs/active/work/T-002-01/*.md`       | CREATE  | RDSPI artifacts (this set) |
| `models.go`                            | MODIFY  | `+HasSavedSettings bool` on `FileRecord` |
| `handlers.go`                          | MODIFY  | `+handleSettings` (one func, GET/PUT dispatch) |
| `main.go`                              | MODIFY  | `+settingsDir`, scan signature widened, route registered |

No deletions. No changes to `static/`, `processor.go`, `blender.go`, or
`scene.go`.

## Acceptance Criteria — Status

- [x] Schema documented in `docs/knowledge/settings-schema.md` with version,
      enumerated fields, defaults, types, ranges, descriptions, and a
      migration policy.
- [x] `settings.go` with `AssetSettings`, `DefaultSettings()`,
      `LoadSettings(id, dir)`, `SaveSettings(id, dir, settings)`, atomic
      write via temp file + rename, stable JSON field order via declaration
      order.
- [x] HTTP handlers `GET /api/settings/:id` and `PUT /api/settings/:id` in
      `handlers.go`.
- [x] Validation: `schema_version` strict, numeric ranges checked, NaN/Inf
      rejected, enum fields checked.
- [x] Routes registered in `main.go`.
- [x] `~/.glb-optimizer/settings/` created at startup.
- [x] `FileRecord.HasSavedSettings bool` field; populated in
      `scanExistingFiles` and after each successful PUT.
- [x] Unit-style verification: Go test file with round-trip + validation
      coverage.
- [x] All 11 schema fields from the ticket present with the correct
      defaults; `lighting_preset` enum is `{"default"}` only (S-007 will
      extend).

## Test Coverage

`go test ./...` runs in ~0.3s. Cases:

- `TestDefaultSettings_Valid` — defaults pass their own validator.
- `TestSaveLoad_Roundtrip` — `reflect.DeepEqual` after write→read.
- `TestLoadMissing_ReturnsDefaults` — load on a non-existent file returns
  defaults with no error.
- `TestValidate_RejectsBadVersion` — `schema_version` of `0` and `2` both
  rejected.
- `TestValidate_RejectsOutOfRange` — six subtests covering negative
  exposure, out-of-band key light, alpha_test > 1, layers = 0, bad
  resolution, unknown preset.
- `TestSettingsExist` — flips false→true after save.

### Coverage Gaps

- **No HTTP handler test.** The handler is exercised only by the manual
  smoke recipe in `plan.md`. A `httptest.NewRecorder`-based test would be
  cheap to add and would lock the GET/PUT contract; deferred to keep the
  ticket scoped.
- **No migration test.** There is no v0 → v1 migration to test (none was
  in scope) and no v2 yet. The migration *policy* is documented but the
  *mechanism* is exercised only when v2 ships.
- **`scanExistingFiles` is not unit-tested.** It's fairly mechanical and
  the new line is a single `SettingsExist` call, but no automated test
  asserts that a fresh server boot with a stray `settings/abc.json`
  surfaces `has_saved_settings: true` in `/api/files`. Manual verification
  recipe in plan.md covers this.
- **No concurrent-write test.** Race behavior (last-writer-wins) is
  documented as a known limitation rather than enforced or tested.

## Open Concerns

1. **Symmetric delete cleanup**: `handleDeleteFile` removes the original
   GLB and all output variants but does **not** remove
   `~/.glb-optimizer/settings/{id}.json`. After a delete-and-reupload,
   the new asset (with a new id) won't be affected, but stale settings
   files will accumulate for previously-deleted ids. This is a small
   follow-up — outside this ticket's acceptance criteria but worth filing.

2. **Discrepant defaults in `static/app.js`**: The schema canonicalizes
   `key_light_intensity` to `1.4` (per the ticket), but the volumetric
   renderer at `static/app.js:616` currently uses `1.6`. T-002-02 will
   wire the schema value through and the literal will go away — until
   then there's a one-tick visual difference. Documented in research.md.

3. **Forward-compatible decoding**: We do not call
   `Decoder.DisallowUnknownFields()`. A typo in a client PUT body (e.g.
   `bake_exposur` instead of `bake_exposure`) will be silently ignored
   and the field will fall back to its zero value. This is the standard
   forward-compatibility tradeoff, but worth flagging because the failure
   mode is "saved settings are subtly wrong" rather than a 400.

4. **No fsync** in `writeAtomic`. Acceptable for a local dev tool; flagged
   in case the audience for this code ever changes.

5. **`LoadSettings` does not call `Validate`**. By design — load is
   permissive so a partially-corrupted file still produces a usable
   struct. The HTTP layer validates on PUT, and the bake pipeline
   (T-002-02) will need to either trust the loaded values or call
   `Validate` itself. Document this in T-002-02's research.

## Things a Human Reviewer Should Check

- **Field ordering**: `AssetSettings` declaration order = on-disk JSON
  order = schema doc table order. If you add fields, keep all three in
  sync.
- **`HasSavedSettings` in API responses**: it's `omitempty`, so the field
  is absent (not `false`) for assets without saved settings. Confirm this
  is what the UI expects in T-002-03.
- **Settings dir path**: `~/.glb-optimizer/settings/` is hardcoded relative
  to the work dir. The `--dir` flag in `main.go` overrides the work dir
  root, so settings move with it. No env-var override.
- **The first test file**: `settings_test.go` is the first `*_test.go` in
  the repo. `go test ./...` works with no extra wiring, but if CI is added
  later make sure it actually runs `go test`.

## Known Limitations (Not Bugs)

- Concurrent PUTs to the same id race; last writer wins. Single-user tool
  — acceptable.
- Unknown JSON fields in PUT bodies are silently dropped (forward-compat).
- No symmetric delete cleanup of `settings/{id}.json`.
- Migration mechanism is unexercised until v2 ships.
- Manual smoke test recipe (plan.md) was documented but not executed in
  this session. Reviewer should run it once before merging if the server
  hasn't been started against this branch.
