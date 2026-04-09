package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// GLB container constants. Magic numbers are little-endian uint32 reads
// of the 4-byte ASCII identifiers from the glTF 2.0 binary spec.
const (
	glbMagic      uint32 = 0x46546C67 // "glTF"
	glbVersion    uint32 = 2
	chunkTypeJSON uint32 = 0x4E4F534A // "JSON"
	chunkTypeBIN  uint32 = 0x004E4942 // "BIN\0"
	packSizeCap          = 5 * 1024 * 1024
)

// gltfDoc mirrors the subset of the glTF 2.0 JSON schema that the pack
// combine pass needs to inspect or rewrite. Fields the combine does not
// touch (asset metadata, sampler bodies) pass through as raw JSON so we
// don't lose data we don't understand.
type gltfDoc struct {
	Asset       json.RawMessage   `json:"asset"`
	Scene       int               `json:"scene"`
	Scenes      []gltfScene       `json:"scenes"`
	Nodes       []gltfNode        `json:"nodes,omitempty"`
	Meshes      []gltfMesh        `json:"meshes,omitempty"`
	Accessors   []gltfAccessor    `json:"accessors,omitempty"`
	BufferViews []gltfBufferView  `json:"bufferViews,omitempty"`
	Buffers     []gltfBuffer      `json:"buffers"`
	Materials   []json.RawMessage `json:"materials,omitempty"`
	Textures    []gltfTexture     `json:"textures,omitempty"`
	Images      []gltfImage       `json:"images,omitempty"`
	Samplers    []json.RawMessage `json:"samplers,omitempty"`
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
	BufferView    *int      `json:"bufferView,omitempty"`
	ByteOffset    int       `json:"byteOffset,omitempty"`
	ComponentType int       `json:"componentType"`
	Normalized    bool      `json:"normalized,omitempty"`
	Count         int       `json:"count"`
	Type          string    `json:"type"`
	Min           []float64 `json:"min,omitempty"`
	Max           []float64 `json:"max,omitempty"`
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
	URI        string `json:"uri,omitempty"`
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

// indexMap holds per-source-slice → out-slice index translation tables
// produced by a single absorb call. Routers consult mesh[srcIdx] to find
// the new mesh index after a merge.
type indexMap struct {
	accessor   []int
	bufferView []int
	image      []int
	sampler    []int
	texture    []int
	material   []int
	mesh       []int
}

// mergeContext is the mutable accumulator that absorb passes write into.
// outBin grows as bufferViews and image bytes are appended; imageHashes
// dedupes images across all inputs by SHA256 of resolved bytes.
type mergeContext struct {
	out         *gltfDoc
	outBin      *bytes.Buffer
	imageHashes map[[32]byte]int
	imageBytes  int64
}

// PackBreakdown decomposes the assembled pack's size into the three
// budgets a baker can act on: texture payload, mesh / vertex payload,
// and the glTF JSON manifest. TextureCount is the number of physical
// image payloads after SHA256 dedup; TextureBytes/MeshBytes are exact
// byte counts of the BIN regions.
type PackBreakdown struct {
	TextureCount int
	TextureBytes int64
	MeshBytes    int64
	JSONBytes    int64
}

// PackOversizeError is returned by CombinePack when the assembled pack
// exceeds the 5 MiB hard cap. It carries enough breakdown for an
// operator to decide what to shrink (texture variants vs mesh density)
// without re-instrumenting the bake. Detect with errors.As; the
// formatted Error() string is meant to be surfaced verbatim by the
// HTTP layer in its 413 response.
type PackOversizeError struct {
	Species     string
	ActualBytes int64
	LimitBytes  int64
	Breakdown   PackBreakdown
}

// Error renders the multi-line layout documented in T-010-05's AC:
// a header with species + actual size, a 3-row breakdown, and a
// fixed hint line.
func (e *PackOversizeError) Error() string {
	var sb strings.Builder
	// The "5 MB" wording is fixed copy from the ticket AC. The
	// underlying constant is binary 5 MiB (5_242_880 bytes), but
	// users think in decimal MB and the spec'd message reflects
	// that. LimitBytes is still exact for any structured consumer.
	fmt.Fprintf(&sb, "pack %q exceeds 5 MB limit (actual: %s)\n",
		e.Species, humanBytes(e.ActualBytes))

	if e.Breakdown.TextureCount > 0 {
		avg := e.Breakdown.TextureBytes / int64(e.Breakdown.TextureCount)
		fmt.Fprintf(&sb, "  textures:    %d × avg %s = %s\n",
			e.Breakdown.TextureCount, humanBytes(avg),
			humanBytes(e.Breakdown.TextureBytes))
	} else {
		sb.WriteString("  textures:    none\n")
	}
	fmt.Fprintf(&sb, "  meshes:      %s\n", humanBytes(e.Breakdown.MeshBytes))
	fmt.Fprintf(&sb, "  metadata:    %s\n", humanBytes(e.Breakdown.JSONBytes))
	sb.WriteString("hint: reduce billboard texture resolution or variant count and re-bake")
	return sb.String()
}

// humanBytes renders n in decimal MB / KB / B with one fractional
// digit for MB and zero for smaller units. Decimal (1_000_000) not
// binary (1<<20) so the output reads "5 MB" — the wording the ticket
// AC and user-facing copy use, even though the cap constant itself is
// binary MiB.
func humanBytes(n int64) string {
	switch {
	case n >= 1_000_000:
		return fmt.Sprintf("%.1f MB", float64(n)/1_000_000)
	case n >= 1000:
		return fmt.Sprintf("%d KB", n/1000)
	default:
		return fmt.Sprintf("%d B", n)
	}
}

// pad4 returns n rounded up to the next multiple of 4. Used for both
// JSON chunk space-padding and BIN chunk zero-padding alignment.
func pad4(n int) int {
	r := n % 4
	if r == 0 {
		return n
	}
	return n + (4 - r)
}

// readGLB parses a GLB byte slice into a gltfDoc + a private copy of the
// BIN chunk payload. The returned BIN slice is independent of the input
// so callers may freely mutate it. Returns an error on any header,
// chunk, or JSON decode failure.
func readGLB(raw []byte) (*gltfDoc, []byte, error) {
	if len(raw) < 12 {
		return nil, nil, fmt.Errorf("file too small for GLB header")
	}
	if binary.LittleEndian.Uint32(raw[0:4]) != glbMagic {
		return nil, nil, fmt.Errorf("not a GLB file (bad magic)")
	}
	if binary.LittleEndian.Uint32(raw[4:8]) != glbVersion {
		return nil, nil, fmt.Errorf("unsupported GLB version")
	}
	totalLen := int(binary.LittleEndian.Uint32(raw[8:12]))
	if totalLen > len(raw) {
		return nil, nil, fmt.Errorf("GLB header length %d exceeds buffer %d", totalLen, len(raw))
	}

	off := 12
	if off+8 > len(raw) {
		return nil, nil, fmt.Errorf("missing JSON chunk header")
	}
	jsonLen := int(binary.LittleEndian.Uint32(raw[off : off+4]))
	jsonType := binary.LittleEndian.Uint32(raw[off+4 : off+8])
	if jsonType != chunkTypeJSON {
		return nil, nil, fmt.Errorf("expected JSON chunk, got 0x%08x", jsonType)
	}
	off += 8
	if off+jsonLen > len(raw) {
		return nil, nil, fmt.Errorf("JSON chunk truncated")
	}
	jsonBytes := raw[off : off+jsonLen]
	off += jsonLen

	var doc gltfDoc
	if err := json.Unmarshal(bytes.TrimRight(jsonBytes, " \x00"), &doc); err != nil {
		return nil, nil, fmt.Errorf("decode glTF JSON: %w", err)
	}

	var bin []byte
	if off+8 <= len(raw) {
		binChunkLen := int(binary.LittleEndian.Uint32(raw[off : off+4]))
		binType := binary.LittleEndian.Uint32(raw[off+4 : off+8])
		off += 8
		if binType == chunkTypeBIN {
			if off+binChunkLen > len(raw) {
				return nil, nil, fmt.Errorf("BIN chunk truncated")
			}
			bin = make([]byte, binChunkLen)
			copy(bin, raw[off:off+binChunkLen])
		}
	}
	return &doc, bin, nil
}

// writeGLB serializes doc + bin into a single GLB byte slice with the
// JSON chunk space-padded and the BIN chunk zero-padded to 4-byte
// alignment, headers populated with the padded lengths, and the file
// header recording the total size.
func writeGLB(doc *gltfDoc, bin []byte) ([]byte, error) {
	jsonRaw, err := json.Marshal(doc)
	if err != nil {
		return nil, fmt.Errorf("marshal glTF JSON: %w", err)
	}
	jsonPadded := pad4(len(jsonRaw))
	jsonChunk := make([]byte, jsonPadded)
	copy(jsonChunk, jsonRaw)
	for i := len(jsonRaw); i < jsonPadded; i++ {
		jsonChunk[i] = 0x20 // ASCII space
	}

	binPadded := pad4(len(bin))
	binChunk := make([]byte, binPadded)
	copy(binChunk, bin)
	// trailing bytes already zero from make()

	headerSize := 12
	chunkHdr := 8
	total := headerSize + chunkHdr + len(jsonChunk)
	if len(bin) > 0 || binPadded > 0 {
		total += chunkHdr + len(binChunk)
	}

	out := make([]byte, 0, total)
	out = binary.LittleEndian.AppendUint32(out, glbMagic)
	out = binary.LittleEndian.AppendUint32(out, glbVersion)
	out = binary.LittleEndian.AppendUint32(out, uint32(total))

	out = binary.LittleEndian.AppendUint32(out, uint32(len(jsonChunk)))
	out = binary.LittleEndian.AppendUint32(out, chunkTypeJSON)
	out = append(out, jsonChunk...)

	if len(bin) > 0 || binPadded > 0 {
		out = binary.LittleEndian.AppendUint32(out, uint32(len(binChunk)))
		out = binary.LittleEndian.AppendUint32(out, chunkTypeBIN)
		out = append(out, binChunk...)
	}
	return out, nil
}

// newMergeContext returns an empty merge accumulator with the minimum
// glTF skeleton in place: asset version, one scene with no nodes yet,
// one zero-length buffer entry. Buffer length is filled in later from
// the final outBin size.
func newMergeContext() *mergeContext {
	asset := json.RawMessage(`{"version":"2.0","generator":"glb-optimizer combine v1"}`)
	return &mergeContext{
		out: &gltfDoc{
			Asset:   asset,
			Scene:   0,
			Scenes:  []gltfScene{{Nodes: []int{}}},
			Buffers: []gltfBuffer{{ByteLength: 0}},
		},
		outBin:      &bytes.Buffer{},
		imageHashes: map[[32]byte]int{},
	}
}

// alignBin pads the running outBin to a 4-byte boundary so the next
// appended payload starts at an aligned offset (required for accessors
// whose componentType has > 1 byte stride).
func (mc *mergeContext) alignBin() {
	for mc.outBin.Len()%4 != 0 {
		mc.outBin.WriteByte(0)
	}
}

// appendBytes copies payload into the output BIN at a 4-aligned offset
// and returns a fresh bufferView entry pointing at it. The caller is
// responsible for appending the bufferView and recording its index.
func (mc *mergeContext) appendBytes(payload []byte) gltfBufferView {
	mc.alignBin()
	off := mc.outBin.Len()
	mc.outBin.Write(payload)
	return gltfBufferView{
		Buffer:     0,
		ByteOffset: off,
		ByteLength: len(payload),
	}
}

// absorb copies one input doc + its BIN into the merge accumulator,
// remapping every internal index as objects are appended. Returns the
// indexMap routers need to translate source mesh indices into output
// mesh indices.
func (mc *mergeContext) absorb(in *gltfDoc, inBin []byte) (indexMap, error) {
	im := indexMap{
		bufferView: make([]int, len(in.BufferViews)),
		accessor:   make([]int, len(in.Accessors)),
		image:      make([]int, len(in.Images)),
		sampler:    make([]int, len(in.Samplers)),
		texture:    make([]int, len(in.Textures)),
		material:   make([]int, len(in.Materials)),
		mesh:       make([]int, len(in.Meshes)),
	}

	// 1. bufferViews — copy the input BIN as one block, then re-base
	// every source bufferView onto buffer 0 at baseline + srcOffset.
	mc.alignBin()
	baseline := mc.outBin.Len()
	mc.outBin.Write(inBin)
	for i, bv := range in.BufferViews {
		nbv := bv
		nbv.Buffer = 0
		nbv.ByteOffset = bv.ByteOffset + baseline
		mc.out.BufferViews = append(mc.out.BufferViews, nbv)
		im.bufferView[i] = len(mc.out.BufferViews) - 1
	}

	// 2. accessors — remap their bufferView pointer.
	for i, a := range in.Accessors {
		na := a
		if a.BufferView != nil {
			v := im.bufferView[*a.BufferView]
			na.BufferView = &v
		}
		mc.out.Accessors = append(mc.out.Accessors, na)
		im.accessor[i] = len(mc.out.Accessors) - 1
	}

	// 3. images — SHA256 dedup over resolved bytes; remap or skip.
	for i, img := range in.Images {
		newIdx, err := mc.absorbImage(img, inBin, in.BufferViews)
		if err != nil {
			return im, fmt.Errorf("image %d: %w", i, err)
		}
		im.image[i] = newIdx
	}

	// 4. samplers — verbatim passthrough.
	for i, s := range in.Samplers {
		mc.out.Samplers = append(mc.out.Samplers, s)
		im.sampler[i] = len(mc.out.Samplers) - 1
	}

	// 5. textures — remap source/sampler indices.
	for i, t := range in.Textures {
		nt := t
		if t.Source != nil {
			v := im.image[*t.Source]
			nt.Source = &v
		}
		if t.Sampler != nil {
			v := im.sampler[*t.Sampler]
			nt.Sampler = &v
		}
		mc.out.Textures = append(mc.out.Textures, nt)
		im.texture[i] = len(mc.out.Textures) - 1
	}

	// 6. materials — walk decoded JSON, rewrite every "*Texture":
	// {"index": k, ...} so k points at the new merged texture index.
	for i, m := range in.Materials {
		nm, err := remapMaterialIndices(m, im.texture)
		if err != nil {
			return im, fmt.Errorf("material %d: %w", i, err)
		}
		mc.out.Materials = append(mc.out.Materials, nm)
		im.material[i] = len(mc.out.Materials) - 1
	}

	// 7. meshes — clone primitives, remap accessor + material indices.
	for i, mesh := range in.Meshes {
		nm := gltfMesh{
			Name:       mesh.Name,
			Primitives: make([]gltfPrimitive, len(mesh.Primitives)),
		}
		for j, p := range mesh.Primitives {
			np := gltfPrimitive{
				Attributes: make(map[string]int, len(p.Attributes)),
				Mode:       p.Mode,
			}
			for k, v := range p.Attributes {
				np.Attributes[k] = im.accessor[v]
			}
			if p.Indices != nil {
				v := im.accessor[*p.Indices]
				np.Indices = &v
			}
			if p.Material != nil {
				v := im.material[*p.Material]
				np.Material = &v
			}
			nm.Primitives[j] = np
		}
		mc.out.Meshes = append(mc.out.Meshes, nm)
		im.mesh[i] = len(mc.out.Meshes) - 1
	}

	return im, nil
}

// absorbImage applies SHA256-based content dedup over image payload
// bytes. If a hash collision exists, returns the existing image index
// without copying. Otherwise stores the image bytes in the output BIN,
// creates a fresh bufferView, and appends a new image entry.
func (mc *mergeContext) absorbImage(img gltfImage, srcBin []byte, srcBVs []gltfBufferView) (int, error) {
	var payload []byte
	switch {
	case img.BufferView != nil:
		idx := *img.BufferView
		if idx < 0 || idx >= len(srcBVs) {
			return 0, fmt.Errorf("bufferView %d out of range", idx)
		}
		bv := srcBVs[idx]
		end := bv.ByteOffset + bv.ByteLength
		if end > len(srcBin) {
			return 0, fmt.Errorf("bufferView %d exceeds BIN length", idx)
		}
		payload = srcBin[bv.ByteOffset:end]
	case img.URI != "":
		// Hash the URI string verbatim — sufficient to dedup identical
		// data URIs and external paths without base64-decoding them.
		payload = []byte(img.URI)
	default:
		return 0, fmt.Errorf("image has neither bufferView nor uri")
	}

	hash := sha256.Sum256(payload)
	if existing, ok := mc.imageHashes[hash]; ok {
		return existing, nil
	}

	newImg := gltfImage{
		MimeType: img.MimeType,
		Name:     img.Name,
	}
	if img.BufferView != nil {
		bv := mc.appendBytes(payload)
		mc.out.BufferViews = append(mc.out.BufferViews, bv)
		bvIdx := len(mc.out.BufferViews) - 1
		newImg.BufferView = &bvIdx
		// Track image bytes for PackOversizeError breakdowns. Only the
		// non-deduped, BIN-resident path increments — URI images and
		// SHA256 dedup hits stay at zero so MeshBytes accounting
		// (BIN.len() - imageBytes) remains exact.
		mc.imageBytes += int64(len(payload))
	} else {
		newImg.URI = img.URI
	}
	mc.out.Images = append(mc.out.Images, newImg)
	idx := len(mc.out.Images) - 1
	mc.imageHashes[hash] = idx
	return idx, nil
}

// textureKeyRe matches the JSON key names whose value is a glTF
// textureInfo object — i.e. an object containing an "index" int that
// references textures[]. Used by remapMaterialIndices to limit
// rewrites to actual texture references.
var textureKeyRe = regexp.MustCompile(`(?i)Texture$`)

// remapMaterialIndices walks the decoded material JSON tree and
// rewrites every "index": int found inside an object whose key name
// ends in "Texture" (case-insensitive). This catches baseColorTexture,
// metallicRoughnessTexture, normalTexture, occlusionTexture,
// emissiveTexture, and any KHR_*_texture extension that follows the
// same convention.
func remapMaterialIndices(raw json.RawMessage, texMap []int) (json.RawMessage, error) {
	if len(raw) == 0 {
		return raw, nil
	}
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return nil, fmt.Errorf("decode: %w", err)
	}
	walked := walkRemapTextures(v, texMap, false)
	out, err := json.Marshal(walked)
	if err != nil {
		return nil, fmt.Errorf("encode: %w", err)
	}
	return out, nil
}

