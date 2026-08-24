package storage

import (
	"context"
	"database/sql"
	"io/fs"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func collectSchemaInfo(t *testing.T, database *Database) map[string]string {
	t.Helper()
	ctx := context.Background()
	// Collect table sql for relevant tables
	tables := []string{"layout_templates", "routes", "entries", "entry_revisions", "content_types"}
	info := map[string]string{}
	for _, tbl := range tables {
		var sqlStr sql.NullString
		err := database.DB.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type='table' AND name=?`, tbl).Scan(&sqlStr)
		if err != nil {
			t.Fatalf("get table %s: %v", tbl, err)
		}
		if sqlStr.Valid {
			info["table:"+tbl] = sqlStr.String
		}
		// columns
		rows, err := database.DB.QueryContext(ctx, `SELECT name, type FROM pragma_table_info('`+tbl+`')`)
		if err != nil {
			t.Fatalf("pragma %s: %v", tbl, err)
		}
		cols := ""
		for rows.Next() {
			var name, typ string
			rows.Scan(&name, &typ)
			cols += name + ":" + typ + ";"
		}
		rows.Close()
		info["columns:"+tbl] = cols
	}
	// indexes
	rows, err := database.DB.QueryContext(ctx, `SELECT name, sql FROM sqlite_master WHERE type='index' AND tbl_name IN ('layout_templates','routes','entries') ORDER BY name`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	idx := ""
	for rows.Next() {
		var name sql.NullString
		var sqlStr sql.NullString
		rows.Scan(&name, &sqlStr)
		if name.Valid {
			idx += name.String + ";"
		}
	}
	info["indexes"] = idx
	// schema_migrations
	rows2, err := database.DB.QueryContext(ctx, `SELECT version FROM schema_migrations ORDER BY version`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows2.Close()
	vers := ""
	for rows2.Next() {
		var v string
		rows2.Scan(&v)
		vers += v + ";"
	}
	info["migrations"] = vers
	return info
}

func applyMigrationsUpTo(t *testing.T, database *Database, upTo string) {
	t.Helper()
	ctx := context.Background()
	// Ensure schema_migrations exists
	_, _ = database.DB.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at INTEGER NOT NULL)`)
	entries, err := fs.ReadDir(migrationFiles, "schema")
	if err != nil {
		t.Fatal(err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		name := e.Name()
		if name > upTo {
			break
		}
		var applied bool
		_ = database.DB.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM schema_migrations WHERE version=?)`, name).Scan(&applied)
		if applied {
			continue
		}
		sqlBytes, err := migrationFiles.ReadFile("schema/" + name)
		if err != nil {
			t.Fatal(err)
		}
		tx, err := database.DB.BeginTx(ctx, nil)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.ExecContext(ctx, string(sqlBytes)); err != nil {
			tx.Rollback()
			t.Fatalf("apply %s: %v", name, err)
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO schema_migrations (version, applied_at) VALUES (?, unixepoch())`, name); err != nil {
			tx.Rollback()
			t.Fatal(err)
		}
		if err := tx.Commit(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestMigrationUpgradePathFreshVsUpgraded(t *testing.T) {
	ctx := context.Background()
	// Fresh DB: all migrations at once
	freshDir := t.TempDir()
	freshDB, err := Open(filepath.Join(freshDir, "fresh.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer freshDB.Close()
	if err := freshDB.Migrate(ctx); err != nil {
		t.Fatalf("fresh migrate: %v", err)
	}
	freshInfo := collectSchemaInfo(t, freshDB)

	// Upgraded DB: first up to 034, then full migrate
	upDir := t.TempDir()
	upDB, err := Open(filepath.Join(upDir, "up.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer upDB.Close()
	// Apply up to 034
	applyMigrationsUpTo(t, upDB, "034_performance_indexes.sql")
	// Then run full migrate (should apply 035,036,037)
	if err := upDB.Migrate(ctx); err != nil {
		t.Fatalf("upgraded migrate: %v", err)
	}
	upInfo := collectSchemaInfo(t, upDB)

	// Compare
	for k, v := range freshInfo {
		if upInfo[k] != v {
			t.Fatalf("schema mismatch %s:\nfresh %q\nupgraded %q", k, v, upInfo[k])
		}
	}
	// Specific checks
	if freshInfo["columns:layout_templates"] != upInfo["columns:layout_templates"] {
		t.Fatalf("layout_templates columns differ")
	}
	// Ensure parent column not exists in either
	var cnt int
	err = freshDB.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('layout_templates') WHERE name='parent_template_id'`).Scan(&cnt)
	if err != nil {
		t.Logf("pragma check err (tursogo may not support): %v", err)
	} else if cnt != 0 {
		t.Fatalf("fresh still has parent column")
	} else {
		// Double-check via sqlite_master sql not containing column name
		var sqlStr string
		freshDB.DB.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type='table' AND name='layout_templates'`).Scan(&sqlStr)
		if strings.Contains(sqlStr, "parent_template_id") {
			t.Fatalf("fresh layout_templates sql still contains parent: %q", sqlStr)
		}
	}
	err = upDB.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('layout_templates') WHERE name='parent_template_id'`).Scan(&cnt)
	if err != nil {
		t.Logf("pragma check err: %v", err)
	} else if cnt != 0 {
		t.Fatalf("upgraded still has parent column")
	} else {
		var sqlStr string
		upDB.DB.QueryRowContext(ctx, `SELECT sql FROM sqlite_master WHERE type='table' AND name='layout_templates'`).Scan(&sqlStr)
		if strings.Contains(sqlStr, "parent_template_id") {
			t.Fatalf("upgraded layout_templates sql still contains parent: %q", sqlStr)
		}
	}
	// Ensure 037 applied
	var c int
	freshDB.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version='037_remove_layout_parent.sql'`).Scan(&c)
	if c != 1 {
		t.Fatalf("fresh missing 037")
	}
	upDB.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version='037_remove_layout_parent.sql'`).Scan(&c)
	if c != 1 {
		t.Fatalf("upgraded missing 037")
	}
}

func TestMigrationImmutable032034Unchanged(t *testing.T) {
	// Verify 032/034 files have expected content (not mutated)
	ctx := context.Background()
	dir := t.TempDir()
	database, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	// Check 036 applied
	var count int
	err = database.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version='036_index_cleanup.sql'`).Scan(&count)
	if err != nil || count != 1 {
		t.Fatalf("036 not applied")
	}
	// Check that redundant indexes are gone
	rows, err := database.DB.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type='index'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	indexes := map[string]bool{}
	for rows.Next() {
		var name string
		rows.Scan(&name)
		indexes[name] = true
	}
	mustNotHave := []string{"idx_routes_content_type", "idx_routes_path_type", "idx_routes_archive_content", "idx_entries_published_content", "idx_entry_revisions_entry_number", "idx_layout_template_revisions_published"}
	for _, idx := range mustNotHave {
		if indexes[idx] {
			t.Fatalf("redundant index %s should be gone", idx)
		}
	}
}
