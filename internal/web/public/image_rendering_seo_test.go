package public

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

// seedDocEntry publishes an entry at path with the given document JSON, using
// the real migrated block definitions end to end.
func seedDocEntry(t *testing.T, handler *Handler, queries *db.Queries, id, path, title, doc string) {
	t.Helper()
	ctx := context.Background()
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: id, ContentTypeID: "page", Slug: strings.TrimPrefix(path, "/"), Status: "active", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{
		ID: id + "-r1", EntryID: id, RevisionNumber: 1, Title: title, DocumentJson: doc, CreatedAt: 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{
		PublishedRevisionID: sql.NullString{String: id + "-r1", Valid: true},
		PublishedAt:         sql.NullInt64{Int64: 1, Valid: true},
		UpdatedAt:           1,
		ID:                  id,
	}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateRoute(ctx, db.CreateRouteParams{ID: id + "-route", Path: path, EntryID: sql.NullString{String: id, Valid: true}, RouteType: "entry", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	handler.Hub().Pages.InvalidateAll()
}

func renderPath(t *testing.T, handler *Handler, path string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s = %d:\n%s", path, rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

// imageDoc builds a document with one core/image node.
func imageDoc(nodeID, mediaID, alt string, decorative bool) string {
	doc := fmt.Sprintf(`{"version":1,"nodes":[{"id":%q,"block":"core/image","version":1,"props":{"mediaId":%q,"alt":%q},"settings":{"align":"none","decorative":%t}}]}`,
		nodeID, mediaID, alt, decorative)
	return doc
}

func TestImageBlockRenderingSEOAttributes(t *testing.T) {
	handler, queries := setupSite(t)
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) { p.SiteUrl = "https://example.com" })
	mediaID := uploadTestImage(t, handler, nil, 900, 600)

	t.Run("explicit alt override and dimensions", func(t *testing.T) {
		// priority "normal" opts this image out of automatic LCP selection.
		doc := strings.Replace(imageDoc("n1", mediaID, "Override alt", false),
			`"decorative":false`, `"decorative":false,"priority":"normal"`, 1)
		seedDocEntry(t, handler, queries, "seoimg1", "/seoimg1", "Img", doc)
		body := renderPath(t, handler, "/seoimg1")
		bodyContains(t, body, `alt="Override alt"`)
		// Intrinsic dimensions from the Media Library reserve layout space.
		bodyContains(t, body, `width="900"`)
		bodyContains(t, body, `height="600"`)
		// Non-LCP images stay lazy with async decoding.
		bodyContains(t, body, `loading="lazy"`)
		bodyContains(t, body, `decoding="async"`)
		if strings.Contains(body, `fetchpriority=`) {
			t.Fatalf("non-priority image must not carry fetchpriority:\n%s", body)
		}
		if strings.Contains(body, `loading=""`) || strings.Contains(body, `fetchpriority=""`) {
			t.Fatalf("empty loading attributes emitted:\n%s", body)
		}
	})

	t.Run("media library alt fallback", func(t *testing.T) {
		if err := handler.Hub().Media.UpdateMetadata(context.Background(), mediaID, "Library alt text", "", "", ""); err != nil {
			t.Fatal(err)
		}
		handler.Hub().Pages.InvalidateAll()
		seedDocEntry(t, handler, queries, "seoimg2", "/seoimg2", "Img", imageDoc("n1", mediaID, "", false))
		body := renderPath(t, handler, "/seoimg2")
		bodyContains(t, body, `alt="Library alt text"`)
	})

	t.Run("decorative renders empty alt attribute", func(t *testing.T) {
		seedDocEntry(t, handler, queries, "seoimg3", "/seoimg3", "Img", imageDoc("n1", mediaID, "Ignored override", true))
		body := renderPath(t, handler, "/seoimg3")
		// The attribute is always present; decorative renders the empty value.
		bodyContains(t, body, `alt=""`)
		if strings.Contains(body, `Ignored override"`) {
			t.Fatalf("decorative image must not leak alt text:\n%s", body)
		}
	})
}

func TestWebPDerivativePictureSourceRendered(t *testing.T) {
	handler, queries := setupSite(t)
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) { p.SiteUrl = "https://example.com" })
	mediaID := uploadTestImage(t, handler, nil, 900, 600)
	seedDocEntry(t, handler, queries, "webpimg", "/webpimg", "Img", imageDoc("n1", mediaID, "Alt", false))

	body := renderPath(t, handler, "/webpimg")
	// <picture> exists only because WebP variants exist for this raster asset.
	bodyContains(t, body, `<picture>`)
	bodyContains(t, body, `<source type="image/webp" srcset="/media/`+mediaID+`/480.webp?v=`)
	if !strings.Contains(body, `/media/`+mediaID+`/768.webp?v=`) {
		t.Fatalf("missing 768.webp candidate in srcset:\n%s", body)
	}
	// The fallback img keeps native candidates and the original is untouched.
	bodyContains(t, body, `srcset="/media/`+mediaID+`/480?v=`)
}

