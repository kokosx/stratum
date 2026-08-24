package routing

import (
	"context"
	"database/sql"
	"testing"

	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func TestChildEntryPathUsesEffectiveParentPath(t *testing.T) {
	if got := ChildEntryPath("/", "about"); got != "/about" {
		t.Fatalf("homepage child = %q, want /about", got)
	}
	if got := ChildEntryPath("/company", "team"); got != "/company/team" {
		t.Fatalf("nested child = %q, want /company/team", got)
	}
}

func TestSyncHierarchyPublishMovesPublishedSubtreeAndRedirects(t *testing.T) {
	runtime, queries, cleanup := newTestRuntime(t)
	defer cleanup()
	_ = runtime
	ctx := context.Background()
	_ = queries.CreateContentType(ctx, db.CreateContentTypeParams{ID: "page", DisplayName: "Page", PluralName: "Pages", Hierarchical: 1, Public: 1, ConfigJson: "{}"})
	for _, entry := range []struct{ id, slug string }{{"company", "company"}, {"team", "team"}} {
		if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: entry.id, ContentTypeID: "page", Slug: entry.slug, Status: "active", CreatedAt: 1, UpdatedAt: 1}); err != nil {
			t.Fatal(err)
		}
	}
	for _, revision := range []struct{ id, entryID, parent string }{{"company-r1", "company", ""}, {"team-r1", "team", "company"}} {
		if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: revision.id, EntryID: revision.entryID, RevisionNumber: 1, Title: revision.entryID, DocumentJson: `{"version":1,"nodes":[]}`, ParentEntryID: nullRevisionParent(revision.parent), CreatedAt: 1}); err != nil {
			t.Fatal(err)
		}
		if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{ID: revision.entryID, PublishedRevisionID: sql.NullString{String: revision.id, Valid: true}, PublishedAt: sql.NullInt64{Int64: 1, Valid: true}, UpdatedAt: 1}); err != nil {
			t.Fatal(err)
		}
	}
	for _, route := range []struct{ id, path, entryID string }{{"company-route", "/company", "company"}, {"team-route", "/company/team", "team"}} {
		if err := queries.CreateRoute(ctx, db.CreateRouteParams{ID: route.id, Path: route.path, EntryID: sql.NullString{String: route.entryID, Valid: true}, RouteType: RouteTypeEntry, CreatedAt: 1, UpdatedAt: 1}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := SyncHierarchyPublish(ctx, queries, HierarchyEntry{EntryID: "company", ContentTypeID: "page", Slug: "about", Title: "company"}, 2); err != nil {
		t.Fatal(err)
	}
	for _, want := range []struct{ path, kind, target string }{{"/about", RouteTypeEntry, ""}, {"/about/team", RouteTypeEntry, ""}, {"/company", RouteTypeRedirect, "/about"}, {"/company/team", RouteTypeRedirect, "/about/team"}} {
		route, err := queries.GetRouteByPath(ctx, want.path)
		if err != nil || route.RouteType != want.kind || (want.target != "" && route.RedirectTo.String != want.target) {
			t.Fatalf("route %s = %#v, %v", want.path, route, err)
		}
	}
}

func TestHierarchyQueriesKeepDraftReparentOutOfPublishedGraph(t *testing.T) {
	_, queries, cleanup := newTestRuntime(t)
	defer cleanup()
	ctx := context.Background()
	_ = queries.CreateContentType(ctx, db.CreateContentTypeParams{ID: "page", DisplayName: "Page", PluralName: "Pages", Hierarchical: 1, Public: 1, ConfigJson: "{}"})
	for _, entry := range []string{"about", "company", "team"} {
		if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: entry, ContentTypeID: "page", Slug: entry, Status: "active", CreatedAt: 1, UpdatedAt: 1}); err != nil {
			t.Fatal(err)
		}
		if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: entry + "-r1", EntryID: entry, RevisionNumber: 1, Title: entry, DocumentJson: `{"version":1,"nodes":[]}`, ParentEntryID: nullRevisionParent(map[string]string{"team": "about"}[entry]), CreatedAt: 1}); err != nil {
			t.Fatal(err)
		}
		if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{ID: entry, PublishedRevisionID: sql.NullString{String: entry + "-r1", Valid: true}, PublishedAt: sql.NullInt64{Int64: 1, Valid: true}, UpdatedAt: 1}); err != nil {
			t.Fatal(err)
		}
	}
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "team-r2", EntryID: "team", RevisionNumber: 2, Title: "team", DocumentJson: `{"version":1,"nodes":[]}`, ParentEntryID: nullRevisionParent("company"), CreatedAt: 2}); err != nil {
		t.Fatal(err)
	}
	latest, err := queries.ListLatestHierarchyForContentType(ctx, "page")
	if err != nil {
		t.Fatal(err)
	}
	published, err := queries.ListPublishedHierarchyForContentType(ctx, "page")
	if err != nil {
		t.Fatal(err)
	}
	if latestParentFor(latest, "team") != "company" || publishedParentFor(published, "team") != "about" {
		t.Fatalf("draft parent leaked into published hierarchy: latest=%q published=%q", latestParentFor(latest, "team"), publishedParentFor(published, "team"))
	}
}

func latestParentFor(rows []db.ListLatestHierarchyForContentTypeRow, entryID string) string {
	for _, row := range rows {
		if row.EntryID == entryID && row.ParentEntryID.Valid {
			return row.ParentEntryID.String
		}
	}
	return ""
}

func publishedParentFor(rows []db.ListPublishedHierarchyForContentTypeRow, entryID string) string {
	for _, row := range rows {
		if row.EntryID == entryID && row.ParentEntryID.Valid {
			return row.ParentEntryID.String
		}
	}
	return ""
}

func nullRevisionParent(parent string) sql.NullString {
	if parent == "" {
		return sql.NullString{}
	}
	return sql.NullString{String: parent, Valid: true}
}