// walkRemapTextures recursively descends a decoded JSON tree. The
// inTextureInfo flag is set when the parent map key matched the
// "Texture" suffix; in that scope an "index" field is rewritten via
// texMap.
func walkRemapTextures(v any, texMap []int, inTextureInfo bool) any {
	switch t := v.(type) {
	case map[string]any:
		for k, child := range t {
			childInTex := inTextureInfo || textureKeyRe.MatchString(k)
			t[k] = walkRemapTextures(child, texMap, childInTex)
		}
		if inTextureInfo {
			if idxVal, ok := t["index"]; ok {
				if f, ok := idxVal.(float64); ok {
					i := int(f)
					if i >= 0 && i < len(texMap) {
						t["index"] = texMap[i]
					}
				}
			}
		}
		return t
	case []any:
		for i, child := range t {
			t[i] = walkRemapTextures(child, texMap, inTextureInfo)
		}
		return t
	default:
		return v
	}
}

// addLeafNode appends a new leaf node carrying the given mesh index and
// returns its index in the output node slice. Used by all three routers
// to register absorbed meshes under their group.
func (mc *mergeContext) addLeafNode(name string, meshIdx int) int {
	m := meshIdx
	mc.out.Nodes = append(mc.out.Nodes, gltfNode{Name: name, Mesh: &m})
	return len(mc.out.Nodes) - 1
}

