package main

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// prepareResult holds the outcome of a single prepare run.
type prepareResult struct {
	Source         string `json:"source"`
	ID             string `json:"id"`
	Species        string `json:"species"`
	Category       string `json:"category"`
	SourceSize     int64  `json:"source_size"`
	OptimizedSize  int64  `json:"optimized_size,omitempty"`
	BillboardSize  int64  `json:"billboard_size,omitempty"`
	TiltedSize     int64  `json:"tilted_size,omitempty"`
	VolumetricSize int64  `json:"volumetric_size,omitempty"`
	PackSize       int64  `json:"pack_size,omitempty"`
	PackPath       string `json:"pack_path,omitempty"`
	Verified       bool   `json:"verified"`
	DurationMS     int64  `json:"duration_ms"`
	Status         string `json:"status"`
	FailedStep     string `json:"failed_step,omitempty"`
	Error          string `json:"error,omitempty"`
}

// prepareOptions holds parsed CLI flags.
type prepareOptions struct {
	category   string
	resolution int
	workDir    string
	jsonOutput bool
	skipLODs   bool
	skipVerify bool
}

// hashFile computes SHA-256 of a file and returns the first 16 bytes
// as a 32-character hex string. This matches the length of generateID()
// so all downstream code (FileStore, settings, etc.) works unchanged.
func hashFile(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:16]), nil
}

// nonAlphaNum matches characters that are not alphanumeric or underscore.
var nonAlphaNum = regexp.MustCompile(`[^a-z0-9_]+`)

// speciesFromFilename derives a clean species identifier from a filename.
// "dahlia_blush.glb" → "dahlia_blush"
// "My Plant (v2).glb" → "my_plant_v2"
func speciesFromFilename(filename string) string {
	base := strings.TrimSuffix(filepath.Base(filename), filepath.Ext(filename))
	s := strings.ToLower(base)
	s = strings.ReplaceAll(s, " ", "_")
	s = strings.ReplaceAll(s, "-", "_")
	s = nonAlphaNum.ReplaceAllString(s, "")
	s = strings.Trim(s, "_")
	if len(s) > 64 {
		s = s[:64]
	}
	if s == "" {
		s = "unknown"
	}
	return s
}

