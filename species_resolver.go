package main

// species_resolver.go is the T-012-01 hash → species resolver.
//
// It walks a six-tier fallback chain so the operator running
// `glb-optimizer pack <id>` against an already-baked tree can produce
// a sensibly-named asset pack without hand-authoring per-asset
// `_meta.json` sidecars. Tier order, highest priority first:
//
//   1. CLI override   — `--species` / `--common-name` flags on `pack`
//   2. Mapping file   — entry from `pack-all --mapping <file.json>`
//   3. _meta.json     — `outputs/{id}_meta.json` (legacy escape hatch)
//   4. FileStore      — in-memory `FileRecord.Filename` from upload
//   5. Upload manifest— `~/.glb-optimizer/uploads.jsonl` (T-012-04)
//   6. Hash fallback  — derived from id; logs WARNING
//
// Every tier's output is normalised through deriveSpeciesFromName so
// hand-edited mapping values like `"Achillea Millefolium"` still
// produce a valid v1 species slug. The resolver never returns a
// non-nil error: friction is what this ticket exists to remove, and
// the hash-fallback tier guarantees a usable identity for any id.

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// SpeciesIdentity is the resolved (species, common_name) tuple.
type SpeciesIdentity struct {
	Species    string
	CommonName string
}

// ResolverSource tags WHICH tier produced an identity. Returned to
// the caller so the operator can audit which fallback fired.
type ResolverSource int

const (
	SourceUnknown ResolverSource = iota
	SourceCLIOverride
	SourceMappingFile
	SourceMetaJSON
	SourceFileStore
	SourceUploadManifest
	SourceContentHash
)

// String renders a ResolverSource as a short kebab-case label that
// matches the documentation in design.md and the per-tier log lines.
func (s ResolverSource) String() string {
	switch s {
	case SourceCLIOverride:
		return "cli-override"
	case SourceMappingFile:
		return "mapping-file"
	case SourceMetaJSON:
		return "meta-json"
	case SourceFileStore:
		return "file-store"
	case SourceUploadManifest:
		return "upload-manifest"
	case SourceContentHash:
		return "content-hash"
	default:
		return "unknown"
	}
}

// ResolverOptions carries the per-call hints supplied by the CLI
// driver. The HTTP path passes a zero value, in which case the
// resolver behaves like the pre-T-012-01 capture: sidecar > store >
// fallback.
type ResolverOptions struct {
	// CLISpecies / CLICommonName: per-asset flags from
	// `glb-optimizer pack <id> --species ... --common-name ...`. Both
	// must be set together; if only one is non-empty the override is
	// silently skipped (the CLI parser is responsible for the
	// both-or-neither check).
	CLISpecies    string
	CLICommonName string

	// Mapping is the parsed contents of `pack-all --mapping <f.json>`,
	// keyed by asset id. nil → tier disabled.
	Mapping map[string]SpeciesIdentity

	// UploadManifestPath is an explicit override for the JSONL
	// manifest path. Empty string → ~/.glb-optimizer/uploads.jsonl.
	// Tests use this to point at a tempdir.
	UploadManifestPath string
}

// hexHashRe matches the 32-hex-char content hashes the upload
// pipeline produces. Used by the hash-fallback tier to render a
// short `species_<first8>` slug rather than a 32-char monstrosity.
var hexHashRe = regexp.MustCompile(`^[0-9a-f]{16,}$`)

// captureOverride is the on-disk shape of outputs/{id}_meta.json. It
// lives here (not in pack_meta_capture.go) because the meta-json
// tier is now an implementation detail of the resolver. The file is
// intentionally minimal in v1 — only species and common_name are
// honored. Any other keys are ignored.
type captureOverride struct {
	Species    string `json:"species"`
	CommonName string `json:"common_name"`
}

// uploadManifestEntry is one record in <workDir>/uploads.jsonl. The
// schema is owned by T-012-04 (see upload_manifest.go for the writer
// and the canonical UploadManifestEntry type). The resolver
// deliberately decodes a narrow subset — only the two fields needed
// to recover an identity — so future schema additions to the writer
// remain backwards compatible here.
type uploadManifestEntry struct {
	Hash             string `json:"hash"`
	OriginalFilename string `json:"original_filename"`
}

