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

// setupSite returns a public handler backed by a fresh database seeded with the
// default content. The returned queries allow tests to mutate site settings and
// entries directly.
func setupSite(t *testing.T) (*Handler, *db.Queries) {
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
	// The media service backs both the block renderer (MediaProvider) and the
	// handler, mirroring the production runtime wiring.
	mediaService := newTestMedia(t, queries)
	registry, err := blocks.NewRegistry(ctx, queries, mediaService)
	if err != nil {
		t.Fatal(err)
	}
	themeRuntime, err := themes.NewRuntime(ctx, queries)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewHandler(queries, registry, themeRuntime, mediaService)
	if err != nil {
		t.Fatal(err)
	}
	return handler, queries
}

// setSettings overwrites the singleton site_settings row.
// When handler is non-nil it reloads the site snapshot so the public handler
// sees the new settings immediately (mirrors the admin write path).
func setSettings(t *testing.T, queries *db.Queries, patch func(*db.UpdateSiteSettingsParams)) {
	t.Helper()
	setSettingsWithHandler(t, nil, queries, patch)
}

func setSettingsWithHandler(t *testing.T, handler *Handler, queries *db.Queries, patch func(*db.UpdateSiteSettingsParams)) {
	t.Helper()
	ctx := context.Background()
	current, err := queries.GetSiteSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	params := db.UpdateSiteSettingsParams{
		SiteTitle:            current.SiteTitle,
		SiteTagline:          current.SiteTagline,
		HomepageMode:         current.HomepageMode,
		HomepageEntryID:      current.HomepageEntryID,
		PostsPageEntryID:     current.PostsPageEntryID,
		PostsPerPage:         current.PostsPerPage,
		Language:             current.Language,
		Timezone:             current.Timezone,
		ActiveTheme:          current.ActiveTheme,
		IndexingEnabled:      current.IndexingEnabled,
		SiteUrl:              current.SiteUrl,
		SitemapEnabled:       current.SitemapEnabled,
		RobotsMode:           current.RobotsMode,
		RobotsCustom:         current.RobotsCustom,
		SpeculationMode:      current.SpeculationMode,
		SpeculationEagerness: current.SpeculationEagerness,
		TitleSeparator:       current.TitleSeparator,
		SiteSocialMediaID:    current.SiteSocialMediaID,
		TwitterSite:          current.TwitterSite,
		SiteRepresents:       current.SiteRepresents,
		UpdatedAt:            current.UpdatedAt,
	}
	patch(&params)
	if err := queries.UpdateSiteSettings(ctx, params); err != nil {
		t.Fatal(err)
	}
	if handler != nil {
		if err := handler.Hub().ReloadSite(ctx); err != nil {
			t.Fatal(err)
		}
	}
}

const testDoc = `{"version":1,"nodes":[]}`

func seedEntry(t *testing.T, queries *db.Queries, id, contentType, slug, path, status, revID string, revNum int64, title string, revCreatedAt int64, published bool) {
	t.Helper()
	ctx := context.Background()
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{
		ID: id, ContentTypeID: contentType, Slug: slug, Status: status, CreatedAt: 1, UpdatedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{
		ID: revID, EntryID: id, RevisionNumber: revNum, Title: title, DocumentJson: testDoc, CreatedAt: revCreatedAt,
	}); err != nil {
		t.Fatal(err)
	}
	if published {
		if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{
			PublishedRevisionID: sql.NullString{String: revID, Valid: true},
			PublishedAt:         sql.NullInt64{Int64: revCreatedAt, Valid: true},
			UpdatedAt:           1, ID: id,
		}); err != nil {
			t.Fatal(err)
		}
	}
	if err := queries.CreateRoute(ctx, db.CreateRouteParams{
		ID: id + "-route", Path: path, EntryID: sql.NullString{String: id, Valid: true}, RouteType: "entry", CreatedAt: 1, UpdatedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}
}

func TestSitemapDisabledReturns404(t *testing.T) {
	handler, queries := setupSite(t)
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) { p.SitemapEnabled = 0 })
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("disabled sitemap = %d, body = %s", rec.Code, rec.Body.String())
	}
}

