package routing

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func TestSyncContentTypeRoutingMovesSinglesArchiveAndRedirects(t *testing.T) {
	ctx := context.Background()
	database, err := storage.Open(filepath.Join(t.TempDir(), "routing.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	q := db.New(database.DB)
	if err := q.CreateContentType(ctx, db.CreateContentTypeParams{ID: "product", DisplayName: "Product", PluralName: "Products", Public: 1, ConfigJson: `{}`, CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := q.CreateEntry(ctx, db.CreateEntryParams{ID: "mac", ContentTypeID: "product", Slug: "mac", Status: "active", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := q.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "mac-r1", EntryID: "mac", RevisionNumber: 1, Slug: "macbook", Title: "MacBook", DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := q.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{ID: "mac", PublishedRevisionID: sql.NullString{String: "mac-r1", Valid: true}, PublishedAt: sql.NullInt64{Int64: 1, Valid: true}, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	for _, route := range []db.CreateRouteParams{{ID: "archive", Path: "/products", RouteType: RouteTypeArchive, ContentTypeID: sql.NullString{String: "product", Valid: true}, CreatedAt: 1, UpdatedAt: 1}, {ID: "single", Path: "/products/macbook", RouteType: RouteTypeEntry, EntryID: sql.NullString{String: "mac", Valid: true}, CreatedAt: 1, UpdatedAt: 1}} {
		if err := q.CreateRoute(ctx, route); err != nil {
			t.Fatal(err)
		}
	}
	if err := SyncContentTypeRouting(ctx, q, "product", "/products", "/catalog", true, 2); err != nil {
		t.Fatal(err)
	}
	for old, target := range map[string]string{"/products": "/catalog", "/products/macbook": "/catalog/macbook"} {
		route, err := q.GetRouteByPath(ctx, old)
		if err != nil || route.RouteType != RouteTypeRedirect || route.RedirectTo.String != target {
			t.Fatalf("redirect %s=%#v err=%v", old, route, err)
		}
	}
	if route, err := q.GetRouteByPath(ctx, "/catalog/macbook"); err != nil || route.EntryID.String != "mac" {
		t.Fatalf("new single=%#v err=%v", route, err)
	}
	if err := q.CreateRoute(ctx, db.CreateRouteParams{ID: "taken", Path: "/taken", RouteType: RouteTypeSystem, CreatedAt: 3, UpdatedAt: 3}); err != nil {
		t.Fatal(err)
	}
	if err := SyncContentTypeRouting(ctx, q, "product", "/catalog", "/taken", true, 4); err == nil {
		t.Fatal("collision accepted")
	}
	if _, err := q.GetRouteByPath(ctx, "/catalog/macbook"); err != nil {
		t.Fatalf("collision partially moved routes: %v", err)
	}
}