// runPrepare executes the 8-step pipeline for a single source GLB.
func runPrepare(sourcePath string, opts prepareOptions) prepareResult {
	start := time.Now()
	result := prepareResult{
		Source: sourcePath,
		Status: "ok",
	}

	sourceInfo, err := os.Stat(sourcePath)
	if err != nil {
		result.Status = "failed"
		result.FailedStep = "copy"
		result.Error = fmt.Sprintf("source file: %v", err)
		result.DurationMS = time.Since(start).Milliseconds()
		return result
	}
	result.SourceSize = sourceInfo.Size()

	workDir, err := resolveWorkdir(opts.workDir)
	if err != nil {
		result.Status = "failed"
		result.FailedStep = "copy"
		result.Error = fmt.Sprintf("resolve workdir: %v", err)
		result.DurationMS = time.Since(start).Milliseconds()
		return result
	}

	originalsDir := filepath.Join(workDir, "originals")
	outputsDir := filepath.Join(workDir, "outputs")
	settingsDir := filepath.Join(workDir, "settings")
	acceptedDir := filepath.Join(workDir, "accepted")
	distDir := filepath.Join(workDir, DistPlantsDir)
	manifestPath := filepath.Join(workDir, "uploads.jsonl")

	// Step 1: Copy + Hash
	id, err := hashFile(sourcePath)
	if err != nil {
		result.Status = "failed"
		result.FailedStep = "copy"
		result.Error = fmt.Sprintf("hash file: %v", err)
		result.DurationMS = time.Since(start).Milliseconds()
		return result
	}
	result.ID = id

	originalPath := filepath.Join(originalsDir, id+".glb")
	if !fileExists(originalPath) {
		if err := copyFile(sourcePath, originalPath); err != nil {
			result.Status = "failed"
			result.FailedStep = "copy"
			result.Error = fmt.Sprintf("copy to originals: %v", err)
			result.DurationMS = time.Since(start).Milliseconds()
			return result
		}
	}

	// Step 2: Register
	species := speciesFromFilename(sourcePath)
	commonName := strings.ReplaceAll(species, "_", " ")

	_ = AppendUploadRecord(manifestPath, UploadManifestEntry{
		Hash:             id,
		OriginalFilename: filepath.Base(sourcePath),
		UploadedAt:       time.Now(),
		Size:             result.SourceSize,
	})

	store := NewFileStore()
	scanExistingFiles(store, originalsDir, outputsDir, settingsDir, acceptedDir)

	// Step 3: Optimize
	optimizedPath := filepath.Join(outputsDir, id+".glb")
	if !fileExists(optimizedPath) {
		settings := Settings{
			Compression: "cc",
		}
		args := BuildCommand(originalPath, optimizedPath, settings)
		output, err := RunGltfpack(args)
		if err != nil {
			result.Status = "failed"
			result.FailedStep = "optimize"
			result.Error = fmt.Sprintf("gltfpack: %s", output)
			result.DurationMS = time.Since(start).Milliseconds()
			return result
		}
	}
	if info, err := os.Stat(optimizedPath); err == nil {
		result.OptimizedSize = info.Size()
		store.Update(id, func(r *FileRecord) {
			r.Status = StatusDone
			r.OutputSize = info.Size()
		})
	}

	// Step 4: Classify
	category := opts.category
	if category == "" {
		classResult, err := RunClassifier(originalPath)
		if err != nil {
			result.Status = "failed"
			result.FailedStep = "classify"
			result.Error = fmt.Sprintf("classifier: %v", err)
			result.DurationMS = time.Since(start).Milliseconds()
			return result
		}
		category = classResult.Category
		_, err = applyClassificationToSettings(id, settingsDir, classResult, true)
		if err != nil {
			result.Status = "failed"
			result.FailedStep = "classify"
			result.Error = fmt.Sprintf("apply classification: %v", err)
			result.DurationMS = time.Since(start).Milliseconds()
			return result
		}
	} else {
		// Manual category: load-or-create settings, stamp category + strategy
		s, err := LoadSettings(id, settingsDir)
		if err != nil {
			s = DefaultSettings()
		}
		s.ShapeCategory = category
		strategy := getStrategyForCategory(category)
		applyShapeStrategyToSettings(s, strategy, true)
		if err := SaveSettings(id, settingsDir, s); err != nil {
			result.Status = "failed"
			result.FailedStep = "classify"
			result.Error = fmt.Sprintf("save settings: %v", err)
			result.DurationMS = time.Since(start).Milliseconds()
			return result
		}
	}
	result.Category = category

	// Apply resolution override
	if opts.resolution > 0 {
		s, err := LoadSettings(id, settingsDir)
		if err != nil {
			result.Status = "failed"
			result.FailedStep = "classify"
			result.Error = fmt.Sprintf("load settings for resolution override: %v", err)
			result.DurationMS = time.Since(start).Milliseconds()
			return result
		}
		s.VolumetricResolution = opts.resolution
		if err := SaveSettings(id, settingsDir, s); err != nil {
			result.Status = "failed"
			result.FailedStep = "classify"
			result.Error = fmt.Sprintf("save settings: %v", err)
			result.DurationMS = time.Since(start).Milliseconds()
			return result
		}
	}

	// Step 5: LODs
	if !opts.skipLODs {
		for _, cfg := range lodConfigs {
			lodPath := filepath.Join(outputsDir, fmt.Sprintf("%s_%s.glb", id, cfg.Label))
			if fileExists(lodPath) {
				continue
			}
			s := Settings{
				Simplification:     cfg.Simplification,
				AggressiveSimplify: cfg.Aggressive,
				PermissiveSimplify: cfg.Permissive,
				Compression:        "cc",
			}
			args := BuildCommand(originalPath, lodPath, s)
			output, err := RunGltfpack(args)
			if err != nil {
				result.Status = "failed"
				result.FailedStep = "lods"
				result.Error = fmt.Sprintf("gltfpack %s: %s", cfg.Label, output)
				result.DurationMS = time.Since(start).Milliseconds()
				return result
			}
		}
	}

	// Step 6: Render
	blender := DetectBlender()
	if !blender.Available {
		result.Status = "failed"
		result.FailedStep = "render"
		result.Error = "blender not found — install Blender and ensure it is on PATH or in /Applications"
		result.DurationMS = time.Since(start).Milliseconds()
		return result
	}

	renderScriptPath := filepath.Join("scripts", "render_production.py")
	if !fileExists(renderScriptPath) {
		// Try relative to executable
		exeDir, _ := os.Executable()
		alt := filepath.Join(filepath.Dir(exeDir), "scripts", "render_production.py")
		if fileExists(alt) {
			renderScriptPath = alt
		} else {
			result.Status = "failed"
			result.FailedStep = "render"
			result.Error = "render script not found: " + renderScriptPath
			result.DurationMS = time.Since(start).Milliseconds()
			return result
		}
	}

	settings, err := LoadSettings(id, settingsDir)
	if err != nil {
		result.Status = "failed"
		result.FailedStep = "render"
		result.Error = fmt.Sprintf("load settings: %v", err)
		result.DurationMS = time.Since(start).Milliseconds()
		return result
	}

	strategy := getStrategyForCategory(category)
	skipVolumetric := strategy.SliceAxis == SliceAxisNA || strategy.SliceCount == 0

	cfg := buildProductionConfig{
		// Feed Blender the ORIGINAL (uncompressed) model, not the
		// gltfpack-optimized one. Blender's glTF importer doesn't
		// support EXT_meshopt_compression, and billboard textures
		// should be rendered from the highest-quality source anyway.
		Source:                filepath.Join(originalsDir, id+".glb"),
		OutputDir:             outputsDir,
		ID:                    id,
		Category:              category,
		Resolution:            settings.VolumetricResolution,
		BillboardAngles:       6,
		TiltedElevation:       30.0,
		VolumetricLayers:      strategy.SliceCount,
		VolumetricResolution:  settings.VolumetricResolution,
		SliceDistributionMode: strategy.SliceDistributionMode,
		SliceAxis:             strategy.SliceAxis,
		DomeHeightFactor:      settings.DomeHeightFactor,
		AlphaTest:             settings.AlphaTest,
		GroundAlign:           settings.GroundAlign,
		BakeExposure:          settings.BakeExposure,
		AmbientIntensity:      settings.AmbientIntensity,
		HemisphereIntensity:   settings.HemisphereIntensity,
		KeyLightIntensity:     settings.KeyLightIntensity,
		BottomFillIntensity:   settings.BottomFillIntensity,
		EnvMapIntensity:       settings.EnvMapIntensity,
		SkipVolumetric:        skipVolumetric,
	}

	configPath := filepath.Join(outputsDir, id+"_render_config.json")
	configData, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		result.Status = "failed"
		result.FailedStep = "render"
		result.Error = fmt.Sprintf("marshal config: %v", err)
		result.DurationMS = time.Since(start).Milliseconds()
		return result
	}
	if err := os.WriteFile(configPath, configData, 0644); err != nil {
		result.Status = "failed"
		result.FailedStep = "render"
		result.Error = fmt.Sprintf("write config: %v", err)
		result.DurationMS = time.Since(start).Milliseconds()
		return result
	}
	defer os.Remove(configPath)

	cmd := exec.Command(blender.Path, "-b", "--python", renderScriptPath, "--", "--config", configPath)
	if blender.ResourceDir != "" {
		cmd.Env = append(os.Environ(), "BLENDER_SYSTEM_RESOURCES="+blender.ResourceDir)
	}
	blenderOutput, err := cmd.CombinedOutput()
	if err != nil {
		msg := string(blenderOutput)
		if len(msg) > 2048 {
			msg = msg[:2048]
		}
		result.Status = "failed"
		result.FailedStep = "render"
		result.Error = fmt.Sprintf("blender: %s", msg)
		result.DurationMS = time.Since(start).Milliseconds()
		return result
	}

	// Verify intermediates
	billboardPath := filepath.Join(outputsDir, id+"_billboard.glb")
	tiltedPath := filepath.Join(outputsDir, id+"_billboard_tilted.glb")
	volumetricPath := filepath.Join(outputsDir, id+"_volumetric.glb")

	if !fileExists(billboardPath) {
		result.Status = "failed"
		result.FailedStep = "render"
		result.Error = "blender completed but billboard intermediate missing"
		result.DurationMS = time.Since(start).Milliseconds()
		return result
	}
	if !fileExists(tiltedPath) {
		result.Status = "failed"
		result.FailedStep = "render"
		result.Error = "blender completed but tilted intermediate missing"
		result.DurationMS = time.Since(start).Milliseconds()
		return result
	}
	if !skipVolumetric && !fileExists(volumetricPath) {
		result.Status = "failed"
		result.FailedStep = "render"
		result.Error = "blender completed but volumetric intermediate missing"
		result.DurationMS = time.Since(start).Milliseconds()
		return result
	}

	if info, err := os.Stat(billboardPath); err == nil {
		result.BillboardSize = info.Size()
	}
	if info, err := os.Stat(tiltedPath); err == nil {
		result.TiltedSize = info.Size()
	}
	if info, err := os.Stat(volumetricPath); err == nil {
		result.VolumetricSize = info.Size()
	}

	// Step 7: Pack
	packOpts := ResolverOptions{
		CLISpecies:    species,
		CLICommonName: commonName,
	}
	packResult := RunPack(id, originalsDir, settingsDir, outputsDir, distDir, store, packOpts)
	if packResult.Status != "ok" {
		result.Status = "failed"
		result.FailedStep = "pack"
		errMsg := packResult.Status
		if packResult.Err != nil {
			errMsg = packResult.Err.Error()
		}
		result.Error = errMsg
		result.DurationMS = time.Since(start).Milliseconds()
		return result
	}
	result.Species = packResult.Species
	result.PackSize = packResult.Size
	result.PackPath = filepath.Join(distDir, packResult.Species+".glb")

	// Step 8: Verify
	if !opts.skipVerify {
		report, err := InspectPack(result.PackPath)
		if err != nil {
			result.Status = "failed"
			result.FailedStep = "verify"
			result.Error = fmt.Sprintf("inspect pack: %v", err)
			result.DurationMS = time.Since(start).Milliseconds()
			return result
		}
		_ = report // Inspection succeeded — pack is valid
		result.Verified = true
	}

	result.DurationMS = time.Since(start).Milliseconds()
	return result
}

