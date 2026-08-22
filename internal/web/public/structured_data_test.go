package public

import (
	"context"
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/themes"
)

// setupStructuredSite is setupSite plus the raw database handle, so tests can
// write columns that have no admin UI yet (social_links) directly.
func setupStructuredSite(t *testing.T) (*Handler, *db.Queries, *sql.DB) {
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
	if err := database.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	queries := db.New(database.DB)
	registry, err := blocks.NewRegistry(ctx, queries)
	if err != nil {
		t.Fatal(err)
	}
	themeRuntime, err := themes.NewRuntime(ctx, queries)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewHandler(queries, registry, themeRuntime, newTestMedia(t, queries))
	if err != nil {
		t.Fatal(err)
	}
	return handler, queries, database.DB
}

const ldScriptTag = `<script type="application/ld+json">`

// structuredDoc fetches path and decodes the JSON-LD payload from the response.
func structuredDoc(t *testing.T, handler *Handler, path string) map[string]any {
	t.Helper()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	body := rec.Body.String()
	start := strings.Index(body, ldScriptTag)
	if start < 0 {
		t.Fatalf("no %s in response for %s:\n%s", ldScriptTag, path, body)
	}
	start += len(ldScriptTag)
	end := strings.Index(body[start:], "</script>")
	if end < 0 {
		t.Fatalf("unterminated JSON-LD script for %s:\n%s", path, body)
	}
	var doc map[string]any
	if err := json.Unmarshal([]byte(body[start:start+end]), &doc); err != nil {
		t.Fatalf("structured data is not valid JSON: %v\n%s", err, body[start:start+end])
	}
	return doc
}

func graphNode(t *testing.T, doc map[string]any, id string) map[string]any {
	t.Helper()
	graph, ok := doc["@graph"].([]any)
	if !ok {
		t.Fatalf("missing @graph in:\n%v", doc)
	}
	for _, raw := range graph {
		if n, ok := raw.(map[string]any); ok && n["@id"] == id {
			return n
		}
	}
	t.Fatalf("no node with @id %q in graph:\n%v", id, doc)
	return nil
}

func refID(t *testing.T, value any, want string) {
	t.Helper()
	m, ok := value.(map[string]any)
	if !ok || m["@id"] != want {
		t.Fatalf("expected @id ref %q, got %v", want, value)
	}
}

func TestStructuredDataValidJSONGraphOnPage(t *testing.T) {
	handler, queries, _ := setupStructuredSite(t)
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) { p.SiteUrl = "https://example.com" })
	doc := structuredDoc(t, handler, "/")
	if doc["@context"] != "https://schema.org" {
		t.Fatalf("@context = %v", doc["@context"])
	}
	graph, ok := doc["@graph"].([]any)
	if !ok || len(graph) < 2 {
		t.Fatalf("expected a multi-node @graph, got:\n%v", doc)
	}
}

func TestStructuredDataWebSiteNode(t *testing.T) {
	handler, queries, _ := setupStructuredSite(t)
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) { p.SiteUrl = "https://example.com" })
	doc := structuredDoc(t, handler, "/about")
	siteNode := graphNode(t, doc, "https://example.com/#website")
	if siteNode["@type"] != "WebSite" {
		t.Fatalf("@type = %v", siteNode["@type"])
	}
	if siteNode["name"] != "StratumCMS" || siteNode["url"] != "https://example.com" {
		t.Fatalf("name/url = %v/%v", siteNode["name"], siteNode["url"])
	}
	refID(t, siteNode["publisher"], "https://example.com/#organization")
}

func TestStructuredDataOrganizationWithSameAs(t *testing.T) {
	handler, queries, rawDB := setupStructuredSite(t)
	ctx := context.Background()
	sameAs := `[{"platform":"x","url":"https://x.com/example","label":"X"}]`
	if _, err := rawDB.ExecContext(ctx, `UPDATE site_settings SET social_links = ? WHERE id = 1`, sameAs); err != nil {
		t.Fatal(err)
	}
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) { p.SiteUrl = "https://example.com" })
	doc := structuredDoc(t, handler, "/about")
	org := graphNode(t, doc, "https://example.com/#organization")
	if org["@type"] != "Organization" || org["name"] != "StratumCMS" || org["url"] != "https://example.com" {
		t.Fatalf("organization = %v", org)
	}
	links, ok := org["sameAs"].([]any)
	if !ok || len(links) != 1 || links[0] != "https://x.com/example" {
		t.Fatalf("sameAs = %v", org["sameAs"])
	}
}

func TestStructuredDataPersonPublisherWhenConfigured(t *testing.T) {
	handler, queries, _ := setupStructuredSite(t)
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) {
		p.SiteUrl = "https://example.com"
		p.SiteRepresents = "person"
	})
	doc := structuredDoc(t, handler, "/about")
	person := graphNode(t, doc, "https://example.com/#person")
	if person["@type"] != "Person" {
		t.Fatalf("person = %v", person)
	}
	for _, raw := range doc["@graph"].([]any) {
		if n, ok := raw.(map[string]any); ok && n["@id"] == "https://example.com/#organization" {
			t.Fatalf("organization must be absent when the site represents a person")
		}
	}
}

