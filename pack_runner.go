package main

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// PackResult is the structured outcome of a single asset pack run.
// It is returned by RunPack and consumed by both the HTTP handler
// (which maps Status to status codes) and the CLI summary table
// (which prints one row per result).
//
// Status taxonomy (see docs/active/work/T-010-04/design.md):
//
//	"ok"             pack succeeded; Species and Size are set
//	"missing-source" required {id}_billboard.glb is absent
//	"oversize"       CombinePack returned *PackOversizeError
//	"failed"         any other read / build-meta / write error
type PackResult struct {
	ID        string
	Species   string
	Size      int64
	HasTilted bool
	HasDome   bool
	Status    string
	Err       error
}

// RunPack reads the up-to-three baked intermediates for asset id,
// constructs a PackMeta via BuildPackMetaFromBake, runs CombinePack,
// and writes the result to {distDir}/{species}.glb. It never panics
// on I/O errors — every failure is encoded in the returned
// PackResult.Status / PackResult.Err pair.
//
// The store argument must already contain a FileRecord for id.
// Callers can populate it via scanExistingFiles or by adding a
// record explicitly (the tests use the latter).
func RunPack(
	id string,
	originalsDir, settingsDir, outputsDir, distDir string,
	store *FileStore,
	opts ResolverOptions,
) PackResult {
	res := PackResult{ID: id}

	if _, ok := store.Get(id); !ok {
		res.Status = "failed"
		res.Err = fmt.Errorf("no FileRecord for id %q", id)
		return res
	}

	// Side intermediate is required.
	sidePath := filepath.Join(outputsDir, id+"_billboard.glb")
	side, err := os.ReadFile(sidePath)
	if err != nil {
		if os.IsNotExist(err) {
			res.Status = "missing-source"
			res.Err = fmt.Errorf("missing %s", sidePath)
			return res
		}
		res.Status = "failed"
		res.Err = fmt.Errorf("read side intermediate: %w", err)
		return res
	}

	// Tilted and volumetric are optional. Probe them up-front so
	// HasTilted / HasDome land in the result even when a later
	// stage fails — the operator wants to see "this asset has
	// tilted but failed to combine" in the summary.
	tilted, tiltedErr := readOptionalIntermediate(outputsDir, id, "_billboard_tilted.glb")
	if tiltedErr != nil {
		res.Status = "failed"
		res.Err = fmt.Errorf("read tilted intermediate: %w", tiltedErr)
		return res
	}
	res.HasTilted = tilted != nil

	volumetric, volErr := readOptionalIntermediate(outputsDir, id, "_volumetric.glb")
	if volErr != nil {
		res.Status = "failed"
		res.Err = fmt.Errorf("read volumetric intermediate: %w", volErr)
		return res
	}
	res.HasDome = volumetric != nil

	meta, err := BuildPackMetaFromBake(id, originalsDir, settingsDir, outputsDir, store, opts)
	if err != nil {
		res.Status = "failed"
		res.Err = fmt.Errorf("build meta: %w", err)
		return res
	}

	packBytes, err := CombinePack(side, tilted, volumetric, meta)
	if err != nil {
		var oversize *PackOversizeError
		if errors.As(err, &oversize) {
			res.Status = "oversize"
			res.Err = oversize
			return res
		}
		res.Status = "failed"
		res.Err = fmt.Errorf("combine: %w", err)
		return res
	}

	if err := WritePack(distDir, meta.Species, packBytes); err != nil {
		res.Status = "failed"
		res.Err = fmt.Errorf("write pack: %w", err)
		return res
	}

	res.Status = "ok"
	res.Species = meta.Species
	res.Size = int64(len(packBytes))
	return res
}

// readOptionalIntermediate returns (nil, nil) when the file does
// not exist (the caller treats absence as "this flavour was not
// baked"), the file bytes when present, or (nil, err) for any
// other read failure.
func readOptionalIntermediate(outputsDir, id, suffix string) ([]byte, error) {
	p := filepath.Join(outputsDir, id+suffix)
	b, err := os.ReadFile(p)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	return b, nil
}