// addGroupNode appends a parent node with the given children and
// returns its index. Children must already exist in the node slice.
func (mc *mergeContext) addGroupNode(name string, children []int) int {
	mc.out.Nodes = append(mc.out.Nodes, gltfNode{Name: name, Children: children})
	return len(mc.out.Nodes) - 1
}

// routeSideMeshes splits the side intermediate's meshes into a
// view_side group (everything except billboard_top, renamed
// variant_N) and an optional view_top group (the single billboard_top
// mesh, if present). Returns pointers to the group node indices, with
// nil meaning "this group has no children, do not add it to root".
func routeSideMeshes(mc *mergeContext, im indexMap, in *gltfDoc) (*int, *int) {
	var sideChildren []int
	var topMeshIdx *int
	variant := 0
	for i, mesh := range in.Meshes {
		newMesh := im.mesh[i]
		if mesh.Name == "billboard_top" {
			v := newMesh
			topMeshIdx = &v
			continue
		}
		leaf := mc.addLeafNode(fmt.Sprintf("variant_%d", variant), newMesh)
		sideChildren = append(sideChildren, leaf)
		variant++
	}

	var sideGroup *int
	if len(sideChildren) > 0 {
		idx := mc.addGroupNode("view_side", sideChildren)
		sideGroup = &idx
	}
	var topGroup *int
	if topMeshIdx != nil {
		// view_top is a group with a single dual-named leaf so the
		// consumer can resolve scene.getObjectByName("view_top") to
		// either node interchangeably.
		leaf := mc.addLeafNode("view_top", *topMeshIdx)
		idx := mc.addGroupNode("view_top", []int{leaf})
		topGroup = &idx
	}
	return sideGroup, topGroup
}

