package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// fixturePackMeta is a deterministic, valid PackMeta used by inspect
// tests so the snapshot test produces stable bytes.
func fixturePackMeta() PackMeta {
	return PackMeta{
		FormatVersion: PackFormatVersion,
		BakeID:        "2026-04-08T00:00:00Z",
		Species:       "salvia_officinalis",
		CommonName:    "Garden Sage",
		Footprint:     Footprint{CanopyRadiusM: 0.45, HeightM: 0.62},
		Fade:          FadeBand{LowStart: 0.30, LowEnd: 0.55, HighStart: 0.75},
	}
}

// buildFixturePack returns the on-disk path of a freshly written
// deterministic pack with the given optional flavours. side is
// always present.
func buildFixturePack(t *testing.T, withTilted, withVol bool) string {
	t.Helper()
	side := makeMinimalGLB(t, []string{"billboard_top", "s0"}, nil)
	var tilted []byte
	if withTilted {
		tilted = makeMinimalGLB(t, []string{"t0", "t1"}, nil)
	}
	var vol []byte
	if withVol {
		vol = makeMinimalGLB(t, []string{"v0", "v1", "v2"}, []float64{0.0, 0.5, 1.0})
	}
	raw, err := CombinePack(side, tilted, vol, fixturePackMeta())
	if err != nil {
		t.Fatalf("CombinePack: %v", err)
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "salvia_officinalis.glb")
	if err := os.WriteFile(path, raw, 0644); err != nil {
		t.Fatalf("write pack: %v", err)
	}
	return path
}

func TestInspectPack_HappyPath(t *testing.T) {
	path := buildFixturePack(t, true, true)
	rep, err := InspectPack(path)
	if err != nil {
		t.Fatalf("InspectPack: %v", err)
	}
	if !rep.Valid {
		t.Errorf("expected Valid=true, got Validation=%q", rep.Validation)
	}
	if rep.SHA256 == "" || len(rep.SHA256) != 64 {
		t.Errorf("expected 64-char sha256, got %q", rep.SHA256)
	}
	if rep.SHA256 != strings.ToLower(rep.SHA256) {
		t.Errorf("sha256 must be lowercase hex, got %q", rep.SHA256)
	}
	if rep.Meta == nil {
		t.Fatal("Meta is nil")
	}
	if rep.Meta.Species != "salvia_officinalis" {
		t.Errorf("species: got %q", rep.Meta.Species)
	}
	if rep.BakeID != "2026-04-08T00:00:00Z" {
		t.Errorf("bake_id: got %q", rep.BakeID)
	}
	if rep.Variants.Side == nil || rep.Variants.Side.Count == 0 {
		t.Error("expected non-empty Side variants")
	}
	if rep.Variants.Top == nil {
		t.Error("expected Top variant (billboard_top)")
	}
	if rep.Variants.Tilted == nil || rep.Variants.Tilted.Count != 2 {
		t.Errorf("expected 2 tilted, got %+v", rep.Variants.Tilted)
	}
	if rep.Variants.Dome == nil || rep.Variants.Dome.Count != 3 {
		t.Errorf("expected 3 dome slices, got %+v", rep.Variants.Dome)
	}
}

func TestInspectPack_AbsentOptionalVariants(t *testing.T) {
	path := buildFixturePack(t, false, false)
	rep, err := InspectPack(path)
	if err != nil {
		t.Fatalf("InspectPack: %v", err)
	}
	if !rep.Valid {
		t.Errorf("expected Valid=true, got %q", rep.Validation)
	}
	if rep.Variants.Side == nil {
		t.Error("Side must be present")
	}
	if rep.Variants.Tilted != nil {
		t.Errorf("Tilted should be nil, got %+v", rep.Variants.Tilted)
	}
	if rep.Variants.Dome != nil {
		t.Errorf("Dome should be nil, got %+v", rep.Variants.Dome)
	}
}

func TestInspectPack_TruncatedFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "junk.glb")
	if err := os.WriteFile(path, []byte("xx"), 0644); err != nil {
		t.Fatal(err)
	}
	_, err := InspectPack(path)
	if err == nil {
		t.Fatal("expected error on truncated file")
	}
	if !strings.Contains(err.Error(), "parse glb") {
		t.Errorf("expected parse error, got %v", err)
	}
}

