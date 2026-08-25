package backup

import (
	"archive/zip"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/kokosx/stratum/internal/storage"
	_ "turso.tech/database/tursogo"
)

// Verify opens archive and validates manifest, paths, checksums, and DB integrity.
// It does not modify the filesystem.
func Verify(archivePath string) error {
	_, err := verifyInternal(archivePath, true)
	return err
}

func verifyInternal(archivePath string, checkIntegrity bool) (*Manifest, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return nil, fmt.Errorf("open archive: %w", err)
	}
	defer r.Close()

	// Find manifest
	var manifestFile *zip.File
	for _, f := range r.File {
		if f.Name == ManifestName {
			manifestFile = f
			break
		}
	}
	if manifestFile == nil {
		return nil, fmt.Errorf("missing %s", ManifestName)
	}
	rc, err := manifestFile.Open()
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(rc)
	rc.Close()
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

	// Validate paths and collect expected files
	expected := map[string]FileEntry{
		m.Database: {Path: m.Database, SHA256: m.DatabaseSHA256},
	}
	for _, fe := range m.Files {
		if fe.Path == ManifestName || fe.Path == m.Database {
			return nil, fmt.Errorf("invalid file path in manifest: %s", fe.Path)
		}
		if strings.Contains(fe.Path, "..") || filepath.IsAbs(fe.Path) || strings.HasPrefix(fe.Path, "/") {
			return nil, fmt.Errorf("path traversal in manifest: %s", fe.Path)
		}
		if _, ok := expected[fe.Path]; ok {
			return nil, fmt.Errorf("duplicate path %s", fe.Path)
		}
		expected[fe.Path] = fe
	}

	// Check all files in ZIP are expected and verify checksums
	found := map[string]bool{}
	for _, f := range r.File {
		if f.Name == ManifestName {
			continue
		}
		// Path safety
		if strings.Contains(f.Name, "..") || filepath.IsAbs(f.Name) {
			return nil, fmt.Errorf("zip slip path %q", f.Name)
		}
		// Check symlink (zip FileHeader Mode)
		if f.FileInfo().Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("symlink not allowed: %s", f.Name)
		}
		fe, ok := expected[f.Name]
		if !ok {
			return nil, fmt.Errorf("unexpected file in archive: %s", f.Name)
		}
		found[f.Name] = true
		// Verify checksum
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		h := sha256.New()
		if _, err := io.Copy(h, rc); err != nil {
			rc.Close()
			return nil, err
		}
		rc.Close()
		got := hex.EncodeToString(h.Sum(nil))
		if fe.SHA256 != "" && got != fe.SHA256 {
			return nil, fmt.Errorf("checksum mismatch for %s: got %s want %s", f.Name, got, fe.SHA256)
		}
	}
	// Ensure all expected files present
	for path := range expected {
		if !found[path] {
			return nil, fmt.Errorf("missing file in archive: %s", path)
		}
	}

	// Verify DB integrity via temp extraction
	if checkIntegrity {
		tmpDir, err := os.MkdirTemp("", "stratum-verify-*")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(tmpDir)
		// Extract DB to temp
		var dbFile *zip.File
		for _, f := range r.File {
			if f.Name == m.Database {
				dbFile = f
				break
			}
		}
		if dbFile == nil {
			return nil, fmt.Errorf("missing database file")
		}
		tmpDB := filepath.Join(tmpDir, "verify.db")
		if err := extractFile(dbFile, tmpDB); err != nil {
			return nil, fmt.Errorf("extract db: %w", err)
		}
		if err := checkIntegrityDB(tmpDB); err != nil {
			return nil, err
		}
		// Check schema version not future
		if m.SchemaVersion != "" {
			latest := storage.LatestAvailableMigration()
			if latest != "" && m.SchemaVersion > latest {
				return nil, fmt.Errorf("backup schema %s is newer than supported %s – refusing restore", m.SchemaVersion, latest)
			}
		}
	}

	return &m, nil
}

func extractFile(zf *zip.File, dest string) error {
	rc, err := zf.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	out, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, rc)
	return err
}

func checkIntegrityDB(path string) error {
	db, err := sql.Open("turso", path)
	if err != nil {
		return fmt.Errorf("open temp db: %w", err)
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	var result string
	// quick_check is faster than integrity_check
	err = db.QueryRow("PRAGMA integrity_check").Scan(&result)
	if err != nil {
		return fmt.Errorf("integrity_check query: %w", err)
	}
	if result != "ok" {
		return fmt.Errorf("integrity_check failed: %s", result)
	}
	// Also check that schema_migrations exists
	return nil
}

func fileSHA256ForVerify(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}
