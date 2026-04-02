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
	HasBillboard bool       `json:"has_billboard,omitempty"`
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
