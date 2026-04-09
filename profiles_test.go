package main

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// fixtureProfile returns a valid Profile suitable for use as a test
// starting point. Mutate fields as needed in individual tests.
func fixtureProfile(name string) *Profile {
	return &Profile{
		SchemaVersion: ProfilesSchemaVersion,
		Name:          name,
		Comment:       "round bushes look great with the dome up",
		CreatedAt:     "2026-04-07T12:00:00Z",
		SourceAssetID: "asset-abc",
		Settings:      DefaultSettings(),
	}
}

func TestDefaultProfile_Valid(t *testing.T) {
	p := fixtureProfile("round-bushes-warm")
	if err := p.Validate(); err != nil {
		t.Fatalf("fixture profile failed validation: %v", err)
	}
}

func TestSaveLoad_ProfileRoundtrip(t *testing.T) {
	dir := t.TempDir()
	p := fixtureProfile("round-bushes-warm")
	p.Settings.BakeExposure = 1.25
	p.Settings.VolumetricLayers = 6

	if err := SaveProfile(p, dir); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}

	loaded, err := LoadProfile("round-bushes-warm", dir)
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}
	if !reflect.DeepEqual(p, loaded) {
		t.Errorf("round-trip mismatch:\n  got:  %+v\n  want: %+v", loaded, p)
	}
}

func TestLoadProfile_MissingReturnsNotExist(t *testing.T) {
	dir := t.TempDir()
	_, err := LoadProfile("does-not-exist", dir)
	if !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("expected fs.ErrNotExist, got %v", err)
	}
}

func TestValidate_RejectsBadName(t *testing.T) {
	cases := []struct {
		name  string
		input string
	}{
		{"empty", ""},
		{"uppercase", "RoundBushes"},
		{"leading dash", "-foo"},
		{"trailing dash", "foo-"},
		{"double dash", "foo--bar"},
		{"underscore", "foo_bar"},
		{"slash", "foo/bar"},
		{"dotdot", ".."},
		{"leading dot", ".foo"},
		{"too long", "a" + string(make([]byte, profileNameMaxLen))},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := fixtureProfile("ok-name")
			p.Name = c.input
			if err := p.Validate(); err == nil {
				t.Errorf("expected validation error for %q, got nil", c.input)
			}
		})
	}
}

func TestValidate_RejectsBadSettings(t *testing.T) {
	p := fixtureProfile("round-bushes-warm")
	p.Settings.VolumetricResolution = 333 // not in the allow-list
	if err := p.Validate(); err == nil {
		t.Error("expected validation error for bad settings, got nil")
	}
}

func TestValidate_RejectsNilSettings(t *testing.T) {
	p := fixtureProfile("round-bushes-warm")
	p.Settings = nil
	if err := p.Validate(); err == nil {
		t.Error("expected validation error for nil settings, got nil")
	}
}

func TestSaveProfile_StampsCreatedAtIfEmpty(t *testing.T) {
	dir := t.TempDir()
	p := fixtureProfile("round-bushes-warm")
	p.CreatedAt = ""
	if err := SaveProfile(p, dir); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	if p.CreatedAt == "" {
		t.Fatal("CreatedAt was not stamped")
	}
	if _, err := time.Parse(time.RFC3339Nano, p.CreatedAt); err != nil {
		t.Errorf("CreatedAt %q is not RFC3339Nano: %v", p.CreatedAt, err)
	}
}

func TestListProfiles_SortedByName(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"zebra", "apple", "mango"} {
		p := fixtureProfile(name)
		if err := SaveProfile(p, dir); err != nil {
			t.Fatalf("SaveProfile %q: %v", name, err)
		}
	}
	list, err := ListProfiles(dir)
	if err != nil {
		t.Fatalf("ListProfiles: %v", err)
	}
	if len(list) != 3 {
		t.Fatalf("expected 3 profiles, got %d", len(list))
	}
	wantOrder := []string{"apple", "mango", "zebra"}
	for i, m := range list {
		if m.Name != wantOrder[i] {
			t.Errorf("position %d: got %q, want %q", i, m.Name, wantOrder[i])
		}
		if m.Comment == "" {
			t.Errorf("position %d: comment is empty", i)
		}
	}
}

func TestListProfiles_SkipsCorrupt(t *testing.T) {
	dir := t.TempDir()
	good := fixtureProfile("good")
	if err := SaveProfile(good, dir); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	// Hand-write a corrupt JSON file in the same dir.
	if err := os.WriteFile(filepath.Join(dir, "corrupt.json"), []byte("{not json"), 0644); err != nil {
		t.Fatalf("write corrupt: %v", err)
	}
	list, err := ListProfiles(dir)
	if err != nil {
		t.Fatalf("ListProfiles: %v", err)
	}
	if len(list) != 1 || list[0].Name != "good" {
		t.Errorf("expected only the good profile, got %+v", list)
	}
}

func TestListProfiles_MissingDirReturnsEmpty(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "nonexistent")
	list, err := ListProfiles(dir)
	if err != nil {
		t.Fatalf("ListProfiles on missing dir: %v", err)
	}
	if len(list) != 0 {
		t.Errorf("expected empty list, got %+v", list)
	}
}

func TestDeleteProfile_RemovesFile(t *testing.T) {
	dir := t.TempDir()
	p := fixtureProfile("doomed")
	if err := SaveProfile(p, dir); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	if err := DeleteProfile("doomed", dir); err != nil {
		t.Fatalf("DeleteProfile: %v", err)
	}
	if _, err := LoadProfile("doomed", dir); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("expected fs.ErrNotExist after delete, got %v", err)
	}
}

func TestDeleteProfile_MissingReturnsNotExist(t *testing.T) {
	dir := t.TempDir()
	if err := DeleteProfile("ghost", dir); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("expected fs.ErrNotExist, got %v", err)
	}
}

func TestSaveProfile_Overwrite(t *testing.T) {
	dir := t.TempDir()
	p := fixtureProfile("round-bushes-warm")
	p.Comment = "first"
	if err := SaveProfile(p, dir); err != nil {
		t.Fatalf("SaveProfile #1: %v", err)
	}
	p2 := fixtureProfile("round-bushes-warm")
	p2.Comment = "second"
	if err := SaveProfile(p2, dir); err != nil {
		t.Fatalf("SaveProfile #2: %v", err)
	}
	loaded, err := LoadProfile("round-bushes-warm", dir)
	if err != nil {
		t.Fatalf("LoadProfile: %v", err)
	}
	if loaded.Comment != "second" {
		t.Errorf("overwrite did not take effect: got comment %q", loaded.Comment)
	}
}

func TestListProfiles_MetadataExcludesSettings(t *testing.T) {
	// Compile-time guarantee: ProfileMetadata has no Settings field.
	// Encode an instance and verify the JSON has no "settings" key.
	m := ProfileMetadata{Name: "x", Comment: "y", CreatedAt: "z", SourceAssetID: "a"}
	data, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if _, ok := raw["settings"]; ok {
		t.Errorf("ProfileMetadata leaked a settings field: %s", data)
	}
}
