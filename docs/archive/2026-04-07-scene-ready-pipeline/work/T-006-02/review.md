# Review — T-006-02: scene template implementations + UI

## What changed

| File | Lines | Notes |
|---|---|---|
| `settings.go` | +35 / -2 | Three new persisted fields, validation, normalization, differ-from-defaults audit |
| `settings_test.go` | +123 / 0 | Six new test cases covering defaults, validation, persistence round-trip, legacy doc migration |
| `analytics.go` | +1 / 0 | New `scene_template_selected` event in the allow-list |
| `docs/knowledge/analytics-schema.md` | +21 / 0 | New event-type section above `strategy_selected` |
| `static/app.js` | ~+170 / -50 | Five new template generators, ground plane init, scene preview UI helpers, picker/count/ground change handlers, settings hydration, init wiring |
| `static/index.html` | +6 / -3 | New picker / count input / ground checkbox in `.stress-controls`; Run button text updated |
| `static/style.css` | +20 / 0 | New `.scene-select` and `.scene-count` rules |

No new files. No deletions. Schema version is unchanged
(additive change, mirroring the T-004-02 / T-004-03 pattern).

## Acceptance criteria check

| AC bullet | Status |
|---|---|
| `hedge-row` template | ✅ single row along X, fixed orientation, tighter spacing |
| `mixed-bed` template | ✅ scatter, 0.85-1.15× scale variation |
| `rock-garden` template | ✅ sparser scatter, 0.7-1.3× scale variation |
| `container` template | ✅ tight cluster, clamped to 5-10 instances |
| `grid` template | ✅ rename of T-006-01 `benchmark`, byte-for-byte same math |
| Toolbar template dropdown | ✅ `<select id="sceneTemplateSelect">`, populated from registry |
| Toolbar instance count input | ✅ `<input type="number" id="sceneInstanceCount">`, range 1-500 |
| "Run scene" button | ✅ button text updated, click handler reads from new input |
| Ground plane toggle | ✅ `<input type="checkbox" id="sceneGroundToggle">` |
| Ground plane (textured plane at Y=0, brown) | ✅ `MeshStandardMaterial(#6b5544)`, 100×100 m, hidden by default |
| Persistence in `currentSettings` | ✅ three fields added to `AssetSettings`, mirrored in `makeDefaults()`, hydrated in `selectFile`/`applyDefaults` |
| Template change emits `scene_template_selected` analytics | ✅ payload `{from, to, instance_count, ground_plane}` |
| Scene preview uses same lighting preset as bake | ✅ `MeshStandardMaterial` picks up the live scene lighting that `applyPresetToLiveScene` (T-007-02) drives |
| Manual: rose + `mixed-bed` → scattered varied bushes | ⏳ **Pending live-browser human run** |
| Manual: trellis + `hedge-row` → believable row (T-004-05) | ⏳ **Pending live-browser human run** |

## Test coverage

**Go side:** new test cases added to `settings_test.go`:

- `TestDefaultSettings_SceneFields` — defaults assertion
- `TestValidate_AcceptsAllSceneTemplates` — round-trips every
  registered template id
- `TestValidate_RejectsBadSceneTemplate` — `"spirals"` rejected
- `TestValidate_RejectsSceneCountOutOfRange` — `0`, `-1`,
  `501`, `1000` rejected
- `TestSettingsDifferFromDefaults_SceneFields` — each field
  individually flagged dirty
- `TestLoadSettings_OldDocMissingSceneFields` — legacy doc
  loads with normalized defaults and validates
- `TestSaveLoad_RoundtripSceneFields` — full save/load cycle
  preserves the three new fields

`go test ./...` is clean.

**JS side:** no new automated tests. The repo has no JS test
infrastructure (no `package.json`, no test runner) — same
constraint T-006-01 inherited. `node --check static/app.js`
passes; manual verification (below) is the only path to
end-to-end confidence.

## Manual verification gate (mirrors the AC)

A human reviewer must walk this checklist before merging.
None of these can be exercised without a live browser session.

1. Boot, select any asset. Picker shows `Grid (benchmark)`,
   count shows `100`, ground unchecked.
2. Click `Run scene` with count 100. Expect the legacy 100×
   grid layout — must be visually identical to pre-T-006-02
   benchmark behavior (the `grid` template is a line-for-line
   port).
3. Switch picker to `Mixed Bed`, set count `50`, click Run
   scene. Expect ~50 scattered instances with light scale
   variation. On a `round-bush` asset (rose) Y rotation
   should also vary.
4. Switch to `Hedge Row`, set count `8`, click Run scene.
   Expect a straight row of 8 instances along X, all facing
   the same way (orientation overridden to `fixed`).
5. Switch to `Rock Garden`, count `30`, Run. Expect a sparser
   scatter with bigger scale variation (0.7-1.3×).
6. Switch to `Container`, count `100`, Run. Expect 10
   instances (clamped from 100 → 10) in a tight cluster.
7. Toggle the Ground checkbox. A brown plane should appear
   at Y=0 under the instances and respond to the active
   lighting preset.
8. Switch the lighting preset (right-panel tuning UI). The
   ground plane should recolor along with the model — same
   shading, same shadows-or-lack-thereof.