// routeTiltedMeshes packs every mesh from the tilted intermediate
// under a view_tilted group, renaming children variant_0, variant_1, …
// in source order. Returns nil if the input has no meshes.
func routeTiltedMeshes(mc *mergeContext, im indexMap, in *gltfDoc) *int {
	if len(in.Meshes) == 0 {
		return nil
	}
	var children []int
	for i := range in.Meshes {
		leaf := mc.addLeafNode(fmt.Sprintf("variant_%d", i), im.mesh[i])
		children = append(children, leaf)
	}
	idx := mc.addGroupNode("view_tilted", children)
	return &idx
}

// routeVolumetricMeshes sorts the volumetric intermediate's meshes by
// their POSITION accessor min-Y (ascending, ties broken by source
// index) and parents them under view_dome with names slice_0…slice_N.
// The Y-sort guarantees per-instance offsets line up across machines.
func routeVolumetricMeshes(mc *mergeContext, im indexMap, in *gltfDoc) *int {
	if len(in.Meshes) == 0 {
		return nil
	}
	type indexedMesh struct {
		src  int
		minY float64
	}
	ordered := make([]indexedMesh, len(in.Meshes))
	for i, mesh := range in.Meshes {
		ordered[i] = indexedMesh{src: i, minY: meshMinY(mesh, in.Accessors)}
	}
	sort.SliceStable(ordered, func(i, j int) bool {
		return ordered[i].minY < ordered[j].minY
	})

	var children []int
	for slice, om := range ordered {
		leaf := mc.addLeafNode(fmt.Sprintf("slice_%d", slice), im.mesh[om.src])
		children = append(children, leaf)
	}
	idx := mc.addGroupNode("view_dome", children)
	return &idx
}

