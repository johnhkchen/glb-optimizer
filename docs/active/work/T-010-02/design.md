# T-010-02 Design — Combine Implementation

## Decision Summary

Implement `CombinePack` as a **typed-glTF, two-pass merge**:

1. Parse each input GLB into an in-memory `gltfDoc` struct that mirrors the
   parts of the glTF schema we touch (asset, scene, nodes, meshes,
   accessors, bufferViews, materials, textures, images, samplers).
2. Walk each input doc and copy its objects into a single output doc,
   remapping every cross-reference index as we go and concatenating the
   inputs' BIN payloads into one output BIN payload.
3. Write the result with a small custom GLB binary writer.

The whole thing lives in **one new file `combine.go`** plus a test file
`combine_test.go`. No third-party glTF library.

## Options Considered

### Option A — Shell out to gltfpack / gltf-transform

Use `gltf-transform merge` (Node.js CLI) under `os/exec`.

**Rejected.** The repo already shells out for bake steps but adding a
Node.js dependency for an in-process Go library function would block the
HTTP endpoint (T-010-03), which needs to call this in a request handler
without forking a process per request. Also: gltf-transform's merge does
not enforce our group node naming, doesn't run our metadata validation,
and would still need a post-processing pass.

### Option B — Use a third-party Go glTF library (`qmuntal/gltf`)

There is one mature Go module (`github.com/qmuntal/gltf`) that handles
GLB read/write.

**Rejected for this ticket** (but flagged as a possible follow-up).
Reasons:
- Adds an external dependency to a repo that currently has zero (only
  the standard library is in `go.mod`)
- The library's API is read-write-roundtrip; it doesn't help with the
  cross-file index remapping which is the actual hard part of the ticket
- The custom writer we need is ~50 lines; the index-remapping pass is
  the same complexity either way
- Pulling in a dep this late in the demo cycle (S19) is risk we can't
  amortize

### Option C — Custom typed merge (chosen)

Define a `gltfDoc` Go struct that mirrors the glTF JSON shape for the
fields Pack v1 cares about. Use `json.RawMessage` passthrough for fields
we don't manipulate (samplers, asset.extras, etc.) so we don't lose data
we don't understand. Write a small `readGLB` / `writeGLB` pair using
only `encoding/json` + `encoding/binary` + `crypto/sha256`.

**Why this wins:**
- Zero new dependencies — keeps `go.mod` clean
- The remapping pass is explicit and small enough to unit-test
- All the hard logic (mesh routing, texture dedup, BIN concatenation,
  size cap) lives in code we own and can tune for the demo
- Mirrors the `scene.go` pattern of "only parse what we touch"

## Architecture

```
combine.go
├── CombinePack(side, tilted, volumetric, meta) ([]byte, error)   // public API
│
├── gltfDoc, gltfNode, gltfMesh, gltfAccessor, gltfBufferView,
│   gltfImage, gltfTexture, gltfMaterial, gltfSampler              // schema types
│
├── readGLB(raw []byte) (*gltfDoc, []byte, error)                  // → doc, bin, err
├── writeGLB(doc *gltfDoc, bin []byte) ([]byte, error)             // → glb bytes
│
├── mergeContext { out *gltfDoc, outBin *bytes.Buffer,
│                  imageHashes map[[32]byte]int }                  // pass-1 state
│
├── (mc *mergeContext) absorb(in *gltfDoc, inBin []byte)           // copy + remap
│       returns indexMap{ accessor, bufferView, image, texture,
│                         material, mesh } so the caller can find
│                         the new mesh indices for routing
│
├── routeSideMeshes(mc, indexMap, in *gltfDoc) (sideGroup,
│                                                topGroup *gltfNode)
├── routeTiltedMeshes(mc, indexMap, in *gltfDoc) *gltfNode
├── routeVolumetricMeshes(mc, indexMap, in *gltfDoc) *gltfNode
│
└── attachExtras(mc, meta) // sets out.Scenes[0].Extras["plantastic"]
```

The four routers each take an `indexMap` (mapping per-source-mesh-index →
new merged-mesh-index) plus the source `gltfDoc` (so they can read mesh
names) and return one or two new group `*gltfNode` they have appended to
`out.Nodes`.

After all intermediates are absorbed and all routers have run, the root
node is built whose `children` are the four group nodes (omitting any
that came back empty), and that root is wired into `Scenes[0].Nodes`.

## Index Remapping Strategy

The merge is **strictly additive**: every object from every input is
appended to the output's parallel slice. The `absorb` pass returns an
`indexMap` of:

```go
type indexMap struct {
    accessor   []int  // src index → out index
    bufferView []int
    image      []int  // accounts for SHA256 dedup
    texture    []int
    material   []int
    mesh       []int
}
```

Order of operations within `absorb` matters because some types reference
each other:

