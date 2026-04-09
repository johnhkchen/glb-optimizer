package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"testing"
)

// validCombineMeta returns a Pack v1 metadata fixture for tests that
// don't care about meta-specific behavior.
func validCombineMeta() PackMeta {
	return PackMeta{
		FormatVersion: 1,
		BakeID:        "2026-04-08T12:00:00Z",
		Species:       "achillea_millefolium",
		CommonName:    "Common Yarrow",
		Footprint:     Footprint{CanopyRadiusM: 0.45, HeightM: 0.62},
		Fade:          FadeBand{LowStart: 0.3, LowEnd: 0.55, HighStart: 0.75},
	}
}

// makeMinimalGLB synthesizes a tiny but valid GLB containing one mesh
// per entry in meshNames. Each mesh has a single triangle with
// fabricated POSITION min/max so volumetric slice ordering tests have
// real Y values to sort on. perMeshMinY[i] supplies the min-Y value for
// mesh i; pass nil to default every mesh's POSITION min[1] to 0.
func makeMinimalGLB(t *testing.T, meshNames []string, perMeshMinY []float64) []byte {
	t.Helper()

	// Layout per mesh: 6 bytes of indices (3 × uint16) + 36 bytes of
	// positions (3 × vec3 float32). Indices first to keep alignment
	// natural — we 4-align the positions block ourselves.
	doc := &gltfDoc{
		Asset:   json.RawMessage(`{"version":"2.0"}`),
		Scene:   0,
		Scenes:  []gltfScene{{Nodes: []int{0}}},
		Buffers: []gltfBuffer{{}},
	}
	bin := &bytes.Buffer{}

	for i, name := range meshNames {
		minY := 0.0
		if perMeshMinY != nil && i < len(perMeshMinY) {
			minY = perMeshMinY[i]
		}

		// indices: 0, 1, 2 (uint16)
		idxOffset := bin.Len()
		bin.Write([]byte{0, 0, 1, 0, 2, 0})
		// pad to 4
		for bin.Len()%4 != 0 {
			bin.WriteByte(0)
		}
		// positions: 3 vec3 float32 — write three vertices with the
		// chosen minY so accessor.min[1] is meaningful.
		posOffset := bin.Len()
		writeF32(bin, 0)
		writeF32(bin, float32(minY))
		writeF32(bin, 0)
		writeF32(bin, 1)
		writeF32(bin, float32(minY+1))
		writeF32(bin, 0)
		writeF32(bin, 0)
		writeF32(bin, float32(minY+1))
		writeF32(bin, 1)

		idxBV := len(doc.BufferViews)
		doc.BufferViews = append(doc.BufferViews, gltfBufferView{
			Buffer: 0, ByteOffset: idxOffset, ByteLength: 6,
		})
		posBV := len(doc.BufferViews)
		doc.BufferViews = append(doc.BufferViews, gltfBufferView{
			Buffer: 0, ByteOffset: posOffset, ByteLength: 36,
		})

		idxAcc := len(doc.Accessors)
		doc.Accessors = append(doc.Accessors, gltfAccessor{
			BufferView:    intPtr(idxBV),
			ComponentType: 5123, // UNSIGNED_SHORT
			Count:         3,
			Type:          "SCALAR",
		})
		posAcc := len(doc.Accessors)
		doc.Accessors = append(doc.Accessors, gltfAccessor{
			BufferView:    intPtr(posBV),
			ComponentType: 5126, // FLOAT
			Count:         3,
			Type:          "VEC3",
			Min:           []float64{0, minY, 0},
			Max:           []float64{1, minY + 1, 1},
		})

		doc.Meshes = append(doc.Meshes, gltfMesh{
			Name: name,
			Primitives: []gltfPrimitive{{
				Attributes: map[string]int{"POSITION": posAcc},
				Indices:    intPtr(idxAcc),
			}},
		})
		doc.Nodes = append(doc.Nodes, gltfNode{Name: name, Mesh: intPtr(i)})
	}

	doc.Scenes[0].Nodes = make([]int, len(meshNames))
	for i := range meshNames {
		doc.Scenes[0].Nodes[i] = i
	}
	doc.Buffers[0].ByteLength = bin.Len()

	raw, err := writeGLB(doc, bin.Bytes())
	if err != nil {
		t.Fatalf("makeMinimalGLB writeGLB: %v", err)
	}
	return raw
}

