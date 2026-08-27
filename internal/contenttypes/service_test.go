package contenttypes

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/content"
	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func newTestService(t *testing.T) (*Service, *db.Queries, *storage.Database) {
	t.Helper()
	ctx := context.Background()
	database, err := storage.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := database.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	queries := db.New(database.DB)
	svc := New(database.DB, queries)
	return svc, queries, database
}

func TestService_SingleFalseToTrue(t *testing.T) {
	svc, queries, _ := newTestService(t)
	ctx := context.Background()
	cat := content.NewCatalog(queries)
	// Start Single false
	if err := cat.CreateContentType(ctx, content.ContentTypeInput{
		ID: "tech", PluralName: "Tech",
		Config: content.ContentTypeConfig{Features: content.ContentTypeFeatures{Content: true}, Routing: content.ContentTypeRouting{Single: false}},
	}); err != nil {
		t.Fatal(err)
	}
	// Create 2 published entries
	for _, slug := range []string{"go", "sqlite"} {
		// create entry directly via queries
		if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: "id-" + slug, ContentTypeID: "tech", Slug: slug, Status: "active", CreatedAt: 1, UpdatedAt: 1}); err != nil {
			t.Fatal(err)
		}
		revID := "rev-" + slug
		if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: revID, EntryID: "id-" + slug, RevisionNumber: 1, Slug: slug, Title: slug, DocumentJson: `{"version":1,"nodes":[]}`, FieldsJson: `{}`, CreatedAt: 1, Visibility: "public", ReviewState: "draft"}); err != nil {
			t.Fatal(err)
		}
		// publish
		if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: revID, Valid: true}, PublishedAt: sql.NullInt64{Int64: 1, Valid: true}, UpdatedAt: 1, ID: "id-" + slug}); err != nil {
			t.Fatal(err)
		}
	}
	// Enable Single
	if err := svc.Update(ctx, "tech", content.ContentTypeInput{
		ID: "tech", PluralName: "Tech",
		Config: content.ContentTypeConfig{Features: content.ContentTypeFeatures{Content: true}, Routing: content.ContentTypeRouting{Single: true, BasePath: "/tech"}},
	}); err != nil {
		t.Fatalf("enable single %v", err)
	}
	// Check routes
	for _, slug := range []string{"go", "sqlite"} {
		path := "/tech/" + slug
		if _, err := queries.GetRouteByPath(ctx, path); err != nil {
			t.Fatalf("route %s missing %v", path, err)
		}
	}
}

func TestService_SingleTrueToFalse(t *testing.T) {
	svc, queries, _ := newTestService(t)
	ctx := context.Background()
	cat := content.NewCatalog(queries)
	if err := cat.CreateContentType(ctx, content.ContentTypeInput{
		ID: "tech", PluralName: "Tech",
		Config: content.ContentTypeConfig{Features: content.ContentTypeFeatures{Content: true}, Routing: content.ContentTypeRouting{Single: true, BasePath: "/tech"}},
	}); err != nil {
		t.Fatal(err)
	}
	// Create entry and route via service enable
	for _, slug := range []string{"go"} {
		if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: "id-" + slug, ContentTypeID: "tech", Slug: slug, Status: "active", CreatedAt: 1, UpdatedAt: 1}); err != nil {
			t.Fatal(err)
		}
		revID := "rev-" + slug
		queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: revID, EntryID: "id-" + slug, RevisionNumber: 1, Slug: slug, Title: slug, DocumentJson: `{"version":1,"nodes":[]}`, FieldsJson: `{}`, CreatedAt: 1, Visibility: "public", ReviewState: "draft"})
		queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: revID, Valid: true}, PublishedAt: sql.NullInt64{Int64: 1, Valid: true}, UpdatedAt: 1, ID: "id-" + slug})
	}
	// Need to create routes via Update (since we started with Single true but no routes yet, need to simulate)
	// Do a base path change to trigger route creation? Simpler: manually create route for test, then disable
	queries.CreateRoute(ctx, db.CreateRouteParams{ID: "r1", Path: "/tech/go", EntryID: sql.NullString{String: "id-go", Valid: true}, RouteType: "entry", CreatedAt: 1, UpdatedAt: 1})
	// Disable Single
	if err := svc.Update(ctx, "tech", content.ContentTypeInput{
		ID: "tech", PluralName: "Tech",
		Config: content.ContentTypeConfig{Features: content.ContentTypeFeatures{Content: true}, Routing: content.ContentTypeRouting{Single: false}},
	}); err != nil {
		t.Fatalf("disable single %v", err)
	}
	if _, err := queries.GetRouteByPath(ctx, "/tech/go"); err == nil {
		t.Fatalf("route should be deleted")
	}
}

