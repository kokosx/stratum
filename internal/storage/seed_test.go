package storage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/document"
	"github.com/kokosx/stratum/internal/rendering"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func TestSeedCreatesAnIdempotentPublishedHomepage(t *testing.T) {
	ctx := context.Background()
	database, err := Open(filepath.Join(t.TempDir(), "data", "stratum.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := database.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	if err := database.Seed(ctx); err != nil {
		t.Fatal(err)
	}

	queries := db.New(database.DB)
	entry, err := queries.GetPublishedEntryByPath(ctx, "/")
	if err != nil {
		t.Fatal(err)
	}
	if entry.Title != "Welcome to StratumCMS" {
		t.Errorf("seeded title = %q, want Welcome to StratumCMS", entry.Title)
	}
	doc, err := document.Decode([]byte(entry.DocumentJson))
	if err != nil {
		t.Fatal(err)
	}
	blockDefinitions, err := queries.ListBlockDefinitions(ctx)
	if err != nil {
		t.Fatal(err)
	}
	definitions := make([]rendering.Definition, 0, len(blockDefinitions))
	for _, definition := range blockDefinitions {
		definitions = append(definitions, rendering.Definition{
			Namespace: definition.Namespace, Name: definition.Name, Version: definition.Version,
			RendererType: definition.RendererType, Template: definition.Template.String,
		})
	}
	renderer, err := rendering.NewRenderer(definitions)
	if err != nil {
		t.Fatal(err)
	}
	content, err := renderer.RenderDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "<h2>Welcome to StratumCMS</h2>") {
		t.Errorf("seeded templates rendered %q", content)
	}

	settings, err := queries.GetSiteSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !settings.HomepageEntryID.Valid || settings.HomepageEntryID.String != seedHomeEntryID {
		t.Errorf("homepage entry = %#v, want %q", settings.HomepageEntryID, seedHomeEntryID)
	}
}
