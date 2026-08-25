package content

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func newTrashHarness(t *testing.T) (*LifecycleService, *storage.Database, *db.Queries, context.Context) {
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
	_ = database.Seed(ctx)
	svc := NewLifecycleService(database.DB, queries)
	return svc, database, queries, ctx
}

func createPublishedEntry(t *testing.T, queries *db.Queries, id, ct, slug string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().Unix()
	_ = queries.CreateEntry(ctx, db.CreateEntryParams{ID: id, ContentTypeID: ct, Slug: slug, Status: "active", CreatedAt: now, UpdatedAt: now})
	revID := id + "-r1"
	_ = queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: revID, EntryID: id, RevisionNumber: 1, Slug: slug, Title: "Title " + id, DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now})
	_ = queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: revID, Valid: true}, PublishedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now, ID: id})
	id2, _ := randomID()
	_ = queries.CreateRoute(ctx, db.CreateRouteParams{ID: id2, Path: "/" + slug, EntryID: sql.NullString{String: id, Valid: true}, RouteType: "entry", CreatedAt: now, UpdatedAt: now})
}

func createDraftEntry(t *testing.T, queries *db.Queries, id, ct, slug string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().Unix()
	_ = queries.CreateEntry(ctx, db.CreateEntryParams{ID: id, ContentTypeID: ct, Slug: slug, Status: "active", CreatedAt: now, UpdatedAt: now})
	revID := id + "-r1"
	_ = queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: revID, EntryID: id, RevisionNumber: 1, Title: "Draft " + id, DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now})
}

func createPrivateEntry(t *testing.T, queries *db.Queries, id, ct, slug string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().Unix()
	_ = queries.CreateEntry(ctx, db.CreateEntryParams{ID: id, ContentTypeID: ct, Slug: slug, Status: "active", CreatedAt: now, UpdatedAt: now})
	revID := id + "-r1"
	_ = queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: revID, EntryID: id, RevisionNumber: 1, Slug: slug, Title: "Private " + id, DocumentJson: `{"version":1,"nodes":[]}`, Visibility: "private", CreatedAt: now})
	_ = queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: revID, Valid: true}, PublishedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now, ID: id})
}

func TestDraftTrashRestore(t *testing.T) {
	svc, _, queries, ctx := newTrashHarness(t)
	createDraftEntry(t, queries, "draft1", "page", "draft-one")
	if err := svc.MoveToTrash(ctx, "draft1"); err != nil {
		t.Fatalf("trash draft: %v", err)
	}
	e, _ := queries.GetEntry(ctx, "draft1")
	if e.Status != "trash" {
		t.Fatalf("status %s", e.Status)
	}
	if e.StatusBeforeTrash.String != "active" {
		t.Fatalf("before %v", e.StatusBeforeTrash)
	}
	if err := svc.Restore(ctx, "draft1"); err != nil {
		t.Fatalf("restore: %v", err)
	}
	e2, _ := queries.GetEntry(ctx, "draft1")
	if e2.Status != "active" {
		t.Fatalf("restored status %s", e2.Status)
	}
}

func TestPublishedActiveTrashRestore(t *testing.T) {
	svc, _, queries, ctx := newTrashHarness(t)
	createPublishedEntry(t, queries, "pub1", "page", "pub-one")
	if err := svc.MoveToTrash(ctx, "pub1"); err != nil {
		t.Fatalf("trash pub: %v", err)
	}
	if _, err := queries.GetRouteByPath(ctx, "/pub-one"); err == nil {
		t.Fatalf("route should be deleted")
	}
	if err := svc.Restore(ctx, "pub1"); err != nil {
		t.Fatalf("restore: %v", err)
	}
	e, _ := queries.GetEntry(ctx, "pub1")
	if e.Status != "active" {
		t.Fatalf("status %s", e.Status)
	}
	if _, err := queries.GetRouteByPath(ctx, "/pub-one"); err != nil {
		t.Fatalf("route should be recreated %v", err)
	}
}