// ResolveSpeciesIdentity walks the six-tier chain and returns the
// first identity whose normalised species id is non-empty. The
// content-hash tier is the safety net and always succeeds, so the
// returned error is always nil today; the signature reserves the
// option for future strict modes (e.g., a `--strict` flag that
// promotes hash fallback to an error).
func ResolveSpeciesIdentity(
	id, outputsDir string,
	store *FileStore,
	opts ResolverOptions,
) (SpeciesIdentity, ResolverSource, error) {
	// Tier 1 — CLI override.
	if opts.CLISpecies != "" && opts.CLICommonName != "" {
		ident, ok := normalizeIdentity(opts.CLISpecies, opts.CLICommonName)
		if ok {
			return ident, SourceCLIOverride, nil
		}
		log.Printf("species_resolver: %s: cli override %q failed normalisation, falling through",
			id, opts.CLISpecies)
	}

	// Tier 2 — mapping file.
	if entry, ok := opts.Mapping[id]; ok {
		ident, ok := normalizeIdentity(entry.Species, entry.CommonName)
		if ok {
			return ident, SourceMappingFile, nil
		}
		log.Printf("species_resolver: %s: mapping entry %q failed normalisation, falling through",
			id, entry.Species)
	}

	// Tier 3 — _meta.json sidecar.
	overridePath := filepath.Join(outputsDir, id+"_meta.json")
	override, err := loadCaptureOverride(overridePath)
	if err != nil {
		// Sidecar exists but is malformed: log and fall through.
		// (Older code returned an error here. The new resolver is
		// permissive — a typo in a hand-edited sidecar should not
		// block the demo pack pipeline.)
		log.Printf("species_resolver: %s: %s malformed: %v, falling through",
			id, overridePath, err)
	} else if override.Species != "" {
		ident, ok := normalizeIdentity(override.Species, override.CommonName)
		if ok {
			return ident, SourceMetaJSON, nil
		}
		log.Printf("species_resolver: %s: sidecar species %q failed normalisation, falling through",
			id, override.Species)
	}

	// Tier 4 — FileStore in-memory filename. Filtered against the
	// post-restart sentinel `{id}.glb` that scanExistingFiles writes
	// when the original upload filename is no longer known.
	if store != nil {
		if rec, ok := store.Get(id); ok && rec.Filename != "" && rec.Filename != id+".glb" {
			ident, ok := identityFromFilename(rec.Filename)
			if ok {
				return ident, SourceFileStore, nil
			}
		}
	}

	// Tier 5 — upload manifest. Opportunistic: T-012-04 will create
	// the file; until then this tier is a no-op.
	if filename, found := lookupUploadManifest(opts.UploadManifestPath, id); found {
		ident, ok := identityFromFilename(filename)
		if ok {
			return ident, SourceUploadManifest, nil
		}
		log.Printf("species_resolver: %s: upload manifest filename %q failed normalisation, falling through",
			id, filename)
	}

	// Tier 6 — hash fallback. Always succeeds.
	ident := hashFallbackIdentity(id)
	log.Printf("species_resolver: %s: WARNING no provenance found, falling back to %q (%q); "+
		"author outputs/%s_meta.json or pass --species to override",
		id, ident.Species, ident.CommonName, id)
	return ident, SourceContentHash, nil
}

// normalizeIdentity runs species through deriveSpeciesFromName and
// returns ok=false if the result is empty. common_name is left
// untouched if non-empty; otherwise it is title-cased from the
// normalised species id.
func normalizeIdentity(species, common string) (SpeciesIdentity, bool) {
	slug := deriveSpeciesFromName(species)
	if slug == "" {
		return SpeciesIdentity{}, false
	}
	if strings.TrimSpace(common) == "" {
		common = titleCaseSpecies(slug)
	}
	return SpeciesIdentity{Species: slug, CommonName: common}, true
}

