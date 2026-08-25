package backup

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

// Manifest describes a portable Stratum backup.
type Manifest struct {
	Format         string      `json:"format"`
	Version        int         `json:"version"`
	CreatedAt      string      `json:"created_at"`
	SchemaVersion  string      `json:"schema_version"`
	StratumVersion string      `json:"stratum_version"`
	Database       string      `json:"database"`
	MediaRoot      string      `json:"media_root"`
	DatabaseSHA256 string      `json:"database_sha256"`
	MediaCount     int         `json:"media_count"`
	Files          []FileEntry `json:"files"`
}

type FileEntry struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Size   int64  `json:"size"`
}

func (m *Manifest) Validate() error {
	if m.Format != "stratum-backup" {
		return fmt.Errorf("invalid format %q", m.Format)
	}
	if m.Version != 1 {
		return fmt.Errorf("unsupported backup version %d", m.Version)
	}
	if m.Database == "" {
		return fmt.Errorf("missing database field")
	}
	if m.SchemaVersion == "" {
		return fmt.Errorf("missing schema_version")
	}
	// Sort files for deterministic checksum comparison
	sort.Slice(m.Files, func(i, j int) bool { return m.Files[i].Path < m.Files[j].Path })
	return nil
}

func readManifest(path string) (*Manifest, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("invalid manifest: %w", err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}
	return &m, nil
}