// meshMinY returns the lowest Y bbox value across the mesh's primitives'
// POSITION accessors. Reads from accessor.min[1] which the glTF spec
// requires on POSITION accessors. Falls back to +Inf for any primitive
// missing the data so such meshes sort to the top of view_dome and
// surface as obvious outliers if they ever appear.
func meshMinY(mesh gltfMesh, accessors []gltfAccessor) float64 {
	min := 1.0e308
	for _, p := range mesh.Primitives {
		ai, ok := p.Attributes["POSITION"]
		if !ok || ai < 0 || ai >= len(accessors) {
			continue
		}
		acc := accessors[ai]
		if len(acc.Min) >= 2 && acc.Min[1] < min {
			min = acc.Min[1]
		}
	}
	return min
}

// attachExtras stamps the validated meta as extras.plantastic on the
// output's root scene. The Pack v1 contract requires this to be on the
// scene (not the asset, not a node) so the consumer's loader can find
// it via gltf.scene.userData.plantastic after parsing.
func attachExtras(mc *mergeContext, meta PackMeta) {
	if mc.out.Scenes[0].Extras == nil {
		mc.out.Scenes[0].Extras = map[string]any{}
	}
	mc.out.Scenes[0].Extras["plantastic"] = meta.ToExtras()
}

// CombinePack merges up to three intermediate GLB byte slices into a
// single Pack v1 asset pack GLB. side is required; tilted and
// volumetric may be nil. The returned bytes are a self-contained .glb
// whose root scene's extras.plantastic block matches meta. Returns an
// error if meta is invalid, any input fails to parse, or the final
// size exceeds the Pack v1 5 MiB cap. Does not mutate the input slices.
func CombinePack(side []byte, tilted []byte, volumetric []byte, meta PackMeta) ([]byte, error) {
	if side == nil {
		return nil, fmt.Errorf("combine: side intermediate is required")
	}
	if err := meta.Validate(); err != nil {
		return nil, fmt.Errorf("combine: invalid meta: %w", err)
	}

	sideDoc, sideBin, err := readGLB(side)
	if err != nil {
		return nil, fmt.Errorf("combine: parse side: %w", err)
	}

	var tiltedDoc *gltfDoc
	var tiltedBin []byte
	if tilted != nil {
		tiltedDoc, tiltedBin, err = readGLB(tilted)
		if err != nil {
			return nil, fmt.Errorf("combine: parse tilted: %w", err)
		}
	}

	var volDoc *gltfDoc
	var volBin []byte
	if volumetric != nil {
		volDoc, volBin, err = readGLB(volumetric)
		if err != nil {
			return nil, fmt.Errorf("combine: parse volumetric: %w", err)
		}
	}

	mc := newMergeContext()

	sideMap, err := mc.absorb(sideDoc, sideBin)
	if err != nil {
		return nil, fmt.Errorf("combine: absorb side: %w", err)
	}
	sideGroup, topGroup := routeSideMeshes(mc, sideMap, sideDoc)

	var tiltedGroup *int
	if tiltedDoc != nil {
		tm, err := mc.absorb(tiltedDoc, tiltedBin)
		if err != nil {
			return nil, fmt.Errorf("combine: absorb tilted: %w", err)
		}
		tiltedGroup = routeTiltedMeshes(mc, tm, tiltedDoc)
	}

	var volGroup *int
	if volDoc != nil {
		vm, err := mc.absorb(volDoc, volBin)
		if err != nil {
			return nil, fmt.Errorf("combine: absorb volumetric: %w", err)
		}
		volGroup = routeVolumetricMeshes(mc, vm, volDoc)
	}

	root := gltfNode{Name: "pack_root"}
	if sideGroup != nil {
		root.Children = append(root.Children, *sideGroup)
	}
	if topGroup != nil {
		root.Children = append(root.Children, *topGroup)
	}
	if tiltedGroup != nil {
		root.Children = append(root.Children, *tiltedGroup)
	}
	if volGroup != nil {
		root.Children = append(root.Children, *volGroup)
	}
	mc.out.Nodes = append(mc.out.Nodes, root)
	rootIdx := len(mc.out.Nodes) - 1
	mc.out.Scenes[0].Nodes = []int{rootIdx}

	attachExtras(mc, meta)

	mc.alignBin()
	mc.out.Buffers[0].ByteLength = mc.outBin.Len()

	raw, err := writeGLB(mc.out, mc.outBin.Bytes())
	if err != nil {
		return nil, fmt.Errorf("combine: write: %w", err)
	}
	if len(raw) > packSizeCap {
		// Re-marshal the JSON for length only — wasted on the failure
		// path but the alternative (plumbing length out of writeGLB)
		// bloats the signature for one caller.
		jsonRaw, _ := json.Marshal(mc.out)
		return nil, &PackOversizeError{
			Species:     meta.Species,
			ActualBytes: int64(len(raw)),
			LimitBytes:  packSizeCap,
			Breakdown: PackBreakdown{
				TextureCount: len(mc.out.Images),
				TextureBytes: mc.imageBytes,
				MeshBytes:    int64(mc.outBin.Len()) - mc.imageBytes,
				JSONBytes:    int64(len(jsonRaw)),
			},
		}
	}
	return raw, nil
}
