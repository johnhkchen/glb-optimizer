package main

// clean_cmd.go is the T-012-05 CLI handler for `glb-optimizer
// clean-stale-packs`. It is a thin wrapper around IdentifyStalePacks
// + RemoveStalePacks; the heavy lifting is in clean_packs.go.
//
// Dry-run is the default; --apply is required to actually delete.
// --mapping accepts the same JSON format as `pack-all --mapping` so
// the live set computed here matches what pack-all would produce.

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
)

// runCleanStalePacksCmd implements `glb-optimizer clean-stale-packs`.
// Returns 0 on success (including the no-stale and dry-run cases),
// 1 on directory-read failure, removal failure, or mapping load
// failure, 2 on usage error.
func runCleanStalePacksCmd(args []string) int {
	fs := flag.NewFlagSet("clean-stale-packs", flag.ContinueOnError)
	dirFlag := fs.String("dir", "", "Working directory (default: ~/.glb-optimizer)")
	applyFlag := fs.Bool("apply", false, "Actually delete (default: dry-run)")
	mappingFlag := fs.String("mapping", "", "JSON file mapping asset id → {species, common_name}")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 0 {
		fmt.Fprintln(os.Stderr, "usage: glb-optimizer clean-stale-packs [--dir PATH] [--apply] [--mapping FILE]")
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

	stale, err := IdentifyStalePacks(distDir, outputsDir, store, opts)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if err := RemoveStalePacks(os.Stdout, stale, !*applyFlag); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}
