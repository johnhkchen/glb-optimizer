package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// ProfilesSchemaVersion is the on-disk schema version for Profile. It is
// pinned to the AssetSettings schema version because the embedded
// Settings field is the bulk of the on-disk shape; profile-level fields
// (name, comment, etc.) can be added additively without bumping. See
// docs/knowledge/settings-schema.md for the migration policy.
const ProfilesSchemaVersion = SettingsSchemaVersion

// profileNameRe enforces kebab-case: lowercase alnum segments joined by
// single dashes, no leading/trailing/double dashes. The same regex
// doubles as a filesystem-safety check: by construction it disallows
// "..", "/", leading dots, and any character that needs shell quoting.
var profileNameRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

const (
	profileNameMaxLen    = 64
	profileCommentMaxLen = 1024
)

// Profile is a named, commented snapshot of AssetSettings the user can
// reuse across assets. Field declaration order is also the on-disk JSON
// order.
type Profile struct {
	SchemaVersion int            `json:"schema_version"`
	Name          string         `json:"name"`
	Comment       string         `json:"comment"`
	CreatedAt     string         `json:"created_at"`
	SourceAssetID string         `json:"source_asset_id"`
	Settings      *AssetSettings `json:"settings"`
}

// ProfileMetadata is the stripped projection used by ListProfiles. It
// excludes the Settings block to keep list responses small — listing 100
// profiles should not require unmarshaling 100 full settings structs.
type ProfileMetadata struct {
	Name          string `json:"name"`
	Comment       string `json:"comment"`
	CreatedAt     string `json:"created_at"`
	SourceAssetID string `json:"source_asset_id"`
}

// ValidateProfileName is the single source of truth for the profile name
// rule. It is called by Validate(), LoadProfile, DeleteProfile, and the
// HTTP layer.
func ValidateProfileName(name string) error {
	if name == "" {
		return fmt.Errorf("profile name must not be empty")
	}
	if len(name) > profileNameMaxLen {
		return fmt.Errorf("profile name %q exceeds %d chars", name, profileNameMaxLen)
	}
	if !profileNameRe.MatchString(name) {
		return fmt.Errorf("profile name %q must be kebab-case (a-z0-9 with single dashes), 1-%d chars", name, profileNameMaxLen)
	}
	return nil
}

// Validate checks the Profile against the v1 schema. It returns the
// first failing field as an error.
func (p *Profile) Validate() error {
	if err := ValidateProfileName(p.Name); err != nil {
		return err
	}
	if p.SchemaVersion != ProfilesSchemaVersion {
		return fmt.Errorf("unsupported schema_version: %d (expected %d)", p.SchemaVersion, ProfilesSchemaVersion)
	}
	if len(p.Comment) > profileCommentMaxLen {
		return fmt.Errorf("comment exceeds %d chars", profileCommentMaxLen)
	}
	if p.Settings == nil {
		return fmt.Errorf("settings must not be null")
	}
	if err := p.Settings.Validate(); err != nil {
		return fmt.Errorf("settings: %w", err)
	}
	return nil
}

// ProfilesFilePath returns the on-disk path for a profile. It does NOT
// validate the name — every public entry point validates first, and
// callers (handlers, helpers) are responsible for that.
func ProfilesFilePath(name, dir string) string {
	return filepath.Join(dir, name+".json")
}

// LoadProfile reads a profile from disk by name. Missing files surface
// as errors that match fs.ErrNotExist (use errors.Is to detect). The
// loaded profile is NOT validated; callers that care should call
// Validate() themselves.
func LoadProfile(name, dir string) (*Profile, error) {
	if err := ValidateProfileName(name); err != nil {
		return nil, err
	}
	path := ProfilesFilePath(name, dir)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fs.ErrNotExist
		}
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return nil, err
	}
	var p Profile
	if err := json.Unmarshal(data, &p); err != nil {
		return nil, fmt.Errorf("decode profile %s: %w", path, err)
	}
	return &p, nil
}

// SaveProfile validates, stamps CreatedAt if empty, and writes the
// profile to disk atomically. Overwriting an existing profile is
// allowed (v1 has no versioning).
func SaveProfile(p *Profile, dir string) error {
	if p == nil {
		return fmt.Errorf("profile must not be nil")
	}
	if p.SchemaVersion == 0 {
		p.SchemaVersion = ProfilesSchemaVersion
	}
	if p.CreatedAt == "" {
		p.CreatedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if err := p.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create profiles dir: %w", err)
	}
	data, err := json.MarshalIndent(p, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal profile: %w", err)
	}
	data = append(data, '\n')
	return writeAtomic(ProfilesFilePath(p.Name, dir), data)
}

// profileMetadataOnly is the decode target used by ListProfiles. The
// JSON tags match the leading fields of Profile so any *.json file
// written by SaveProfile decodes cleanly into it; the Settings field is
// ignored at decode time.
type profileMetadataOnly struct {
	Name          string `json:"name"`
	Comment       string `json:"comment"`
	CreatedAt     string `json:"created_at"`
	SourceAssetID string `json:"source_asset_id"`
}

// ListProfiles reads the profiles directory and returns metadata for
// every valid *.json file, sorted by name ascending. Files that fail
// to decode are skipped (and a warning is written to stderr) so a
// single corrupt profile cannot break the list.
//
// A missing dir returns an empty list and nil error — main.go MkdirAll's
// the dir at startup, but tolerating its absence keeps unit tests and
// fresh installs happy.
func ListProfiles(dir string) ([]ProfileMetadata, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return []ProfileMetadata{}, nil
		}
		return nil, fmt.Errorf("read profiles dir: %w", err)
	}
	out := make([]ProfileMetadata, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		data, err := os.ReadFile(path)
		if err != nil {
			fmt.Fprintf(os.Stderr, "ListProfiles: skip %s: %v\n", path, err)
			continue
		}
		var m profileMetadataOnly
		if err := json.Unmarshal(data, &m); err != nil {
			fmt.Fprintf(os.Stderr, "ListProfiles: skip %s: %v\n", path, err)
			continue
		}
		if m.Name == "" {
			// Fall back to the filename stem when the file lacks a
			// name field (shouldn't happen for files SaveProfile
			// wrote, but be defensive).
			m.Name = strings.TrimSuffix(e.Name(), ".json")
		}
		out = append(out, ProfileMetadata{
			Name:          m.Name,
			Comment:       m.Comment,
			CreatedAt:     m.CreatedAt,
			SourceAssetID: m.SourceAssetID,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// DeleteProfile removes the profile file by name. Missing files surface
// as fs.ErrNotExist (use errors.Is to detect).
func DeleteProfile(name, dir string) error {
	if err := ValidateProfileName(name); err != nil {
		return err
	}
	path := ProfilesFilePath(name, dir)
	if err := os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return fs.ErrNotExist
		}
		return err
	}
	return nil
}

