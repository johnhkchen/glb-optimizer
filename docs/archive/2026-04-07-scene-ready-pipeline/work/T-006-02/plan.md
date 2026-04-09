# Plan — T-006-02

## Step 1 — Settings schema (Go side)

**Where:** `settings.go`, `settings_test.go`.

**Add:**

- `AssetSettings.SceneTemplateId` / `SceneInstanceCount` /
  `SceneGroundPlane` fields.
- Defaults in `DefaultSettings()`.
- `validSceneTemplates` map (`grid`, `hedge-row`, `mixed-bed`,
  `rock-garden`, `container`).
- `Validate()` checks: known template, count in `[1,500]`.
- `LoadSettings()` forward-compat: empty template id → `"grid"`,
  zero count → 100, ground plane bool zero is correct migration.
- `SettingsDifferFromDefaults()` extends with the three new
  comparisons.

**Test:**

- `TestDefaultSettings_SceneFields` — defaults assertion.
- `TestValidate_AcceptsAllSceneTemplates` — every id in the
  validation map round-trips.
- `TestValidate_RejectsBadSceneTemplate` — `"foo"` rejected.
- `TestValidate_RejectsSceneCountOutOfRange` — `0`, `-1`, `501`,
  `1000` rejected.
- `TestSettingsDifferFromDefaults_SceneFields` — mutate each
  field individually, expect true.
- `TestLoadSettings_NormalizesLegacySceneFields` — write a JSON
  doc missing the three keys, load, assert defaults.

**Verify:** `go test ./...` clean.

**Commit:** `T-006-02: persist scene preview selection in AssetSettings`

---

## Step 2 — Analytics allow-list

**Where:** `analytics.go`, `docs/knowledge/analytics-schema.md`.

**Add:**

- `"scene_template_selected": true` in `validEventTypes`.
- New event-type section in analytics-schema.md (see structure.md).

**Verify:** `go test ./...` clean (existing analytics tests
should not regress).

**Commit:** `T-006-02: register scene_template_selected analytics event`

---

## Step 3 — JS template implementations

**Where:** `static/app.js` Scene Templates section (3122-3171).

**Replace** the `SCENE_TEMPLATES` registry contents with the
five new template definitions: `grid`, `hedge-row`, `mixed-bed`,
`rock-garden`, `container`. Keep all helpers unchanged.

**Delete** the `debug-scatter` template entry.

**Change** `activeSceneTemplate` default from `'benchmark'` to
`'grid'`.

**Verify:**

- `node --check static/app.js` clean.
- Devtools smoke test:
  - `__SCENE_TEMPLATES.grid.generate({bbox:{size:{x:1,y:1,z:1}}, orientationRule:'random-y', seed:0}, 4)` → 4 specs in a 2×2 layout.
  - `__SCENE_TEMPLATES['hedge-row'].generate({bbox:{size:{x:1,y:1,z:1}}, orientationRule:'fixed', seed:0}, 5)` → 5 specs along X axis, all `rotationY === 0`.
  - `__SCENE_TEMPLATES.container.generate({...}, 100)` → 10 specs (clamped).

**Commit:** `T-006-02: implement five scene preview templates`

---

## Step 4 — HTML + CSS scaffolding

**Where:** `static/index.html` (lines 65-74), `static/style.css`
(after line 435).

**Add** the `<select>`, `<input type="number">`, and `<input
type="checkbox">` to `.stress-controls`. **Remove** the
`stressCount` range slider and its value span.

**Add** `.scene-select` and `.scene-count` CSS rules.

**Rename** the Run button text from "Run" to "Run scene".

**Verify:** load the page, confirm the new controls appear and
the layout doesn't wrap badly. Buttons inert at this point.

**Commit:** `T-006-02: scene preview picker UI scaffolding`

---

## Step 5 — JS wiring (the behavioral commit)

**Where:** `static/app.js` — DOM refs section, `initThreeJS`,
`makeDefaults`, new helpers, event listeners section, init block.

**Add:**

1. Module-level `let groundPlane = null;` near line 36.
2. DOM refs: `sceneTemplateSelect`, `sceneInstanceCount`,
   `sceneGroundToggle`.
3. Ground plane creation in `initThreeJS()` after the
   `GridHelper` line (2820).
4. Three new keys in `makeDefaults()`: `scene_template_id: 'grid'`,
   `scene_instance_count: 100`, `scene_ground_plane: false`.
5. `populateScenePreviewSelect()` — populates the `<select>` from
   `SCENE_TEMPLATES` at boot.
