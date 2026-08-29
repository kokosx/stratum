package admin

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/notfound"
	"github.com/kokosx/stratum/internal/runtimehub"
	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/themes"
	publicweb "github.com/kokosx/stratum/internal/web/public"
)

func newEpic5Harness(t *testing.T) (*Handler, *db.Queries, *publicweb.Handler, *notfound.Store) {
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
	mediaService := newTestMedia(t, queries)
	registry, err := blocks.NewRegistry(ctx, queries, mediaService)
	if err != nil {
		t.Fatal(err)
	}
	themeRuntime, err := themes.NewRuntime(ctx, queries)
	if err != nil {
		t.Fatal(err)
	}
	sharedHub, err := runtimehub.New(queries, registry, themeRuntime, mediaService)
	if err != nil {
		t.Fatal(err)
	}
	adminHandler, err := NewHandler(database.DB, queries, nil, registry, themeRuntime, mediaService, sharedHub)
	if err != nil {
		t.Fatal(err)
	}
	publicHandler, err := publicweb.NewHandlerWithHub(sharedHub)
	if err != nil {
		t.Fatal(err)
	}
	store := notfound.New(database.DB)
	return adminHandler, queries, publicHandler, store
}

func TestEpic5_ManualRedirectViaAdmin(t *testing.T) {
	h, queries, publicHandler, _ := newEpic5Harness(t)
	ctx := context.Background()
	now := int64(1000)
	if err := queries.CreateRoute(ctx, db.CreateRouteParams{ID: "man1", Path: "/old-services", RouteType: "redirect", RedirectTo: sql.NullString{String: "/services", Valid: true}, RedirectStatus: sql.NullInt64{Int64: 301, Valid: true}, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if h.runtime != nil {
		_ = h.runtime.Routes.Reload(ctx)
	}
	rec := httptest.NewRecorder()
	publicHandler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/old-services", nil))
	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("expected 301 got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/services" {
		t.Fatalf("location %q want /services", loc)
	}
}

func TestEpic5_URLChangeRedirect(t *testing.T) {
	h, queries, publicHandler, _ := newEpic5Harness(t)
	publishEntryAt(t, h, "epic5-old", "old-page", true)
	rec := httptest.NewRecorder()
	publicHandler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/old-page", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /old-page = %d want 200", rec.Code)
	}
	err := h.writeEntry(context.Background(), "page", "author", "epic5-old", entryInput{title: "E", slug: "new-page", documentJSON: `{"version":1,"nodes":[]}`}, false, false)
	if err != nil {
		t.Fatal(err)
	}
	if h.runtime != nil {
		_ = h.runtime.Routes.Reload(context.Background())
		h.runtime.Pages.InvalidateAll()
	}
	rec2 := httptest.NewRecorder()
	publicHandler.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/old-page", nil))
	if rec2.Code != http.StatusOK {
		t.Fatalf("after draft GET /old-page = %d want 200", rec2.Code)
	}
	rec3 := httptest.NewRecorder()
	publicHandler.ServeHTTP(rec3, httptest.NewRequest(http.MethodGet, "/new-page", nil))
	if rec3.Code != http.StatusNotFound {
		t.Fatalf("GET /new-page before publish = %d want 404", rec3.Code)
	}
	err = h.writeEntry(context.Background(), "page", "author", "epic5-old", entryInput{title: "E", slug: "new-page", documentJSON: `{"version":1,"nodes":[]}`}, false, true)
	if err != nil {
		t.Fatalf("second publish failed: %v", err)
	}
	if h.runtime != nil {
		_ = h.runtime.Routes.Reload(context.Background())
		h.runtime.Pages.InvalidateAll()
	}
	rec4 := httptest.NewRecorder()
	publicHandler.ServeHTTP(rec4, httptest.NewRequest(http.MethodGet, "/old-page", nil))
	if rec4.Code != http.StatusMovedPermanently || rec4.Header().Get("Location") != "/new-page" {
		t.Fatalf("GET /old-page after publish = %d loc %q want 301 /new-page", rec4.Code, rec4.Header().Get("Location"))
	}
	rec5 := httptest.NewRecorder()
	publicHandler.ServeHTTP(rec5, httptest.NewRequest(http.MethodGet, "/new-page", nil))
	if rec5.Code != http.StatusOK {
		t.Fatalf("GET /new-page = %d want 200", rec5.Code)
	}
	entries, err := queries.ListSitemapEntries(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	hasOld, hasNew := false, false
	for _, e := range entries {
		if e.RoutePath == "/old-page" {
			hasOld = true
		}
		if e.RoutePath == "/new-page" {
			hasNew = true
		}
	}
	if hasOld {
		t.Fatalf("sitemap should not contain old /old-page")
	}
	if !hasNew {
		t.Fatalf("sitemap should contain /new-page")
	}
}

func TestEpic5_SecondURLChange(t *testing.T) {
	h, queries, publicHandler, _ := newEpic5Harness(t)
	publishEntryAt(t, h, "epic5-chain", "a", true)
	publishEntryAt(t, h, "epic5-chain", "b", false)
	publishEntryAt(t, h, "epic5-chain", "c", false)
	for _, src := range []string{"/a", "/b"} {
		route, err := queries.GetRouteByPath(context.Background(), src)
		if err != nil || route.RedirectTo.String != "/c" {
			t.Fatalf("%s redirect %v err %v", src, route, err)
		}
		rec := httptest.NewRecorder()
		publicHandler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, src, nil))
		if rec.Code != 301 || rec.Header().Get("Location") != "/c" {
			t.Fatalf("%s not 301 /c", src)
		}
	}
}

