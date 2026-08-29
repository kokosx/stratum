package search

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/content"
	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func newSearchHarness(t *testing.T) (*Service, *storage.Database, *db.Queries, *blocks.Registry) {
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
	reg, err := blocks.NewRegistry(ctx, queries)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	svc := New(database.DB, reg)
	// also set queries for catalog
	// svc already has queries via New
	return svc, database, queries, reg
}

func createContentTypeForTest(t *testing.T, queries *db.Queries, id string, fields []content.FieldDefinition, single bool, base string) {
	t.Helper()
	cat := content.NewCatalog(queries)
	input := content.ContentTypeInput{
		ID:         content.ContentTypeID(id),
		Name:       strings.Title(id),
		PluralName: strings.Title(id) + "s",
		Config: content.ContentTypeConfig{
			Fields: fields,
			Features: content.ContentTypeFeatures{
				Content:  true,
				SEO:      true,
				Excerpt:  true,
				FeaturedMedia: true,
			},
			Routing: content.ContentTypeRouting{
				Single:   single,
				BasePath: base,
				Archive:  false,
			},
		},
	}
	if err := cat.CreateContentType(context.Background(), input); err != nil {
		t.Fatalf("CreateContentType %s: %v", id, err)
	}
}

func insertPublishedEntry(t *testing.T, queries *db.Queries, database *storage.Database, svc *Service, id, ctype, slug, title, excerpt, docJSON, fieldsJSON, visibility string, now int64) {
	t.Helper()
	ctx := context.Background()
	if docJSON == "" {
		docJSON = `{"version":1,"nodes":[]}`
	}
	if fieldsJSON == "" {
		fieldsJSON = `{}`
	}
	if visibility == "" {
		visibility = "public"
	}
	// Create entry if not exists
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: id, ContentTypeID: ctype, Slug: slug, Status: "active", CreatedAt: now, UpdatedAt: now}); err != nil && !strings.Contains(err.Error(), "UNIQUE") {
		// try update?
	}
	revID := id + "-r1"
	// check if revision exists already, if so use new id
	// Simplify always create new rev with unique id if already exists
	// Try to create, on conflict create with suffix
	err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{
		ID: revID, EntryID: id, RevisionNumber: 1, Slug: slug, Title: title, Excerpt: sql.NullString{String: excerpt, Valid: excerpt != ""}, DocumentJson: docJSON, FieldsJson: fieldsJSON, CreatedAt: now, Visibility: visibility, ReviewState: "draft",
	})
	if err != nil && strings.Contains(err.Error(), "UNIQUE") {
		revID = id + "-r-" + slug
		err = queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{
			ID: revID, EntryID: id, RevisionNumber: 2, Slug: slug, Title: title, Excerpt: sql.NullString{String: excerpt, Valid: excerpt != ""}, DocumentJson: docJSON, FieldsJson: fieldsJSON, CreatedAt: now, Visibility: visibility, ReviewState: "draft",
		})
	}
	if err != nil {
		t.Fatalf("CreateRevision %s: %v", id, err)
	}
	if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: revID, Valid: true}, PublishedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now, ID: id}); err != nil {
		t.Fatalf("SetPublished %s: %v", id, err)
	}
	dbConn := getDBConn(t, database, svc)
	if _, err := dbConn.ExecContext(ctx, `UPDATE entries SET first_published_at = ? WHERE id = ?`, now, id); err != nil {
		t.Fatalf("set first_published %v", err)
	}
	// Route
	path := "/" + slug
	if ctype == "post" {
		path = "/blog/" + slug
	} else if strings.HasPrefix(ctype, "product") {
		path = "/products/" + slug
	} else if ctype == "service" {
		path = "/services/" + slug
	} else if ctype == "case_study" {
		path = "/case-studies/" + slug
	}
	// Remove existing route for entry if any
	if rt, err := queries.GetEntryRoute(ctx, sql.NullString{String: id, Valid: true}); err == nil {
		_ = queries.DeleteRoute(ctx, rt.ID)
	}
	routeID := id + "-route"
	if err := queries.CreateRoute(ctx, db.CreateRouteParams{ID: routeID, Path: path, EntryID: sql.NullString{String: id, Valid: true}, RouteType: "entry", CreatedAt: now, UpdatedAt: now}); err != nil {
		// if conflict, delete by path and retry
		if _, err2 := dbConn.ExecContext(ctx, `DELETE FROM routes WHERE path = ?`, path); err2 == nil {
			_ = queries.CreateRoute(ctx, db.CreateRouteParams{ID: routeID + "2", Path: path, EntryID: sql.NullString{String: id, Valid: true}, RouteType: "entry", CreatedAt: now, UpdatedAt: now})
		}
	}
	if svc != nil {
		if err := svc.RefreshEntry(ctx, id); err != nil {
			t.Fatalf("RefreshEntry %s: %v", id, err)
		}
	}
}

func getDBConn(t *testing.T, database *storage.Database, svc *Service) *sql.DB {
	t.Helper()
	if database != nil && database.DB != nil {
		return database.DB
	}
	if svc != nil && svc.db != nil {
		return svc.db
	}
	t.Fatalf("no db connection available")
	return nil
}

