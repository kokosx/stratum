package admin

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
	publicweb "github.com/kokosx/stratum/internal/web/public"
)

// newSlugTestHarness builds an admin handler (for entry writes) and a public
// handler (to verify how redirects are actually served) over one seeded database.
func newSlugTestHarness(t *testing.T) (*Handler, *db.Queries, *publicweb.Handler) {
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
	registry, err := blocks.NewRegistry(ctx, queries)
	if err != nil {
		t.Fatal(err)
	}
	themeRuntime, err := themes.NewRuntime(ctx, queries)
	if err != nil {
		t.Fatal(err)
	}
	adminHandler, err := NewHandler(database.DB, queries, nil, registry, themeRuntime, newTestMedia(t, queries))
	if err != nil {
		t.Fatal(err)
	}
	publicHandler, err := publicweb.NewHandler(queries, registry, themeRuntime, newTestMedia(t, queries))
	if err != nil {
		t.Fatal(err)
	}
	return adminHandler, queries, publicHandler
}

func publishEntryAt(t *testing.T, h *Handler, entryID, slug string, create bool) {
	t.Helper()
	const doc = `{"version":1,"nodes":[]}`
	err := h.writeEntry(context.Background(), "page", "author", entryID,
		entryInput{title: "Entry", slug: slug, documentJSON: doc},
		create, true)
	if err != nil {
		t.Fatalf("publish %s: %v", slug, err)
	}
}

func assertPublicRedirect(t *testing.T, publicHandler *publicweb.Handler, path, wantLocation string) {
	t.Helper()
	rec := httptest.NewRecorder()
	publicHandler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
	if rec.Code != http.StatusMovedPermanently {
		t.Fatalf("GET %s = %d, want 301", path, rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != wantLocation {
		t.Fatalf("GET %s Location = %q, want %q", path, loc, wantLocation)
	}
}

// TestSlugChangeCreatesRedirect verifies the URL-history rule: republishing an
// entry under a new slug must leave a 301 redirect from the old URL to the new
// one, so no external link ever breaks.
func TestSlugChangeCreatesRedirect(t *testing.T) {
	h, queries, publicHandler := newSlugTestHarness(t)
	ctx := context.Background()

	publishEntryAt(t, h, "slug-entry", "first-slug", true)
	publishEntryAt(t, h, "slug-entry", "second-slug", false)

	oldRoute, err := queries.GetRouteByPath(ctx, "/first-slug")
	if err != nil {
		t.Fatal(err)
	}
	if oldRoute.RouteType != "redirect" || !oldRoute.RedirectTo.Valid || oldRoute.RedirectTo.String != "/second-slug" {
		t.Fatalf("/first-slug = %#v, want redirect to /second-slug", oldRoute)
	}
	if !oldRoute.RedirectStatus.Valid || oldRoute.RedirectStatus.Int64 != http.StatusMovedPermanently {
		t.Fatalf("redirect status = %#v, want 301", oldRoute.RedirectStatus)
	}
	entryRoute, err := queries.GetEntryRoute(ctx, sql.NullString{String: "slug-entry", Valid: true})
	if err != nil {
		t.Fatal(err)
	}
	if entryRoute.Path != "/second-slug" || entryRoute.RouteType != "entry" {
		t.Fatalf("entry route moved to %#v, want /second-slug entry route", entryRoute)
	}

	assertPublicRedirect(t, publicHandler, "/first-slug", "/second-slug")
	rec := httptest.NewRecorder()
	publicHandler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/second-slug", nil))
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /second-slug = %d, body = %s", rec.Code, rec.Body.String())
	}
}

// TestSlugChangesDoNotCreateRedirectChains is the chain-flattening guarantee:
// after A → B → C the routes must hold A → C and B → C, never A → B → C. The
// public handler must answer /a with a single hop straight to /c.
func TestSlugChangesDoNotCreateRedirectChains(t *testing.T) {
	h, queries, publicHandler := newSlugTestHarness(t)
	ctx := context.Background()

	publishEntryAt(t, h, "chain-entry", "a", true)
	publishEntryAt(t, h, "chain-entry", "b", false)
	publishEntryAt(t, h, "chain-entry", "c", false)

	for _, tc := range []struct{ source, target string }{
		{"/a", "/c"},
		{"/b", "/c"},
	} {
		route, err := queries.GetRouteByPath(ctx, tc.source)
		if err != nil {
			t.Fatal(err)
		}
		if route.RouteType != "redirect" || !route.RedirectTo.Valid || route.RedirectTo.String != tc.target {
			t.Fatalf("%s = %#v, want redirect straight to %s", tc.source, route, tc.target)
		}
		assertPublicRedirect(t, publicHandler, tc.source, tc.target)
	}
	// No redirect may point at another redirect: every target is the live entry route.
	target, err := queries.GetRouteByPath(ctx, "/c")
	if err != nil {
		t.Fatal(err)
	}
	if target.RouteType != "entry" {
		t.Fatalf("/c = %#v, want the live entry route", target)
	}
}
