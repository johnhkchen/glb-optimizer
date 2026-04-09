package main

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestDefaultSettings_Valid(t *testing.T) {
	s := DefaultSettings()
	if err := s.Validate(); err != nil {
		t.Fatalf("DefaultSettings() failed validation: %v", err)
	}
	if s.SchemaVersion != SettingsSchemaVersion {
		t.Errorf("default SchemaVersion = %d, want %d", s.SchemaVersion, SettingsSchemaVersion)
	}
}

func TestSaveLoad_Roundtrip(t *testing.T) {
	dir := t.TempDir()
	id := "abc123"

	original := DefaultSettings()
	original.BakeExposure = 1.25
	original.VolumetricLayers = 6

	if err := SaveSettings(id, dir, original); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}

	loaded, err := LoadSettings(id, dir)
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if !reflect.DeepEqual(original, loaded) {
		t.Errorf("round-trip mismatch:\n  got:  %+v\n  want: %+v", loaded, original)
	}
}

func TestLoadMissing_ReturnsDefaults(t *testing.T) {
	dir := t.TempDir()
	loaded, err := LoadSettings("does-not-exist", dir)
	if err != nil {
		t.Fatalf("LoadSettings on missing file returned error: %v", err)
	}
	if !reflect.DeepEqual(loaded, DefaultSettings()) {
		t.Errorf("missing-file load did not return defaults:\n  got: %+v", loaded)
	}
}

func TestValidate_RejectsBadVersion(t *testing.T) {
	s := DefaultSettings()
	s.SchemaVersion = 2
	if err := s.Validate(); err == nil {
		t.Error("expected error for schema_version=2, got nil")
	}
	s.SchemaVersion = 0
	if err := s.Validate(); err == nil {
		t.Error("expected error for schema_version=0, got nil")
	}
}

func TestValidate_RejectsOutOfRange(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*AssetSettings)
	}{
		{"negative bake_exposure", func(s *AssetSettings) { s.BakeExposure = -1 }},
		{"huge key_light", func(s *AssetSettings) { s.KeyLightIntensity = 99 }},
		{"alpha_test > 1", func(s *AssetSettings) { s.AlphaTest = 1.5 }},
		{"layers = 0", func(s *AssetSettings) { s.VolumetricLayers = 0 }},
		{"bad resolution", func(s *AssetSettings) { s.VolumetricResolution = 333 }},
		{"unknown preset", func(s *AssetSettings) { s.LightingPreset = "studio" }},
		{"empty slice mode", func(s *AssetSettings) { s.SliceDistributionMode = "" }},
		{"unknown slice mode", func(s *AssetSettings) { s.SliceDistributionMode = "spirals" }},
		{"empty slice axis", func(s *AssetSettings) { s.SliceAxis = "" }},
		{"unknown slice axis", func(s *AssetSettings) { s.SliceAxis = "diagonal" }},
		{"tilted_fade_low_start < 0", func(s *AssetSettings) { s.TiltedFadeLowStart = -0.1 }},
		{"tilted_fade_low_end > 1", func(s *AssetSettings) { s.TiltedFadeLowEnd = 1.5 }},
		{"tilted_fade_high_start > 1", func(s *AssetSettings) { s.TiltedFadeHighStart = 2 }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := DefaultSettings()
			c.mut(s)
			if err := s.Validate(); err == nil {
				t.Errorf("expected validation error, got nil")
			}
		})
	}
}

// TestValidate_AcceptsAllPresets ensures every id named in
// validLightingPresets passes Validate(). T-007-01 grew this enum from
// {default} to a 6-value set; this guards against accidentally dropping
// one of the new ids.
func TestValidate_AcceptsAllPresets(t *testing.T) {
	ids := []string{
		"default",
		"midday-sun",
		"overcast",
		"golden-hour",
		"dusk",
		"indoor",
		"from-reference-image",
	}
	for _, id := range ids {
		t.Run(id, func(t *testing.T) {
			s := DefaultSettings()
			s.LightingPreset = id
			if err := s.Validate(); err != nil {
				t.Errorf("preset %q rejected: %v", id, err)
			}
		})
	}
}

