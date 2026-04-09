//go:build ignore

// scripts/backfill-uploads-manifest.go is the T-012-04 one-shot
// migration: walk an existing <workDir>/originals directory and write
// one uploads.jsonl line per *.glb file that is not already recorded.
//
// The server's upload handler now writes the manifest live, but any
// pre-existing intermediates from before T-012-04 shipped have no
// record. Run this script once after deploying:
//
//   go run scripts/backfill-uploads-manifest.go [-dir ~/.glb-optimizer]
//
// Idempotent: re-running scans the existing manifest first and skips
// any hash already recorded. Lost data caveat: the script cannot
// recover the human-readable upload-time name (it was never stored on
// disk), so original_filename for backfilled rows is "<hash>.glb" —
// the resolver will correctly identify these as the post-restart
// sentinel and fall through to its hash-fallback tier.
//
// This file lives under //go:build ignore so `go build ./...` does
// not pick it up as a second package main.

package main

import (
	"bufio"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

type entry struct {
	Hash             string    `json:"hash"`
	OriginalFilename string    `json:"original_filename"`
	UploadedAt       time.Time `json:"uploaded_at"`
	Size             int64     `json:"size"`
}

func main() {
	var dir string
	flag.StringVar(&dir, "dir", "", "workDir (default ~/.glb-optimizer)")
	flag.Parse()

	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			die("home dir: %v", err)
		}
		dir = filepath.Join(home, ".glb-optimizer")
	}
	originalsDir := filepath.Join(dir, "originals")
	manifestPath := filepath.Join(dir, "uploads.jsonl")

	seen, err := loadSeenHashes(manifestPath)
	if err != nil {
		die("read existing manifest %s: %v", manifestPath, err)
	}

	entries, err := os.ReadDir(originalsDir)
	if err != nil {
		die("read originals %s: %v", originalsDir, err)
	}

	scanned, skipped, appended := 0, 0, 0
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".glb") {
			continue
		}
		scanned++
		hash := strings.TrimSuffix(e.Name(), ".glb")
		if seen[hash] {
			skipped++
			continue
		}
		info, err := e.Info()
		if err != nil {
			fmt.Fprintf(os.Stderr, "stat %s: %v\n", e.Name(), err)
			continue
		}
		rec := entry{
			Hash:             hash,
			OriginalFilename: e.Name(), // best effort — see header comment
			UploadedAt:       info.ModTime().UTC(),
			Size:             info.Size(),
		}
		if err := appendOne(manifestPath, rec); err != nil {
			fmt.Fprintf(os.Stderr, "append %s: %v\n", hash, err)
			continue
		}
		seen[hash] = true
		appended++
	}

	fmt.Printf("backfill: scanned=%d skipped=%d appended=%d manifest=%s\n",
		scanned, skipped, appended, manifestPath)
}

func loadSeenHashes(path string) (map[string]bool, error) {
	out := map[string]bool{}
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return out, nil
		}
		return nil, err
	}
	defer f.Close()

	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		if len(line) == 0 {
			continue
		}
		var e entry
		if err := json.Unmarshal(line, &e); err != nil {
			continue
		}
		if e.Hash != "" {
			out[e.Hash] = true
		}
	}
	return out, sc.Err()
}

func appendOne(path string, e entry) error {
	line, err := json.Marshal(e)
	if err != nil {
		return err
	}
	line = append(line, '\n')
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	if _, err := f.Write(line); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func die(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}
