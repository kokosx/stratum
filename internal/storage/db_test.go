package storage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/document"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func TestOpenEnablesForeignKeys(t *testing.T) {
	ctx := context.Background()
	database, err := Open(filepath.Join(t.TempDir(), "stratum.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	var enabled bool
	if err := database.DB.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&enabled); err != nil {
		t.Fatal(err)
	}
	if !enabled {
		t.Fatal("foreign key enforcement is disabled")
	}
	if _, err := database.DB.ExecContext(ctx, `
		INSERT INTO entries (id, content_type_id, slug, created_at, updated_at)
		VALUES ('orphan', 'missing-content-type', 'orphan', 1, 1)
	`); err == nil {
		t.Fatal("database accepted an entry with a missing content type")
	}
}

func TestMigrateInstallsCoreBlockDefinitions(t *testing.T) {
	ctx := context.Background()
	database, err := Open(filepath.Join(t.TempDir(), "stratum.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}

	var count int
	if err := database.DB.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM block_definitions
		WHERE namespace = 'core' AND name = 'text' AND version = 1 AND enabled = 1
	`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("enabled core/text@1 definitions = %d, want 1", count)
	}
	registry, err := blocks.NewRegistry(ctx, db.New(database.DB))
	if err != nil {
		t.Fatal(err)
	}
	doc, err := document.Decode([]byte(`{"version":1,"nodes":[{"id":"text","block":"core/text","version":1,"props":{"text":"fresh install"}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	rendered, err := registry.RenderDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(rendered), "<p>fresh install</p>") {
		t.Fatalf("rendered core text = %q", rendered)
	}
}
