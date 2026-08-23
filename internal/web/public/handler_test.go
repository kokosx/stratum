package public

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/media"
	"github.com/kokosx/stratum/internal/navigation"
	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/themes"
)

func newTestMedia(t *testing.T, queries *db.Queries) *media.Service {
	t.Helper()
	store, err := media.NewLocalStorage(filepath.Join(t.TempDir(), "media"))
	if err != nil {
		t.Fatal(err)
	}
	return media.NewService(queries, store)
}

func TestDefaultThemeReceivesPrimaryAndFooterNavigation(t *testing.T) {
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
	service := navigation.NewService(database.DB, queries)
	if err := service.SaveMenu(ctx, "default-main-navigation", "Main Navigation", []navigation.ItemInput{
		{ID: "home-link", Label: "Home menu label", TargetType: "entry", EntryID: "seed-home"},
		{ID: "services", Position: 1, Label: "Services", TargetType: "group"},
		{ID: "consulting", ParentID: "services", Label: "Consulting", TargetType: "url", URL: "/consulting"},
	}, []string{"primary"}); err != nil {
		t.Fatal(err)
	}
	if err := service.SaveMenu(ctx, "default-footer-navigation", "Footer", []navigation.ItemInput{{ID: "privacy-link", Label: "Privacy menu label", TargetType: "url", URL: "/privacy"}}, []string{"footer"}); err != nil {
		t.Fatal(err)
	}
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
	if err := handler.Hub().ReloadNavigation(ctx); err != nil {
		t.Fatal(err)
	}
	if err := handler.themes.Save(ctx, map[string]any{
		"colors.primary": "#123456", "layout.contentWidth": 1234, "header.layout": "split",
	}, ""); err != nil {
		t.Fatal(err)
	}
	if err := handler.Hub().ReloadTheme(ctx); err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", response.Code, response.Body.String())
	}
	body := response.Body.String()
	for _, want := range []string{`site-header--split`, `aria-label="Primary navigation"`, `href="/" aria-current="page">Home menu label</a>`, `aria-label="Toggle Services submenu"`, `href="/consulting">Consulting</a>`, `aria-label="Footer navigation"`, `href="/privacy">Privacy menu label</a>`, `Welcome to StratumCMS`} {
		if !strings.Contains(body, want) {
			t.Errorf("public template missing %q in %s", want, body)
		}
	}
	// Fingerprinted assets: follow legacy redirect if needed.
	_, themeCSS, _ := handler.Hub().Assets.URLs()
	target := themeCSS
	if target == "" {
		target = "/stratum/theme.css"
	}
	stylesheetRequest := httptest.NewRequest(http.MethodGet, target, nil)
	stylesheetResponse := httptest.NewRecorder()
	handler.ServeHTTP(stylesheetResponse, stylesheetRequest)
	// Legacy path redirects to fingerprinted URL.
	if stylesheetResponse.Code == http.StatusFound {
		loc := stylesheetResponse.Header().Get("Location")
		stylesheetRequest = httptest.NewRequest(http.MethodGet, loc, nil)
		stylesheetResponse = httptest.NewRecorder()
		handler.ServeHTTP(stylesheetResponse, stylesheetRequest)
	}
	for _, want := range []string{"--st-color-primary:#123456", "--st-content-width:1234px"} {
		if !strings.Contains(stylesheetResponse.Body.String(), want) {
			t.Errorf("theme stylesheet missing %q in %q", want, stylesheetResponse.Body.String())
		}
	}
}

func TestSitemapUnavailableWithoutSiteURL(t *testing.T) {
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

	request := httptest.NewRequest(http.MethodGet, "/sitemap.xml", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusNotFound {
		t.Fatalf("sitemap without Site URL status = %d, want 404", response.Code)
	}
}
