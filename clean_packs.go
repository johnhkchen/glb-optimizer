package main

// clean_packs.go is the T-012-05 stale pack identifier and remover.
//
// A pack file at dist/plants/{species}.glb is "stale" iff there is
// no current intermediate in outputs/ whose ResolveSpeciesIdentity
// output has the same species slug. Equivalently: build the live
// set L = { ResolveSpeciesIdentity(id).Species | id ∈ outputs }, and
// remove every dist/plants/*.glb whose basename is not in L.
//
// The forward-mapping (id → species) approach is correct because
// the resolver is many-to-one and has no general inverse.
//
// Removal is dry-run by default everywhere; the only safety net is
// the operator's explicit --apply (or pack-all --clean, where the
// caller has opted into apply by definition).

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// IdentifyStalePacks returns the absolute paths of *.glb files in
// distDir that have no live source intermediate in outputsDir.
//
// store may be nil; opts is passed through to ResolveSpeciesIdentity
// so callers that loaded a --mapping JSON for pack-all can keep
// their resolver inputs symmetric here.
//
// A missing distDir is NOT an error — it returns an empty list and
// nil error so a clean install with no packs ever built is a no-op.
// A missing outputsDir IS an error: the caller is asking us to
// compute a live set from a directory that does not exist, which is
// almost certainly a misconfigured --dir.
func IdentifyStalePacks(
	distDir, outputsDir string,
	store *FileStore,
	opts ResolverOptions,
) ([]string, error) {
	ids, err := discoverPackableIDs(outputsDir)
	if err != nil {
		return nil, fmt.Errorf("discover intermediates: %w", err)
	}
	live := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		ident, _, _ := ResolveSpeciesIdentity(id, outputsDir, store, opts)
		if ident.Species != "" {
			live[ident.Species] = struct{}{}
		}
	}

	entries, err := os.ReadDir(distDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("read dist dir: %w", err)
	}
	var stale []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".glb") {
			continue
		}
		species := strings.TrimSuffix(e.Name(), ".glb")
		if _, ok := live[species]; !ok {
			stale = append(stale, filepath.Join(distDir, e.Name()))
		}
	}
	sort.Strings(stale)
	return stale, nil
}

// RemoveStalePacks deletes (or, with dryRun=true, prints what it
// would delete) the supplied paths. It writes one human-readable
// line per file to w; the caller decides whether w is os.Stdout or
// a buffer for tests.
//
// Per-file removal failures are logged to w and accumulated; the
// loop never aborts mid-list. The returned error is errors.Join of
// every per-file failure (or nil if none). Dry-run never returns an
// error.
func RemoveStalePacks(w io.Writer, stale []string, dryRun bool) error {
	if len(stale) == 0 {
		fmt.Fprintln(w, "No stale packs.")
		return nil
	}

	if dryRun {
		fmt.Fprintln(w, "Stale packs (dry-run, would remove):")
	} else {
		fmt.Fprintln(w, "Removing stale packs:")
	}

	var errs []error
	removed := 0
	for _, p := range stale {
		size := statSize(p)
		base := filepath.Base(p)
		if dryRun {
			fmt.Fprintf(w, "  - %s (%s)\n", base, humanBytes(size))
			continue
		}
		if err := os.Remove(p); err != nil {
			fmt.Fprintf(w, "  - FAILED: %s: %v\n", base, err)
			errs = append(errs, fmt.Errorf("%s: %w", base, err))
			continue
		}
		fmt.Fprintf(w, "  - removed: %s (%s)\n", base, humanBytes(size))
		removed++
	}

	suffix := ""
	if dryRun {
		suffix = " (dry-run)"
	}
	fmt.Fprintf(w, "TOTAL: %d stale, %d removed%s\n", len(stale), removed, suffix)
	return errors.Join(errs...)
}

// statSize returns the file size or 0 if stat fails. Used only for
// the human-readable size column in output, never for correctness,
// so a stat failure is silently absorbed (the removal loop will
// surface the real error on the os.Remove call).
func statSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}
