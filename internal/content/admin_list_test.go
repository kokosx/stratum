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

func newAdminListHarness(t *testing.T) (*Repository, *storage.Database, *db.Queries) {
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
	repo := NewRepository(queries)
	return repo, database, queries
}

func insertAdminEntry(t *testing.T, queries *db.Queries, id, contentType, slug, status string, publishedRevisionID sql.NullString, title string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().Unix()
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: id, ContentTypeID: contentType, Slug: slug, Status: status, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateEntry %s: %v", id, err)
	}
	revID := id + "-rev1"
	doc := `{"version":1,"nodes":[]}`
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: revID, EntryID: id, RevisionNumber: 1, Title: title, DocumentJson: doc, CreatedAt: now}); err != nil {
		t.Fatalf("CreateRevision %s: %v", id, err)
	}
	if publishedRevisionID.Valid {
		if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: publishedRevisionID, PublishedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now, ID: id}); err != nil {
			t.Fatalf("SetPublished %s: %v", id, err)
		}
	}
}

func TestAdminListStatusCounts(t *testing.T) {
	repo, _, queries := newAdminListHarness(t)
	ctx := context.Background()
	insertAdminEntry(t, queries, "p1", "page", "p1", "active", sql.NullString{String: "p1-rev1", Valid: true}, "Published Page")
	insertAdminEntry(t, queries, "p2", "page", "p2", "active", sql.NullString{Valid: false}, "Draft Page")
	insertAdminEntry(t, queries, "p3", "page", "p3", "private", sql.NullString{Valid: false}, "Private Page")
	insertAdminEntry(t, queries, "p4", "page", "p4", "trash", sql.NullString{Valid: false}, "Trashed Page")
	res, err := repo.AdminList(ctx, EntryAdminListQuery{ContentType: ContentTypePage, Status: AdminStatusAll, Page: 1, PerPage: 20})
	if err != nil {
		t.Fatal(err)
	}
	if res.Counts.All != 3 {
		t.Fatalf("All %d want 3", res.Counts.All)
	}
	if res.Counts.Published != 1 {
		t.Fatalf("Published %d", res.Counts.Published)
	}
	if res.Counts.Draft != 1 {
		t.Fatalf("Draft %d", res.Counts.Draft)
	}
	if res.Counts.Private != 1 {
		t.Fatalf("Private %d", res.Counts.Private)
	}
	if res.Counts.Trash != 1 {
		t.Fatalf("Trash %d", res.Counts.Trash)
	}
}

func TestAdminListAllExcludesTrash(t *testing.T) {
	repo, _, queries := newAdminListHarness(t)
	ctx := context.Background()
	insertAdminEntry(t, queries, "a1", "post", "a1", "active", sql.NullString{String: "a1-rev1", Valid: true}, "Post A")
	insertAdminEntry(t, queries, "a2", "post", "a2", "trash", sql.NullString{Valid: false}, "Post Trash")
	res, _ := repo.AdminList(ctx, EntryAdminListQuery{ContentType: ContentTypePost, Status: AdminStatusAll, Page: 1, PerPage: 20})
	for _, e := range res.Entries {
		if e.ID == "a2" {
			t.Fatalf("All should exclude trash")
		}
	}
	if res.Total != 1 {
		t.Fatalf("total %d want 1", res.Total)
	}
}

func TestAdminListSearchByTitle(t *testing.T) {
	repo, _, queries := newAdminListHarness(t)
	ctx := context.Background()
	insertAdminEntry(t, queries, "s1", "page", "slug-one", "active", sql.NullString{Valid: false}, "UniqueTitleXYZ")
	insertAdminEntry(t, queries, "s2", "page", "slug-two", "active", sql.NullString{Valid: false}, "Other")
	res, _ := repo.AdminList(ctx, EntryAdminListQuery{ContentType: ContentTypePage, Search: "UniqueTitleXYZ", Status: AdminStatusAll, Page: 1, PerPage: 20})
	if len(res.Entries) != 1 || res.Entries[0].ID != "s1" {
		t.Fatalf("search by title failed %+v", res.Entries)
	}
}

func TestAdminListSearchBySlug(t *testing.T) {
	repo, _, queries := newAdminListHarness(t)
	ctx := context.Background()
	insertAdminEntry(t, queries, "sg1", "page", "find-me-slug", "active", sql.NullString{Valid: false}, "Title")
	res, _ := repo.AdminList(ctx, EntryAdminListQuery{ContentType: ContentTypePage, Search: "find-me-slug", Status: AdminStatusAll, Page: 1, PerPage: 20})
	if len(res.Entries) != 1 || res.Entries[0].ID != "sg1" {
		t.Fatalf("search by slug failed")
	}
}

func TestAdminListPaginationDeterministic(t *testing.T) {
	repo, _, queries := newAdminListHarness(t)
	ctx := context.Background()
	insertAdminEntry(t, queries, "pg1", "page", "pg1", "active", sql.NullString{Valid: false}, "Pg1")
	insertAdminEntry(t, queries, "pg2", "page", "pg2", "active", sql.NullString{Valid: false}, "Pg2")
	insertAdminEntry(t, queries, "pg3", "page", "pg3", "active", sql.NullString{Valid: false}, "Pg3")
	res1, _ := repo.AdminList(ctx, EntryAdminListQuery{ContentType: ContentTypePage, Status: AdminStatusAll, Page: 1, PerPage: 2})
	res2, _ := repo.AdminList(ctx, EntryAdminListQuery{ContentType: ContentTypePage, Status: AdminStatusAll, Page: 2, PerPage: 2})
	seen := map[string]bool{}
	for _, e := range res1.Entries {
		seen[e.ID] = true
	}
	for _, e := range res2.Entries {
		if seen[e.ID] {
			t.Fatalf("overlap pagination")
		}
	}
	if res1.Total != 3 {
		t.Fatalf("total %d", res1.Total)
	}
}

func TestAdminListClampsOutOfRangeBeforeQuery(t *testing.T) {
	repo, _, queries := newAdminListHarness(t)
	ctx := context.Background()
	insertAdminEntry(t, queries, "last1", "page", "last1", "active", sql.NullString{}, "Last")
	res, err := repo.AdminList(ctx, EntryAdminListQuery{ContentType: ContentTypePage, Status: AdminStatusAll, Page: 999, PerPage: 20})
	if err != nil {
		t.Fatal(err)
	}
	if res.Page != 1 || len(res.Entries) != 1 || res.Entries[0].ID != "last1" {
		t.Fatalf("out-of-range page returned %+v", res)
	}
}

func TestAdminListUnknownStatusNormalized(t *testing.T) {
	repo, _, queries := newAdminListHarness(t)
	ctx := context.Background()
	insertAdminEntry(t, queries, "u1", "page", "u1", "active", sql.NullString{Valid: false}, "U")
	res, _ := repo.AdminList(ctx, EntryAdminListQuery{ContentType: ContentTypePage, Status: AdminStatus("unknown"), Page: 1, PerPage: 20})
	if res == nil || res.Total != 1 {
		t.Fatalf("unknown status should be normalized to all")
	}
}
