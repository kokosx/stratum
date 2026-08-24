package storage

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

func TestDynamicBlockDefinitionWorksFromDatabaseWithoutApplicationCode(t *testing.T) {
	ctx := context.Background()
	database, err := Open(filepath.Join(t.TempDir(), "stratum.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	queries := db.New(database.DB)
	registry, err := blocks.NewRegistry(ctx, queries)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().Unix()
	schema := `{"schemaVersion":1,"props":{"type":"object","required":["title","description"],"properties":{"title":{"type":"string","default":""},"description":{"type":"string","default":""}}},"settings":{"type":"object","properties":{"variant":{"type":"string","enum":["plain","featured"],"default":"plain"}}},"children":{"mode":"none"},"editor":{"category":"test","fields":{"props.title":{"label":"Title","control":"text"},"props.description":{"label":"Description","control":"textarea"},"settings.variant":{"label":"Variant","control":"select","group":"Style"}}}}`
	if err := queries.CreateBlockDefinition(ctx, db.CreateBlockDefinitionParams{
		ID: "test-card-v1", Namespace: "test", Name: "card", Version: 1, DisplayName: "Test Card",
		Description: sql.NullString{String: "A database-defined card", Valid: true}, SchemaJson: schema,
		RendererType: "template", Template: sql.NullString{String: `<article class="card-{{ .Settings.variant }}"><h2>{{ .Props.title }}</h2><p>{{ .Props.description }}</p></article>`, Valid: true},
		Styles: sql.NullString{String: ".card-featured{font-weight:bold}", Valid: true}, Source: "test", Enabled: 1, CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatal(err)
	}
	if err := registry.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	found := false
	for _, definition := range registry.EditorCatalog() {
		if definition.Block == "test/card" && definition.Version == 1 {
			found = true
		}
	}
	if !found {
		t.Fatal("dynamic test/card@1 did not appear in editor catalog")
	}
	doc, err := document.Decode([]byte(`{"version":1,"nodes":[{"id":"card","block":"test/card","version":1,"props":{"title":"Hello","description":"From DB"},"settings":{"variant":"featured"}}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if err := registry.ValidateDocument(doc); err != nil {
		t.Fatal(err)
	}
	rendered, err := registry.RenderDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	if got := string(rendered); got != `<article class="card-featured"><h2>Hello</h2><p>From DB</p></article>` {
		t.Fatalf("dynamic render = %q", got)
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
		WHERE namespace = 'core' AND name = 'text' AND version = 2 AND enabled = 1
	`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("enabled core/text@2 definitions = %d, want 1", count)
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
	if !strings.Contains(string(rendered), `stratum-align-left`) || !strings.Contains(string(rendered), `fresh install`) {
		t.Fatalf("rendered core text = %q", rendered)
	}
}