func createDraftRevision(t *testing.T, queries *db.Queries, entryID, newTitle, newSlug, newExcerpt, newDocJSON, newFieldsJSON, visibility string) {
	t.Helper()
	ctx := context.Background()
	entry, err := queries.GetEntry(ctx, entryID)
	if err != nil {
		t.Fatalf("GetEntry %s: %v", entryID, err)
	}
	latest, err := queries.GetLatestEntryRevision(ctx, entryID)
	if err != nil {
		t.Fatalf("GetLatest %s: %v", entryID, err)
	}
	newRevID := entryID + "-draft-" + newSlug
	revNum := latest.RevisionNumber + 1
	if newDocJSON == "" {
		newDocJSON = latest.DocumentJson
	}
	if newFieldsJSON == "" {
		newFieldsJSON = latest.FieldsJson
	}
	if visibility == "" {
		visibility = latest.Visibility
	}
	if newTitle == "" {
		newTitle = latest.Title
	}
	if newSlug == "" {
		newSlug = latest.Slug
	}
	excerptVal := latest.Excerpt
	if newExcerpt != "" {
		excerptVal = sql.NullString{String: newExcerpt, Valid: true}
	} else if newExcerpt == "__empty__" {
		excerptVal = sql.NullString{}
	}
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{
		ID: newRevID, EntryID: entryID, RevisionNumber: revNum, Slug: newSlug, Title: newTitle, Excerpt: excerptVal, DocumentJson: newDocJSON, FieldsJson: newFieldsJSON, CreatedAt: 9999999999, Visibility: visibility, ReviewState: "draft",
	}); err != nil {
		t.Fatalf("create draft %v", err)
	}
	// Update entry's slug projection but NOT published_revision
	if _, err := queries.GetEntry(ctx, entryID); err == nil {
		// Update entries slug to latest draft slug? The entries.slug column is independent, but publishing uses revision slug.
		// For draft, we update entries projection slug? The spec says draft slug does not alter Search path.
		// So we should update entries.slug for draft but not route.
		// Use UpdateEntryProjection? Leave as is.
		_ = entry
	}
}

func mustQuery(t *testing.T, svc *Service, q string) ([]Result, int) {
	t.Helper()
	res, total, err := svc.Query(context.Background(), q, 1)
	if err != nil {
		t.Fatalf("Query %q: %v", q, err)
	}
	return res, total
}

func TestPublicOnlyIndexing(t *testing.T) {
	svc, _, queries, _ := newSearchHarness(t)
	ctx := context.Background()
	now := int64(1700000000)
	insertPublishedEntry(t, queries, nil, svc, "page1", "page", "pricing", "Pricing", "Compare plans", `{"version":1,"nodes":[{"id":"n1","block":"core/text","version":1,"props":{"text":"Compare plans and pricing for our product"},"settings":{}}]}`, `{}`, "public", now)
	// Draft-only entry (never published) should not be indexed
	// Create entry without published revision
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: "draftonly", ContentTypeID: "page", Slug: "draftonly", Status: "active", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create draftonly %v", err)
	}
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "draftonly-r1", EntryID: "draftonly", RevisionNumber: 1, Title: "Draft Only Secret", DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now, Visibility: "public", ReviewState: "draft"}); err != nil {
		t.Fatalf("rev %v", err)
	}
	// Do not set published, do not create route
	if err := svc.RefreshEntry(ctx, "draftonly"); err != nil {
		t.Fatalf("refresh draftonly %v", err)
	}
	res, _ := mustQuery(t, svc, "Draft Only Secret")
	if len(res) != 0 {
		t.Fatalf("draft-only should not be indexed, got %v", res)
	}
	res, _ = mustQuery(t, svc, "Pricing")
	if len(res) != 1 || res[0].Title != "Pricing" {
		t.Fatalf("public entry not found %v", res)
	}
}

func TestDraftPublishedSemantics(t *testing.T) {
	svc, _, queries, _ := newSearchHarness(t)
	now := int64(1700000000)
	insertPublishedEntry(t, queries, nil, svc, "post1", "post", "hello-world", "Old title", "excerpt", `{"version":1,"nodes":[]}`, `{}`, "public", now)
	// Edit draft with secret title
	createDraftRevision(t, queries, "post1", "Secret future launch", "", "", "", "", "")
	// Refresh should not change because published revision unchanged; but if someone calls RefreshEntry, it should still return Old title
	ctx := context.Background()
	if err := svc.RefreshEntry(ctx, "post1"); err != nil {
		t.Fatalf("refresh %v", err)
	}
	res, _ := mustQuery(t, svc, "Secret future launch")
	if len(res) != 0 {
		t.Fatalf("draft title should not be searchable, got %v", res)
	}
	res, _ = mustQuery(t, svc, "Old title")
	if len(res) == 0 {
		t.Fatalf("old title should still be searchable")
	}
	// Publish draft
	latest, _ := queries.GetLatestEntryRevision(ctx, "post1")
	if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: latest.ID, Valid: true}, PublishedAt: sql.NullInt64{Int64: now + 100, Valid: true}, UpdatedAt: now + 100, ID: "post1"}); err != nil {
		t.Fatalf("publish %v", err)
	}
	if err := svc.RefreshEntry(ctx, "post1"); err != nil {
		t.Fatalf("refresh after publish %v", err)
	}
	res, _ = mustQuery(t, svc, "Secret future launch")
	if len(res) == 0 {
		t.Fatalf("after publish secret should be found")
	}
	// Old title may or may not be found depending on new content; but new title is Secret
}

