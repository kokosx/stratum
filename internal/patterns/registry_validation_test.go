package patterns

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func TestCorePatternsAgainstMigratedRegistry(t *testing.T) {
	dir := t.TempDir()
	database, err := storage.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx := context.Background()
	if err := database.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	queries := db.New(database.DB)
	reg, err := blocks.NewRegistry(ctx, queries)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	cat := NewCatalog()
	if err := cat.ValidateAll(reg); err != nil {
		t.Fatalf("ValidateAll with real registry: %v", err)
	}
	// Ensure canonical contexts
	for _, p := range cat.List("archive-template") {
		for _, c := range p.Contexts {
			if c == "layout-template" {
				t.Fatalf("pattern %s still uses layout-template", p.ID)
			}
		}
	}
	// Entry should have hero patterns
	found := false
	for _, p := range cat.List("entry") {
		if p.ID == "hero-centered" {
			found = true
		}
		if len(p.Contexts) == 0 {
			t.Fatalf("pattern %s has empty contexts", p.ID)
		}
	}
	if !found {
		t.Fatalf("hero-centered not in entry context")
	}
	// Archive-only pattern must not appear in entry
	for _, p := range cat.List("entry") {
		if p.ID == "archive-collection-grid" {
			t.Fatalf("archive pattern should not appear in entry")
		}
	}
	// Single-only pattern not in site-part?
	for _, p := range cat.List("site-part") {
		if p.ID == "single-article" {
			t.Fatalf("single-article should not appear in site-part")
		}
	}
	// Single should appear in single-template
	found = false
	for _, p := range cat.List("single-template") {
		if p.ID == "single-article" {
			found = true
		}
	}
	if !found {
		t.Fatalf("single-article not in single-template")
	}
}
