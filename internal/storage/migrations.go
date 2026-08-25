package storage

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strconv"
	"strings"
)

//go:embed schema/*.sql
var migrationFiles embed.FS

// Migrate applies each embedded migration once, in filename order.
func (d *Database) Migrate(ctx context.Context) error {
	if _, err := d.DB.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at INTEGER NOT NULL
		)`); err != nil {
		return fmt.Errorf("create migration table: %w", err)
	}

	if err := d.applyTransitionalMarkersIfNeeded(ctx); err != nil {
		return err
	}

	entries, err := fs.ReadDir(migrationFiles, "schema")
	if err != nil {
		return fmt.Errorf("read embedded migrations: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })

	for _, entry := range entries {
		if entry.IsDir() || entry.Name()[len(entry.Name())-4:] != ".sql" {
			continue
		}

		var applied bool
		if err := d.DB.QueryRowContext(ctx,
			"SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = ?)", entry.Name(),
		).Scan(&applied); err != nil {
			return fmt.Errorf("check migration %s: %w", entry.Name(), err)
		}
		if applied {
			continue
		}

		sql, err := migrationFiles.ReadFile("schema/" + entry.Name())
		if err != nil {
			return fmt.Errorf("read migration %s: %w", entry.Name(), err)
		}

		if entry.Name() == "041_revision_scoped_slug.sql" {
			if err := d.migrateEntriesRebuild(ctx, entry.Name(), sql); err != nil {
				return err
			}
			continue
		}
		if err := d.applyMigration(ctx, entry.Name(), sql); err != nil {
			return err
		}
	}

	return nil
}

func (d *Database) applyMigration(ctx context.Context, name string, migration []byte) error {
	tx, err := d.DB.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", name, err)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, string(migration)); err != nil {
		return fmt.Errorf("apply migration %s: %w", name, err)
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO schema_migrations (version, applied_at) VALUES (?, unixepoch())", name); err != nil {
		return fmt.Errorf("record migration %s: %w", name, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %s: %w", name, err)
	}
	return nil
}

// migrateEntriesRebuild always restores foreign key enforcement on the single
// SQLite connection, including failed DDL and migration-record paths.
func (d *Database) migrateEntriesRebuild(ctx context.Context, name string, migration []byte) (err error) {
	if _, err = d.DB.ExecContext(ctx, "PRAGMA foreign_keys = OFF"); err != nil {
		return fmt.Errorf("disable foreign keys for %s: %w", name, err)
	}
	defer func() {
		if _, restoreErr := d.DB.ExecContext(ctx, "PRAGMA foreign_keys = ON"); restoreErr != nil && err == nil {
			err = fmt.Errorf("restore foreign keys after %s: %w", name, restoreErr)
		}
	}()
	return d.applyMigration(ctx, name, migration)
}

// applyTransitionalMarkersIfNeeded detects a previous short-lived set of
// migration filenames (0001_baseline.sql … 0007_*.sql). When present we
// atomically mark the corresponding current filenames (001_… and 020_…025_)
// plus the squashed 002-019 range so the normal loop never re-executes DDL
// that would produce "table already exists" errors. Fresh 001-019 and 026+
// paths remain unaffected.
func (d *Database) applyTransitionalMarkersIfNeeded(ctx context.Context) error {
	oldMarkers := []string{
		"0001_baseline.sql",
		"0002_fix_block_rendering.sql",
		"0003_lcp_and_assets.sql",
		"0004_seo_foundation.sql",
		"0005_social_seo.sql",
		"0006_structured_data.sql",
		"0007_image_seo_performance.sql",
	}
	hasOld := false
	for _, m := range oldMarkers {
		var exists bool
		if err := d.DB.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = ?)", m).Scan(&exists); err != nil {
			return fmt.Errorf("check transitional %s: %w", m, err)
		}
		if exists {
			hasOld = true
			break
		}
	}
	if !hasOld {
		return nil
	}

	mapping := map[string]string{
		"0001_baseline.sql":              "001_initial.sql",
		"0002_fix_block_rendering.sql":   "020_fix_block_rendering.sql",
		"0003_lcp_and_assets.sql":        "021_lcp_and_assets.sql",
		"0004_seo_foundation.sql":        "022_seo_foundation.sql",
		"0005_social_seo.sql":            "023_social_seo.sql",
		"0006_structured_data.sql":       "024_structured_data.sql",
		"0007_image_seo_performance.sql": "025_image_seo_performance.sql",
	}

	for old, curr := range mapping {
		var hasOldMarker bool
		_ = d.DB.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = ?)", old).Scan(&hasOldMarker)
		if !hasOldMarker {
			continue
		}
		var hasCurr bool
		if err := d.DB.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = ?)", curr).Scan(&hasCurr); err != nil {
			return fmt.Errorf("check current %s: %w", curr, err)
		}
		if hasCurr {
			continue
		}
		if _, err := d.DB.ExecContext(ctx,
			"INSERT OR IGNORE INTO schema_migrations (version, applied_at) VALUES (?, unixepoch())", curr); err != nil {
			return fmt.Errorf("record mapped migration %s: %w", curr, err)
		}
	}

	// Mark squashed range (002-019) under current names to avoid re-execution.
	// The 015_* pair and other pre-020 files must be covered.
	squashed := []string{
		"002_auth.sql", "003_core_blocks.sql", "004_block_schema_v1.sql",
		"005_navigation.sql", "006_navigation_groups.sql", "007_theme_customizations.sql",
		"008_theme_block_tokens.sql", "009_expanded_core_blocks.sql", "010_seo.sql",
		"011_site_settings_runtime.sql", "012_media.sql", "013_site_icon.sql",
		"014_featured_media.sql", "015_core_image_block.sql", "015_stage1_blocks.sql",
		"016_stage2_content.sql", "017_dynamic_blocks.sql", "018_fix_core_image_template.sql",
		"018_stage2_media.sql", "019_stage2_dynamic.sql",
	}
	for _, name := range squashed {
		var has bool
		_ = d.DB.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version = ?)", name).Scan(&has)
		if !has {
			_, _ = d.DB.ExecContext(ctx, "INSERT OR IGNORE INTO schema_migrations (version, applied_at) VALUES (?, unixepoch())", name)
		}
	}
	return nil
}

// CompareMigrationVersions orders migration filenames by their numeric prefix,
// avoiding incorrect ordering once a migration number exceeds a digit boundary.
func CompareMigrationVersions(a, b string) int {
	parse := func(v string) (int, bool) {
		i := strings.IndexByte(v, '_')
		if i <= 0 {
			return 0, false
		}
		n, err := strconv.Atoi(v[:i])
		return n, err == nil
	}
	an, aok := parse(a)
	bn, bok := parse(b)
	if aok && bok && an != bn {
		if an < bn {
			return -1
		}
		return 1
	}
	return strings.Compare(a, b)
}

// LatestAvailableMigration returns the greatest embedded migration version.
func LatestAvailableMigration() string {
	entries, err := fs.ReadDir(migrationFiles, "schema")
	if err != nil {
		return ""
	}
	var max string
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if len(name) < 4 || name[len(name)-4:] != ".sql" {
			continue
		}
		if CompareMigrationVersions(name, max) > 0 {
			max = name
		}
	}
	return max
}

// CurrentSchemaVersion returns the latest applied migration version from the DB,
// or empty if none.
func (d *Database) CurrentSchemaVersion(ctx context.Context) (string, error) {
	var v string
	err := d.DB.QueryRowContext(ctx, "SELECT version FROM schema_migrations ORDER BY version DESC LIMIT 1").Scan(&v)
	if err != nil {
		// No migrations yet
		return "", nil
	}
	return v, nil
}
