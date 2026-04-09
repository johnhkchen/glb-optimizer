# T-010-02 Research — Combine Implementation

## Scope

Implement `CombinePack(side, tilted, volumetric, meta) ([]byte, error)` that
merges up to three intermediate `.glb` files into a single Pack v1 GLB matching
the contract frozen in T-010-01 / E-002.

## Existing GLB Touchpoints in Repo

The repo currently treats GLB as an opaque artifact almost everywhere. There is
exactly one place that parses the binary container directly:

- `scene.go` `CountTrianglesGLB(path string) (int, error)` (lines ~84–140)
  - Reads file → checks 12-byte header magic `0x46546C67` ("glTF")
  - Reads first chunk header (length + type), asserts type is JSON
    (`0x4E4F534A`)
  - Decodes the JSON chunk into a tiny anonymous struct that knows about
    `accessors[].count` and `meshes[].primitives[].indices`
  - Does not look at the BIN chunk at all — only uses accessor counts to
    estimate triangle count
  - Reads from disk (`os.ReadFile`); takes `path string`

This is the only existing reader. There is no existing GLB writer in the repo.

The `processor.go` / `blender.go` paths shell out to Python and gltfpack to
produce GLBs; the Go side only counts triangles after the fact.

## What `CombinePack` Inherits vs. Must Add

Inherits from `scene.go`:
- Constants for the magic numbers (`glTF`, `JSON`, `BIN\0`) — currently inline
  literals; can either lift them out or duplicate
- The "JSON chunk decode" pattern — the Combine implementation needs much more
  of the glTF schema (nodes, scenes, meshes, accessors, bufferViews, buffers,
  materials, textures, images, samplers) but the bytes-in / json.Unmarshal
  shape is the same

Must add:
- Binary writer for the GLB container (header + JSON chunk + BIN chunk)
- Re-offsetting of `bufferView.byteOffset` when concatenating BIN chunks
- Re-offsetting of `bufferView.buffer` when merging multiple intermediate
  buffers into the single output buffer (Pack v1 mandates one buffer)
- Index remapping for every cross-reference inside the gltf JSON
  (`accessor.bufferView`, `mesh.primitives.attributes/indices/material`,
  `material.pbrMetallicRoughness.baseColorTexture.index`,
  `texture.source/sampler`, `node.mesh`, `scene.nodes`)
- Construction of four named group nodes (`view_side`, `view_top`,
  `view_tilted`, `view_dome`) parented under one root node, with the
  intermediates' meshes hung as children
- SHA256-based image dedup
- Embedding of `meta.ToExtras()` under the root scene's `extras.plantastic`
- Hard cap of 5 MiB on the output

## glTF / GLB Container Spec — Just-Enough Reference

GLB binary layout (little-endian):

```
[ 0..3 ] magic     uint32  = 0x46546C67   ("glTF")
[ 4..7 ] version   uint32  = 2
[ 8..11] length    uint32  = total file length

For each chunk:
  [0..3] chunkLength uint32  (length of chunk DATA, excluding this header)
  [4..7] chunkType   uint32  (0x4E4F534A "JSON" or 0x004E4942 "BIN\0")
  [8..]  chunkData   bytes
```

Padding rules (mandatory):
- JSON chunk data must be padded with **trailing 0x20 (space)** bytes to a
  multiple of 4
- BIN chunk data must be padded with **trailing 0x00** bytes to a multiple
  of 4
- Chunk lengths recorded in the header are the *padded* lengths

A valid Pack v1 GLB has exactly two chunks: JSON then BIN (BIN may be empty
length=0 but the chunk header must still appear if any bufferView refers to
buffer 0; for our case BIN will always be non-empty because mesh data exists).

The glTF JSON top-level object we care about:

```jsonc
{
  "asset": { "version": "2.0", "generator": "..." },
  "scene": 0,
  "scenes": [ { "nodes": [0], "extras": { "plantastic": { ... } } } ],
  "nodes": [ ... ],
  "meshes": [ ... ],
  "accessors": [ ... ],
  "bufferViews": [ ... ],
  "buffers": [ { "byteLength": N } ],   // exactly one for Pack v1
  "materials": [ ... ],
  "textures": [ ... ],
  "images": [ ... ],   // bufferView-backed images (no external URIs)
  "samplers": [ ... ]
}
```

