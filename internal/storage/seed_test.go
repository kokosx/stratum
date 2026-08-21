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
	registry, err := blocks.NewRegistry(ctx, queries)
	if err != nil {
		t.Fatal(err)
	}
	content, err := registry.RenderDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), `stratum-align-left`) || !strings.Contains(string(content), `Welcome to StratumCMS`) {
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