func intPtr(v int) *int { return &v }

func writeF32(buf *bytes.Buffer, f float32) {
	bits := math.Float32bits(f)
	buf.WriteByte(byte(bits))
	buf.WriteByte(byte(bits >> 8))
	buf.WriteByte(byte(bits >> 16))
	buf.WriteByte(byte(bits >> 24))
}

// --- I/O round-trip ---

func TestReadWriteRoundTrip(t *testing.T) {
	raw := makeMinimalGLB(t, []string{"a", "b"}, nil)
	doc, bin, err := readGLB(raw)
	if err != nil {
		t.Fatalf("readGLB: %v", err)
	}
	if len(doc.Meshes) != 2 {
		t.Fatalf("want 2 meshes, got %d", len(doc.Meshes))
	}
	if doc.Meshes[0].Name != "a" || doc.Meshes[1].Name != "b" {
		t.Fatalf("mesh names: %+v", []string{doc.Meshes[0].Name, doc.Meshes[1].Name})
	}
	if len(bin) == 0 {
		t.Fatalf("bin should not be empty")
	}
}

// --- Argument validation ---

func TestCombine_RejectsNilSide(t *testing.T) {
	_, err := CombinePack(nil, nil, nil, validCombineMeta())
	if err == nil || !strings.Contains(err.Error(), "side intermediate is required") {
		t.Fatalf("want side-required error, got %v", err)
	}
}

func TestCombine_RejectsInvalidMeta(t *testing.T) {
	side := makeMinimalGLB(t, []string{"x"}, nil)
	bad := validCombineMeta()
	bad.Species = "Bad-Caps"
	_, err := CombinePack(side, nil, nil, bad)
	if err == nil || !strings.Contains(err.Error(), "invalid meta") {
		t.Fatalf("want invalid meta error, got %v", err)
	}
}

// --- Mesh routing ---

func TestCombine_SideOnly_RoutesBillboardTop(t *testing.T) {
	side := makeMinimalGLB(t, []string{"billboard_top", "s0", "s1"}, nil)
	out, err := CombinePack(side, nil, nil, validCombineMeta())
	if err != nil {
		t.Fatalf("CombinePack: %v", err)
	}
	doc, _, err := readGLB(out)
	if err != nil {
		t.Fatalf("readGLB: %v", err)
	}
	names := nodeNamesByName(doc)
	if _, ok := names["view_side"]; !ok {
		t.Fatalf("view_side missing; nodes=%v", nodeNameList(doc))
	}
	if _, ok := names["view_top"]; !ok {
		t.Fatalf("view_top missing; nodes=%v", nodeNameList(doc))
	}
	// view_side group should have variant_0 and variant_1 children only
	sideGroup := names["view_side"]
	if len(sideGroup.Children) != 2 {
		t.Fatalf("view_side children: want 2, got %d", len(sideGroup.Children))
	}
	for i, c := range sideGroup.Children {
		want := fmtVariant(i)
		if doc.Nodes[c].Name != want {
			t.Fatalf("view_side child %d: want %q got %q", i, want, doc.Nodes[c].Name)
		}
	}
}

func TestCombine_SideOnly_NoBillboardTop(t *testing.T) {
	side := makeMinimalGLB(t, []string{"s0", "s1"}, nil)
	out, err := CombinePack(side, nil, nil, validCombineMeta())
	if err != nil {
		t.Fatalf("CombinePack: %v", err)
	}
	doc, _, _ := readGLB(out)
	names := nodeNamesByName(doc)
	if _, ok := names["view_top"]; ok {
		t.Fatalf("view_top should not exist when billboard_top absent")
	}
	if _, ok := names["view_side"]; !ok {
		t.Fatalf("view_side missing")
	}
}

