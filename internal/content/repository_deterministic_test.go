package content

import (
	"context"
	"database/sql"
	"path/filepath"
	"sort"
	"testing"

	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func newTestRepository(t *testing.T) (*Repository, *storage.Database, *db.Queries) {
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

func insertPublishedPost(t *testing.T, queries *db.Queries, database *storage.Database, id, slug, title string, now int64, firstPublished, published sql.NullInt64) {
	t.Helper()
	ctx := context.Background()
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{
		ID: id, ContentTypeID: "post", Slug: slug, Status: "active",
		CreatedAt: now, UpdatedAt: now, PublishedAt: published,
	}); err != nil {
		t.Fatalf("CreateEntry %s: %v", id, err)
	}
	revID := id + "-r1"
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{
		ID: revID, EntryID: id, RevisionNumber: 1, Title: title, DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now,
	}); err != nil {
		t.Fatalf("CreateRevision %s: %v", id, err)
	}
	if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{
		PublishedRevisionID: sql.NullString{String: revID, Valid: true}, PublishedAt: published, UpdatedAt: now, ID: id,
	}); err != nil {
		t.Fatalf("SetPublished %s: %v", id, err)
	}
	// Directly update first_published_at to requested value
	if firstPublished.Valid {
		if _, err := database.DB.ExecContext(ctx, `UPDATE entries SET first_published_at = ?, published_at = ? WHERE id = ?`, firstPublished.Int64, published.Int64, id); err != nil {
			t.Fatalf("update timestamps %s: %v", id, err)
		}
	}
	routeID := id + "-route"
	if err := queries.CreateRoute(ctx, db.CreateRouteParams{
		ID: routeID, Path: "/blog/" + slug, EntryID: sql.NullString{String: id, Valid: true}, RouteType: "entry", CreatedAt: now, UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateRoute %s: %v", id, err)
	}
	if err := queries.SetFirstPublishedAtIfNull(ctx, db.SetFirstPublishedAtIfNullParams{FirstPublishedAt: firstPublished, ID: id}); err != nil {
		// ignore if already set
	}
	if _, err := database.DB.ExecContext(ctx, `UPDATE entries SET first_published_at = ?, published_at = ? WHERE id = ?`, firstPublished.Int64, published.Int64, id); err != nil {
		t.Fatalf("force timestamps %s: %v", id, err)
	}
}