// printPrepareSummary writes a human-readable summary to w.
func printPrepareSummary(w io.Writer, r prepareResult) {
	if r.Status == "failed" {
		fmt.Fprintf(w, "✗ %s (failed at %s)\n", filepath.Base(r.Source), r.FailedStep)
		fmt.Fprintf(w, "  error: %s\n", r.Error)
		return
	}
	fmt.Fprintf(w, "✓ %s\n", filepath.Base(r.Source))
	fmt.Fprintf(w, "  source:     %s (%s)\n", r.Source, formatSize(r.SourceSize))
	if r.OptimizedSize > 0 {
		fmt.Fprintf(w, "  optimized:  outputs/%s.glb (%s)\n", r.ID, formatSize(r.OptimizedSize))
	}
	if r.BillboardSize > 0 {
		fmt.Fprintf(w, "  billboard:  outputs/%s_billboard.glb (%s)\n", r.ID, formatSize(r.BillboardSize))
	}
	if r.TiltedSize > 0 {
		fmt.Fprintf(w, "  tilted:     outputs/%s_billboard_tilted.glb (%s)\n", r.ID, formatSize(r.TiltedSize))
	}
	if r.VolumetricSize > 0 {
		fmt.Fprintf(w, "  volumetric: outputs/%s_volumetric.glb (%s)\n", r.ID, formatSize(r.VolumetricSize))
	}
	if r.PackPath != "" {
		verified := ""
		if r.Verified {
			verified = " ✓ verified"
		}
		fmt.Fprintf(w, "  pack:       %s (%s)%s\n", filepath.Base(r.PackPath), formatSize(r.PackSize), verified)
	}
	fmt.Fprintf(w, "  duration:   %ds\n", r.DurationMS/1000)
}