func TestCombine_TiltedAdded_VariantNaming(t *testing.T) {
	side := makeMinimalGLB(t, []string{"billboard_top", "a"}, nil)
	tilted := makeMinimalGLB(t, []string{"t0", "t1", "t2"}, nil)
	out, err := CombinePack(side, tilted, nil, validCombineMeta())
	if err != nil {
		t.Fatalf("CombinePack: %v", err)
	}
	doc, _, _ := readGLB(out)
	names := nodeNamesByName(doc)
	tg, ok := names["view_tilted"]
	if !ok {
		t.Fatalf("view_tilted missing")
	}
	if len(tg.Children) != 3 {
		t.Fatalf("view_tilted children: want 3, got %d", len(tg.Children))
	}
	for i, c := range tg.Children {
		want := fmtVariant(i)
		if doc.Nodes[c].Name != want {
			t.Fatalf("view_tilted child %d: want %q got %q", i, want, doc.Nodes[c].Name)
		}
	}
}

func TestCombine_VolumetricSliceOrder(t *testing.T) {
	// Y values: indices [0,1,2,3] should sort to [1, 3, 0, 2]
	side := makeMinimalGLB(t, []string{"s"}, nil)
	vol := makeMinimalGLB(t, []string{"m0", "m1", "m2", "m3"}, []float64{0.5, 0.0, 0.75, 0.25})
	out, err := CombinePack(side, nil, vol, validCombineMeta())
	if err != nil {
		t.Fatalf("CombinePack: %v", err)
	}
	doc, _, _ := readGLB(out)
	names := nodeNamesByName(doc)
	dome, ok := names["view_dome"]
	if !ok {
		t.Fatalf("view_dome missing")
	}
	if len(dome.Children) != 4 {
		t.Fatalf("dome children: want 4, got %d", len(dome.Children))
	}
	// Each child node carries a mesh index. The mesh's POSITION
	// accessor min[1] tells us which source slice it was. Sort order
	// should be ascending min-Y.
	wantMinY := []float64{0.0, 0.25, 0.5, 0.75}
	for i, c := range dome.Children {
		node := doc.Nodes[c]
		if node.Name != fmtSlice(i) {
			t.Fatalf("dome child %d name: want slice_%d got %q", i, i, node.Name)
		}
		mesh := doc.Meshes[*node.Mesh]
		acc := doc.Accessors[mesh.Primitives[0].Attributes["POSITION"]]
		if acc.Min[1] != wantMinY[i] {
			t.Fatalf("dome child %d minY: want %g got %g", i, wantMinY[i], acc.Min[1])
		}
	}
}

// --- Extras + parseability ---

func TestCombine_EmbedsExtras(t *testing.T) {
	side := makeMinimalGLB(t, []string{"x"}, nil)
	out, err := CombinePack(side, nil, nil, validCombineMeta())
	if err != nil {
		t.Fatalf("CombinePack: %v", err)
	}
	doc, _, _ := readGLB(out)
	extras := doc.Scenes[0].Extras
	if extras == nil {
		t.Fatalf("scene extras missing")
	}
	plant, ok := extras["plantastic"].(map[string]any)
	if !ok {
		t.Fatalf("extras.plantastic missing or wrong type: %T", extras["plantastic"])
	}
	if plant["species"] != "achillea_millefolium" {
		t.Fatalf("species: got %v", plant["species"])
	}
	if plant["format_version"] != float64(1) {
		t.Fatalf("format_version: got %v", plant["format_version"])
	}
}

func TestCombine_RoundTripParseable(t *testing.T) {
	side := makeMinimalGLB(t, []string{"billboard_top", "s0"}, nil)
	tilted := makeMinimalGLB(t, []string{"t0"}, nil)
	vol := makeMinimalGLB(t, []string{"v0", "v1"}, []float64{1.0, 0.0})
	out, err := CombinePack(side, tilted, vol, validCombineMeta())
	if err != nil {
		t.Fatalf("CombinePack: %v", err)
	}
	doc, bin, err := readGLB(out)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if doc.Buffers[0].ByteLength != len(bin) {
		t.Fatalf("buffer length mismatch: header %d vs bin %d",
			doc.Buffers[0].ByteLength, len(bin))
	}
	for _, want := range []string{"view_side", "view_top", "view_tilted", "view_dome", "pack_root"} {
		if _, ok := nodeNamesByName(doc)[want]; !ok {
			t.Errorf("missing node %q after round trip", want)
		}
	}
}

