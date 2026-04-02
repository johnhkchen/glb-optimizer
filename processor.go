package main

import (
	"fmt"
	"os/exec"
	"strings"
)

// BuildCommand constructs the gltfpack command line from settings.
func BuildCommand(inputPath, outputPath string, s Settings) []string {
	args := []string{"-i", inputPath, "-o", outputPath}

	if s.Simplification > 0 && s.Simplification < 1.0 {
		args = append(args, "-si", fmt.Sprintf("%.2f", s.Simplification))
	}

	if s.AggressiveSimplify {
		args = append(args, "-sa")
	}

	if s.PermissiveSimplify {
		args = append(args, "-sp")
	}

	if s.LockBorders {
		args = append(args, "-slb")
	}

	switch s.Compression {
	case "cc":
		args = append(args, "-cc", "-ce", "ext")
	case "cz":
		args = append(args, "-cz", "-ce", "ext")
	}

	switch s.TextureCompression {
	case "tc":
		args = append(args, "-tc")
	case "tw":
		args = append(args, "-tw")
	}

	if s.TextureQuality > 0 && s.TextureCompression != "" {
		args = append(args, "-tq", fmt.Sprintf("%d", s.TextureQuality))
	}

	if s.TextureSize > 0 {
		args = append(args, "-tl", fmt.Sprintf("%d", s.TextureSize))
	}

	if s.KeepNodes {
		args = append(args, "-kn")
	}

	if s.KeepMaterials {
		args = append(args, "-km")
	}

	if s.FloatPositions {
		args = append(args, "-vpf")
	}

	return args
}

// FormatCommand returns the full shell command as a string for display.
func FormatCommand(args []string) string {
	parts := make([]string, len(args)+1)
	parts[0] = "gltfpack"
	for i, a := range args {
		if strings.Contains(a, " ") {
			parts[i+1] = fmt.Sprintf("%q", a)
		} else {
			parts[i+1] = a
		}
	}
	return strings.Join(parts, " ")
}

// RunGltfpack executes gltfpack with the given arguments.
// Returns the combined stderr output and any error.
func RunGltfpack(args []string) (string, error) {
	cmd := exec.Command("gltfpack", args...)
	output, err := cmd.CombinedOutput()
	return string(output), err
}