6. `populateScenePreviewUI()` — hydrates the three controls
   from `currentSettings` and applies them to JS state.
7. Hydration call sites: invoke `populateScenePreviewUI()` from
   `selectFile`'s `loadSettings(...).then(...)` chain (just
   beside `populateTuningUI()`) and from `applyDefaults()`.
8. Change handlers for picker / count / ground (per
   structure.md).
9. Replace the stress button click handler to read from
   `sceneInstanceCount.value` instead of the deleted slider.
10. Remove the two lines in `clearStressInstances` that reset
    the deleted slider.
11. Init block: `populateScenePreviewSelect();`.

**Verify (manual, blocks the commit):**

1. Boot, select an asset. Picker shows `grid`, count shows 100,
   ground unchecked.
2. Switch to `mixed-bed`, count 50, click Run scene. Expect
   ~50 scattered instances with light scale variation.
3. Switch to `grid`, count 100, click Run scene. Expect the
   legacy 100x benchmark grid.
4. Switch to `hedge-row`, count 8, click Run scene. Expect a
   straight row.
5. `container`, count 100, Run. Expect 10 instances (clamp).
6. Toggle ground plane on; brown plane appears at Y=0.
7. Switch assets; values restore per-asset. Reload; values
   survive.
8. Tail the JSONL: each picker change emits one
   `scene_template_selected` event with `from`, `to`,
   `instance_count`, `ground_plane`.

**Commit:** `T-006-02: wire scene preview picker, ground plane, and persistence`

---

## Step 6 — Cleanup + manual verification

- Audit any leftover references to `stressCount` /
  `stressCountValue` / `stressSlider` / `stressValueEl` and
  remove them. (`clearStressInstances` and the init block were
  the known sites; grep for any others.)
- Audit any leftover references to `'benchmark'` /
  `'debug-scatter'` and remove or update them.
- Smoke-test all five templates against a `round-bush` asset
  (rose) and a `directional` asset (trellis from T-004-05) per
  the AC's "manual verification" lines. Document anything
  surprising in `progress.md` so review.md can flag it.

**Verify:**

- `go test ./...` clean.
- `node --check static/app.js` clean.
- Manual checklist above.

**Commit:** `T-006-02: scene preview cleanup pass`

---

## Testing strategy summary

| What | How | When |
|---|---|---|
| Settings schema additions | `go test ./...` (six new test cases) | Step 1 |
| Analytics allow-list | `go test ./...` (existing tests still pass) | Step 2 |
| Template generators | Devtools smoke test on each `generate(...)` | Step 3 |
| HTML/CSS layout | Visual inspection in browser | Step 4 |
| End-to-end wiring | Manual verification per AC | Step 5 |
| No leftover references | grep + final smoke test | Step 6 |

## Risks & mitigations

- **Renaming `benchmark` → `grid` could break devtools muscle
  memory.** Mitigation: T-006-01 review.md explicitly called
  out the temporary `setSceneTemplate('benchmark')` hook as a
  pre-T-006-02 affordance, so the rename is expected. No
  external scripts depend on it.
- **The number input is more clicks than the slider for
  minor count tweaks.** Mitigation: spin-buttons + `step=1`,
  comfortable width (56 px), keyboard arrows work.
- **Container template clamping is silent.** Mitigation:
  `title` attribute on the input notes that container clamps
  to [5,10]. Out of scope to add per-template UI hints.
- **Settings round-trip relies on additive schema (no
  bump).** Mitigation: existing pattern from T-004-02
  (`ShapeCategory`) and T-004-03 (`SliceAxis`) is followed
  identically; the forward-compat normalization in
  `LoadSettings` covers legacy files.
- **Ground plane is a `MeshStandardMaterial` and will pick up
  every lighting preset change.** That's the AC's
  "consistency from S-007" requirement, not a bug. If the
  preset changes during a stress test, the plane recolors
  along with the asset — same as today's `currentModel`
  behavior.
- **No automated JS tests.** Same as T-006-01. Manual
  verification gates the JS commits; the Go side has full
  test coverage.

## Step ordering rationale

Steps 1-4 are pure additions or scaffolding — safe to land
incrementally. Step 5 is the only commit that can regress
existing behavior (the stress button rewires onto the new
input, the picker/count/ground state hydrates from settings,
the ground plane appears in scene init). Keeping it isolated
makes bisect cheap. Step 6 catches any leftover dangling
references and exercises the manual checklist.