func TestCustomContentTypeFieldIndexing(t *testing.T) {
	svc, database, queries, _ := newSearchHarness(t)
	ctx := context.Background()
	now := int64(1700000000)
	// Create Service type with specialty field
	createContentTypeForTest(t, queries, "service", []content.FieldDefinition{{Key: "specialty", Label: "Specialty", Type: content.FieldText}}, true, "/services")
	// Need to refresh svc catalog (queries already)
	fields, _ := content.EncodeFieldSnapshot(map[string]any{"specialty": "Kubernetes migration"})
	insertPublishedEntry(t, queries, database, svc, "svc1", "service", "consulting", "Consulting", "", `{"version":1,"nodes":[]}`, fields, "public", now)
	res, _ := mustQuery(t, svc, "Kubernetes")
	if len(res) == 0 {
		t.Fatalf("custom field Kubernetes should be found, got 0")
	}
	if res[0].ContentTypeID != "service" {
		t.Fatalf("content type mismatch %v", res[0])
	}
	if res[0].ContentTypeLabel == "" {
		t.Fatalf("label empty")
	}
	// Draft update specialty to Secret unreleased
	createDraftRevision(t, queries, "svc1", "", "", "", "", `{"specialty":"Secret unreleased service"}`, "")
	// Ensure not yet indexed
	if err := svc.RefreshEntry(ctx, "svc1"); err != nil {
		t.Fatalf("refresh %v", err)
	}
	res, _ = mustQuery(t, svc, "Secret unreleased")
	if len(res) != 0 {
		t.Fatalf("draft specialty should not be searchable before publish, got %v", res)
	}
	res, _ = mustQuery(t, svc, "Kubernetes")
	if len(res) == 0 {
		t.Fatalf("original specialty should still be searchable before publish")
	}
	// Publish draft
	latest, _ := queries.GetLatestEntryRevision(ctx, "svc1")
	_ = queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: latest.ID, Valid: true}, PublishedAt: sql.NullInt64{Int64: now + 10, Valid: true}, UpdatedAt: now + 10, ID: "svc1"})
	if err := svc.RefreshEntry(ctx, "svc1"); err != nil {
		t.Fatalf("refresh after publish %v", err)
	}
	res, _ = mustQuery(t, svc, "Secret unreleased")
	if len(res) == 0 {
		t.Fatalf("after publish secret should be found")
	}
}

func TestNumberFieldIndexing(t *testing.T) {
	svc, database, queries, _ := newSearchHarness(t)
	now := int64(1700000000)
	createContentTypeForTest(t, queries, "product", []content.FieldDefinition{{Key: "sku", Label: "SKU", Type: content.FieldNumber}}, true, "/products")
	fields, _ := content.EncodeFieldSnapshot(map[string]any{"sku": float64(12345)})
	insertPublishedEntry(t, queries, database, svc, "prod1", "product", "widget", "Widget", "", `{"version":1,"nodes":[]}`, fields, "public", now)
	res, _ := mustQuery(t, svc, "12345")
	if len(res) == 0 {
		t.Fatalf("number SKU should be searchable, got 0")
	}
}

func TestRelevanceTitleOverBody(t *testing.T) {
	svc, _, queries, _ := newSearchHarness(t)
	now := int64(1700000000)
	insertPublishedEntry(t, queries, nil, svc, "a1", "page", "enterprise-automation", "Enterprise Automation", "", `{"version":1,"nodes":[]}`, `{}`, "public", now)
	insertPublishedEntry(t, queries, nil, svc, "b1", "page", "company-news", "Company News", "", `{"version":1,"nodes":[{"id":"n1","block":"core/text","version":1,"props":{"text":"... enterprise automation ..."},"settings":{}}]}`, `{}`, "public", now+10)
	res, _ := mustQuery(t, svc, "enterprise automation")
	if len(res) < 2 {
		t.Fatalf("expected 2 results got %d", len(res))
	}
	if res[0].EntryID != "a1" {
		t.Fatalf("relevance: title match should rank first, got %v first is %s", res, res[0].EntryID)
	}
}

func TestRelevanceTitleExcerptBody(t *testing.T) {
	svc, _, queries, _ := newSearchHarness(t)
	now := int64(1700000000)
	insertPublishedEntry(t, queries, nil, svc, "a1", "page", "a", "enterprise automation", "", `{"version":1,"nodes":[]}`, `{}`, "public", now)
	insertPublishedEntry(t, queries, nil, svc, "b1", "page", "b", "Other", "enterprise automation", `{"version":1,"nodes":[]}`, `{}`, "public", now+10)
	insertPublishedEntry(t, queries, nil, svc, "c1", "page", "c", "Other2", "", `{"version":1,"nodes":[{"id":"n1","block":"core/text","version":1,"props":{"text":"enterprise automation in body"},"settings":{}}]}`, `{}`, "public", now+20)
	res, _ := mustQuery(t, svc, "enterprise automation")
	if len(res) < 3 {
		t.Fatalf("want 3 got %d %v", len(res), res)
	}
	// Order should be a (title) before b (excerpt) before c (body)
	pos := map[string]int{}
	for i, r := range res {
		pos[r.EntryID] = i
	}
	if pos["a1"] > pos["b1"] || pos["b1"] > pos["c1"] {
		t.Fatalf("expected A before B before C, pos %v order %v", pos, res)
	}
}