func TestEpic5_404Aggregation(t *testing.T) {
	_, _, publicHandler, store := newEpic5Harness(t)
	for i := 0; i < 3; i++ {
		rec := httptest.NewRecorder()
		publicHandler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/does-not-exist", nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("expected 404 got %d", rec.Code)
		}
	}
	ctx := context.Background()
	rec, err := store.Get(ctx, "/does-not-exist")
	if err != nil {
		t.Fatal(err)
	}
	if rec.HitCount != 3 {
		t.Fatalf("hits %d want 3", rec.HitCount)
	}
}

func TestEpic5_RedirectedNot404(t *testing.T) {
	h2, queries2, publicHandler2, store2b := newEpic5Harness(t)
	now := int64(1000)
	if err := queries2.CreateRoute(context.Background(), db.CreateRouteParams{ID: "r-old", Path: "/old", RouteType: "redirect", RedirectTo: sql.NullString{String: "/new", Valid: true}, RedirectStatus: sql.NullInt64{Int64: 301, Valid: true}, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if h2.runtime != nil {
		_ = h2.runtime.Routes.Reload(context.Background())
	}
	rec2 := httptest.NewRecorder()
	publicHandler2.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/old", nil))
	if rec2.Code != 301 {
		t.Fatalf("expected redirect 301 got %d", rec2.Code)
	}
	if rec, err := store2b.Get(context.Background(), "/old"); err == nil {
		t.Fatalf("redirected path should not be counted as 404, got %#v", rec)
	}
}

func TestEpic5_CreateRedirectFrom404(t *testing.T) {
	h2, queries2, publicHandler2, _ := newEpic5Harness(t)
	ctx := context.Background()
	store2 := notfound.New(h2.database)
	if err := store2.Record(ctx, "/oldd"); err != nil {
		t.Fatal(err)
	}
	now := int64(1000)
	if err := queries2.CreateRoute(ctx, db.CreateRouteParams{ID: "r-oldd", Path: "/oldd", RouteType: "redirect", RedirectTo: sql.NullString{String: "/old", Valid: true}, RedirectStatus: sql.NullInt64{Int64: 301, Valid: true}, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if h2.runtime != nil {
		_ = h2.runtime.Routes.Reload(ctx)
	}
	rec := httptest.NewRecorder()
	publicHandler2.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/oldd", nil))
	if rec.Code != 301 {
		t.Fatalf("after redirect creation, /oldd should 301 got %d", rec.Code)
	}
	_ = store2.Delete(ctx, "/oldd")
	if _, err := store2.Get(ctx, "/oldd"); err == nil {
		t.Fatalf("404 should be deleted after redirect creation")
	}
}

func TestEpic5_HierarchicalDescendantRedirect(t *testing.T) {
	h, queries, publicHandler, _ := newEpic5Harness(t)
	ctx := context.Background()
	// Create parent /services and child /services/web
	publishEntryAt(t, h, "parent", "services", true)
	// Child needs parent ID
	childID := "child-web"
	doc := `{"version":1,"nodes":[]}`
	// Create child via writeEntry with parent
	err := h.writeEntry(ctx, "page", "author", childID, entryInput{title: "Web", slug: "web", documentJSON: doc, parentEntryID: "parent"}, true, true)
	if err != nil {
		t.Fatalf("create child: %v", err)
	}
	if h.runtime != nil {
		_ = h.runtime.Routes.Reload(ctx)
		h.runtime.Pages.InvalidateAll()
	}
	// Verify child at /services/web
	rec := httptest.NewRecorder()
	publicHandler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/services/web", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("child initial /services/web = %d want 200", rec.Code)
	}
	// Change parent slug to /offer
	err = h.writeEntry(ctx, "page", "author", "parent", entryInput{title: "Services", slug: "offer", documentJSON: doc}, false, true)
	if err != nil {
		t.Fatalf("publish parent new slug: %v", err)
	}
	if h.runtime != nil {
		_ = h.runtime.Routes.Reload(ctx)
		h.runtime.Pages.InvalidateAll()
	}
	// Verify old parent redirects
	rec2 := httptest.NewRecorder()
	publicHandler.ServeHTTP(rec2, httptest.NewRequest(http.MethodGet, "/services", nil))
	if rec2.Code != 301 || rec2.Header().Get("Location") != "/offer" {
		t.Fatalf("/services should redirect to /offer got %d %q", rec2.Code, rec2.Header().Get("Location"))
	}
	// Verify child now at /offer/web
	rec3 := httptest.NewRecorder()
	publicHandler.ServeHTTP(rec3, httptest.NewRequest(http.MethodGet, "/offer/web", nil))
	if rec3.Code != http.StatusOK {
		t.Fatalf("/offer/web = %d want 200", rec3.Code)
	}
	// Old child should redirect to new child
	rec4 := httptest.NewRecorder()
	publicHandler.ServeHTTP(rec4, httptest.NewRequest(http.MethodGet, "/services/web", nil))
	if rec4.Code != 301 || rec4.Header().Get("Location") != "/offer/web" {
		t.Fatalf("/services/web should redirect to /offer/web got %d %q", rec4.Code, rec4.Header().Get("Location"))
	}
	// Ensure sitemap contains new child not old
	entries, _ := queries.ListSitemapEntries(ctx)
	for _, e := range entries {
		if e.RoutePath == "/services/web" {
			t.Fatalf("sitemap should not contain old child")
		}
	}
	_ = queries
}