func TestDefaultSettings_NewFields(t *testing.T) {
	s := DefaultSettings()
	if s.SliceDistributionMode != "visual-density" {
		t.Errorf("SliceDistributionMode default = %q, want %q", s.SliceDistributionMode, "visual-density")
	}
	if s.GroundAlign != true {
		t.Errorf("GroundAlign default = %v, want true", s.GroundAlign)
	}
}

// T-009-03: assert the three tunable fade-band thresholds default
// to the values mandated by the ticket. The JS makeDefaults() must
// stay in sync with these (kept by hand, see settings.go comment).
func TestDefaultSettings_TiltedFadeFields(t *testing.T) {
	s := DefaultSettings()
	if s.TiltedFadeLowStart != 0.30 {
		t.Errorf("TiltedFadeLowStart default = %v, want 0.30", s.TiltedFadeLowStart)
	}
	if s.TiltedFadeLowEnd != 0.55 {
		t.Errorf("TiltedFadeLowEnd default = %v, want 0.55", s.TiltedFadeLowEnd)
	}
	if s.TiltedFadeHighStart != 0.75 {
		t.Errorf("TiltedFadeHighStart default = %v, want 0.75", s.TiltedFadeHighStart)
	}
}

// TestLoadSettings_MigratesOldFile asserts that a settings JSON written
// before T-005-01 (no slice_distribution_mode, no ground_align keys)
// loads with the migration defaults filled in and validates cleanly.
func TestLoadSettings_MigratesOldFile(t *testing.T) {
	dir := t.TempDir()
	id := "legacy"
	// Hand-rolled v1 document, intentionally missing the two new keys.
	legacy := `{
  "schema_version": 1,
  "volumetric_layers": 4,
  "volumetric_resolution": 512,
  "dome_height_factor": 0.5,
  "bake_exposure": 1.0,
  "ambient_intensity": 0.5,
  "hemisphere_intensity": 1.0,
  "key_light_intensity": 1.4,
  "bottom_fill_intensity": 0.4,
  "env_map_intensity": 1.2,
  "alpha_test": 0.10,
  "lighting_preset": "default"
}
`
	path := filepath.Join(dir, id+".json")
	if err := os.WriteFile(path, []byte(legacy), 0644); err != nil {
		t.Fatalf("write legacy file: %v", err)
	}
	loaded, err := LoadSettings(id, dir)
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if loaded.SliceDistributionMode != "visual-density" {
		t.Errorf("migration: SliceDistributionMode = %q, want %q", loaded.SliceDistributionMode, "visual-density")
	}
	if loaded.GroundAlign != true {
		t.Errorf("migration: GroundAlign = %v, want true", loaded.GroundAlign)
	}
	if err := loaded.Validate(); err != nil {
		t.Errorf("migrated settings failed validation: %v", err)
	}
}

// TestLoadSettings_ExplicitFalseGroundAlign asserts that a file which
// *explicitly* sets ground_align to false is preserved (the migration
// only fills in the default when the key is absent).
func TestLoadSettings_ExplicitFalseGroundAlign(t *testing.T) {
	dir := t.TempDir()
	id := "explicit"
	doc := `{
  "schema_version": 1,
  "volumetric_layers": 4,
  "volumetric_resolution": 512,
  "dome_height_factor": 0.5,
  "bake_exposure": 1.0,
  "ambient_intensity": 0.5,
  "hemisphere_intensity": 1.0,
  "key_light_intensity": 1.4,
  "bottom_fill_intensity": 0.4,
  "env_map_intensity": 1.2,
  "alpha_test": 0.10,
  "lighting_preset": "default",
  "slice_distribution_mode": "equal-height",
  "ground_align": false
}
`
	path := filepath.Join(dir, id+".json")
	if err := os.WriteFile(path, []byte(doc), 0644); err != nil {
		t.Fatalf("write doc: %v", err)
	}
	loaded, err := LoadSettings(id, dir)
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if loaded.GroundAlign != false {
		t.Errorf("explicit-false GroundAlign was overwritten: %v", loaded.GroundAlign)
	}
	if loaded.SliceDistributionMode != "equal-height" {
		t.Errorf("SliceDistributionMode = %q, want %q", loaded.SliceDistributionMode, "equal-height")
	}
}