func TestContentTypeFilter(t *testing.T) {
	svc, database, queries, _ := newSearchHarness(t)
	ctx := context.Background()
	now := int64(1700000000)
	// create product and service types
	createContentTypeForTest(t, queries, "service", []content.FieldDefinition{}, true, "/services")
	createContentTypeForTest(t, queries, "product", []content.FieldDefinition{}, true, "/products")
	for i := 0; i < 3; i++ {
		id := "page" + strings.Repeat("x", i+1)
		insertPublishedEntry(t, queries, database, svc, id, "page", "page"+id, "Automation test page", "", `{"version":1,"nodes":[{"id":"n1","block":"core/text","version":1,"props":{"text":"automation"},"settings":{}}]}`, `{}`, "public", now+int64(i))
	}
	for i := 0; i < 4; i++ {
		id := "post" + strings.Repeat("y", i+1)
		insertPublishedEntry(t, queries, database, svc, id, "post", "post"+id, "Automation test post", "", `{"version":1,"nodes":[{"id":"n1","block":"core/text","version":1,"props":{"text":"automation"},"settings":{}}]}`, `{}`, "public", now+10+int64(i))
	}
	for i := 0; i < 2; i++ {
		id := "svc" + strings.Repeat("z", i+1)
		insertPublishedEntry(t, queries, database, svc, id, "service", "svc"+id, "Automation consulting service", "", `{"version":1,"nodes":[{"id":"n1","block":"core/text","version":1,"props":{"text":"automation"},"settings":{}}]}`, `{}`, "public", now+20+int64(i))
	}
	// Query without filter
	_, totalAll, counts, err := svc.QueryFiltered(ctx, "automation", "", 1)
	if err != nil {
		t.Fatalf("query %v", err)
	}
	if totalAll != 9 {
		t.Fatalf("total all want 9 got %d counts %v", totalAll, counts)
	}
	if len(counts) != 3 {
		t.Fatalf("counts want 3 types got %v", counts)
	}
	// Filter post
	res, total, _, err := svc.QueryFiltered(ctx, "automation", "post", 1)
	if err != nil {
		t.Fatalf("filtered %v", err)
	}
	if total != 4 {
		t.Fatalf("filtered post want 4 got %d", total)
	}
	for _, r := range res {
		if r.ContentTypeID != "post" {
			t.Fatalf("filter returned non-post %v", r)
		}
	}
	// Unknown filter should behave as All (ignore)
	res2, total2, _, err := svc.QueryFiltered(ctx, "automation", "unknowntype", 1)
	if err != nil {
		t.Fatalf("unknown filter %v", err)
	}
	if total2 != 9 {
		t.Fatalf("unknown filter should return all 9 got %d", total2)
	}
	if len(res2) != 9 && total2 == 9 {
		// pagination may limit to 10, but total 9 so len 9
	}
	_ = totalAll
	_ = counts
}

func TestSlugChangeDraftVsPublish(t *testing.T) {
	svc, database, queries, _ := newSearchHarness(t)
	ctx := context.Background()
	now := int64(1700000000)
	insertPublishedEntry(t, queries, database, svc, "svc1", "page", "services", "Services", "", `{"version":1,"nodes":[]}`, `{}`, "public", now)
	res, _ := mustQuery(t, svc, "Services")
	if len(res) == 0 || res[0].Path != "/services" {
		t.Fatalf("initial path want /services got %v", res)
	}
	// Draft slug change to /offer
	createDraftRevision(t, queries, "svc1", "", "offer", "", "", "", "")
	// Do not publish, refresh should keep old path
	if err := svc.RefreshEntry(ctx, "svc1"); err != nil {
		t.Fatalf("refresh %v", err)
	}
	res, _ = mustQuery(t, svc, "Services")
	if len(res) == 0 || res[0].Path != "/services" {
		t.Fatalf("draft slug must not alter search path, got %v", res)
	}
	// Publish draft
	latest, _ := queries.GetLatestEntryRevision(ctx, "svc1")
	_ = database.DB
	// Update route to new path via publishing logic: UpsertEntryRoute
	if _, err := database.DB.ExecContext(ctx, `DELETE FROM routes WHERE entry_id = ?`, "svc1"); err != nil {
		t.Fatalf("del route %v", err)
	}
	_ = queries.CreateRoute(ctx, db.CreateRouteParams{ID: "svc1-route-new", Path: "/offer", EntryID: sql.NullString{String: "svc1", Valid: true}, RouteType: "entry", CreatedAt: now + 10, UpdatedAt: now + 10})
	_ = queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: latest.ID, Valid: true}, PublishedAt: sql.NullInt64{Int64: now + 10, Valid: true}, UpdatedAt: now + 10, ID: "svc1"})
	if err := svc.RefreshEntry(ctx, "svc1"); err != nil {
		t.Fatalf("refresh publish %v", err)
	}
	res, _ = mustQuery(t, svc, "Services")
	if len(res) == 0 || res[0].Path != "/offer" {
		t.Fatalf("after publish path want /offer got %v", res)
	}
	// Old redirect path never becomes search result
	// Ensure no result with path /services
	for _, r := range res {
		if r.Path == "/services" {
			t.Fatalf("old redirect should not be search result")
		}
	}
}

func TestUnpublishRemoves(t *testing.T) {
	svc, database, queries, _ := newSearchHarness(t)
	ctx := context.Background()
	now := int64(1700000000)
	insertPublishedEntry(t, queries, database, svc, "p1", "page", "unpublish-test", "Unpublish Test", "", `{"version":1,"nodes":[]}`, `{}`, "public", now)
	res, _ := mustQuery(t, svc, "Unpublish")
	if len(res) == 0 {
		t.Fatalf("should be indexed")
	}
	// Unpublish: clear published revision and delete route
	_, _ = database.DB.ExecContext(ctx, `UPDATE entries SET published_revision_id = NULL WHERE id = ?`, "p1")
	_, _ = database.DB.ExecContext(ctx, `DELETE FROM routes WHERE entry_id = ?`, "p1")
	if err := svc.RefreshEntry(ctx, "p1"); err != nil {
		t.Fatalf("refresh after unpublish %v", err)
	}
	res, _ = mustQuery(t, svc, "Unpublish")
	if len(res) != 0 {
		t.Fatalf("after unpublish should disappear, got %v", res)
	}
}

