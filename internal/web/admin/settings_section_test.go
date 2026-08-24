package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/auth"
	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/themes"
)

func newSectionHandler(t *testing.T) (*Handler, *db.Queries) {
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
	registry, _ := blocks.NewRegistry(ctx, queries)
	themeRuntime, _ := themes.NewRuntime(ctx, queries)
	authService, _ := auth.NewService(database.DB, queries, false)
	h, _ := NewHandler(database.DB, queries, authService, registry, themeRuntime, newTestMedia(t, queries))
	return h, queries
}

func TestSettingsRedirect(t *testing.T) {
	h, _ := newSectionHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/admin/settings", nil)
	rec := httptest.NewRecorder()
	h.settingsRedirect(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("GET /admin/settings code %d want 303", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/admin/settings/general" {
		t.Fatalf("Location %q", loc)
	}
}

func TestSettingsGeneralRouteDoesNotFallThroughToAdminRedirect(t *testing.T) {
	h, _ := newSectionHandler(t)
	token, err := h.auth.Setup(context.Background(), h.auth.SetupCode(), "Test Site", "admin@example.com", "correct-horse-battery-staple")
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/admin/settings/general", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("GET /admin/settings/general status %d, location %q", rec.Code, rec.Header().Get("Location"))
	}
	if !strings.Contains(rec.Body.String(), "<h2>General</h2>") {
		t.Fatalf("GET /admin/settings/general did not render General settings")
	}
}

func TestUnknownAdminPathDoesNotPermanentlyRedirectToAdmin(t *testing.T) {
	h, _ := newSectionHandler(t)
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/settings/not-a-section", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("GET unknown admin path status %d, location %q; want 404", rec.Code, rec.Header().Get("Location"))
	}
}

func TestGeneralPOSTModifiesOnlyGeneralFields(t *testing.T) {
	h, queries := newSectionHandler(t)
	ctx := context.Background()
	row, _ := queries.GetSiteSettings(ctx)
	origReading := row.PostsBasePath
	csrfRec := httptest.NewRecorder()
	csrfReq := httptest.NewRequest(http.MethodGet, "/admin/settings/general", nil)
	token, _ := h.csrfToken(csrfRec, csrfReq)
	cookies := csrfRec.Result().Cookies()
	form := url.Values{
		"site_title":      {"General Title"},
		"site_url":        {"https://example.com"},
		"language":        {"pl"},
		"timezone":        {"Europe/Warsaw"},
		"site_represents": {"person"},
		"csrf_token":      {token},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/settings/general", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.saveSettingsGeneral(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST general %d body %s", rec.Code, rec.Body.String())
	}
	newRow, _ := queries.GetSiteSettings(ctx)
	if newRow.SiteTitle != "General Title" {
		t.Fatalf("general not saved")
	}
	if newRow.PostsBasePath != origReading {
		t.Fatalf("reading field mutated by general POST: %q vs %q", newRow.PostsBasePath, origReading)
	}
}

func TestReadingPOSTModifiesOnlyReadingFields(t *testing.T) {
	h, queries := newSectionHandler(t)
	ctx := context.Background()
	row, _ := queries.GetSiteSettings(ctx)
	origTitle := row.SiteTitle
	csrfRec := httptest.NewRecorder()
	csrfReq := httptest.NewRequest(http.MethodGet, "/admin/settings/reading", nil)
	token, _ := h.csrfToken(csrfRec, csrfReq)
	cookies := csrfRec.Result().Cookies()
	form := url.Values{
		"homepage_mode_choice": {"latest"},
		"posts_per_page":       {"7"},
		"csrf_token":           {token},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/settings/reading", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.saveSettingsReading(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST reading %d body %s", rec.Code, rec.Body.String())
	}
	newRow, _ := queries.GetSiteSettings(ctx)
	if newRow.PostsPerPage != 7 {
		t.Fatalf("reading not saved %d", newRow.PostsPerPage)
	}
	if newRow.SiteTitle != origTitle {
		t.Fatalf("general field mutated by reading POST %q vs %q", newRow.SiteTitle, origTitle)
	}
}

func TestSEOPOSTModifiesOnlySEOFields(t *testing.T) {
	h, queries := newSectionHandler(t)
	ctx := context.Background()
	row, _ := queries.GetSiteSettings(ctx)
	origTitle := row.SiteTitle
	csrfRec := httptest.NewRecorder()
	csrfReq := httptest.NewRequest(http.MethodGet, "/admin/settings/seo", nil)
	token, _ := h.csrfToken(csrfRec, csrfReq)
	cookies := csrfRec.Result().Cookies()
	form := url.Values{
		"robots_mode":     {"custom"},
		"robots_custom":   {"User-agent: *\nDisallow: /"},
		"title_separator": {"|"},
		"csrf_token":      {token},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/settings/seo", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.saveSettingsSEO(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST seo %d body %s", rec.Code, rec.Body.String())
	}
	newRow, _ := queries.GetSiteSettings(ctx)
	if newRow.RobotsMode != "custom" {
		t.Fatalf("seo not saved")
	}
	if newRow.SiteTitle != origTitle {
		t.Fatalf("general mutated")
	}
}

func TestPerformancePOSTModifiesOnlyPerformanceFields(t *testing.T) {
	h, queries := newSectionHandler(t)
	ctx := context.Background()
	row, _ := queries.GetSiteSettings(ctx)
	origTitle := row.SiteTitle
	csrfRec := httptest.NewRecorder()
	csrfReq := httptest.NewRequest(http.MethodGet, "/admin/settings/performance", nil)
	token, _ := h.csrfToken(csrfRec, csrfReq)
	cookies := csrfRec.Result().Cookies()
	form := url.Values{
		"speculation_enabled":   {"on"},
		"speculation_mode":      {"prerender"},
		"speculation_eagerness": {"eager"},
		"csrf_token":            {token},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/settings/performance", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.saveSettingsPerformance(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("POST perf %d body %s", rec.Code, rec.Body.String())
	}
	newRow, _ := queries.GetSiteSettings(ctx)
	if newRow.SpeculationMode != "prerender" {
		t.Fatalf("perf not saved")
	}
	if newRow.SiteTitle != origTitle {
		t.Fatalf("general mutated by perf")
	}
}

func TestNoJSRedirectPathWorks(t *testing.T) {
	h, _ := newSectionHandler(t)
	csrfRec := httptest.NewRecorder()
	csrfReq := httptest.NewRequest(http.MethodGet, "/admin/settings/general", nil)
	token, _ := h.csrfToken(csrfRec, csrfReq)
	cookies := csrfRec.Result().Cookies()
	form := url.Values{"site_title": {"NoJS Title"}, "site_url": {"https://example.com"}, "language": {"en"}, "timezone": {"UTC"}, "site_represents": {"organization"}, "csrf_token": {token}}
	req := httptest.NewRequest(http.MethodPost, "/admin/settings/general", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.saveSettingsGeneral(rec, req)
	if rec.Code != http.StatusSeeOther {
		t.Fatalf("no-js POST should redirect, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/admin/settings/general" {
		t.Fatalf("Location %q", loc)
	}
	_ = strings.Contains("", "")
	_ = filepath.Join("", "")
}
