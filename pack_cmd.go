package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
)

// resolveWorkdir reproduces main()'s workdir resolution. If
// dirFlag is empty, it falls back to ~/.glb-optimizer. The
// returned path is created on disk along with every subdirectory
// the server expects, so a fresh laptop running just `pack-all`
// against an empty home does not crash on a missing dist root.
func resolveWorkdir(dirFlag string) (string, error) {
	workDir := dirFlag
	if workDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot determine home directory: %w", err)
		}
		workDir = filepath.Join(home, ".glb-optimizer")
	}
	subdirs := []string{
		filepath.Join(workDir, "originals"),
		filepath.Join(workDir, "outputs"),
		filepath.Join(workDir, "settings"),
		filepath.Join(workDir, "tuning"),
		filepath.Join(workDir, "profiles"),
		filepath.Join(workDir, "accepted"),
		filepath.Join(workDir, "accepted", "thumbs"),
		filepath.Join(workDir, DistPlantsDir),
	}
	for _, d := range subdirs {
		if err := os.MkdirAll(d, 0755); err != nil {
			return "", fmt.Errorf("create %s: %w", d, err)
		}
	}
	return workDir, nil
}

// discoverPackableIDs walks outputsDir for files matching the
// {id}_billboard.glb suffix (the only required pack input) and
// returns the recovered ids in deterministic sorted order. The
// _billboard_tilted.glb suffix would be a false positive for a
// naive HasSuffix check, so it is filtered out explicitly.
func discoverPackableIDs(outputsDir string) ([]string, error) {
	entries, err := os.ReadDir(outputsDir)
	if err != nil {
		return nil, err
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if !strings.HasSuffix(name, "_billboard.glb") {
			continue
		}
		if strings.HasSuffix(name, "_billboard_tilted.glb") {
			continue
		}
		ids = append(ids, strings.TrimSuffix(name, "_billboard.glb"))
	}
	sort.Strings(ids)
	return ids, nil
}

// printPackSummary writes a tabwriter-aligned table of pack
// outcomes followed by a single TOTAL line. Failure rows get an
// indented second line with the (truncated) error message so the
// operator does not have to re-run with --verbose to see what
// went wrong.
func printPackSummary(w io.Writer, results []PackResult) {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "SPECIES\tSIZE\tTILTED\tDOME\tSTATUS")

	okCount := 0
	failCount := 0
	for _, r := range results {
		species := r.Species
		size := "-"
		tilted := "-"
		dome := "-"
		if r.Status == "ok" {
			size = humanBytes(r.Size)
			tilted = yesNo(r.HasTilted)
			dome = yesNo(r.HasDome)
			okCount++
		} else {
			// Even failed rows can carry tilted/dome info if the
			// optional reads succeeded before the failure.
			tilted = yesNo(r.HasTilted)
			dome = yesNo(r.HasDome)
			failCount++
		}
		if species == "" {
			species = r.ID
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\n", species, size, tilted, dome, r.Status)
		if r.Status != "ok" && r.Err != nil {
			msg := truncateOneLine(r.Err.Error(), 80)
			fmt.Fprintf(tw, "  \t \t \t \t%s\n", msg)
		}
	}
	tw.Flush()
	fmt.Fprintf(w, "TOTAL: %d packs, %d ok, %d failed\n",
		len(results), okCount, failCount)
}

func yesNo(b bool) string {
	if b {
		return "yes"
	}
	return "no"
}

// truncateOneLine collapses newlines and clips long messages so
// the table stays one row per error.
func truncateOneLine(s string, max int) string {
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(s)
	if len(s) > max {
		return s[:max-1] + "…"
	}
	return s
}

// runPackCmd implements `glb-optimizer pack <id>`. It packs a
// single asset by id and prints a one-row summary. Returns 0 on
// success, 1 on any non-ok status (including missing-source).
//
// T-012-01: optional --species / --common-name flags thread a CLI
// override to the species resolver as the highest-priority tier.
// Both flags must be provided together; passing one without the
// other is an error so a typo doesn't silently produce a wrong slug.
func runPackCmd(args []string) int {
	fs := flag.NewFlagSet("pack", flag.ContinueOnError)
	dirFlag := fs.String("dir", "", "Working directory (default: ~/.glb-optimizer)")
	speciesFlag := fs.String("species", "", "Override species id (must be provided with --common-name)")
	commonFlag := fs.String("common-name", "", "Override common name (must be provided with --species)")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: glb-optimizer pack [--dir PATH] [--species SLUG --common-name NAME] <id>")
		return 2
	}
	if (*speciesFlag == "") != (*commonFlag == "") {
		fmt.Fprintln(os.Stderr, "error: --species and --common-name must be provided together")
		return 2
	}
	id := fs.Arg(0)

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

	store := NewFileStore()
	scanExistingFiles(store, originalsDir, outputsDir, settingsDir, acceptedDir)

	opts := ResolverOptions{
		CLISpecies:    *speciesFlag,
		CLICommonName: *commonFlag,
	}
	res := RunPack(id, originalsDir, settingsDir, outputsDir, distDir, store, opts)
	printPackSummary(os.Stdout, []PackResult{res})
	if res.Status != "ok" {
		return 1
	}
	return 0
}

// runPackAllCmd implements `glb-optimizer pack-all`. It walks
// outputsDir for every asset that has a baked side billboard,
// packs each one sequentially, prints a summary table, and
// returns 0 iff every row is ok.
func runPackAllCmd(args []string) int {
	fs := flag.NewFlagSet("pack-all", flag.ContinueOnError)
	dirFlag := fs.String("dir", "", "Working directory (default: ~/.glb-optimizer)")
	mappingFlag := fs.String("mapping", "", "JSON file mapping asset id → {species, common_name}")
	cleanFlag := fs.Bool("clean", false, "After successful packing, remove stale packs from dist/plants/")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: glb-optimizer pack-all [--dir PATH] [--mapping FILE] [--clean]")
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

	store := NewFileStore()
	scanExistingFiles(store, originalsDir, outputsDir, settingsDir, acceptedDir)

	mapping, err := LoadMappingFile(*mappingFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	opts := ResolverOptions{Mapping: mapping}

	ids, err := discoverPackableIDs(outputsDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "discover: %v\n", err)
		return 1
	}

	results := make([]PackResult, 0, len(ids))
	for _, id := range ids {
		results = append(results, RunPack(id, originalsDir, settingsDir, outputsDir, distDir, store, opts))
	}
	printPackSummary(os.Stdout, results)
	anyFailed := false
	for _, r := range results {
		if r.Status != "ok" {
			anyFailed = true
			break
		}
	}
	if *cleanFlag {
		fmt.Fprintln(os.Stdout, "")
		if anyFailed {
			fmt.Fprintln(os.Stdout, "Skipped stale-pack cleanup: pack-all had failures.")
		} else {
			fmt.Fprintln(os.Stdout, "Cleaned stale packs:")
			stale, err := IdentifyStalePacks(distDir, outputsDir, store, opts)
			if err != nil {
				fmt.Fprintf(os.Stderr, "cleanup: %v\n", err)
				return 1
			}
			if err := RemoveStalePacks(os.Stdout, stale, false); err != nil {
				fmt.Fprintf(os.Stderr, "cleanup: %v\n", err)
				return 1
			}
		}
	}
	if anyFailed {
		return 1
	}
	return 0
}
