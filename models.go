package main

import "sync"

// FileStatus represents the processing state of a file.
type FileStatus string

const (
	StatusPending    FileStatus = "pending"
	StatusProcessing FileStatus = "processing"
	StatusDone       FileStatus = "done"
	StatusError      FileStatus = "error"
)

// Settings maps to gltfpack CLI flags.
type Settings struct {
	Simplification     float64 `json:"simplification"`
	Compression        string  `json:"compression"`         // "" | "cc" | "cz"
	TextureCompression string  `json:"texture_compression"` // "" | "tc" | "tw"
	TextureQuality     int     `json:"texture_quality"`
	TextureSize        int     `json:"texture_size"` // 0 = original, or 256/512/1024/2048
	KeepNodes          bool    `json:"keep_nodes"`
	KeepMaterials      bool    `json:"keep_materials"`
	FloatPositions     bool    `json:"float_positions"`
	AggressiveSimplify bool    `json:"aggressive_simplify"`
	PermissiveSimplify bool    `json:"permissive_simplify"`
	LockBorders        bool    `json:"lock_borders"`
}

// LODLevel holds info about a single LOD output.
type LODLevel struct {
	Level   int    `json:"level"` // 0-3, or -1 for billboard
	Size    int64  `json:"size"`
	Command string `json:"command,omitempty"`
	Error   string `json:"error,omitempty"`
}

// LODMeta holds recommended switch distances and aggregate stats for a LOD chain.
type LODMeta struct {
	Distances []float64 `json:"distances"`  // switch thresholds as multiples of bounding sphere radius
	TotalSize int64     `json:"total_size"` // sum of all LOD level sizes in bytes
}

// FileRecord tracks an uploaded file and its processing state.
type FileRecord struct {
	ID           string     `json:"id"`
	Filename     string     `json:"filename"`
	OriginalSize int64      `json:"original_size"`
	Status       FileStatus `json:"status"`
	OutputSize   int64      `json:"output_size,omitempty"`
	Command      string     `json:"command,omitempty"`
	Error        string     `json:"error,omitempty"`
	SettingsUsed *Settings  `json:"settings_used,omitempty"`
	LODs         []LODLevel `json:"lods,omitempty"`
	HasBillboard      bool       `json:"has_billboard,omitempty"`
	HasVolumetric     bool       `json:"has_volumetric,omitempty"`
	VolumetricLODs    []LODLevel `json:"volumetric_lods,omitempty"`
	VolumetricLODMeta *LODMeta   `json:"volumetric_lod_meta,omitempty"`
	HasReference      bool       `json:"has_reference,omitempty"`
	ReferenceExt      string     `json:"reference_ext,omitempty"` // ".png" or ".jpg"
	HasSavedSettings  bool       `json:"has_saved_settings,omitempty"`
	SettingsDirty     bool       `json:"settings_dirty,omitempty"`
	IsAccepted        bool       `json:"is_accepted,omitempty"`
}

// FileStore is a thread-safe in-memory store for file records.
type FileStore struct {
	mu    sync.RWMutex
	files map[string]*FileRecord
	order []string // maintain insertion order
}

func NewFileStore() *FileStore {
	return &FileStore{
		files: make(map[string]*FileRecord),
	}
}

func (s *FileStore) Add(record *FileRecord) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.files[record.ID] = record
	s.order = append(s.order, record.ID)
}

func (s *FileStore) Get(id string) (*FileRecord, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	r, ok := s.files[id]
	return r, ok
}

func (s *FileStore) Delete(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.files, id)
	for i, oid := range s.order {
		if oid == id {
			s.order = append(s.order[:i], s.order[i+1:]...)
			break
		}
	}
}

func (s *FileStore) All() []*FileRecord {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]*FileRecord, 0, len(s.order))
	for _, id := range s.order {
		if r, ok := s.files[id]; ok {
			result = append(result, r)
		}
	}
	return result
}

func (s *FileStore) Update(id string, fn func(*FileRecord)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if r, ok := s.files[id]; ok {
		fn(r)
	}
}

// ── Scene Budget System ──

// SceneBudget defines total resource limits for a scene.
type SceneBudget struct {
	MaxTriangles      int `json:"max_triangles"`
	MaxTextureMemoryKB int `json:"max_texture_memory_kb"`
}

// SceneAsset defines a single asset in a scene optimization request.
type SceneAsset struct {
	FileID    string `json:"file_id"`
	AssetType string `json:"asset_type"` // "hard-surface" or "organic"
	SceneRole string `json:"scene_role"` // "hero", "mid-ground", "background"
	Label     string `json:"label"`
}

// SceneRequest is the request body for POST /api/optimize-scene.
type SceneRequest struct {
	Budget SceneBudget  `json:"budget"`
	Assets []SceneAsset `json:"assets"`
}

// SceneAssetResult describes the optimization result for a single asset.
type SceneAssetResult struct {
	Label         string `json:"label"`
	FileID        string `json:"file_id"`
	AssetType     string `json:"asset_type"`
	SceneRole     string `json:"scene_role"`
	Strategy      string `json:"strategy"`
	TriangleCount int    `json:"triangle_count"`
	TextureSizeKB int    `json:"texture_size_kb"`
	OutputFile    string `json:"output_file"`
}

// SceneResult is the response for POST /api/optimize-scene.
type SceneResult struct {
	SceneID     string             `json:"scene_id"`
	BudgetUsed  SceneBudget        `json:"budget_used"`
	BudgetTotal SceneBudget        `json:"budget_total"`
	Assets      []SceneAssetResult `json:"assets"`
}