func TestOnlyLCPImageGetsHighPriority(t *testing.T) {
	handler, queries := setupSite(t)
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) { p.SiteUrl = "https://example.com" })
	m1 := uploadTestImage(t, handler, nil, 800, 500)
	m2 := uploadTestImage(t, handler, nil, 800, 500)
	m3 := uploadTestImage(t, handler, nil, 800, 500)

	doc := fmt.Sprintf(`{"version":1,"nodes":[
		{"id":"lcp","block":"core/image","version":1,"props":{"mediaId":%q},"settings":{"priority":"high"}},
		{"id":"rest","block":"core/image","version":1,"props":{"mediaId":%q},"settings":{}},
		{"id":"rest2","block":"core/image","version":1,"props":{"mediaId":%q},"settings":{"eager":true}}
	]}`, m1, m2, m3)
	seedDocEntry(t, handler, queries, "lcpimg", "/lcpimg", "Img", doc)

	body := renderPath(t, handler, "/lcpimg")
	if got := strings.Count(body, `fetchpriority="high"`); got != 1 {
		t.Fatalf("fetchpriority=high count = %d, want exactly 1:\n%s", got, body)
	}
	// The prioritized node is eager; every other image stays lazy even when an
	// author left the legacy eager flag set (it only feeds LCP selection).
	// The loading/fetchpriority attrs sit at the end of the <img> tag, so the
	// owning image is identified by the srcset immediately before them.
	lcpIdx := strings.Index(body, `loading="eager" fetchpriority="high"`)
	if lcpIdx < 0 {
		t.Fatalf("no eager LCP image rendered:\n%s", body)
	}
	head := body[max(0, lcpIdx-800):lcpIdx]
	if !strings.Contains(head, `/media/`+m1+`/480`) && !strings.Contains(head, `src="/media/`+m1+`/`) {
		t.Fatalf("priority attrs not on image %s:\n%s", m1, head)
	}
	for _, m := range []string{m2, m3} {
		img := fmt.Sprintf(`<img src="/media/%s/`, m)
		i := strings.Index(body, img)
		if i < 0 {
			i = strings.Index(body, `srcset="/media/`+m+`/`)
		}
		if i < 0 {
			t.Fatalf("image %s missing from page:\n%s", m, body)
		}
		chunk := body[i:min(i+700, len(body))]
		if !strings.Contains(chunk, `loading="lazy"`) {
			t.Fatalf("non-LCP image %s must stay lazy:\n%s", m, chunk)
		}
	}
}

func TestGalleryImagesGetDimensionsAndLazyLoading(t *testing.T) {
	handler, queries := setupSite(t)
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) { p.SiteUrl = "https://example.com" })
	m1 := uploadTestImage(t, handler, nil, 640, 480)
	m2 := uploadTestImage(t, handler, nil, 640, 480)
	if err := handler.Hub().Media.UpdateMetadata(context.Background(), m2, "Second gallery alt", "", "", ""); err != nil {
		t.Fatal(err)
	}
	handler.Hub().Pages.InvalidateAll()

	doc := fmt.Sprintf(`{"version":1,"nodes":[{"id":"g","block":"core/gallery","version":1,"props":{"images":%q},"settings":{"columns":3}}]}`, m1+","+m2)
	seedDocEntry(t, handler, queries, "gal", "/gal", "Gallery", doc)

	body := renderPath(t, handler, "/gal")
	// Every gallery <img> carries intrinsic dimensions now (the gallery never
	// had width/height before this pass).
	if strings.Count(body, `width="640"`) != 2 || strings.Count(body, `height="480"`) != 2 {
		t.Fatalf("gallery imgs missing width/height:\n%s", body)
	}
	// Galleries are never LCP: lazy + async decoding, no fetchpriority races.
	if strings.Count(body, `loading="lazy" decoding="async"`) != 2 {
		t.Fatalf("gallery imgs missing lazy+async:\n%s", body)
	}
	if strings.Contains(body, `fetchpriority=`) {
		t.Fatalf("gallery must not emit fetchpriority:\n%s", body)
	}
	// Alt falls back to the Media Library value per image.
	bodyContains(t, body, `alt="Second gallery alt"`)
	// WebP sources render per item.
	if strings.Count(body, `<source type="image/webp"`) != 2 {
		t.Fatalf("gallery missing webp sources:\n%s", body)
	}
}

func TestFeaturedImageDimensionsAndVersionedURLs(t *testing.T) {
	handler, queries := setupSite(t)
	setSettingsWithHandler(t, handler, queries, func(p *db.UpdateSiteSettingsParams) { p.SiteUrl = "https://example.com" })
	featID := uploadTestImage(t, handler, nil, 1200, 800)
	ctx := context.Background()

	doc := `{"version":1,"nodes":[{"id":"f","block":"core/featured-image","version":1,"props":{},"settings":{"objectFit":"cover","aspectRatio":"16:9"}}]}`
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: "feat", ContentTypeID: "post", Slug: "feat", Status: "active", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{
		ID: "feat-r1", EntryID: "feat", RevisionNumber: 1, Title: "Feat", DocumentJson: doc, CreatedAt: 1,
		FeaturedMediaID: sql.NullString{String: featID, Valid: true},
	}); err != nil {
		t.Fatal(err)
	}
	if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{
		PublishedRevisionID: sql.NullString{String: "feat-r1", Valid: true},
		PublishedAt:         sql.NullInt64{Int64: 1, Valid: true}, UpdatedAt: 1, ID: "feat",
	}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateRoute(ctx, db.CreateRouteParams{ID: "feat-route", Path: "/feat", EntryID: sql.NullString{String: "feat", Valid: true}, RouteType: "entry", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	handler.Hub().Pages.InvalidateAll()

	body := renderPath(t, handler, "/feat")
	bodyContains(t, body, `width="1200"`)
	bodyContains(t, body, `height="800"`)
	bodyContains(t, body, `<source type="image/webp" srcset="/media/`+featID+`/480.webp?v=`)
	// Featured image alone is not the LCP unless it is a core/image block.
	bodyContains(t, body, `loading="lazy"`)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
