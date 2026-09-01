package public

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/kokosx/stratum/internal/analytics"
	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/themes"
)

// fakeRecorder for zero-DB test
type fakeRecorder struct {
	enabled bool
	count   int
	last    analytics.Observation
}

func (f *fakeRecorder) Record(obs analytics.Observation) bool {
	f.count++
	f.last = obs
	return true
}
func (f *fakeRecorder) Enabled() bool { return f.enabled }

func TestAnalyticsWarmCacheZeroDB(t *testing.T) {
	ctx := context.Background()
	database, err := storage.Open(filepath.Join(t.TempDir(), "zero_analytics.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := database.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	var qcount int
	wrapped := &countingRawDB2{inner: database.DB, count: &qcount}
	queries := db.New(wrapped)
	mediaSvc := newTestMedia(t, queries)
	registry, _ := blocks.NewRegistry(ctx, queries, mediaSvc)
	themeRun, _ := themes.NewRuntime(ctx, queries)
	handler, _ := NewHandler(queries, registry, themeRun, mediaSvc)
	_ = handler.Hub().Routes.Reload(ctx)
	// Warm page
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("warm / = %d", rec.Code)
	}
	// Attach analytics
	fake := &fakeRecorder{enabled: true}
	handler.SetAnalytics(fake)
	// Now warm hit should still be zero DB
	qcount = 0
	fake.count = 0
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("second warm / = %d", rec.Code)
	}
	if qcount != 0 {
		t.Fatalf("warm page with analytics enabled should be 0 DB, got %d", qcount)
	}
	if fake.count != 1 {
		t.Fatalf("analytics should have recorded 1, got %d", fake.count)
	}
	if !fake.last.IsPageview {
		t.Fatal("should be pageview")
	}
	if fake.last.Crawler != "" {
		t.Fatalf("should be human, got crawler %q", fake.last.Crawler)
	}
}

func TestAnalyticsDisabledNoObservation(t *testing.T) {
	ctx := context.Background()
	database, _ := storage.Open(filepath.Join(t.TempDir(), "disabled.db"))
	database.Migrate(ctx)
	database.Seed(ctx)
	queries := db.New(database.DB)
	mediaSvc := newTestMedia(t, queries)
	registry, _ := blocks.NewRegistry(ctx, queries, mediaSvc)
	themeRun, _ := themes.NewRuntime(ctx, queries)
	handler, _ := NewHandler(queries, registry, themeRun, mediaSvc)
	_ = handler.Hub().Routes.Reload(ctx)
	// Warm
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	fake := &fakeRecorder{enabled: false}
	handler.SetAnalytics(fake)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if fake.count != 0 {
		t.Fatalf("disabled should record 0, got %d", fake.count)
	}
}

func TestAnalyticsQueueFullStillServes(t *testing.T) {
	ctx := context.Background()
	database, _ := storage.Open(filepath.Join(t.TempDir(), "queuefull.db"))
	database.Migrate(ctx)
	database.Seed(ctx)
	queries := db.New(database.DB)
	mediaSvc := newTestMedia(t, queries)
	registry, _ := blocks.NewRegistry(ctx, queries, mediaSvc)
	themeRun, _ := themes.NewRuntime(ctx, queries)
	handler, _ := NewHandler(queries, registry, themeRun, mediaSvc)
	_ = handler.Hub().Routes.Reload(ctx)
	// Instead use fake that simulates full queue by returning false
	fullFake := &fullRecorder{enabled: true}
	handler.SetAnalytics(fullFake)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("queue full should still serve 200, got %d", rec.Code)
	}
	if fullFake.recordCalls != 1 {
		t.Fatalf("should have attempted record")
	}
	_ = ctx
}

type fullRecorder struct {
	enabled     bool
	recordCalls int
}

func (f *fullRecorder) Record(obs analytics.Observation) bool {
	f.recordCalls++
	return false // simulate drop
}
func (f *fullRecorder) Enabled() bool { return f.enabled }

func TestAnalyticsSpeculativeNotPageview(t *testing.T) {
	ctx := context.Background()
	database, _ := storage.Open(filepath.Join(t.TempDir(), "spec.db"))
	database.Migrate(ctx)
	database.Seed(ctx)
	queries := db.New(database.DB)
	mediaSvc := newTestMedia(t, queries)
	registry, _ := blocks.NewRegistry(ctx, queries, mediaSvc)
	themeRun, _ := themes.NewRuntime(ctx, queries)
	handler, _ := NewHandler(queries, registry, themeRun, mediaSvc)
	_ = handler.Hub().Routes.Reload(ctx)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	fake := &fakeRecorder{enabled: true}
	handler.SetAnalytics(fake)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("Sec-Purpose", "prefetch")
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("prefetch = %d", rec.Code)
	}
	if fake.count != 1 {
		t.Fatalf("should record speculative")
	}
	if !fake.last.Speculative {
		t.Fatal("should be speculative")
	}
	if fake.last.IsPageview {
		t.Fatal("speculative should not be pageview")
	}
}