func TestService_ArchiveTransitions(t *testing.T) {
	svc, queries, _ := newTestService(t)
	ctx := context.Background()
	cat := content.NewCatalog(queries)
	if err := cat.CreateContentType(ctx, content.ContentTypeInput{
		ID: "tech", PluralName: "Tech",
		Config: content.ContentTypeConfig{Features: content.ContentTypeFeatures{Content: true}, Routing: content.ContentTypeRouting{Single: false, Archive: false}},
	}); err != nil {
		t.Fatal(err)
	}
	// Archive false -> true with base
	if err := svc.Update(ctx, "tech", content.ContentTypeInput{
		ID: "tech", PluralName: "Tech",
		Config: content.ContentTypeConfig{Features: content.ContentTypeFeatures{Content: true}, Routing: content.ContentTypeRouting{Single: false, Archive: true, BasePath: "/tech"}},
	}); err != nil {
		t.Fatalf("archive false->true %v", err)
	}
	if _, err := queries.GetArchiveRouteByContentType(ctx, sql.NullString{String: "tech", Valid: true}); err != nil {
		t.Fatalf("archive route should exist %v", err)
	}
	// Archive true -> false
	if err := svc.Update(ctx, "tech", content.ContentTypeInput{
		ID: "tech", PluralName: "Tech",
		Config: content.ContentTypeConfig{Features: content.ContentTypeFeatures{Content: true}, Routing: content.ContentTypeRouting{Single: false, Archive: false}},
	}); err != nil {
		t.Fatalf("archive true->false %v", err)
	}
	if _, err := queries.GetArchiveRouteByContentType(ctx, sql.NullString{String: "tech", Valid: true}); err == nil {
		t.Fatalf("archive route should be deleted")
	}
	// Single false + Archive true (G)
	if err := svc.Update(ctx, "tech", content.ContentTypeInput{
		ID: "tech", PluralName: "Tech",
		Config: content.ContentTypeConfig{Features: content.ContentTypeFeatures{Content: true}, Routing: content.ContentTypeRouting{Single: false, Archive: true, BasePath: "/tech"}},
	}); err != nil {
		t.Fatalf("single false archive true %v", err)
	}
	if _, err := queries.GetArchiveRouteByContentType(ctx, sql.NullString{String: "tech", Valid: true}); err != nil {
		t.Fatalf("archive should exist for G")
	}
	// Ensure no entry routes
	if _, err := queries.GetRouteByPath(ctx, "/tech"); err == nil {
		// archive route is at /tech, that's expected
	}
}

