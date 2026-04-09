package main

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestDiscoverPackableIDs_Filters(t *testing.T) {
	dir := t.TempDir()
	files := []string{
		"a_billboard.glb",                  // ok → "a"
		"b_billboard.glb",                  // ok → "b"
		"b_billboard_tilted.glb",           // tilted partner of b
		"b_volumetric.glb",                 // dome partner of b
		"c_billboard_tilted.glb",           // tilted-only, should NOT pack
		"d.glb",                            // unrelated
		"e_billboard_tilted_billboard.glb", // pathological — _billboard.glb suffix but NOT a side billboard. Excluded by tilted-rejection.
	}
	for _, name := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte("x"), 0644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	if err := os.Mkdir(filepath.Join(dir, "thumbs"), 0755); err != nil {
		t.Fatalf("mkdir thumbs: %v", err)
	}

	got, err := discoverPackableIDs(dir)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	want := []string{"a", "b", "e_billboard_tilted"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestDiscoverPackableIDs_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	got, err := discoverPackableIDs(dir)
	if err != nil {
		t.Fatalf("discover: %v", err)
	}
	if len(got) != 0 {
		t.Fatalf("got %v, want empty slice", got)
	}
}

func TestPrintPackSummary_FormatsTable(t *testing.T) {
	results := []PackResult{
		{ID: "a", Species: "achillea_millefolium", Size: 1234567, HasTilted: true, HasDome: true, Status: "ok"},
		{ID: "b", Species: "rose_julia_child", Size: 4_800_000, HasTilted: true, HasDome: false, Status: "ok"},
		{ID: "c-fail", HasTilted: false, HasDome: false, Status: "failed", Err: errFromString("build meta: missing source")},
	}
	var buf bytes.Buffer
	printPackSummary(&buf, results)
	out := buf.String()

	for _, want := range []string{
		"SPECIES",
		"SIZE",
		"TILTED",
		"DOME",
		"STATUS",
		"achillea_millefolium",
		"rose_julia_child",
		"c-fail",
		"failed",
		"build meta: missing source",
		"TOTAL: 3 packs, 2 ok, 1 failed",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("summary missing %q\nfull output:\n%s", want, out)
		}
	}
}

func TestRunPackAllCmd_HappyPath(t *testing.T) {
	workDir := setupCLIWorkdir(t)
	// Asset ids are chosen so deriveSpeciesFromName({id}.glb) ==
	// {id}: scanExistingFiles forgets the original filename and
	// passes record.Filename = "{id}.glb", so the slug equals the
	// id. Picking ids in species shape keeps the test free of a
	// settings-override fixture.
	registerAsset(t, workDir, "achillea_millefolium", true)
	registerAsset(t, workDir, "rose_julia_child", true)

	rc := runPackAllCmd([]string{"--dir", workDir})
	if rc != 0 {
		t.Fatalf("exit %d, want 0", rc)
	}
	for _, species := range []string{"achillea_millefolium", "rose_julia_child"} {
		p := filepath.Join(workDir, DistPlantsDir, species+".glb")
		if _, err := os.Stat(p); err != nil {
			t.Errorf("missing %s: %v", p, err)
		}
	}
}

func TestRunPackAllCmd_MixedFailure(t *testing.T) {
	workDir := setupCLIWorkdir(t)
	// asset_good has its source GLB; asset_bad has only the side
	// intermediate, so scanExistingFiles never registers a record
	// and RunPack returns "failed: no FileRecord".
	registerAsset(t, workDir, "lavandula_angustifolia", true)
	registerAsset(t, workDir, "echinacea_purpurea", false)

	rc := runPackAllCmd([]string{"--dir", workDir})
	if rc != 1 {
		t.Fatalf("exit %d, want 1", rc)
	}
	good := filepath.Join(workDir, DistPlantsDir, "lavandula_angustifolia.glb")
	if _, err := os.Stat(good); err != nil {
		t.Errorf("good asset pack missing: %v", err)
	}
	bad := filepath.Join(workDir, DistPlantsDir, "echinacea_purpurea.glb")
	if _, err := os.Stat(bad); err == nil {
		t.Errorf("bad asset should NOT have produced a pack")
	}
}

func TestRunPackCmd_SingleAssetHappy(t *testing.T) {
	workDir := setupCLIWorkdir(t)
	registerAsset(t, workDir, "salvia_officinalis", true)

	rc := runPackCmd([]string{"--dir", workDir, "salvia_officinalis"})
	if rc != 0 {
		t.Fatalf("exit %d, want 0", rc)
	}
	p := filepath.Join(workDir, DistPlantsDir, "salvia_officinalis.glb")
	if _, err := os.Stat(p); err != nil {
		t.Errorf("missing pack: %v", err)
	}
}

func TestRunPackCmd_BogusIDExits1(t *testing.T) {
	workDir := setupCLIWorkdir(t)
	rc := runPackCmd([]string{"--dir", workDir, "no-such-asset"})
	if rc != 1 {
		t.Fatalf("exit %d, want 1", rc)
	}
}

// --- helpers ---

// setupCLIWorkdir builds a tempdir with the same layout
// resolveWorkdir would create on a fresh laptop. Returns the
// workdir root.
func setupCLIWorkdir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	// Use resolveWorkdir to mirror exactly what the CLI does.
	wd, err := resolveWorkdir(root)
	if err != nil {
		t.Fatalf("resolveWorkdir: %v", err)
	}
	return wd
}

// registerAsset writes a synthetic source GLB at
// originals/{id}.glb (when writeSource is true) and a side
// billboard intermediate at outputs/{id}_billboard.glb. The
// caller picks ids that already match the species regex so the
// slug derived by BuildPackMetaFromBake equals the id — see the
// caller-side comment for why this side-steps the lost-filename
// issue.
func registerAsset(t *testing.T, workDir, id string, writeSource bool) {
	t.Helper()
	originals := filepath.Join(workDir, "originals")
	outputs := filepath.Join(workDir, "outputs")

	if writeSource {
		src := makeMinimalGLB(t, []string{"trunk"}, nil)
		if err := os.WriteFile(filepath.Join(originals, id+".glb"), src, 0644); err != nil {
			t.Fatalf("write source: %v", err)
		}
	}

	side := makeMinimalGLB(t, []string{"billboard_top", "s0"}, nil)
	if err := os.WriteFile(filepath.Join(outputs, id+"_billboard.glb"), side, 0644); err != nil {
		t.Fatalf("write side: %v", err)
	}
}

// errFromString is a tiny helper for the summary-table test so we
// don't have to import "errors" just for one literal.
func errFromString(s string) error { return &stringErr{s} }

type stringErr struct{ msg string }

func (e *stringErr) Error() string { return e.msg }
