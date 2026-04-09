# T-010-02 Structure — Combine Implementation

## Files Created

### `combine.go` (new, ~450 lines, package main)

Single Go file holding the entire combine implementation. Lives next to
`pack_meta.go` and `scene.go` in the project root, no subpackage —
matches the convention from T-010-01 (single file in package main, no
new subpackage; see memory observation 244).

### `combine_test.go` (new, ~350 lines, package main)

Companion tests. Builds synthetic minimal GLBs in-memory using the same
`writeGLB` helper the implementation uses, then drives them through
`CombinePack` and asserts on the resulting tree.

## Files Modified

None. `scene.go` is **not** refactored — its `CountTrianglesGLB` keeps
its inline magic number constants. We could share constants but the
duplication is 4 lines and the alternative is touching a file the bake
pipeline depends on, which isn't worth the merge-conflict surface for
the demo.

## Files Deleted

None.

## Public API Surface

```go
// CombinePack merges up to three intermediate GLB byte slices into a
// single Pack v1 asset pack GLB. side is required; tilted and volumetric
// may be nil. The returned bytes are a self-contained .glb whose root
// scene's extras.plantastic block matches meta. Returns an error if
// meta is invalid, any input fails to parse, or the final size exceeds
// the Pack v1 5 MiB cap.
func CombinePack(side []byte, tilted []byte, volumetric []byte, meta PackMeta) ([]byte, error)
```

That is the **only** exported symbol added. Everything else is
unexported and lives entirely inside `combine.go`.

## Internal Types (unexported, all in `combine.go`)

```go
// glTF JSON schema mirror — only fields we touch are typed; the rest
// passes through via json.RawMessage in the parent struct.
type gltfDoc struct {
    Asset       json.RawMessage     `json:"asset"`
    Scene       int                 `json:"scene"`
    Scenes      []gltfScene         `json:"scenes"`
    Nodes       []gltfNode          `json:"nodes,omitempty"`
    Meshes      []gltfMesh          `json:"meshes,omitempty"`
    Accessors   []gltfAccessor      `json:"accessors,omitempty"`
    BufferViews []gltfBufferView    `json:"bufferViews,omitempty"`
    Buffers     []gltfBuffer        `json:"buffers,omitempty"`
    Materials   []gltfMaterial      `json:"materials,omitempty"`
    Textures    []gltfTexture       `json:"textures,omitempty"`
    Images      []gltfImage         `json:"images,omitempty"`
    Samplers    []json.RawMessage   `json:"samplers,omitempty"`
}

type gltfScene struct {
    Nodes  []int          `json:"nodes"`
    Extras map[string]any `json:"extras,omitempty"`
}

type gltfNode struct {
    Name     string `json:"name,omitempty"`
    Mesh     *int   `json:"mesh,omitempty"`
    Children []int  `json:"children,omitempty"`
}

type gltfMesh struct {
    Name       string          `json:"name,omitempty"`
    Primitives []gltfPrimitive `json:"primitives"`
}

type gltfPrimitive struct {
    Attributes map[string]int `json:"attributes"`
    Indices    *int           `json:"indices,omitempty"`
    Material   *int           `json:"material,omitempty"`
    Mode       *int           `json:"mode,omitempty"`
}

type gltfAccessor struct {
    BufferView    *int            `json:"bufferView,omitempty"`
    ByteOffset    int             `json:"byteOffset,omitempty"`
    ComponentType int             `json:"componentType"`
    Normalized    bool            `json:"normalized,omitempty"`
    Count         int             `json:"count"`
    Type          string          `json:"type"`
    Min           []float64       `json:"min,omitempty"`
    Max           []float64       `json:"max,omitempty"`
}

type gltfBufferView struct {
    Buffer     int  `json:"buffer"`
    ByteOffset int  `json:"byteOffset,omitempty"`
    ByteLength int  `json:"byteLength"`
    ByteStride *int `json:"byteStride,omitempty"`
    Target     *int `json:"target,omitempty"`
}

type gltfBuffer struct {
    ByteLength int    `json:"byteLength"`
    URI        string `json:"uri,omitempty"`  // omitted in GLB
}

type gltfMaterial struct {
    // We re-encode materials by round-tripping the raw JSON through
    // a remap walker that rewrites every "index": int it finds inside
    // a "*Texture": {} object. Cheaper than typing the whole material
    // schema (PBR, KHR extensions, etc.) and forward-compatible.
    Raw json.RawMessage `json:"-"`
}

type gltfTexture struct {
    Source  *int `json:"source,omitempty"`
    Sampler *int `json:"sampler,omitempty"`
}

type gltfImage struct {
    BufferView *int   `json:"bufferView,omitempty"`
    MimeType   string `json:"mimeType,omitempty"`
    URI        string `json:"uri,omitempty"`
    Name       string `json:"name,omitempty"`
}

type indexMap struct {
    accessor   []int
    bufferView []int
    image      []int
    texture    []int
    material   []int
    mesh       []int
}

type mergeContext struct {
    out         *gltfDoc
    outBin      *bytes.Buffer
    imageHashes map[[32]byte]int
}
```

