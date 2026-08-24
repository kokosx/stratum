package public

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

// TestSitemapExcludesNoindexEntries verifies the indexability filter: a
// published, active entry whose published revision overrides to noindex must
// not appear in the sitemap, while its indexable neighbours stay listed.
func TestSitemapExcludesNoindexEntries(t *testing.T) {
	handler, queries := setupSite(t)
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) {
		p.SiteUrl = "https://example.com"
		p.SitemapEnabled = 1
	})
	ctx := context.Background()

	// Published entry with a revision-level noindex override.
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: "noidx", ContentTypeID: "page", Slug: "noidx", Status: "active", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{
		ID: "noidx-r1", EntryID: "noidx", RevisionNumber: 1, Title: "Noindex",
		DocumentJson: testDoc, CreatedAt: 100,
		SeoRobotsIndex: sql.NullInt64{Int64: 0, Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{
		PublishedRevisionID: sql.NullString{String: "noidx-r1", Valid: true},
		PublishedAt:         sql.NullInt64{Int64: 100, Valid: true},
		UpdatedAt:           1, ID: "noidx",
	}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateRoute(ctx, db.CreateRouteParams{
		ID: "noidx-route", Path: "/noidx", EntryID: sql.NullString{String: "noidx", Valid: true}, RouteType: "entry", CreatedAt: 1, UpdatedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}
	reloadRoutesForTest(t, queries)

	// A regular indexable entry for contrast.
	seedEntry(t, queries, "idx", "page", "idx", "/idx", "active", "idx-r1", 1, "Indexable", 200, true)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil))
	body := rec.Body.String()
	if rec.Code != http.StatusOK {
		t.Fatalf("sitemap = %d", rec.Code)
	}
	if !strings.Contains(body, "https://example.com/idx") {
		t.Fatalf("indexable entry missing from sitemap:\n%s", body)
	}
	if strings.Contains(body, "/noidx") {
		t.Fatalf("noindex entry must be excluded from the sitemap:\n%s", body)
	}
}

// TestSitemapEmptyWhenSiteIndexingDisabled verifies that with site-wide
// indexing off (every page resolves noindex) the sitemap serves an empty
// urlset instead of advertising non-indexable URLs.
func TestSitemapEmptyWhenSiteIndexingDisabled(t *testing.T) {
	handler, queries := setupSite(t)
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) {
		p.SiteUrl = "https://example.com"
		p.SitemapEnabled = 1
		p.IndexingEnabled = 0
	})
	seedEntry(t, queries, "gone", "page", "gone", "/gone", "active", "gone-r1", 1, "Gone", 200, true)

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("sitemap = %d", rec.Code)
	}
	body := rec.Body.String()
	if strings.Contains(body, "<url>") || strings.Contains(body, "/gone") {
		t.Fatalf("sitemap must be empty when indexing is disabled:\n%s", body)
	}
	if !strings.Contains(body, "<urlset") {
		t.Fatalf("expected a valid empty urlset:\n%s", body)
	}
}