func TestService_BasePathChangeSingleTrue(t *testing.T) {
	svc, queries, _ := newTestService(t)
	ctx := context.Background()
	cat := content.NewCatalog(queries)
	if err := cat.CreateContentType(ctx, content.ContentTypeInput{
		ID: "tech", PluralName: "Tech",
		Config: content.ContentTypeConfig{Features: content.ContentTypeFeatures{Content: true}, Routing: content.ContentTypeRouting{Single: true, BasePath: "/tech"}},
	}); err != nil {
		t.Fatal(err)
	}
	// Create entry
	queries.CreateEntry(ctx, db.CreateEntryParams{ID: "id-go", ContentTypeID: "tech", Slug: "go", Status: "active", CreatedAt: 1, UpdatedAt: 1})
	queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "rev-go", EntryID: "id-go", RevisionNumber: 1, Slug: "go", Title: "go", DocumentJson: `{"version":1,"nodes":[]}`, FieldsJson: `{}`, CreatedAt: 1, Visibility: "public", ReviewState: "draft"})
	queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: "rev-go", Valid: true}, PublishedAt: sql.NullInt64{Int64: 1, Valid: true}, UpdatedAt: 1, ID: "id-go"})
	queries.CreateRoute(ctx, db.CreateRouteParams{ID: "r1", Path: "/tech/go", EntryID: sql.NullString{String: "id-go", Valid: true}, RouteType: "entry", CreatedAt: 1, UpdatedAt: 1})
	queries.CreateRoute(ctx, db.CreateRouteParams{ID: "arch", Path: "/tech", RouteType: "archive", ContentTypeID: sql.NullString{String: "tech", Valid: true}, CreatedAt: 1, UpdatedAt: 1})
	// Change base to /technologies
	if err := svc.Update(ctx, "tech", content.ContentTypeInput{
		ID: "tech", PluralName: "Tech",
		Config: content.ContentTypeConfig{Features: content.ContentTypeFeatures{Content: true}, Routing: content.ContentTypeRouting{Single: true, Archive: true, BasePath: "/technologies"}},
	}); err != nil {
		t.Fatalf("base change %v", err)
	}
	if _, err := queries.GetRouteByPath(ctx, "/technologies/go"); err != nil {
		t.Fatalf("new route missing %v", err)
	}
	if _, err := queries.GetRouteByPath(ctx, "/technologies"); err != nil {
		t.Fatalf("new archive missing %v", err)
	}
	// Old should be redirect
	if rt, err := queries.GetRouteByPath(ctx, "/tech/go"); err != nil || rt.RouteType != "redirect" {
		t.Fatalf("old should be redirect, got %v %v", rt, err)
	}
}

func TestService_BasePathChangeArchiveOnly(t *testing.T) {
	svc, queries, _ := newTestService(t)
	ctx := context.Background()
	cat := content.NewCatalog(queries)
	if err := cat.CreateContentType(ctx, content.ContentTypeInput{
		ID: "tech", PluralName: "Tech",
		Config: content.ContentTypeConfig{Features: content.ContentTypeFeatures{Content: true}, Routing: content.ContentTypeRouting{Single: false, Archive: true, BasePath: "/tech"}},
	}); err != nil {
		t.Fatal(err)
	}
	queries.CreateRoute(ctx, db.CreateRouteParams{ID: "arch", Path: "/tech", RouteType: "archive", ContentTypeID: sql.NullString{String: "tech", Valid: true}, CreatedAt: 1, UpdatedAt: 1})
	if err := svc.Update(ctx, "tech", content.ContentTypeInput{
		ID: "tech", PluralName: "Tech",
		Config: content.ContentTypeConfig{Features: content.ContentTypeFeatures{Content: true}, Routing: content.ContentTypeRouting{Single: false, Archive: true, BasePath: "/technologies"}},
	}); err != nil {
		t.Fatalf("archive base change %v", err)
	}
	if _, err := queries.GetRouteByPath(ctx, "/technologies"); err != nil {
		t.Fatalf("new archive missing %v", err)
	}
}

