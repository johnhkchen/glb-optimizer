# Asset Settings Schema (v1)

The per-asset settings document captures every parameter that drives the
volumetric bake and related lighting behavior. One file per asset, stored
on disk, served via `GET`/`PUT /api/settings/{id}`.

This document is the source of truth for the schema. The Go struct
`AssetSettings` in `settings.go` mirrors it field-for-field.

## Versioning

```
schema_version: 1
```

The schema is versioned with a single integer. Bump on any breaking change
(field rename, type change, removal). Additive changes (new optional fields)
do **not** require a version bump because the loader is forward-compatible
on unknown fields.

## Fields

| Field                    | Type    | Default     | Range / Enum                | Description |
|--------------------------|---------|-------------|-----------------------------|-------------|
| `schema_version`         | int     | `1`         | `1`                         | Schema version of this document. Must equal the server's current version. |
| `volumetric_layers`      | int     | `4`         | `[1, 16]`                   | Base horizontal slice count for the volumetric bake. The renderer may adapt this upward (+1 or +2) for tall, narrow models; the saved value is the **base**, not the adapted count. |
| `volumetric_resolution`  | int     | `512`       | `{128,256,512,1024,2048}`   | Texture resolution per slice, in pixels. |
| `dome_height_factor`     | float   | `0.5`       | `[0.0, 2.0]`                | Multiplier on per-slice thickness used to dome each layer quad. `0.0` = flat quads. |
| `bake_exposure`          | float   | `1.0`       | `[0.0, 4.0]`                | Tone-mapping exposure (`toneMappingExposure`) applied to the bake renderer. |
| `ambient_intensity`      | float   | `0.5`       | `[0.0, 4.0]`                | Intensity of the ambient light in the bake scene. |
| `hemisphere_intensity`   | float   | `1.0`       | `[0.0, 4.0]`                | Intensity of the hemisphere light in the bake scene. |
| `key_light_intensity`    | float   | `1.4`       | `[0.0, 8.0]`                | Intensity of the top-down directional key light. |
| `bottom_fill_intensity`  | float   | `0.4`       | `[0.0, 4.0]`                | Intensity of the bottom-fill directional light. |
| `env_map_intensity`      | float   | `1.2`       | `[0.0, 4.0]`                | Multiplier applied to material `envMapIntensity` in the bake scene. |
| `alpha_test`             | float   | `0.10`      | `[0.0, 1.0]`                | Alpha test threshold baked into volumetric and billboard quad materials at export time. (Note: a separate runtime override at instance-creation time uses tighter values for billboard/volumetric instances; that is not governed by this field.) |
| `lighting_preset`        | string  | `"default"` | `{"default", "midday-sun", "overcast", "golden-hour", "dusk", "indoor", "from-reference-image"}` | Named lighting preset (T-007-01, T-007-03). Picking a preset overwrites the dependent intensity fields (`ambient_intensity`, `hemisphere_intensity`, `key_light_intensity`, `bottom_fill_intensity`, `env_map_intensity`, `bake_exposure`) and supplies the colors used by the bake pipeline. The `from-reference-image` preset (T-007-03) is special: when selected and the asset has an uploaded reference image, the bake/preview palette is derived from the image's dominant colors via the existing palette extraction; without an image its `bake_config` is a neutral baseline. Full preset definitions live in `static/presets/lighting.js`; the backend only validates membership. |
| `slice_distribution_mode`| string  | `"visual-density"` | `{"equal-height","vertex-quantile","visual-density"}` | How `renderHorizontalLayerGLB` places per-slice Y boundaries. `equal-height` = linear interpolation across the bounding box (legacy simple slicing). `vertex-quantile` = legacy adaptive picker; boundaries fall at vertex-count quantiles. `visual-density` = trunk-filtered, radial-weighted quantile that biases boundaries toward visible foliage rather than dense lower geometry (T-005-01). |
| `ground_align`           | bool    | `true`      | `true` / `false`            | When true, the volumetric export scene is translated so the bottom slice's floor sits exactly at `Y=0`, preventing leaves from clipping into the scene preview ground (T-005-01). |
| `reference_image_path`   | string  | `""`        | free string                 | Optional tag pointing to the asset's reference image on disk (e.g. `outputs/{id}_reference.png`). Set automatically by the client after a successful upload to `/api/upload-reference/:id`. Not dereferenced server-side; the image is served by `/api/reference/:id` (T-005-03). |

Numeric ranges are intentionally permissive — they exist to catch typos and
NaN/infinity, not to enforce taste. UI sliders (T-002-03) will set tighter
*recommended* clamps.

## JSON Example

