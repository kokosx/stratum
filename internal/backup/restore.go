package backup

import (
	"archive/zip"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/kokosx/stratum/internal/datalock"
	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

// Restore validates an archive and atomically replaces the live site.
func Restore(ctx context.Context, archivePath, dataDir string) (err error) {
	if dataDir == "" {
		dataDir = "data"
	}
	lock, err := datalock.Acquire(dataDir)
	if err != nil {
		return fmt.Errorf("acquire exclusive data directory access: %w", err)
	}
	defer func() {
		if closeErr := lock.Close(); closeErr != nil {
			err = errors.Join(err, closeErr)
		}
	}()

	manifest, err := verifyInternal(archivePath, true)
	if err != nil {
		return fmt.Errorf("backup verification failed: %w", err)
	}
	if latest := storage.LatestAvailableMigration(); latest != "" && storage.CompareMigrationVersions(manifest.SchemaVersion, latest) > 0 {
		return fmt.Errorf("backup schema %s newer than supported %s", manifest.SchemaVersion, latest)
	}

	tmpExtract, err := os.MkdirTemp(filepath.Dir(dataDir), ".stratum-restore-*")
	if err != nil {
		return fmt.Errorf("create restore staging directory: %w", err)
	}
	defer os.RemoveAll(tmpExtract)
	if err := extractArchive(archivePath, tmpExtract); err != nil {
		return err
	}

	extractedDB := filepath.Join(tmpExtract, manifest.Database)
	if _, err := os.Stat(extractedDB); err != nil {
		return fmt.Errorf("extracted database missing: %w", err)
	}
	if _, _, err := checkIntegrityDB(extractedDB); err != nil {
		return fmt.Errorf("extracted db integrity: %w", err)
	}

	liveDBPath := filepath.Join(dataDir, "stratum.db")
	if _, err := os.Stat(liveDBPath); err == nil {
		safetyPath, err := uniqueSafetyPath(filepath.Join(dataDir, "backups"))
		if err != nil {
			return err
		}
		if err := createSafetyBackup(dataDir, safetyPath); err != nil {
			return fmt.Errorf("create mandatory pre-restore safety backup: %w", err)
		}
		fmt.Fprintf(os.Stderr, "created pre-restore safety backup: %s\n", safetyPath)
	} else if !os.IsNotExist(err) {
		return fmt.Errorf("stat live database: %w", err)
	}

	liveMediaRoot := filepath.Join(dataDir, "media")
	stagedMediaRoot := filepath.Join(tmpExtract, strings.TrimSuffix(manifest.MediaRoot, "/"))
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		return fmt.Errorf("create data directory: %w", err)
	}
	oldDB := liveDBPath + ".pre-restore.bak"
	oldMedia := liveMediaRoot + ".pre-restore.bak"
	if err := removeIfExists(oldDB); err != nil {
		return fmt.Errorf("remove previous database rollback file: %w", err)
	}
	if err := os.RemoveAll(oldMedia); err != nil {
		return fmt.Errorf("remove previous media rollback directory: %w", err)
	}

	dbMoved, mediaMoved := false, false
	rollback := func(cause error) error {
		return errors.Join(cause, restoreRollback(liveDBPath, liveMediaRoot, oldDB, oldMedia, dbMoved, mediaMoved))
	}
	if exists, err := pathExists(liveDBPath); err != nil {
		return err
	} else if exists {
		if err := os.Rename(liveDBPath, oldDB); err != nil {
			return fmt.Errorf("move live database aside: %w", err)
		}
		dbMoved = true
	}
	if exists, err := pathExists(liveMediaRoot); err != nil {
		return rollback(err)
	} else if exists {
		if err := os.Rename(liveMediaRoot, oldMedia); err != nil {
			return rollback(fmt.Errorf("move live media aside: %w", err))
		}
		mediaMoved = true
	}
	if err := os.Rename(extractedDB, liveDBPath); err != nil {
		return rollback(fmt.Errorf("restore database: %w", err))
	}
	if err := os.Chmod(liveDBPath, 0o600); err != nil {
		return rollback(fmt.Errorf("set restored database permissions: %w", err))
	}
	if exists, err := pathExists(stagedMediaRoot); err != nil {
		return rollback(err)
	} else if exists {
		if err := os.Rename(stagedMediaRoot, liveMediaRoot); err != nil {
			return rollback(fmt.Errorf("restore media: %w", err))
		}
	} else if err := os.MkdirAll(liveMediaRoot, 0o755); err != nil {
		return rollback(fmt.Errorf("create restored media directory: %w", err))
	}
	if err := syncDir(dataDir); err != nil {
		return rollback(fmt.Errorf("sync restored data directory: %w", err))
	}

	restoredDB, err := storage.Open(liveDBPath)
	if err != nil {
		return rollback(fmt.Errorf("open restored database: %w", err))
	}
	if err := restoredDB.Migrate(ctx); err != nil {
		closeErr := restoredDB.Close()
		return rollback(errors.Join(fmt.Errorf("migrate restored database: %w", err), closeErr))
	}
	if err := restoredDB.Close(); err != nil {
		return rollback(fmt.Errorf("close restored database: %w", err))
	}
	if _, _, err := checkIntegrityDB(liveDBPath); err != nil {
		return rollback(fmt.Errorf("verify migrated database: %w", err))
	}
	if err := removeIfExists(oldDB); err != nil {
		return fmt.Errorf("cleanup database rollback file: %w", err)
	}
	if err := os.RemoveAll(oldMedia); err != nil {
		return fmt.Errorf("cleanup media rollback directory: %w", err)
	}
	return syncDir(dataDir)
}