// TestSettingsDifferFromDefaults covers the comparison helper used to
// populate FileRecord.SettingsDirty (T-005-02).
func TestSettingsDifferFromDefaults(t *testing.T) {
	t.Run("defaults_match", func(t *testing.T) {
		if SettingsDifferFromDefaults(DefaultSettings()) {
			t.Error("DefaultSettings() should not differ from defaults")
		}
	})

	t.Run("single_field_mutated", func(t *testing.T) {
		cases := []struct {
			name string
			mut  func(*AssetSettings)
		}{
			{"key_light_intensity", func(s *AssetSettings) { s.KeyLightIntensity = 2.0 }},
			{"ground_align", func(s *AssetSettings) { s.GroundAlign = false }},
			{"slice_distribution_mode", func(s *AssetSettings) { s.SliceDistributionMode = "equal-height" }},
			{"lighting_preset", func(s *AssetSettings) { s.LightingPreset = "from-reference-image" }},
			{"reference_image_path", func(s *AssetSettings) { s.ReferenceImagePath = "outputs/x_reference.png" }},
		}
		for _, c := range cases {
			t.Run(c.name, func(t *testing.T) {
				s := DefaultSettings()
				c.mut(s)
				if !SettingsDifferFromDefaults(s) {
					t.Errorf("expected %s mutation to be flagged dirty", c.name)
				}
			})
		}
	})

	t.Run("schema_version_change_ignored", func(t *testing.T) {
		s := DefaultSettings()
		s.SchemaVersion = 99
		if SettingsDifferFromDefaults(s) {
			t.Error("schema_version-only divergence should not flag dirty")
		}
	})
}

// TestLoadSettings_MigratesColorCalibrationMode asserts that a
// pre-T-007-03 document carrying the legacy
// `color_calibration_mode: "from-reference-image"` key (with the
// default lighting preset) is migrated to
// `lighting_preset: "from-reference-image"` on load.
func TestLoadSettings_MigratesColorCalibrationMode(t *testing.T) {
	dir := t.TempDir()
	id := "legacy-cal"
	doc := `{
  "schema_version": 1,
  "volumetric_layers": 4,
  "volumetric_resolution": 512,
  "dome_height_factor": 0.5,
  "bake_exposure": 1.0,
  "ambient_intensity": 0.5,
  "hemisphere_intensity": 1.0,
  "key_light_intensity": 1.4,
  "bottom_fill_intensity": 0.4,
  "env_map_intensity": 1.2,
  "alpha_test": 0.10,
  "lighting_preset": "default",
  "slice_distribution_mode": "visual-density",
  "ground_align": true,
  "color_calibration_mode": "from-reference-image"
}
`
	path := filepath.Join(dir, id+".json")
	if err := os.WriteFile(path, []byte(doc), 0644); err != nil {
		t.Fatalf("write doc: %v", err)
	}
	loaded, err := LoadSettings(id, dir)
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if loaded.LightingPreset != "from-reference-image" {
		t.Errorf("LightingPreset = %q, want %q", loaded.LightingPreset, "from-reference-image")
	}
	if err := loaded.Validate(); err != nil {
		t.Errorf("migrated doc failed validation: %v", err)
	}
}