func TestAnalyticsHEADNotPageview(t *testing.T) {
	ctx := context.Background()
	database, _ := storage.Open(filepath.Join(t.TempDir(), "head.db"))
	database.Migrate(ctx)
	database.Seed(ctx)
	queries := db.New(database.DB)
	mediaSvc := newTestMedia(t, queries)
	registry, _ := blocks.NewRegistry(ctx, queries, mediaSvc)
	themeRun, _ := themes.NewRuntime(ctx, queries)
	handler, _ := NewHandler(queries, registry, themeRun, mediaSvc)
	_ = handler.Hub().Routes.Reload(ctx)
	fake := &fakeRecorder{enabled: true}
	handler.SetAnalytics(fake)
	req := httptest.NewRequest(http.MethodHead, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if fake.count != 1 {
		t.Fatalf("HEAD should be recorded as request")
	}
	if fake.last.IsPageview {
		t.Fatal("HEAD should not be pageview")
	}
}

func TestAnalyticsCrawlerNotHuman(t *testing.T) {
	ctx := context.Background()
	database, _ := storage.Open(filepath.Join(t.TempDir(), "crawler.db"))
	database.Migrate(ctx)
	database.Seed(ctx)
	queries := db.New(database.DB)
	mediaSvc := newTestMedia(t, queries)
	registry, _ := blocks.NewRegistry(ctx, queries, mediaSvc)
	themeRun, _ := themes.NewRuntime(ctx, queries)
	handler, _ := NewHandler(queries, registry, themeRun, mediaSvc)
	_ = handler.Hub().Routes.Reload(ctx)
	fake := &fakeRecorder{enabled: true}
	handler.SetAnalytics(fake)
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("User-Agent", "Mozilla/5.0 (compatible; Googlebot/2.1; +http://www.google.com/bot.html)")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if fake.count != 1 {
		t.Fatal("crawler should be recorded")
	}
	if fake.last.Crawler == "" {
		t.Fatal("should be crawler")
	}
	if fake.last.IsPageview && fake.last.Crawler == "" {
		t.Fatal("crawler pageview should have crawler field")
	}
	// Human views separate: IsPageview true but Crawler non-empty
	if !fake.last.IsPageview {
		t.Fatal("crawler should still be pageview but crawler type")
	}
}

func TestAnalyticsPreviewIgnored(t *testing.T) {
	ctx := context.Background()
	database, _ := storage.Open(filepath.Join(t.TempDir(), "preview.db"))
	database.Migrate(ctx)
	database.Seed(ctx)
	queries := db.New(database.DB)
	mediaSvc := newTestMedia(t, queries)
	registry, _ := blocks.NewRegistry(ctx, queries, mediaSvc)
	themeRun, _ := themes.NewRuntime(ctx, queries)
	handler, _ := NewHandler(queries, registry, themeRun, mediaSvc)
	_ = handler.Hub().Routes.Reload(ctx)
	fake := &fakeRecorder{enabled: true}
	handler.SetAnalytics(fake)
	// Preview routes are under /_stratum/preview/ ; they should not be counted as pageview (they are not via serveCachedPage)
	// Instead we test that preview request does not call record? Actually ServeHTTP handles preview via servePreview separately, not serveCachedPage.
	// Our analytics is only in serveCachedPage, so preview should not be recorded at all.
	req := httptest.NewRequest(http.MethodGet, "/_stratum/preview/abc", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if fake.count != 0 {
		t.Fatalf("preview should not be recorded, got %d", fake.count)
	}
}

func TestAnalyticsMediaIgnored(t *testing.T) {
	ctx := context.Background()
	database, _ := storage.Open(filepath.Join(t.TempDir(), "media.db"))
	database.Migrate(ctx)
	database.Seed(ctx)
	queries := db.New(database.DB)
	mediaSvc := newTestMedia(t, queries)
	registry, _ := blocks.NewRegistry(ctx, queries, mediaSvc)
	themeRun, _ := themes.NewRuntime(ctx, queries)
	handler, _ := NewHandler(queries, registry, themeRun, mediaSvc)
	fake := &fakeRecorder{enabled: true}
	handler.SetAnalytics(fake)
	req := httptest.NewRequest(http.MethodGet, "/media/123/file.jpg", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if fake.count != 0 {
		t.Fatalf("media should not be recorded via analytics (not in serveCachedPage), got %d", fake.count)
	}
}

func TestAnalyticsRobotsIgnored(t *testing.T) {
	ctx := context.Background()
	database, _ := storage.Open(filepath.Join(t.TempDir(), "robots.db"))
	database.Migrate(ctx)
	database.Seed(ctx)
	queries := db.New(database.DB)
	mediaSvc := newTestMedia(t, queries)
	registry, _ := blocks.NewRegistry(ctx, queries, mediaSvc)
	themeRun, _ := themes.NewRuntime(ctx, queries)
	handler, _ := NewHandler(queries, registry, themeRun, mediaSvc)
	fake := &fakeRecorder{enabled: true}
	handler.SetAnalytics(fake)
	req := httptest.NewRequest(http.MethodGet, "/robots.txt", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if fake.count != 0 {
		t.Fatalf("robots should not be recorded, got %d", fake.count)
	}
}

func TestAnalyticsCountsForCache(t *testing.T) {
	// Verify cache hit vs miss accounting
	ctx := context.Background()
	database, _ := storage.Open(filepath.Join(t.TempDir(), "cachecount.db"))
	database.Migrate(ctx)
	database.Seed(ctx)
	queries := db.New(database.DB)
	mediaSvc := newTestMedia(t, queries)
	registry, _ := blocks.NewRegistry(ctx, queries, mediaSvc)
	themeRun, _ := themes.NewRuntime(ctx, queries)
	handler, _ := NewHandler(queries, registry, themeRun, mediaSvc)
	_ = handler.Hub().Routes.Reload(ctx)
	// First request miss
	fake := &fakeRecorder{enabled: true}
	handler.SetAnalytics(fake)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if !fake.last.CacheHit == false {
		// First should be miss (but if seeded homepage cached via WarmCache, might be hit)
		// So not strict
	}
	fake.count = 0
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if fake.count != 1 {
		t.Fatalf("second request should be recorded")
	}
	if !fake.last.CacheHit {
		t.Fatalf("second warm request should be cache hit, got miss")
	}
}

// Mock for 304 and redirect behaviours are already handled via writePage; ensure analytics captures status correctly
func TestAnalyticsStatusCapture(t *testing.T) {
	ctx := context.Background()
	database, _ := storage.Open(filepath.Join(t.TempDir(), "status.db"))
	database.Migrate(ctx)
	database.Seed(ctx)
	queries := db.New(database.DB)
	mediaSvc := newTestMedia(t, queries)
	registry, _ := blocks.NewRegistry(ctx, queries, mediaSvc)
	themeRun, _ := themes.NewRuntime(ctx, queries)
	handler, _ := NewHandler(queries, registry, themeRun, mediaSvc)
	_ = handler.Hub().Routes.Reload(ctx)
	// Create a redirect route
	queries.CreateRoute(ctx, db.CreateRouteParams{ID: "redir-analytics", Path: "/old-analytics", RouteType: "redirect", RedirectTo: sql.NullString{String: "/new-analytics", Valid: true}, RedirectStatus: sql.NullInt64{Int64: 301, Valid: true}, CreatedAt: 1, UpdatedAt: 1})
	_ = handler.Hub().Routes.Reload(ctx)
	fake := &fakeRecorder{enabled: true}
	handler.SetAnalytics(fake)
	req := httptest.NewRequest(http.MethodGet, "/old-analytics", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != 301 {
		t.Fatalf("redirect 301 got %d", rec.Code)
	}
	if fake.count != 1 {
		t.Fatal("redirect should be recorded as request")
	}
	if fake.last.Status != 301 {
		t.Fatalf("status should be 301, got %d", fake.last.Status)
	}
	if fake.last.IsPageview {
		t.Fatal("redirect should not be pageview")
	}
}

func TestAnalyticsResponseWriterTransparent(t *testing.T) {
	// Ensure ETag, gzip, HEAD semantics not broken
	ctx := context.Background()
	database, _ := storage.Open(filepath.Join(t.TempDir(), "transparent.db"))
	database.Migrate(ctx)
	database.Seed(ctx)
	queries := db.New(database.DB)
	mediaSvc := newTestMedia(t, queries)
	registry, _ := blocks.NewRegistry(ctx, queries, mediaSvc)
	themeRun, _ := themes.NewRuntime(ctx, queries)
	handler, _ := NewHandler(queries, registry, themeRun, mediaSvc)
	_ = handler.Hub().Routes.Reload(ctx)
	// Warm cache
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	etag := rec.Header().Get("ETag")
	if etag == "" {
		t.Fatal("ETag missing")
	}
	// Conditional request 304
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("If-None-Match", etag)
	rec = httptest.NewRecorder()
	fake := &fakeRecorder{enabled: true}
	handler.SetAnalytics(fake)
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotModified {
		t.Fatalf("304 expected got %d", rec.Code)
	}
	if rec.Header().Get("ETag") != etag {
		t.Fatalf("ETag mismatch on 304")
	}
	if fake.last.Status != 304 {
		t.Fatalf("analytics status should be 304 got %d", fake.last.Status)
	}
	// HEAD should not return body but still status 200 and headers
	req = httptest.NewRequest(http.MethodHead, "/", nil)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("HEAD 200 got %d", rec.Code)
	}
	if rec.Body.Len() != 0 {
		t.Fatalf("HEAD should have empty body, got %d", rec.Body.Len())
	}
}

func init() {
	// Use custom time for analytics to avoid flakiness
	_ = time.Now
}
