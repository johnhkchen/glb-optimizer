# Review — T-002-03: tuning-panel-ui-skeleton

## Summary

Added a "Tuning" section to the right panel of `static/index.html`
exposing one control per `AssetSettings` field, wired into the
existing `currentSettings` / `loadSettings` / `saveSettings` plumbing
that T-002-02 prepared. Eleven controls total (nine sliders, two
selects), a Reset-to-defaults button, a subtle dirty-state dot in the
section header, and a debounce drop from 500 ms → 300 ms (per ticket
AC). No backend changes.

## Files Changed

| File                | Action  | Notes                                                              |
|---------------------|---------|--------------------------------------------------------------------|
| `static/index.html` | MODIFY  | New `<div class="settings-section" id="tuningSection">` block (+95 lines) |
| `static/app.js`     | MODIFY  | Extracted `makeDefaults()`; added `TUNING_SPEC`, `populateTuningUI`, `wireTuningUI`, `updateTuningDirty`; one-line `selectFile` hook; three-line init; debounce 500 → 300 |
| `static/style.css`  | MODIFY  | New `.dirty-dot` / `.dirty-dot.dirty` rules                         |
| `docs/active/work/T-002-03/*.md` | CREATE | Full RDSPI artifact set (this set)                       |

No deletions. No new files outside the work artifacts. No changes to
any Go file, no changes to `docs/knowledge/settings-schema.md`.

## Acceptance Criteria — Status

- [x] New "Tuning" section in `static/index.html`'s right panel,
      below existing settings sections.
- [x] One control per `AssetSettings` field (eleven total):
  - [x] `volumetric_layers` → range 1–12
  - [x] `volumetric_resolution` → dropdown 256/512/1024
  - [x] `dome_height_factor` → slider 0.0–1.0
  - [x] `bake_exposure` → slider 0.5–2.5
  - [x] `ambient_intensity` → slider 0.0–2.0
  - [x] `hemisphere_intensity` → slider 0.0–2.0
  - [x] `key_light_intensity` → slider 0.0–3.0
  - [x] `bottom_fill_intensity` → slider 0.0–1.5
  - [x] `env_map_intensity` → slider 0.0–3.0
  - [x] `alpha_test` → slider 0.0–0.5
  - [x] `lighting_preset` → dropdown ({default} only)
- [x] Each control shows its current value next to it (live update on
      drag).
- [x] Selecting a file populates the controls from `currentSettings`
      via `populateTuningUI()` after `loadSettings()`.
- [x] Changing a control updates `currentSettings` and triggers a
      debounced PUT (300 ms).
- [x] "Reset to defaults" button restores schema defaults and saves.
- [x] Re-baking via existing buttons consumes the tuned settings
      (the bake call sites already read from `currentSettings`,
      unchanged from T-002-02).
- [x] Dirty-state dot next to "Tuning" header when settings differ
      from defaults.
- [x] Live preview is NOT implemented (per AC: explicitly out of scope).
- [x] Existing presets and Mesh/Texture/Output sections unchanged.
- [x] Layout is additive — Tuning section scrolls with the rest of
      the right panel using the existing CSS idiom.

## Test Coverage

### Automated
- `go build ./...` — PASS.
- `go test ./...` — PASS (`ok glb-optimizer (cached)`).
- `node --check static/app.js` — PASS (no syntax errors).
- The Go test suite is unaffected; no Go files changed.

### Manual (deferred to human reviewer)
The plan's §"Manual smoke test" walks through:
1. File select → controls populate.
2. Slider drag → live readout + dirty dot + debounced PUT.
3. Bake (Production Asset) → reflects tuned values.
4. Reset → controls + dirty dot revert; PUT writes defaults.
5. Multi-file round-trip.
6. Unchanged Mesh/Texture/Output sections.

No headless browser available in this session.

### Coverage gaps
- **No JS tests at all**, same as T-002-02. Adding test infra for one
  ticket would dwarf the change. Recommend a follow-up scoped to
  S-008.
