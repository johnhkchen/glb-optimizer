# T-012-05 Structure — File-Level Changes

## File footprint

```
NEW  clean_packs.go              ~120 LOC
NEW  clean_packs_test.go         ~180 LOC
NEW  clean_cmd.go                ~80  LOC
MOD  main.go                     +3 LOC  (case "clean-stale-packs")
MOD  pack_cmd.go                 +35 LOC (--clean flag + post-pack hook)
MOD  pack_cmd_test.go            +60 LOC (one --clean integration test)
MOD  justfile                    +10 LOC (two new recipes)
```

No other files touched. The resolver, pack runner, writer, and HTTP
handlers are unchanged. T-012-04's uploads.jsonl path resolution is
inherited automatically through `ResolveSpeciesIdentity`.

## clean_packs.go

```go
package main

import (
    "errors"
    "fmt"
    "io"
    "os"
    "path/filepath"
    "sort"
    "strings"
)

// IdentifyStalePacks: forward-mapping algorithm.
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

// RemoveStalePacks: dry-run-aware deletion loop.
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

// statSize returns the file size or 0 if stat fails — used only for
// human-readable output, never for correctness.
func statSize(path string) int64 {
    info, err := os.Stat(path)
    if err != nil {
        return 0
    }
    return info.Size()
}
```

## clean_cmd.go

```go
package main

import (
    "flag"
    "fmt"
    "os"
    "path/filepath"
)

// runCleanStalePacksCmd implements `glb-optimizer clean-stale-packs`.
// Dry-run by default; --apply actually deletes.
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
```

## main.go diff

```go
case "pack-inspect":
    os.Exit(runPackInspectCmd(os.Args[2:]))
case "clean-stale-packs":          // NEW
    os.Exit(runCleanStalePacksCmd(os.Args[2:]))
```

## pack_cmd.go diff

In `runPackAllCmd`, after `fs.Parse` and the existing flags, add:

```go
cleanFlag := fs.Bool("clean", false, "Remove stale packs after a successful pack-all")
```

After the `printPackSummary` call and the failCount accounting, before
the final return, add:

```go
if *cleanFlag {
    failed := false
    for _, r := range results {
        if r.Status != "ok" {
            failed = true
            break
        }
    }
    if failed {
        fmt.Fprintln(os.Stdout, "")
        fmt.Fprintln(os.Stdout, "Skipped stale-pack cleanup: pack-all had failures.")
    } else {
        fmt.Fprintln(os.Stdout, "")
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
```

The existing return-1-on-fail loop must run *before* the cleanup
hook, OR cleanup must short-circuit on fail. Cleanest is to compute
`failCount` once near the top, gate the post-summary `return 1` and
the cleanup branch off it. See plan.md step 4 for the exact diff.

## clean_packs_test.go

Tests (one func each):
1. `TestIdentifyStalePacks_AllLive` — three intermediates, three packs
   matching their resolved species → stale list empty.
2. `TestIdentifyStalePacks_AllStale` — empty outputs, three packs →
   all three stale.
3. `TestIdentifyStalePacks_Mixed` — two intermediates, three packs
   (one matching, two orphaned) → exactly the two orphans returned.
4. `TestIdentifyStalePacks_EmptyDist` — empty dist → empty list.
5. `TestIdentifyStalePacks_MissingDist` — distDir does not exist →
   empty list, nil error.
6. `TestIdentifyStalePacks_IgnoresNonGLB` — drop a `.DS_Store` and a
   `notes.txt` into dist → ignored, not classified as stale.
7. `TestRemoveStalePacks_DryRunLeavesFiles` — write two real files,
   call dry-run, assert files still exist + output mentions "(dry-run)".
8. `TestRemoveStalePacks_ApplyDeletes` — write two real files, call
   apply, assert files gone + output mentions "removed:".
9. `TestRemoveStalePacks_MissingFileLogsContinues` — list contains
   one nonexistent path; remove returns aggregated error but still
   processes the remaining file.
10. `TestRemoveStalePacks_EmptyList` — `[]` → "No stale packs." line,
    nil error.

## pack_cmd_test.go addition

11. `TestRunPackAllCmd_CleanRemovesStalePack` — set up tempdir with
    one billboard intermediate + a pre-existing stale pack file,
    run `runPackAllCmd([]string{"--dir", tmp, "--clean"})`, assert
    the stale file is gone afterward and the live pack remains.
    Skipped if combine.go's pack pipeline can't run on synthetic GLB
    bytes; if so, downgrade to a unit test that calls IdentifyStalePacks
    + RemoveStalePacks directly.

## justfile diff

```makefile
# Show stale packs in dist/plants/ that no longer have a source
# intermediate (dry-run; nothing is deleted).
clean-stale-packs: build
    ./glb-optimizer clean-stale-packs

# Actually delete the stale packs identified above. No undo.
clean-stale-packs-apply: build
    ./glb-optimizer clean-stale-packs --apply
```

## Ordering of changes

1. clean_packs.go (pure functions, no deps on CLI changes).
2. clean_packs_test.go (validates 1 in isolation).
3. clean_cmd.go (depends on 1).
4. main.go case (depends on 3).
5. pack_cmd.go --clean flag (depends on 1).
6. pack_cmd_test.go integration (depends on 5).
7. justfile recipes (depends on 4).

Each step is an atomic commit-able unit; steps 1+2 alone are usable
as a library.
