---
id: E-002
title: asset-pack-format-and-combine
type: epic
status: open
priority: critical
sprint: demo-day
stories: [S-010, S-011]
related_repo: plantastic E-028
---

## From Inedible Muffins to Edible Plates

### Thesis

The bake pipeline produces excellent hybrid impostors. The four-variant scheme — side billboards (N variants) + horizontal top + tilted billboards (N variants) + volumetric dome slices — is mobile-friendly and visually convincing across the full camera arc. The runtime preview here proves it works at 100+ instances under 50 MB.

The problem: those four variants live in **three** separate intermediate files (`{id}_billboard.glb`, `{id}_billboard_tilted.glb`, `{id}_volumetric.glb`), wired together by globals in `static/app.js`. There is no portable artifact a downstream renderer can consume. The plantastic scene viewer (the consumer) can load GLBs but has no way to receive the hybrid scheme as a coherent unit.

This epic adds the **asset pack** — one `.glb` per species, containing all four variants in named subtrees with metadata in `extras.plantastic`, ready for the consumer to load and render via the same hybrid math we already proved.

### Anchor Quote

> "The rendering scheme in this pipeline is great. Now we want that optimized system in the actual scene renderer."

### Why Now

Demo day. A landscaping company admin will see the Powell & Market scene (LIDAR backdrop) populated with realistic, varied plants drawn from this pipeline's bake outputs. The pitch is "drastically improved assets, mobile-friendly, cheap to render." Today the demo machine has no way to consume what we've baked.

### Scope of This Epic

**In scope:**
- Define and freeze the **Pack Format v1** (single GLB, four named subtrees, `extras.plantastic` metadata block)
- Build the **combine step** that takes the existing three intermediates and produces one pack
- Capture **bake-time settings** (`tilted_fade_low_start/end/high_start`, footprint dims, species id) and embed them in the pack's metadata
- Output to `dist/plants/{species}.glb` with predictable filenames (latin name, lowercase, underscore-separated)
- Hard 5 MB cap per pack, fail loud at bake time
- Justfile recipe to pack every asset in outputs in one shot

**Out of scope (explicit):**
- Centralized asset server / HTTPS publishing — manual USB drop is the v1 distribution
- KTX2/Basis texture compression — accept PNG-in-GLB for v1, optimize later
- Multi-elevation tilted billboards (single tilt only — already deferred from S-009)
- Mesh LODs — the rendering vocabulary is **only** the four impostor variants. No high-poly meshes ship in packs.
- Pack signing / checksums
- Per-species spacing override (consumer uses one bed-level spacing)
- Streaming / progressive loading
- Re-bake on the consumer side (consumer cannot regenerate, only receive)

### Coordination With plantastic E-028

The **Pack Format v1 spec below is the contract**. Any change to it requires updating both repos. Both epics reference this section as the source of truth.

Sequencing for demo day:
1. T-010-01 ships first — defines the schema both sides build against
2. T-010-02 (combine) and plantastic T-080-01 (loader) can develop in parallel against the schema
3. First real pack (one species, end-to-end) is the integration milestone — once one pack loads in plantastic, the rest is mechanical

The combine step writes packs to `dist/plants/`. A human carries that directory to the demo laptop on a USB drive and drops it into plantastic's `web/static/potree-viewer/assets/plants/`. No CLI coupling between repos.

---

## Pack Format v1 — The Contract

### File

One `.glb` per species. Filename is the species id: `{species_id}.glb` where `species_id` is lowercase latin name with underscores (e.g. `achillea_millefolium.glb`, `coffeeberry.glb`).

Hard size cap: **5 MB**. Combine fails if exceeded.

### Scene structure

```
scene
├── view_side    (Group, REQUIRED)   ← N variant Mesh children
│   ├── variant_0 (Mesh)
│   ├── variant_1 (Mesh)
│   └── ...
├── view_top     (Mesh,  REQUIRED)   ← single horizontal quad
├── view_tilted  (Group, OPTIONAL)   ← N variant Mesh children
│   └── variant_* (Mesh)
└── view_dome    (Group, OPTIONAL)   ← N slice Mesh children, ordered bottom→top
    └── slice_*  (Mesh)
```

Required: `view_side` and `view_top`. Optional: `view_tilted`, `view_dome`. Consumer detects optional groups and gracefully skips the corresponding instancer arrays. A flat groundcover may ship side+top only.

Variant counts are detected by counting children — no need to declare in metadata.

Each Mesh's material has its baked-view texture as `pbrMetallicRoughness.baseColorTexture`. Materials are NOT shared across variants — each variant keeps its own material so the consumer can drive opacity per variant independently.

### Metadata — `scene.extras.plantastic`

```json
{
  "format_version": 1,
  "bake_id": "2026-04-08T11:32:00Z",
  "species": "achillea_millefolium",
  "common_name": "Common Yarrow",
  "footprint": {
    "canopy_radius_m": 0.45,
    "height_m": 0.62
  },
  "fade": {
    "low_start": 0.30,
    "low_end": 0.55,
    "high_start": 0.75
  }
}
```

| Field | Required | Notes |
|---|---|---|
| `format_version` | yes | Integer. Bumped on breaking schema changes. v1 = this spec. |
| `bake_id` | yes | ISO 8601 timestamp of the bake. Reserved for future asset-server cache busting. |
| `species` | yes | Lowercase latin name, underscored. Must match the filename minus `.glb`. |
| `common_name` | yes | Human display string. |
| `footprint.canopy_radius_m` | yes | For layout / no-walk zones in the consumer. |
| `footprint.height_m` | yes | Consumer scales the unit-reference geometry by this value. |
| `fade.low_start` | yes | Crossfade band 1 start (lookDown 0..1). |
| `fade.low_end` | yes | Crossfade band 1 end. |
| `fade.high_start` | yes | Crossfade band 2 start. (band 2 end is implicit 1.0) |

Defaults if absent on the consumer side: `fade = {0.30, 0.55, 0.75}` matches `currentSettings` defaults in app.js.

### Reference scale

All variant geometries are baked to **unit reference**: 1m tall, canopy radius 1m. The consumer multiplies by `footprint.height_m` per instance and applies any per-instance scale jitter on top.

### Variant assignment

Per-instance variant selection happens **in the consumer** (deterministic by `(species_id, instance_index)`), NOT baked into the pack. The pack just ships N variants; the consumer buckets them across instances using the same `seededRandom` formula as `createBillboardInstances` in app.js:3892.

---

## Stories

| Story | Title | Tickets |
|-------|-------|---------|
| S-010 | pack-combine-step | T-010-01..05 |
| S-011 | pack-distribution-and-bake-capture | T-011-01..03 |

### Wave Order

- **Wave 0**: T-010-01 (schema). Blocks everything in both repos.
- **Wave 1**: T-010-02 (combine), T-011-02 (bake-time capture). Parallel.
- **Wave 2**: T-010-03 (UI button + endpoint), T-010-04 (justfile), T-010-05 (size cap), T-011-01 (dist output), T-011-03 (bake_id).
- **Integration milestone**: produce one real pack for one species, hand to plantastic team.

### Out of Scope (deferred to follow-up epics)

- HTTPS asset server / `publish` CLI — USB drop only for demo
- KTX2/Basis texture compression
- Multi-elevation tilted billboards
- Pack signing / integrity checks
- Re-bake on consumer side
- Cross-pack texture deduplication
- Per-species spacing overrides
- Hardscape (chair, planter) packs — this epic is plant-only