- **No automated visual smoke** of the new section's rendered layout.
- **No HTTP-handler test** for the GET/PUT roundtrip (still
  outstanding from T-002-01's review).

## Open Concerns

1. **`makeDefaults()` is still hand-synced with `DefaultSettings()`**
   in `settings.go`. This ticket *narrows* the duplication — only one
   JS literal exists now, used by both `applyDefaults` and the dirty
   compare — but it does not eliminate it. A `/api/settings/defaults`
   endpoint would resolve this; out of scope here, recommended as a
   follow-up. Mitigation: every field is also a JSON field that the
   load/save round trip exercises, so a typo would surface as a
   visible bake regression on first selection.

2. **300 ms debounce is global, not per-control.** A user dragging
   multiple sliders in sequence within 300 ms each will still trigger
   exactly one PUT after the last edit, which is fine. A user
   *holding* one slider for several seconds also triggers exactly one
   PUT 300 ms after release. Both are correct. Worth flagging only
   because the previous T-002-02 placeholder was 500 ms — any
   consumer that depended on 500 ms (none in tree) would now see
   shorter latency.

3. **`updateTuningDirty()` uses strict equality** (`!==`) against
   `makeDefaults()`. Floats round-tripped through `parseFloat` are
   exact for the values our schema uses, so this is fine in
   practice. A user editing the JSON file by hand to e.g.
   `0.5000000001` would show "dirty" forever; acceptable for a
   single-user dev tool.

4. **Reset writes the canonical defaults to disk** rather than
   deleting the settings file. This means after Reset, the file's
   record will still report `HasSavedSettings = true` on the
   backend. The semantics differ from "never tuned" but match the
   user's mental model ("I just set it to defaults"). If desired,
   a follow-up could add a true "delete saved settings" affordance.

5. **The dirty dot is invisible until the user has actually tuned
   something**, which means a user opening a file with previously
   saved (non-default) settings will see the dot lit up with no
   recent action of their own. This is intentional — the dot
   represents "diverges from defaults", not "unsaved" — but worth
   confirming with the human reviewer that the intent matches.

6. **Validator allows `volumetric_resolution` ∈ {128, 256, 512,
   1024, 2048}**, but the dropdown only exposes `{256, 512, 1024}`
   (per ticket AC). A settings file edited by hand to `128` or
   `2048` will load fine but the dropdown will display blank for
   that value (the `<select>` has no matching `<option>`). Low-risk;
   the user would have to be deliberately editing JSON.

7. **`lighting_preset` dropdown has only one option.** Per ticket
   ("only 'default' for now; S-007 fills it in"). The control is in
   place so S-007 can populate it via DOM additions without
   restructuring the wiring.

8. **No retry / no error toast** on failed PUT. The existing
   `saveSettings` warns to console and falls through. A user
   tweaking sliders in an offline tab would see no UI signal. Same
   limitation as T-002-02; acceptable for a single-user dev tool.

## Things a Human Reviewer Should Check

- **Run the smoke test in `plan.md`.** Specifically: tune `bake_exposure`
  to ~1.5, click "Production Asset", confirm the bake brightens. Then
  Reset and re-bake — the result should match the pre-T-002-03 default
  output.
- **Confirm `index.html` layout** — the Tuning section should appear
  below "Output", and the right panel should still scroll cleanly. The
  page should not extend below the viewport in an unexpected way on a
  standard monitor.
- **Confirm the dirty dot** is subtle and uses `--accent`. If it
  visually clashes, tweak `.dirty-dot.dirty`'s background.
- **Spot-check `TUNING_SPEC`** in `app.js` — the eleven entries must
  match the eleven schema fields and the eleven HTML ids exactly.
  A typo here would silently disable one control.
- **Confirm the `selectFile` hook** at the only call site — one new
  line, `populateTuningUI();`, after the existing
  `await loadSettings(id);`.
- **Confirm the debounce drop** is intentional (500 → 300).

## Known Limitations (Not Bugs)

- The JS defaults literal (`makeDefaults`) must be hand-synced with
  `settings.go`'s `DefaultSettings()`.
- The dropdown for `volumetric_resolution` exposes only the three
  values from the ticket, not the full validator-allowed set.
- Reset writes defaults to disk (rather than deleting the file).
- No live re-render on slider drag — the user must click a generate
  button to see the effect of tuning. (Explicit non-goal in the
  ticket.)
- No tooltips, named profiles, undo, or unsaved-changes warning.
  All deferred to S-003 / S-007 / S-008.

## Handoff to Lisa / Reviewer

The implementation is complete and self-tested with the available
tooling (`go build`, `go test`, `node --check`). The four-commit
sequence proposed in `plan.md` is the recommended landing strategy.

The next ticket in the chain is whatever S-002 specifies after
T-002-03 — likely a real visual-tuning workflow that uses this
skeleton, or the long-deferred backend defaults endpoint as a
follow-up to remove the JS-literal hand-sync.

No further plumbing in `app.js` is required to support the eventual
"named profile" or "preset" extensions — they can be wired purely as
new buttons that mutate `currentSettings` and call
`populateTuningUI()` + `saveSettings()`.
