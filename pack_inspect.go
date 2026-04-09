package main

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// PackInspectReport is the structured outcome of a pack-inspect run.
// It is the source of truth for both the JSON output (--json) and the
// rendered human-readable output. Field declaration order controls
// JSON key order.
type PackInspectReport struct {
	Path       string         `json:"path"`
	Size       int64          `json:"size_bytes"`
	SizeHuman  string         `json:"size_human"`
	SHA256     string         `json:"sha256"`
	Format     string         `json:"format"`
	BakeID     string         `json:"bake_id"`
	Meta       *PackMeta      `json:"metadata,omitempty"`
	Variants   VariantSummary `json:"variants"`
	Validation string         `json:"validation"`
	Valid      bool           `json:"valid"`
}

// VariantSummary holds the per-group node counts and average byte
// sizes derived from the pack_root → group → leaf node tree built by
// CombinePack. Optional groups (top, tilted, dome) are nil when the
// corresponding intermediate was not baked.
type VariantSummary struct {
	Side   *VariantGroup `json:"view_side,omitempty"`
	Top    *VariantGroup `json:"view_top,omitempty"`
	Tilted *VariantGroup `json:"view_tilted,omitempty"`
	Dome   *VariantGroup `json:"view_dome,omitempty"`
}

// VariantGroup is the per-group leaf count and average mesh+index
// byte attribution. Image bytes are excluded so the number reflects
// what the bake actually controls per variant.
type VariantGroup struct {
	Count    int   `json:"count"`
	AvgBytes int64 `json:"avg_bytes"`
}

// InspectPack reads a pack file and returns a populated report. It
// returns a non-nil error only on I/O failure or unparseable GLB;
// schema-validation failures are surfaced via report.Valid=false and
// report.Validation containing the error message so callers can still
// render the rest of the report.
func InspectPack(path string) (*PackInspectReport, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read pack: %w", err)
	}

	sum := sha256.Sum256(raw)

	rep := &PackInspectReport{
		Path:      path,
		Size:      int64(len(raw)),
		SizeHuman: humanBytes(int64(len(raw))),
		SHA256:    fmt.Sprintf("%x", sum),
		Format:    "Pack v1",
	}

	doc, _, err := readGLB(raw)
	if err != nil {
		return nil, fmt.Errorf("parse glb: %w", err)
	}

	meta, metaErr := extractPackMeta(doc)
	if metaErr != nil {
		rep.Validation = metaErr.Error()
		rep.Valid = false
	} else {
		rep.Meta = meta
		rep.BakeID = meta.BakeID
		rep.Validation = "OK"
		rep.Valid = true
	}

	rep.Variants = summarizeVariants(doc)
	return rep, nil
}

// extractPackMeta pulls scenes[scene].extras.plantastic out of a
// parsed gltfDoc and runs it through ParsePackMeta. Returns nil meta
// + descriptive error if the extras block is absent or malformed.
func extractPackMeta(doc *gltfDoc) (*PackMeta, error) {
	if len(doc.Scenes) == 0 {
		return nil, fmt.Errorf("pack has no scenes")
	}
	sceneIdx := doc.Scene
	if sceneIdx < 0 || sceneIdx >= len(doc.Scenes) {
		sceneIdx = 0
	}
	scene := doc.Scenes[sceneIdx]
	if scene.Extras == nil {
		return nil, fmt.Errorf("scene has no extras")
	}
	rawExtras, ok := scene.Extras["plantastic"]
	if !ok {
		return nil, fmt.Errorf("scene.extras.plantastic missing — not a Pack v1 file")
	}
	jsonBytes, err := json.Marshal(rawExtras)
	if err != nil {
		return nil, fmt.Errorf("re-marshal extras: %w", err)
	}
	meta, err := ParsePackMeta(jsonBytes)
	if err != nil {
		return nil, err
	}
	return &meta, nil
}

// summarizeVariants walks the pack_root → group → leaf node tree and
// builds a VariantSummary by group name. Side is required by the
// CombinePack invariant; the rest are optional and stay nil if not
// emitted by the bake.
func summarizeVariants(doc *gltfDoc) VariantSummary {
	var s VariantSummary
	if len(doc.Scenes) == 0 || len(doc.Nodes) == 0 {
		return s
	}
	sceneIdx := doc.Scene
	if sceneIdx < 0 || sceneIdx >= len(doc.Scenes) {
		sceneIdx = 0
	}
	scene := doc.Scenes[sceneIdx]
	if len(scene.Nodes) == 0 {
		return s
	}
	rootIdx := scene.Nodes[0]
	if rootIdx < 0 || rootIdx >= len(doc.Nodes) {
		return s
	}
	root := doc.Nodes[rootIdx]

	for _, groupIdx := range root.Children {
		if groupIdx < 0 || groupIdx >= len(doc.Nodes) {
			continue
		}
		group := doc.Nodes[groupIdx]
		leaves := collectLeafMeshes(doc, group)
		if len(leaves) == 0 {
			continue
		}
		bytes := variantBytes(doc, leaves)
		var avg int64
		if len(leaves) > 0 {
			avg = bytes / int64(len(leaves))
		}
		vg := &VariantGroup{Count: len(leaves), AvgBytes: avg}
		switch group.Name {
		case "view_side":
			s.Side = vg
		case "view_top":
			s.Top = vg
		case "view_tilted":
			s.Tilted = vg
		case "view_dome":
			s.Dome = vg
		}
	}
	return s
}

