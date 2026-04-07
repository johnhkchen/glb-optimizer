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

func TestDefaultSettings_NewFields(t *testing.T) {
	s := DefaultSettings()
	if s.SliceDistributionMode != "visual-density" {
		t.Errorf("SliceDistributionMode default = %q, want %q", s.SliceDistributionMode, "visual-density")
	}
	if s.GroundAlign != true {
		t.Errorf("GroundAlign default = %v, want true", s.GroundAlign)
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
