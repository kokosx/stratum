package backup

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

const (
	Format         = "stratum-backup"
	Version        = 1
	DatabaseName   = "database.sqlite"
	ManifestName   = "manifest.json"
	MediaPrefix    = "media/"
	StratumVersion = "dev"
)

// Create creates a portable backup archive.
// dataDir is the Stratum data directory (contains stratum.db and media/).
// outputPath is the desired ZIP path; if empty a default name is generated in the current directory.
// It returns the archive path.
func Create(ctx context.Context, database *storage.Database, queries *db.Queries, dataDir, outputPath string) (string, error) {
	if dataDir == "" {
		dataDir = "data"
	}
	dbPath := filepath.Join(dataDir, "stratum.db")
	mediaRoot := filepath.Join(dataDir, "media")

	// Ensure DB file exists
	if _, err := os.Stat(dbPath); err != nil {
		if os.IsNotExist(err) {
			// Fresh site – still create backup with empty DB snapshot
		} else {
			return "", fmt.Errorf("stat database: %w", err)
		}
	}

	if outputPath == "" {
		ts := time.Now().UTC().Format("2006-01-02-150405")
		outputPath = fmt.Sprintf("stratum-backup-%s.zip", ts)
	}
	// Ensure output directory exists
	if dir := filepath.Dir(outputPath); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return "", fmt.Errorf("create output dir: %w", err)
		}
	}

	tmpDir, err := os.MkdirTemp("", "stratum-backup-*")
	if err != nil {
		return "", fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// 1. Consistent DB snapshot via VACUUM INTO (online safe)
	snapshotPath := filepath.Join(tmpDir, "snapshot.db")
	if err := snapshotDB(ctx, database.DB, snapshotPath); err != nil {
		return "", fmt.Errorf("snapshot database: %w", err)
	}

	// 2. Compute DB checksum and size
	dbSHA, dbSize, err := fileSHA256(snapshotPath)
	if err != nil {
		return "", fmt.Errorf("hash snapshot: %w", err)
	}

	// 3. Collect media files (complete managed storage)
	var mediaFiles []FileEntry
	var mediaTotal int64
	if info, err := os.Stat(mediaRoot); err == nil && info.IsDir() {
		err = filepath.Walk(mediaRoot, func(path string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if info.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(mediaRoot, path)
			if err != nil {
				return err
			}
			// Normalize to forward slashes for ZIP
			zipPath := filepath.ToSlash(filepath.Join(MediaPrefix, rel))
			// Prevent path traversal and absolute
			if strings.Contains(zipPath, "..") || filepath.IsAbs(zipPath) {
				return fmt.Errorf("invalid media path %q", zipPath)
			}
			sha, size, err := fileSHA256(path)
			if err != nil {
				return err
			}
			mediaFiles = append(mediaFiles, FileEntry{Path: zipPath, SHA256: sha, Size: size})
			mediaTotal += size
			return nil
		})
		if err != nil {
			return "", fmt.Errorf("walk media: %w", err)
		}
	}
	sort.Slice(mediaFiles, func(i, j int) bool { return mediaFiles[i].Path < mediaFiles[j].Path })

	// 4. Schema version
	schemaVer := ""
	if queries != nil {
		if v, err := database.CurrentSchemaVersion(ctx); err == nil {
			schemaVer = v
		}
	}
	if schemaVer == "" {
		schemaVer = storage.LatestAvailableMigration()
	}

	manifest := Manifest{
		Format:         Format,
		Version:        Version,
		CreatedAt:      time.Now().UTC().Format(time.RFC3339),
		SchemaVersion:  schemaVer,
		StratumVersion: StratumVersion,
		Database:       DatabaseName,
		MediaRoot:      MediaPrefix,
		DatabaseSHA256: dbSHA,
		MediaCount:     len(mediaFiles),
		Files:          mediaFiles,
	}
	if err := manifest.Validate(); err != nil {
		return "", err
	}

	// 5. Create ZIP
	outFile, err := os.Create(outputPath)
	if err != nil {
		return "", fmt.Errorf("create archive: %w", err)
	}
	defer outFile.Close()
	zipWriter := zip.NewWriter(outFile)
	defer zipWriter.Close()

	// Ensure restrictive permissions
	if err := os.Chmod(outputPath, 0600); err != nil {
		// non-fatal
	}

	// Add database
	if err := addFileToZip(zipWriter, snapshotPath, DatabaseName, 0600); err != nil {
		return "", fmt.Errorf("add database to zip: %w", err)
	}
	// Add media files
	for _, fe := range mediaFiles {
		// fe.Path is like "media/originals/xxx.jpg" – strip prefix to get local path
		rel := strings.TrimPrefix(fe.Path, MediaPrefix)
		localPath := filepath.Join(mediaRoot, filepath.FromSlash(rel))
		if err := addFileToZip(zipWriter, localPath, fe.Path, 0600); err != nil {
			return "", fmt.Errorf("add media %s: %w", fe.Path, err)
		}
	}
	// Add manifest (must be last, after we know all files)
	manifestData, _ := json.MarshalIndent(manifest, "", "  ")
	header := &zip.FileHeader{
		Name:   ManifestName,
		Method: zip.Deflate,
	}
	header.SetMode(0600)
	w, err := zipWriter.CreateHeader(header)
	if err != nil {
		return "", err
	}
	if _, err := w.Write(manifestData); err != nil {
		return "", err
	}

	if err := zipWriter.Close(); err != nil {
		return "", err
	}
	if err := outFile.Close(); err != nil {
		return "", err
	}

	info, _ := os.Stat(outputPath)
	_ = dbSize
	_ = mediaTotal
	_ = info

	return outputPath, nil
}

func snapshotDB(ctx context.Context, db *sql.DB, dest string) error {
	// Ensure dest does not exist (VACUUM INTO fails if exists)
	_ = os.Remove(dest)
	// Quote path for SQL: replace single quote with ''
	safe := strings.ReplaceAll(dest, "'", "''")
	_, err := db.ExecContext(ctx, fmt.Sprintf("VACUUM INTO '%s'", safe))
	if err != nil {
		return err
	}
	// Ensure file exists and is not empty
	info, err := os.Stat(dest)
	if err != nil {
		return fmt.Errorf("snapshot not created: %w", err)
	}
	if info.Size() == 0 {
		return fmt.Errorf("snapshot is empty")
	}
	return nil
}

func fileSHA256(path string) (string, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", 0, err
	}
	defer f.Close()
	h := sha256.New()
	size, err := io.Copy(h, f)
	if err != nil {
		return "", 0, err
	}
	return hex.EncodeToString(h.Sum(nil)), size, nil
}

func addFileToZip(zw *zip.Writer, localPath, zipPath string, mode os.FileMode) error {
	// Prevent traversal
	if strings.Contains(zipPath, "..") || filepath.IsAbs(zipPath) {
		return fmt.Errorf("invalid zip path %q", zipPath)
	}
	// Reject symlinks
	info, err := os.Lstat(localPath)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("symlink not allowed: %s", localPath)
	}
	f, err := os.Open(localPath)
	if err != nil {
		return err
	}
	defer f.Close()
	header := &zip.FileHeader{
		Name:   zipPath,
		Method: zip.Deflate,
	}
	header.SetMode(mode)
	header.Modified = info.ModTime()
	w, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(w, f)
	return err
}