// collectLeafMeshes returns the mesh indices of every direct-child
// leaf node under group. A "leaf" here means a node that has a Mesh
// pointer set; combine routes meshes to leaves directly under the
// group, so a single level of recursion is sufficient.
func collectLeafMeshes(doc *gltfDoc, group gltfNode) []int {
	var out []int
	for _, childIdx := range group.Children {
		if childIdx < 0 || childIdx >= len(doc.Nodes) {
			continue
		}
		child := doc.Nodes[childIdx]
		if child.Mesh != nil {
			out = append(out, *child.Mesh)
		}
	}
	return out
}

// variantBytes sums the unique bufferView byte lengths referenced by
// every primitive of every mesh in leafMeshes. Image-bound bufferViews
// are excluded so the per-variant number reflects mesh+index data
// only — what the bake actually controls per variant.
func variantBytes(doc *gltfDoc, leafMeshes []int) int64 {
	imageBVs := map[int]bool{}
	for _, img := range doc.Images {
		if img.BufferView != nil {
			imageBVs[*img.BufferView] = true
		}
	}

	seenBV := map[int]bool{}
	var total int64
	for _, mi := range leafMeshes {
		if mi < 0 || mi >= len(doc.Meshes) {
			continue
		}
		mesh := doc.Meshes[mi]
		for _, p := range mesh.Primitives {
			for _, ai := range p.Attributes {
				addAccessorBV(doc, ai, seenBV, imageBVs, &total)
			}
			if p.Indices != nil {
				addAccessorBV(doc, *p.Indices, seenBV, imageBVs, &total)
			}
		}
	}
	return total
}

// addAccessorBV resolves accessor → bufferView and adds its byte
// length once if not already counted and not an image payload.
func addAccessorBV(doc *gltfDoc, accessorIdx int, seen, imageBVs map[int]bool, total *int64) {
	if accessorIdx < 0 || accessorIdx >= len(doc.Accessors) {
		return
	}
	a := doc.Accessors[accessorIdx]
	if a.BufferView == nil {
		return
	}
	bvIdx := *a.BufferView
	if seen[bvIdx] || imageBVs[bvIdx] {
		return
	}
	if bvIdx < 0 || bvIdx >= len(doc.BufferViews) {
		return
	}
	seen[bvIdx] = true
	*total += int64(doc.BufferViews[bvIdx].ByteLength)
}

// renderHuman writes the multi-block terminal-friendly layout shown
// in the ticket AC. Section order is fixed: header, metadata,
// variants, validation. Group rows print in fixed order: side, top,
// tilted, dome. Optional groups render as "(absent)" rather than
// being omitted, so the operator can spot a missing flavour at a
// glance.
func renderHuman(w io.Writer, r *PackInspectReport) {
	base := filepath.Base(r.Path)
	fmt.Fprintf(w, "pack: %s\n", base)
	fmt.Fprintf(w, "  path:        %s\n", r.Path)
	fmt.Fprintf(w, "  size:        %s (%d bytes)\n", r.SizeHuman, r.Size)
	fmt.Fprintf(w, "  sha256:      %s\n", r.SHA256)
	fmt.Fprintf(w, "  format:      %s\n", r.Format)
	fmt.Fprintf(w, "  bake_id:     %s\n", r.BakeID)
	fmt.Fprintln(w)

	if r.Meta != nil {
		fmt.Fprintln(w, "metadata")
		fmt.Fprintf(w, "  species:           %s\n", r.Meta.Species)
		fmt.Fprintf(w, "  common_name:       %s\n", r.Meta.CommonName)
		fmt.Fprintf(w, "  canopy_radius_m:   %g\n", r.Meta.Footprint.CanopyRadiusM)
		fmt.Fprintf(w, "  height_m:          %g\n", r.Meta.Footprint.HeightM)
		fmt.Fprintf(w, "  fade.low_start:    %g\n", r.Meta.Fade.LowStart)
		fmt.Fprintf(w, "  fade.low_end:      %g\n", r.Meta.Fade.LowEnd)
		fmt.Fprintf(w, "  fade.high_start:   %g\n", r.Meta.Fade.HighStart)
		fmt.Fprintln(w)
	} else {
		fmt.Fprintln(w, "metadata: (unavailable — see validation)")
		fmt.Fprintln(w)
	}

	fmt.Fprintln(w, "variants")
	fmt.Fprintf(w, "  view_side:    %s\n", formatVariantRow(r.Variants.Side, "variants"))
	fmt.Fprintf(w, "  view_top:     %s\n", formatTopRow(r.Variants.Top))
	fmt.Fprintf(w, "  view_tilted:  %s\n", formatVariantRow(r.Variants.Tilted, "variants"))
	fmt.Fprintf(w, "  view_dome:    %s\n", formatVariantRow(r.Variants.Dome, "slices"))
	fmt.Fprintln(w)

	fmt.Fprintf(w, "validation: %s\n", r.Validation)
}

