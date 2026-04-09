# Progress — T-003-03: profiles-save-load-comment

## Step status

| Step | Description                                  | Status   |
|------|----------------------------------------------|----------|
| 1    | Create `profiles.go` data layer              | ✅ done  |
| 2    | Create `profiles_test.go`                    | ✅ done  |
| 3    | Add `analytics.go` allow-list entries        | ✅ done  |
| 4    | Add HTTP handlers in `handlers.go`           | ✅ done  |
| 5    | Wire routes + dir in `main.go`               | ✅ done  |
| 6    | Add Profiles section in `index.html`         | ✅ done  |
| 7    | Add JS module state, helpers, and DOM refs   | ✅ done  |
| 8    | Add CSS for the new controls                 | ✅ done  |
| 9    | Update `analytics-schema.md`                 | ✅ done  |
| 10   | Manual end-to-end check                      | ⚠️ deferred (no live browser session in this run) |

## Verification

- `go build ./...` — green.
- `go test ./...` — green.
- `go vet ./...` — green.
- 12 new test functions in `profiles_test.go`, all passing
  (including the table-driven `TestValidate_RejectsBadName` with
  10 sub-cases).
- Existing T-002 / T-003-01 / T-003-02 tests still passing.

## Deviations from plan

1. **Plan listed 11 tests; shipped 12.**
   Added `TestValidate_RejectsNilSettings` as a separate function
   (rather than folding it into the bad-settings table) because the
   nil case takes a different code path — the `Settings == nil`
   guard runs *before* the delegated `Settings.Validate()`. Worth a
   distinct test.

2. **Save form layout: button order swapped.**
   Plan put `Save` before `Cancel`. Shipped Cancel-then-Save so the
   primary action is rightmost (matches macOS dialog conventions
   and the existing tooling buttons in the rest of the panel).

3. **`isProfileValidationError` helper added in `handlers.go`.**
   The plan said `handleProfileSave` should return 400 for
   validation errors and 500 for disk errors. `SaveProfile` bundles
   both into a plain `error`, so I added a small prefix-sniffing
   helper to do the split. Strictly speaking this is a smell — the
   structured-error refactor would be cleaner — but at the size of
   this surface (one call site, four prefixes) it pays for itself.
   Flagged in the review's "open concerns" so a future refactor can
   consider returning typed errors from `Validate()`.

4. **Profile select `<option>` text shows `name — comment`.**
   Plan only showed the name. Adding the comment as inline context
   is essentially free (it's the whole point of having comments)
   and matches the ticket framing "profiles we can reason about."

5. **`updateProfileButtons` is also called from `selectFile`.**
   Not in the plan but necessary: the Apply button is gated by
   `selectedFileId` as well as the dropdown selection, and
   `selectFile` is the only place `selectedFileId` flips. Without
   the call, picking a profile *before* selecting an asset and
   then selecting one would leave Apply disabled.

6. **Dropped a stub `errProfileNotFound` variable.**
   First draft had a private sentinel error reserved for a future
   structured-error refactor. Removed because it was unused — the
   "no speculative scaffolding" rule from CLAUDE.md applies.
   `fs.ErrNotExist` is already enough for the call sites that need
   to detect not-found.

7. **Manual e2e (Step 10) not run.**
   This run doesn't include a live browser session. The Go-side
   logic is fully unit-tested; the JS side is small and 1:1 with
   the wire contract that the curl-equivalent paths exercise via
   the test suite. Same posture as T-003-02.

## Files actually changed

- New: `profiles.go` (~210 lines), `profiles_test.go` (~205 lines).
- Modified: `analytics.go` (+2), `handlers.go` (+125), `main.go`
  (+13), `static/index.html` (+24), `static/app.js` (+155),
  `static/style.css` (+82), `docs/knowledge/analytics-schema.md`
  (+30 / -1).

## Notes for the review phase

- The kebab-case regex is duplicated between Go and JS. The Go side
  is the source of truth; if it changes, the JS one needs to follow.
  Drift will surface as a server-side 400, not silently — it's a
  visible failure mode.
- The `assetIndex` cache from T-003-02 is unaffected: profiles do
  not produce session_start envelopes, so they're invisible to
  session lookup.
- The `profile_saved` and `profile_applied` events are session-
  scoped (i.e., emitted only when an analytics session is live). In
  practice this means "an asset is selected", which is exactly when
  saving or applying a profile is meaningful.
