package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

// TestLegacyDatabaseUpgradeToCurrent simulates a site that ran the pre-squash
// migration set (001_initial.sql … 019_stage2_dynamic.sql) and then upgrades
// through the current Migrate() path. Data must survive and new SEO columns
// must appear without duplicate-table/column errors.
func TestLegacyDatabaseUpgradeToCurrent(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "legacy.db")

	legacy, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := applyNamedMigrations(ctx, legacy, legacyMigrationCutoff); err != nil {
		legacy.Close()
		t.Fatalf("seed legacy migrations: %v", err)
	}
	now := time.Now().Unix()
	if err := seedLegacyContent(ctx, legacy.DB, now); err != nil {
		legacy.Close()
		t.Fatalf("seed legacy content: %v", err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}

	// Upgrade with the current migrator (all embedded files, including 020+).
	upgraded, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = upgraded.Close() })
	if err := upgraded.Migrate(ctx); err != nil {
		t.Fatalf("Migrate on legacy DB failed: %v", err)
	}

	var title string
	if err := upgraded.DB.QueryRowContext(ctx,
		`SELECT title FROM entry_revisions WHERE id = 'rev1'`,
	).Scan(&title); err != nil {
		t.Fatalf("legacy revision missing after upgrade: %v", err)
	}
	if title != "Hello Legacy" {
		t.Fatalf("title = %q, want Hello Legacy", title)
	}

	requiredColumns := map[string][]string{
		"entry_revisions": {"featured_media_id", "social_media_id", "seo_robots_index", "seo_robots_follow"},
		"site_settings":   {"site_social_media_id", "twitter_site", "site_represents"},
		"entries":         {"first_published_at"},
		"media_variants":  {"content_hash"},
	}
	for table, cols := range requiredColumns {
		for _, col := range cols {
			if !columnExists(ctx, t, upgraded.DB, table, col) {
				t.Errorf("expected column %s.%s after upgrade", table, col)
			}
		}
	}

	// Featured media copied from entries → published revision by 022.
	var featured sql.NullString
	if err := upgraded.DB.QueryRowContext(ctx,
		`SELECT featured_media_id FROM entry_revisions WHERE id = 'rev1'`,
	).Scan(&featured); err != nil {
		t.Fatal(err)
	}
	// No media was seeded; column must exist and be null-safe.
	_ = featured

	// Fresh install still works.
	fresh, err := Open(filepath.Join(dir, "fresh.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = fresh.Close() })
	if err := fresh.Migrate(ctx); err != nil {
		t.Fatalf("fresh Migrate failed: %v", err)
	}
	if !columnExists(ctx, t, fresh.DB, "entry_revisions", "featured_media_id") {
		t.Error("fresh install missing entry_revisions.featured_media_id")
	}
}

// legacyMigrationCutoff is the last historical migration name present before
// the SEO/performance series. Files sort lexicographically; "019_" < "020_".
const legacyMigrationCutoff = "019_stage2_dynamic.sql"

func applyNamedMigrations(ctx context.Context, d *Database, upToInclusive string) error {
	if _, err := d.DB.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS schema_migrations (
			version TEXT PRIMARY KEY,
			applied_at INTEGER NOT NULL
		)`); err != nil {
		return err
	}
	entries, err := migrationFiles.ReadDir("schema")
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() || len(e.Name()) < 5 || e.Name()[len(e.Name())-4:] != ".sql" {
			continue
		}
		names = append(names, e.Name())
	}
	for i := 1; i < len(names); i++ {
		for j := i; j > 0 && names[j] < names[j-1]; j-- {
			names[j], names[j-1] = names[j-1], names[j]
		}
	}
	for _, name := range names {
		if name > upToInclusive {
			break
		}
		sqlBytes, err := migrationFiles.ReadFile("schema/" + name)
		if err != nil {
			return err
		}
		tx, err := d.DB.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, string(sqlBytes)); err != nil {
			_ = tx.Rollback()
			return err
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO schema_migrations (version, applied_at) VALUES (?, unixepoch())`, name,
		); err != nil {
			_ = tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func seedLegacyContent(ctx context.Context, sqldb *sql.DB, now int64) error {
	if _, err := sqldb.ExecContext(ctx, `
		INSERT INTO entries (id, content_type_id, slug, status, created_at, updated_at)
		VALUES ('e1', 'page', 'hello-legacy', 'active', ?, ?)
	`, now, now); err != nil {
		return err
	}
	if _, err := sqldb.ExecContext(ctx, `
		INSERT INTO entry_revisions (id, entry_id, revision_number, title, excerpt, document_json, seo_title, seo_description, created_at)
		VALUES ('rev1', 'e1', 1, 'Hello Legacy', 'An excerpt', '{"version":1,"nodes":[]}', 'SEO Hello', 'SEO desc', ?)
	`, now); err != nil {
		return err
	}
	if _, err := sqldb.ExecContext(ctx, `
		UPDATE entries SET published_revision_id = 'rev1', published_at = ? WHERE id = 'e1'
	`, now); err != nil {
		return err
	}
	if _, err := sqldb.ExecContext(ctx, `
		INSERT INTO routes (id, path, entry_id, route_type, created_at, updated_at)
		VALUES ('r1', '/hello-legacy', 'e1', 'entry', ?, ?)
	`, now, now); err != nil {
		return err
	}
	return nil
}

func columnExists(ctx context.Context, t *testing.T, sqldb *sql.DB, table, column string) bool {
	t.Helper()
	rows, err := sqldb.QueryContext(ctx, `PRAGMA table_info(`+table+`)`)
	if err != nil {
		t.Fatalf("pragma table_info(%s): %v", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			t.Fatal(err)
		}
		if name == column {
			return true
		}
	}
	return false
}
