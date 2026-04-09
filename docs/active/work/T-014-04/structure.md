# T-014-04 Structure: CLI Prepare Subcommand

## File Layout

```
prepare_cmd.go          # New — prepare + prepare-all subcommands (~350 lines)
prepare_cmd_test.go     # New — tests for prepare pipeline
main.go                 # Modified — add "prepare" and "prepare-all" to dispatch switch
```

## New Types

```go
// prepareResult holds the outcome of a single prepare run.
type prepareResult struct {
    Source          string        `json:"source"`
    ID              string        `json:"id"`
    Species         string        `json:"species"`
    Category        string        `json:"category"`
    SourceSize      int64         `json:"source_size"`
    OptimizedSize   int64         `json:"optimized_size,omitempty"`
    BillboardSize   int64         `json:"billboard_size,omitempty"`
    TiltedSize      int64         `json:"tilted_size,omitempty"`
    VolumetricSize  int64         `json:"volumetric_size,omitempty"`
    PackSize        int64         `json:"pack_size,omitempty"`
    PackPath        string        `json:"pack_path,omitempty"`
    Verified        bool          `json:"verified"`
    Duration        time.Duration `json:"-"`
    DurationMS      int64         `json:"duration_ms"`
    Status          string        `json:"status"`           // "ok" | "failed"
    FailedStep      string        `json:"failed_step,omitempty"`
    Error           string        `json:"error,omitempty"`
}

// prepareOptions holds parsed CLI flags.
type prepareOptions struct {
    category    string
    resolution  int
    workDir     string
    jsonOutput  bool
    skipLODs    bool
    skipVerify  bool
}
```

## New Functions

```go
// runPrepareCmd handles `glb-optimizer prepare <source.glb> [flags]`.
// Returns exit code: 0=success, 1=runtime error, 2=usage error.
func runPrepareCmd(args []string) int

// runPrepareAllCmd handles `glb-optimizer prepare-all <dir> [flags]`.
// Returns exit code: 0=all succeeded, 1=any failure, 2=usage error.
func runPrepareAllCmd(args []string) int

// runPrepare executes the 8-step pipeline for a single source GLB.
// Pure logic — no os.Exit, no flag parsing.
func runPrepare(sourcePath string, opts prepareOptions) prepareResult

// hashFile computes SHA-256 of a file, returns first 16 bytes as 32-char hex.
func hashFile(path string) (string, error)

// speciesFromFilename derives a clean species identifier from a filename.
// "dahlia_blush.glb" → "dahlia_blush", "My Plant (v2).glb" → "my_plant_v2"
func speciesFromFilename(filename string) string

// printPrepareSummary writes the human-readable summary to w.
func printPrepareSummary(w io.Writer, r prepareResult)

// printPrepareJSON writes the JSON result to w.
func printPrepareJSON(w io.Writer, r prepareResult)
```

## Modified Functions

```go
// main.go — add two cases to the switch:
case "prepare":
    os.Exit(runPrepareCmd(os.Args[2:]))
case "prepare-all":
    os.Exit(runPrepareAllCmd(os.Args[2:]))
```

## Pipeline Step Mapping (inside runPrepare)

| Step | Action | Existing Functions Used |
|------|--------|----------------------|
| 1. Copy+Hash | Hash source, copy to originals/{id}.glb | hashFile (new), copyFile (handlers.go) |
| 2. Register | AppendUploadRecord + FileStore.Add | AppendUploadRecord, NewFileStore, scanExistingFiles |
| 3. Optimize | Run gltfpack with default cc settings | BuildCommand, RunGltfpack |
| 4. Classify | --category flag or RunClassifier | RunClassifier, applyClassificationToSettings, applyShapeStrategyToSettings, SaveSettings |
| 5. LODs | Run gltfpack at 4 levels | lodConfigs, BuildCommand, RunGltfpack |
| 6. Render | Write config JSON, invoke Blender | buildProductionConfig, DetectBlender, exec.Command |
| 7. Pack | RunPack with ResolverOptions | RunPack |
| 8. Verify | InspectPack on output | InspectPack |

## Dependencies

- `crypto/sha256` — for hashFile
- `encoding/hex` — for hash encoding
- `io` — for copyFile (already in handlers.go, but the function is accessible)
- All other deps already imported by existing code in the package
