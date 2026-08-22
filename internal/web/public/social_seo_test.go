package public

import (
	"bytes"
	"context"
	"database/sql"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/media"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func testPNGBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{R: 100, G: 150, B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func uploadTestImage(t *testing.T, handler *Handler, _ interface{}, w, h int) string {
	// Use handler's media service directly
	ctx := context.Background()
	asset, err := handler.Hub().Media.Upload(ctx, "test.png", "", bytes.NewReader(testPNGBytes(t, w, h)))
	if err != nil {
		t.Fatalf("upload test image %dx%d: %v", w, h, err)
	}
	return asset.ID
}

func bodyContains(t *testing.T, body, substr string) {
	t.Helper()
	if !strings.Contains(body, substr) {
		t.Fatalf("expected body to contain %q, got:\n%s", substr, body)
	}
}
func bodyNotContains(t *testing.T, body, substr string) {
	t.Helper()
	if strings.Contains(body, substr) {
		t.Fatalf("expected body NOT to contain %q, got:\n%s", substr, body)
	}
}

func TestSocialOGTypePageVsPost(t *testing.T) {
	handler, queries := setupSite(t)
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) { p.SiteUrl = "https://example.com" })
	// Page should be website
	seedEntry(t, queries, "page1", "page", "page1", "/page1", "active", "page1-r1", 1, "Page One", 100, true)
	// Post should be article
	seedEntry(t, queries, "post1", "post", "post1", "/news/post1", "active", "post1-r1", 1, "Post One", 101, true)
	handler.Hub().Pages.InvalidateAll()

	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/page1", nil))
	body := rec.Body.String()
	bodyContains(t, body, `<meta property="og:type" content="website">`)

	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/news/post1", nil))
	body = rec.Body.String()
	bodyContains(t, body, `<meta property="og:type" content="article">`)
}

func TestSocialTitleDescriptionFallback(t *testing.T) {
	handler, queries := setupSite(t)
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) {
		p.SiteUrl = "https://example.com"
		p.SiteTitle = "SiteName"
	})
	ctx := context.Background()
	// Entry with seo_title and seo_description: OG should use those (raw)
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: "seo1", ContentTypeID: "post", Slug: "seo1", Status: "active", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{
		ID: "seo1-r1", EntryID: "seo1", RevisionNumber: 1, Title: "Entry Title",
		Excerpt:        sql.NullString{String: "Excerpt text", Valid: true},
		SeoTitle:       sql.NullString{String: "SEO Title", Valid: true},
		SeoDescription: sql.NullString{String: "SEO Desc", Valid: true},
		DocumentJson:   testDoc, CreatedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: "seo1-r1", Valid: true}, PublishedAt: sql.NullInt64{Int64: 1, Valid: true}, UpdatedAt: 1, ID: "seo1"}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateRoute(ctx, db.CreateRouteParams{ID: "seo1-route", Path: "/seo1", EntryID: sql.NullString{String: "seo1", Valid: true}, RouteType: "entry", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	handler.Hub().Pages.InvalidateAll()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/seo1", nil))
	body := rec.Body.String()
	bodyContains(t, body, `<meta property="og:title" content="SEO Title">`)
	bodyContains(t, body, `<meta property="og:description" content="SEO Desc">`)
	bodyContains(t, body, `<meta name="twitter:title" content="SEO Title">`)
	bodyContains(t, body, `<meta name="twitter:description" content="SEO Desc">`)
	// Ensure site title suffix not in OG (OG is raw)
	if strings.Contains(body, `<meta property="og:title" content="SEO Title |`) {
		t.Fatalf("og:title should be raw without site suffix, got:\n%s", body)
	}
	// Fallback: when seo_title empty, entry title used
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: "seo2", ContentTypeID: "post", Slug: "seo2", Status: "active", CreatedAt: 2, UpdatedAt: 2}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{
		ID: "seo2-r1", EntryID: "seo2", RevisionNumber: 1, Title: "Fallback Title",
		Excerpt:      sql.NullString{String: "Excerpt Fallback", Valid: true},
		DocumentJson: testDoc, CreatedAt: 2,
	}); err != nil {
		t.Fatal(err)
	}
	if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: "seo2-r1", Valid: true}, PublishedAt: sql.NullInt64{Int64: 2, Valid: true}, UpdatedAt: 2, ID: "seo2"}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateRoute(ctx, db.CreateRouteParams{ID: "seo2-route", Path: "/seo2", EntryID: sql.NullString{String: "seo2", Valid: true}, RouteType: "entry", CreatedAt: 2, UpdatedAt: 2}); err != nil {
		t.Fatal(err)
	}
	handler.Hub().Pages.InvalidateAll()
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/seo2", nil))
	body = rec.Body.String()
	bodyContains(t, body, `<meta property="og:title" content="Fallback Title">`)
	bodyContains(t, body, `<meta property="og:description" content="Excerpt Fallback">`)
}