```json
{
  "schema_version": 1,
  "volumetric_layers": 4,
  "volumetric_resolution": 512,
  "dome_height_factor": 0.5,
  "bake_exposure": 1.0,
  "ambient_intensity": 0.5,
  "hemisphere_intensity": 1.0,
  "key_light_intensity": 1.4,
  "bottom_fill_intensity": 0.4,
  "env_map_intensity": 1.2,
  "alpha_test": 0.10,
  "lighting_preset": "default",
  "slice_distribution_mode": "visual-density",
  "ground_align": true
}
```

Field declaration order in `AssetSettings` determines field order on disk;
do not reorder casually.

## Storage

- **Path**: `~/.glb-optimizer/settings/{id}.json`, where `{id}` matches the
  hex id used by `originals/{id}.glb` and `outputs/{id}.glb`.
- **Directory**: created on server startup if missing.
- **Atomicity**: writes go through `os.CreateTemp` in the same directory
  followed by `os.Rename`, which is atomic on the same filesystem (POSIX,
  including macOS APFS).
- **Encoding**: UTF-8 JSON, 2-space indent, trailing newline.
- **Concurrency**: this is a single-user local tool. Two simultaneous PUTs
  to the same id race; the last writer wins. Acceptable for the use case.

## Endpoints

| Method | Path                  | Body                  | Response |
|--------|-----------------------|-----------------------|----------|
| `GET`  | `/api/settings/{id}`  | —                     | `200` + `AssetSettings`. Returns `DefaultSettings()` if no file exists yet. |
| `PUT`  | `/api/settings/{id}`  | `AssetSettings` JSON  | `200` + canonical (post-validation) `AssetSettings`. `400` on decode/validation errors. `404` if the asset id is not registered. |

The `FileRecord` returned by `GET /api/files` includes a derived
`has_saved_settings: true` field for assets with a settings file on disk,
to support a "tuned" indicator in the UI later.

## Migration Policy

The server reads a settings file in three steps:

1. **Inspect** `schema_version`.
2. **Compare** to the server's `SettingsSchemaVersion` constant.
3. **Branch**:
   - **Equal** → validate and use as-is.
   - **Lower** → run the registered migrator chain
     (`migrateV1toV2`, `migrateV2toV3`, …) to bring the document up to the
     current version, then validate.
   - **Higher** → log a warning and fall back to defaults. **Never silently
     downgrade** — a forward-version document may encode semantics this
     server can't honor.

Migration runs inside `LoadSettings` so callers always observe the current
shape. There is no v0; this ticket lands the schema and there are no
pre-existing files to migrate from.

### Forward-compat normalization

Some additive fields cannot use Go's zero value as their migration
default. `LoadSettings` runs a normalization pass *after* JSON decode
and *before* `Validate` for these cases:

- `slice_distribution_mode`: empty string (key absent) → `"visual-density"`.
  The empty string would otherwise fail enum validation.
- `ground_align`: key absent → `true`. To distinguish "explicit false"
  from "absent" the loader re-decodes the same byte slice into a tiny
  struct with `*bool` so `nil` means "no key on disk". An explicit
  `"ground_align": false` is preserved.
- `color_calibration_mode` → `lighting_preset` (T-007-03): the legacy
  T-005-03 field has been removed from `AssetSettings`. On load, if a
  document still carries `"color_calibration_mode": "from-reference-image"`
  AND the explicit `lighting_preset` is the bare default `"default"`,
  the loader rewrites `lighting_preset` to `"from-reference-image"`.
  An explicit non-default preset is treated as a user override and
  wins. The same `*pointer` re-decode trick is used so a key that is
  absent vs. present-but-empty can be distinguished.

These normalizations are **not** schema-version bumps; the on-disk
shape is unchanged for any document that already includes the keys.

### Adding fields without a version bump

A purely additive field (new optional value with a sensible zero/default)
does **not** require a version bump. Old documents will load; the new
field reads as its zero value. Always provide a default for the new field
in `DefaultSettings()` so missing-from-disk and missing-in-file behave
identically.

### When to bump the version

- A field is renamed, removed, or changes type.
- A field's semantics change (same name, different meaning).
- A default is changed in a way that would surprise existing users.

When you bump, write a migrator function and add it to the chain in
`LoadSettings`. Add a regression test that loads a fixture document at the
old version and asserts it converts cleanly.

## Out of Scope (v1)

- Profiles or named bundles (S-007).
- Analytics events on settings change (S-003).
- Tighter recommended ranges per UI slider (T-002-03).
- Symmetric cleanup of `settings/{id}.json` when an asset is deleted.