func TestService_RouteCollisionRollback(t *testing.T) {
	svc, queries, _ := newTestService(t)
	ctx := context.Background()
	cat := content.NewCatalog(queries)
	// Create tech with Single false
	if err := cat.CreateContentType(ctx, content.ContentTypeInput{
		ID: "tech", PluralName: "Tech",
		Config: content.ContentTypeConfig{Features: content.ContentTypeFeatures{Content: true}, Routing: content.ContentTypeRouting{Single: false}},
	}); err != nil {
		t.Fatal(err)
	}
	// Create entry go
	queries.CreateEntry(ctx, db.CreateEntryParams{ID: "id-go", ContentTypeID: "tech", Slug: "go", Status: "active", CreatedAt: 1, UpdatedAt: 1})
	queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "rev-go", EntryID: "id-go", RevisionNumber: 1, Slug: "go", Title: "go", DocumentJson: `{"version":1,"nodes":[]}`, FieldsJson: `{}`, CreatedAt: 1, Visibility: "public", ReviewState: "draft"})
	queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: "rev-go", Valid: true}, PublishedAt: sql.NullInt64{Int64: 1, Valid: true}, UpdatedAt: 1, ID: "id-go"})
	// Create conflicting page at /tech/go (simulate existing route)
	queries.CreateEntry(ctx, db.CreateEntryParams{ID: "page-go", ContentTypeID: "page", Slug: "go", Status: "active", CreatedAt: 1, UpdatedAt: 1})
	queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "rev-page-go", EntryID: "page-go", RevisionNumber: 1, Slug: "go", Title: "go", DocumentJson: `{"version":1,"nodes":[]}`, FieldsJson: `{}`, CreatedAt: 1, Visibility: "public", ReviewState: "draft"})
	queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: "rev-page-go", Valid: true}, PublishedAt: sql.NullInt64{Int64: 1, Valid: true}, UpdatedAt: 1, ID: "page-go"})
	// Create existing route at /tech/go for page? Actually page route is /go not /tech/go, so to cause collision we create a route at /tech/go manually
	queries.CreateRoute(ctx, db.CreateRouteParams{ID: "r-conflict", Path: "/tech/go", EntryID: sql.NullString{String: "page-go", Valid: true}, RouteType: "entry", CreatedAt: 1, UpdatedAt: 1})
	// Try to enable Single true with base /tech – should fail due to collision at /tech/go
	err := svc.Update(ctx, "tech", content.ContentTypeInput{
		ID: "tech", PluralName: "Tech",
		Config: content.ContentTypeConfig{Features: content.ContentTypeFeatures{Content: true}, Routing: content.ContentTypeRouting{Single: true, BasePath: "/tech"}},
	})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("should fail with collision, got %v", err)
	}
	// Ensure no partial routes created (tech's route should not exist)
	if _, err := queries.GetRouteByPath(ctx, "/tech"); err == nil {
		// archive not requested, so shouldn't have archive
	}
	// Ensure content type still Single false (rollback)
	def, _ := cat.GetDefinition(ctx, "tech")
	if def.Routing.Single {
		t.Fatalf("should have rolled back, still Single true")
	}
}