func TestSitemapEnabledIncludesPublicEntries(t *testing.T) {
	handler, queries := setupSite(t)
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) {
		p.SiteUrl = "https://example.com"
		p.SitemapEnabled = 1
	})
	seedEntry(t, queries, "team", "page", "team", "/team", "active", "team-r1", 1, "Team", 200, true)
	seedEntry(t, queries, "intro", "post", "intro", "/news/intro", "active", "intro-r1", 1, "Intro", 300, true)
	// Homepage is already seeded at "/".

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("sitemap = %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/xml; charset=utf-8" {
		t.Fatalf("content type = %q", ct)
	}
	body := rec.Body.String()
	for _, want := range []string{
		"https://example.com/",
		"https://example.com/team",
		"https://example.com/news/intro",
		"<lastmod>1970-01-01T00:03:20Z</lastmod>", // ts 200
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("sitemap missing %q in:\n%s", want, body)
		}
	}
}

func TestSitemapExcludesNonPublic(t *testing.T) {
	handler, queries := setupSite(t)
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) {
		p.SiteUrl = "https://example.com"
		p.SitemapEnabled = 1
	})
	ctx := context.Background()
	seedEntry(t, queries, "draft", "page", "draft", "/draft", "active", "draft-r1", 1, "Draft", 400, false)
	seedEntry(t, queries, "priv", "page", "priv", "/priv", "private", "priv-r1", 1, "Private", 500, false)
	seedEntry(t, queries, "trash", "page", "trash", "/trash", "trash", "trash-r1", 1, "Trash", 600, false)
	// redirect route (not an entry route)
	if err := queries.CreateRoute(ctx, db.CreateRouteParams{
		ID: "redir", Path: "/old", RouteType: "redirect", RedirectTo: sql.NullString{String: "/about", Valid: true}, RedirectStatus: sql.NullInt64{Int64: 301, Valid: true}, CreatedAt: 1, UpdatedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil))
	body := rec.Body.String()
	for _, forbidden := range []string{"/draft", "/priv", "/trash", "/old"} {
		if strings.Contains(body, forbidden) {
			t.Fatalf("sitemap must exclude %q, got:\n%s", forbidden, body)
		}
	}
}

func TestSitemapUsesPublishedRevisionTimestamp(t *testing.T) {
	handler, queries := setupSite(t)
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) {
		p.SiteUrl = "https://example.com"
		p.SitemapEnabled = 1
	})
	seedEntry(t, queries, "team", "page", "team", "/team", "active", "team-r1", 1, "Team", 200, true)
	// Newer draft must NOT change the sitemap lastmod.
	if err := queries.CreateEntryRevision(context.Background(), db.CreateEntryRevisionParams{
		ID: "team-r2", EntryID: "team", RevisionNumber: 2, Title: "Team Draft", DocumentJson: testDoc, CreatedAt: 999,
	}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil))
	body := rec.Body.String()
	if !strings.Contains(body, "<lastmod>1970-01-01T00:03:20Z</lastmod>") {
		t.Fatalf("sitemap should use published revision timestamp, got:\n%s", body)
	}
	if strings.Contains(body, "1970-01-01T00:16:39Z") { // ts 999
		t.Fatalf("sitemap leaked draft timestamp:\n%s", body)
	}
}

func TestRobotsManagedIndexingEnabled(t *testing.T) {
	handler, queries := setupSite(t)
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) {
		p.SiteUrl = "https://example.com"
		p.IndexingEnabled = 1
		p.SitemapEnabled = 1
		p.RobotsMode = "managed"
	})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/robots.txt", nil))
	body := rec.Body.String()
	if ct := rec.Header().Get("Content-Type"); ct != "text/plain; charset=utf-8" {
		t.Fatalf("content type = %q", ct)
	}
	for _, want := range []string{
		"User-agent: *", "Allow: /", "Disallow: /admin/", "Sitemap: https://example.com/sitemap.xml",
	} {
		if !strings.Contains(body, want) {
			t.Fatalf("robots missing %q in:\n%s", want, body)
		}
	}
}

func TestRobotsManagedIndexingDisabled(t *testing.T) {
	handler, queries := setupSite(t)
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) {
		p.SiteUrl = "https://example.com"
		p.IndexingEnabled = 0
		p.SitemapEnabled = 1
		p.RobotsMode = "managed"
	})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/robots.txt", nil))
	body := rec.Body.String()
	if !strings.Contains(body, "Disallow: /") || strings.Contains(body, "Sitemap:") {
		t.Fatalf("indexing-off robots should disallow all and omit sitemap:\n%s", body)
	}
}