func TestPrivateTrashRestore(t *testing.T) {
	svc, _, queries, ctx := newTrashHarness(t)
	createPrivateEntry(t, queries, "priv1", "page", "priv-one")
	if err := svc.MoveToTrash(ctx, "priv1"); err != nil {
		t.Fatalf("trash private: %v", err)
	}
	if err := svc.Restore(ctx, "priv1"); err != nil {
		t.Fatalf("restore private: %v", err)
	}
	e, _ := queries.GetEntry(ctx, "priv1")
	if e.Status != "active" {
		t.Fatalf("expected active after private restore, got %s", e.Status)
	}
	if !e.PublishedRevisionID.Valid {
		t.Fatalf("private entry should still have published revision")
	}
	rev, _ := queries.GetEntryRevision(ctx, e.PublishedRevisionID.String)
	if rev.Visibility != "private" {
		t.Fatalf("expected private visibility, got %s", rev.Visibility)
	}
	if _, err := queries.GetRouteByPath(ctx, "/priv-one"); err == nil {
		t.Fatal("private restore must not recreate a public route")
	}
}

func TestTrashRemovesPublicAvailability(t *testing.T) {
	svc, _, queries, ctx := newTrashHarness(t)
	createPublishedEntry(t, queries, "pub2", "page", "pub-two")
	if err := svc.MoveToTrash(ctx, "pub2"); err != nil {
		t.Fatal(err)
	}
	if _, err := queries.GetRouteByPath(ctx, "/pub-two"); err == nil {
		t.Fatal("public route should disappear")
	}
}

func TestRestoreRecreatesPublicAvailability(t *testing.T) {
	svc, _, queries, ctx := newTrashHarness(t)
	createPublishedEntry(t, queries, "pub3", "page", "pub-three")
	_ = svc.MoveToTrash(ctx, "pub3")
	if err := svc.Restore(ctx, "pub3"); err != nil {
		t.Fatal(err)
	}
	if _, err := queries.GetRouteByPath(ctx, "/pub-three"); err != nil {
		t.Fatalf("route missing %v", err)
	}
}

