# T-012-01 Structure: File-Level Blueprint

## New files

### `species_resolver.go`

```
package main

import (
    "encoding/json"
    "fmt"
    "log"
    "os"
    "path/filepath"
    "strings"
)

// SpeciesIdentity is the resolved (species, common_name) tuple ...
type SpeciesIdentity struct {
    Species    string
    CommonName string
}

// ResolverSource tags WHICH tier produced an identity ...
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
func (s ResolverSource) String() string

// ResolverOptions carries the per-call hints (CLI flags, mapping file)
type ResolverOptions struct {
    CLISpecies         string
    CLICommonName      string
    Mapping            map[string]SpeciesIdentity
    UploadManifestPath string // "" → ~/.glb-optimizer/uploads.jsonl
}

// LoadMappingFile parses the JSON object format documented in the
// ticket and returns a map ready to drop into ResolverOptions.Mapping.
// Empty path → empty map (not error).
func LoadMappingFile(path string) (map[string]SpeciesIdentity, error)

// ResolveSpeciesIdentity walks the 6-tier chain (CLI → mapping → meta
// → store → manifest → hash) and returns the first identity whose
// normalised species id is non-empty.
func ResolveSpeciesIdentity(
    id, outputsDir string,
    store *FileStore,
    opts ResolverOptions,
) (SpeciesIdentity, ResolverSource, error)

// captureOverride moves here from pack_meta_capture.go (private impl
// detail of the meta-json tier).
type captureOverride struct {
    Species    string `json:"species"`
    CommonName string `json:"common_name"`
}
func loadCaptureOverride(path string) (captureOverride, error)

// uploadManifestEntry is one record in ~/.glb-optimizer/uploads.jsonl
// (T-012-04 will write this; T-012-01 only reads).
type uploadManifestEntry struct {
    ID       string `json:"id"`
    Filename string `json:"filename"`
}
func lookupUploadManifest(path, id string) (string, bool)

// hashFallbackSpecies builds "species_<first8>" from a content hash.
func hashFallbackSpecies(id string) (SpeciesIdentity)
```

Approximate length: ~220 lines including comments.

### `species_resolver_test.go`

Test functions, all using `t.TempDir()` for outputs/manifest dirs:

```
TestResolver_CLIOverrideWins
TestResolver_MappingFileBeatsSidecar
TestResolver_SidecarBeatsFileStore
TestResolver_FileStoreFallback
TestResolver_FileStoreSentinelSkipped
TestResolver_UploadManifestTier
TestResolver_HashFallback
TestResolver_NormalizesMessyMappingValues
TestResolver_NormalizesUploadManifestFilename
TestResolver_LoadMappingFile_HappyPath
TestResolver_LoadMappingFile_MissingFile
TestResolver_LoadMappingFile_BadJSON
TestResolverSource_String
```

Approximate length: ~250 lines.

## Modified files

### `pack_meta_capture.go`

Diff sketch:

- DELETE `captureOverride` struct (moves to `species_resolver.go`)
- DELETE `loadCaptureOverride` (moves)
- KEEP `deriveSpeciesFromName`, `titleCaseSpecies`, `nonAlnumRe`
  (now consumed by the resolver too — make them package-level
  references; no signature change)
- CHANGE `BuildPackMetaFromBake` signature:
  ```go
  func BuildPackMetaFromBake(
      id, originalsDir, settingsDir, outputsDir string,
      store *FileStore,
      opts ResolverOptions,   // NEW
  ) (PackMeta, error)
  ```
- REPLACE the `filename`/`override`/`species`/`common` block with one
  call to `ResolveSpeciesIdentity` and a single `log.Printf` of which
  source was used.
- The footprint, fade, and bake-id sections are untouched.

### `pack_runner.go`

Add `opts` parameter to `RunPack`:

```go
func RunPack(
    id string,
    originalsDir, settingsDir, outputsDir, distDir string,
    store *FileStore,
    opts ResolverOptions,   // NEW
) PackResult
```

The body change is one line: pass `opts` through to
`BuildPackMetaFromBake`.

### `pack_cmd.go`

`runPackCmd`:

- Add `--species`/`--common-name` flags via `fs.String(...)`.
- Build a `ResolverOptions` from those flags.
- Pass it to `RunPack`.
- Error if `--species` is provided without `--common-name` (or vice
  versa) — both-or-neither.

`runPackAllCmd`:

- Add `--mapping <file>` flag.
- Call `LoadMappingFile(*mappingFlag)`; bail with non-zero exit on
  parse error.
- Build `ResolverOptions{Mapping: m}` once and pass to every `RunPack`
  call in the loop.

### `handlers.go`

`handleBuildPack` (and any other call sites of `RunPack` /
`BuildPackMetaFromBake`): pass an empty `ResolverOptions{}`. Behaviour
unchanged because the resolver chain falls through to the FileStore
tier exactly as the old code did.

### `pack_meta_capture_test.go`

- Update every existing call to `BuildPackMetaFromBake` to pass
  `ResolverOptions{}`. About 7 call sites.
- Update `TestBuildPackMetaFromBake_DerivationFails` — it currently
  asserts an error; now the resolver hash-fallbacks and returns a
  valid pack with a `species_<first8>` slug. Rename the test to
  `TestBuildPackMetaFromBake_FallbackToHash` and assert the new
  behaviour.

### `pack_runner_test.go` and `pack_cmd_test.go`

Add the `ResolverOptions{}` argument at every existing call site.

## Ordering of changes

The Go compiler is unforgiving about half-applied refactors, so the
order is:

1. Create `species_resolver.go` with all symbols stubbed but
   functional. Compiles standalone (no other file needs editing).
2. Update `pack_meta_capture.go` to call the resolver (signature
   change). Update its tests.
3. Update `pack_runner.go` signature; update its tests; update
   `handlers.go::handleBuildPack` and any other callers.
4. Update `pack_cmd.go` to add flags and parse the mapping file;
   update its tests.
5. Add `species_resolver_test.go` last (most tests, easiest to defer).

After each step `go build ./... && go test ./...` must pass before
proceeding to the next.