// TestLoadSettings_ExplicitPresetWinsOverLegacyMode asserts that an
// explicit non-default `lighting_preset` is preserved when a legacy
// `color_calibration_mode: from-reference-image` key is also present —
// the user has already chosen a preset and that choice wins.
func TestLoadSettings_ExplicitPresetWinsOverLegacyMode(t *testing.T) {
	dir := t.TempDir()
	id := "explicit-preset"
	doc := `{
  "schema_version": 1,
  "volumetric_layers": 4,
  "volumetric_resolution": 512,
  "dome_height_factor": 0.5,
  "bake_exposure": 1.0,
  "ambient_intensity": 0.5,
  "hemisphere_intensity": 1.0,
  "key_light_intensity": 1.4,
  "bottom_fill_intensity": 0.4,
  "env_map_intensity": 1.2,
  "alpha_test": 0.10,
  "lighting_preset": "midday-sun",
  "slice_distribution_mode": "visual-density",
  "ground_align": true,
  "color_calibration_mode": "from-reference-image"
}
`
	path := filepath.Join(dir, id+".json")
	if err := os.WriteFile(path, []byte(doc), 0644); err != nil {
		t.Fatalf("write doc: %v", err)
	}
	loaded, err := LoadSettings(id, dir)
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if loaded.LightingPreset != "midday-sun" {
		t.Errorf("LightingPreset = %q, want %q (explicit must win)", loaded.LightingPreset, "midday-sun")
	}
	if err := loaded.Validate(); err != nil {
		t.Errorf("doc failed validation: %v", err)
	}
}

// T-004-02: shape classifier fields.

func TestDefaultSettings_ShapeFields(t *testing.T) {
	s := DefaultSettings()
	if s.ShapeCategory != "unknown" {
		t.Errorf("ShapeCategory default = %q, want %q", s.ShapeCategory, "unknown")
	}
	if s.ShapeConfidence != 0 {
		t.Errorf("ShapeConfidence default = %v, want 0", s.ShapeConfidence)
	}
}

func TestValidate_RejectsBadShapeCategory(t *testing.T) {
	s := DefaultSettings()
	s.ShapeCategory = "spirals"
	if err := s.Validate(); err == nil {
		t.Error("expected validation error for unknown shape_category")
	}
}

func TestValidate_RejectsConfidenceOutOfRange(t *testing.T) {
	for _, v := range []float64{-0.1, 1.5} {
		s := DefaultSettings()
		s.ShapeConfidence = v
		if err := s.Validate(); err == nil {
			t.Errorf("expected validation error for shape_confidence=%v", v)
		}
	}
}

func TestValidate_AcceptsAllShapeCategories(t *testing.T) {
	for _, c := range []string{"round-bush", "directional", "tall-narrow", "planar", "hard-surface", "unknown"} {
		s := DefaultSettings()
		s.ShapeCategory = c
		s.ShapeConfidence = 0.5
		if err := s.Validate(); err != nil {
			t.Errorf("category %q rejected: %v", c, err)
		}
	}
}

func TestSaveLoad_RoundtripShapeFields(t *testing.T) {
	dir := t.TempDir()
	id := "shape"
	original := DefaultSettings()
	original.ShapeCategory = "round-bush"
	original.ShapeConfidence = 0.83
	if err := SaveSettings(id, dir, original); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	loaded, err := LoadSettings(id, dir)
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if loaded.ShapeCategory != "round-bush" || loaded.ShapeConfidence != 0.83 {
		t.Errorf("round-trip mismatch: got (%q, %v)", loaded.ShapeCategory, loaded.ShapeConfidence)
	}
}

