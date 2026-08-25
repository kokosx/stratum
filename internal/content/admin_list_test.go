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
	// Normalize legacy private status to active + private visibility revision.
	entryStatus := status
	visibility := "public"
	if status == "private" {
		entryStatus = "active"
		visibility = "private"
	}
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: id, ContentTypeID: contentType, Slug: slug, Status: entryStatus, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateEntry %s: %v", id, err)
	}
	revID := id + "-rev1"
	doc := `{"version":1,"nodes":[]}`
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: revID, EntryID: id, RevisionNumber: 1, Title: title, DocumentJson: doc, Visibility: visibility, CreatedAt: now}); err != nil {
		t.Fatalf("CreateRevision %s: %v", id, err)
	}
	if status == "private" {
		// Private entries are published with private visibility.
		if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: revID, Valid: true}, PublishedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now, ID: id}); err != nil {
			t.Fatalf("SetPublished %s: %v", id, err)
		}
		return
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

func TestAdminListWorkflowStatuses(t *testing.T) {
	repo, _, queries := newAdminListHarness(t)
	ctx := context.Background()
	now := time.Now().Unix()

	// Helper to create entry with latest revision and optionally published revision
	create := func(id, slug, latestState, publishedVis string, withSchedule bool, status string) {
		if status == "" {
			status = "active"
		}
		if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: id, ContentTypeID: "page", Slug: slug, Status: status, CreatedAt: now, UpdatedAt: now}); err != nil {
			t.Fatalf("CreateEntry %s: %v", id, err)
		}
		latestID := id + "-latest"
		latestVis := "public"
		if publishedVis == "private" || publishedVis == "password" {
			latestVis = publishedVis
		}
		if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: latestID, EntryID: id, RevisionNumber: 1, Slug: slug, Title: "Title " + id, DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now, Visibility: latestVis, ReviewState: latestState}); err != nil {
			t.Fatalf("latest %s: %v", id, err)
		}
		if publishedVis != "" {
			// Need published revision (maybe same as latest if latest is public and we want published)
			pubID := id + "-pub"
			if latestState == "draft" && publishedVis == "public" && !withSchedule {
				// For published with no unpublished changes, make published same as latest
				if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: latestID, Valid: true}, PublishedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now, ID: id}); err != nil {
					t.Fatalf("set pub %s: %v", id, err)
				}
				if publishedVis == "private" || publishedVis == "password" {
					// Need to set visibility correctly – already set via latest
				}
			} else {
				// Create separate published revision
				pubVis := publishedVis
				if pubVis == "" {
					pubVis = "public"
				}
				if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: pubID, EntryID: id, RevisionNumber: 2, Slug: slug, Title: "Pub " + id, DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now - 10, Visibility: pubVis, ReviewState: "draft"}); err != nil {
					// If duplicate, ignore
				}
				if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: pubID, Valid: true}, PublishedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now, ID: id}); err != nil {
					t.Fatalf("set pub2 %s: %v", id, err)
				}
				// Latest already created as second revision? We already have latest as r1, need to make it r2 for unpublished cases
				// For simplicity, for publishedWithDraft we already have latest as draft and pub as separate
			}
		}
		if withSchedule {
			// Create scheduled job for latest
			jobID := id + "-job"
			latest, _ := queries.GetLatestEntryRevision(ctx, id)
			if err := queries.CreatePublicationJob(ctx, db.CreatePublicationJobParams{ID: jobID, EntryID: id, RevisionID: latest.ID, ScheduledAt: now + 10000, CreatedAt: now, UpdatedAt: now}); err != nil {
				t.Fatalf("schedule %s: %v", id, err)
			}
		}
		if status == "trash" {
			queries.MoveEntryToTrash(ctx, db.MoveEntryToTrashParams{ID: id, TrashedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now})
		}
	}

	// Draft: no published, latest draft, no schedule
	create("draft1", "draft1", "draft", "", false, "active")
	// Pending: latest pending, not published
	create("pending1", "pending1", "pending", "", false, "active")
	// Scheduled: active schedule
	create("sched1", "sched1", "draft", "", true, "active")
	// Published: public
	create("pub1", "pub1", "draft", "public", false, "active")
	// Published with unpublished draft
	func() {
		id := "pubWithDraft"
		slug := "pubwithdraft"
		queries.CreateEntry(ctx, db.CreateEntryParams{ID: id, ContentTypeID: "page", Slug: slug, Status: "active", CreatedAt: now, UpdatedAt: now})
		// Published rev
		pubID := id + "-pub"
		queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: pubID, EntryID: id, RevisionNumber: 1, Slug: slug, Title: "Pub", DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now - 10, Visibility: "public", ReviewState: "draft"})
		queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: pubID, Valid: true}, PublishedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now, ID: id})
		// Latest draft different
		latestID := id + "-latest"
		queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: latestID, EntryID: id, RevisionNumber: 2, Slug: slug, Title: "Latest Draft", DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now, Visibility: "public", ReviewState: "draft"})
	}()
	// Private: published private
	create("private1", "private1", "draft", "private", false, "active")
	// Password: published password – should be counted as published, not private
	func() {
		id := "pwd1"
		slug := "pwd1"
		queries.CreateEntry(ctx, db.CreateEntryParams{ID: id, ContentTypeID: "page", Slug: slug, Status: "active", CreatedAt: now, UpdatedAt: now})
		revID := id + "-r1"
		queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: revID, EntryID: id, RevisionNumber: 1, Slug: slug, Title: "Pwd", DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now, Visibility: "password", PasswordHash: sql.NullString{String: "$2a$10$dummyhashdummyhashdummyhashdummyha", Valid: true}, ReviewState: "draft"})
		queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: revID, Valid: true}, PublishedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now, ID: id})
	}()
	// Trash
	create("trash1", "trash1", "draft", "", false, "trash")

	resAll, _ := repo.AdminList(ctx, EntryAdminListQuery{ContentType: ContentTypePage, Status: AdminStatusAll, Page: 1, PerPage: 50})
	if resAll.Counts.All != 7 {
		t.Fatalf("All %d want 7", resAll.Counts.All)
	}
	if resAll.Counts.Published != 3 {
		t.Fatalf("Published %d want 3 (pub1, pubWithDraft, pwd1)", resAll.Counts.Published)
	}
	if resAll.Counts.Draft != 1 {
		t.Fatalf("Draft %d want 1 (draft1)", resAll.Counts.Draft)
	}
	if resAll.Counts.Pending != 1 {
		t.Fatalf("Pending %d want 1", resAll.Counts.Pending)
	}
	if resAll.Counts.Scheduled != 1 {
		t.Fatalf("Scheduled %d want 1", resAll.Counts.Scheduled)
	}
	if resAll.Counts.Private != 1 {
		t.Fatalf("Private %d want 1", resAll.Counts.Private)
	}
	if resAll.Counts.Trash != 1 {
		t.Fatalf("Trash %d want 1", resAll.Counts.Trash)
	}

	// Verify each tab returns expected rows
	checkTab := func(status AdminStatus, wantIDs []string) {
		res, _ := repo.AdminList(ctx, EntryAdminListQuery{ContentType: ContentTypePage, Status: status, Page: 1, PerPage: 50})
		got := map[string]bool{}
		for _, e := range res.Entries {
			got[e.ID] = true
		}
		for _, want := range wantIDs {
			if !got[want] {
				t.Fatalf("status %s should contain %s, got %+v", status, want, got)
			}
		}
	}
	checkTab(AdminStatusPublished, []string{"pub1", "pubWithDraft", "pwd1"})
	checkTab(AdminStatusDraft, []string{"draft1"})
	checkTab(AdminStatusPending, []string{"pending1"})
	checkTab(AdminStatusScheduled, []string{"sched1"})
	checkTab(AdminStatusPrivate, []string{"private1"})
	checkTab(AdminStatusTrash, []string{"trash1"})
	// Password is published, not private
	resPriv, _ := repo.AdminList(ctx, EntryAdminListQuery{ContentType: ContentTypePage, Status: AdminStatusPrivate, Page: 1, PerPage: 50})
	for _, e := range resPriv.Entries {
		if e.ID == "pwd1" {
			t.Fatalf("password should not be in private tab")
		}
	}
}