9. Switch to a second asset. The picker / count / ground
   restore the second asset's saved values (or defaults).
   Switch back: the first asset's values restore.
10. Reload the page. The most recent asset's saved values
    survive.
11. Open the asset's tuning JSONL. Each picker change should
    have produced one `scene_template_selected` event with
    `from`, `to`, `instance_count`, `ground_plane` keys.
    Count input changes and ground toggles should NOT have
    emitted their own events (their resting values ride
    along in `session_end.final_settings`).
12. **Trellis test from T-004-05**: load a `directional`
    trellis asset. Pick `Hedge Row`, count 6. Expect a
    believable straight line of trellises all facing the
    same way.

## Open concerns / known limitations

1. **Manual verification has not been performed by Claude.**
   The AC requires a live-browser walkthrough. The reviewer
   must walk the checklist above before merging.

2. **LOD path still uses `Vector3[]`** — per-instance scale
   variation does not propagate when the LOD checkbox is on.
   T-006-01's review.md flagged this as a known limitation;
   T-006-02 inherits it. `mixed-bed` + LOD = scattered
   positions but uniform scale.

3. **Production hybrid path also uses `Vector3[]`** — same
   reason. Same limitation. The LOD migration is out of
   scope for both T-006-01 and T-006-02.

4. **Side billboards camera-face every frame** — rotation
   variation is invisible in billboard side mode. Top quads
   honor rotation, scale honored on all variants. Property
   of the existing billboard rendering.

5. **`debug-scatter` was deleted entirely.** It was a
   framework smoke test from T-006-01, never user-facing.
   If anyone has muscle memory for `setSceneTemplate('debug-scatter')`
   in devtools, that call now silently no-ops with a warning.

6. **The `benchmark` template id is gone.** Renamed to
   `grid`. The T-006-01 review.md called out
   `setSceneTemplate('benchmark')` as a temporary
   pre-T-006-02 hook, so the rename was anticipated. No
   external scripts depend on the old id.

7. **Container template clamping is silent.** If the user
   types `100` with `Container` selected and clicks Run,
   they get 10 instances. The clamp is documented in the
   number input's `title` attribute but there is no
   per-template UI hint. Considered adding a "max for this
   template" readout but it complicated the toolbar; the
   FPS overlay's `Instances:` row already shows the
   effective count, which is the source of truth.

8. **Ground plane is 100×100 m.** That covers any practical
   asset footprint at the templates' default counts. A
   pathological 500-instance `mixed-bed` of a very large
   asset could overrun the plane. If this becomes a problem,
   the plane size could be derived from
   `boundsFromSpecs(specs)` at Run time, but that adds
   coupling between the persistent ground plane and the
   ephemeral stress state.

9. **No analytics on count or ground toggle changes.**
   Intentional (see design.md): the resting values are
   captured in `session_end.final_settings`, and emitting
   on every spin-button click would generate noise. If
   downstream training discovers it needs per-change
   timestamps, the change handlers are the right place to
   add `setting_changed` events later.

10. **Schema version not bumped.** Additive change matching
    the T-004-02 / T-004-03 pattern. Forward-compat
    normalization in `LoadSettings()` covers legacy files.

11. **Init order required tweaking.** `populateScenePreviewUI`
    sets `sceneTemplateSelect.value`, which silently fails
    on an empty `<select>`. Boot order is now
    `populateScenePreviewSelect()` → `applyDefaults()` (which
    calls `populateScenePreviewUI`) so the options exist
    before the value is set. This is a small footgun for
    future contributors — flagged here so the init block
    isn't reordered casually.

## Files diff (textual)

```
 settings.go                              | +35 / -2
 settings_test.go                         | +123 / 0
 analytics.go                             |  +1 / 0
 docs/knowledge/analytics-schema.md       | +21 / 0
 static/app.js                            | ~+170 / -50
 static/index.html                        |  +6 / -3
 static/style.css                         | +20 / 0
 docs/active/work/T-006-02/{research,
   design,structure,plan,progress,
   review}.md                             | new
```

## What a human reviewer should focus on

1. **Manual verification (the AC's two named scenarios).**
   The rose + mixed-bed visual check and the trellis +
   hedge-row check are the only end-to-end gates this work
   has. Both require a live browser.

2. **Template parameter feel.** The `span = spread *
   sqrt(count) * 1.4` formula for `mixed-bed` (and `2.2`
   for `rock-garden`) is a reasonable first guess, not a
   tuned value. Designer feedback may push these. Same for
   the min-distance multipliers (`0.9`, `1.6`, `0.7`).

3. **Container's silent clamping.** If a designer expects
   to see N instances after typing N, the clamp may
   surprise them. The FPS overlay's `Instances:` row shows
   the effective count, but only post-Run.

4. **Ground plane material vs. lighting presets.** The
   `MeshStandardMaterial` choice (over `MeshBasicMaterial`)
   is what makes "consistency from S-007" work, but it
   means the plane participates in every lighting tweak.
   That's the design intent; flagged in case the reviewer
   wants the plane to be a lighting-independent slab.

5. **The init-order requirement.** See open concern #11.
   The boot block order matters now in a way it didn't
   before — `populateScenePreviewSelect` must run before
   `applyDefaults`. A regression here would silently break
   the picker hydration.