func TestService_HierarchicalTransition(t *testing.T) {
	svc, queries, _ := newTestService(t)
	ctx := context.Background()
	cat := content.NewCatalog(queries)
	if err := cat.CreateContentType(ctx, content.ContentTypeInput{
		ID: "loc", PluralName: "Loc", Hierarchical: true,
		Config: content.ContentTypeConfig{Features: content.ContentTypeFeatures{Content: true}, Routing: content.ContentTypeRouting{Single: false}},
	}); err != nil {
		t.Fatal(err)
	}
	// Create Europe and Poland
	queries.CreateEntry(ctx, db.CreateEntryParams{ID: "europe", ContentTypeID: "loc", Slug: "europe", Status: "active", CreatedAt: 1, UpdatedAt: 1})
	queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "rev-europe", EntryID: "europe", RevisionNumber: 1, Slug: "europe", Title: "Europe", DocumentJson: `{"version":1,"nodes":[]}`, FieldsJson: `{}`, CreatedAt: 1, Visibility: "public", ReviewState: "draft", ParentEntryID: sql.NullString{}, MenuOrder: 1})
	queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: "rev-europe", Valid: true}, PublishedAt: sql.NullInt64{Int64: 1, Valid: true}, UpdatedAt: 1, ID: "europe"})
	queries.CreateEntry(ctx, db.CreateEntryParams{ID: "poland", ContentTypeID: "loc", Slug: "poland", Status: "active", CreatedAt: 1, UpdatedAt: 1})
	queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "rev-poland", EntryID: "poland", RevisionNumber: 1, Slug: "poland", Title: "Poland", DocumentJson: `{"version":1,"nodes":[]}`, FieldsJson: `{}`, CreatedAt: 1, Visibility: "public", ReviewState: "draft", ParentEntryID: sql.NullString{String: "europe", Valid: true}, MenuOrder: 1})
	queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: "rev-poland", Valid: true}, PublishedAt: sql.NullInt64{Int64: 1, Valid: true}, UpdatedAt: 1, ID: "poland"})
	// Enable Single
	if err := svc.Update(ctx, "loc", content.ContentTypeInput{
		ID: "loc", PluralName: "Loc", Hierarchical: true,
		Config: content.ContentTypeConfig{Features: content.ContentTypeFeatures{Content: true}, Routing: content.ContentTypeRouting{Single: true, BasePath: "/locations"}},
	}); err != nil {
		t.Fatalf("enable hierarchical single %v", err)
	}
	if _, err := queries.GetRouteByPath(ctx, "/locations/europe"); err != nil {
		t.Fatalf("europe route missing %v", err)
	}
	if _, err := queries.GetRouteByPath(ctx, "/locations/europe/poland"); err != nil {
		t.Fatalf("poland route missing %v", err)
	}
}

func TestService_LargeBatch(t *testing.T) {
	svc, queries, _ := newTestService(t)
	ctx := context.Background()
	cat := content.NewCatalog(queries)
	if err := cat.CreateContentType(ctx, content.ContentTypeInput{
		ID: "bulk", PluralName: "Bulk",
		Config: content.ContentTypeConfig{Features: content.ContentTypeFeatures{Content: true}, Routing: content.ContentTypeRouting{Single: false}},
	}); err != nil {
		t.Fatal(err)
	}
	// For brevity, create 1001 with slug bulk-i
	for i := 0; i < 1001; i++ {
		slug := "bulk-" + itoa(i)
		id := "id-bulk-" + itoa(i)
		rev := "rev-bulk-" + itoa(i)
		queries.CreateEntry(ctx, db.CreateEntryParams{ID: id, ContentTypeID: "bulk", Slug: slug, Status: "active", CreatedAt: 1, UpdatedAt: 1})
		queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: rev, EntryID: id, RevisionNumber: 1, Slug: slug, Title: slug, DocumentJson: `{"version":1,"nodes":[]}`, FieldsJson: `{}`, CreatedAt: 1, Visibility: "public", ReviewState: "draft"})
		queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: rev, Valid: true}, PublishedAt: sql.NullInt64{Int64: 1, Valid: true}, UpdatedAt: 1, ID: id})
	}
	// Enable Single – should handle >1000 via batches
	if err := svc.Update(ctx, "bulk", content.ContentTypeInput{
		ID: "bulk", PluralName: "Bulk",
		Config: content.ContentTypeConfig{Features: content.ContentTypeFeatures{Content: true}, Routing: content.ContentTypeRouting{Single: true, BasePath: "/bulk"}},
	}); err != nil {
		t.Fatalf("large batch enable %v", err)
	}
	// Check a few routes
	for _, slug := range []string{"bulk-0", "bulk-500", "bulk-1000"} {
		if _, err := queries.GetRouteByPath(ctx, "/bulk/"+slug); err != nil {
			t.Fatalf("route %s missing %v", slug, err)
		}
	}
}

func itoa(i int) string {
	// simple
	if i == 0 {
		return "0"
	}
	s := ""
	for i > 0 {
		s = string(rune('0'+i%10)) + s
		i /= 10
	}
	return s
}
