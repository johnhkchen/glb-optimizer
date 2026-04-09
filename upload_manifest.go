package main

// upload_manifest.go is the T-012-04 persistent upload index.
//
// The HTTP upload handler stores files under a content-addressed name
// (`{hash}.glb`) and the in-memory FileStore is the only place that
// remembers the human-readable upload filename. After a server
// restart, scanExistingFiles rebuilds the store from disk and the
// original name is gone — the root cause of T-012-01's resolver
// friction.
//
// The fix is an append-only JSONL log at <workDir>/uploads.jsonl. One
// line per upload, written best-effort: append failure logs a warning
// but never fails the upload itself. Append-only means a partial
// crash truncates at most the final line, which the scanner skips.
//
// Reads go through LookupUploadFilename, which keeps a mtime-keyed
// cache so a busy resolver does not re-parse the file on every call.

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sync"
	"time"
)

// UploadManifestEntry is one record in uploads.jsonl. The Hash field
// is the upload's content-addressed id (the same string used as the
// `{hash}.glb` filename in originalsDir). OriginalFilename is the
// upload-time multipart filename — the value the resolver wants to
// recover after restart.
type UploadManifestEntry struct {
	Hash             string    `json:"hash"`
	OriginalFilename string    `json:"original_filename"`
	UploadedAt       time.Time `json:"uploaded_at"`
	Size             int64     `json:"size"`
}

// ErrManifestNotFound is returned by LookupUploadFilename when the
// requested hash is not present in the manifest, OR when the manifest
// file does not exist at all. Distinguishing the two is left to the
// caller via os.Stat if needed; in the only consumer (the resolver)
// both cases get the same fall-through treatment.
var ErrManifestNotFound = errors.New("upload manifest: hash not found")

// AppendUploadRecord writes one entry to path. The file is created
// with mode 0644 if absent. f.Sync is called before close so the
// record is durable across a process or kernel crash — the manifest's
// entire reason to exist is to survive restarts.
func AppendUploadRecord(path string, entry UploadManifestEntry) error {
	if path == "" {
		return errors.New("upload manifest: empty path")
	}
	line, err := json.Marshal(entry)
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	line = append(line, '\n')

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open: %w", err)
	}
	if _, err := f.Write(line); err != nil {
		f.Close()
		return fmt.Errorf("write: %w", err)
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return fmt.Errorf("sync: %w", err)
	}
	return f.Close()
}

// LookupUploadFilename returns the most recent original_filename
// recorded for hash. The first call (or any call after the underlying
// file's mtime advances) re-scans the manifest from disk; subsequent
// calls serve from a process-wide cache. Both "file does not exist"
// and "hash not present" produce ErrManifestNotFound — the resolver
// treats them identically.
func LookupUploadFilename(path, hash string) (string, error) {
	idx, err := loadManifestCached(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrManifestNotFound
		}
		return "", err
	}
	name, ok := idx[hash]
	if !ok {
		return "", ErrManifestNotFound
	}
	return name, nil
}

// manifestCache is a single-slot, process-wide cache. The single-slot
// shape is fine because the server only ever reads one manifest path
// (the one main.go derives from workDir). If a test or a future
// caller switches paths, the cache invalidates on path mismatch.
var manifestCache struct {
	sync.Mutex
	path  string
	mtime time.Time
	index map[string]string
}

// loadManifestCached returns the index for path, re-reading the
// underlying file only when the mtime has changed (or the cached path
// differs). Returned map must NOT be mutated by the caller.
func loadManifestCached(path string) (map[string]string, error) {
	info, err := os.Stat(path)
	if err != nil {
		// Reset cache so a stale index doesn't outlive a deleted file.
		manifestCache.Lock()
		if manifestCache.path == path {
			manifestCache.path = ""
			manifestCache.index = nil
			manifestCache.mtime = time.Time{}
		}
		manifestCache.Unlock()
		return nil, err
	}

	manifestCache.Lock()
	defer manifestCache.Unlock()

	if manifestCache.path == path && manifestCache.mtime.Equal(info.ModTime()) && manifestCache.index != nil {
		return manifestCache.index, nil
	}

	idx, err := scanManifest(path)
	if err != nil {
		return nil, err
	}
	manifestCache.path = path
	manifestCache.mtime = info.ModTime()
	manifestCache.index = idx
	return idx, nil
}

// scanManifest reads path line-by-line and returns a hash → most-
// recent original_filename map. Last-write-wins is the contract: a
// re-upload of the same hash with a renamed file shadows earlier
// entries. Malformed lines are silently skipped — a truncated final
// line from a crashed write should not poison the whole file.
func scanManifest(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	out := make(map[string]string)
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry UploadManifestEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if entry.Hash == "" || entry.OriginalFilename == "" {
			continue
		}
		out[entry.Hash] = entry.OriginalFilename
	}
	if err := sc.Err(); err != nil {
		return nil, err
	}
	return out, nil
}