`buffers[0].uri` is **omitted** in GLB-embedded files; the renderer treats
`buffer 0` as "the BIN chunk".

`images[i]` may either be `bufferView`-backed (`{ "bufferView": k,
"mimeType": "image/png" }`) or `uri`-backed (data URL or external). The
intermediates produced by gltfpack are bufferView-backed in practice, but
robust merging must handle both forms even if it just preserves data URIs
verbatim.

## Mesh Routing Rules (from ticket)

- **side intermediate (required):**
  - The mesh whose `name == "billboard_top"` → reparented under `view_top`,
    renamed to `view_top` itself (per ticket: "child of new view_top node
    (renamed to view_top)")
  - All other meshes → children of `view_side`, renamed `variant_0`,
    `variant_1`, … in the order they appear in the source `meshes` array
- **tilted intermediate (optional):**
  - All meshes → children of `view_tilted`, renamed `variant_0`, …
- **volumetric intermediate (optional):**
  - All meshes → children of `view_dome`, renamed `slice_0`, `slice_1`, …
    sorted by **min Y of each mesh's bounding box**, ascending (bottom→top)
  - Bounding box derivation: read `accessor.min[1]` for each primitive's
    POSITION accessor and take the per-mesh minimum across primitives.
    (glTF spec mandates `min`/`max` on POSITION accessors, so this is safe.)

## Material vs. Texture Sharing

The ticket explicitly requires:

> Each mesh keeps its own material instance even if textures are shared
> (consumer needs per-variant opacity).

Reading `app.js` createBillboardInstances confirms why: the consumer mutates
material `opacity` per variant during the crossfade. Sharing material objects
would cause cross-variant opacity bleed.

So the dedup rule is:
- **images**: deduped by SHA256(content) — one `images[i]` shared across all
  textures whose underlying bytes match
- **textures**: not deduped by us. Each material continues to point at
  whatever `textures[]` entry the merge produces. Textures may or may not
  collapse depending on whether their `(image,sampler)` pairs land on the
  same merged image — we leave them per-source for simplicity.
- **materials**: never deduped; copied verbatim per intermediate

## Constraints & Gotchas Surfaced from Ticket Notes

1. **Do not mutate inputs.** `side`, `tilted`, `volumetric` are `[]byte`
   parameters. Implementation must work on copies / parsed structures, never
   slice into the input and write through.
2. **Slice ordering for view_dome must be deterministic** even if two slices
   have identical Y values (ties broken by original mesh index).
3. **5 MiB cap.** Returns error after assembling output if `len(out) >
   5*1024*1024`. The ticket says "structured error" is the cap pattern but
   the structured `PackTooLargeError` type belongs to T-010-05 — for this
   ticket a plain `fmt.Errorf` with the size is sufficient.
4. **`meta.Validate()` must be called before write.** Not after — we don't
   want to spend cycles writing a 4 MiB file just to reject it.
5. **Test fixtures synthesized in-memory.** No dependence on real baked
   assets. The test writes minimal valid GLBs (one mesh, one accessor, one
   bufferView, no textures) using the same writer the implementation uses,
   then feeds them back through `CombinePack`.

## Out of Scope (verbatim from ticket)

- HTTP endpoint (T-010-03)
- Bake-time meta capture (T-011-02)
- Texture format conversion (PNG stays PNG)

## Open Questions / Assumptions

- **Animations / skins / cameras / lights**: the bake intermediates do not
  contain any of these. Assumption: drop silently if present, don't try to
  remap. If they ever appear, acceptance test will fail loudly because
  unknown fields stay in the JSON via `json.RawMessage` passthrough.
- **Multiple `scenes[]` in input**: assume each intermediate has exactly
  one scene (`scene 0`). Combine ignores anything beyond `scenes[0]`.
- **Sparse accessors**: not present in our intermediates (gltfpack doesn't
  emit them for static geometry). Sparse accessor `indices.bufferView` and
  `values.bufferView` would need remapping too — flagged as a known
  limitation in the review if we ever encounter them.