// formatSize returns a human-readable file size.
func formatSize(bytes int64) string {
	const (
		kb = 1024
		mb = 1024 * kb
	)
	switch {
	case bytes >= mb:
		return fmt.Sprintf("%.1f MB", float64(bytes)/float64(mb))
	case bytes >= kb:
		return fmt.Sprintf("%.1f KB", float64(bytes)/float64(kb))
	default:
		return fmt.Sprintf("%d B", bytes)
	}
}

// printPrepareJSON writes the result as JSON to w.
func printPrepareJSON(w io.Writer, r prepareResult) {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(r)
}

// runPrepareCmd handles `glb-optimizer prepare <source.glb> [flags]`.
func runPrepareCmd(args []string) int {
	fs := flag.NewFlagSet("prepare", flag.ContinueOnError)
	categoryFlag := fs.String("category", "", "Shape category (default: auto-classify)")
	resolutionFlag := fs.Int("resolution", 512, "Billboard render resolution")
	dirFlag := fs.String("dir", "", "Working directory (default: ~/.glb-optimizer)")
	jsonFlag := fs.Bool("json", false, "Structured JSON output for agent consumption")
	skipLODsFlag := fs.Bool("skip-lods", false, "Skip LOD generation")
	skipVerifyFlag := fs.Bool("skip-verify", false, "Skip post-pack verification")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: glb-optimizer prepare <source.glb> [--category CAT] [--resolution PX] [--dir PATH] [--json] [--skip-lods] [--skip-verify]")
		return 2
	}

	sourcePath := fs.Arg(0)
	if _, err := os.Stat(sourcePath); err != nil {
		fmt.Fprintf(os.Stderr, "error: source file not found: %s\n", sourcePath)
		return 2
	}

	opts := prepareOptions{
		category:   *categoryFlag,
		resolution: *resolutionFlag,
		workDir:    *dirFlag,
		jsonOutput: *jsonFlag,
		skipLODs:   *skipLODsFlag,
		skipVerify: *skipVerifyFlag,
	}

	result := runPrepare(sourcePath, opts)

	if opts.jsonOutput {
		printPrepareJSON(os.Stdout, result)
	} else {
		printPrepareSummary(os.Stdout, result)
	}

	if result.Status != "ok" {
		return 1
	}
	return 0
}