func TestStructuredDataWebPageForPageContentType(t *testing.T) {
	handler, queries, _ := setupStructuredSite(t)
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) { p.SiteUrl = "https://example.com" })
	ctx := context.Background()
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: "team", ContentTypeID: "page", Slug: "team", Status: "active", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{
		ID: "team-r1", EntryID: "team", RevisionNumber: 1, Title: "Team",
		Excerpt:      sql.NullString{String: "Meet the team.", Valid: true},
		DocumentJson: testDoc, CreatedAt: 200,
	}); err != nil {
		t.Fatal(err)
	}
	publishRevision(t, queries, "team", "team-r1", 200)
	if err := queries.CreateRoute(ctx, db.CreateRouteParams{ID: "team-route", Path: "/team", EntryID: sql.NullString{String: "team", Valid: true}, RouteType: "entry", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	handler.Hub().Pages.InvalidateAll()

	doc := structuredDoc(t, handler, "/team")
	page := graphNode(t, doc, "https://example.com/team/#webpage")
	if page["@type"] != "WebPage" {
		t.Fatalf("@type = %v", page["@type"])
	}
	if page["url"] != "https://example.com/team" || page["name"] != "Team" {
		t.Fatalf("url/name = %v/%v", page["url"], page["name"])
	}
	if page["description"] != "Meet the team." {
		t.Fatalf("description = %v", page["description"])
	}
	if page["inLanguage"] != "en" {
		t.Fatalf("inLanguage = %v", page["inLanguage"])
	}
	refID(t, page["isPartOf"], "https://example.com/#website")
	refID(t, page["breadcrumb"], "https://example.com/team/#breadcrumb")

	crumb := graphNode(t, doc, "https://example.com/team/#breadcrumb")
	items, ok := crumb["itemListElement"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("breadcrumb items = %v", crumb["itemListElement"])
	}
	last := items[1].(map[string]any)
	if last["item"] != "https://example.com/team" {
		t.Fatalf("current breadcrumb item = %v", last)
	}
}

func TestStructuredDataBlogPostingForPostContentType(t *testing.T) {
	handler, queries, _ := setupStructuredSite(t)
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) { p.SiteUrl = "https://example.com" })
	ctx := context.Background()
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: "post-sd", ContentTypeID: "post", Slug: "hello", Status: "active", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{
		ID: "post-sd-r1", EntryID: "post-sd", RevisionNumber: 1, Title: "Hello Structured Data",
		Excerpt:       sql.NullString{String: "An introduction.", Valid: true},
		SocialMediaID: sql.NullString{String: uploadTestImage(t, handler, nil, 1300, 700), Valid: true},
		DocumentJson:  testDoc, CreatedAt: 100,
	}); err != nil {
		t.Fatal(err)
	}
	publishRevision(t, queries, "post-sd", "post-sd-r1", 1700000000)
	if err := queries.CreateRoute(ctx, db.CreateRouteParams{ID: "post-sd-route", Path: "/blog/hello", EntryID: sql.NullString{String: "post-sd", Valid: true}, RouteType: "entry", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	handler.Hub().Pages.InvalidateAll()

	doc := structuredDoc(t, handler, "/blog/hello")
	article := graphNode(t, doc, "https://example.com/blog/hello/#article")
	if article["@type"] != "BlogPosting" || article["headline"] != "Hello Structured Data" {
		t.Fatalf("article = %v", article)
	}
	if article["description"] != "An introduction." {
		t.Fatalf("description = %v", article["description"])
	}
	refID(t, article["mainEntityOfPage"], "https://example.com/blog/hello/#webpage")
	refID(t, article["publisher"], "https://example.com/#organization")
	if _, has := article["author"]; has {
		t.Fatalf("private admin user data must not become an author: %v", article["author"])
	}

	img := graphNode(t, doc, "https://example.com/blog/hello/#primaryimage")
	if img["url"] == "" || img["width"] != float64(1200) || img["height"] != float64(630) {
		t.Fatalf("image object = %v", img)
	}
	images, ok := article["image"].([]any)
	if !ok || len(images) != 1 {
		t.Fatalf("article image = %v", article["image"])
	}
	page := graphNode(t, doc, "https://example.com/blog/hello/#webpage")
	refID(t, page["primaryImageOfPage"], "https://example.com/blog/hello/#primaryimage")
}

// publishRevision mimics the admin write path: record first publication (once),
// then publish the revision at the given timestamp.
func publishRevision(t *testing.T, queries *db.Queries, entryID, revisionID string, publishedAt int64) {
	t.Helper()
	ctx := context.Background()
	if err := queries.SetFirstPublishedAtIfNull(ctx, db.SetFirstPublishedAtIfNullParams{FirstPublishedAt: sql.NullInt64{Int64: publishedAt, Valid: true}, ID: entryID}); err != nil {
		t.Fatal(err)
	}
	if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{
		PublishedRevisionID: sql.NullString{String: revisionID, Valid: true},
		PublishedAt:         sql.NullInt64{Int64: publishedAt, Valid: true},
		UpdatedAt:           publishedAt, ID: entryID,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestStructuredDataDatePublishedStaysFixedOnRepublish(t *testing.T) {
	handler, queries, _ := setupStructuredSite(t)
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) { p.SiteUrl = "https://example.com" })
	ctx := context.Background()
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: "dates", ContentTypeID: "post", Slug: "dates", Status: "active", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "dates-r1", EntryID: "dates", RevisionNumber: 1, Title: "Dates One", DocumentJson: testDoc, CreatedAt: 100}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateRoute(ctx, db.CreateRouteParams{ID: "dates-route", Path: "/blog/dates", EntryID: sql.NullString{String: "dates", Valid: true}, RouteType: "entry", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	publishRevision(t, queries, "dates", "dates-r1", 1700000000)
	handler.Hub().Pages.InvalidateAll()

	before := structuredDoc(t, handler, "/blog/dates")
	article := graphNode(t, before, "https://example.com/blog/dates/#article")
	if article["datePublished"] != "2023-11-14T22:13:20Z" {
		t.Fatalf("initial datePublished = %v", article["datePublished"])
	}

	// A later revision publishes much later; datePublished must not move.
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "dates-r2", EntryID: "dates", RevisionNumber: 2, Title: "Dates Two", DocumentJson: testDoc, CreatedAt: 200}); err != nil {
		t.Fatal(err)
	}
	publishRevision(t, queries, "dates", "dates-r2", 1800000000)
	handler.Hub().Pages.InvalidateAll()

	after := structuredDoc(t, handler, "/blog/dates")
	article = graphNode(t, after, "https://example.com/blog/dates/#article")
	if article["datePublished"] != "2023-11-14T22:13:20Z" {
		t.Fatalf("datePublished moved on re-publish: %v", article["datePublished"])
	}
	if article["dateModified"] != "2027-01-15T08:00:00Z" {
		t.Fatalf("dateModified = %v, want the new publication time", article["dateModified"])
	}
}

