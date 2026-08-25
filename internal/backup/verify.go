package backup

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/kokosx/stratum/internal/storage"
	_ "turso.tech/database/tursogo"
)

const (
	maxArchiveEntries = 100_000
	maxArchiveBytes   = int64(1 << 40) // a guardrail, not a media quota
)

// Verify validates an untrusted backup archive without changing live data.
func Verify(archivePath string) error { _, err := verifyInternal(archivePath, true); return err }

func verifyInternal(archivePath string, checkIntegrity bool) (*Manifest, error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return nil, fmt.Errorf("open archive: %w", err)
	}
	defer r.Close()
	if len(r.File) > maxArchiveEntries {
		return nil, fmt.Errorf("archive has too many entries")
	}

	var manifestFile *zip.File
	seen := map[string]struct{}{}
	var total uint64
	for _, f := range r.File {
		if !validArchivePath(f.Name) || f.FileInfo().Mode()&os.ModeSymlink != 0 {
			return nil, fmt.Errorf("invalid zip path %q", f.Name)
		}
		if _, ok := seen[f.Name]; ok {
			return nil, fmt.Errorf("duplicate zip path %q", f.Name)
		}
		seen[f.Name] = struct{}{}
		total += f.UncompressedSize64
		if total > uint64(maxArchiveBytes) {
			return nil, fmt.Errorf("archive uncompressed size exceeds limit")
		}
		if f.Name == ManifestName {
			manifestFile = f
		}
	}
	if manifestFile == nil {
		return nil, fmt.Errorf("missing %s", ManifestName)
	}
	rc, err := manifestFile.Open()
	if err != nil {
		return nil, err
	}
	data, readErr := io.ReadAll(io.LimitReader(rc, maxManifestBytes+1))
	closeErr := rc.Close()
	if readErr != nil {
		return nil, readErr
	}
	if closeErr != nil {
		return nil, closeErr
	}
	if len(data) > maxManifestBytes {
		return nil, fmt.Errorf("manifest exceeds %d bytes", maxManifestBytes)
	}
	var m Manifest
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, fmt.Errorf("invalid manifest: %w", err)
	}
	if err := m.Validate(); err != nil {
		return nil, err
	}

	expected := map[string]FileEntry{m.Database: {Path: m.Database, SHA256: m.DatabaseSHA256, Size: m.DatabaseSize}}
	for _, fe := range m.Files {
		if _, ok := expected[fe.Path]; ok {
			return nil, fmt.Errorf("duplicate path %s", fe.Path)
		}
		expected[fe.Path] = fe
	}
	found := map[string]bool{}
	for _, f := range r.File {
		if f.Name == ManifestName {
			continue
		}
		fe, ok := expected[f.Name]
		if !ok {
			return nil, fmt.Errorf("unexpected file in archive: %s", f.Name)
		}
		if int64(f.UncompressedSize64) != fe.Size {
			return nil, fmt.Errorf("size mismatch for %s", f.Name)
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		h := sha256.New()
		n, copyErr := io.Copy(h, rc)
		closeErr := rc.Close()
		if copyErr != nil {
			return nil, copyErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
		if n != fe.Size {
			return nil, fmt.Errorf("streamed size mismatch for %s", f.Name)
		}
		if got := hex.EncodeToString(h.Sum(nil)); got != fe.SHA256 {
			return nil, fmt.Errorf("checksum mismatch for %s", f.Name)
		}
		found[f.Name] = true
	}
	for name := range expected {
		if !found[name] {
			return nil, fmt.Errorf("missing file in archive: %s", name)
		}
	}

	if !checkIntegrity {
		return &m, nil
	}
	tmpDir, err := os.MkdirTemp("", "stratum-verify-*")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmpDir)
	var dbFile *zip.File
	for _, f := range r.File {
		if f.Name == DatabaseName {
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
	schema, mediaFiles, err := checkIntegrityDB(tmpDB)
	if err != nil {
		return nil, err
	}
	if schema != m.SchemaVersion {
		return nil, fmt.Errorf("schema version mismatch: archive has %s, manifest has %s", schema, m.SchemaVersion)
	}
	if latest := storage.LatestAvailableMigration(); latest != "" && storage.CompareMigrationVersions(schema, latest) > 0 {
		return nil, fmt.Errorf("backup schema %s is newer than supported %s", schema, latest)
	}
	for _, media := range mediaFiles {
		file, ok := expected[MediaPrefix+media.key]
		if !ok {
			return nil, fmt.Errorf("backup database references missing media %q", media.key)
		}
		if file.Size != media.size {
			return nil, fmt.Errorf("media size mismatch for %q: archive=%d database=%d", media.key, file.Size, media.size)
		}
	}
	return &m, nil
}

func extractFile(zf *zip.File, dest string) (err error) {
	rc, err := zf.Open()
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := rc.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	n, copyErr := io.Copy(out, rc)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if n != int64(zf.UncompressedSize64) {
		return fmt.Errorf("extracted size mismatch")
	}
	return nil
}

type managedMedia struct {
	key  string
	size int64
}

func checkIntegrityDB(path string) (string, []managedMedia, error) {
	db, err := sql.Open("turso", path)
	if err != nil {
		return "", nil, fmt.Errorf("open temp db: %w", err)
	}
	defer db.Close()
	var result string
	if err := db.QueryRow("PRAGMA integrity_check").Scan(&result); err != nil {
		return "", nil, err
	}
	if result != "ok" {
		return "", nil, fmt.Errorf("integrity_check failed: %s", result)
	}
	rows, err := db.Query("PRAGMA foreign_key_check")
	if err != nil {
		return "", nil, err
	}
	defer rows.Close()
	if rows.Next() {
		return "", nil, fmt.Errorf("foreign_key_check failed")
	}
	if err := rows.Err(); err != nil {
		return "", nil, err
	}
	var schema string
	if err := db.QueryRow("SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1").Scan(&schema); err != nil {
		return "", nil, fmt.Errorf("read schema_migrations: %w", err)
	}
	media, err := managedMediaRows(context.Background(), db)
	if err != nil {
		return "", nil, err
	}
	return schema, media, nil
}

// managedMediaRows discovers media tables because archives may predate variants.
func managedMediaRows(ctx context.Context, database *sql.DB) ([]managedMedia, error) {
	tables, err := database.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type = 'table' AND name IN ('media', 'media_variants') ORDER BY name`)
	if err != nil {
		return nil, err
	}
	var names []string
	for tables.Next() {
		var table string
		if err := tables.Scan(&table); err != nil {
			_ = tables.Close()
			return nil, err
		}
		names = append(names, table)
	}
	if err := tables.Close(); err != nil {
		return nil, err
	}
	if err := tables.Err(); err != nil {
		return nil, err
	}
	var result []managedMedia
	for _, table := range names {
		rows, err := database.QueryContext(ctx, `SELECT storage_key, file_size FROM `+table+` ORDER BY storage_key`)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var media managedMedia
			if err := rows.Scan(&media.key, &media.size); err != nil {
				_ = rows.Close()
				return nil, err
			}
			if !validArchivePath(MediaPrefix+media.key) || media.size < 0 {
				_ = rows.Close()
				return nil, fmt.Errorf("invalid managed media %q", media.key)
			}
			result = append(result, media)
		}
		if err := rows.Close(); err != nil {
			return nil, err
		}
	}
	return result, nil
}
