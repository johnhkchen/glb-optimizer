package main

import (
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// AcceptedSchemaVersion is the on-disk schema version for AcceptedSettings.
// It is pinned to SettingsSchemaVersion because the embedded Settings field
// is the bulk of the on-disk shape; per-snapshot fields (comment,
// thumbnail_path, ...) can be added additively without bumping. See
// docs/knowledge/settings-schema.md for the migration policy.
const AcceptedSchemaVersion = SettingsSchemaVersion

const acceptedCommentMaxLen = 1024

// AcceptedSettings is a per-asset "this is shippable" snapshot of the
// current AssetSettings, captured when the user marks an asset as accepted.
// It is the highest-value training signal in the S-003 epic — see
// docs/active/tickets/T-003-04.md "Context".
//
// Field declaration order is also the on-disk JSON order.
type AcceptedSettings struct {
	SchemaVersion int            `json:"schema_version"`
	AssetID       string         `json:"asset_id"`
	AcceptedAt    string         `json:"accepted_at"`
	Comment       string         `json:"comment"`
	ThumbnailPath string         `json:"thumbnail_path"`
	Settings      *AssetSettings `json:"settings"`
}

// Validate checks an AcceptedSettings against the v1 schema.
func (a *AcceptedSettings) Validate() error {
	if a.SchemaVersion != AcceptedSchemaVersion {
		return fmt.Errorf("unsupported schema_version: %d (expected %d)", a.SchemaVersion, AcceptedSchemaVersion)
	}
	if a.AssetID == "" {
		return fmt.Errorf("asset_id must be set")
	}
	if len(a.Comment) > acceptedCommentMaxLen {
		return fmt.Errorf("comment exceeds %d chars", acceptedCommentMaxLen)
	}
	if a.Settings == nil {
		return fmt.Errorf("settings must not be null")
	}
	if err := a.Settings.Validate(); err != nil {
		return fmt.Errorf("settings: %w", err)
	}
	return nil
}

// AcceptedFilePath returns the on-disk path of an asset's accepted snapshot.
func AcceptedFilePath(id, dir string) string {
	return filepath.Join(dir, id+".json")
}

// AcceptedThumbPath returns the on-disk path of an asset's accepted
// thumbnail. The thumbs directory is typically `<acceptedDir>/thumbs`.
func AcceptedThumbPath(id, thumbsDir string) string {
	return filepath.Join(thumbsDir, id+".jpg")
}

// AcceptedExists reports whether an accepted snapshot file is present on
// disk for the given asset id.
func AcceptedExists(id, dir string) bool {
	_, err := os.Stat(AcceptedFilePath(id, dir))
	return err == nil
}

// LoadAccepted reads an asset's accepted snapshot from disk. Missing files
// surface as fs.ErrNotExist (use errors.Is to detect). The loaded snapshot
// is NOT validated; callers that care should call Validate() themselves.
func LoadAccepted(id, dir string) (*AcceptedSettings, error) {
	path := AcceptedFilePath(id, dir)
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
	var a AcceptedSettings
	if err := json.Unmarshal(data, &a); err != nil {
		return nil, fmt.Errorf("decode accepted %s: %w", path, err)
	}
	return &a, nil
}

// SaveAccepted validates, stamps AcceptedAt if empty, and writes the
// accepted snapshot to disk atomically. Re-saving overwrites the existing
// snapshot — v1 keeps no accept history (see T-003-04 ticket).
func SaveAccepted(a *AcceptedSettings, dir string) error {
	if a == nil {
		return fmt.Errorf("accepted must not be nil")
	}
	if a.SchemaVersion == 0 {
		a.SchemaVersion = AcceptedSchemaVersion
	}
	if a.AcceptedAt == "" {
		a.AcceptedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	if err := a.Validate(); err != nil {
		return err
	}
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create accepted dir: %w", err)
	}
	data, err := json.MarshalIndent(a, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal accepted: %w", err)
	}
	data = append(data, '\n')
	return writeAtomic(AcceptedFilePath(a.AssetID, dir), data)
}

// WriteThumbnail writes the JPEG bytes for an asset's accepted thumbnail
// atomically. The thumbs directory is created if missing. Empty bytes are
// not allowed — callers should skip the call entirely if no thumbnail was
// captured.
func WriteThumbnail(id, thumbsDir string, jpegBytes []byte) error {
	if len(jpegBytes) == 0 {
		return fmt.Errorf("thumbnail bytes must not be empty")
	}
	if err := os.MkdirAll(thumbsDir, 0755); err != nil {
		return fmt.Errorf("create thumbs dir: %w", err)
	}
	return writeAtomic(AcceptedThumbPath(id, thumbsDir), jpegBytes)
}
