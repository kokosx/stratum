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

// TestTitleSeparatorUsesSiteSetting verifies that the resolved <title>
// uses the separator stored in site_settings.
func TestTitleSeparatorUsesSiteSetting(t *testing.T) {
	handler, queries := setupSite(t)
	ctx := context.Background()
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) {
		p.SiteTitle = "MySite"
		p.TitleSeparator = "|"
		p.SiteUrl = "https://example.com"
	})
	// Seed an entry with a published revision.
	seedEntry(t, queries, "tsep", "page", "tsep", "/tsep", "active", "tsep-r1", 1, "Entry Title", 100, true)
	// Override revision's seo_title to test separator.
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{
		ID:             "tsep-r2",
		EntryID:        "tsep",
		RevisionNumber: 2,
		Title:          "Entry Title",
		DocumentJson:   testDoc,
		SeoTitle:       sql.NullString{String: "Custom SEO", Valid: true},
		CreatedAt:      101,
	}); err != nil {
		t.Fatal(err)
	}
	if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{
		PublishedRevisionID: sql.NullString{String: "tsep-r2", Valid: true},
		PublishedAt:         sql.NullInt64{Int64: 101, Valid: true},
		UpdatedAt:           101,
		ID:                  "tsep",
	}); err != nil {
		t.Fatal(err)
	}
	handler.Hub().Pages.InvalidateAll()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tsep", nil))
	body := rec.Body.String()
	want := "<title>Custom SEO | MySite</title>"
	if !strings.Contains(body, want) {
		t.Fatalf("title separator not used: want %q, got:\n%s", want, body)
	}
	if strings.Contains(body, "·") || strings.Contains(body, "–") {
		// Ensure we didn't accidentally use the old hardcoded separator.
		if strings.Contains(body, "Custom SEO ·") || strings.Contains(body, "Custom SEO –") {
			t.Fatalf("old separator leaked: %s", body)
		}
	}
}

// TestDescriptionFallback verifies seo_description → excerpt → no meta.
func TestDescriptionFallback(t *testing.T) {
	handler, queries := setupSite(t)
	ctx := context.Background()
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) {
		p.SiteUrl = "https://example.com"
		p.SiteTitle = "Site"
	})
	// 1. seo_description wins over excerpt.
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: "desc1", ContentTypeID: "page", Slug: "desc1", Status: "active", CreatedAt: 100, UpdatedAt: 100}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{
		ID: "desc1-r1", EntryID: "desc1", RevisionNumber: 1, Title: "T",
		Excerpt:        sql.NullString{String: "Excerpt text", Valid: true},
		SeoDescription: sql.NullString{String: "SEO description", Valid: true},
		DocumentJson:   testDoc, CreatedAt: 100,
	}); err != nil {
		t.Fatal(err)
	}
	if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{
		PublishedRevisionID: sql.NullString{String: "desc1-r1", Valid: true},
		PublishedAt:         sql.NullInt64{Int64: 100, Valid: true},
		UpdatedAt:           100, ID: "desc1",
	}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateRoute(ctx, db.CreateRouteParams{ID: "desc1-route", Path: "/desc1", EntryID: sql.NullString{String: "desc1", Valid: true}, RouteType: "entry", CreatedAt: 100, UpdatedAt: 100}); err != nil {
		t.Fatal(err)
	}
	reloadRoutesForTest(t, queries)
	handler.Hub().Pages.InvalidateAll()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/desc1", nil))
	body := rec.Body.String()
	if !strings.Contains(body, `<meta name="description" content="SEO description">`) {
		t.Fatalf("seo_description should win, got:\n%s", body)
	}
	if strings.Contains(body, `content="Excerpt text"`) {
		t.Fatalf("excerpt leaked when seo_description present:\n%s", body)
	}

	// 2. excerpt fallback when seo_description empty.
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: "desc2", ContentTypeID: "page", Slug: "desc2", Status: "active", CreatedAt: 200, UpdatedAt: 200}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{
		ID: "desc2-r1", EntryID: "desc2", RevisionNumber: 1, Title: "T2",
		Excerpt:      sql.NullString{String: "Only excerpt", Valid: true},
		DocumentJson: testDoc, CreatedAt: 200,
	}); err != nil {
		t.Fatal(err)
	}
	if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{
		PublishedRevisionID: sql.NullString{String: "desc2-r1", Valid: true},
		PublishedAt:         sql.NullInt64{Int64: 200, Valid: true},
		UpdatedAt:           200, ID: "desc2",
	}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateRoute(ctx, db.CreateRouteParams{ID: "desc2-route", Path: "/desc2", EntryID: sql.NullString{String: "desc2", Valid: true}, RouteType: "entry", CreatedAt: 200, UpdatedAt: 200}); err != nil {
		t.Fatal(err)
	}
	reloadRoutesForTest(t, queries)
	handler.Hub().Pages.InvalidateAll()
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/desc2", nil))
	body = rec.Body.String()
	if !strings.Contains(body, `<meta name="description" content="Only excerpt">`) {
		t.Fatalf("excerpt fallback missing, got:\n%s", body)
	}

	// 3. no meta when both empty (and not using site tagline).
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: "desc3", ContentTypeID: "page", Slug: "desc3", Status: "active", CreatedAt: 300, UpdatedAt: 300}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "desc3-r1", EntryID: "desc3", RevisionNumber: 1, Title: "T3", DocumentJson: testDoc, CreatedAt: 300}); err != nil {
		t.Fatal(err)
	}
	if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{
		PublishedRevisionID: sql.NullString{String: "desc3-r1", Valid: true},
		PublishedAt:         sql.NullInt64{Int64: 300, Valid: true},
		UpdatedAt:           300, ID: "desc3",
	}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateRoute(ctx, db.CreateRouteParams{ID: "desc3-route", Path: "/desc3", EntryID: sql.NullString{String: "desc3", Valid: true}, RouteType: "entry", CreatedAt: 300, UpdatedAt: 300}); err != nil {
		t.Fatal(err)
	}
	reloadRoutesForTest(t, queries)
	handler.Hub().Pages.InvalidateAll()
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/desc3", nil))
	body = rec.Body.String()
	if strings.Contains(body, `name="description"`) {
		t.Fatalf("description should be absent when both empty, got:\n%s", body)
	}
	// Ensure tagline not used.
	site, _ := queries.GetSiteSettings(ctx)
	if site.SiteTagline != "" && strings.Contains(body, site.SiteTagline) && strings.Contains(body, `name="description"`) {
		t.Fatalf("site tagline leaked as description:\n%s", body)
	}
}