func TestInspectPack_MissingMetadataBlock(t *testing.T) {
	// makeMinimalGLB produces a valid GLB with no plantastic extras.
	raw := makeMinimalGLB(t, []string{"a"}, nil)
	dir := t.TempDir()
	path := filepath.Join(dir, "no_meta.glb")
	if err := os.WriteFile(path, raw, 0644); err != nil {
		t.Fatal(err)
	}
	rep, err := InspectPack(path)
	if err != nil {
		t.Fatalf("InspectPack should not error on missing extras: %v", err)
	}
	if rep.Valid {
		t.Error("expected Valid=false on missing extras")
	}
	if !strings.Contains(rep.Validation, "extras") && !strings.Contains(rep.Validation, "Pack v1") {
		t.Errorf("expected extras error in Validation, got %q", rep.Validation)
	}
}

func TestRunPackInspectCmd_HappyPathStdout(t *testing.T) {
	workDir := setupCLIWorkdir(t)
	registerAsset(t, workDir, "achillea_millefolium", true)
	if rc := runPackCmd([]string{"--dir", workDir, "achillea_millefolium"}); rc != 0 {
		t.Fatalf("setup pack: rc=%d", rc)
	}

	var buf bytes.Buffer
	rc := runPackInspectCmdW([]string{"--dir", workDir, "achillea_millefolium"}, &buf)
	if rc != 0 {
		t.Fatalf("inspect rc=%d, output=%s", rc, buf.String())
	}
	out := buf.String()
	for _, want := range []string{
		"pack: achillea_millefolium.glb",
		"sha256:",
		"format:      Pack v1",
		"validation: OK",
		"view_side:",
		"metadata",
		"species:",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
}

func TestRunPackInspectCmd_JSONFlag(t *testing.T) {
	workDir := setupCLIWorkdir(t)
	registerAsset(t, workDir, "lavandula_angustifolia", true)
	if rc := runPackCmd([]string{"--dir", workDir, "lavandula_angustifolia"}); rc != 0 {
		t.Fatalf("setup pack: rc=%d", rc)
	}

	var buf bytes.Buffer
	rc := runPackInspectCmdW([]string{"--dir", workDir, "--json", "lavandula_angustifolia"}, &buf)
	if rc != 0 {
		t.Fatalf("inspect rc=%d", rc)
	}
	var rep PackInspectReport
	if err := json.Unmarshal(buf.Bytes(), &rep); err != nil {
		t.Fatalf("decode JSON: %v\n%s", err, buf.String())
	}
	if !rep.Valid {
		t.Errorf("expected Valid=true, got %q", rep.Validation)
	}
	if rep.Meta == nil || rep.Meta.Species != "lavandula_angustifolia" {
		t.Errorf("species mismatch: %+v", rep.Meta)
	}
	if rep.SHA256 == "" {
		t.Error("missing sha256")
	}
}

func TestRunPackInspectCmd_QuietFlag(t *testing.T) {
	workDir := setupCLIWorkdir(t)
	registerAsset(t, workDir, "rosa_julia", true)
	if rc := runPackCmd([]string{"--dir", workDir, "rosa_julia"}); rc != 0 {
		t.Fatalf("setup pack: rc=%d", rc)
	}

	var buf bytes.Buffer
	rc := runPackInspectCmdW([]string{"--dir", workDir, "--quiet", "rosa_julia"}, &buf)
	if rc != 0 {
		t.Fatalf("inspect rc=%d", rc)
	}
	out := strings.TrimSuffix(buf.String(), "\n")
	if strings.Contains(out, "\n") {
		t.Errorf("expected single line, got:\n%s", out)
	}
	fields := strings.Fields(out)
	if len(fields) != 3 {
		t.Fatalf("expected 3 fields, got %d: %v", len(fields), fields)
	}
	if len(fields[0]) != 64 {
		t.Errorf("field 0 should be 64-char sha256, got %q", fields[0])
	}
	if fields[2] != "OK" {
		t.Errorf("field 2 should be OK, got %q", fields[2])
	}
}

func TestRunPackInspectCmd_NonExistentSpecies(t *testing.T) {
	workDir := setupCLIWorkdir(t)
	rc := runPackInspectCmdW([]string{"--dir", workDir, "ghost_plant"}, &bytes.Buffer{})
	if rc != 1 {
		t.Errorf("expected exit 1, got %d", rc)
	}
}

func TestRunPackInspectCmd_BadFlagsExit2(t *testing.T) {
	workDir := setupCLIWorkdir(t)
	rc := runPackInspectCmdW([]string{"--dir", workDir, "--json", "--quiet", "x"}, &bytes.Buffer{})
	if rc != 2 {
		t.Errorf("expected exit 2 on mutex flags, got %d", rc)
	}
}

func TestRunPackInspectCmd_NoArgsExit2(t *testing.T) {
	rc := runPackInspectCmdW([]string{}, &bytes.Buffer{})
	if rc != 2 {
		t.Errorf("expected exit 2 on missing arg, got %d", rc)
	}
}

func TestRunPackInspectCmd_PathArg(t *testing.T) {
	path := buildFixturePack(t, true, false)

	var buf bytes.Buffer
	rc := runPackInspectCmdW([]string{path}, &buf)
	if rc != 0 {
		t.Fatalf("inspect rc=%d, output=%s", rc, buf.String())
	}
	if !strings.Contains(buf.String(), "salvia_officinalis.glb") {
		t.Errorf("expected basename in output:\n%s", buf.String())
	}
}

func TestRunPackInspectCmd_PathArgMissing(t *testing.T) {
	rc := runPackInspectCmdW([]string{"/nonexistent/path/x.glb"}, &bytes.Buffer{})
	if rc != 1 {
		t.Errorf("expected exit 1, got %d", rc)
	}
}

func TestVariantBytes_DedupesSharedBufferViews(t *testing.T) {
	// Two primitives sharing the same accessor → same bufferView →
	// must be counted once.
	one := 1
	doc := &gltfDoc{
		BufferViews: []gltfBufferView{
			{Buffer: 0, ByteLength: 100},
			{Buffer: 0, ByteLength: 200},
		},
		Accessors: []gltfAccessor{
			{BufferView: intPtr(0), ComponentType: 5126, Count: 1, Type: "VEC3"},
			{BufferView: intPtr(1), ComponentType: 5126, Count: 1, Type: "VEC3"},
		},
		Meshes: []gltfMesh{
			{
				Primitives: []gltfPrimitive{
					{Attributes: map[string]int{"POSITION": 0}, Indices: &one},
					{Attributes: map[string]int{"POSITION": 0}}, // shares accessor 0
				},
			},
		},
	}
	got := variantBytes(doc, []int{0})
	if got != 300 {
		t.Errorf("expected 300 (100+200, no dedup error), got %d", got)
	}
}

func TestVariantBytes_ExcludesImageBufferViews(t *testing.T) {
	// An image-bound bufferView must not contribute to mesh byte count.
	doc := &gltfDoc{
		BufferViews: []gltfBufferView{
			{Buffer: 0, ByteLength: 50},   // mesh
			{Buffer: 0, ByteLength: 9999}, // image
		},
		Accessors: []gltfAccessor{
			{BufferView: intPtr(0), ComponentType: 5126, Count: 1, Type: "VEC3"},
		},
		Images: []gltfImage{
			{BufferView: intPtr(1)},
		},
		Meshes: []gltfMesh{
			{Primitives: []gltfPrimitive{{Attributes: map[string]int{"POSITION": 0}}}},
		},
	}
	got := variantBytes(doc, []int{0})
	if got != 50 {
		t.Errorf("expected 50 (image bv excluded), got %d", got)
	}
}


func TestRenderHuman_VariantAbsentLines(t *testing.T) {
	rep := &PackInspectReport{
		Path:      "/tmp/x.glb",
		Size:      1234,
		SizeHuman: "1 KB",
		SHA256:    strings.Repeat("a", 64),
		Format:    "Pack v1",
		BakeID:    "2026-04-08T00:00:00Z",
		Meta: &PackMeta{
			FormatVersion: 1, BakeID: "2026-04-08T00:00:00Z",
			Species: "x", CommonName: "X",
			Footprint: Footprint{1, 1},
			Fade:      FadeBand{0.1, 0.2, 0.3},
		},
		Variants: VariantSummary{
			Side: &VariantGroup{Count: 4, AvgBytes: 1024},
		},
		Validation: "OK",
		Valid:      true,
	}
	var buf bytes.Buffer
	renderHuman(&buf, rep)
	out := buf.String()
	for _, want := range []string{
		"view_side:    4 variants × avg 1 KB",
		"view_top:     (absent)",
		"view_tilted:  (absent)",
		"view_dome:    (absent)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in output:\n%s", want, out)
		}
	}
}