func TestRobotsCustomReturnedExactly(t *testing.T) {
	handler, queries := setupSite(t)
	custom := "User-agent: *\nDisallow: /secret/\n"
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) {
		p.RobotsMode = "custom"
		p.RobotsCustom = custom
	})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/robots.txt", nil))
	if rec.Body.String() != custom {
		t.Fatalf("custom robots mismatch, got:\n%q", rec.Body.String())
	}
}

func TestRobotsSitemapDeclarationOnlyWhenAppropriate(t *testing.T) {
	handler, queries := setupSite(t)
	// Indexing on but sitemap disabled and no site URL -> no Sitemap line.
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) {
		p.IndexingEnabled = 1
		p.SitemapEnabled = 0
		p.RobotsMode = "managed"
	})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/robots.txt", nil))
	if strings.Contains(rec.Body.String(), "Sitemap:") {
		t.Fatalf("sitemap declaration should be absent when sitemap disabled:\n%s", rec.Body.String())
	}
}

func TestIndexingDisabledAddsXRobotsTag(t *testing.T) {
	handler, queries := setupSite(t)
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) { p.IndexingEnabled = 0 })
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("home = %d", rec.Code)
	}
	if got := rec.Header().Get("X-Robots-Tag"); got != "noindex,nofollow" {
		t.Fatalf("X-Robots-Tag = %q", got)
	}
	if !strings.Contains(rec.Body.String(), `<meta name="robots" content="noindex,nofollow">`) {
		t.Fatalf("expected noindex meta in head:\n%s", rec.Body.String())
	}
}

func TestCanonicalGeneratedFromSiteURL(t *testing.T) {
	handler, queries := setupSite(t)
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) { p.SiteUrl = "https://example.com" })
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/about", nil))
	if !strings.Contains(rec.Body.String(), `<link rel="canonical" href="https://example.com/about">`) {
		t.Fatalf("canonical not derived from site_url:\n%s", rec.Body.String())
	}
}

func TestCanonicalOverrideWins(t *testing.T) {
	handler, queries := setupSite(t)
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) { p.SiteUrl = "https://example.com" })
	ctx := context.Background()
	seedEntry(t, queries, "canon", "page", "canon", "/canon", "active", "canon-r1", 1, "Canon", 200, true)
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{
		ID: "canon-r2", EntryID: "canon", RevisionNumber: 2, Title: "Canon", DocumentJson: testDoc,
		CanonicalUrl: sql.NullString{String: "https://canonical.example/override", Valid: true}, CreatedAt: 201,
	}); err != nil {
		t.Fatal(err)
	}
	// Publish the override revision so it becomes the canonical source.
	if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{
		PublishedRevisionID: sql.NullString{String: "canon-r2", Valid: true},
		PublishedAt:         sql.NullInt64{Int64: 201, Valid: true}, UpdatedAt: 1, ID: "canon",
	}); err != nil {
		t.Fatal(err)
	}
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/canon", nil))
	if !strings.Contains(rec.Body.String(), `<link rel="canonical" href="https://canonical.example/override">`) {
		t.Fatalf("canonical override should win:\n%s", rec.Body.String())
	}
}

func TestSpeculationDisabledNoScript(t *testing.T) {
	handler, queries := setupSite(t)
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) { p.SpeculationMode = "off" })
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/about", nil))
	if strings.Contains(rec.Body.String(), "speculationrules") {
		t.Fatalf("disabled speculation should not emit a script:\n%s", rec.Body.String())
	}
}

func TestSpeculationEnabledValidJSON(t *testing.T) {
	handler, queries := setupSite(t)
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) {
		p.SpeculationMode = "prefetch"
		p.SpeculationEagerness = "conservative"
	})
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/about", nil))
	body := rec.Body.String()
	if !strings.Contains(body, `<script type="speculationrules">`) {
		t.Fatalf("expected speculationrules script:\n%s", body)
	}
	if !strings.Contains(body, "prefetch") || !strings.Contains(body, "/admin/*") || !strings.Contains(body, "data-no-speculate") {
		t.Fatalf("speculation rules missing safe exclusions:\n%s", body)
	}
	if strings.Contains(body, "http://external.example") {
		t.Fatalf("speculation must not target external URLs:\n%s", body)
	}
}