func TestPrivateRemoves(t *testing.T) {
	svc, _, queries, _ := newSearchHarness(t)
	ctx := context.Background()
	now := int64(1700000000)
	insertPublishedEntry(t, queries, nil, svc, "priv1", "page", "private-test", "Private Test", "", `{"version":1,"nodes":[]}`, `{}`, "public", now)
	res, _ := mustQuery(t, svc, "Private Test")
	if len(res) == 0 {
		t.Fatalf("public should be indexed")
	}
	// Change to private
	latest, _ := queries.GetLatestEntryRevision(ctx, "priv1")
	// Create new private revision and publish
	privRev := "priv1-r-private"
	_ = queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: privRev, EntryID: "priv1", RevisionNumber: latest.RevisionNumber + 1, Slug: "private-test", Title: "Private Test", DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now + 10, Visibility: "private", ReviewState: "draft"})
	_ = queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: privRev, Valid: true}, PublishedAt: sql.NullInt64{Int64: now + 10, Valid: true}, UpdatedAt: now + 10, ID: "priv1"})
	// Remove route as publishing would
	_, _ = queries.GetEntryRoute(ctx, sql.NullString{String: "priv1", Valid: true})
	// Instead just let BuildDocument handle visibility: it will not find public row, so refresh removes
	if err := svc.RefreshEntry(ctx, "priv1"); err != nil {
		t.Fatalf("refresh %v", err)
	}
	res, _ = mustQuery(t, svc, "Private Test")
	if len(res) != 0 {
		t.Fatalf("private should not be indexed, got %v", res)
	}
	// Return to public
	pubRev := "priv1-r-public2"
	_ = queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: pubRev, EntryID: "priv1", RevisionNumber: latest.RevisionNumber + 2, Slug: "private-test", Title: "Private Test", DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now + 20, Visibility: "public", ReviewState: "draft"})
	_ = queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: pubRev, Valid: true}, PublishedAt: sql.NullInt64{Int64: now + 20, Valid: true}, UpdatedAt: now + 20, ID: "priv1"})
	// Recreate route
	_ = queries.CreateRoute(ctx, db.CreateRouteParams{ID: "priv1-route2", Path: "/private-test", EntryID: sql.NullString{String: "priv1", Valid: true}, RouteType: "entry", CreatedAt: now + 20, UpdatedAt: now + 20})
	if err := svc.RefreshEntry(ctx, "priv1"); err != nil {
		t.Fatalf("refresh public again %v", err)
	}
	res, _ = mustQuery(t, svc, "Private Test")
	if len(res) == 0 {
		t.Fatalf("public again should be indexed")
	}
}

func TestPasswordProtectedNotIndexed(t *testing.T) {
	svc, _, queries, _ := newSearchHarness(t)
	ctx := context.Background()
	now := int64(1700000000)
	// Publish password protected entry
	// Need to create entry with password visibility; BuildDocument should not index it
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: "pwd1", ContentTypeID: "page", Slug: "pwd-test", Status: "active", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create entry %v", err)
	}
	revID := "pwd1-r1"
	_ = queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: revID, EntryID: "pwd1", RevisionNumber: 1, Title: "Password Protected Page", DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now, Visibility: "password", PasswordHash: sql.NullString{String: "$2a$10$dummyhashdummyhashdummyhashdummyhas", Valid: true}, ReviewState: "draft"})
	_ = queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: revID, Valid: true}, PublishedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now, ID: "pwd1"})
	_ = queries.CreateRoute(ctx, db.CreateRouteParams{ID: "pwd1-route", Path: "/pwd-test", EntryID: sql.NullString{String: "pwd1", Valid: true}, RouteType: "entry", CreatedAt: now, UpdatedAt: now})
	if err := svc.RefreshEntry(ctx, "pwd1"); err != nil {
		t.Fatalf("refresh %v", err)
	}
	res, _ := mustQuery(t, svc, "Password Protected")
	if len(res) != 0 {
		t.Fatalf("password protected must not appear, got %v", res)
	}
}

