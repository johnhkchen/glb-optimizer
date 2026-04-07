package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
)

// SettingsSchemaVersion is the current on-disk schema version for AssetSettings.
// Bump this when the shape of AssetSettings changes in a way that requires a
// migration. See docs/knowledge/settings-schema.md for the migration policy.
const SettingsSchemaVersion = 1

// AssetSettings is the per-asset, persisted bake/tuning configuration. It
// captures every parameter that affects the volumetric bake (slice count,
// dome height, exposure, light intensities, env map intensity, alpha test,
// color calibration mode). Field declaration order is also the on-disk JSON
// order.
type AssetSettings struct {
	SchemaVersion        int     `json:"schema_version"`
	VolumetricLayers     int     `json:"volumetric_layers"`
	VolumetricResolution int     `json:"volumetric_resolution"`
	DomeHeightFactor     float64 `json:"dome_height_factor"`
	BakeExposure         float64 `json:"bake_exposure"`
	AmbientIntensity     float64 `json:"ambient_intensity"`
	HemisphereIntensity  float64 `json:"hemisphere_intensity"`
	KeyLightIntensity    float64 `json:"key_light_intensity"`
	BottomFillIntensity  float64 `json:"bottom_fill_intensity"`
	EnvMapIntensity      float64 `json:"env_map_intensity"`
	AlphaTest             float64 `json:"alpha_test"`
	LightingPreset        string  `json:"lighting_preset"`
	SliceDistributionMode string  `json:"slice_distribution_mode"`
	GroundAlign           bool    `json:"ground_align"`
	ColorCalibrationMode  string  `json:"color_calibration_mode"`
	ReferenceImagePath    string  `json:"reference_image_path,omitempty"`
}

// DefaultSettings returns the canonical v1 defaults. These match the
// hardcoded constants in static/app.js as of T-002-01.
func DefaultSettings() *AssetSettings {
	return &AssetSettings{
		SchemaVersion:        SettingsSchemaVersion,
		VolumetricLayers:     4,
		VolumetricResolution: 512,
		DomeHeightFactor:     0.5,
		BakeExposure:         1.0,
		AmbientIntensity:     0.5,
		HemisphereIntensity:  1.0,
		KeyLightIntensity:    1.4,
		BottomFillIntensity:  0.4,
		EnvMapIntensity:      1.2,
		AlphaTest:             0.10,
		LightingPreset:        "default",
		SliceDistributionMode: "visual-density",
		GroundAlign:           true,
		ColorCalibrationMode:  "none",
		ReferenceImagePath:    "",
	}
}

// validResolutions enumerates the allowed values for VolumetricResolution.
var validResolutions = map[int]bool{
	128: true, 256: true, 512: true, 1024: true, 2048: true,
}

// validLightingPresets enumerates the named lighting presets exposed
// in the tuning panel. The full preset definitions (colors, intensities,
// env gradient) live in static/presets/lighting.js — the backend only
// validates that the id is one of the known set. Added in T-007-01.
var validLightingPresets = map[string]bool{
	"default":     true,
	"midday-sun":  true,
	"overcast":    true,
	"golden-hour": true,
	"dusk":        true,
	"indoor":      true,
}

// validSliceDistributionModes enumerates the allowed values for
// SliceDistributionMode (T-005-01). See docs/knowledge/settings-schema.md
// for the per-mode behavior table.
var validSliceDistributionModes = map[string]bool{
	"equal-height":    true,
	"vertex-quantile": true,
	"visual-density":  true,
}

// validColorCalibrationModes enumerates the allowed values for
// ColorCalibrationMode (T-005-03). The full preset enum lands in S-007.
var validColorCalibrationModes = map[string]bool{
	"none":                 true,
	"from-reference-image": true,
}

// Validate checks the AssetSettings against the v1 schema. It returns the
// first failing field as an error. Successful validation returns nil.
func (s *AssetSettings) Validate() error {
	if s.SchemaVersion != SettingsSchemaVersion {
		return fmt.Errorf("unsupported schema_version: %d (expected %d)", s.SchemaVersion, SettingsSchemaVersion)
	}
	if s.VolumetricLayers < 1 || s.VolumetricLayers > 16 {
		return fmt.Errorf("volumetric_layers out of range [1,16]: %d", s.VolumetricLayers)
	}
	if !validResolutions[s.VolumetricResolution] {
		return fmt.Errorf("volumetric_resolution must be one of {128,256,512,1024,2048}: got %d", s.VolumetricResolution)
	}
	if err := checkRange("dome_height_factor", s.DomeHeightFactor, 0, 2); err != nil {
		return err
	}
	if err := checkRange("bake_exposure", s.BakeExposure, 0, 4); err != nil {
		return err
	}
	if err := checkRange("ambient_intensity", s.AmbientIntensity, 0, 4); err != nil {
		return err
	}
	if err := checkRange("hemisphere_intensity", s.HemisphereIntensity, 0, 4); err != nil {
		return err
	}
	if err := checkRange("key_light_intensity", s.KeyLightIntensity, 0, 8); err != nil {
		return err
	}
	if err := checkRange("bottom_fill_intensity", s.BottomFillIntensity, 0, 4); err != nil {
		return err
	}
	if err := checkRange("env_map_intensity", s.EnvMapIntensity, 0, 4); err != nil {
		return err
	}
	if err := checkRange("alpha_test", s.AlphaTest, 0, 1); err != nil {
		return err
	}
	if !validLightingPresets[s.LightingPreset] {
		return fmt.Errorf("lighting_preset %q is not a known preset", s.LightingPreset)
	}
	if !validSliceDistributionModes[s.SliceDistributionMode] {
		return fmt.Errorf("slice_distribution_mode %q is not a known mode", s.SliceDistributionMode)
	}
	if !validColorCalibrationModes[s.ColorCalibrationMode] {
		return fmt.Errorf("color_calibration_mode %q is not a known mode", s.ColorCalibrationMode)
	}
	// GroundAlign is a bool; both values are valid.
	// ReferenceImagePath is a free string; empty means "not set".
	return nil
}

