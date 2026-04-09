package main

import (
	"encoding/json"
	"fmt"
	"os/exec"
)

// ClassificationResult is the parsed stdout of scripts/classify_shape.py.
// Features are kept opaque (a generic JSON map) because they're written
// straight into the analytics event payload, which is also opaque per
// docs/knowledge/analytics-schema.md.
type ClassificationResult struct {
	Category      string                 `json:"category"`
	Confidence    float64                `json:"confidence"`
	IsHardSurface bool                   `json:"is_hard_surface"`
	Features      map[string]interface{} `json:"features"`
}

// RunClassifier shells out to the Python classifier and parses the
// single JSON line written to stdout. The Go wrapper enforces the
// category enum on the way out — a script that prints "spirals" must
// not poison settings on disk.
//
// Returns a *ClassificationResult on success or an error wrapping the
// stderr text on subprocess failure / parse failure / unknown category.
func RunClassifier(glbPath string) (*ClassificationResult, error) {
	cmd := exec.Command("python3", "scripts/classify_shape.py", glbPath)
	stdout, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			return nil, fmt.Errorf("classify_shape.py failed: %s: %w", string(exitErr.Stderr), err)
		}
		return nil, fmt.Errorf("classify_shape.py: %w", err)
	}

	var result ClassificationResult
	if err := json.Unmarshal(stdout, &result); err != nil {
		return nil, fmt.Errorf("parse classifier output: %w", err)
	}

	if !validShapeCategories[result.Category] {
		return nil, fmt.Errorf("classifier returned unknown category %q", result.Category)
	}
	if result.Confidence < 0 || result.Confidence > 1 {
		return nil, fmt.Errorf("classifier returned out-of-range confidence: %v", result.Confidence)
	}
	return &result, nil
}
