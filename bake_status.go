package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
)

// knownSuffixes lists the intermediate-file suffixes that
// discoverAllIDs strips to recover the base content-hash. Order
// matters: longer suffixes must appear before their prefixes so
// _billboard_tilted.glb is stripped before _billboard.glb.
var knownSuffixes = []string{
	"_billboard_tilted.glb",
	"_billboard.glb",
	"_volumetric.glb",
	"_lod0.glb",
	"_lod1.glb",
	"_lod2.glb",
	"_lod3.glb",
	"_bake.json",
	"_reference.png",
	".glb",
}

// discoverAllIDs walks outputsDir and returns every unique
// content-hash prefix, sorted. Unlike discoverPackableIDs (which
// requires _billboard.glb), this returns assets at any stage of
// the bake pipeline.
func discoverAllIDs(outputsDir string) ([]string, error) {
	entries, err := os.ReadDir(outputsDir)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]bool)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		for _, suffix := range knownSuffixes {
			if strings.HasSuffix(name, suffix) {
				id := strings.TrimSuffix(name, suffix)
				if id != "" {
					seen[id] = true
				}
				break
			}
		}
	}
	ids := make([]string, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids, nil
}

// checkIntermediates returns the existence of the three key
// intermediates for a given asset id.
func checkIntermediates(outputsDir, id string) (billboard, tilted, dome bool) {
	billboard = fileExists(filepath.Join(outputsDir, id+"_billboard.glb"))
	tilted = fileExists(filepath.Join(outputsDir, id+"_billboard_tilted.glb"))
	dome = fileExists(filepath.Join(outputsDir, id+"_volumetric.glb"))
	return
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

// runBakeStatusCmd implements `glb-optimizer bake-status`. It
// prints a table of all assets in outputs/ with their
// intermediate completeness and pack status.
func runBakeStatusCmd(args []string) int {
	fs := flag.NewFlagSet("bake-status", flag.ContinueOnError)
	dirFlag := fs.String("dir", "", "Working directory (default: ~/.glb-optimizer)")
	if err := fs.Parse(args); err != nil {
		return 2
	}

	workDir, err := resolveWorkdir(*dirFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	originalsDir := filepath.Join(workDir, "originals")
	outputsDir := filepath.Join(workDir, "outputs")
	settingsDir := filepath.Join(workDir, "settings")
	acceptedDir := filepath.Join(workDir, "accepted")
	distDir := filepath.Join(workDir, DistPlantsDir)

	ids, err := discoverAllIDs(outputsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "discover: %v\n", err)
		return 1
	}
	if len(ids) == 0 {
		fmt.Println("no assets found in outputs/")
		return 0
	}

	store := NewFileStore()
	scanExistingFiles(store, originalsDir, outputsDir, settingsDir, acceptedDir)

	tw := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SPECIES\tBILLBOARD\tTILTED\tDOME\tPACK")

	packedCount := 0
	for _, id := range ids {
		billboard, tilted, dome := checkIntermediates(outputsDir, id)

		// Resolve species name for display
		species := id[:min(8, len(id))]
		ident, _, resolveErr := ResolveSpeciesIdentity(id, outputsDir, store, ResolverOptions{})
		if resolveErr == nil {
			species = ident.Species
		}

		// Check if pack exists
		hasPack := fileExists(filepath.Join(distDir, species+".glb"))

		if hasPack {
			packedCount++
		}

		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n",
			species,
			yesNo(billboard),
			yesNo(tilted),
			yesNo(dome),
			yesNo(hasPack),
		)
	}
	tw.Flush()
	fmt.Fprintf(os.Stdout, "TOTAL: %d assets, %d packed\n", len(ids), packedCount)
	return 0
}
