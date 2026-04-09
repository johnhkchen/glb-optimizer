package main

import (
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strings"
)

// PackFormatVersion is the on-disk schema version for Pack v1 metadata
// embedded in the root scene's extras.plantastic block of every asset
// pack GLB. Bumped on breaking schema changes. The contract is frozen
// in docs/active/epics/E-002-asset-pack-format.md.
const PackFormatVersion = 1

// speciesRe enforces the lowercase-latin-name slug rule for species ids:
// must start with [a-z] and contain only [a-z0-9_] thereafter.
var speciesRe = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)

// Footprint records the per-instance physical dimensions the consumer
// uses to scale unit-reference geometry and to lay out no-walk zones.
type Footprint struct {
	CanopyRadiusM float64 `json:"canopy_radius_m"`
	HeightM       float64 `json:"height_m"`
}

// FadeBand records the three crossfade thresholds (band 1 start/end,
// band 2 start) used by the consumer's hybrid-impostor renderer. Band 2
// end is implicit 1.0. All values are in [0,1] of |dot(camDir,-Y)| and
// must satisfy low_start < low_end < high_start <= 1.0.
type FadeBand struct {
	LowStart  float64 `json:"low_start"`
	LowEnd    float64 `json:"low_end"`
	HighStart float64 `json:"high_start"`
}

// PackMeta is the v1 contract for the metadata embedded at
// scene.extras.plantastic of every asset pack GLB. Field declaration
// order matches the canonical example in E-002.
type PackMeta struct {
	FormatVersion int       `json:"format_version"`
	BakeID        string    `json:"bake_id"`
	Species       string    `json:"species"`
	CommonName    string    `json:"common_name"`
	Footprint     Footprint `json:"footprint"`
	Fade          FadeBand  `json:"fade"`
}

// Validate returns the first failing field as an error, or nil if the
// metadata satisfies the v1 contract. Validation order is:
// version → required strings → footprint dims → fade range → fade order.
func (m PackMeta) Validate() error {
	if m.FormatVersion != PackFormatVersion {
		return fmt.Errorf("unsupported format_version: %d (expected %d)", m.FormatVersion, PackFormatVersion)
	}
	if strings.TrimSpace(m.Species) == "" {
		return fmt.Errorf("species is required")
	}
	if !speciesRe.MatchString(m.Species) {
		return fmt.Errorf("species %q must match ^[a-z][a-z0-9_]*$", m.Species)
	}
	if strings.TrimSpace(m.CommonName) == "" {
		return fmt.Errorf("common_name is required")
	}
	if strings.TrimSpace(m.BakeID) == "" {
		return fmt.Errorf("bake_id is required")
	}
	if err := checkPositive("footprint.canopy_radius_m", m.Footprint.CanopyRadiusM); err != nil {
		return err
	}
	if err := checkPositive("footprint.height_m", m.Footprint.HeightM); err != nil {
		return err
	}
	if err := checkRange("fade.low_start", m.Fade.LowStart, 0, 1); err != nil {
		return err
	}
	if err := checkRange("fade.low_end", m.Fade.LowEnd, 0, 1); err != nil {
		return err
	}
	if err := checkRange("fade.high_start", m.Fade.HighStart, 0, 1); err != nil {
		return err
	}
	if !(m.Fade.LowStart < m.Fade.LowEnd) {
		return fmt.Errorf("fade.low_start (%g) must be < fade.low_end (%g)", m.Fade.LowStart, m.Fade.LowEnd)
	}
	if !(m.Fade.LowEnd < m.Fade.HighStart) {
		return fmt.Errorf("fade.low_end (%g) must be < fade.high_start (%g)", m.Fade.LowEnd, m.Fade.HighStart)
	}
	if m.Fade.HighStart > 1.0 {
		return fmt.Errorf("fade.high_start (%g) must be <= 1.0", m.Fade.HighStart)
	}
	return nil
}

// checkPositive verifies that v is finite and strictly greater than zero.
// Used for footprint dims, where checkRange's inclusive lower bound would
// wrongly admit zero.
func checkPositive(name string, v float64) error {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return fmt.Errorf("%s must be finite, got %v", name, v)
	}
	if v <= 0 {
		return fmt.Errorf("%s must be > 0, got %g", name, v)
	}
	return nil
}

// ToExtras returns the canonical map shape for embedding under
// scene.extras["plantastic"] in a glTF asset pack. Built field-by-field
// for determinism and review-friendliness.
func (m PackMeta) ToExtras() map[string]any {
	return map[string]any{
		"format_version": m.FormatVersion,
		"bake_id":        m.BakeID,
		"species":        m.Species,
		"common_name":    m.CommonName,
		"footprint": map[string]any{
			"canopy_radius_m": m.Footprint.CanopyRadiusM,
			"height_m":        m.Footprint.HeightM,
		},
		"fade": map[string]any{
			"low_start":  m.Fade.LowStart,
			"low_end":    m.Fade.LowEnd,
			"high_start": m.Fade.HighStart,
		},
	}
}

// ParsePackMeta decodes a JSON blob (typically the bytes pulled from
// extras.plantastic on the consumer side) into a validated PackMeta.
// Errors from decode and validate are wrapped with a "pack_meta:" prefix.
func ParsePackMeta(raw json.RawMessage) (PackMeta, error) {
	var m PackMeta
	if err := json.Unmarshal(raw, &m); err != nil {
		return PackMeta{}, fmt.Errorf("pack_meta: decode: %w", err)
	}
	if err := m.Validate(); err != nil {
		return PackMeta{}, fmt.Errorf("pack_meta: validate: %w", err)
	}
	return m, nil
}