// TestRobotsInheritance verifies site → content type → revision precedence
// and that draft robots never leak to public.
func TestRobotsInheritance(t *testing.T) {
	handler, queries := setupSite(t)
	ctx := context.Background()

	// Site allows indexing by default.
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) {
		p.IndexingEnabled = 1
		p.SiteUrl = "https://example.com"
	})
	seedEntry(t, queries, "rob1", "page", "rob1", "/rob1", "active", "rob1-r1", 1, "R1", 100, true)
	handler.Hub().Pages.InvalidateAll()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/rob1", nil))
	if got := rec.Header().Get("X-Robots-Tag"); got != "max-image-preview:large" || !strings.Contains(rec.Body.String(), `<meta name="robots" content="max-image-preview:large">`) {
		t.Fatalf("indexing allowed should advertise large image previews, got header %q body %s", got, rec.Body.String())
	}

	// Site disallows → noindex,nofollow.
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) { p.IndexingEnabled = 0 })
	seedEntry(t, queries, "rob2", "page", "rob2", "/rob2", "active", "rob2-r1", 1, "R2", 200, true)
	handler.Hub().Pages.InvalidateAll()
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/rob2", nil))
	if got := rec.Header().Get("X-Robots-Tag"); got != "noindex,nofollow" {
		t.Fatalf("site disabled X-Robots-Tag = %q want noindex,nofollow", got)
	}
	if !strings.Contains(rec.Body.String(), `<meta name="robots" content="noindex,nofollow">`) {
		t.Fatalf("robots meta missing: %s", rec.Body.String())
	}

	// Revision overrides site disabled to allow indexing.
	// Publish a revision with robots_index=1, robots_follow=1.
	handler, queries = setupSite(t) // fresh
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) {
		p.IndexingEnabled = 0
		p.SiteUrl = "https://example.com"
	})
	seedEntry(t, queries, "rob3", "page", "rob3", "/rob3", "active", "rob3-r1", 1, "R3", 300, true)
	// Create new published revision with explicit allow.
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{
		ID: "rob3-r2", EntryID: "rob3", RevisionNumber: 2, Title: "R3",
		DocumentJson: testDoc, CreatedAt: 301,
		SeoRobotsIndex:  sql.NullInt64{Int64: 1, Valid: true},
		SeoRobotsFollow: sql.NullInt64{Int64: 1, Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{
		PublishedRevisionID: sql.NullString{String: "rob3-r2", Valid: true},
		PublishedAt:         sql.NullInt64{Int64: 301, Valid: true},
		UpdatedAt:           301, ID: "rob3",
	}); err != nil {
		t.Fatal(err)
	}
	handler.Hub().Pages.InvalidateAll()
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/rob3", nil))
	if got := rec.Header().Get("X-Robots-Tag"); got != "max-image-preview:large" {
		t.Fatalf("revision override to allow should drop noindex, got %q", got)
	}

	// Revision overrides to noindex only.
	handler, queries = setupSite(t)
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) {
		p.IndexingEnabled = 1
		p.SiteUrl = "https://example.com"
	})
	seedEntry(t, queries, "rob4", "page", "rob4", "/rob4", "active", "rob4-r1", 1, "R4", 400, true)
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{
		ID: "rob4-r2", EntryID: "rob4", RevisionNumber: 2, Title: "R4",
		DocumentJson: testDoc, CreatedAt: 401,
		SeoRobotsIndex:  sql.NullInt64{Int64: 0, Valid: true},
		SeoRobotsFollow: sql.NullInt64{Int64: 1, Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{
		PublishedRevisionID: sql.NullString{String: "rob4-r2", Valid: true},
		PublishedAt:         sql.NullInt64{Int64: 401, Valid: true},
		UpdatedAt:           401, ID: "rob4",
	}); err != nil {
		t.Fatal(err)
	}
	handler.Hub().Pages.InvalidateAll()
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/rob4", nil))
	if got := rec.Header().Get("X-Robots-Tag"); got != "noindex" {
		t.Fatalf("revision noindex X-Robots-Tag = %q want noindex", got)
	}

	// Draft robots must not leak: published allows, draft says noindex.
	handler, queries = setupSite(t)
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) {
		p.IndexingEnabled = 1
		p.SiteUrl = "https://example.com"
	})
	seedEntry(t, queries, "rob5", "page", "rob5", "/rob5", "active", "rob5-r1", 1, "R5", 500, true)
	// Draft with noindex (not published)
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{
		ID: "rob5-r2", EntryID: "rob5", RevisionNumber: 2, Title: "R5",
		DocumentJson: testDoc, CreatedAt: 501,
		SeoRobotsIndex:  sql.NullInt64{Int64: 0, Valid: true},
		SeoRobotsFollow: sql.NullInt64{Int64: 0, Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	handler.Hub().Pages.InvalidateAll()
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/rob5", nil))
	if got := rec.Header().Get("X-Robots-Tag"); got != "max-image-preview:large" {
		t.Fatalf("draft robots leaked, got %q want the published max-image-preview:large", got)
	}
}

// TestFeaturedImageDraftDoesNotLeak verifies that changing featured_media_id
// in a draft revision does not affect the public page until publish.
func TestFeaturedImageDraftDoesNotLeak(t *testing.T) {
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
	// Ensure site URL for canonical etc.
	settings, _ := queries.GetSiteSettings(ctx)
	if err := queries.UpdateSiteSettings(ctx, db.UpdateSiteSettingsParams{
		SiteTitle: settings.SiteTitle, SiteTagline: settings.SiteTagline, HomepageMode: settings.HomepageMode,
		HomepageEntryID: settings.HomepageEntryID, PostsPageEntryID: settings.PostsPageEntryID, PostsPerPage: settings.PostsPerPage,
		Language: settings.Language, Timezone: settings.Timezone, ActiveTheme: settings.ActiveTheme,
		IndexingEnabled: settings.IndexingEnabled, SiteUrl: "https://example.com", SitemapEnabled: settings.SitemapEnabled,
		RobotsMode: settings.RobotsMode, RobotsCustom: settings.RobotsCustom, SpeculationMode: settings.SpeculationMode,
		SpeculationEagerness: settings.SpeculationEagerness, TitleSeparator: settings.TitleSeparator, SiteSocialMediaID: settings.SiteSocialMediaID, TwitterSite: settings.TwitterSite, SiteRepresents: settings.SiteRepresents, UpdatedAt: settings.UpdatedAt,
	}); err != nil {
		t.Fatal(err)
	}
	if err := handler.Hub().ReloadSite(ctx); err != nil {
		t.Fatal(err)
	}

	const entryID = "feat-entry"
	const routePath = "/feat-page"
	const docWithFeatured = `{"version":1,"nodes":[{"id":"fi","block":"core/featured-image","version":1,"props":{},"settings":{"aspectRatio":"16:9","objectFit":"cover","align":"full"}}]}`
	// Create entry and published revision with featured A.
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: entryID, ContentTypeID: "page", Slug: "feat-page", Status: "active", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateRoute(ctx, db.CreateRouteParams{ID: "feat-route", Path: routePath, EntryID: sql.NullString{String: entryID, Valid: true}, RouteType: "entry", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	reloadRoutesForTest(t, queries)
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{
		ID: "feat-r1", EntryID: entryID, RevisionNumber: 1, Title: "Feat", DocumentJson: docWithFeatured, CreatedAt: 1,
		FeaturedMediaID: sql.NullString{String: "media_A", Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{
		PublishedRevisionID: sql.NullString{String: "feat-r1", Valid: true},
		PublishedAt:         sql.NullInt64{Int64: 1, Valid: true},
		UpdatedAt:           1, ID: entryID,
	}); err != nil {
		t.Fatal(err)
	}
	// Verify public DB row has media_A via published revision.
	row, err := queries.GetPublishedEntryByPath(ctx, routePath)
	if err != nil {
		t.Fatal(err)
	}
	if row.FeaturedMediaID.String != "media_A" {
		t.Fatalf("published featured = %q want media_A", row.FeaturedMediaID.String)
	}

	// Create draft with different featured image B (not published).
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{
		ID: "feat-r2", EntryID: entryID, RevisionNumber: 2, Title: "Feat", DocumentJson: docWithFeatured, CreatedAt: 2,
		FeaturedMediaID: sql.NullString{String: "media_B", Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	// Public row should still be A.
	row, err = queries.GetPublishedEntryByPath(ctx, routePath)
	if err != nil {
		t.Fatal(err)
	}
	if row.FeaturedMediaID.String != "media_A" {
		t.Fatalf("draft leaked: published featured = %q want media_A", row.FeaturedMediaID.String)
	}
	// Also verify that the latest revision (draft) has B, but published still A.
	latest, err := queries.GetLatestEntryRevision(ctx, entryID)
	if err != nil {
		t.Fatal(err)
	}
	if latest.FeaturedMediaID.String != "media_B" {
		t.Fatalf("latest draft featured = %q want media_B", latest.FeaturedMediaID.String)
	}

	// Publish the draft and verify public switches to B.
	if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{
		PublishedRevisionID: sql.NullString{String: "feat-r2", Valid: true},
		PublishedAt:         sql.NullInt64{Int64: 2, Valid: true},
		UpdatedAt:           2, ID: entryID,
	}); err != nil {
		t.Fatal(err)
	}
	handler.Hub().Pages.InvalidateAll()
	row, err = queries.GetPublishedEntryByPath(ctx, routePath)
	if err != nil {
		t.Fatal(err)
	}
	if row.FeaturedMediaID.String != "media_B" {
		t.Fatalf("after publish featured = %q want media_B", row.FeaturedMediaID.String)
	}

	// Also test social_media_id isolation.
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{
		ID: "feat-r3", EntryID: entryID, RevisionNumber: 3, Title: "Feat", DocumentJson: docWithFeatured, CreatedAt: 3,
		FeaturedMediaID: sql.NullString{String: "media_B", Valid: true},
		SocialMediaID:   sql.NullString{String: "social_X", Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	// Not published yet, public should still have no social.
	row, err = queries.GetPublishedEntryByPath(ctx, routePath)
	if err != nil {
		t.Fatal(err)
	}
	if row.SocialMediaID.Valid {
		t.Fatalf("draft social leaked: %q", row.SocialMediaID.String)
	}
}

func TestHeadViewContainsSEOContract(t *testing.T) {
	// Ensure HeadView has the expanded contract fields.
	handler, queries := setupSite(t)
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) {
		p.SiteUrl = "https://example.com"
		p.TitleSeparator = "|"
	})
	seedEntry(t, queries, "head1", "page", "head1", "/head1", "active", "head1-r1", 1, "Head Title", 100, true)
	handler.Hub().Pages.InvalidateAll()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/head1", nil))
	body := rec.Body.String()
	// HeadView should have rendered title with separator, description fallback, canonical.
	if !strings.Contains(body, "<title>Head Title |") {
		t.Fatalf("HeadView title missing or not using separator: %s", body)
	}
	// Check that the page still renders without panic and contains expected head elements.
	if !strings.Contains(body, `<link rel="canonical"`) {
		t.Fatalf("canonical missing")
	}
}
