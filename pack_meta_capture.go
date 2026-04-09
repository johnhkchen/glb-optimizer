package main

// pack_meta_capture.go is the bridge between bake-time state and the
// PackMeta contract frozen in pack_meta.go. Its single exported entry
// point, BuildPackMetaFromBake, reads the un-decimated source mesh,
// the per-asset settings (or defaults), and an optional opaque
// per-asset override file, then assembles a fully populated PackMeta
// ready for combine (T-010-02) to embed at scene.extras.plantastic.
//
// Capture fails loudly. A missing source mesh, a degenerate footprint,
// or a derived species id that fails the v1 slug regex are all hard
// errors so the operator sees the problem before a half-baked pack
// ships.

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// nonAlnumRe matches any run of characters that the species slug
// rule does not allow (anything outside [a-z0-9_]). Used by
// deriveSpeciesFromName to collapse separators to underscores.
var nonAlnumRe = regexp.MustCompile(`[^a-z0-9_]+`)

// BuildPackMetaFromBake reads the bake-time state for asset id and
// assembles a fully populated, validated PackMeta. Inputs:
//   - the un-decimated source mesh at originalsDir/{id}.glb
//   - the species/common_name resolved by ResolveSpeciesIdentity
//     (T-012-01); see species_resolver.go for the tier order
//   - the current AssetSettings at settingsDir/{id}.json (or defaults)
//
// Returns a zero-value PackMeta and an error on any failure.
func BuildPackMetaFromBake(
	id, originalsDir, settingsDir, outputsDir string,
	store *FileStore,
	opts ResolverOptions,
) (PackMeta, error) {
	// T-012-01: species and common_name come from the resolver. The
	// resolver never returns a non-nil error today; the safety-net
	// content-hash tier guarantees a usable identity for any id.
	identity, source, _ := ResolveSpeciesIdentity(id, outputsDir, store, opts)
	log.Printf("pack_meta_capture: %s species=%s common_name=%q source=%s",
		id, identity.Species, identity.CommonName, source)
	species := identity.Species
	common := identity.CommonName

	// Footprint from the un-decimated source mesh.
	sourcePath := filepath.Join(originalsDir, id+".glb")
	footprint, err := readSourceFootprint(sourcePath)
	if err != nil {
		return PackMeta{}, fmt.Errorf("pack_meta_capture: footprint for %s: %w", sourcePath, err)
	}

	// Fade band from current settings (defaults if no settings file).
	fade, err := captureFadeFromSettings(id, settingsDir)
	if err != nil {
		return PackMeta{}, fmt.Errorf("pack_meta_capture: fade for %q: %w", id, err)
	}

	bakeID, err := resolveBakeID(id, outputsDir)
	if err != nil {
		return PackMeta{}, fmt.Errorf("pack_meta_capture: bake_id: %w", err)
	}

	meta := PackMeta{
		FormatVersion: PackFormatVersion,
		BakeID:        bakeID,
		Species:       species,
		CommonName:    common,
		Footprint:     footprint,
		Fade:          fade,
	}
	if err := meta.Validate(); err != nil {
		return PackMeta{}, fmt.Errorf("pack_meta_capture: validate: %w", err)
	}
	return meta, nil
}

// resolveBakeID returns a stable bake_id for the asset. It first looks
// for {outputsDir}/{id}_bake.json (written by WriteBakeStamp when the
// bake driver completes — see T-011-03). If absent or empty, it logs a
// one-line warning and falls back to a fresh time.Now() UTC stamp. A
// malformed file is propagated as an error: silently masking it would
// let combine ship a pack with a bogus id.
func resolveBakeID(id, outputsDir string) (string, error) {
	stamp, err := ReadBakeStamp(outputsDir, id)
	if err != nil {
		return "", err
	}
	if stamp.BakeID != "" {
		return stamp.BakeID, nil
	}
	log.Printf("pack_meta_capture: %s: no bake stamp at %s, falling back to current time as bake_id; "+
		"rebake to get a stable id", id, bakeStampPath(outputsDir, id))
	return time.Now().UTC().Format(time.RFC3339), nil
}

// deriveSpeciesFromName turns a filename or id into a slug satisfying
// PackMeta's species regex (^[a-z][a-z0-9_]*$). Returns "" if the
// input has no usable letters; the caller emits the operator-facing
// error pointing at the override file.
//
// Pipeline: strip extension → lowercase → non-alnum → "_" → strip
// leading non-letters → strip trailing "_" → collapse repeated "_".
func deriveSpeciesFromName(name string) string {
	// Strip the file extension if any. Unlike a hard-coded ".glb"
	// match, this also handles ".gltf" and arbitrary uppercase
	// variants without special-casing.
	if ext := filepath.Ext(name); ext != "" {
		name = strings.TrimSuffix(name, ext)
	}
	s := strings.ToLower(name)
	s = nonAlnumRe.ReplaceAllString(s, "_")
	// Strip leading characters that are not in [a-z]. The species
	// regex demands the first character be a letter, so digits,
	// underscores, and any leftover separator runs collapsed by the
	// previous step are all stripped here.
	for len(s) > 0 && !(s[0] >= 'a' && s[0] <= 'z') {
		s = s[1:]
	}
	s = strings.TrimRight(s, "_")
	// Collapse runs of underscores produced by adjacent separators
	// (e.g. "rose - julia" → "rose___julia" → "rose_julia").
	for strings.Contains(s, "__") {
		s = strings.ReplaceAll(s, "__", "_")
	}
	return s
}