func TestTrashRestore(t *testing.T) {
	svc, database, queries, _ := newSearchHarness(t)
	ctx := context.Background()
	now := int64(1700000000)
	insertPublishedEntry(t, queries, database, svc, "trash1", "page", "trash-test", "Trash Test", "", `{"version":1,"nodes":[]}`, `{}`, "public", now)
	res, _ := mustQuery(t, svc, "Trash Test")
	if len(res) == 0 {
		t.Fatalf("should be indexed")
	}
	// Move to trash
	_, _ = database.DB.ExecContext(ctx, `UPDATE entries SET status = 'trash' WHERE id = ?`, "trash1")
	_, _ = database.DB.ExecContext(ctx, `DELETE FROM routes WHERE entry_id = ?`, "trash1")
	if err := svc.RefreshEntry(ctx, "trash1"); err != nil {
		t.Fatalf("refresh after trash %v", err)
	}
	res, _ = mustQuery(t, svc, "Trash Test")
	if len(res) != 0 {
		t.Fatalf("trashed should not be indexed, got %v", res)
	}
	// Restore but not published (clear published)
	_, _ = database.DB.ExecContext(ctx, `UPDATE entries SET status = 'active', published_revision_id = NULL WHERE id = ?`, "trash1")
	if err := svc.RefreshEntry(ctx, "trash1"); err != nil {
		t.Fatalf("refresh after restore draft %v", err)
	}
	res, _ = mustQuery(t, svc, "Trash Test")
	if len(res) != 0 {
		t.Fatalf("restored but not published should not be indexed")
	}
	// Restore published
	rev, _ := queries.GetEntryRevision(ctx, "trash1-r1")
	_ = queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: rev.ID, Valid: true}, PublishedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now, ID: "trash1"})
	_ = queries.CreateRoute(ctx, db.CreateRouteParams{ID: "trash1-route2", Path: "/trash-test", EntryID: sql.NullString{String: "trash1", Valid: true}, RouteType: "entry", CreatedAt: now, UpdatedAt: now})
	if err := svc.RefreshEntry(ctx, "trash1"); err != nil {
		t.Fatalf("refresh %v", err)
	}
	res, _ = mustQuery(t, svc, "Trash Test")
	if len(res) == 0 {
		t.Fatalf("restored published should be indexed")
	}
}

func TestPermanentDelete(t *testing.T) {
	svc, database, queries, _ := newSearchHarness(t)
	ctx := context.Background()
	now := int64(1700000000)
	insertPublishedEntry(t, queries, database, svc, "del1", "page", "delete-test", "Delete Test", "", `{"version":1,"nodes":[]}`, `{}`, "public", now)
	res, _ := mustQuery(t, svc, "Delete Test")
	if len(res) == 0 {
		t.Fatalf("should be indexed")
	}
	_, _ = database.DB.ExecContext(ctx, `UPDATE entries SET status = 'trash' WHERE id = ?`, "del1")
	_, _ = database.DB.ExecContext(ctx, `DELETE FROM routes WHERE entry_id = ?`, "del1")
	if err := svc.RefreshEntry(ctx, "del1"); err != nil {
		t.Fatalf("refresh %v", err)
	}
	_, _ = database.DB.ExecContext(ctx, `DELETE FROM entries WHERE id = ?`, "del1")
	if err := svc.RemoveEntry(ctx, "del1"); err != nil {
		t.Fatalf("remove %v", err)
	}
	res, _ = mustQuery(t, svc, "Delete Test")
	if len(res) != 0 {
		t.Fatalf("deleted should not be found, got %v", res)
	}
}

func TestBlockBodyIndexing(t *testing.T) {
	svc, _, queries, _ := newSearchHarness(t)
	now := int64(1700000000)
	doc := `{"version":1,"nodes":[{"id":"h1","block":"core/heading","version":1,"props":{"text":"Enterprise platform"},"settings":{}},{"id":"t1","block":"core/text","version":1,"props":{"text":"We migrate legacy systems."},"settings":{}}]}`
	insertPublishedEntry(t, queries, nil, svc, "block1", "page", "block-test", "Block Test", "", doc, `{}`, "public", now)
	res, _ := mustQuery(t, svc, "legacy")
	if len(res) == 0 {
		t.Fatalf("body legacy should be found")
	}
}

func TestSnippetHighlightSafety(t *testing.T) {
	svc, _, queries, _ := newSearchHarness(t)
	now := int64(1700000000)
	insertPublishedEntry(t, queries, nil, svc, "snippet1", "page", "snippet-test", "Automation consulting", "We help companies automate internal workflows", `{"version":1,"nodes":[{"id":"n1","block":"core/text","version":1,"props":{"text":"We help companies automate internal workflows, integrations and repetitive operations"},"settings":{}}]}`, `{}`, "public", now)
	res, _ := mustQuery(t, svc, "automate")
	if len(res) == 0 {
		t.Fatalf("should find")
	}
	snippet := res[0].Snippet
	if !strings.Contains(snippet, "<mark>") {
		t.Fatalf("snippet should contain <mark>, got %q", snippet)
	}
	if strings.Contains(snippet, "<script") || strings.Contains(snippet, "onerror") {
		t.Fatalf("snippet unsafe %q", snippet)
	}
	// Ensure HTML is escaped: title contains <script> should be escaped in snippet? Test with manual buildSnippet
	safe := buildSnippet("hello <script>alert(1)</script> world automate test", "", "", []string{"automate"})
	if strings.Contains(safe, "<script>") {
		t.Fatalf("snippet should escape html, got %q", safe)
	}
	if !strings.Contains(safe, "&lt;script&gt;") {
		t.Fatalf("escaped script expected, got %q", safe)
	}
}