## Internal Function Inventory

| Function | Returns | Purpose |
|----------|---------|---------|
| `readGLB(raw []byte)` | `(*gltfDoc, []byte, error)` | Parse magic, both chunk headers; return parsed JSON + raw BIN slice (copied — not aliasing input) |
| `writeGLB(doc *gltfDoc, bin []byte)` | `([]byte, error)` | Marshal JSON, pad to 4 with `0x20`, pad BIN to 4 with `0x00`, build header, return concatenated bytes |
| `newMergeContext()` | `*mergeContext` | Initialize empty out doc with `asset.version="2.0"`, single scene, single buffer |
| `(mc) absorb(in *gltfDoc, inBin []byte)` | `(indexMap, error)` | Run the 7-step copy+remap pass |
| `(mc) appendBufferView(srcBV gltfBufferView, srcBin []byte)` | `int` | Copy bytes into outBin (4-aligned), append a new bufferView, return its index |
| `(mc) absorbImage(img gltfImage, srcBin []byte, bvMap []int)` | `(int, error)` | Apply SHA256 dedup and bufferView remap; return out index |
| `routeSideMeshes(mc, im, in)` | `(side, top *int)` | Build view_side group node and (if billboard_top exists) view_top group/leaf; return their node indices |
| `routeTiltedMeshes(mc, im, in)` | `*int` | Build view_tilted group node, return its index |
| `routeVolumetricMeshes(mc, im, in)` | `*int` | Build view_dome group node sorted by min-Y, return its index |
| `meshMinY(mesh gltfMesh, accessors []gltfAccessor)` | `float64` | Look up POSITION accessors and return min of `min[1]`s |
| `attachExtras(mc, meta)` | — | Set `out.Scenes[0].Extras["plantastic"] = meta.ToExtras()` |
| `remapMaterialIndices(raw json.RawMessage, texMap []int)` | `(json.RawMessage, error)` | Walk decoded interface, rewrite every `"index"` field inside a key matching `*[Tt]exture` |
| `pad4(n int)` | `int` | Round n up to next multiple of 4 |

## File Sketch — `combine.go`