// formatVariantRow renders one variant group row. unit is the noun
// for the count ("variants" or "slices"). Returns "(absent)" when
// the group was not present in the pack.
func formatVariantRow(g *VariantGroup, unit string) string {
	if g == nil {
		return "(absent)"
	}
	return fmt.Sprintf("%d %s × avg %s", g.Count, unit, humanBytes(g.AvgBytes))
}

// formatTopRow is the irregular case: view_top is a single quad in
// the canonical CombinePack tree, so we say "1 quad × <size>" rather
// than "1 variants × avg <size>".
func formatTopRow(g *VariantGroup) string {
	if g == nil {
		return "(absent)"
	}
	if g.Count == 1 {
		return fmt.Sprintf("1 quad × %s", humanBytes(g.AvgBytes))
	}
	return fmt.Sprintf("%d quads × avg %s", g.Count, humanBytes(g.AvgBytes))
}

// renderJSON writes a single indented JSON document. Indent matches
// `jq .` so a human can scan the output without piping. The struct
// shape is the public contract — change with care.
func renderJSON(w io.Writer, r *PackInspectReport) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}

// renderQuiet writes a single space-separated line for shell pipes:
//
//	<sha256> <size_bytes> <OK|FAIL>
//
// Always one line, always three fields, no JSON, no trailing comma.
func renderQuiet(w io.Writer, r *PackInspectReport) {
	status := "FAIL"
	if r.Valid {
		status = "OK"
	}
	fmt.Fprintf(w, "%s %d %s\n", r.SHA256, r.Size, status)
}

// resolveInspectTarget classifies arg as either a species id (matches
// speciesRe → look up dist/plants/{id}.glb) or a path. ~ expansion is
// applied to path arguments so operators can paste shell-style paths
// directly. Returns an error with a hint about --dir when a species
// id resolves to a missing file.
func resolveInspectTarget(arg, workDir string) (string, error) {
	if speciesRe.MatchString(arg) {
		path := filepath.Join(workDir, DistPlantsDir, arg+".glb")
		if _, err := os.Stat(path); err != nil {
			return "", fmt.Errorf("no pack for species %q at %s (use --dir to override workdir)", arg, path)
		}
		return path, nil
	}

	path := arg
	if strings.HasPrefix(path, "~/") {
		if home, err := os.UserHomeDir(); err == nil {
			path = filepath.Join(home, path[2:])
		}
	}
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("no such file: %s", path)
	}
	return path, nil
}

// runPackInspectCmd implements `glb-optimizer pack-inspect <arg>`.
// Returns 0 on a valid Pack v1 file, 1 on any read/parse/validation
// failure, 2 on flag parse failure or mutually-exclusive flags.
func runPackInspectCmd(args []string) int {
	return runPackInspectCmdW(args, os.Stdout)
}

// runPackInspectCmdW is the test seam — the exported entry point is
// a 1-line wrapper that passes os.Stdout. Tests pass a buffer.
func runPackInspectCmdW(args []string, w io.Writer) int {
	fs := flag.NewFlagSet("pack-inspect", flag.ContinueOnError)
	dirFlag := fs.String("dir", "", "Working directory (default: ~/.glb-optimizer)")
	jsonFlag := fs.Bool("json", false, "Emit JSON output for scripting")
	quietFlag := fs.Bool("quiet", false, "Emit one-line sha256+size+status for shell pipelines")
	fs.SetOutput(os.Stderr)
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: glb-optimizer pack-inspect [--dir PATH] [--json|--quiet] <species_id_or_path>")
		return 2
	}
	if *jsonFlag && *quietFlag {
		fmt.Fprintln(os.Stderr, "--json and --quiet are mutually exclusive")
		return 2
	}
	arg := fs.Arg(0)

	workDir, err := resolveWorkdir(*dirFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	path, err := resolveInspectTarget(arg, workDir)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	report, err := InspectPack(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}

	switch {
	case *jsonFlag:
		if err := renderJSON(w, report); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	case *quietFlag:
		renderQuiet(w, report)
	default:
		renderHuman(w, report)
	}

	if !report.Valid {
		return 1
	}
	return 0
}
