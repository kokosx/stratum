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
	"path/filepath"
	"testing"

	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/media"
	"github.com/kokosx/stratum/internal/rendering"
	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/themes"
)

func benchmarkHandler(b *testing.B) *Handler {
	b.Helper()
	ctx := context.Background()
	database, _ := storage.Open(filepath.Join(b.TempDir(), "bench.db"))
	_ = database.Migrate(ctx)
	_ = database.Seed(ctx)
	queries := db.New(database.DB)
	store, _ := media.NewLocalStorage(filepath.Join(b.TempDir(), "media"))
	mediaSvc := media.NewService(queries, store)
	registry, _ := blocks.NewRegistry(ctx, queries, mediaSvc)
	themeRun, _ := themes.NewRuntime(ctx, queries)
	h, _ := NewHandler(queries, registry, themeRun, mediaSvc)
	img := image.NewRGBA(image.Rect(0, 0, 20, 20))
	for x := 0; x < 20; x++ {
		for y := 0; y < 20; y++ {
			img.Set(x, y, color.RGBA{R: 100, G: 150, B: 200, A: 255})
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	for i := 0; i < 10; i++ {
		asset, _ := mediaSvc.Upload(ctx, "test.png", "", bytes.NewReader(buf.Bytes()))
		featID := ""
		if asset != nil {
			featID = asset.ID
		}
		id := "bench-post-" + string(rune('0'+i))
		slug := "bench-post-" + string(rune('0'+i))
		path := "/blog/" + slug
		_ = queries.CreateEntry(ctx, db.CreateEntryParams{ID: id, ContentTypeID: "post", Slug: slug, Status: "active", CreatedAt: int64(i), UpdatedAt: int64(i)})
		revID := id + "-r1"
		_ = queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: revID, EntryID: id, RevisionNumber: 1, Title: "Post " + string(rune('0'+i)), DocumentJson: `{"version":1,"nodes":[]}`, FeaturedMediaID: sql.NullString{String: featID, Valid: featID != ""}, CreatedAt: int64(i)})
		_ = queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: revID, Valid: true}, PublishedAt: sql.NullInt64{Int64: int64(i), Valid: true}, UpdatedAt: int64(i), ID: id})
		_ = queries.CreateRoute(ctx, db.CreateRouteParams{ID: id + "-route", Path: path, EntryID: sql.NullString{String: id, Valid: true}, RouteType: "entry", CreatedAt: int64(i), UpdatedAt: int64(i)})
	}
	_ = h.Hub().Routes.Reload(ctx)
	req := httptest.NewRequest(http.MethodGet, "/blog", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return h
}

func BenchmarkPublicPageCacheHit(b *testing.B) {
	b.ReportAllocs()
	h := benchmarkHandler(b)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
		if rec.Code != 200 {
			b.Fatalf("code %d", rec.Code)
		}
	}
}

func BenchmarkPublic404(b *testing.B) {
	b.ReportAllocs()
	h := benchmarkHandler(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/not-found", nil))
		if rec.Code != 404 {
			b.Fatalf("code %d", rec.Code)
		}
	}
}

func BenchmarkRedirectLookupWarm(b *testing.B) {
	b.ReportAllocs()
	h := benchmarkHandler(b)
	ctx := context.Background()
	_ = h.Hub().Queries.CreateRoute(ctx, db.CreateRouteParams{ID: "redir", Path: "/old", RouteType: "redirect", RedirectTo: sql.NullString{String: "/new", Valid: true}, RedirectStatus: sql.NullInt64{Int64: 301, Valid: true}, CreatedAt: 1, UpdatedAt: 1})
	_ = h.Hub().Routes.Reload(ctx)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/old", nil))
		if rec.Code != 301 {
			b.Fatalf("code %d", rec.Code)
		}
	}
}

func BenchmarkBlocksCSSForWarm(b *testing.B) {
	b.ReportAllocs()
	h := benchmarkHandler(b)
	keys := []rendering.BlockKey{{Name: "core/heading", Version: 1}, {Name: "core/text", Version: 1}, {Name: "core/button", Version: 1}}
	// Warm once
	_ = h.Hub().Assets.BlocksCSSFor(keys)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = h.Hub().Assets.BlocksCSSFor(keys)
	}
}

func BenchmarkArchiveColdRender10(b *testing.B) {
	b.ReportAllocs()
	h := benchmarkHandler(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		h.Hub().Pages.InvalidateAll()
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/blog", nil))
		if rec.Code != 200 {
			b.Fatalf("code %d", rec.Code)
		}
	}
}