```go
package main

import (
    "bytes"
    "crypto/sha256"
    "encoding/binary"
    "encoding/json"
    "fmt"
    "sort"
)

const (
    glbMagic     uint32 = 0x46546C67 // "glTF"
    glbVersion   uint32 = 2
    chunkTypeJSON uint32 = 0x4E4F534A // "JSON"
    chunkTypeBIN  uint32 = 0x004E4942 // "BIN\0"
    packSizeCap   = 5 * 1024 * 1024
)

func CombinePack(side, tilted, volumetric []byte, meta PackMeta) ([]byte, error) {
    if side == nil {
        return nil, fmt.Errorf("combine: side intermediate is required")
    }
    if err := meta.Validate(); err != nil {
        return nil, fmt.Errorf("combine: invalid meta: %w", err)
    }

    sideDoc, sideBin, err := readGLB(side)
    if err != nil { return nil, fmt.Errorf("combine: parse side: %w", err) }

    var tiltedDoc *gltfDoc; var tiltedBin []byte
    if tilted != nil {
        tiltedDoc, tiltedBin, err = readGLB(tilted)
        if err != nil { return nil, fmt.Errorf("combine: parse tilted: %w", err) }
    }
    var volDoc *gltfDoc; var volBin []byte
    if volumetric != nil {
        volDoc, volBin, err = readGLB(volumetric)
        if err != nil { return nil, fmt.Errorf("combine: parse volumetric: %w", err) }
    }

    mc := newMergeContext()

    sideMap, err := mc.absorb(sideDoc, sideBin)
    if err != nil { return nil, fmt.Errorf("combine: absorb side: %w", err) }
    sideGroup, topGroup := routeSideMeshes(mc, sideMap, sideDoc)

    var tiltedGroup *int
    if tiltedDoc != nil {
        m, err := mc.absorb(tiltedDoc, tiltedBin)
        if err != nil { return nil, fmt.Errorf("combine: absorb tilted: %w", err) }
        tiltedGroup = routeTiltedMeshes(mc, m, tiltedDoc)
    }
    var volGroup *int
    if volDoc != nil {
        m, err := mc.absorb(volDoc, volBin)
        if err != nil { return nil, fmt.Errorf("combine: absorb volumetric: %w", err) }
        volGroup = routeVolumetricMeshes(mc, m, volDoc)
    }

    // Build root node, parent the four groups, set as scene root.
    root := gltfNode{Name: "pack_root"}
    if sideGroup != nil { root.Children = append(root.Children, *sideGroup) }
    if topGroup != nil  { root.Children = append(root.Children, *topGroup) }
    if tiltedGroup != nil { root.Children = append(root.Children, *tiltedGroup) }
    if volGroup != nil  { root.Children = append(root.Children, *volGroup) }
    mc.out.Nodes = append(mc.out.Nodes, root)
    rootIdx := len(mc.out.Nodes) - 1
    mc.out.Scenes[0].Nodes = []int{rootIdx}

    attachExtras(mc, meta)

    // Finalize buffer length on the single buffer entry.
    mc.out.Buffers[0].ByteLength = mc.outBin.Len()

    raw, err := writeGLB(mc.out, mc.outBin.Bytes())
    if err != nil { return nil, fmt.Errorf("combine: write: %w", err) }
    if len(raw) > packSizeCap {
        return nil, fmt.Errorf("combine: pack size %d exceeds 5 MiB cap", len(raw))
    }
    return raw, nil
}

// ... readGLB / writeGLB / mergeContext / absorb / routers / helpers ...
```

## File Sketch — `combine_test.go`

```go
package main

import (
    "bytes"
    "encoding/json"
    "testing"
)

// makeMinimalGLB synthesizes a tiny but valid GLB containing one or more
// meshes whose names are taken from `meshNames`. Each mesh has a single
// triangle (3 indices) so an accessor min/max is well-defined. Returns
// the GLB bytes ready to feed into CombinePack.
func makeMinimalGLB(meshNames []string, perMeshMinY []float64) []byte { ... }

func TestCombine_SideOnly_RoutesBillboardTop(t *testing.T) { ... }
func TestCombine_SideOnly_NoBillboardTop(t *testing.T)     { ... }
func TestCombine_TiltedAdded_VariantNaming(t *testing.T)   { ... }
func TestCombine_VolumetricSliceOrder(t *testing.T)        { ... }
func TestCombine_RejectsNilSide(t *testing.T)              { ... }
func TestCombine_RejectsInvalidMeta(t *testing.T)          { ... }
func TestCombine_EmbedsExtras(t *testing.T)                { ... }
func TestCombine_RoundTripParseable(t *testing.T)          { ... }
func TestCombine_SizeCapRejection(t *testing.T)            { ... }   // forced via large bin payload
func TestCombine_ImageDedup(t *testing.T)                  { ... }   // identical PNG bytes in both inputs collapse
```

## Module Boundaries

- `combine.go` depends only on the standard library and on `pack_meta.go`
  (for the `PackMeta` type and `Validate` / `ToExtras` methods).
- No other file imports anything from `combine.go` in this ticket.
  T-010-03 (HTTP endpoint) is the first consumer.
- Tests live in the same package (`package main`) so they can call
  unexported helpers like `readGLB`, `writeGLB`, `meshMinY`.

## Ordering of Changes

1. Land `combine.go` types + `readGLB` + `writeGLB` + the empty
   `CombinePack` shell. Verify `go build` and `go vet` clean.
2. Add `combine_test.go` with `makeMinimalGLB` + the two simplest tests
   (`SideOnly_RoutesBillboardTop`, `RejectsNilSide`). Confirm they fail
   for the right reason against the empty shell.
3. Implement `mergeContext`, `absorb`, the four routers, helpers.
4. Run tests; fill in the remaining test cases as the implementation
   stabilizes.
5. Run full `go test ./...` to confirm no regressions in unrelated
   packages.
