package public

import (
	"context"
	"database/sql"
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

// TestDraftSeoDoesNotLeakToPublic verifies that a newer draft revision carrying
// different SEO values (title, description, canonical override) does not change
// the public <title>, <meta name="description"> or <link rel="canonical"> until
// the draft is published.
func TestDraftSeoDoesNotLeakToPublic(t *testing.T) {
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

	// Configure the canonical public origin.
	settings, err := queries.GetSiteSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if err := queries.UpdateSiteSettings(ctx, db.UpdateSiteSettingsParams{
		SiteTitle:            settings.SiteTitle,
		SiteTagline:          settings.SiteTagline,
		HomepageMode:         settings.HomepageMode,
		HomepageEntryID:      settings.HomepageEntryID,
		PostsPageEntryID:     settings.PostsPageEntryID,
		PostsPerPage:         settings.PostsPerPage,
		Language:             settings.Language,
		Timezone:             settings.Timezone,
		ActiveTheme:          settings.ActiveTheme,
		IndexingEnabled:      settings.IndexingEnabled,
		SiteUrl:              "https://example.com",
		SitemapEnabled:       settings.SitemapEnabled,
		RobotsMode:           settings.RobotsMode,
		RobotsCustom:         settings.RobotsCustom,
		SpeculationMode:      settings.SpeculationMode,
		SpeculationEagerness: settings.SpeculationEagerness,
		TitleSeparator:       settings.TitleSeparator,
		UpdatedAt:            settings.UpdatedAt,
	}); err != nil {
		t.Fatal(err)
	}

	const (
		entryID   = "seo-entry"
		routePath = "/seo-page"
		rev1ID    = "seo-rev-1"
		rev2ID    = "seo-rev-2"
		doc       = `{"version":1,"nodes":[]}`
	)

	if err := queries.CreateEntry(ctx, db.CreateEntryParams{
		ID:            entryID,
		ContentTypeID: "page",
		Slug:          "seo-page",
		Status:        "active",
		CreatedAt:     1,
		UpdatedAt:     1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateRoute(ctx, db.CreateRouteParams{
		ID:        "seo-route",
		Path:      routePath,
		EntryID:   sql.NullString{String: entryID, Valid: true},
		RouteType: "entry",
		CreatedAt: 1,
		UpdatedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}
	// Published revision with its own SEO values.
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{
		ID:             rev1ID,
		EntryID:        entryID,
		RevisionNumber: 1,
		Title:          "Published Title",
		DocumentJson:   doc,
		SeoTitle:       sql.NullString{String: "Published SEO Title", Valid: true},
		SeoDescription: sql.NullString{String: "Published description.", Valid: true},
		CreatedAt:      1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{
		PublishedRevisionID: sql.NullString{String: rev1ID, Valid: true},
		PublishedAt:         sql.NullInt64{Int64: 1, Valid: true},
		UpdatedAt:           1,
		ID:                  entryID,
	}); err != nil {
		t.Fatal(err)
	}

	render := func() string {
		request := httptest.NewRequest(http.MethodGet, routePath, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != http.StatusOK {
			t.Fatalf("render %s = %d, body = %s", routePath, response.Code, response.Body.String())
		}
		return response.Body.String()
	}

	published := render()
	for _, want := range []string{
		"<title>Published SEO Title · StratumCMS</title>",
		`<meta name="description" content="Published description.">`,
		`<link rel="canonical" href="https://example.com/seo-page">`,
	} {
		if !strings.Contains(published, want) {
			t.Fatalf("published page missing %q in:\n%s", want, published)
		}
	}

	// Newer draft with different SEO values and a canonical override. Not published.
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{
		ID:             rev2ID,
		EntryID:        entryID,
		RevisionNumber: 2,
		Title:          "Draft Title",
		DocumentJson:   doc,
		SeoTitle:       sql.NullString{String: "Draft SEO Title", Valid: true},
		SeoDescription: sql.NullString{String: "Draft description.", Valid: true},
		CanonicalUrl:   sql.NullString{String: "https://evil.example/override", Valid: true},
		CreatedAt:      2,
	}); err != nil {
		t.Fatal(err)
	}

	draft := render()
	for _, want := range []string{
		"<title>Published SEO Title · StratumCMS</title>",
		`<meta name="description" content="Published description.">`,
		`<link rel="canonical" href="https://example.com/seo-page">`,
	} {
		if !strings.Contains(draft, want) {
			t.Fatalf("draft leaked into public page, missing %q in:\n%s", want, draft)
		}
	}
	for _, forbidden := range []string{
		"Draft SEO Title",
		"Draft description.",
		"https://evil.example/override",
	} {
		if strings.Contains(draft, forbidden) {
			t.Fatalf("draft SEO leaked into public page, found %q in:\n%s", forbidden, draft)
		}
	}
}