func extractArchive(archivePath, destRoot string) (err error) {
	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return fmt.Errorf("open archive: %w", err)
	}
	defer func() {
		if closeErr := r.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close archive: %w", closeErr))
		}
	}()
	for _, f := range r.File {
		if !validArchivePath(f.Name) || f.FileInfo().Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("invalid archive path %q", f.Name)
		}
		dest := filepath.Join(destRoot, filepath.FromSlash(f.Name))
		if err := extractFile(f, dest); err != nil {
			return fmt.Errorf("extract %s: %w", f.Name, err)
		}
		if err := os.Chmod(dest, 0o600); err != nil {
			return fmt.Errorf("set extracted file permissions: %w", err)
		}
	}
	return nil
}

func uniqueSafetyPath(backupDir string) (string, error) {
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		return "", fmt.Errorf("create safety backup directory: %w", err)
	}
	f, err := os.CreateTemp(backupDir, "pre-restore-*.zip")
	if err != nil {
		return "", fmt.Errorf("reserve safety backup path: %w", err)
	}
	path := f.Name()
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("close safety backup reservation: %w", err)
	}
	if err := os.Remove(path); err != nil {
		return "", fmt.Errorf("release safety backup reservation: %w", err)
	}
	return path, nil
}

func restoreRollback(liveDB, liveMedia, oldDB, oldMedia string, dbMoved, mediaMoved bool) error {
	var errs []error
	if err := removeIfExists(liveDB); err != nil {
		errs = append(errs, fmt.Errorf("remove restored database: %w", err))
	}
	if err := os.RemoveAll(liveMedia); err != nil {
		errs = append(errs, fmt.Errorf("remove restored media: %w", err))
	}
	if dbMoved {
		if err := os.Rename(oldDB, liveDB); err != nil {
			errs = append(errs, fmt.Errorf("restore original database: %w", err))
		}
	}
	if mediaMoved {
		if err := os.Rename(oldMedia, liveMedia); err != nil {
			errs = append(errs, fmt.Errorf("restore original media: %w", err))
		}
	}
	return errors.Join(errs...)
}

func removeIfExists(path string) error {
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

func pathExists(path string) (bool, error) {
	_, err := os.Stat(path)
	if err == nil {
		return true, nil
	}
	if os.IsNotExist(err) {
		return false, nil
	}
	return false, err
}

func syncDir(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return errors.Join(err, f.Close())
	}
	return f.Close()
}

func createSafetyBackup(dataDir, dest string) error {
	dbPath := filepath.Join(dataDir, "stratum.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil
	} else if err != nil {
		return err
	}
	database, err := storage.Open(dbPath)
	if err != nil {
		return err
	}
	queries := db.New(database.DB)
	_, backupErr := Create(context.Background(), database, queries, dataDir, dest)
	closeErr := database.Close()
	return errors.Join(backupErr, closeErr)
}
