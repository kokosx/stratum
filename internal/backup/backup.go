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
	"strings"
	"time"

	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/version"
)

const (
	Format       = "stratum-backup"
	Version      = 1
	DatabaseName = "database.sqlite"
	ManifestName = "manifest.json"
	MediaPrefix  = "media/"
)

type Result struct {
	Path, SchemaVersion                   string
	DatabaseSize, MediaBytes, ArchiveSize int64
	MediaCount                            int
}

// Create creates a portable backup archive.
// dataDir is the Stratum data directory (contains stratum.db and media/).
// outputPath is the desired ZIP path; if empty a default name is generated in the current directory.
// It returns the archive path.
func Create(ctx context.Context, database *storage.Database, queries *db.Queries, dataDir, outputPath string) (string, error) {
	result, err := CreateResult(ctx, database, queries, dataDir, outputPath)
	return result.Path, err
}

// CreateResult snapshots the database first, stages the managed media set named
// by that snapshot, then atomically publishes a verified archive.
func CreateResult(ctx context.Context, database *storage.Database, queries *db.Queries, dataDir, outputPath string) (Result, error) {
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
			return Result{}, fmt.Errorf("stat database: %w", err)
		}
	}

	if outputPath == "" {
		ts := time.Now().UTC().Format("2006-01-02-150405")
		outputPath = fmt.Sprintf("stratum-backup-%s.zip", ts)
	}
	// Ensure output directory exists
	if dir := filepath.Dir(outputPath); dir != "." && dir != "" {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return Result{}, fmt.Errorf("create output dir: %w", err)
		}
	}
	if _, err := os.Stat(outputPath); err == nil {
		return Result{}, fmt.Errorf("backup output already exists: %s", outputPath)
	} else if !os.IsNotExist(err) {
		return Result{}, fmt.Errorf("stat backup output: %w", err)
	}

	tmpDir, err := os.MkdirTemp("", "stratum-backup-*")
	if err != nil {
		return Result{}, fmt.Errorf("create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	// 1. Consistent DB snapshot via VACUUM INTO (online safe)
	snapshotPath := filepath.Join(tmpDir, "snapshot.db")
	if err := snapshotDB(ctx, database.DB, snapshotPath); err != nil {
		return Result{}, fmt.Errorf("snapshot database: %w", err)
	}

	// 2. Compute DB checksum and size
	dbSHA, dbSize, err := fileSHA256(snapshotPath)
	if err != nil {
		return Result{}, fmt.Errorf("hash snapshot: %w", err)
	}
	schemaVer, mediaFilesDB, err := snapshotMetadata(ctx, snapshotPath)
	if err != nil {
		return Result{}, err
	}

	// Database rows are authoritative. Originals and media variants are preserved;
	// unrelated files under media/ are deliberately excluded.
	stageRoot := filepath.Join(tmpDir, "media")
	var mediaFiles []FileEntry
	var mediaTotal int64
	for _, media := range mediaFilesDB {
		key := media.key
		zipPath := MediaPrefix + key
		if !validArchivePath(zipPath) {
			return Result{}, fmt.Errorf("invalid managed media key %q", key)
		}
		source := filepath.Join(mediaRoot, filepath.FromSlash(key))
		dest := filepath.Join(stageRoot, filepath.FromSlash(key))
		if err := copyRegularFile(source, dest); err != nil {
			return Result{}, fmt.Errorf("stage managed media %q: %w", key, err)
		}
		sha, size, err := fileSHA256(dest)
		if err != nil {
			return Result{}, err
		}
		if size != media.size {
			return Result{}, fmt.Errorf("managed media %q size differs from database: file=%d database=%d", key, size, media.size)
		}
		mediaFiles = append(mediaFiles, FileEntry{Path: zipPath, SHA256: sha, Size: size})
		mediaTotal += size
	}

	manifest := Manifest{
		Format:         Format,
		Version:        Version,
		CreatedAt:      time.Now().UTC().Format(time.RFC3339),
		SchemaVersion:  schemaVer,
		StratumVersion: version.Version,
		Database:       DatabaseName,
		MediaRoot:      MediaPrefix,
		DatabaseSHA256: dbSHA,
		DatabaseSize:   dbSize,
		MediaCount:     len(mediaFiles),
		Files:          mediaFiles,
	}
	if err := manifest.Validate(); err != nil {
		return Result{}, err
	}

	// Write beside the final output so rename is atomic.
	outFile, err := os.CreateTemp(filepath.Dir(outputPath), ".backup.zip.tmp-*")
	if err != nil {
		return Result{}, fmt.Errorf("create archive: %w", err)
	}
	tmpOutput := outFile.Name()
	defer os.Remove(tmpOutput)
	zipWriter := zip.NewWriter(outFile)
	defer zipWriter.Close()

	// Ensure restrictive permissions
	if err := outFile.Chmod(0600); err != nil {
		return Result{}, err
	}

	// Add database
	if err := addFileToZip(zipWriter, snapshotPath, DatabaseName, 0600); err != nil {
		return Result{}, fmt.Errorf("add database to zip: %w", err)
	}
	// Add media files
	for _, fe := range mediaFiles {
		rel := strings.TrimPrefix(fe.Path, MediaPrefix)
		localPath := filepath.Join(stageRoot, filepath.FromSlash(rel))
		if err := addFileToZip(zipWriter, localPath, fe.Path, 0600); err != nil {
			return Result{}, fmt.Errorf("add media %s: %w", fe.Path, err)
		}
	}
	// Add manifest (must be last, after we know all files)
	manifestData, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return Result{}, err
	}
	header := &zip.FileHeader{
		Name:   ManifestName,
		Method: zip.Deflate,
	}
	header.SetMode(0600)
	w, err := zipWriter.CreateHeader(header)
	if err != nil {
		return Result{}, err
	}
	if _, err := w.Write(manifestData); err != nil {
		return Result{}, err
	}

	if err := zipWriter.Close(); err != nil {
		return Result{}, err
	}
	if err := outFile.Sync(); err != nil {
		return Result{}, err
	}
	if err := outFile.Close(); err != nil {
		return Result{}, err
	}
	if err := Verify(tmpOutput); err != nil {
		return Result{}, fmt.Errorf("verify created archive: %w", err)
	}
	if err := os.Rename(tmpOutput, outputPath); err != nil {
		return Result{}, err
	}
	if err := syncDir(filepath.Dir(outputPath)); err != nil {
		return Result{}, fmt.Errorf("sync backup output directory: %w", err)
	}
	info, err := os.Stat(outputPath)
	if err != nil {
		return Result{}, err
	}
	return Result{Path: outputPath, SchemaVersion: schemaVer, DatabaseSize: dbSize, MediaCount: len(mediaFiles), MediaBytes: mediaTotal, ArchiveSize: info.Size()}, nil
}

func snapshotDB(ctx context.Context, db *sql.DB, dest string) error {
	// Ensure dest does not exist (VACUUM INTO fails if exists)
	if err := os.Remove(dest); err != nil && !os.IsNotExist(err) {
		return err
	}
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
	if !validArchivePath(zipPath) {
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

func copyRegularFile(source, dest string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("not a regular file")
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	in, err := os.Open(source)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dest, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func snapshotMetadata(ctx context.Context, snapshotPath string) (string, []managedMedia, error) {
	database, err := storage.OpenReadOnly(snapshotPath)
	if err != nil {
		return "", nil, fmt.Errorf("open snapshot: %w", err)
	}
	defer database.Close()
	schema, err := database.CurrentSchemaVersion(ctx)
	if err != nil {
		return "", nil, fmt.Errorf("read snapshot schema_migrations: %w", err)
	}
	media, err := managedMediaRows(ctx, database.DB)
	if err != nil {
		return "", nil, fmt.Errorf("list snapshot media: %w", err)
	}
	return schema, media, nil
}