func TestRestoreUsesPublishedRevisionSlug(t *testing.T) {
	svc, _, queries, ctx := newTrashHarness(t)
	createPublishedEntry(t, queries, "slug1", "page", "restore-about")
	now := time.Now().Unix()
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "slug1-r2", EntryID: "slug1", RevisionNumber: 2, Slug: "restore-company", Title: "Company", DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := queries.UpdateEntryProjection(ctx, db.UpdateEntryProjectionParams{ID: "slug1", Slug: "restore-company", Status: "active", UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := svc.MoveToTrash(ctx, "slug1"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Restore(ctx, "slug1"); err != nil {
		t.Fatal(err)
	}
	if _, err := queries.GetRouteByPath(ctx, "/restore-about"); err != nil {
		t.Fatalf("published path was not restored: %v", err)
	}
	if _, err := queries.GetRouteByPath(ctx, "/restore-company"); err == nil {
		t.Fatal("draft slug became public during restore")
	}
}

func TestRestoreUsesPublishedHierarchyInsteadOfDraftParent(t *testing.T) {
	svc, _, queries, ctx := newTrashHarness(t)
	createPublishedEntry(t, queries, "company", "page", "restore-company-root")
	createPublishedEntry(t, queries, "services", "page", "restore-services-root")
	now := time.Now().Unix()
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: "team", ContentTypeID: "page", Slug: "team", Status: "active", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "team-r1", EntryID: "team", RevisionNumber: 1, Slug: "team", Title: "Team", ParentEntryID: sql.NullString{String: "company", Valid: true}, DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{ID: "team", PublishedRevisionID: sql.NullString{String: "team-r1", Valid: true}, PublishedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateRoute(ctx, db.CreateRouteParams{ID: "team-route", Path: "/restore-company-root/team", EntryID: sql.NullString{String: "team", Valid: true}, RouteType: "entry", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "team-r2", EntryID: "team", RevisionNumber: 2, Slug: "staff", Title: "Staff", ParentEntryID: sql.NullString{String: "services", Valid: true}, DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := svc.MoveToTrash(ctx, "team"); err != nil {
		t.Fatal(err)
	}
	if err := svc.Restore(ctx, "team"); err != nil {
		t.Fatal(err)
	}
	if _, err := queries.GetRouteByPath(ctx, "/restore-company-root/team"); err != nil {
		t.Fatalf("published hierarchy path was not restored: %v", err)
	}
	if _, err := queries.GetRouteByPath(ctx, "/restore-services-root/staff"); err == nil {
		t.Fatal("draft hierarchy became public during restore")
	}
}

func TestHomepageCannotBeTrashed(t *testing.T) {
	svc, _, queries, ctx := newTrashHarness(t)
	createPublishedEntry(t, queries, "home1", "page", "home-one")
	_ = queries.UpdateReadingSettings(ctx, db.UpdateReadingSettingsParams{HomepageMode: "page", HomepageEntryID: sql.NullString{String: "home1", Valid: true}, PostsPageEntryID: sql.NullString{}, PostsPerPage: 10, PostsBasePath: "/blog", UpdatedAt: time.Now().Unix()})
	if err := svc.MoveToTrash(ctx, "home1"); err != ErrProtectedPage {
		t.Fatalf("expected protected, got %v", err)
	}
	e, _ := queries.GetEntry(ctx, "home1")
	if e.Status == "trash" {
		t.Fatal("should not be trashed")
	}
}

func TestPostsPageCannotBeTrashed(t *testing.T) {
	svc, _, queries, ctx := newTrashHarness(t)
	createPublishedEntry(t, queries, "posts1", "page", "posts-one")
	_ = queries.UpdateReadingSettings(ctx, db.UpdateReadingSettingsParams{HomepageMode: "latest_posts", PostsPageEntryID: sql.NullString{String: "posts1", Valid: true}, PostsPerPage: 10, PostsBasePath: "/blog", UpdatedAt: time.Now().Unix()})
	if err := svc.MoveToTrash(ctx, "posts1"); err != ErrProtectedPage {
		t.Fatalf("expected protected, got %v", err)
	}
}

func TestPermanentDeleteOnlyFromTrash(t *testing.T) {
	svc, _, queries, ctx := newTrashHarness(t)
	createPublishedEntry(t, queries, "del1", "page", "del-one")
	if err := svc.DeletePermanently(ctx, "del1"); err != ErrPermanentDeleteOnlyTrash {
		t.Fatalf("expected only from trash, got %v", err)
	}
	_ = svc.MoveToTrash(ctx, "del1")
	if err := svc.DeletePermanently(ctx, "del1"); err != nil {
		t.Fatalf("delete from trash %v", err)
	}
	if _, err := queries.GetEntry(ctx, "del1"); err == nil {
		t.Fatal("entry should be deleted")
	}
}

func TestBulkValidationNoPartial(t *testing.T) {
	svc, _, queries, ctx := newTrashHarness(t)
	createPublishedEntry(t, queries, "b1", "page", "b-one")
	createPublishedEntry(t, queries, "b2", "page", "b-two")
	_ = queries.UpdateReadingSettings(ctx, db.UpdateReadingSettingsParams{HomepageMode: "page", HomepageEntryID: sql.NullString{String: "b2", Valid: true}, PostsPageEntryID: sql.NullString{}, PostsPerPage: 10, PostsBasePath: "/blog", UpdatedAt: time.Now().Unix()})
	err := svc.BulkTrash(ctx, "page", []string{"b1", "b2"})
	if err != ErrProtectedPage {
		t.Fatalf("expected protected, got %v", err)
	}
	e1, _ := queries.GetEntry(ctx, "b1")
	if e1.Status == "trash" {
		t.Fatal("b1 should not be trashed due to validation failure")
	}
}