func TestSocialImageOverride(t *testing.T) {
	handler, queries := setupSite(t)
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) { p.SiteUrl = "https://example.com" })
	ctx := context.Background()
	socialID := uploadTestImage(t, handler, nil, 1300, 700)
	featID := uploadTestImage(t, handler, nil, 1300, 700)
	// Entry with both: social should win
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: "img1", ContentTypeID: "page", Slug: "img1", Status: "active", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{
		ID: "img1-r1", EntryID: "img1", RevisionNumber: 1, Title: "Img", DocumentJson: testDoc, CreatedAt: 1,
		FeaturedMediaID: sql.NullString{String: featID, Valid: true},
		SocialMediaID:   sql.NullString{String: socialID, Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: "img1-r1", Valid: true}, PublishedAt: sql.NullInt64{Int64: 1, Valid: true}, UpdatedAt: 1, ID: "img1"}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateRoute(ctx, db.CreateRouteParams{ID: "img1-route", Path: "/img1", EntryID: sql.NullString{String: "img1", Valid: true}, RouteType: "entry", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	handler.Hub().Pages.InvalidateAll()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/img1", nil))
	body := rec.Body.String()
	// Social image URL should contain social ID, not featured
	if !strings.Contains(body, "/media/"+socialID+"/social") {
		t.Fatalf("social image should win, body missing social id %s:\n%s", socialID, body)
	}
	if strings.Contains(body, "/media/"+featID+"/social") {
		t.Fatalf("featured image leaked when social present:\n%s", body)
	}
	// OG and Twitter should share same image; derivative URLs are content-
	// hashed (?v=) so regenerated bytes never hide behind an immutable cache.
	bodyContains(t, body, `<meta property="og:image" content="https://example.com/media/`+socialID+`/social?v=`)
	bodyContains(t, body, `<meta name="twitter:image" content="https://example.com/media/`+socialID+`/social?v=`)
	bodyContains(t, body, `<meta name="twitter:card" content="summary_large_image">`)
	// Check dimensions for 1200x630 variant
	bodyContains(t, body, `<meta property="og:image:width" content="1200">`)
	bodyContains(t, body, `<meta property="og:image:height" content="630">`)
}

func TestSocialFeaturedFallback(t *testing.T) {
	handler, queries := setupSite(t)
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) { p.SiteUrl = "https://example.com" })
	ctx := context.Background()
	featID := uploadTestImage(t, handler, nil, 1300, 700)
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: "img2", ContentTypeID: "page", Slug: "img2", Status: "active", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{
		ID: "img2-r1", EntryID: "img2", RevisionNumber: 1, Title: "Img2", DocumentJson: testDoc, CreatedAt: 1,
		FeaturedMediaID: sql.NullString{String: featID, Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: "img2-r1", Valid: true}, PublishedAt: sql.NullInt64{Int64: 1, Valid: true}, UpdatedAt: 1, ID: "img2"}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateRoute(ctx, db.CreateRouteParams{ID: "img2-route", Path: "/img2", EntryID: sql.NullString{String: "img2", Valid: true}, RouteType: "entry", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	handler.Hub().Pages.InvalidateAll()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/img2", nil))
	body := rec.Body.String()
	bodyContains(t, body, "/media/"+featID+"/social")
	bodyContains(t, body, `<meta property="og:image"`)
}

func TestSocialGlobalFallback(t *testing.T) {
	handler, queries := setupSite(t)
	ctx := context.Background()
	globalID := uploadTestImage(t, handler, nil, 1300, 700)
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) {
		p.SiteUrl = "https://example.com"
		p.SiteSocialMediaID = sql.NullString{String: globalID, Valid: true}
	})
	// Entry with no images
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: "img3", ContentTypeID: "page", Slug: "img3", Status: "active", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "img3-r1", EntryID: "img3", RevisionNumber: 1, Title: "NoImg", DocumentJson: testDoc, CreatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: "img3-r1", Valid: true}, PublishedAt: sql.NullInt64{Int64: 1, Valid: true}, UpdatedAt: 1, ID: "img3"}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateRoute(ctx, db.CreateRouteParams{ID: "img3-route", Path: "/img3", EntryID: sql.NullString{String: "img3", Valid: true}, RouteType: "entry", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	handler.Hub().Pages.InvalidateAll()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/img3", nil))
	body := rec.Body.String()
	bodyContains(t, body, "/media/"+globalID+"/social")
	bodyContains(t, body, `<meta property="og:image"`)
	bodyContains(t, body, `<meta name="twitter:image"`)
}

func TestSocialAbsoluteURLs(t *testing.T) {
	handler, queries := setupSite(t)
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) { p.SiteUrl = "https://example.com" })
	ctx := context.Background()
	socialID := uploadTestImage(t, handler, nil, 1300, 700)
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: "abs1", ContentTypeID: "page", Slug: "abs1", Status: "active", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "abs1-r1", EntryID: "abs1", RevisionNumber: 1, Title: "Abs", DocumentJson: testDoc, CreatedAt: 1, SocialMediaID: sql.NullString{String: socialID, Valid: true}}); err != nil {
		t.Fatal(err)
	}
	if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: "abs1-r1", Valid: true}, PublishedAt: sql.NullInt64{Int64: 1, Valid: true}, UpdatedAt: 1, ID: "abs1"}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateRoute(ctx, db.CreateRouteParams{ID: "abs1-route", Path: "/abs1", EntryID: sql.NullString{String: "abs1", Valid: true}, RouteType: "entry", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	handler.Hub().Pages.InvalidateAll()
	req := httptest.NewRequest(http.MethodGet, "/abs1", nil)
	req.Host = "example.com"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	body := rec.Body.String()
	// og:url must be absolute
	bodyContains(t, body, `<meta property="og:url" content="https://example.com/abs1">`)
	// og:image must be absolute with scheme
	if !strings.Contains(body, `og:image" content="https://example.com/media/`+socialID+`/social`) {
		t.Fatalf("og:image should be absolute, got:\n%s", body)
	}
	bodyContains(t, body, `<meta name="twitter:image" content="https://example.com/media/`)

	// When site URL empty, origin fallback should make URLs absolute with request host
	handler2, queries2 := setupSite(t)
	// Do not set site URL, leave empty; request origin will be http://fallback.test
	setSettingsWithHandler(t, handler2, queries2, func(p *db.UpdateSiteSettingsParams) { p.SiteUrl = "" })
	ctx2 := context.Background()
	socialID2 := func() string {
		asset, err := handler2.Hub().Media.Upload(ctx2, "fallback.png", "", bytes.NewReader(testPNGBytes(t, 1300, 700)))
		if err != nil {
			t.Fatalf("upload: %v", err)
		}
		return asset.ID
	}()
	if err := queries2.CreateEntry(ctx2, db.CreateEntryParams{ID: "abs2", ContentTypeID: "page", Slug: "abs2", Status: "active", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := queries2.CreateEntryRevision(ctx2, db.CreateEntryRevisionParams{ID: "abs2-r1", EntryID: "abs2", RevisionNumber: 1, Title: "Abs2", DocumentJson: testDoc, CreatedAt: 1, SocialMediaID: sql.NullString{String: socialID2, Valid: true}}); err != nil {
		t.Fatal(err)
	}
	if err := queries2.SetPublishedRevision(ctx2, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: "abs2-r1", Valid: true}, PublishedAt: sql.NullInt64{Int64: 1, Valid: true}, UpdatedAt: 1, ID: "abs2"}); err != nil {
		t.Fatal(err)
	}
	if err := queries2.CreateRoute(ctx2, db.CreateRouteParams{ID: "abs2-route", Path: "/abs2", EntryID: sql.NullString{String: "abs2", Valid: true}, RouteType: "entry", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	handler2.Hub().Pages.InvalidateAll()
	req2 := httptest.NewRequest(http.MethodGet, "/abs2", nil)
	req2.Host = "fallback.test"
	rec2 := httptest.NewRecorder()
	handler2.ServeHTTP(rec2, req2)
	body2 := rec2.Body.String()
	// Should contain http://fallback.test as host
	if !strings.Contains(body2, `content="http://fallback.test/abs2"`) && !strings.Contains(body2, `content="https://fallback.test/abs2"`) {
		t.Fatalf("og:url should use origin when site_url empty, got:\n%s", body2)
	}
}

func TestSocialDraftDoesNotLeak(t *testing.T) {
	handler, queries := setupSite(t)
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) { p.SiteUrl = "https://example.com" })
	ctx := context.Background()
	socialPub := uploadTestImage(t, handler, nil, 1300, 700)
	socialDraft := uploadTestImage(t, handler, nil, 1300, 700)
	// Create entry with published revision containing socialPub
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: "draft1", ContentTypeID: "page", Slug: "draft1", Status: "active", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "draft1-r1", EntryID: "draft1", RevisionNumber: 1, Title: "DraftTest", DocumentJson: testDoc, CreatedAt: 1, SocialMediaID: sql.NullString{String: socialPub, Valid: true}}); err != nil {
		t.Fatal(err)
	}
	if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: "draft1-r1", Valid: true}, PublishedAt: sql.NullInt64{Int64: 1, Valid: true}, UpdatedAt: 1, ID: "draft1"}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateRoute(ctx, db.CreateRouteParams{ID: "draft1-route", Path: "/draft1", EntryID: sql.NullString{String: "draft1", Valid: true}, RouteType: "entry", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	handler.Hub().Pages.InvalidateAll()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/draft1", nil))
	body := rec.Body.String()
	bodyContains(t, body, "/media/"+socialPub+"/social")
	// Add draft revision with different social image (not published)
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "draft1-r2", EntryID: "draft1", RevisionNumber: 2, Title: "DraftTest", DocumentJson: testDoc, CreatedAt: 2, SocialMediaID: sql.NullString{String: socialDraft, Valid: true}}); err != nil {
		t.Fatal(err)
	}
	handler.Hub().Pages.InvalidateAll()
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/draft1", nil))
	body = rec.Body.String()
	// Public should still show pub image, not draft
	bodyContains(t, body, "/media/"+socialPub+"/social")
	if strings.Contains(body, "/media/"+socialDraft+"/social") {
		t.Fatalf("draft social image leaked to public page:\n%s", body)
	}
	// Publish draft and verify switch
	if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: "draft1-r2", Valid: true}, PublishedAt: sql.NullInt64{Int64: 2, Valid: true}, UpdatedAt: 2, ID: "draft1"}); err != nil {
		t.Fatal(err)
	}
	handler.Hub().Pages.InvalidateAll()
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/draft1", nil))
	body = rec.Body.String()
	bodyContains(t, body, "/media/"+socialDraft+"/social")
}

