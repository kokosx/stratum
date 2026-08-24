package navigation

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"testing"

	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func TestMultipleMenusKeepIndependentTargetsTreesLocationsAndOrder(t *testing.T) {
	ctx, database, queries := navigationTestDatabase(t)
	createPublishedPage(t, ctx, queries, "page-one", "one", "Page One", "/old-path", "active")

	service := NewService(database.DB, queries)
	loader := NewLoader(queries)
	primary, err := service.CreateMenu(ctx, "Test Primary")
	if err != nil {
		t.Fatal(err)
	}
	footer, err := service.CreateMenu(ctx, "Test Footer")
	if err != nil {
		t.Fatal(err)
	}

	primaryItems := []ItemInput{
		{ID: "entry", Position: 10, Label: "Current page", TargetType: "entry", EntryID: "page-one"},
		{ID: "external", ParentID: "entry", Position: 80, Label: "External", TargetType: "url", URL: "https://example.com"},
		{ID: "internal", Position: 20, Label: "Catalog", TargetType: "url", URL: "/files/catalog.pdf"},
		{ID: "anchor", Position: 30, Label: "Contact", TargetType: "url", URL: "#contact"},
	}
	if err := service.SaveMenu(ctx, primary.ID, "Test Primary Renamed", primaryItems, []string{"primary"}); err != nil {
		t.Fatal(err)
	}
	if err := service.SaveMenu(ctx, footer.ID, footer.Name, []ItemInput{{ID: "fixed", Label: "Fixed old path", TargetType: "url", URL: "/old-path"}}, []string{"footer"}); err != nil {
		t.Fatal(err)
	}

	locations, err := loader.LoadLocations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if locations["primary"].ID != primary.ID || locations["footer"].ID != footer.ID {
		t.Fatalf("location assignments = primary:%q footer:%q", locations["primary"].ID, locations["footer"].ID)
	}
	items := locations["primary"].Items
	if len(items) != 3 || items[0].ID != "entry" || items[1].ID != "internal" || items[2].ID != "anchor" {
		t.Fatalf("primary order = %#v", items)
	}
	if items[0].URL != "/old-path" || len(items[0].Children) != 1 || items[0].Children[0].URL != "https://example.com" {
		t.Fatalf("resolved nested entry = %#v", items[0])
	}
	if locations["footer"].Items[0].URL != "/old-path" {
		t.Fatalf("fixed URL = %q", locations["footer"].Items[0].URL)
	}

	route, err := queries.GetEntryRoute(ctx, sql.NullString{String: "page-one", Valid: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := queries.UpdateRoute(ctx, db.UpdateRouteParams{ID: route.ID, Path: "/new-path", EntryID: route.EntryID, RouteType: route.RouteType, RedirectTo: route.RedirectTo, RedirectStatus: route.RedirectStatus, UpdatedAt: 20}); err != nil {
		t.Fatal(err)
	}
	locations, err = loader.LoadLocations(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if got := locations["primary"].Items[0].URL; got != "/new-path" {
		t.Fatalf("entry URL after route change = %q", got)
	}
	if got := locations["footer"].Items[0].URL; got != "/old-path" {
		t.Fatalf("fixed URL after route change = %q", got)
	}

	rows, err := queries.ListNavigationItemsByMenu(ctx, primary.ID)
	if err != nil {
		t.Fatal(err)
	}
	positions := map[string]int64{}
	for _, row := range rows {
		positions[row.ID] = row.Position
	}
	if positions["entry"] != 0 || positions["internal"] != 1 || positions["anchor"] != 2 || positions["external"] != 0 {
		t.Fatalf("normalized sibling positions = %#v", positions)
	}
}

func TestSaveMenuRejectsParentCyclesAndInconsistentTargets(t *testing.T) {
	ctx, database, queries := navigationTestDatabase(t)
	service := NewService(database.DB, queries)
	menu, err := service.CreateMenu(ctx, "Cycle Test")
	if err != nil {
		t.Fatal(err)
	}
	err = service.SaveMenu(ctx, menu.ID, menu.Name, []ItemInput{
		{ID: "a", ParentID: "b", Label: "A", TargetType: "url", URL: "/a"},
		{ID: "b", ParentID: "a", Label: "B", TargetType: "url", URL: "/b"},
	}, nil)
	if !errors.Is(err, ErrInvalidMenu) {
		t.Fatalf("cycle error = %v", err)
	}
	err = service.SaveMenu(ctx, menu.ID, menu.Name, []ItemInput{{ID: "bad", Label: "Bad", TargetType: "entry", EntryID: "missing", URL: "/also-set"}}, nil)
	if !errors.Is(err, ErrInvalidMenu) {
		t.Fatalf("inconsistent target error = %v", err)
	}
}

func TestGroupItemCreatesANestedDropdownWithoutAURL(t *testing.T) {
	ctx, database, queries := navigationTestDatabase(t)
	service := NewService(database.DB, queries)
	menu, err := service.CreateMenu(ctx, "Groups")
	if err != nil {
		t.Fatal(err)
	}
	if err := service.SaveMenu(ctx, menu.ID, menu.Name, []ItemInput{
		{ID: "services", Label: "Services", TargetType: "group"},
		{ID: "design", ParentID: "services", Label: "Design", TargetType: "url", URL: "/design"},
	}, nil); err != nil {
		t.Fatal(err)
	}
	loaded, err := NewLoader(queries).LoadMenu(ctx, menu.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Items) != 1 || loaded.Items[0].URL != "" || len(loaded.Items[0].Children) != 1 || loaded.Items[0].Children[0].URL != "/design" {
		t.Fatalf("group menu = %#v", loaded)
	}
}

func TestNonPublicAndDeletedEntryTargetsAreRetainedForAdminButHiddenPublicly(t *testing.T) {
	ctx, database, queries := navigationTestDatabase(t)
	createPublishedPage(t, ctx, queries, "private-page", "private", "Private", "/private", "private")
	createPublishedPage(t, ctx, queries, "deleted-page", "deleted", "Deleted", "/deleted", "active")
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: "draft-page", ContentTypeID: "page", Slug: "draft", Status: "active", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	service := NewService(database.DB, queries)
	loader := NewLoader(queries)
	menu, err := service.CreateMenu(ctx, "Visibility Test")
	if err != nil {
		t.Fatal(err)
	}
	items := []ItemInput{
		{ID: "private-item", Label: "Private", TargetType: "entry", EntryID: "private-page"},
		{ID: "deleted-item", Label: "Deleted", TargetType: "entry", EntryID: "deleted-page"},
		{ID: "draft-item", Label: "Draft", TargetType: "entry", EntryID: "draft-page"},
	}
	if err := service.SaveMenu(ctx, menu.ID, menu.Name, items, nil); err != nil {
		t.Fatal(err)
	}
	if err := queries.DeleteEntry(ctx, "deleted-page"); err != nil {
		t.Fatal(err)
	}

	publicMenu, err := loader.LoadMenu(ctx, menu.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(publicMenu.Items) != 0 {
		t.Fatalf("non-public items rendered: %#v", publicMenu.Items)
	}
	adminItems, err := loader.LoadAdminItems(ctx, menu.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(adminItems) != 3 || adminItems[0].Public || adminItems[1].Public || adminItems[2].Public {
		t.Fatalf("admin visibility state = %#v", adminItems)
	}
}

func navigationTestDatabase(t *testing.T) (context.Context, *storage.Database, *db.Queries) {
	t.Helper()
	ctx := context.Background()
	database, err := storage.Open(filepath.Join(t.TempDir(), "stratum.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	return ctx, database, db.New(database.DB)
}

func createPublishedPage(t *testing.T, ctx context.Context, queries *db.Queries, id, slug, title, path, status string) {
	t.Helper()
	revisionID := id + "-r1"
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: id, ContentTypeID: "page", Slug: slug, Status: status, CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: revisionID, EntryID: id, RevisionNumber: 1, Title: title, DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: revisionID, Valid: true}, PublishedAt: sql.NullInt64{Int64: 1, Valid: true}, UpdatedAt: 1, ID: id}); err != nil {
		t.Fatal(err)
	}
	if status != "active" {
		entry, getErr := queries.GetEntry(ctx, id)
		if getErr != nil {
			t.Fatal(getErr)
		}
		if err := queries.UpdateEntryProjection(ctx, db.UpdateEntryProjectionParams{Slug: slug, Status: status, UpdatedAt: 2, PublishedAt: entry.PublishedAt, ID: id}); err != nil {
			t.Fatal(err)
		}
	}
	if err := queries.CreateRoute(ctx, db.CreateRouteParams{ID: id + "-route", Path: path, EntryID: sql.NullString{String: id, Valid: true}, RouteType: "entry", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
}