// runPrepareAllCmd handles `glb-optimizer prepare-all <dir> [flags]`.
func runPrepareAllCmd(args []string) int {
	fs := flag.NewFlagSet("prepare-all", flag.ContinueOnError)
	categoryFlag := fs.String("category", "", "Shape category (default: auto-classify)")
	resolutionFlag := fs.Int("resolution", 512, "Billboard render resolution")
	dirFlag := fs.String("dir", "", "Working directory (default: ~/.glb-optimizer)")
	jsonFlag := fs.Bool("json", false, "Structured JSON output for agent consumption")
	skipLODsFlag := fs.Bool("skip-lods", false, "Skip LOD generation")
	skipVerifyFlag := fs.Bool("skip-verify", false, "Skip post-pack verification")

	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(os.Stderr, "usage: glb-optimizer prepare-all <inbox-dir> [--category CAT] [--resolution PX] [--dir PATH] [--json] [--skip-lods] [--skip-verify]")
		return 2
	}

	inboxDir := fs.Arg(0)
	entries, err := os.ReadDir(inboxDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: cannot read directory: %v\n", err)
		return 1
	}

	var glbFiles []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".glb") {
			continue
		}
		glbFiles = append(glbFiles, filepath.Join(inboxDir, e.Name()))
	}

	if len(glbFiles) == 0 {
		fmt.Fprintln(os.Stderr, "no .glb files found in", inboxDir)
		return 0
	}

	opts := prepareOptions{
		category:   *categoryFlag,
		resolution: *resolutionFlag,
		workDir:    *dirFlag,
		jsonOutput: *jsonFlag,
		skipLODs:   *skipLODsFlag,
		skipVerify: *skipVerifyFlag,
	}

	doneDir := filepath.Join(inboxDir, "done")
	anyFailed := false

	for _, sourcePath := range glbFiles {
		result := runPrepare(sourcePath, opts)

		if opts.jsonOutput {
			printPrepareJSON(os.Stdout, result)
		} else {
			printPrepareSummary(os.Stdout, result)
		}

		if result.Status == "ok" {
			// Move to done/
			if err := os.MkdirAll(doneDir, 0755); err == nil {
				dest := filepath.Join(doneDir, filepath.Base(sourcePath))
				os.Rename(sourcePath, dest)
			}
		} else {
			anyFailed = true
		}
	}

	if !opts.jsonOutput {
		fmt.Fprintf(os.Stdout, "\nprocessed %d file(s)\n", len(glbFiles))
	}

	if anyFailed {
		return 1
	}
	return 0
}
