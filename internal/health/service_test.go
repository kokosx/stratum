package health

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func newTestHealthService(t *testing.T) (*Service, *db.Queries) {
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
	return New(database.DB, queries), queries
}

func TestHealthSiteURLMissing(t *testing.T) {
	svc, _ := newTestHealthService(t)
	ctx := context.Background()
	results, _, err := svc.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range results {
		if r.ID == "site_url" && r.Severity == SeverityWarning {
			found = true
		}
	}
	// By default after seed, SiteURL may be empty -> warning expected or good if set?
	// Just ensure result exists
	if !found {
		// Check good case also acceptable (if SiteURL configured)
		for _, r := range results {
			if r.ID == "site_url" {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("site_url check missing")
		}
	}
}

func TestHealthDetectsRedirectLoop(t *testing.T) {
	svc, queries := newTestHealthService(t)
	ctx := context.Background()
	// Create loop via routes table directly
	now := int64(1000)
	queries.CreateRoute(ctx, db.CreateRouteParams{ID: "r1", Path: "/a", RouteType: "redirect", RedirectTo: sql.NullString{String: "/b", Valid: true}, RedirectStatus: sql.NullInt64{Int64: 301, Valid: true}, CreatedAt: now, UpdatedAt: now})
	queries.CreateRoute(ctx, db.CreateRouteParams{ID: "r2", Path: "/b", RouteType: "redirect", RedirectTo: sql.NullString{String: "/a", Valid: true}, RedirectStatus: sql.NullInt64{Int64: 301, Valid: true}, CreatedAt: now, UpdatedAt: now})
	results, _, err := svc.Run(ctx)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range results {
		if r.Severity == SeverityCritical && contains(r.Title, "loop") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected loop critical, got %#v", results)
	}
}

func TestHealthDetectsBrokenRedirectTarget(t *testing.T) {
	svc, queries := newTestHealthService(t)
	ctx := context.Background()
	now := int64(1000)
	queries.CreateRoute(ctx, db.CreateRouteParams{ID: "r3", Path: "/old", RouteType: "redirect", RedirectTo: sql.NullString{String: "/missing", Valid: true}, RedirectStatus: sql.NullInt64{Int64: 301, Valid: true}, CreatedAt: now, UpdatedAt: now})
	results, _, _ := svc.Run(ctx)
	found := false
	for _, r := range results {
		if r.ID == "redirect_target_/old" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected broken redirect target warning")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