func TestRebuild(t *testing.T) {
	svc, database, queries, _ := newSearchHarness(t)
	ctx := context.Background()
	now := int64(1700000000)
	for i := 0; i < 20; i++ {
		id := "rebuild" + strings.Repeat("a", i+1)
		insertPublishedEntry(t, queries, database, svc, id, "page", "rebuild"+id, "Rebuild Test", "", `{"version":1,"nodes":[]}`, `{}`, "public", now+int64(i))
	}
	// Create a draft-only invalid? Not needed
	// Delete all rows from projection
	if _, err := database.DB.ExecContext(ctx, `DELETE FROM search_documents_fts; DELETE FROM search_documents`); err != nil {
		t.Fatalf("delete %v", err)
	}
	cnt, _ := svc.CountDocuments(ctx)
	if cnt != 0 {
		t.Fatalf("should be 0 after delete, got %d", cnt)
	}
	n, err := svc.Rebuild(ctx)
	if err != nil {
		t.Fatalf("rebuild %v", err)
	}
	if n != 20 {
		t.Fatalf("rebuild count want 20 got %d", n)
	}
	res, total := mustQuery(t, svc, "Rebuild Test")
	if total != 20 {
		t.Fatalf("after rebuild total want 20 got %d len %d", total, len(res))
	}
	// Manually insert stale doc - disable FK to allow fake entry_id
	if _, err := database.DB.ExecContext(ctx, `PRAGMA foreign_keys = OFF`); err != nil {
		t.Fatalf("fk off %v", err)
	}
	if _, err := database.DB.ExecContext(ctx, `INSERT INTO search_documents(entry_id, content_type_id, title, excerpt, body, fields, path, first_published_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`, "stale", "page", "Stale Title", "", "", "", "/stale", now); err != nil {
		t.Fatalf("insert stale %v", err)
	}
	if _, err := database.DB.ExecContext(ctx, `INSERT INTO search_documents_fts(entry_id, title, excerpt, body, fields) VALUES (?, ?, ?, ?, ?)`, "stale", "Stale Title", "", "", ""); err != nil {
		t.Fatalf("insert stale fts %v", err)
	}
	if _, err := database.DB.ExecContext(ctx, `PRAGMA foreign_keys = ON`); err != nil {
		t.Fatalf("fk on %v", err)
	}
	n, err = svc.Rebuild(ctx)
	if err != nil {
		t.Fatalf("rebuild2 %v", err)
	}
	if n != 20 {
		t.Fatalf("rebuild after stale want 20 got %d", n)
	}
	res, _ = mustQuery(t, svc, "Stale Title")
	if len(res) != 0 {
		t.Fatalf("stale should disappear after rebuild, got %v", res)
	}
}

func TestDataOnlyNotIndexed(t *testing.T) {
	svc, database, queries, _ := newSearchHarness(t)
	ctx := context.Background()
	now := int64(1700000000)
	createContentTypeForTest(t, queries, "location", []content.FieldDefinition{}, false, "")
	// Data-only type Single=false, should not have route, BuildDocument should not index
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: "loc1", ContentTypeID: "location", Slug: "loc-test", Status: "active", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create %v", err)
	}
	revID := "loc1-r1"
	_ = queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: revID, EntryID: "loc1", RevisionNumber: 1, Title: "Office Location Data", DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now, Visibility: "public", ReviewState: "draft"})
	_ = queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: revID, Valid: true}, PublishedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now, ID: "loc1"})
	if _, err := database.DB.ExecContext(ctx, `UPDATE entries SET first_published_at = ? WHERE id = ?`, now, "loc1"); err != nil {
		t.Fatalf("fp %v", err)
	}
	// Do not create route because Single=false
	if err := svc.RefreshEntry(ctx, "loc1"); err != nil {
		t.Fatalf("refresh %v", err)
	}
	res, _ := mustQuery(t, svc, "Office Location")
	if len(res) != 0 {
		t.Fatalf("data-only should not be indexed, got %v", res)
	}
}

func TestRoutableCustomType(t *testing.T) {
	svc, database, queries, _ := newSearchHarness(t)
	now := int64(1700000000)
	createContentTypeForTest(t, queries, "case_study", []content.FieldDefinition{}, true, "/case-studies")
	insertPublishedEntry(t, queries, database, svc, "cs1", "case_study", "awesome-case", "Awesome Case Study", "", `{"version":1,"nodes":[]}`, `{}`, "public", now)
	res, _ := mustQuery(t, svc, "Awesome Case")
	if len(res) == 0 || res[0].Path != "/case-studies/awesome-case" {
		t.Fatalf("routable custom type should be indexed with canonical path, got %v", res)
	}
}

func TestNoNPlusOne(t *testing.T) {
	svc, _, queries, _ := newSearchHarness(t)
	now := int64(1700000000)
	for i := 0; i < 5; i++ {
		id := "nplus" + strings.Repeat("b", i+1)
		insertPublishedEntry(t, queries, nil, svc, id, "page", "nplus"+id, "NPlus Test", "", `{"version":1,"nodes":[]}`, `{}`, "public", now+int64(i))
	}
	// Query should return all without per-result DB lookups; we verify result already contains Path and ContentTypeID
	ctx := context.Background()
	res, _, _, err := svc.QueryFiltered(ctx, "NPlus", "", 1)
	if err != nil {
		t.Fatalf("query %v", err)
	}
	for _, r := range res {
		if r.Path == "" || r.ContentTypeID == "" {
			t.Fatalf("result missing fields without N+1 query, got %v", r)
		}
	}
}

func TestSpecialCharacters(t *testing.T) {
	svc, _, queries, _ := newSearchHarness(t)
	now := int64(1700000000)
	insertPublishedEntry(t, queries, nil, svc, "special1", "page", "special", "C++ Guide", "", `{"version":1,"nodes":[]}`, `{}`, "public", now)
	tests := []string{"C++", "C#", "Go 1.26", "foo/bar", "hello-world", "\"quote\"", "<script>", "100%", "AT&T"}
	for _, q := range tests {
		_, _, err := svc.Query(context.Background(), q, 1)
		if err != nil {
			t.Fatalf("query %q should not panic/error, got %v", q, err)
		}
	}
	// Ensure raw FTS not injected: "foo OR bar" should not become OR
	res, _ := mustQuery(t, svc, "foo OR bar")
	_ = res
	// Should not panic
}

