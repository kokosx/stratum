package public

import (
	"context"
	"database/sql"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/themes"
)

type countingRawDB2 struct {
	inner *sql.DB
	count *int
}

func (c *countingRawDB2) ExecContext(ctx context.Context, query string, args ...interface{}) (sql.Result, error) {
	*c.count++
	return c.inner.ExecContext(ctx, query, args...)
}
func (c *countingRawDB2) PrepareContext(ctx context.Context, query string) (*sql.Stmt, error) {
	*c.count++
	return c.inner.PrepareContext(ctx, query)
}
func (c *countingRawDB2) QueryContext(ctx context.Context, query string, args ...interface{}) (*sql.Rows, error) {
	*c.count++
	return c.inner.QueryContext(ctx, query, args...)
}
func (c *countingRawDB2) QueryRowContext(ctx context.Context, query string, args ...interface{}) *sql.Row {
	*c.count++
	return c.inner.QueryRowContext(ctx, query, args...)
}

func TestRouteSnapshot404ZeroDB(t *testing.T) {
	ctx := context.Background()
	database, err := storage.Open(filepath.Join(t.TempDir(), "zero.db"))
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

	// Warm: ensure snapshot loaded
	_ = handler.Hub().Routes.Reload(ctx)
	// Reset count before 404s
	qcount = 0
	paths := []string{"/wp-login.php", "/.env", "/random-garbage", "/wp-admin", "/xmlrpc.php", "/phpmyadmin"}
	for _, p := range paths {
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, p, nil))
		if rec.Code != http.StatusNotFound {
			t.Fatalf("GET %s = %d, want 404", p, rec.Code)
		}
	}
	if qcount != 0 {
		t.Fatalf("404s with loaded snapshot should be 0 DB queries, got %d", qcount)
	}
	// Also check known hit is 0 DB? Warm page hit should be 0 DB (pagecache hit)
	// First warm the page
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("warm / = %d", rec.Code)
	}
	qcount = 0
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("second warm / = %d", rec.Code)
	}
	if qcount != 0 {
		t.Fatalf("warm page should be 0 DB, got %d", qcount)
	}
	// Redirect also 0
	qcount = 0
	// Create a redirect route and reload
	_ = queries.CreateRoute(ctx, db.CreateRouteParams{ID: "redir-zero", Path: "/old-zero", RouteType: "redirect", RedirectTo: sql.NullString{String: "/new-zero", Valid: true}, RedirectStatus: sql.NullInt64{Int64: 301, Valid: true}, CreatedAt: 1, UpdatedAt: 1})
	_ = handler.Hub().Routes.Reload(ctx)
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/old-zero", nil))
	if rec.Code != 301 {
		t.Fatalf("redirect = %d", rec.Code)
	}
	qcount = 0
	rec = httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/old-zero", nil))
	if rec.Code != 301 {
		t.Fatalf("redirect second = %d", rec.Code)
	}
	if qcount != 0 {
		t.Fatalf("warm redirect should be 0 DB, got %d", qcount)
	}
}

func TestRouteSnapshotMissIs404WithoutDBWhenLoaded(t *testing.T) {
	ctx := context.Background()
	database, err := storage.Open(filepath.Join(t.TempDir(), "zero2.db"))
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
	qcount = 0
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/missing-page-xyz", nil))
	if rec.Code != 404 {
		t.Fatalf("want 404 got %d", rec.Code)
	}
	if qcount != 0 {
		t.Fatalf("snapshot miss should be 0 DB, got %d", qcount)
	}
}
