package backup

import (
	"encoding/json"
	"fmt"
	"os"
	"path"
	"sort"
	"strings"
)

const maxManifestBytes = 1 << 20

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
	DatabaseSize   int64       `json:"database_size"`
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
	if m.Database != DatabaseName {
		return fmt.Errorf("database must be %q", DatabaseName)
	}
	if m.MediaRoot != MediaPrefix {
		return fmt.Errorf("media_root must be %q", MediaPrefix)
	}
	if m.SchemaVersion == "" {
		return fmt.Errorf("missing schema_version")
	}
	if m.DatabaseSize < 0 {
		return fmt.Errorf("invalid database_size")
	}
	if m.MediaCount != len(m.Files) {
		return fmt.Errorf("media_count does not match files")
	}
	seen := map[string]struct{}{}
	for _, file := range m.Files {
		if !strings.HasPrefix(file.Path, MediaPrefix) || !validArchivePath(file.Path) || file.Size < 0 {
			return fmt.Errorf("invalid media path %q", file.Path)
		}
		if _, ok := seen[file.Path]; ok {
			return fmt.Errorf("duplicate media path %q", file.Path)
		}
		seen[file.Path] = struct{}{}
	}
	sort.Slice(m.Files, func(i, j int) bool { return m.Files[i].Path < m.Files[j].Path })
	return nil
}

// validArchivePath accepts only clean, slash-separated relative ZIP paths.
func validArchivePath(name string) bool {
	if name == "" || strings.HasPrefix(name, "/") || strings.Contains(name, "\\") {
		return false
	}
	if len(name) >= 2 && name[1] == ':' {
		return false
	}
	if path.Clean(name) != name || strings.HasPrefix(name, "../") || name == ".." {
		return false
	}
	for _, segment := range strings.Split(name, "/") {
		if segment == "" || segment == "." || segment == ".." {
			return false
		}
	}
	return true
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
