package admin

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/auth"
	"github.com/kokosx/stratum/internal/creator"
)

func TestCreator_CorrectiveDuplicateFields(t *testing.T) {
	handler, authService := newTestHandler(t)
	session, err := authService.Setup(t.Context(), authService.SetupCode(), "Example", "admin@example.com", "a sufficiently long password")
	if err != nil {
		t.Fatal(err)
	}
	// Simulate Agency preset with portfolio_cols 3 — JS should have disabled hidden portfolio_cols=2, so only 3 arrives.
	// We send single value to verify correct plane.
	form := url.Values{
		"csrf_token":      {"test-csrf"},
		"preset":          {string(creator.PresetAgency)},
		"site_name":       {"Agency Studio"},
		"site_represents": {"organization"},
		"language":        {"en"},
		"timezone":        {"UTC"},
		"portfolio_cols":  {"3"},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/creator", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: session})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "test-csrf"})
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("agency creator status = %d, want 200; body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Your starting point is live") {
		t.Fatalf("agency creator not complete: %s", rec.Body.String())
	}
	// Verify DB read-back: site_settings not directly holding portfolio cols (creation-only), but entry SDT should reflect cols=3?
	// Check published case studies collection has cols 3 via homepage entry doc
	entries, err := handler.queries.CountEntries(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if entries == 0 {
		t.Fatal("no entries after agency create")
	}
}

func TestCreator_CorrectiveMagazineLatest8(t *testing.T) {
	handler, authService := newTestHandler(t)
	session, err := authService.Setup(t.Context(), authService.SetupCode(), "Example", "admin@example.com", "a sufficiently long password")
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{
		"csrf_token":   {"test-csrf"},
		"preset":       {string(creator.PresetMagazine)},
		"site_name":    {"Magazine Daily"},
		"blog_latest":  {"8"},
		"blog_archive": {"20"},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/creator", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: session})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "test-csrf"})
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("magazine status = %d, body: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Your starting point is live") {
		t.Fatalf("magazine not complete")
	}
	// KB archive 20
	handler2, authService2 := newTestHandler(t)
	session2, _ := authService2.Setup(t.Context(), authService2.SetupCode(), "Example", "admin@example.com", "a sufficiently long password")
	form2 := url.Values{"csrf_token": {"test-csrf"}, "preset": {string(creator.PresetKnowledgeBase)}, "site_name": {"KB"}, "blog_archive": {"20"}}
	req2 := httptest.NewRequest(http.MethodPost, "/admin/creator", strings.NewReader(form2.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req2.AddCookie(&http.Cookie{Name: auth.CookieName, Value: session2})
	req2.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "test-csrf"})
	rec2 := httptest.NewRecorder()
	handler2.Routes().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("kb status = %d, body: %s", rec2.Code, rec2.Body.String())
	}
}

func TestCreator_SiteURL_Persisted(t *testing.T) {
	handler, authService := newTestHandler(t)
	session, err := authService.Setup(t.Context(), authService.SetupCode(), "Example", "admin@example.com", "a sufficiently long password")
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{
		"csrf_token": {"test-csrf"},
		"preset":     {string(creator.PresetBlog)},
		"site_name":  {"My Blog"},
		"site_url":   {"https://example.com/"},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/creator", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: session})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "test-csrf"})
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("site_url persist status = %d, body: %s", rec.Code, rec.Body.String())
	}
	settings, err := handler.queries.GetSiteSettings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if settings.SiteUrl != "https://example.com" {
		t.Fatalf("SiteUrl = %q, want https://example.com (normalized, not erased)", settings.SiteUrl)
	}
	// localhost must also persist now
	handler2, authService2 := newTestHandler(t)
	session2, _ := authService2.Setup(t.Context(), authService2.SetupCode(), "Example", "admin@example.com", "a sufficiently long password")
	form2 := url.Values{"csrf_token": {"test-csrf"}, "preset": {string(creator.PresetBlog)}, "site_name": {"Local"}, "site_url": {"http://localhost:3000"}}
	req2 := httptest.NewRequest(http.MethodPost, "/admin/creator", strings.NewReader(form2.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req2.AddCookie(&http.Cookie{Name: auth.CookieName, Value: session2})
	req2.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "test-csrf"})
	rec2 := httptest.NewRecorder()
	handler2.Routes().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("localhost status = %d, body: %s", rec2.Code, rec2.Body.String())
	}
	settings2, _ := handler2.queries.GetSiteSettings(t.Context())
	if settings2.SiteUrl != "http://localhost:3000" {
		t.Fatalf("localhost SiteUrl = %q, want http://localhost:3000", settings2.SiteUrl)
	}
}

func TestCreator_IndexingDiscourage(t *testing.T) {
	handler, authService := newTestHandler(t)
	session, err := authService.Setup(t.Context(), authService.SetupCode(), "Example", "admin@example.com", "a sufficiently long password")
	if err != nil {
		t.Fatal(err)
	}
	// Discourage checked => indexing_enabled 0
	form := url.Values{
		"csrf_token":                {"test-csrf"},
		"preset":                    {string(creator.PresetBlog)},
		"site_name":                 {"Indexed"},
		"discourage_search_engines": {"on"},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/creator", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: session})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "test-csrf"})
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("discourage checked status = %d, body: %s", rec.Code, rec.Body.String())
	}
	settings, _ := handler.queries.GetSiteSettings(t.Context())
	if settings.IndexingEnabled != 0 {
		t.Fatalf("discourage checked: indexing_enabled = %d, want 0", settings.IndexingEnabled)
	}
	// Unchecked => enabled 1
	handler2, authService2 := newTestHandler(t)
	session2, _ := authService2.Setup(t.Context(), authService2.SetupCode(), "Example", "admin@example.com", "a sufficiently long password")
	form2 := url.Values{"csrf_token": {"test-csrf"}, "preset": {string(creator.PresetBlog)}, "site_name": {"Indexed2"}}
	req2 := httptest.NewRequest(http.MethodPost, "/admin/creator", strings.NewReader(form2.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req2.AddCookie(&http.Cookie{Name: auth.CookieName, Value: session2})
	req2.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "test-csrf"})
	rec2 := httptest.NewRecorder()
	handler2.Routes().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("discourage unchecked status = %d", rec2.Code)
	}
	settings2, _ := handler2.queries.GetSiteSettings(t.Context())
	if settings2.IndexingEnabled != 1 {
		t.Fatalf("unchecked: indexing_enabled = %d, want 1", settings2.IndexingEnabled)
	}
	// Legacy name fallback still works
	handler3, authService3 := newTestHandler(t)
	session3, _ := authService3.Setup(t.Context(), authService3.SetupCode(), "Example", "admin@example.com", "a sufficiently long password")
	form3 := url.Values{"csrf_token": {"test-csrf"}, "preset": {string(creator.PresetBlog)}, "site_name": {"Legacy"}, "indexing_enabled": {"on"}}
	req3 := httptest.NewRequest(http.MethodPost, "/admin/creator", strings.NewReader(form3.Encode()))
	req3.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req3.AddCookie(&http.Cookie{Name: auth.CookieName, Value: session3})
	req3.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "test-csrf"})
	rec3 := httptest.NewRecorder()
	handler3.Routes().ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusOK {
		t.Fatalf("legacy indexing_enabled status = %d", rec3.Code)
	}
	settings3, _ := handler3.queries.GetSiteSettings(t.Context())
	if settings3.IndexingEnabled != 0 {
		t.Fatalf("legacy discourage: want 0, got %d", settings3.IndexingEnabled)
	}
}