// TestLoadSettings_OldDocMissingShapeCategory asserts that a settings
// document written before T-004-02 (no shape_category key) loads with
// shape_category normalized to "unknown" and validates cleanly.
func TestLoadSettings_OldDocMissingShapeCategory(t *testing.T) {
	dir := t.TempDir()
	id := "preshape"
	doc := `{
  "schema_version": 1,
  "volumetric_layers": 4,
  "volumetric_resolution": 512,
  "dome_height_factor": 0.5,
  "bake_exposure": 1.0,
  "ambient_intensity": 0.5,
  "hemisphere_intensity": 1.0,
  "key_light_intensity": 1.4,
  "bottom_fill_intensity": 0.4,
  "env_map_intensity": 1.2,
  "alpha_test": 0.10,
  "lighting_preset": "default",
  "slice_distribution_mode": "visual-density",
  "ground_align": true
}
`
	path := filepath.Join(dir, id+".json")
	if err := os.WriteFile(path, []byte(doc), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	loaded, err := LoadSettings(id, dir)
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if loaded.ShapeCategory != "unknown" {
		t.Errorf("ShapeCategory = %q, want %q", loaded.ShapeCategory, "unknown")
	}
	if err := loaded.Validate(); err != nil {
		t.Errorf("validate: %v", err)
	}
}

func TestSettingsDifferFromDefaults_ShapeFields(t *testing.T) {
	s := DefaultSettings()
	s.ShapeCategory = "round-bush"
	if !SettingsDifferFromDefaults(s) {
		t.Error("ShapeCategory mutation should be flagged dirty")
	}
	s = DefaultSettings()
	s.ShapeConfidence = 0.5
	if !SettingsDifferFromDefaults(s) {
		t.Error("ShapeConfidence mutation should be flagged dirty")
	}
	s = DefaultSettings()
	s.SliceAxis = "auto-horizontal"
	if !SettingsDifferFromDefaults(s) {
		t.Error("SliceAxis mutation should be flagged dirty")
	}
}

// TestValidate_AcceptsAllSliceAxes asserts every member of
// validSliceAxes passes Validate(). Drift between the strategy
// router's SliceAxis* constants and validSliceAxes is the failure
// mode this test catches.
func TestValidate_AcceptsAllSliceAxes(t *testing.T) {
	for axis := range validSliceAxes {
		t.Run(axis, func(t *testing.T) {
			s := DefaultSettings()
			s.SliceAxis = axis
			if err := s.Validate(); err != nil {
				t.Errorf("axis %q rejected: %v", axis, err)
			}
		})
	}
}

// TestLoadSettings_OldDocMissingSliceAxis asserts that a settings
// document written before T-004-03 (no slice_axis key) loads with
// slice_axis normalized to "y" and validates cleanly.
func TestLoadSettings_OldDocMissingSliceAxis(t *testing.T) {
	dir := t.TempDir()
	id := "preaxis"
	doc := `{
  "schema_version": 1,
  "volumetric_layers": 4,
  "volumetric_resolution": 512,
  "dome_height_factor": 0.5,
  "bake_exposure": 1.0,
  "ambient_intensity": 0.5,
  "hemisphere_intensity": 1.0,
  "key_light_intensity": 1.4,
  "bottom_fill_intensity": 0.4,
  "env_map_intensity": 1.2,
  "alpha_test": 0.10,
  "lighting_preset": "default",
  "slice_distribution_mode": "visual-density",
  "ground_align": true,
  "shape_category": "unknown"
}
`
	path := filepath.Join(dir, id+".json")
	if err := os.WriteFile(path, []byte(doc), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	loaded, err := LoadSettings(id, dir)
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if loaded.SliceAxis != "y" {
		t.Errorf("SliceAxis = %q, want %q", loaded.SliceAxis, "y")
	}
	if err := loaded.Validate(); err != nil {
		t.Errorf("validate: %v", err)
	}
}

// T-006-02: scene preview persistence fields.

func TestDefaultSettings_SceneFields(t *testing.T) {
	s := DefaultSettings()
	if s.SceneTemplateId != "grid" {
		t.Errorf("SceneTemplateId default = %q, want %q", s.SceneTemplateId, "grid")
	}
	if s.SceneInstanceCount != 100 {
		t.Errorf("SceneInstanceCount default = %d, want 100", s.SceneInstanceCount)
	}
	if s.SceneGroundPlane != false {
		t.Errorf("SceneGroundPlane default = %v, want false", s.SceneGroundPlane)
	}
}

func TestValidate_AcceptsAllSceneTemplates(t *testing.T) {
	for id := range validSceneTemplates {
		t.Run(id, func(t *testing.T) {
			s := DefaultSettings()
			s.SceneTemplateId = id
			if err := s.Validate(); err != nil {
				t.Errorf("template %q rejected: %v", id, err)
			}
		})
	}
}

func TestValidate_RejectsBadSceneTemplate(t *testing.T) {
	s := DefaultSettings()
	s.SceneTemplateId = "spirals"
	if err := s.Validate(); err == nil {
		t.Error("expected validation error for unknown scene_template_id")
	}
}

func TestValidate_RejectsSceneCountOutOfRange(t *testing.T) {
	for _, n := range []int{0, -1, 501, 1000} {
		s := DefaultSettings()
		s.SceneInstanceCount = n
		if err := s.Validate(); err == nil {
			t.Errorf("expected validation error for scene_instance_count=%d", n)
		}
	}
}

func TestSettingsDifferFromDefaults_SceneFields(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*AssetSettings)
	}{
		{"scene_template_id", func(s *AssetSettings) { s.SceneTemplateId = "mixed-bed" }},
		{"scene_instance_count", func(s *AssetSettings) { s.SceneInstanceCount = 50 }},
		{"scene_ground_plane", func(s *AssetSettings) { s.SceneGroundPlane = true }},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			s := DefaultSettings()
			c.mut(s)
			if !SettingsDifferFromDefaults(s) {
				t.Errorf("expected %s mutation to be flagged dirty", c.name)
			}
		})
	}
}

