package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
)

import _ "embed"

//go:embed scripts/remesh_lod.py
var remeshScript []byte

// BlenderInfo holds detected Blender installation info.
type BlenderInfo struct {
	Available   bool   `json:"available"`
	Path        string `json:"path,omitempty"`
	Version     string `json:"version,omitempty"`
	ResourceDir string `json:"-"`
}

// DetectBlender searches for a usable Blender installation.
func DetectBlender() BlenderInfo {
	// Try common macOS .app locations first
	if runtime.GOOS == "darwin" {
		appPaths := []string{
			"/Applications/Blender.app",
			"/Volumes/ext1/Applications/Blender.app",
			filepath.Join(os.Getenv("HOME"), "Applications/Blender.app"),
		}
		for _, appPath := range appPaths {
			binPath := filepath.Join(appPath, "Contents/MacOS/Blender")
			resPath := filepath.Join(appPath, "Contents/Resources")
			if _, err := os.Stat(binPath); err == nil {
				info := BlenderInfo{
					Available:   true,
					Path:        binPath,
					ResourceDir: resPath,
				}
				// Get version
				if v := getBlenderVersion(info); v != "" {
					info.Version = v
				}
				return info
			}
		}
	}

	// Try PATH
	if p, err := exec.LookPath("blender"); err == nil {
		info := BlenderInfo{Available: true, Path: p}
		if v := getBlenderVersion(info); v != "" {
			info.Version = v
		}
		return info
	}

	return BlenderInfo{}
}

func getBlenderVersion(info BlenderInfo) string {
	cmd := exec.Command(info.Path, "-b", "--python-expr",
		`import bpy; print('BVER=' + '.'.join(map(str, bpy.app.version)))`)
	if info.ResourceDir != "" {
		cmd.Env = append(os.Environ(), "BLENDER_SYSTEM_RESOURCES="+info.ResourceDir)
	}
	out, err := cmd.CombinedOutput()
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "BVER=") {
			return strings.TrimPrefix(line, "BVER=")
		}
	}
	return ""
}

// BlenderLODConfig defines settings for a single Blender LOD level.
type BlenderLODConfig struct {
	Label        string
	Mode         string  // remesh_decimate, decimate, planar
	VoxelSize    float64 // 0 = auto
	DecimateRatio float64
	PlanarAngle  float64
	TargetTris   int
}

// DefaultBlenderLODs uses collapse-decimate for all levels.
// Voxel remesh destroys thin geometry (leaves, grass, hair) so we avoid it by default.
var DefaultBlenderLODs = []BlenderLODConfig{
	{Label: "lod0", Mode: "decimate", DecimateRatio: 0.5, PlanarAngle: 3},
	{Label: "lod1", Mode: "decimate", DecimateRatio: 0.2, PlanarAngle: 5},
	{Label: "lod2", Mode: "decimate", DecimateRatio: 0.05, PlanarAngle: 8},
	{Label: "lod3", Mode: "decimate", DecimateRatio: 0.03, PlanarAngle: 10},
}

// RunBlenderLOD runs the Blender remesh/decimate script for a single LOD level.
func RunBlenderLOD(info BlenderInfo, scriptPath, inputPath, outputPath string, cfg BlenderLODConfig) (string, error) {
	args := []string{
		"-b", "--python", scriptPath, "--",
		"--input", inputPath,
		"--output", outputPath,
		"--mode", cfg.Mode,
		"--decimate-ratio", fmt.Sprintf("%.4f", cfg.DecimateRatio),
		"--planar-angle", fmt.Sprintf("%.1f", cfg.PlanarAngle),
	}

	if cfg.VoxelSize > 0 {
		args = append(args, "--voxel-size", fmt.Sprintf("%.6f", cfg.VoxelSize))
	}
	if cfg.TargetTris > 0 {
		args = append(args, "--target-tris", fmt.Sprintf("%d", cfg.TargetTris))
	}

	cmd := exec.Command(info.Path, args...)
	if info.ResourceDir != "" {
		cmd.Env = append(os.Environ(), "BLENDER_SYSTEM_RESOURCES="+info.ResourceDir)
	}

	output, err := cmd.CombinedOutput()
	return string(output), err
}

// WriteEmbeddedScript writes the embedded Python script to a temporary file and returns its path.
func WriteEmbeddedScript(workDir string) (string, error) {
	scriptPath := filepath.Join(workDir, "remesh_lod.py")
	if err := os.WriteFile(scriptPath, remeshScript, 0644); err != nil {
		return "", fmt.Errorf("failed to write blender script: %w", err)
	}
	return scriptPath, nil
}