// --- Image dedup ---

func TestCombine_ImageDedup(t *testing.T) {
	// Both inputs hand-crafted to share a single PNG-shaped payload.
	payload := []byte("PNGLIKE-DEDUPE-TEST-PAYLOAD")

	makeWithImage := func(meshName string) []byte {
		doc := &gltfDoc{
			Asset:   json.RawMessage(`{"version":"2.0"}`),
			Scene:   0,
			Scenes:  []gltfScene{{Nodes: []int{0}}},
			Buffers: []gltfBuffer{{}},
		}
		bin := &bytes.Buffer{}

		// indices + positions for one triangle
		idxOff := bin.Len()
		bin.Write([]byte{0, 0, 1, 0, 2, 0})
		for bin.Len()%4 != 0 {
			bin.WriteByte(0)
		}
		posOff := bin.Len()
		for k := 0; k < 9; k++ {
			writeF32(bin, float32(k))
		}
		// image bytes
		imgOff := bin.Len()
		bin.Write(payload)

		doc.BufferViews = []gltfBufferView{
			{Buffer: 0, ByteOffset: idxOff, ByteLength: 6},
			{Buffer: 0, ByteOffset: posOff, ByteLength: 36},
			{Buffer: 0, ByteOffset: imgOff, ByteLength: len(payload)},
		}
		doc.Accessors = []gltfAccessor{
			{BufferView: intPtr(0), ComponentType: 5123, Count: 3, Type: "SCALAR"},
			{BufferView: intPtr(1), ComponentType: 5126, Count: 3, Type: "VEC3",
				Min: []float64{0, 0, 0}, Max: []float64{1, 1, 1}},
		}
		doc.Meshes = []gltfMesh{{
			Name: meshName,
			Primitives: []gltfPrimitive{{
				Attributes: map[string]int{"POSITION": 1},
				Indices:    intPtr(0),
			}},
		}}
		doc.Images = []gltfImage{{BufferView: intPtr(2), MimeType: "image/png"}}
		doc.Nodes = []gltfNode{{Name: meshName, Mesh: intPtr(0)}}
		doc.Buffers[0].ByteLength = bin.Len()

		raw, err := writeGLB(doc, bin.Bytes())
		if err != nil {
			t.Fatalf("writeGLB: %v", err)
		}
		return raw
	}

	side := makeWithImage("s0")
	tilted := makeWithImage("t0")
	out, err := CombinePack(side, tilted, nil, validCombineMeta())
	if err != nil {
		t.Fatalf("CombinePack: %v", err)
	}
	doc, _, _ := readGLB(out)
	if len(doc.Images) != 1 {
		t.Fatalf("image dedup failed: got %d images, want 1", len(doc.Images))
	}
}

// --- Size cap ---