// TestLoadSettings_OldDocMissingSceneFields asserts that a settings
// document written before T-006-02 (no scene_* keys) loads with the
// scene_* fields normalized to their defaults and validates cleanly.
func TestLoadSettings_OldDocMissingSceneFields(t *testing.T) {
	dir := t.TempDir()
	id := "prescene"
	doc := `{
  "schema_version": 1,
  "volumetric_layers": 4,
  "volumetric_resolution": 512,
  "dome_height_factor": 0.5,
  "bake_exposure": 1.0,
  "ambient_intensity": 0.5,
  "hemisphere_intensity": 1.0,
  "key_light_intensity": 1.4,
  "bottom_fill_intensity": 0.4,
  "env_map_intensity": 1.2,
  "alpha_test": 0.10,
  "lighting_preset": "default",
  "slice_distribution_mode": "visual-density",
  "ground_align": true,
  "shape_category": "unknown",
  "slice_axis": "y"
}
`
	path := filepath.Join(dir, id+".json")
	if err := os.WriteFile(path, []byte(doc), 0644); err != nil {
		t.Fatalf("write: %v", err)
	}
	loaded, err := LoadSettings(id, dir)
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if loaded.SceneTemplateId != "grid" {
		t.Errorf("SceneTemplateId = %q, want %q", loaded.SceneTemplateId, "grid")
	}
	if loaded.SceneInstanceCount != 100 {
		t.Errorf("SceneInstanceCount = %d, want 100", loaded.SceneInstanceCount)
	}
	if loaded.SceneGroundPlane != false {
		t.Errorf("SceneGroundPlane = %v, want false", loaded.SceneGroundPlane)
	}
	if err := loaded.Validate(); err != nil {
		t.Errorf("validate: %v", err)
	}
}

func TestSaveLoad_RoundtripSceneFields(t *testing.T) {
	dir := t.TempDir()
	id := "scene"
	original := DefaultSettings()
	original.SceneTemplateId = "mixed-bed"
	original.SceneInstanceCount = 50
	original.SceneGroundPlane = true
	if err := SaveSettings(id, dir, original); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	loaded, err := LoadSettings(id, dir)
	if err != nil {
		t.Fatalf("LoadSettings: %v", err)
	}
	if loaded.SceneTemplateId != "mixed-bed" ||
		loaded.SceneInstanceCount != 50 ||
		loaded.SceneGroundPlane != true {
		t.Errorf("round-trip mismatch: got (%q, %d, %v)",
			loaded.SceneTemplateId, loaded.SceneInstanceCount, loaded.SceneGroundPlane)
	}
}

func TestSettingsExist(t *testing.T) {
	dir := t.TempDir()
	id := "xyz"
	if SettingsExist(id, dir) {
		t.Error("SettingsExist should be false before save")
	}
	if err := SaveSettings(id, dir, DefaultSettings()); err != nil {
		t.Fatalf("SaveSettings: %v", err)
	}
	if !SettingsExist(id, dir) {
		t.Error("SettingsExist should be true after save")
	}
}
