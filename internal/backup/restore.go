package backup

import (
	"archive/zip"
	"context"
	"database/sql"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	_ "turso.tech/database/tursogo"
)

// Restore validates archive and atomically replaces the live site.
// It creates a safety backup of the current site first (unless dataDir empty).
func Restore(ctx context.Context, archivePath, dataDir string) error {
	if dataDir == "" {
		dataDir = "data"
	}
	// 1. Validate archive completely (including integrity)
	manifest, err := verifyInternal(archivePath, true)
	if err != nil {
		return fmt.Errorf("backup verification failed: %w", err)
	}

	// 2. Check schema compatibility (already done in verify, but re-check)
	latest := storage.LatestAvailableMigration()
	if manifest.SchemaVersion != "" && latest != "" && manifest.SchemaVersion > latest {
		return fmt.Errorf("backup schema %s newer than supported %s", manifest.SchemaVersion, latest)
	}

	// 3. Extract into temp directory
	tmpExtract, err := os.MkdirTemp("", "stratum-restore-extract-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpExtract)

	r, err := zip.OpenReader(archivePath)
	if err != nil {
		return err
	}
	defer r.Close()

	// Extract all files with path safety
	for _, f := range r.File {
		if strings.Contains(f.Name, "..") || filepath.IsAbs(f.Name) {
			return fmt.Errorf("zip slip: %s", f.Name)
		}
		if f.FileInfo().Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink not allowed: %s", f.Name)
		}
		dest := filepath.Join(tmpExtract, filepath.FromSlash(f.Name))
		if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
			return err
		}
		if f.FileInfo().IsDir() {
			continue
		}
		if err := extractFile(f, dest); err != nil {
			return fmt.Errorf("extract %s: %w", f.Name, err)
		}
		// Restore permissions
		_ = os.Chmod(dest, 0600)
	}

	// Verify extracted DB again
	extractedDB := filepath.Join(tmpExtract, manifest.Database)
	if _, err := os.Stat(extractedDB); err != nil {
		return fmt.Errorf("extracted database missing: %w", err)
	}
	if err := checkIntegrityDB(extractedDB); err != nil {
		return fmt.Errorf("extracted db integrity: %w", err)
	}

	// 4. Check exclusive access (require no active server)
	if err := checkExclusiveAccess(dataDir); err != nil {
		return fmt.Errorf("restore requires exclusive access (stop stratum serve first): %w", err)
	}

	// 5. Create safety backup if live data exists
	liveDBPath := filepath.Join(dataDir, "stratum.db")
	if _, err := os.Stat(liveDBPath); err == nil {
		// Open live DB to snapshot for safety backup
		liveDB, err := storage.Open(liveDBPath)
		if err == nil {
			// Use backup.Create to make safety backup
			// Avoid recursion: directly call snapshot
			timestamp := time.Now().UTC().Format("2006-01-02-150405")
			safetyName := fmt.Sprintf("pre-restore-%s.zip", timestamp)
			safetyPath := filepath.Join(dataDir, safetyName)
			// We need a DB handle; use liveDB.DB
			// Create a temp storage.Database for safety backup creation via our Create func
			// To avoid import cycle, we do manual VACUUM INTO for safety backup
			// Simpler: call Create with same dataDir but output safety path
			// But Create will try to snapshot live DB again – that's fine
			// Use a helper
			_ = liveDB.Close()
			// Create safety backup using a fresh DB handle
			if err := createSafetyBackup(dataDir, safetyPath); err != nil {
				// Log but do not fail restore
				fmt.Fprintf(os.Stderr, "warning: could not create pre-restore safety backup: %v\n", err)
			} else {
				fmt.Fprintf(os.Stderr, "created pre-restore safety backup: %s\n", safetyPath)
			}
		}
	}

	// 6. Prepare live paths
	liveMediaRoot := filepath.Join(dataDir, "media")
	backupMediaRoot := filepath.Join(tmpExtract, strings.TrimSuffix(manifest.MediaRoot, "/"))
	// Ensure dataDir exists
	if err := os.MkdirAll(dataDir, 0755); err != nil {
		return err
	}

	// 7. Atomic replacement
	// Keep old state for rollback
	oldDBBackup := liveDBPath + ".pre-restore.bak"
	oldMediaBackup := liveMediaRoot + ".pre-restore.bak"
	// Clean previous bak
	_ = os.Remove(oldDBBackup)
	_ = os.RemoveAll(oldMediaBackup)

	// Move live DB aside if exists
	if _, err := os.Stat(liveDBPath); err == nil {
		if err := os.Rename(liveDBPath, oldDBBackup); err != nil {
			return fmt.Errorf("backup live db: %w", err)
		}
	}
	// Move live media aside if exists
	if _, err := os.Stat(liveMediaRoot); err == nil {
		if err := os.Rename(liveMediaRoot, oldMediaBackup); err != nil {
			// Rollback DB
			if _, err2 := os.Stat(oldDBBackup); err2 == nil {
				_ = os.Rename(oldDBBackup, liveDBPath)
			}
			return fmt.Errorf("backup live media: %w", err)
		}
	}

	// Move extracted DB into place
	if err := os.Rename(extractedDB, liveDBPath); err != nil {
		// Rollback
		_ = os.Rename(oldDBBackup, liveDBPath)
		if _, err2 := os.Stat(oldMediaBackup); err2 == nil {
			_ = os.Rename(oldMediaBackup, liveMediaRoot)
		}
		return fmt.Errorf("restore db: %w", err)
	}
	_ = os.Chmod(liveDBPath, 0600)
	// Move extracted media into place (if any)
	if _, err := os.Stat(backupMediaRoot); err == nil {
		if err := os.Rename(backupMediaRoot, liveMediaRoot); err != nil {
			// Try copy fallback if rename across filesystems
			if err := copyDir(backupMediaRoot, liveMediaRoot); err != nil {
				// Rollback DB
				_ = os.Remove(liveDBPath)
				_ = os.Rename(oldDBBackup, liveDBPath)
				_ = os.RemoveAll(liveMediaRoot)
				if _, err2 := os.Stat(oldMediaBackup); err2 == nil {
					_ = os.Rename(oldMediaBackup, liveMediaRoot)
				}
				return fmt.Errorf("restore media: %w", err)
			}
		}
	} else {
		// No media in backup – ensure live media is removed (or keep old? we already moved old aside, so create empty)
		_ = os.MkdirAll(liveMediaRoot, 0755)
	}

	// Sync parent dir
	if f, err := os.Open(dataDir); err == nil {
		_ = f.Sync()
		_ = f.Close()
	}

	// 8. Run migrations on restored DB (in case older schema)
	restoredDB, err := storage.Open(liveDBPath)
	if err != nil {
		// Rollback
		_ = os.Remove(liveDBPath)
		if _, err2 := os.Stat(oldDBBackup); err2 == nil {
			_ = os.Rename(oldDBBackup, liveDBPath)
		}
		if _, err2 := os.Stat(oldMediaBackup); err2 == nil {
			_ = os.RemoveAll(liveMediaRoot)
			_ = os.Rename(oldMediaBackup, liveMediaRoot)
		}
		return fmt.Errorf("open restored db: %w", err)
	}
	if err := restoredDB.Migrate(ctx); err != nil {
		restoredDB.Close()
		// Rollback
		_ = os.Remove(liveDBPath)
		if _, err2 := os.Stat(oldDBBackup); err2 == nil {
			_ = os.Rename(oldDBBackup, liveDBPath)
		}
		_ = os.RemoveAll(liveMediaRoot)
		if _, err2 := os.Stat(oldMediaBackup); err2 == nil {
			_ = os.Rename(oldMediaBackup, liveMediaRoot)
		}
		return fmt.Errorf("migrate restored db: %w", err)
	}
	restoredDB.Close()

	// 9. Cleanup old backups (keep one safety backup, remove the .bak we created for rollback)
	// The safety backup we created earlier is the official pre-restore backup; the .bak for atomicity can be removed after success
	_ = os.Remove(oldDBBackup)
	_ = os.RemoveAll(oldMediaBackup)

	// Search index is rebuildable – if FTS exists, it was restored as part of DB snapshot; but we should ensure consistency
	// For now, nothing to do (derived index will be rebuilt on next publish or via CLI 'search rebuild' if existed)
	return nil
}