func checkRange(name string, v, lo, hi float64) error {
	if math.IsNaN(v) || math.IsInf(v, 0) {
		return fmt.Errorf("%s must be finite, got %v", name, v)
	}
	if v < lo || v > hi {
		return fmt.Errorf("%s out of range [%g,%g]: %g", name, lo, hi, v)
	}
	return nil
}

// SettingsDifferFromDefaults reports whether any user-facing field of s
// diverges from DefaultSettings(). SchemaVersion is intentionally excluded
// so that loading a future schema version does not falsely flag every asset
// as dirty. Fields are enumerated explicitly (rather than via reflect) so
// adding a new field forces a compile-time visit of this function.
func SettingsDifferFromDefaults(s *AssetSettings) bool {
	d := DefaultSettings()
	return s.VolumetricLayers != d.VolumetricLayers ||
		s.VolumetricResolution != d.VolumetricResolution ||
		s.DomeHeightFactor != d.DomeHeightFactor ||
		s.BakeExposure != d.BakeExposure ||
		s.AmbientIntensity != d.AmbientIntensity ||
		s.HemisphereIntensity != d.HemisphereIntensity ||
		s.KeyLightIntensity != d.KeyLightIntensity ||
		s.BottomFillIntensity != d.BottomFillIntensity ||
		s.EnvMapIntensity != d.EnvMapIntensity ||
		s.AlphaTest != d.AlphaTest ||
		s.LightingPreset != d.LightingPreset ||
		s.SliceDistributionMode != d.SliceDistributionMode ||
		s.GroundAlign != d.GroundAlign ||
		s.ColorCalibrationMode != d.ColorCalibrationMode ||
		s.ReferenceImagePath != d.ReferenceImagePath
}

// SettingsFilePath returns the on-disk path for the given asset id.
func SettingsFilePath(id, dir string) string {
	return filepath.Join(dir, id+".json")
}

// SettingsExist reports whether a settings file is present on disk for the
// given asset id.
func SettingsExist(id, dir string) bool {
	_, err := os.Stat(SettingsFilePath(id, dir))
	return err == nil
}

// LoadSettings reads the asset's settings from disk. If the file does not
// exist, it returns DefaultSettings() and a nil error — callers should treat
// "no file" as "use defaults". Validation is intentionally NOT performed
// here; callers that care should call Validate() themselves.
func LoadSettings(id, dir string) (*AssetSettings, error) {
	path := SettingsFilePath(id, dir)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return DefaultSettings(), nil
		}
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	var s AssetSettings
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("decode settings %s: %w", path, err)
	}
	// Forward-compat normalization for files written before T-005-01.
	// The two new fields (slice_distribution_mode, ground_align) are
	// absent from older on-disk files; their Go zero values are unsafe
	// (empty string fails enum validation; ground_align=false is the
	// wrong migration default). See docs/knowledge/settings-schema.md
	// "Forward-compat normalization".
	if s.SliceDistributionMode == "" {
		s.SliceDistributionMode = "visual-density"
	}
	// To distinguish "explicit false" from "absent", re-decode just
	// ground_align as a *bool. nil → migration default of true.
	var probe struct {
		GroundAlign *bool `json:"ground_align"`
	}
	if err := json.Unmarshal(data, &probe); err == nil && probe.GroundAlign == nil {
		s.GroundAlign = true
	}
	// Forward-compat normalization for files written before T-005-03.
	// Empty string would otherwise fail enum validation; default to
	// the documented "none" (neutral lighting).
	if s.ColorCalibrationMode == "" {
		s.ColorCalibrationMode = "none"
	}
	return &s, nil
}

// SaveSettings writes the asset's settings to disk atomically. The directory
// is created if missing. Marshaling uses 2-space indentation for human
// readability; declaration order in AssetSettings determines field order.
func SaveSettings(id, dir string, s *AssetSettings) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create settings dir: %w", err)
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	data = append(data, '\n')
	return writeAtomic(SettingsFilePath(id, dir), data)
}

// writeAtomic writes data to path via a temp file in the same directory
// followed by os.Rename. The temp file is removed on any error.
func writeAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	base := filepath.Base(path)
	tmp, err := os.CreateTemp(dir, base+".*.tmp")
	if err != nil {
		return fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpPath)
		return fmt.Errorf("write temp file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("close temp file: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename temp file: %w", err)
	}
	return nil
}