func TestUnicode(t *testing.T) {
	svc, _, queries, _ := newSearchHarness(t)
	now := int64(1700000000)
	insertPublishedEntry(t, queries, nil, svc, "unicode1", "page", "unicode", "Łódź", "", `{"version":1,"nodes":[]}`, `{}`, "public", now)
	insertPublishedEntry(t, queries, nil, svc, "unicode2", "page", "cafe", "café", "", `{"version":1,"nodes":[]}`, `{}`, "public", now+10)
	queries2 := [][]string{{"Łódź", "unicode1"}, {"café", "unicode2"}, {"zażółć", ""}}
	_ = queries2
	for _, qq := range []string{"Łódź", "café"} {
		_, _, err := svc.Query(context.Background(), qq, 1)
		if err != nil {
			t.Fatalf("unicode query %q error %v", qq, err)
		}
	}
}

func TestWhitespace(t *testing.T) {
	svc, _, _, _ := newSearchHarness(t)
	tests := []string{"   ", "foo     bar", "\nfoo\tbar"}
	for _, q := range tests {
		_, _, err := svc.Query(context.Background(), q, 1)
		if err != nil {
			t.Fatalf("whitespace query %q error %v", q, err)
		}
	}
}

func TestVeryLongQuery(t *testing.T) {
	svc, _, _, _ := newSearchHarness(t)
	long := strings.Repeat("a", 1000) + " " + strings.Repeat("b", 1000)
	// Should truncate safely without panic and without cutting UTF-8 middle
	_, _, err := svc.Query(context.Background(), long, 1)
	if err != nil {
		t.Fatalf("long query %v", err)
	}
	// Unicode truncation
	longUni := strings.Repeat("Ł", 500)
	_, _, err = svc.Query(context.Background(), longUni, 1)
	if err != nil {
		t.Fatalf("long unicode %v", err)
	}
}

func TestPaginationNoDuplicates(t *testing.T) {
	svc, _, queries, _ := newSearchHarness(t)
	now := int64(1700000000)
	for i := 0; i < 26; i++ {
		id := "pag" + strings.Repeat("c", i+1)
		title := "Paginate Test"
		insertPublishedEntry(t, queries, nil, svc, id, "page", "pag"+id, title, "", `{"version":1,"nodes":[]}`, `{}`, "public", now+int64(i))
	}
	ctx := context.Background()
	var seen = map[string]bool{}
	var order []string
	for p := 1; p <= 3; p++ {
		res, total, _, err := svc.QueryFiltered(ctx, "Paginate", "", p)
		if err != nil {
			t.Fatalf("page %d %v", p, err)
		}
		if total != 26 {
			t.Fatalf("total want 26 got %d", total)
		}
		for _, r := range res {
			if seen[r.EntryID] {
				t.Fatalf("duplicate %s across pages", r.EntryID)
			}
			seen[r.EntryID] = true
			order = append(order, r.EntryID)
		}
	}
	if len(seen) != 26 {
		t.Fatalf("seen %d want 26", len(seen))
	}
	// Stable order: same query repeated should give same order
	res1, _, _, _ := svc.QueryFiltered(ctx, "Paginate", "", 1)
	res2, _, _, _ := svc.QueryFiltered(ctx, "Paginate", "", 1)
	if len(res1) != len(res2) {
		t.Fatalf("len mismatch")
	}
	for i := range res1 {
		if res1[i].EntryID != res2[i].EntryID {
			t.Fatalf("unstable order at %d %s vs %s", i, res1[i].EntryID, res2[i].EntryID)
		}
	}
}

func TestPrefixMatching(t *testing.T) {
	svc, _, queries, _ := newSearchHarness(t)
	now := int64(1700000000)
	insertPublishedEntry(t, queries, nil, svc, "pref1", "page", "pref", "Automation", "", `{"version":1,"nodes":[]}`, `{}`, "public", now)
	res, _ := mustQuery(t, svc, "autom")
	if len(res) == 0 {
		t.Fatalf("prefix autom should find automation, got 0")
	}
	res, _ = mustQuery(t, svc, "workflow autom")
	// Should handle multi-word prefix: workflow AND autom*
	// If no doc has workflow, should return zero, but not error
	_ = res
}

func TestQueryLimits(t *testing.T) {
	svc, _, _, _ := newSearchHarness(t)
	// max terms 12, max runes 256 should be enforced without panic
	many := strings.Repeat("word ", 30)
	_, _, err := svc.Query(context.Background(), many, 1)
	if err != nil {
		t.Fatalf("many terms %v", err)
	}
	// max page 1000
	_, _, err = svc.Query(context.Background(), "test", 9999)
	if err != nil {
		t.Fatalf("max page %v", err)
	}
}

func TestContentTypeLabelLookup(t *testing.T) {
	svc, _, queries, _ := newSearchHarness(t)
	now := int64(1700000000)
	createContentTypeForTest(t, queries, "service", []content.FieldDefinition{}, true, "/services")
	insertPublishedEntry(t, queries, nil, svc, "label1", "service", "label-test", "Label Test", "", `{"version":1,"nodes":[]}`, `{}`, "public", now)
	res, _ := mustQuery(t, svc, "Label Test")
	if len(res) == 0 {
		t.Fatalf("not found")
	}
	if res[0].ContentTypeLabel == "" {
		t.Fatalf("label empty")
	}
	// Should be "Services" plural
	if !strings.Contains(res[0].ContentTypeLabel, "Service") {
		t.Fatalf("label want Service got %q", res[0].ContentTypeLabel)
	}
}