func TestCombine_SizeCapRejection(t *testing.T) {
	// Build an oversized side intermediate by stuffing a 6 MiB
	// bufferView in alongside a real triangle.
	doc := &gltfDoc{
		Asset:   json.RawMessage(`{"version":"2.0"}`),
		Scene:   0,
		Scenes:  []gltfScene{{Nodes: []int{0}}},
		Buffers: []gltfBuffer{{}},
	}
	bin := &bytes.Buffer{}

	// triangle indices + positions
	idxOff := bin.Len()
	bin.Write([]byte{0, 0, 1, 0, 2, 0})
	for bin.Len()%4 != 0 {
		bin.WriteByte(0)
	}
	posOff := bin.Len()
	for k := 0; k < 9; k++ {
		writeF32(bin, float32(k))
	}
	// 6 MiB ballast bytes addressed by a real bufferView (so the
	// merge actually copies them rather than dropping orphans)
	ballastOff := bin.Len()
	const ballastLen = 6 * 1024 * 1024
	bin.Write(make([]byte, ballastLen))

	doc.BufferViews = []gltfBufferView{
		{Buffer: 0, ByteOffset: idxOff, ByteLength: 6},
		{Buffer: 0, ByteOffset: posOff, ByteLength: 36},
		{Buffer: 0, ByteOffset: ballastOff, ByteLength: ballastLen},
	}
	doc.Accessors = []gltfAccessor{
		{BufferView: intPtr(0), ComponentType: 5123, Count: 3, Type: "SCALAR"},
		{BufferView: intPtr(1), ComponentType: 5126, Count: 3, Type: "VEC3",
			Min: []float64{0, 0, 0}, Max: []float64{1, 1, 1}},
	}
	doc.Meshes = []gltfMesh{{
		Name: "s",
		Primitives: []gltfPrimitive{{
			Attributes: map[string]int{"POSITION": 1},
			Indices:    intPtr(0),
		}},
	}}
	doc.Nodes = []gltfNode{{Name: "s", Mesh: intPtr(0)}}
	doc.Buffers[0].ByteLength = bin.Len()

	raw, err := writeGLB(doc, bin.Bytes())
	if err != nil {
		t.Fatalf("writeGLB: %v", err)
	}

	_, err = CombinePack(raw, nil, nil, validCombineMeta())
	if err == nil {
		t.Fatalf("want size-cap error, got nil")
	}
	var poe *PackOversizeError
	if !errors.As(err, &poe) {
		t.Fatalf("want *PackOversizeError, got %T: %v", err, err)
	}
	if poe.Species != "achillea_millefolium" {
		t.Errorf("species: got %q want achillea_millefolium", poe.Species)
	}
	if poe.LimitBytes != 5*1024*1024 {
		t.Errorf("limit bytes: got %d want %d", poe.LimitBytes, 5*1024*1024)
	}
	if poe.ActualBytes <= poe.LimitBytes {
		t.Errorf("actual bytes %d should exceed limit %d", poe.ActualBytes, poe.LimitBytes)
	}
	if poe.Breakdown.TextureCount != 0 {
		t.Errorf("texture count: got %d want 0 (fixture has no images)", poe.Breakdown.TextureCount)
	}
	if poe.Breakdown.TextureBytes != 0 {
		t.Errorf("texture bytes: got %d want 0", poe.Breakdown.TextureBytes)
	}
	if poe.Breakdown.MeshBytes < ballastLen {
		t.Errorf("mesh bytes: got %d want >= %d (ballast)", poe.Breakdown.MeshBytes, ballastLen)
	}
	if poe.Breakdown.JSONBytes <= 0 {
		t.Errorf("json bytes: got %d want > 0", poe.Breakdown.JSONBytes)
	}
	msg := poe.Error()
	for _, want := range []string{
		`pack "achillea_millefolium"`,
		"exceeds 5 MB limit",
		"meshes:",
		"metadata:",
		"hint: reduce billboard texture resolution",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("Error() missing %q\nfull message:\n%s", want, msg)
		}
	}
	t.Logf("PackOversizeError.Error():\n%s", msg)
}

// --- Input immutability ---

func TestCombine_DoesNotMutateInputs(t *testing.T) {
	side := makeMinimalGLB(t, []string{"billboard_top", "s0"}, nil)
	tilted := makeMinimalGLB(t, []string{"t0"}, nil)
	sideSnap := bytes.Clone(side)
	tiltedSnap := bytes.Clone(tilted)

	if _, err := CombinePack(side, tilted, nil, validCombineMeta()); err != nil {
		t.Fatalf("CombinePack: %v", err)
	}
	if !bytes.Equal(side, sideSnap) {
		t.Fatalf("side input was mutated")
	}
	if !bytes.Equal(tilted, tiltedSnap) {
		t.Fatalf("tilted input was mutated")
	}
}

// --- helpers ---

func nodeNamesByName(doc *gltfDoc) map[string]gltfNode {
	out := map[string]gltfNode{}
	for _, n := range doc.Nodes {
		if n.Name != "" {
			out[n.Name] = n
		}
	}
	return out
}

func nodeNameList(doc *gltfDoc) []string {
	names := make([]string, len(doc.Nodes))
	for i, n := range doc.Nodes {
		names[i] = n.Name
	}
	return names
}

func fmtVariant(i int) string {
	return "variant_" + itoa(i)
}

func fmtSlice(i int) string {
	return "slice_" + itoa(i)
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	digits := []byte{}
	for i > 0 {
		digits = append([]byte{byte('0' + i%10)}, digits...)
		i /= 10
	}
	return string(digits)
}