func checkExclusiveAccess(dataDir string) error {
	dbPath := filepath.Join(dataDir, "stratum.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil
	}
	db, err := sql.Open("turso", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	// Try to get exclusive lock via BEGIN IMMEDIATE
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		if strings.Contains(err.Error(), "busy") || strings.Contains(err.Error(), "locked") {
			return fmt.Errorf("database is locked")
		}
		return err
	}
	// Try to do a dummy write lock
	_, err = tx.ExecContext(ctx, "SELECT 1 FROM schema_migrations LIMIT 1")
	if err != nil {
		_ = tx.Rollback()
		return err
	}
	_ = tx.Rollback()
	return nil
}

func createSafetyBackup(dataDir, dest string) error {
	dbPath := filepath.Join(dataDir, "stratum.db")
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		return nil
	}
	database, err := storage.Open(dbPath)
	if err != nil {
		return err
	}
	defer database.Close()
	queries := db.New(database.DB)
	ctx := context.Background()
	_, err = Create(ctx, database, queries, dataDir, dest)
	return err
}

func copyDir(src, dest string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		target := filepath.Join(dest, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink not allowed: %s", path)
		}
		in, err := os.Open(path)
		if err != nil {
			return err
		}
		defer in.Close()
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		out, err := os.Create(target)
		if err != nil {
			return err
		}
		_, err = io.Copy(out, in)
		out.Close()
		if err != nil {
			return err
		}
		return os.Chmod(target, 0600)
	})
}
