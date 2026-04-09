package main

import (
	"encoding/json"
	"math"
	"reflect"
	"strings"
	"testing"
)

// canonicalJSON is a hand-typed Pack v1 metadata blob used to verify that
// the JSON struct tags decode to the right Go fields. It must NOT be
// produced by serializing a struct — that would make the round-trip test
// tautological. See ticket T-010-01 Notes.
const canonicalJSON = `{
  "format_version": 1,
  "bake_id": "2026-04-08T11:32:00Z",
  "species": "achillea_millefolium",
  "common_name": "Common Yarrow",
  "footprint": {"canopy_radius_m": 0.45, "height_m": 0.62},
  "fade": {"low_start": 0.3, "low_end": 0.55, "high_start": 0.75}
}`

// validMeta is the matching Go value. Tests mutate a copy of this and
// assert that Validate now fails.
func validMeta() PackMeta {
	return PackMeta{
		FormatVersion: 1,
		BakeID:        "2026-04-08T11:32:00Z",
		Species:       "achillea_millefolium",
		CommonName:    "Common Yarrow",
		Footprint:     Footprint{CanopyRadiusM: 0.45, HeightM: 0.62},
		Fade:          FadeBand{LowStart: 0.3, LowEnd: 0.55, HighStart: 0.75},
	}
}

func TestPackMeta_DefaultIsValid(t *testing.T) {
	if err := validMeta().Validate(); err != nil {
		t.Fatalf("validMeta().Validate() = %v, want nil", err)
	}
}

func TestPackMeta_RoundTrip(t *testing.T) {
	var m PackMeta
	if err := json.Unmarshal([]byte(canonicalJSON), &m); err != nil {
		t.Fatalf("decode canonicalJSON: %v", err)
	}
	// Assert decoded fields equal the literals as typed in canonicalJSON.
	// This catches wrong/missing JSON tag names.
	if m.FormatVersion != 1 {
		t.Errorf("FormatVersion = %d, want 1", m.FormatVersion)
	}
	if m.BakeID != "2026-04-08T11:32:00Z" {
		t.Errorf("BakeID = %q", m.BakeID)
	}
	if m.Species != "achillea_millefolium" {
		t.Errorf("Species = %q", m.Species)
	}
	if m.CommonName != "Common Yarrow" {
		t.Errorf("CommonName = %q", m.CommonName)
	}
	if m.Footprint.CanopyRadiusM != 0.45 {
		t.Errorf("Footprint.CanopyRadiusM = %g, want 0.45", m.Footprint.CanopyRadiusM)
	}
	if m.Footprint.HeightM != 0.62 {
		t.Errorf("Footprint.HeightM = %g, want 0.62", m.Footprint.HeightM)
	}
	if m.Fade.LowStart != 0.3 || m.Fade.LowEnd != 0.55 || m.Fade.HighStart != 0.75 {
		t.Errorf("Fade = %+v", m.Fade)
	}
	if err := m.Validate(); err != nil {
		t.Errorf("decoded Validate: %v", err)
	}

	// Re-encode and decode again. Compare structurally (whitespace varies).
	out, err := json.Marshal(m)
	if err != nil {
		t.Fatalf("re-encode: %v", err)
	}
	var m2 PackMeta
	if err := json.Unmarshal(out, &m2); err != nil {
		t.Fatalf("re-decode: %v", err)
	}
	if !reflect.DeepEqual(m, m2) {
		t.Errorf("round-trip mismatch:\n  got:  %+v\n  want: %+v", m2, m)
	}
}

func TestPackMeta_RejectsBadVersion(t *testing.T) {
	for _, v := range []int{0, 2, -1, 99} {
		m := validMeta()
		m.FormatVersion = v
		if err := m.Validate(); err == nil {
			t.Errorf("FormatVersion=%d: expected error, got nil", v)
		}
	}
}

func TestPackMeta_RejectsBadSpecies(t *testing.T) {
	cases := []string{
		"",
		"   ",
		"1leading_digit",
		"with-dash",
		"WithCaps",
		"with.dot",
		"with space",
		"_leading_underscore",
	}
	for _, sp := range cases {
		m := validMeta()
		m.Species = sp
		if err := m.Validate(); err == nil {
			t.Errorf("species=%q: expected error, got nil", sp)
		}
	}
}

func TestPackMeta_RejectsMissingCommonName(t *testing.T) {
	for _, name := range []string{"", "   ", "\t\n"} {
		m := validMeta()
		m.CommonName = name
		if err := m.Validate(); err == nil {
			t.Errorf("common_name=%q: expected error, got nil", name)
		}
	}
}

func TestPackMeta_RejectsMissingBakeID(t *testing.T) {
	for _, id := range []string{"", "  "} {
		m := validMeta()
		m.BakeID = id
		if err := m.Validate(); err == nil {
			t.Errorf("bake_id=%q: expected error, got nil", id)
		}
	}
}