func TestRepositoryDeterministicOrderWithIdenticalTimestamps(t *testing.T) {
	repo, database, queries := newTestRepository(t)
	ctx := context.Background()
	now := int64(1700000000)
	identical := sql.NullInt64{Int64: now, Valid: true}

	// Clean any seeded posts that might interfere? Seed not called, so empty. Insert 3 with same timestamps.
	ids := []string{"post-a", "post-b", "post-c"}
	for _, id := range ids {
		slug := id
		insertPublishedPost(t, queries, database, id, slug, "Title "+id, now, identical, identical)
	}

	// Query DESC many times – must be stable
	var firstOrder []string
	for i := 0; i < 5; i++ {
		rows, err := repo.QueryPublished(ctx, EntryQuery{ContentType: "post", Limit: 10, Offset: 0, Order: "published_desc"})
		if err != nil {
			t.Fatalf("QueryPublished desc: %v", err)
		}
		order := make([]string, len(rows))
		for j, r := range rows {
			order[j] = r.ID
		}
		if i == 0 {
			firstOrder = order
		} else if !equalStrings(order, firstOrder) {
			t.Fatalf("unstable order attempt %d: %v vs first %v", i, order, firstOrder)
		}
	}
	// DESC should be id descending due to tie-breaker (post-c, post-b, post-a)
	expectedDesc := []string{"post-c", "post-b", "post-a"}
	if !equalStrings(firstOrder, expectedDesc) {
		t.Fatalf("DESC order = %v, want %v", firstOrder, expectedDesc)
	}

	// ASC should be opposite
	rowsAsc, err := repo.QueryPublished(ctx, EntryQuery{ContentType: "post", Limit: 10, Offset: 0, Order: "published_asc"})
	if err != nil {
		t.Fatalf("asc: %v", err)
	}
	ascOrder := make([]string, len(rowsAsc))
	for i, r := range rowsAsc {
		ascOrder[i] = r.ID
	}
	expectedAsc := []string{"post-a", "post-b", "post-c"}
	if !equalStrings(ascOrder, expectedAsc) {
		t.Fatalf("ASC order = %v, want %v", ascOrder, expectedAsc)
	}

	// Pagination stability: page1 limit2 offset0 and page2 offset2 should have no duplicates and cover all
	page1, _ := repo.QueryPublished(ctx, EntryQuery{ContentType: "post", Limit: 2, Offset: 0, Order: "published_desc"})
	page2, _ := repo.QueryPublished(ctx, EntryQuery{ContentType: "post", Limit: 2, Offset: 2, Order: "published_desc"})
	seen := map[string]bool{}
	for _, r := range page1 {
		if seen[r.ID] {
			t.Fatalf("duplicate in page1: %s", r.ID)
		}
		seen[r.ID] = true
	}
	for _, r := range page2 {
		if seen[r.ID] {
			t.Fatalf("duplicate across pages: %s", r.ID)
		}
		seen[r.ID] = true
	}
	if len(seen) != 3 {
		t.Fatalf("paginated seen %d, want 3, seen %v", len(seen), seen)
	}
	// Also verify that combined sorted equals desc order
	combined := append([]string{}, page1[0].ID, page1[1].ID)
	if len(page2) > 0 {
		combined = append(combined, page2[0].ID)
	}
	sort.Strings(combined)
	expectedSorted := []string{"post-a", "post-b", "post-c"}
	sort.Strings(expectedDesc)
	if !equalStrings(combined, expectedSorted) {
		// Actually combined should be exactly expectedDesc partitioned
		if page1[0].ID != "post-c" || page1[1].ID != "post-b" || page2[0].ID != "post-a" {
			t.Fatalf("pagination order wrong: page1 %v page2 %v", page1, page2)
		}
	}
}

func TestRepositoryOrderWithDistinctTimestamps(t *testing.T) {
	repo, database, queries := newTestRepository(t)
	ctx := context.Background()
	now := int64(1700000000)
	// A oldest, B middle, C newest
	insertPublishedPost(t, queries, database, "post-old", "old", "Old", now, sql.NullInt64{Int64: now, Valid: true}, sql.NullInt64{Int64: now, Valid: true})
	insertPublishedPost(t, queries, database, "post-mid", "mid", "Mid", now, sql.NullInt64{Int64: now + 100, Valid: true}, sql.NullInt64{Int64: now + 100, Valid: true})
	insertPublishedPost(t, queries, database, "post-new", "new", "New", now, sql.NullInt64{Int64: now + 200, Valid: true}, sql.NullInt64{Int64: now + 200, Valid: true})

	rowsDesc, err := repo.QueryPublished(ctx, EntryQuery{ContentType: "post", Limit: 10, Offset: 0, Order: "published_desc"})
	if err != nil {
		t.Fatalf("desc: %v", err)
	}
	if len(rowsDesc) != 3 {
		t.Fatalf("rows %d want 3", len(rowsDesc))
	}
	if rowsDesc[0].ID != "post-new" || rowsDesc[1].ID != "post-mid" || rowsDesc[2].ID != "post-old" {
		t.Fatalf("desc order %v", rowsDesc)
	}
	rowsAsc, _ := repo.QueryPublished(ctx, EntryQuery{ContentType: "post", Limit: 10, Offset: 0, Order: "published_asc"})
	if rowsAsc[0].ID != "post-old" || rowsAsc[1].ID != "post-mid" || rowsAsc[2].ID != "post-new" {
		t.Fatalf("asc order %v", rowsAsc)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
