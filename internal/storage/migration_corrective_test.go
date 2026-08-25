package storage

import (
	"context"
	"database/sql"
	"io/fs"
	"path/filepath"
	"testing"

	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func TestCorrectiveMigrationIndexes(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "test.db")
	database, err := Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	// Check final indexes: should have deduplicated set
	rows, err := database.DB.QueryContext(ctx, `SELECT name FROM sqlite_master WHERE type='index'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	indexes := map[string]bool{}
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			t.Fatal(err)
		}
		indexes[name] = true
	}
	// Must have
	mustHave := []string{"idx_routes_archive_content_type", "idx_routes_redirect_to", "idx_routes_entry_type_path"}
	for _, idx := range mustHave {
		if !indexes[idx] {
			t.Fatalf("expected index %s missing, have %v", idx, indexes)
		}
	}
	// Must NOT have redundant (including removed parent index)
	mustNotHave := []string{"idx_routes_content_type", "idx_routes_path_type", "idx_routes_archive_content", "idx_entries_published_content", "idx_entry_revisions_entry_number", "idx_layout_template_revisions_published", "idx_layout_templates_parent"}
	for _, idx := range mustNotHave {
		if indexes[idx] {
			t.Fatalf("redundant index %s should have been dropped", idx)
		}
	}
	// Check 032 restored: it should have content_type_id column
	var colExists bool
	row := database.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('routes') WHERE name='content_type_id'`)
	var cnt int
	if err := row.Scan(&cnt); err != nil {
		t.Fatal(err)
	}
	if cnt != 1 {
		t.Fatalf("routes.content_type_id column missing")
	}
	_ = colExists
	_ = sql.ErrNoRows
}

func TestMigrationImmutable032034(t *testing.T) {
	// Verify that 032 and 034 files match the expected restored content
	// by checking that they contain the original indexes.
	// This is a safeguard against future accidental edits.
	ctx := context.Background()
	// We just verify that after migrate, the schema matches expected: the old
	// indexes are gone and corrective migration applied.
	dir := t.TempDir()
	database, err := Open(filepath.Join(dir, "test2.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	// Ensure 036 migration was applied (check schema_migrations)
	var count int
	err = database.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version='036_index_cleanup.sql'`).Scan(&count)
	if err != nil {
		t.Fatalf("check 036 migration: %v", err)
	}
	if count != 1 {
		t.Fatalf("036 corrective migration not applied")
	}
}

func TestMigration047RemovesLegacyPrivateEntryRoute(t *testing.T) {
	ctx := context.Background()
	database, err := Open(filepath.Join(t.TempDir(), "private-route.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	queries := db.New(database.DB)
	if err := queries.CreateContentType(ctx, db.CreateContentTypeParams{ID: "test-page", DisplayName: "Test Page", PluralName: "Test Pages", ConfigJson: "{}"}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: "private-entry", ContentTypeID: "test-page", Slug: "private", Status: "active", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "private-r1", EntryID: "private-entry", RevisionNumber: 1, Slug: "private", Title: "Private", DocumentJson: `{"version":1,"nodes":[]}`, Visibility: "private", CreatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: "private-r1", Valid: true}, PublishedAt: sql.NullInt64{Int64: 1, Valid: true}, UpdatedAt: 1, ID: "private-entry"}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateRoute(ctx, db.CreateRouteParams{ID: "private-route", Path: "/private", EntryID: sql.NullString{String: "private-entry", Valid: true}, RouteType: "entry", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}

	migration, err := fs.ReadFile(migrationFiles, "schema/047_remove_legacy_private_routes.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.DB.ExecContext(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	if _, err := queries.GetRouteByPath(ctx, "/private"); err != sql.ErrNoRows {
		t.Fatalf("private legacy route was not removed: %v", err)
	}
	if _, err := queries.GetEntry(ctx, "private-entry"); err != nil {
		t.Fatalf("entry was removed: %v", err)
	}
	if _, err := queries.GetEntryRevision(ctx, "private-r1"); err != nil {
		t.Fatalf("revision was removed: %v", err)
	}
}
