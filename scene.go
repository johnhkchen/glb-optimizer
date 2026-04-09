package main

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
)

// Strategy describes how to process an asset for a scene.
type Strategy struct {
	Name           string  // "parametric", "gltfpack", "volumetric"
	Simplification float64 // for gltfpack
	Aggressive     bool
	Permissive     bool
	VolumetricLOD  int // 0-3, for volumetric asset lookup
}

// SelectStrategy picks the distillation strategy based on asset type and scene role.
func SelectStrategy(assetType, sceneRole string) Strategy {
	switch assetType {
	case "hard-surface":
		switch sceneRole {
		case "hero":
			return Strategy{Name: "parametric"}
		case "mid-ground":
			return Strategy{Name: "gltfpack", Simplification: 0.2, Aggressive: true}
		case "background":
			return Strategy{Name: "gltfpack", Simplification: 0.05, Aggressive: true, Permissive: true}
		}
	case "organic":
		switch sceneRole {
		case "hero":
			return Strategy{Name: "volumetric", VolumetricLOD: 0}
		case "mid-ground":
			return Strategy{Name: "volumetric", VolumetricLOD: 1}
		case "background":
			return Strategy{Name: "volumetric", VolumetricLOD: 2}
		}
	}
	// Fallback: gltfpack with moderate simplification
	return Strategy{Name: "gltfpack", Simplification: 0.5}
}

// AllocateBudget distributes the triangle budget across assets by scene role.
// Returns a map from asset label to allocated triangle budget.
func AllocateBudget(budget SceneBudget, assets []SceneAsset) map[string]int {
	allocation := make(map[string]int)

	// Group by role
	roleAssets := map[string][]string{
		"hero":       {},
		"mid-ground": {},
		"background": {},
	}
	for _, a := range assets {
		roleAssets[a.SceneRole] = append(roleAssets[a.SceneRole], a.Label)
	}

	// Budget shares: hero 50%, mid-ground 30%, background 15%, reserve 5%
	shares := map[string]float64{
		"hero":       0.50,
		"mid-ground": 0.30,
		"background": 0.15,
	}

	total := budget.MaxTriangles
	for role, labels := range roleAssets {
		if len(labels) == 0 {
			continue
		}
		share := shares[role]
		roleBudget := int(float64(total) * share)
		perAsset := roleBudget / len(labels)
		for _, label := range labels {
			allocation[label] = perAsset
		}
	}

	return allocation
}

// RunParametricReconstruct invokes the parametric reconstruction Python script.
func RunParametricReconstruct(inputPath, outputPath string) (string, error) {
	cmd := exec.Command("python3", "scripts/parametric_reconstruct.py",
		"--input", inputPath,
		"--output", outputPath)
	output, err := cmd.CombinedOutput()
	return string(output), err
}

// CountTrianglesGLB reads a GLB file and returns the total triangle count.
// It parses only the JSON chunk to read accessor counts.
func CountTrianglesGLB(path string) (int, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, fmt.Errorf("read GLB: %w", err)
	}

	if len(data) < 12 {
		return 0, fmt.Errorf("file too small for GLB header")
	}

	// Verify magic
	magic := uint32(data[0]) | uint32(data[1])<<8 | uint32(data[2])<<16 | uint32(data[3])<<24
	if magic != 0x46546C67 {
		return 0, fmt.Errorf("not a GLB file")
	}

	// Read JSON chunk
	if len(data) < 20 {
		return 0, fmt.Errorf("file too small for chunk header")
	}
	chunkLen := uint32(data[12]) | uint32(data[13])<<8 | uint32(data[14])<<16 | uint32(data[15])<<24
	chunkType := uint32(data[16]) | uint32(data[17])<<8 | uint32(data[18])<<16 | uint32(data[19])<<24
	if chunkType != 0x4E4F534A {
		return 0, fmt.Errorf("expected JSON chunk")
	}

	if len(data) < int(20+chunkLen) {
		return 0, fmt.Errorf("JSON chunk truncated")
	}

	jsonData := data[20 : 20+chunkLen]

	var gltf struct {
		Accessors []struct {
			Count int `json:"count"`
		} `json:"accessors"`
		Meshes []struct {
			Primitives []struct {
				Indices int `json:"indices"`
			} `json:"primitives"`
		} `json:"meshes"`
	}

	if err := json.Unmarshal(jsonData, &gltf); err != nil {
		return 0, fmt.Errorf("parse glTF JSON: %w", err)
	}

	totalTris := 0
	for _, mesh := range gltf.Meshes {
		for _, prim := range mesh.Primitives {
			if prim.Indices >= 0 && prim.Indices < len(gltf.Accessors) {
				totalTris += gltf.Accessors[prim.Indices].Count / 3
			}
		}
	}

	return totalTris, nil
}
