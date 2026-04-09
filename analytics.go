package main

import (
	"bufio"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// AnalyticsSchemaVersion is the on-disk envelope version for tuning events.
// Bump this when the envelope shape changes in a way that requires a
// migration. Payload contents may evolve additively without bumping this.
// See docs/knowledge/analytics-schema.md for the migration policy.
const AnalyticsSchemaVersion = 1

// validEventTypes enumerates the v1 event_type set. Anything outside this
// set is rejected by Validate(). New event types require an additive change
// here and a documentation update in analytics-schema.md, but no schema bump.
var validEventTypes = map[string]bool{
	"session_start":   true,
	"session_end":     true,
	"setting_changed": true,
	"regenerate":      true,
	"accept":          true,
	"discard":         true,
	"profile_saved":   true, // T-003-03
	"profile_applied": true, // T-003-03
	"classification":  true, // T-004-02
	"strategy_selected": true, // T-004-03
	"classification_override": true, // T-004-04
	"scene_template_selected": true, // T-006-02
	"prepare_for_scene": true, // T-008-01
}

// Event is the canonical envelope for a tuning analytics event. Each event
// is written as a single JSON line to ~/.glb-optimizer/tuning/{session}.jsonl.
//
// The envelope is the contract; the Payload field is intentionally an opaque
// map so new event types and payload fields can be added without forcing a
// schema migration. Per-type payload schemas are documented (but not
// enforced server-side) in docs/knowledge/analytics-schema.md.
type Event struct {
	SchemaVersion int                    `json:"schema_version"`
	EventType     string                 `json:"event_type"`
	Timestamp     string                 `json:"timestamp"`
	SessionID     string                 `json:"session_id"`
	AssetID       string                 `json:"asset_id"`
	Payload       map[string]interface{} `json:"payload"`
}

// Validate enforces the envelope contract. Payload contents are not
// inspected here — see analytics-schema.md for the per-type payload shapes.
func (e *Event) Validate() error {
	if e.SchemaVersion != AnalyticsSchemaVersion {
		return fmt.Errorf("unsupported schema_version: %d (expected %d)", e.SchemaVersion, AnalyticsSchemaVersion)
	}
	if !validEventTypes[e.EventType] {
		return fmt.Errorf("unknown event_type: %q", e.EventType)
	}
	if e.Timestamp == "" {
		return fmt.Errorf("timestamp must be set")
	}
	if e.SessionID == "" {
		return fmt.Errorf("session_id must be set")
	}
	if e.Payload == nil {
		return fmt.Errorf("payload must be a JSON object (use {} for empty)")
	}
	return nil
}

// newSessionID returns an RFC 4122 v4 UUID. Hand-rolled because the project
// has no third-party dependencies. The visual format is intentionally
// distinguishable from the 32-char hex generateID() used for file ids in
// the same workdir.
func newSessionID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		// crypto/rand should never fail on supported platforms; if it
		// does, fall back to a deterministic but still unique-enough
		// value derived from the current nanosecond clock.
		ns := time.Now().UnixNano()
		for i := 0; i < 16; i++ {
			b[i] = byte(ns >> (uint(i) * 4))
		}
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// AnalyticsLogger owns the tuning directory and serializes appends to the
// per-session JSONL files. A single process-wide mutex is sufficient at the
// expected (human-rate) event throughput.
type AnalyticsLogger struct {
	mu sync.Mutex
	// tuningDir is where {sessionID}.jsonl files live.
	tuningDir string
	// assetIndex caches "which session id belongs to this asset id" so
	// repeated lookups within one process don't re-scan the directory.
	// Populated lazily by LookupOrStartSession; not persisted.
	assetIndex map[string]string
}

// NewAnalyticsLogger constructs a logger writing into tuningDir. The
// directory itself is expected to exist already (main.go MkdirAll's it at
// startup); the logger does not create it lazily so that startup-time
// permission errors surface immediately.
func NewAnalyticsLogger(tuningDir string) *AnalyticsLogger {
	return &AnalyticsLogger{
		tuningDir:  tuningDir,
		assetIndex: make(map[string]string),
	}
}

// AppendEvent appends a single event to the per-session JSONL file. The
// write is performed under a process-wide mutex against a freshly opened
// O_APPEND file handle, which gives us:
//
//   - In-process serialization (the mutex).
//   - POSIX append atomicity for writes <= PIPE_BUF (4096 bytes); a tuning
//     envelope is far smaller than that.
//   - Crash safety: any prior bytes already on disk are intact, only the
//     in-flight event is lost.
//
// The handle is closed after every write so we never have to think about
// rotate semantics, fsync schedules, or long-lived file descriptors.
func (a *AnalyticsLogger) AppendEvent(sessionID string, ev Event) error {
	if sessionID == "" {
		return fmt.Errorf("sessionID must be set")
	}
	data, err := json.Marshal(&ev)
	if err != nil {
		return fmt.Errorf("marshal event: %w", err)
	}
	data = append(data, '\n')

	a.mu.Lock()
	defer a.mu.Unlock()

	path := filepath.Join(a.tuningDir, sessionID+".jsonl")
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	if _, err := f.Write(data); err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}

// StartSession mints a new session id, writes a session_start envelope to
// the corresponding JSONL file, and returns the new id.
//
// In v1 the *frontend* is the primary producer of events and mints its own
// session ids client-side via crypto.randomUUID(); this Go-side StartSession
// exists for symmetry, for unit testing, and for the eventual case where
// the backend logs events on its own behalf (e.g. a future processing
// pipeline). Both sides write into the same on-disk format.
func (a *AnalyticsLogger) StartSession(assetID string) (string, error) {
	id := newSessionID()
	ev := Event{
		SchemaVersion: AnalyticsSchemaVersion,
		EventType:     "session_start",
		Timestamp:     time.Now().UTC().Format(time.RFC3339Nano),
		SessionID:     id,
		AssetID:       assetID,
		Payload:       map[string]interface{}{"trigger": "open_asset"},
	}
	if err := a.AppendEvent(id, ev); err != nil {
		return "", err
	}
	return id, nil
}

// envHead is the minimal projection of a session_start envelope used to
// identify which asset a JSONL file belongs to. Only the fields we need
// to scan are decoded.
type envHead struct {
	EventType string `json:"event_type"`
	AssetID   string `json:"asset_id"`
	SessionID string `json:"session_id"`
}

// firstEnvelope reads and decodes the first JSON line of a tuning JSONL
// file. Returns an error if the file cannot be opened, has no first line,
// or the first line is not valid JSON. Callers should treat any error
// here as "skip this file" rather than fatal.
func firstEnvelope(path string) (envHead, error) {
	var h envHead
	f, err := os.Open(path)
	if err != nil {
		return h, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	// Tuning envelopes are tiny (<1 KiB) but allocate a generous buffer
	// so a future, larger payload doesn't break first-line scanning.
	sc.Buffer(make([]byte, 64*1024), 1024*1024)
	if !sc.Scan() {
		if err := sc.Err(); err != nil {
			return h, err
		}
		return h, fmt.Errorf("empty file")
	}
	if err := json.Unmarshal(sc.Bytes(), &h); err != nil {
		return h, err
	}
	return h, nil
}

// LookupOrStartSession returns the session id associated with the given
// asset. If a JSONL file in the tuning dir already starts with a
// session_start envelope for this asset_id, that session id is returned
// with resumed=true; otherwise a fresh session is minted via StartSession
// (which writes a new session_start line) and resumed=false.
//
// The lookup is O(N) where N is the number of session files on disk, but
// the result is cached in the in-memory assetIndex map so subsequent
// lookups within the same process are O(1). The cache is not persisted
// across restarts; on a fresh process the first lookup per asset
// re-scans, which is fine at human selection rates.
//
// Lock discipline: we hold mu while reading the cache and scanning the
// directory, but release it before calling StartSession (which itself
// takes mu via AppendEvent). After StartSession returns we re-acquire mu
// to update the cache.
func (a *AnalyticsLogger) LookupOrStartSession(assetID string) (string, bool, error) {
	if assetID == "" {
		return "", false, fmt.Errorf("assetID must be set")
	}

	a.mu.Lock()
	if id, ok := a.assetIndex[assetID]; ok {
		a.mu.Unlock()
		return id, true, nil
	}

	// Scan the directory for an existing session_start matching assetID.
	// Sort by mtime descending so the most recent session wins; tiebreak
	// by name for determinism.
	entries, err := os.ReadDir(a.tuningDir)
	if err != nil {
		a.mu.Unlock()
		return "", false, fmt.Errorf("read tuning dir: %w", err)
	}
	type entryInfo struct {
		name  string
		mtime time.Time
	}
	var jsonlFiles []entryInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		jsonlFiles = append(jsonlFiles, entryInfo{name: e.Name(), mtime: info.ModTime()})
	}
	sort.Slice(jsonlFiles, func(i, j int) bool {
		if !jsonlFiles[i].mtime.Equal(jsonlFiles[j].mtime) {
			return jsonlFiles[i].mtime.After(jsonlFiles[j].mtime)
		}
		return jsonlFiles[i].name < jsonlFiles[j].name
	})

	for _, ef := range jsonlFiles {
		path := filepath.Join(a.tuningDir, ef.name)
		h, err := firstEnvelope(path)
		if err != nil {
			// Corrupt or unreadable; skip silently and keep scanning.
			continue
		}
		if h.EventType == "session_start" && h.AssetID == assetID && h.SessionID != "" {
			a.assetIndex[assetID] = h.SessionID
			a.mu.Unlock()
			return h.SessionID, true, nil
		}
	}

	a.mu.Unlock()

	// No prior session found; mint a new one. StartSession reacquires mu
	// internally via AppendEvent.
	id, err := a.StartSession(assetID)
	if err != nil {
		return "", false, err
	}

	a.mu.Lock()
	a.assetIndex[assetID] = id
	a.mu.Unlock()

	return id, false, nil
}
