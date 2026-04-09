package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// packTestEnv bundles the on-disk fixture directories used by the
// /api/pack/:id handler tests. Each test gets a fresh tempdir tree
// so the assertions are hermetic.
type packTestEnv struct {
	originalsDir string
	settingsDir  string
	outputsDir   string
	distDir      string
	store        *FileStore
	id           string
	filename     string
}

func newPackTestEnv(t *testing.T) *packTestEnv {
	t.Helper()
	root := t.TempDir()
	env := &packTestEnv{
		originalsDir: filepath.Join(root, "originals"),
		settingsDir:  filepath.Join(root, "settings"),
		outputsDir:   filepath.Join(root, "outputs"),
		distDir:      filepath.Join(root, "dist", "plants"),
		store:        NewFileStore(),
		id:           "asset-pack-1",
		filename:     "Achillea Millefolium.glb",
	}
	for _, d := range []string{env.originalsDir, env.settingsDir, env.outputsDir, env.distDir} {
		if err := os.MkdirAll(d, 0755); err != nil {
			t.Fatalf("mkdir %s: %v", d, err)
		}
	}
	return env
}

// register the FileRecord and write a synthetic source GLB so
// BuildPackMetaFromBake's footprint reader has something to parse.
func (e *packTestEnv) registerSource(t *testing.T) {
	t.Helper()
	e.store.Add(&FileRecord{
		ID:       e.id,
		Filename: e.filename,
		Status:   StatusDone,
	})
	srcPath := filepath.Join(e.originalsDir, e.id+".glb")
	srcGLB := makeMinimalGLB(t, []string{"trunk"}, nil)
	if err := os.WriteFile(srcPath, srcGLB, 0644); err != nil {
		t.Fatalf("write source: %v", err)
	}
}

func (e *packTestEnv) writeIntermediate(t *testing.T, suffix string, glb []byte) {
	t.Helper()
	p := filepath.Join(e.outputsDir, e.id+suffix)
	if err := os.WriteFile(p, glb, 0644); err != nil {
		t.Fatalf("write %s: %v", suffix, err)
	}
}

func (e *packTestEnv) handler() http.HandlerFunc {
	return handleBuildPack(e.store, e.originalsDir, e.settingsDir, e.outputsDir, e.distDir)
}

func (e *packTestEnv) post(t *testing.T) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/api/pack/"+e.id, nil)
	rr := httptest.NewRecorder()
	e.handler()(rr, req)
	return rr
}

// --- Happy paths ---

func TestHandleBuildPack_HappyPath_AllThree(t *testing.T) {
	env := newPackTestEnv(t)
	env.registerSource(t)
	env.writeIntermediate(t, "_billboard.glb",
		makeMinimalGLB(t, []string{"billboard_top", "s0"}, nil))
	env.writeIntermediate(t, "_billboard_tilted.glb",
		makeMinimalGLB(t, []string{"t0"}, nil))
	env.writeIntermediate(t, "_volumetric.glb",
		makeMinimalGLB(t, []string{"v0", "v1"}, []float64{0, 0.5}))

	rr := env.post(t)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		PackPath string `json:"pack_path"`
		Size     int    `json:"size"`
		Species  string `json:"species"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if resp.Species != "achillea_millefolium" {
		t.Fatalf("species = %q, want achillea_millefolium", resp.Species)
	}
	wantPath := filepath.Join(env.distDir, "achillea_millefolium.glb")
	if resp.PackPath != wantPath {
		t.Fatalf("pack_path = %q, want %q", resp.PackPath, wantPath)
	}
	info, err := os.Stat(wantPath)
	if err != nil {
		t.Fatalf("stat pack: %v", err)
	}
	if int(info.Size()) != resp.Size {
		t.Fatalf("on-disk size %d != response size %d", info.Size(), resp.Size)
	}
	if resp.Size == 0 {
		t.Fatalf("size is zero")
	}
}

func TestHandleBuildPack_TiltedOnly(t *testing.T) {
	env := newPackTestEnv(t)
	env.registerSource(t)
	env.writeIntermediate(t, "_billboard.glb",
		makeMinimalGLB(t, []string{"billboard_top", "s0"}, nil))
	env.writeIntermediate(t, "_billboard_tilted.glb",
		makeMinimalGLB(t, []string{"t0"}, nil))

	rr := env.post(t)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleBuildPack_VolumetricOnly(t *testing.T) {
	env := newPackTestEnv(t)
	env.registerSource(t)
	env.writeIntermediate(t, "_billboard.glb",
		makeMinimalGLB(t, []string{"billboard_top", "s0"}, nil))
	env.writeIntermediate(t, "_volumetric.glb",
		makeMinimalGLB(t, []string{"v0", "v1"}, []float64{0, 0.5}))

	rr := env.post(t)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", rr.Code, rr.Body.String())
	}
}

// --- Error paths ---

func TestHandleBuildPack_MissingSide(t *testing.T) {
	env := newPackTestEnv(t)
	env.registerSource(t)
	// Only the optional intermediates exist; required side is absent.
	env.writeIntermediate(t, "_volumetric.glb",
		makeMinimalGLB(t, []string{"v0"}, nil))

	rr := env.post(t)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("want 400, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "missing intermediate") {
		t.Fatalf("body should mention missing intermediate, got %s", rr.Body.String())
	}
}

func TestHandleBuildPack_UnknownID(t *testing.T) {
	env := newPackTestEnv(t)
	// No registerSource — store is empty.
	rr := env.post(t)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("want 404, got %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestHandleBuildPack_MethodNotAllowed(t *testing.T) {
	env := newPackTestEnv(t)
	env.registerSource(t)
	req := httptest.NewRequest(http.MethodGet, "/api/pack/"+env.id, nil)
	rr := httptest.NewRecorder()
	env.handler()(rr, req)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("want 405, got %d", rr.Code)
	}
}

func TestHandleBuildPack_OversizePack(t *testing.T) {
	env := newPackTestEnv(t)
	env.registerSource(t)
	env.writeIntermediate(t, "_billboard.glb", makeOversizeGLB(t))

	rr := env.post(t)
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("want 413, got %d body=%s", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "5 MB") {
		t.Fatalf("body should mention 5 MB, got %s", rr.Body.String())
	}
}

// makeOversizeGLB constructs a side intermediate with a 6 MiB ballast
// bufferView, mirroring TestCombine_SizeCapRejection. The merged pack
// will exceed the 5 MiB cap and trigger CombinePack's size error.
func makeOversizeGLB(t *testing.T) []byte {
	t.Helper()
	doc := &gltfDoc{
		Asset:   json.RawMessage(`{"version":"2.0"}`),
		Scene:   0,
		Scenes:  []gltfScene{{Nodes: []int{0}}},
		Buffers: []gltfBuffer{{}},
	}
	bin := &bytes.Buffer{}

	idxOff := bin.Len()
	bin.Write([]byte{0, 0, 1, 0, 2, 0})
	for bin.Len()%4 != 0 {
		bin.WriteByte(0)
	}
	posOff := bin.Len()
	for k := 0; k < 9; k++ {
		writeF32(bin, float32(k))
	}
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
		t.Fatalf("makeOversizeGLB writeGLB: %v", err)
	}
	return raw
}