func TestSocialTwitterSharesOG(t *testing.T) {
	handler, queries := setupSite(t)
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) {
		p.SiteUrl = "https://example.com"
		p.TwitterSite = "@stratum"
	})
	ctx := context.Background()
	socialID := uploadTestImage(t, handler, nil, 1300, 700)
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: "tw1", ContentTypeID: "page", Slug: "tw1", Status: "active", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "tw1-r1", EntryID: "tw1", RevisionNumber: 1, Title: "TwTitle", Excerpt: sql.NullString{String: "TwDesc", Valid: true}, DocumentJson: testDoc, CreatedAt: 1, SocialMediaID: sql.NullString{String: socialID, Valid: true}}); err != nil {
		t.Fatal(err)
	}
	if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: "tw1-r1", Valid: true}, PublishedAt: sql.NullInt64{Int64: 1, Valid: true}, UpdatedAt: 1, ID: "tw1"}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateRoute(ctx, db.CreateRouteParams{ID: "tw1-route", Path: "/tw1", EntryID: sql.NullString{String: "tw1", Valid: true}, RouteType: "entry", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	handler.Hub().Pages.InvalidateAll()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/tw1", nil))
	body := rec.Body.String()
	bodyContains(t, body, `<meta property="og:title" content="TwTitle">`)
	bodyContains(t, body, `<meta name="twitter:title" content="TwTitle">`)
	bodyContains(t, body, `<meta name="twitter:card" content="summary_large_image">`)
	bodyContains(t, body, `<meta name="twitter:site" content="@stratum">`)
	// Twitter image same as OG
	if !strings.Contains(body, `twitter:image" content="https://example.com/media/`+socialID+`/social`) {
		t.Fatalf("twitter image should match OG, got:\n%s", body)
	}
}

func TestSocialImageVariantExists(t *testing.T) {
	handler, _ := setupSite(t)
	ctx := context.Background()
	id := uploadTestImage(t, handler, nil, 1500, 900)
	// Check variant rows
	mediaService := handler.Hub().Media
	if _, _, err := mediaService.ReadVariant(ctx, id, "social"); err != nil {
		t.Fatalf("social variant should exist after upload, err %v", err)
	}
	view, ok := mediaService.SocialView(ctx, id)
	if !ok {
		t.Fatal("SocialView should exist")
	}
	if view.Width != 1200 || view.Height != 630 {
		t.Fatalf("social view dims = %dx%d want 1200x630", view.Width, view.Height)
	}
	if !strings.HasPrefix(view.URL, "/media/"+id+"/social?v=") {
		t.Fatalf("social URL = %q, want content-hashed /media/<id>/social?v=<hash>", view.URL)
	}
}

// Ensure media import used
var _ = media.FocalPoint{}