func TestPackMeta_RejectsBadFootprint(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*PackMeta)
	}{
		{"zero canopy radius", func(m *PackMeta) { m.Footprint.CanopyRadiusM = 0 }},
		{"negative canopy radius", func(m *PackMeta) { m.Footprint.CanopyRadiusM = -0.1 }},
		{"NaN canopy radius", func(m *PackMeta) { m.Footprint.CanopyRadiusM = math.NaN() }},
		{"Inf canopy radius", func(m *PackMeta) { m.Footprint.CanopyRadiusM = math.Inf(1) }},
		{"zero height", func(m *PackMeta) { m.Footprint.HeightM = 0 }},
		{"negative height", func(m *PackMeta) { m.Footprint.HeightM = -1 }},
		{"NaN height", func(m *PackMeta) { m.Footprint.HeightM = math.NaN() }},
	}
	for _, c := range cases {
		m := validMeta()
		c.mut(&m)
		if err := m.Validate(); err == nil {
			t.Errorf("%s: expected error, got nil", c.name)
		}
	}
}

func TestPackMeta_RejectsFadeOutOfRange(t *testing.T) {
	cases := []struct {
		name string
		mut  func(*PackMeta)
	}{
		{"low_start < 0", func(m *PackMeta) { m.Fade.LowStart = -0.1 }},
		{"low_end > 1", func(m *PackMeta) { m.Fade.LowEnd = 1.5 }},
		{"high_start > 1", func(m *PackMeta) { m.Fade.HighStart = 1.5 }},
		{"low_start NaN", func(m *PackMeta) { m.Fade.LowStart = math.NaN() }},
	}
	for _, c := range cases {
		m := validMeta()
		c.mut(&m)
		if err := m.Validate(); err == nil {
			t.Errorf("%s: expected error, got nil", c.name)
		}
	}
}

func TestPackMeta_RejectsFadeOutOfOrder(t *testing.T) {
	cases := []struct {
		name              string
		ls, le, hs        float64
	}{
		{"low_start == low_end", 0.5, 0.5, 0.75},
		{"low_end == high_start", 0.3, 0.6, 0.6},
		{"low_start > low_end", 0.6, 0.5, 0.75},
		{"low_end > high_start", 0.3, 0.8, 0.5},
		{"all zero", 0, 0, 0},
	}
	for _, c := range cases {
		m := validMeta()
		m.Fade = FadeBand{LowStart: c.ls, LowEnd: c.le, HighStart: c.hs}
		if err := m.Validate(); err == nil {
			t.Errorf("%s: expected error, got nil", c.name)
		}
	}
}

func TestPackMeta_AllowsHighStartEqualOne(t *testing.T) {
	// Degenerate "no high crossfade" case — legal for groundcover that
	// has no tilted view.
	m := validMeta()
	m.Fade = FadeBand{LowStart: 0.3, LowEnd: 0.55, HighStart: 1.0}
	if err := m.Validate(); err != nil {
		t.Errorf("high_start=1.0 should be allowed: %v", err)
	}
}

func TestPackMeta_ToExtras(t *testing.T) {
	m := validMeta()
	got := m.ToExtras()
	want := map[string]any{
		"format_version": 1,
		"bake_id":        "2026-04-08T11:32:00Z",
		"species":        "achillea_millefolium",
		"common_name":    "Common Yarrow",
		"footprint": map[string]any{
			"canopy_radius_m": 0.45,
			"height_m":        0.62,
		},
		"fade": map[string]any{
			"low_start":  0.3,
			"low_end":    0.55,
			"high_start": 0.75,
		},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("ToExtras mismatch:\n  got:  %#v\n  want: %#v", got, want)
	}
}

func TestParsePackMeta_RoundTrip(t *testing.T) {
	m, err := ParsePackMeta(json.RawMessage(canonicalJSON))
	if err != nil {
		t.Fatalf("ParsePackMeta: %v", err)
	}
	if m.Species != "achillea_millefolium" {
		t.Errorf("Species = %q", m.Species)
	}
	// Pipe ToExtras back through Marshal and re-parse to confirm closure.
	out, err := json.Marshal(m.ToExtras())
	if err != nil {
		t.Fatalf("marshal extras: %v", err)
	}
	m2, err := ParsePackMeta(out)
	if err != nil {
		t.Fatalf("re-parse: %v", err)
	}
	if !reflect.DeepEqual(m, m2) {
		t.Errorf("ToExtras round-trip mismatch:\n  got:  %+v\n  want: %+v", m2, m)
	}
}

func TestParsePackMeta_RejectsBadJSON(t *testing.T) {
	_, err := ParsePackMeta(json.RawMessage(`{not json`))
	if err == nil {
		t.Fatal("expected decode error, got nil")
	}
	if !strings.Contains(err.Error(), "pack_meta: decode") {
		t.Errorf("error not prefixed with 'pack_meta: decode': %v", err)
	}
}

func TestParsePackMeta_RejectsBadValues(t *testing.T) {
	// Valid JSON, fade out of order.
	bad := `{
      "format_version": 1,
      "bake_id": "2026-04-08T00:00:00Z",
      "species": "test_plant",
      "common_name": "Test",
      "footprint": {"canopy_radius_m": 0.5, "height_m": 1.0},
      "fade": {"low_start": 0.8, "low_end": 0.5, "high_start": 0.9}
    }`
	_, err := ParsePackMeta(json.RawMessage(bad))
	if err == nil {
		t.Fatal("expected validate error, got nil")
	}
	if !strings.Contains(err.Error(), "pack_meta: validate") {
		t.Errorf("error not prefixed with 'pack_meta: validate': %v", err)
	}
}
