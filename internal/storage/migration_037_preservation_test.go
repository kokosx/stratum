package storage

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestMigration037PreservesLayoutDataAndFK(t *testing.T) {
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
	// Ensure parent column exists to simulate pre-037 state (fresh DB already has 037, so re-add)
	var cnt int
	_ = database.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('layout_templates') WHERE name='parent_template_id'`).Scan(&cnt)
	if cnt == 0 {
		if _, err := database.DB.ExecContext(ctx, `ALTER TABLE layout_templates ADD COLUMN parent_template_id TEXT REFERENCES layout_templates(id) ON DELETE SET NULL`); err != nil {
			t.Fatalf("add parent column: %v", err)
		}
		if _, err := database.DB.ExecContext(ctx, `CREATE INDEX idx_layout_templates_parent ON layout_templates(parent_template_id)`); err != nil {
			t.Fatalf("create index: %v", err)
		}
		// Mark 035 as not applied to simulate pre-037? Not needed for data test, just need column
	}
	// Create layout template T1 with revision R1
	if _, err := database.DB.ExecContext(ctx, `INSERT INTO layout_templates (id, name, content_type_id, published_revision_id, created_at, updated_at, parent_template_id) VALUES ('t1','Test','page',NULL,1,1,NULL)`); err != nil {
		t.Fatalf("insert t1: %v", err)
	}
	if _, err := database.DB.ExecContext(ctx, `INSERT INTO layout_template_revisions (id, template_id, revision_number, document_json, created_at) VALUES ('rev1','t1',1,'{"version":1,"nodes":[{"id":"slot","block":"core/content-slot","version":1,"props":{},"settings":{}}]}',1)`); err != nil {
		t.Fatalf("insert rev1: %v", err)
	}
	if _, err := database.DB.ExecContext(ctx, `UPDATE layout_templates SET published_revision_id='rev1' WHERE id='t1'`); err != nil {
		t.Fatalf("update pub: %v", err)
	}
	if _, err := database.DB.ExecContext(ctx, `UPDATE content_types SET default_layout_template_id='t1' WHERE id='page'`); err != nil {
		t.Fatalf("update default: %v", err)
	}
	if _, err := database.DB.ExecContext(ctx, `INSERT INTO entries (id, content_type_id, slug, status, created_at, updated_at) VALUES ('e1','page','test','active',1,1)`); err != nil {
		t.Fatalf("insert e1: %v", err)
	}
	if _, err := database.DB.ExecContext(ctx, `INSERT INTO entry_revisions (id, entry_id, revision_number, title, document_json, layout_template_id, created_at) VALUES ('er1','e1',1,'T','{"version":1,"nodes":[]}','t1',1)`); err != nil {
		t.Fatalf("insert er1: %v", err)
	}
	// Verify FK before
	var fkBefore int
	_ = database.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_foreign_key_check`).Scan(&fkBefore)
	if fkBefore != 0 {
		t.Fatalf("fk before %d", fkBefore)
	}
	// Now run 037 as migration runner would (BeginTx + Exec file)
	sqlBytes, err := os.ReadFile(filepath.Join("schema", "037_remove_layout_parent.sql"))
	if err != nil {
		// Fallback to embedded via migrationFiles? Try alternate path
		sqlBytes, _ = os.ReadFile("internal/storage/schema/037_remove_layout_parent.sql")
		if len(sqlBytes) == 0 {
			t.Fatalf("read 037: %v", err)
		}
	}
	tx, err := database.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.ExecContext(ctx, string(sqlBytes)); err != nil {
		tx.Rollback()
		t.Fatalf("exec 037: %v", err)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit 037: %v", err)
	}
	// Assert preservation
	var name, pub string
	if err := database.DB.QueryRowContext(ctx, `SELECT name, published_revision_id FROM layout_templates WHERE id='t1'`).Scan(&name, &pub); err != nil {
		t.Fatalf("t1 not preserved: %v", err)
	}
	if name != "Test" || pub != "rev1" {
		t.Fatalf("t1 mismatch %q %q", name, pub)
	}
	var revDoc string
	if err := database.DB.QueryRowContext(ctx, `SELECT document_json FROM layout_template_revisions WHERE id='rev1'`).Scan(&revDoc); err != nil || revDoc == "" {
		t.Fatalf("rev1 not preserved: %v", err)
	}
	var def string
	if err := database.DB.QueryRowContext(ctx, `SELECT default_layout_template_id FROM content_types WHERE id='page'`).Scan(&def); err != nil || def != "t1" {
		t.Fatalf("default not preserved %q err %v", def, err)
	}
	var erTpl string
	if err := database.DB.QueryRowContext(ctx, `SELECT layout_template_id FROM entry_revisions WHERE id='er1'`).Scan(&erTpl); err != nil || erTpl != "t1" {
		t.Fatalf("er1 not preserved %q err %v", erTpl, err)
	}
	var hasParent int
	_ = database.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('layout_templates') WHERE name='parent_template_id'`).Scan(&hasParent)
	if hasParent != 0 {
		t.Fatalf("parent column still exists")
	}
	var idxCnt int
	_ = database.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name='idx_layout_templates_parent'`).Scan(&idxCnt)
	if idxCnt != 0 {
		t.Fatalf("parent index still exists")
	}
	var fkAfter int
	_ = database.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_foreign_key_check`).Scan(&fkAfter)
	if fkAfter != 0 {
		t.Fatalf("fk after %d, want 0", fkAfter)
	}
}

func TestDropColumnSupportOnTurso(t *testing.T) {
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
	// Ensure parent column exists
	var cnt int
	_ = database.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_table_info('layout_templates') WHERE name='parent_template_id'`).Scan(&cnt)
	if cnt == 0 {
		if _, err := database.DB.ExecContext(ctx, `ALTER TABLE layout_templates ADD COLUMN parent_template_id TEXT REFERENCES layout_templates(id) ON DELETE SET NULL`); err != nil {
			t.Fatalf("add parent: %v", err)
		}
		if _, err := database.DB.ExecContext(ctx, `CREATE INDEX idx_layout_templates_parent ON layout_templates(parent_template_id)`); err != nil {
			t.Fatalf("create idx: %v", err)
		}
	}
	// Try DROP COLUMN inside transaction as migration runner would
	tx, err := database.DB.BeginTx(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	_, err = tx.ExecContext(ctx, `DROP INDEX IF EXISTS idx_layout_templates_parent; ALTER TABLE layout_templates DROP COLUMN parent_template_id;`)
	if err == nil {
		tx.Commit()
		t.Logf("DROP COLUMN succeeded on this driver – 037 could use simple ALTER TABLE")
		// Verify FK check still clean
		var fk int
		_ = database.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM pragma_foreign_key_check`).Scan(&fk)
		if fk != 0 {
			t.Fatalf("fk after drop %d", fk)
		}
	} else {
		tx.Rollback()
		t.Logf("DROP COLUMN failed as expected on tursogo (self-FK): %v", err)
		// This is the expected path for current Turso, so rebuild is required
		if cnt == 0 {
			// Clean up added column for other tests
			_, _ = database.DB.ExecContext(ctx, `DROP INDEX IF EXISTS idx_layout_templates_parent`)
			_, _ = database.DB.ExecContext(ctx, `ALTER TABLE layout_templates DROP COLUMN parent_template_id`)
		}
	}
}
