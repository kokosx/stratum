package content

import (
	"context"
	"database/sql"
	"testing"

	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func insertPublishedProduct(t *testing.T, queries *db.Queries, id, title, fields string, published int64) {
	t.Helper()
	ctx := context.Background()
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: id, ContentTypeID: "product", Slug: id, Status: "active", CreatedAt: published, UpdatedAt: published}); err != nil {
		t.Fatal(err)
	}
	rev := id + "-r1"
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: rev, EntryID: id, RevisionNumber: 1, Slug: id, Title: title, DocumentJson: `{"version":1,"nodes":[]}`, FieldsJson: fields, CreatedAt: published}); err != nil {
		t.Fatal(err)
	}
	if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{ID: id, PublishedRevisionID: sql.NullString{String: rev, Valid: true}, PublishedAt: sql.NullInt64{Int64: published, Valid: true}, UpdatedAt: published}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateRoute(ctx, db.CreateRouteParams{ID: id + "-route", Path: "/products/" + id, EntryID: sql.NullString{String: id, Valid: true}, RouteType: "entry", CreatedAt: published, UpdatedAt: published}); err != nil {
		t.Fatal(err)
	}
}

func TestEntryQueryFiltersSortsPublishedRevisionFields(t *testing.T) {
	repo, _, queries := newTestRepository(t)
	ctx := context.Background()
	if err := queries.CreateContentType(ctx, db.CreateContentTypeParams{ID: "product", DisplayName: "Product", PluralName: "Products", Public: 1, ConfigJson: `{}`, CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	insertPublishedProduct(t, queries, "a", "Product A", `{"price":999,"featured":true,"sku":"ABC"}`, 1)
	insertPublishedProduct(t, queries, "b", "Product B", `{"price":499,"featured":false,"sku":"DEF"}`, 2)
	insertPublishedProduct(t, queries, "c", "Product C", `{"price":799,"featured":true,"sku":"GHI"}`, 3)
	// A newer draft must never affect the public query.
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "a-r2", EntryID: "a", RevisionNumber: 2, Slug: "a", Title: "Draft A", DocumentJson: `{"version":1,"nodes":[]}`, FieldsJson: `{"price":1,"featured":false}`, CreatedAt: 4}); err != nil {
		t.Fatal(err)
	}
	query := EntryQuery{ContentType: "product", Limit: 6, Filters: []EntryFilter{{Field: "fields.featured", Operator: OpIsTrue}}, OrderBy: "fields.price", Direction: "asc"}
	rows, err := repo.QueryPublished(ctx, query)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 2 || rows[0].ID != "c" || rows[1].ID != "a" {
		t.Fatalf("rows=%#v", rows)
	}
	price, _, err := publishedValue(rows[1], "fields.price")
	if err != nil || price != float64(999) {
		t.Fatalf("draft leaked, price=%#v err=%v", price, err)
	}
}

func TestEntryQueryOperatorsAndStableCacheKey(t *testing.T) {
	q1 := EntryQuery{ContentType: "product", Limit: 6, Filters: []EntryFilter{{Field: "fields.sku", Operator: OpContains, Value: "AB"}, {Field: "fields.price", Operator: OpGreater, Value: "100"}}, OrderBy: "entry.title", Direction: "asc", ExcludeIDs: []string{"b", "a"}}
	q2 := q1
	q2.Filters = []EntryFilter{q1.Filters[1], q1.Filters[0]}
	q2.ExcludeIDs = []string{"a", "b"}
	if q1.CacheKey() != q2.CacheKey() {
		t.Fatalf("equivalent queries have different cache keys: %s %s", q1.CacheKey(), q2.CacheKey())
	}
	if err := q1.Validate(); err != nil {
		t.Fatal(err)
	}
	bad := q1
	bad.Filters = append(bad.Filters, bad.Filters[0], bad.Filters[0], bad.Filters[0], bad.Filters[0])
	if err := bad.Validate(); err == nil {
		t.Fatal("too many filters accepted")
	}
}