1. **bufferViews** (must come first; they only reference `buffer 0`,
   which we rewrite to point at the output's single buffer; their
   `byteOffset` is shifted by the running `outBin.Len()` baseline taken
   *before* this input's BIN was concatenated)
2. **accessors** (rewrite `bufferView` index)
3. **images** (rewrite `bufferView` index OR keep `uri`; SHA256 dedup
   based on the resolved bytes)
4. **samplers** (no cross-references; copied verbatim)
5. **textures** (rewrite `source` and `sampler` indices)
6. **materials** (rewrite all texture indices found inside; copied
   verbatim otherwise)
7. **meshes** (rewrite per-primitive `attributes`, `indices`, `material`)

Nodes are NOT absorbed wholesale — we only build the four group nodes and
hang the absorbed meshes off them. This keeps node-graph parenting
simple: every absorbed mesh becomes a brand-new leaf node, and source
nodes' transforms are dropped (justification: bake intermediates have
identity-transform meshes; the bake step has already baked transforms
into vertices).

## SHA256 Image Dedup Mechanism

The `mergeContext` carries `imageHashes map[[32]byte]int`. When absorbing
an image:

```
1. Resolve image bytes:
   - if image.bufferView != nil, slice the bytes out of the input BIN
     using the *original* bufferView (before remap)
   - if image.uri starts with "data:", base64-decode the payload
   - if image.uri is external (no "data:" prefix), hash the URI string
     itself as a stand-in (external textures are not part of demo flow,
     but we don't crash on them)

2. Compute sha256 of those bytes.

3. If the hash is already in imageHashes, set indexMap.image[srcIdx] to
   the existing out index and skip the append. Otherwise:
   - Append the bytes to outBin (re-aligned to 4 bytes)
   - Create a new bufferView at the new offset
   - Append a new image{ bufferView: newBV, mimeType: src.mimeType }
   - Record indexMap.image[srcIdx] = len(out.Images)-1
```

This guarantees the consumer's renderer sees one image even if both
side and tilted intermediates were baked from the same source texture.

## Mesh Routing Detail — `view_top` Special Case

The ticket text is slightly ambiguous:

> child of new `view_top` node (renamed to `view_top`)

Reading carefully, what this means is: the `billboard_top` mesh becomes
the **only** child under a node named `view_top`, and that child node
itself is named `view_top` as well. The intent is that the consumer can
do `scene.getObjectByName('view_top')` and get either the group OR the
mesh — they're the same thing semantically. We implement it as:

```
group "view_top"
└── leaf "view_top" (mesh = billboard_top remapped)
```

If `billboard_top` is missing from the side input, `view_top` is omitted
entirely (no empty group).

## Volumetric Slice Ordering

For each volumetric mesh, we walk its primitives, find each primitive's
POSITION accessor, read `accessor.min[1]` (Y coordinate of bbox min), and
take the minimum across primitives. Tie-break is the original mesh index.
Sort is stable. This is deterministic across runs and across machines
because we read the values out of the JSON and don't recompute from
binary.

Children are renamed `slice_0`, `slice_1`, … in sorted order.

## Output Document Skeleton

```jsonc
{
  "asset":   { "version": "2.0", "generator": "glb-optimizer combine v1" },
  "scene":   0,
  "scenes":  [ { "nodes": [0], "extras": { "plantastic": {...} } } ],
  "nodes":   [
    { "name": "pack_root", "children": [1, 4, ...] },   // root
    { "name": "view_side", "children": [2, 3] },        // group
    { "name": "variant_0", "mesh": 0 },
    { "name": "variant_1", "mesh": 1 },
    { "name": "view_top",  "children": [5] },
    { "name": "view_top",  "mesh": 2 },                 // dual-named leaf
    ...
  ],
  "meshes":      [ ... merged ... ],
  "accessors":   [ ... merged & remapped ... ],
  "bufferViews": [ ... merged & remapped to buffer 0 ... ],
  "buffers":     [ { "byteLength": <total bin length> } ],
  "materials":   [ ... ],
  "textures":    [ ... ],
  "images":      [ ... deduped ... ],
  "samplers":    [ ... ]
}
```

## Error Cases (all return `(nil, error)`)

| Condition                                  | Error message |
|--------------------------------------------|---------------|
| `side == nil`                              | `combine: side intermediate is required` |
| Any input fails GLB header parse           | `combine: parse <which>: %w` |
| `meta.Validate()` fails                    | `combine: invalid meta: %w` |
| Final output > 5 MiB                       | `combine: pack size %d exceeds 5 MiB cap` |
| Image bufferView refers to missing index   | `combine: image %d: bufferView %d out of range` |

## Why This Design Will Work for the Demo

The demo path (S15–S19, plantastic Powell scene) needs **side + dome**
intermediates merged for 4 species (chair/planter/botanical scans aren't
through this pipeline). The mesh count per pack is small (≤8 side
variants + ≤16 dome slices), the texture count is small (1 atlas per
intermediate), and the demo runs offline on a dev laptop. The custom
typed merge handles this load without breaking a sweat — performance is
not a concern, correctness is.