func TestStructuredDataIDsAreStableAcrossRenders(t *testing.T) {
	handler, queries, _ := setupStructuredSite(t)
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) { p.SiteUrl = "https://example.com" })
	seedEntry(t, queries, "stable", "post", "stable", "/blog/stable", "active", "stable-r1", 1, "Stable", 300, true)
	handler.Hub().Pages.InvalidateAll()

	first := structuredDoc(t, handler, "/blog/stable")
	second := structuredDoc(t, handler, "/blog/stable")
	for _, id := range []string{
		"https://example.com/#website",
		"https://example.com/#organization",
		"https://example.com/blog/stable/#webpage",
		"https://example.com/blog/stable/#article",
	} {
		graphNode(t, first, id)
		graphNode(t, second, id)
	}
}

func TestStructuredDataOmitsDraftRevisionData(t *testing.T) {
	handler, queries, _ := setupStructuredSite(t)
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) { p.SiteUrl = "https://example.com" })
	ctx := context.Background()
	draftImage := uploadTestImage(t, handler, nil, 1300, 700)
	seedEntry(t, queries, "draftsd", "page", "draftsd", "/draftsd", "active", "draftsd-r1", 1, "Public Title", 400, true)
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{
		ID: "draftsd-r2", EntryID: "draftsd", RevisionNumber: 2, Title: "Draft Title",
		Excerpt:       sql.NullString{String: "Draft excerpt.", Valid: true},
		SocialMediaID: sql.NullString{String: draftImage, Valid: true},
		DocumentJson:  testDoc, CreatedAt: 500,
	}); err != nil {
		t.Fatal(err)
	}
	handler.Hub().Pages.InvalidateAll()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/draftsd", nil))
	body := rec.Body.String()
	start := strings.Index(body, ldScriptTag)
	if start < 0 {
		t.Fatalf("no structured data script:\n%s", body)
	}
	payload := body[start+len(ldScriptTag):]
	payload = payload[:strings.Index(payload, "</script>")]
	if strings.Contains(payload, "Draft Title") || strings.Contains(payload, draftImage) || strings.Contains(payload, "Draft excerpt") {
		t.Fatalf("draft revision data leaked into structured data:\n%s", payload)
	}
	if !strings.Contains(payload, "Public Title") {
		t.Fatalf("published title missing from structured data:\n%s", payload)
	}

	// A fully unpublished entry serves a 404 with no structured data at all.
	seedEntry(t, queries, "pure-draft", "page", "puredraft", "/puredraft", "active", "pd-r1", 1, "Pure Draft", 600, false)
	handler.Hub().Pages.InvalidateAll()
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/puredraft", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unpublished entry status = %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), ldScriptTag) {
		t.Fatalf("draft entry must not emit structured data:\n%s", rec.Body.String())
	}
}
