# Progress — T-002-03: tuning-panel-ui-skeleton

## Plan steps — execution log

### Step 1 — CSS: dirty dot — DONE
Appended `.dirty-dot` and `.dirty-dot.dirty` rules to
`static/style.css` after the `.tooltip` rule. +14 lines, 0 deletions.
No existing rules touched.

### Step 2 — HTML: Tuning section — DONE
Inserted the `<div class="settings-section" id="tuningSection">` block
in `static/index.html` after the "Output" section, still inside
`.panel-right`. Eleven controls plus the Reset button. +95 lines, 0
deletions.

### Step 3 — JS: extract `makeDefaults()` — DONE
Pulled the literal out of `applyDefaults()` into a pure
`makeDefaults()` factory. `applyDefaults()` is now a one-liner that
assigns `currentSettings = makeDefaults()`. Behaviorally identical
to before.

### Step 4 — JS: debounce 500 → 300 — DONE
Single literal change in `saveSettings`. The only previous caller of
`saveSettings` was a console-only path (T-002-02 contract); the new
listeners in step 5 inherit the new 300 ms window.

### Step 5 — JS: Tuning UI block — DONE
Added `TUNING_SPEC`, `populateTuningUI`, `wireTuningUI`,
`updateTuningDirty` directly after `applyDefaults()`. ~75 lines.

### Step 6 — JS: hook `selectFile` — DONE
Added one line — `populateTuningUI();` — directly after the existing
`await loadSettings(id);` inside `selectFile`'s `loadEnv.then` block.

### Step 7 — JS: module init — DONE
Added three lines after `refreshFiles();` in the `// ── Init ──`
block: `applyDefaults(); wireTuningUI(); populateTuningUI();`. This
guarantees the tuning controls are bound and showing valid defaults
before any user interaction.

## Static verification — PASSED
- `go build ./...` — clean.
- `go test ./...` — `ok glb-optimizer (cached)`.
- `node --check static/app.js` — no syntax errors.

## Deviations from the plan
None. All seven steps executed as designed.

## Known things to verify manually
The plan's §"Manual smoke test" remains the recommended human gate
before merge:

1. Drop a file → Tuning controls populate from defaults.
2. Drag a slider → live readout, dirty dot lights up, ~300 ms later
   PUT fires.
3. Bake (Production Asset) → reflects the tuned values.
4. Reset → all controls revert, dirty dot off, immediate PUT.
5. Switch files → controls update to the new file's saved settings.
6. Existing Mesh/Texture/Output sections unchanged.

The session has no headless browser, so step 1–6 are deferred to the
human reviewer.

## Files modified

| File                 | +    | -  |
|----------------------|------|----|
| `static/style.css`   | +14  | -0 |
| `static/index.html`  | +95  | -0 |
| `static/app.js`      | +97  | -16|

(`app.js` `-16` is mostly the in-place rewrite of `applyDefaults()` →
`makeDefaults()`+`applyDefaults()` and the new listener block; no
existing logic was removed.)

## Files NOT modified
- All Go files.
- `docs/knowledge/settings-schema.md`.
- Any other static asset.

## Commits
Implementation lives uncommitted in the working tree. The plan
proposes four commits (CSS+HTML, makeDefaults refactor, debounce
constant, JS wiring + selectFile hook + init). Lisa or the human
reviewer can land them as preferred.