// identityFromFilename converts an upload-time basename like
// `Achillea Millefolium.glb` into the corresponding identity. The
// extension is stripped by deriveSpeciesFromName, the common name is
// reconstructed via titleCaseSpecies of the slug.
func identityFromFilename(filename string) (SpeciesIdentity, bool) {
	slug := deriveSpeciesFromName(filename)
	if slug == "" {
		return SpeciesIdentity{}, false
	}
	return SpeciesIdentity{
		Species:    slug,
		CommonName: titleCaseSpecies(slug),
	}, true
}

// hashFallbackIdentity is the safety-net tier. For real 16+-char
// lowercase hex hashes it produces a short `species_<first8>` slug
// and an "Unknown Species (...)" common name so the operator can
// trace the row back to a hash. For non-hex ids (e.g. test fixtures
// whose ids are already valid slugs), it falls back to deriving a
// slug from the id itself, which preserves the long-standing
// "id-as-slug" behaviour the CLI tests rely on.
func hashFallbackIdentity(id string) SpeciesIdentity {
	if hexHashRe.MatchString(id) {
		prefix := id
		if len(prefix) > 8 {
			prefix = prefix[:8]
		}
		return SpeciesIdentity{
			Species:    "species_" + prefix,
			CommonName: fmt.Sprintf("Unknown Species (%s)", prefix),
		}
	}
	slug := deriveSpeciesFromName(id)
	if slug == "" {
		return SpeciesIdentity{
			Species:    "species_unknown",
			CommonName: "Unknown Species",
		}
	}
	return SpeciesIdentity{
		Species:    slug,
		CommonName: titleCaseSpecies(slug),
	}
}

// loadCaptureOverride reads the optional outputs/{id}_meta.json. A
// missing file is not an error; it returns the zero captureOverride.
// A present-but-malformed file IS an error so the caller can decide
// whether to log-and-fall-through or hard-fail.
func loadCaptureOverride(path string) (captureOverride, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return captureOverride{}, nil
		}
		return captureOverride{}, err
	}
	var ov captureOverride
	if err := json.Unmarshal(data, &ov); err != nil {
		return captureOverride{}, fmt.Errorf("decode: %w", err)
	}
	return ov, nil
}

// LoadMappingFile parses the JSON object format documented in the
// T-012-01 ticket and returns a map ready to drop into
// ResolverOptions.Mapping. Empty path → empty map (NOT an error) so
// `pack-all` without a `--mapping` flag is a no-op rather than a
// special case at the call site. Missing file IS an error so the
// operator who passed `--mapping wrongname.json` finds out fast.
func LoadMappingFile(path string) (map[string]SpeciesIdentity, error) {
	if path == "" {
		return nil, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("load mapping file %s: %w", path, err)
	}
	raw := map[string]struct {
		Species    string `json:"species"`
		CommonName string `json:"common_name"`
	}{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("parse mapping file %s: %w", path, err)
	}
	out := make(map[string]SpeciesIdentity, len(raw))
	for k, v := range raw {
		out[k] = SpeciesIdentity{Species: v.Species, CommonName: v.CommonName}
	}
	return out, nil
}

// lookupUploadManifest scans an append-only JSONL manifest for the
// last entry whose ID matches the requested asset id. The "last"
// rule means a re-upload of the same hash with a different name
// shadows earlier entries — useful when a file is renamed and
// re-dropped through the UI. Empty path → ~/.glb-optimizer/uploads.jsonl.
// Returns ("", false) on any IO error or missing file: the resolver
// chain treats absence as "tier disabled".
func lookupUploadManifest(path, id string) (string, bool) {
	if path == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", false
		}
		path = filepath.Join(home, ".glb-optimizer", "uploads.jsonl")
	}
	f, err := os.Open(path)
	if err != nil {
		return "", false
	}
	defer f.Close()

	var found string
	scanner := bufio.NewScanner(f)
	// Allow long lines: a future schema might include extra fields.
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var entry uploadManifestEntry
		if err := json.Unmarshal(line, &entry); err != nil {
			continue
		}
		if entry.Hash == id && entry.OriginalFilename != "" {
			found = entry.OriginalFilename
		}
	}
	if found == "" {
		return "", false
	}
	return found, true
}
