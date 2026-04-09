package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestRunPack_HappyPath_AllThree exercises the same fixture as
// TestHandleBuildPack_HappyPath_AllThree but skips the HTTP shell.
// It pins the contract that RunPack populates Species, Size,
// HasTilted, HasDome and writes the pack file to disk.
func TestRunPack_HappyPath_AllThree(t *testing.T) {
	env := newPackTestEnv(t)
	env.registerSource(t)
	env.writeIntermediate(t, "_billboard.glb",
		makeMinimalGLB(t, []string{"billboard_top", "s0"}, nil))
	env.writeIntermediate(t, "_billboard_tilted.glb",
		makeMinimalGLB(t, []string{"billboard_tilted_top", "s0"}, nil))
	env.writeIntermediate(t, "_volumetric.glb",
		makeMinimalGLB(t, []string{"vlod0_root", "vlod0"}, nil))

	res := RunPack(env.id, env.originalsDir, env.settingsDir, env.outputsDir, env.distDir, env.store, ResolverOptions{})

	if res.Status != "ok" {
		t.Fatalf("Status = %q, want ok (err: %v)", res.Status, res.Err)
	}
	if res.Species == "" {
		t.Fatalf("Species empty on success result")
	}
	if res.Size <= 0 {
		t.Fatalf("Size = %d, want > 0", res.Size)
	}
	if !res.HasTilted {
		t.Errorf("HasTilted = false, want true")
	}
	if !res.HasDome {
		t.Errorf("HasDome = false, want true")
	}
	distPath := filepath.Join(env.distDir, res.Species+".glb")
	if _, err := os.Stat(distPath); err != nil {
		t.Fatalf("dist pack not written: %v", err)
	}
}

// TestRunPack_HappyPath_BillboardOnly checks the optional flags
// stay false when only the side intermediate is present.
func TestRunPack_HappyPath_BillboardOnly(t *testing.T) {
	env := newPackTestEnv(t)
	env.registerSource(t)
	env.writeIntermediate(t, "_billboard.glb",
		makeMinimalGLB(t, []string{"billboard_top", "s0"}, nil))

	res := RunPack(env.id, env.originalsDir, env.settingsDir, env.outputsDir, env.distDir, env.store, ResolverOptions{})

	if res.Status != "ok" {
		t.Fatalf("Status = %q, want ok (err: %v)", res.Status, res.Err)
	}
	if res.HasTilted {
		t.Errorf("HasTilted = true, want false")
	}
	if res.HasDome {
		t.Errorf("HasDome = true, want false")
	}
}

// TestRunPack_MissingSide returns "missing-source" — the CLI
// surfaces this as a failed row, the handler as a 400.
func TestRunPack_MissingSide(t *testing.T) {
	env := newPackTestEnv(t)
	env.registerSource(t)
	// no intermediates written

	res := RunPack(env.id, env.originalsDir, env.settingsDir, env.outputsDir, env.distDir, env.store, ResolverOptions{})

	if res.Status != "missing-source" {
		t.Fatalf("Status = %q, want missing-source (err: %v)", res.Status, res.Err)
	}
	// dist file should NOT have been created
	matches, _ := filepath.Glob(filepath.Join(env.distDir, "*.glb"))
	if len(matches) > 0 {
		t.Errorf("dist dir leaked files on missing-source: %v", matches)
	}
}

// TestRunPack_Oversize uses the same 6 MiB ballast trick as the
// handler oversize test to provoke *PackOversizeError.
func TestRunPack_Oversize(t *testing.T) {
	env := newPackTestEnv(t)
	env.registerSource(t)
	env.writeIntermediate(t, "_billboard.glb", makeOversizeGLB(t))

	res := RunPack(env.id, env.originalsDir, env.settingsDir, env.outputsDir, env.distDir, env.store, ResolverOptions{})

	if res.Status != "oversize" {
		t.Fatalf("Status = %q, want oversize (err: %v)", res.Status, res.Err)
	}
	var oversize *PackOversizeError
	if !errors.As(res.Err, &oversize) {
		t.Fatalf("Err = %T, want *PackOversizeError", res.Err)
	}
	matches, _ := filepath.Glob(filepath.Join(env.distDir, "*.glb"))
	if len(matches) > 0 {
		t.Errorf("dist dir leaked files on oversize: %v", matches)
	}
}

// TestRunPack_BuildMetaFails removes the source GLB so
// BuildPackMetaFromBake fails its footprint read. The result
// should land in "failed" with the build-meta error wrapped.
func TestRunPack_BuildMetaFails(t *testing.T) {
	env := newPackTestEnv(t)
	env.registerSource(t)
	// Delete the source GLB BuildPackMetaFromBake needs.
	if err := os.Remove(filepath.Join(env.originalsDir, env.id+".glb")); err != nil {
		t.Fatalf("remove source: %v", err)
	}
	env.writeIntermediate(t, "_billboard.glb",
		makeMinimalGLB(t, []string{"billboard_top", "s0"}, nil))

	res := RunPack(env.id, env.originalsDir, env.settingsDir, env.outputsDir, env.distDir, env.store, ResolverOptions{})

	if res.Status != "failed" {
		t.Fatalf("Status = %q, want failed (err: %v)", res.Status, res.Err)
	}
	if !strings.Contains(res.Err.Error(), "build meta:") {
		t.Errorf("Err = %q, want it to contain build meta:", res.Err.Error())
	}
}