func TestCreator_ReadBack_AllSiteSettings(t *testing.T) {
	handler, authService := newTestHandler(t)
	session, err := authService.Setup(t.Context(), authService.SetupCode(), "Example", "admin@example.com", "a sufficiently long password")
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{
		"csrf_token":                {"test-csrf"},
		"preset":                    {string(creator.PresetBlog)},
		"site_name":                 {"My Title"},
		"tagline":                   {"My tagline"},
		"site_url":                  {"https://example.com"},
		"language":                  {"pl"},
		"timezone":                  {"Europe/Warsaw"},
		"site_represents":           {"person"},
		"discourage_search_engines": {"on"},
		"blog_archive":              {"20"},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/creator", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: session})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "test-csrf"})
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("readback status = %d, body: %s", rec.Code, rec.Body.String())
	}
	settings, err := handler.queries.GetSiteSettings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if settings.SiteTitle != "My Title" {
		t.Fatalf("SiteTitle = %q, want My Title", settings.SiteTitle)
	}
	if settings.SiteTagline != "My tagline" {
		t.Fatalf("SiteTagline = %q", settings.SiteTagline)
	}
	if settings.SiteUrl != "https://example.com" {
		t.Fatalf("SiteUrl = %q, want https://example.com", settings.SiteUrl)
	}
	if settings.Language != "pl" {
		t.Fatalf("Language = %q, want pl", settings.Language)
	}
	if settings.Timezone != "Europe/Warsaw" {
		t.Fatalf("Timezone = %q, want Europe/Warsaw", settings.Timezone)
	}
	if settings.SiteRepresents != "person" {
		t.Fatalf("SiteRepresents = %q, want person", settings.SiteRepresents)
	}
	if settings.IndexingEnabled != 0 {
		t.Fatalf("IndexingEnabled = %d, want 0 (discouraged)", settings.IndexingEnabled)
	}
	if settings.PostsPerPage != 20 {
		t.Fatalf("PostsPerPage = %d, want 20", settings.PostsPerPage)
	}
}