// titleCaseSpecies maps a species slug like "rose_julia_child" to its
// human-readable common-name fallback "Rose Julia Child". ASCII-only;
// the species regex already restricts inputs to [a-z0-9_].
func titleCaseSpecies(species string) string {
	parts := strings.Split(species, "_")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p == "" {
			continue
		}
		b := []byte(p)
		if b[0] >= 'a' && b[0] <= 'z' {
			b[0] -= 'a' - 'A'
		}
		out = append(out, string(b))
	}
	return strings.Join(out, " ")
}

// captureFadeFromSettings reads the per-asset settings (or defaults)
// and projects the three tilted-fade thresholds into a FadeBand.
// AssetSettings.Validate is intentionally NOT called here — the
// downstream PackMeta.Validate will catch any out-of-range or
// mis-ordered values with field names matching the pack contract.
func captureFadeFromSettings(id, settingsDir string) (FadeBand, error) {
	s, err := LoadSettings(id, settingsDir)
	if err != nil {
		return FadeBand{}, err
	}
	return FadeBand{
		LowStart:  s.TiltedFadeLowStart,
		LowEnd:    s.TiltedFadeLowEnd,
		HighStart: s.TiltedFadeHighStart,
	}, nil
}

// readSourceFootprint opens a GLB at path, parses just its JSON
// chunk, and returns the footprint computed from the local-space
// AABB across every primitive's POSITION accessor min/max.
//
// glTF 2.0 §3.6.2.4 requires POSITION accessors to carry min/max,
// so this never decodes binary buffers. Node TRS is ignored — see
// design.md for the rationale and the override-file escape hatch.
func readSourceFootprint(path string) (Footprint, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Footprint{}, fmt.Errorf("read GLB: %w", err)
	}
	if len(data) < 12 {
		return Footprint{}, fmt.Errorf("file too small for GLB header")
	}
	magic := uint32(data[0]) | uint32(data[1])<<8 | uint32(data[2])<<16 | uint32(data[3])<<24
	if magic != 0x46546C67 {
		return Footprint{}, fmt.Errorf("not a GLB file")
	}
	if len(data) < 20 {
		return Footprint{}, fmt.Errorf("file too small for chunk header")
	}
	chunkLen := uint32(data[12]) | uint32(data[13])<<8 | uint32(data[14])<<16 | uint32(data[15])<<24
	chunkType := uint32(data[16]) | uint32(data[17])<<8 | uint32(data[18])<<16 | uint32(data[19])<<24
	if chunkType != 0x4E4F534A {
		return Footprint{}, fmt.Errorf("expected JSON chunk, got 0x%08X", chunkType)
	}
	if len(data) < int(20+chunkLen) {
		return Footprint{}, fmt.Errorf("JSON chunk truncated")
	}
	jsonData := data[20 : 20+chunkLen]

	var gltf struct {
		Accessors []struct {
			Min []float64 `json:"min"`
			Max []float64 `json:"max"`
		} `json:"accessors"`
		Meshes []struct {
			Primitives []struct {
				Attributes struct {
					POSITION int `json:"POSITION"`
				} `json:"attributes"`
			} `json:"primitives"`
		} `json:"meshes"`
	}
	if err := json.Unmarshal(jsonData, &gltf); err != nil {
		return Footprint{}, fmt.Errorf("parse glTF JSON: %w", err)
	}

	minX, minY, minZ := math.Inf(1), math.Inf(1), math.Inf(1)
	maxX, maxY, maxZ := math.Inf(-1), math.Inf(-1), math.Inf(-1)
	seen := 0
	for mi, mesh := range gltf.Meshes {
		for pi, prim := range mesh.Primitives {
			idx := prim.Attributes.POSITION
			if idx < 0 || idx >= len(gltf.Accessors) {
				return Footprint{}, fmt.Errorf("mesh[%d].primitive[%d]: POSITION accessor index %d out of range", mi, pi, idx)
			}
			a := gltf.Accessors[idx]
			if len(a.Min) < 3 || len(a.Max) < 3 {
				return Footprint{}, fmt.Errorf("mesh[%d].primitive[%d]: POSITION accessor missing 3-component min/max", mi, pi)
			}
			if a.Min[0] < minX {
				minX = a.Min[0]
			}
			if a.Min[1] < minY {
				minY = a.Min[1]
			}
			if a.Min[2] < minZ {
				minZ = a.Min[2]
			}
			if a.Max[0] > maxX {
				maxX = a.Max[0]
			}
			if a.Max[1] > maxY {
				maxY = a.Max[1]
			}
			if a.Max[2] > maxZ {
				maxZ = a.Max[2]
			}
			seen++
		}
	}
	if seen == 0 {
		return Footprint{}, fmt.Errorf("no POSITION accessors with min/max found")
	}
	width := maxX - minX
	height := maxY - minY
	depth := maxZ - minZ
	if height <= 0 || width <= 0 || depth <= 0 {
		return Footprint{}, fmt.Errorf("degenerate mesh AABB: width=%g height=%g depth=%g", width, height, depth)
	}
	radius := width
	if depth > radius {
		radius = depth
	}
	radius /= 2
	return Footprint{
		CanopyRadiusM: radius,
		HeightM:       height,
	}, nil
}
