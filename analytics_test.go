package main

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"sync"
	"testing"
	"time"
)

var uuidV4Re = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNewSessionID_Format(t *testing.T) {
	for i := 0; i < 100; i++ {
		id := newSessionID()
		if !uuidV4Re.MatchString(id) {
			t.Fatalf("newSessionID() returned %q which is not RFC 4122 v4", id)
		}
	}
}

func validEvent() Event {
	return Event{
		SchemaVersion: AnalyticsSchemaVersion,
		EventType:     "setting_changed",
		Timestamp:     "2026-04-07T00:00:00Z",
		SessionID:     "00000000-0000-4000-8000-000000000000",
		AssetID:       "abc",
		Payload: map[string]interface{}{
			"key":       "bake_exposure",
			"old_value": 1.0,
			"new_value": 1.25,
		},
	}
}

func TestEventValidate_OK(t *testing.T) {
	ev := validEvent()
	if err := ev.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

// T-004-02: classification event type.
func TestEventValidate_AcceptsClassificationType(t *testing.T) {
	ev := validEvent()
	ev.EventType = "classification"
	ev.Payload = map[string]interface{}{
		"category":   "round-bush",
		"confidence": 0.83,
		"features":   map[string]interface{}{},
	}
	if err := ev.Validate(); err != nil {
		t.Errorf("classification event rejected: %v", err)
	}
}

// T-004-03: strategy_selected event type.
func TestEventValidate_AcceptsStrategySelectedType(t *testing.T) {
	ev := validEvent()
	ev.EventType = "strategy_selected"
	ev.Payload = map[string]interface{}{
		"category": "directional",
		"strategy": map[string]interface{}{
			"category":                  "directional",
			"slice_axis":                "auto-horizontal",
			"slice_count":               4,
			"slice_distribution_mode":   "equal-height",
			"instance_orientation_rule": "fixed",
			"default_budget_priority":   "mid",
		},
	}
	if err := ev.Validate(); err != nil {
		t.Errorf("strategy_selected event rejected: %v", err)
	}
}

// T-004-04: classification_override event type.
func TestEventValidate_AcceptsClassificationOverrideType(t *testing.T) {
	ev := validEvent()
	ev.EventType = "classification_override"
	ev.Payload = map[string]interface{}{
		"original_category":   "planar",
		"original_confidence": 0.42,
		"candidates": []interface{}{
			map[string]interface{}{"category": "planar", "score": 0.42},
			map[string]interface{}{"category": "directional", "score": 0.31},
		},
		"chosen_category": "directional",
		"features":        map[string]interface{}{},
	}
	if err := ev.Validate(); err != nil {
		t.Errorf("classification_override event rejected: %v", err)
	}
}

func TestEventValidate_RejectsBadVersion(t *testing.T) {
	ev := validEvent()
	ev.SchemaVersion = 2
	if err := ev.Validate(); err == nil {
		t.Error("expected error for schema_version=2")
	}
	ev.SchemaVersion = 0
	if err := ev.Validate(); err == nil {
		t.Error("expected error for schema_version=0")
	}
}

func TestEventValidate_RejectsUnknownType(t *testing.T) {
	ev := validEvent()
	ev.EventType = "lol"
	if err := ev.Validate(); err == nil {
		t.Error("expected error for event_type=lol")
	}
}

func TestEventValidate_RejectsEmptySession(t *testing.T) {
	ev := validEvent()
	ev.SessionID = ""
	if err := ev.Validate(); err == nil {
		t.Error("expected error for empty session_id")
	}
}

func TestEventValidate_RejectsEmptyTimestamp(t *testing.T) {
	ev := validEvent()
	ev.Timestamp = ""
	if err := ev.Validate(); err == nil {
		t.Error("expected error for empty timestamp")
	}
}

func TestEventValidate_RejectsNilPayload(t *testing.T) {
	ev := validEvent()
	ev.Payload = nil
	if err := ev.Validate(); err == nil {
		t.Error("expected error for nil payload")
	}
}

func TestAppendEvent_WritesJSONLine(t *testing.T) {
	dir := t.TempDir()
	logger := NewAnalyticsLogger(dir)
	ev := validEvent()
	if err := logger.AppendEvent(ev.SessionID, ev); err != nil {
		t.Fatalf("AppendEvent: %v", err)
	}

	path := filepath.Join(dir, ev.SessionID+".jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// Exactly one line, terminated by \n.
	if n := countLines(data); n != 1 {
		t.Errorf("expected 1 line, got %d", n)
	}

	var got Event
	if err := json.Unmarshal(stripTrailingNewline(data), &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got.EventType != ev.EventType || got.AssetID != ev.AssetID {
		t.Errorf("round-trip mismatch: got %+v want %+v", got, ev)
	}
}

func TestAppendEvent_AppendsMultiple(t *testing.T) {
	dir := t.TempDir()
	logger := NewAnalyticsLogger(dir)
	sid := "00000000-0000-4000-8000-000000000001"

	for i := 0; i < 3; i++ {
		ev := validEvent()
		ev.SessionID = sid
		ev.Payload["i"] = i
		if err := logger.AppendEvent(sid, ev); err != nil {
			t.Fatalf("AppendEvent[%d]: %v", i, err)
		}
	}

	path := filepath.Join(dir, sid+".jsonl")
	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	var lines []Event
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		var ev Event
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			t.Fatalf("unmarshal line: %v", err)
		}
		lines = append(lines, ev)
	}
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	for i, ev := range lines {
		// json.Unmarshal decodes JSON numbers as float64.
		got, ok := ev.Payload["i"].(float64)
		if !ok || int(got) != i {
			t.Errorf("line %d: payload.i = %v want %d", i, ev.Payload["i"], i)
		}
	}
}

func TestAppendEvent_ConcurrentWrites(t *testing.T) {
	dir := t.TempDir()
	logger := NewAnalyticsLogger(dir)
	sid := "00000000-0000-4000-8000-0000000000ff"

	const goroutines = 50
	const perG = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(g int) {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				ev := validEvent()
				ev.SessionID = sid
				ev.Payload["g"] = g
				ev.Payload["i"] = i
				if err := logger.AppendEvent(sid, ev); err != nil {
					t.Errorf("AppendEvent: %v", err)
					return
				}
			}
		}(g)
	}
	wg.Wait()

	f, err := os.Open(filepath.Join(dir, sid+".jsonl"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	defer f.Close()

	count := 0
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for scanner.Scan() {
		var ev Event
		if err := json.Unmarshal(scanner.Bytes(), &ev); err != nil {
			t.Fatalf("torn or invalid line %d: %v\n  raw: %q", count, err, scanner.Text())
		}
		count++
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan: %v", err)
	}
	if count != goroutines*perG {
		t.Errorf("expected %d lines, got %d", goroutines*perG, count)
	}
}

func TestAppendEvent_RejectsEmptySessionID(t *testing.T) {
	dir := t.TempDir()
	logger := NewAnalyticsLogger(dir)
	if err := logger.AppendEvent("", validEvent()); err == nil {
		t.Error("expected error for empty sessionID")
	}
}

func TestStartSession_EmitsSessionStart(t *testing.T) {
	dir := t.TempDir()
	logger := NewAnalyticsLogger(dir)

	id, err := logger.StartSession("asset-xyz")
	if err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if !uuidV4Re.MatchString(id) {
		t.Fatalf("StartSession returned non-v4 UUID: %q", id)
	}

	data, err := os.ReadFile(filepath.Join(dir, id+".jsonl"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if countLines(data) != 1 {
		t.Errorf("expected 1 line, got %d", countLines(data))
	}

	var ev Event
	if err := json.Unmarshal(stripTrailingNewline(data), &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ev.EventType != "session_start" {
		t.Errorf("event_type = %q, want session_start", ev.EventType)
	}
	if ev.AssetID != "asset-xyz" {
		t.Errorf("asset_id = %q, want asset-xyz", ev.AssetID)
	}
	if ev.SessionID != id {
		t.Errorf("session_id = %q, want %q", ev.SessionID, id)
	}
	if trig, _ := ev.Payload["trigger"].(string); trig != "open_asset" {
		t.Errorf("payload.trigger = %v, want open_asset", ev.Payload["trigger"])
	}
}

// ── LookupOrStartSession ──

func TestLookupOrStartSession_NewAsset(t *testing.T) {
	dir := t.TempDir()
	logger := NewAnalyticsLogger(dir)

	id, resumed, err := logger.LookupOrStartSession("asset-new")
	if err != nil {
		t.Fatalf("LookupOrStartSession: %v", err)
	}
	if resumed {
		t.Error("resumed = true on first lookup, want false")
	}
	if !uuidV4Re.MatchString(id) {
		t.Errorf("id = %q, not a v4 UUID", id)
	}

	// File exists with one session_start envelope.
	data, err := os.ReadFile(filepath.Join(dir, id+".jsonl"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	if countLines(data) != 1 {
		t.Errorf("expected 1 line, got %d", countLines(data))
	}
	var ev Event
	if err := json.Unmarshal(stripTrailingNewline(data), &ev); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ev.EventType != "session_start" || ev.AssetID != "asset-new" {
		t.Errorf("envelope mismatch: type=%q asset=%q", ev.EventType, ev.AssetID)
	}

	// Second lookup hits the in-memory cache → resumed=true, same id.
	id2, resumed2, err := logger.LookupOrStartSession("asset-new")
	if err != nil {
		t.Fatalf("second lookup: %v", err)
	}
	if !resumed2 || id2 != id {
		t.Errorf("second lookup: id=%q resumed=%v, want id=%q resumed=true", id2, resumed2, id)
	}
}

func TestLookupOrStartSession_ResumesExisting(t *testing.T) {
	dir := t.TempDir()

	// Pre-create a JSONL with a valid session_start for "abc".
	existingID := "11111111-1111-4111-8111-111111111111"
	ev := Event{
		SchemaVersion: AnalyticsSchemaVersion,
		EventType:     "session_start",
		Timestamp:     "2026-04-07T00:00:00Z",
		SessionID:     existingID,
		AssetID:       "abc",
		Payload:       map[string]interface{}{"trigger": "open_asset"},
	}
	line, _ := json.Marshal(&ev)
	if err := os.WriteFile(filepath.Join(dir, existingID+".jsonl"), append(line, '\n'), 0644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}

	logger := NewAnalyticsLogger(dir)
	id, resumed, err := logger.LookupOrStartSession("abc")
	if err != nil {
		t.Fatalf("LookupOrStartSession: %v", err)
	}
	if !resumed {
		t.Error("resumed = false, want true")
	}
	if id != existingID {
		t.Errorf("id = %q, want %q", id, existingID)
	}

	// No new file should have been created.
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Errorf("expected 1 file, got %d", len(entries))
	}
}

func TestLookupOrStartSession_PicksMostRecent(t *testing.T) {
	dir := t.TempDir()

	older := "22222222-2222-4222-8222-222222222222"
	newer := "33333333-3333-4333-8333-333333333333"

	for _, id := range []string{older, newer} {
		ev := Event{
			SchemaVersion: AnalyticsSchemaVersion,
			EventType:     "session_start",
			Timestamp:     "2026-04-07T00:00:00Z",
			SessionID:     id,
			AssetID:       "shared-asset",
			Payload:       map[string]interface{}{"trigger": "open_asset"},
		}
		line, _ := json.Marshal(&ev)
		path := filepath.Join(dir, id+".jsonl")
		if err := os.WriteFile(path, append(line, '\n'), 0644); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}

	// Force older to have an mtime in the past.
	past := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(filepath.Join(dir, older+".jsonl"), past, past); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	logger := NewAnalyticsLogger(dir)
	id, resumed, err := logger.LookupOrStartSession("shared-asset")
	if err != nil {
		t.Fatalf("LookupOrStartSession: %v", err)
	}
	if !resumed {
		t.Error("resumed = false, want true")
	}
	if id != newer {
		t.Errorf("id = %q, want %q (newer)", id, newer)
	}
}

func TestLookupOrStartSession_SkipsCorrupt(t *testing.T) {
	dir := t.TempDir()

	// File 1: corrupt (non-JSON first line), newer mtime.
	corruptPath := filepath.Join(dir, "00000000-0000-4000-8000-000000000000.jsonl")
	if err := os.WriteFile(corruptPath, []byte("not-json\n"), 0644); err != nil {
		t.Fatalf("WriteFile corrupt: %v", err)
	}

	// File 2: valid session_start for "asset-skip", older mtime.
	validID := "44444444-4444-4444-8444-444444444444"
	ev := Event{
		SchemaVersion: AnalyticsSchemaVersion,
		EventType:     "session_start",
		Timestamp:     "2026-04-07T00:00:00Z",
		SessionID:     validID,
		AssetID:       "asset-skip",
		Payload:       map[string]interface{}{"trigger": "open_asset"},
	}
	line, _ := json.Marshal(&ev)
	validPath := filepath.Join(dir, validID+".jsonl")
	if err := os.WriteFile(validPath, append(line, '\n'), 0644); err != nil {
		t.Fatalf("WriteFile valid: %v", err)
	}
	past := time.Now().Add(-1 * time.Hour)
	if err := os.Chtimes(validPath, past, past); err != nil {
		t.Fatalf("Chtimes: %v", err)
	}

	logger := NewAnalyticsLogger(dir)
	id, resumed, err := logger.LookupOrStartSession("asset-skip")
	if err != nil {
		t.Fatalf("LookupOrStartSession: %v", err)
	}
	if !resumed {
		t.Error("resumed = false, want true (should have found the valid file)")
	}
	if id != validID {
		t.Errorf("id = %q, want %q", id, validID)
	}
}

// ── helpers ──

func countLines(data []byte) int {
	if len(data) == 0 {
		return 0
	}
	n := 0
	for _, b := range data {
		if b == '\n' {
			n++
		}
	}
	return n
}

func stripTrailingNewline(data []byte) []byte {
	if len(data) > 0 && data[len(data)-1] == '\n' {
		return data[:len(data)-1]
	}
	return data
}
