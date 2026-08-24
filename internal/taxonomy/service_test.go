package taxonomy

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func newTestService(t *testing.T) (*Service, *storage.Database, *db.Queries) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	database, err := storage.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	queries := db.New(database.DB)
	svc := New(database.DB, queries)
	return svc, database, queries
}

func TestTaxonomyBuiltinHierarchical(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	taxonomies, err := svc.ListTaxonomies(ctx)
	if err != nil {
		t.Fatal(err)
	}
	m := map[string]Taxonomy{}
	for _, tx := range taxonomies {
		m[tx.ID] = tx
	}
	cat, ok := m["category"]
	if !ok {
		t.Fatal("category not found")
	}
	if !cat.Hierarchical {
		t.Fatal("category should be hierarchical")
	}
	if !cat.Public {
		t.Fatal("category should be public")
	}
	tag, ok := m["tag"]
	if !ok {
		t.Fatal("tag not found")
	}
	if tag.Hierarchical {
		t.Fatal("tag should be flat")
	}
}

func TestCreateUpdateDeleteTerm(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	term, err := svc.CreateTerm(ctx, "category", "News", "news", "desc", "")
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if term.Slug != "news" {
		t.Fatalf("slug %s", term.Slug)
	}
	updated, err := svc.UpdateTerm(ctx, term.ID, "Company News", "company-news", "new desc", "")
	if err != nil {
		t.Fatalf("update: %v", err)
	}
	if updated.Slug != "company-news" {
		t.Fatalf("updated slug %s", updated.Slug)
	}
	if err := svc.DeleteTerm(ctx, term.ID); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestDuplicateSlugSameTaxonomyRejected(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	_, err := svc.CreateTerm(ctx, "category", "News", "news", "", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.CreateTerm(ctx, "category", "Other", "news", "", "")
	if err != ErrDuplicateSlug {
		t.Fatalf("expected duplicate, got %v", err)
	}
}

func TestSameSlugDifferentTaxonomyAllowed(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	_, err := svc.CreateTerm(ctx, "category", "News", "news", "", "")
	if err != nil {
		t.Fatal(err)
	}
	_, err = svc.CreateTerm(ctx, "tag", "News", "news", "", "")
	if err != nil {
		t.Fatalf("same slug different taxonomy should be allowed: %v", err)
	}
}

func TestCrossTaxonomyParentRejected(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	cat, _ := svc.CreateTerm(ctx, "category", "Parent", "parent", "", "")
	tag, _ := svc.CreateTerm(ctx, "tag", "TagParent", "tagparent", "", "")
	_, err := svc.CreateTerm(ctx, "category", "Child", "child", "", tag.ID)
	if err != ErrParentSameTax {
		t.Fatalf("expected parent same tax error, got %v", err)
	}
	_, err = svc.UpdateTerm(ctx, cat.ID, "Parent", "parent", "", tag.ID)
	if err != ErrParentSameTax {
		t.Fatalf("expected parent same tax on update, got %v", err)
	}
}

func TestTagParentRejected(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	parent, _ := svc.CreateTerm(ctx, "category", "CatParent", "catparent", "", "")
	_, err := svc.CreateTerm(ctx, "tag", "TagChild", "tagchild", "", parent.ID)
	if err != ErrParentNotAllowed {
		t.Fatalf("expected parent not allowed for tag, got %v", err)
	}
}

func TestCategoryCycleRejected(t *testing.T) {
	svc, _, _ := newTestService(t)
	ctx := context.Background()
	a, _ := svc.CreateTerm(ctx, "category", "A", "a", "", "")
	b, _ := svc.CreateTerm(ctx, "category", "B", "b", "", a.ID)
	_, err := svc.UpdateTerm(ctx, a.ID, "A", "a", "", b.ID)
	if err != ErrCycle {
		t.Fatalf("expected cycle, got %v", err)
	}
}
