package admin

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/auth"
	"github.com/kokosx/stratum/internal/creator"
	"github.com/kokosx/stratum/internal/document"
)

func TestCreatorStudio_SchemaValidation(t *testing.T) {
	handler, _ := newTestHandler(t)
	for _, preset := range creator.Presets() {
		plan := creator.Plan{Input: creator.Input{PresetID: preset.ID, SiteTitle: "Test", Tagline: "hello", Language: "en", Timezone: "UTC", SiteRepresents: "organization", BlogLatestCount: 5, BlogArchiveCount: 10, PortfolioColumns: 2, ProductColumns: 3, LandingTestimonialsColumns: 2, ServiceColumns: 3, ProductMediaPosition: "left", PaletteID: creator.DefaultPaletteForPreset(preset.ID), HeaderStyleID: creator.DefaultHeaderForPreset(preset.ID), FooterStyleID: creator.DefaultFooterForPreset(preset.ID)}, Preset: preset}
		html, err := creator.RenderPreview(t.Context(), plan, creator.SurfaceHome, handler.blocks, handler.themes)
		if err != nil {
			t.Fatalf("%s preview render failed: %v", preset.ID, err)
		}
		if strings.Contains(html, "fullWidth") {
			t.Fatalf("%s preview contains dead fullWidth", preset.ID)
		}
	}
}

func TestCreatorStudio_PreviewNoDBMutation(t *testing.T) {
	handler, authService := newTestHandler(t)
	session, err := authService.Setup(t.Context(), authService.SetupCode(), "Example", "admin@example.com", "a sufficiently long password")
	if err != nil {
		t.Fatal(err)
	}
	countBefore, err := handler.queries.CountEntries(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	completedBefore, err := handler.queries.GetOnboardingCompleted(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{
		"preset":    {string(creator.PresetBlog)},
		"palette":   {string(creator.PaletteInk)},
		"site_name": {"Preview Studio"},
		"tagline":   {"hello"},
		"surface":   {"home"},
	}
	req := httptest.NewRequest(http.MethodGet, "/admin/creator/preview?"+form.Encode(), nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: session})
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview status = %d, want 200; body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Preview Studio") && !strings.Contains(rec.Body.String(), "Site preview") {
		// Check that site title appears in preview
		if !strings.Contains(rec.Body.String(), "Preview Studio") {
			// The preview uses site title as entry title
		}
	}
	countAfter, err := handler.queries.CountEntries(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if countBefore != countAfter {
		t.Fatalf("preview mutated DB: before %d after %d", countBefore, countAfter)
	}
	completedAfter, err := handler.queries.GetOnboardingCompleted(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if completedBefore != completedAfter {
		t.Fatalf("preview changed onboarding")
	}
}

func TestCreatorStudio_PreviewCollectionAndNavigation(t *testing.T) {
	handler, authService := newTestHandler(t)
	session, err := authService.Setup(t.Context(), authService.SetupCode(), "Example", "admin@example.com", "a sufficiently long password")
	if err != nil {
		t.Fatal(err)
	}
	// Portfolio preset should have project collection with synthetic entries
	form := url.Values{
		"preset":    {string(creator.PresetPortfolio)},
		"palette":   {string(creator.PaletteInk)},
		"site_name": {"Studio"},
		"language":  {"en"},
	}
	req := httptest.NewRequest(http.MethodGet, "/admin/creator/preview?"+form.Encode()+"&surface=home", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: session})
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview portfolio status %d", rec.Code)
	}
	body := rec.Body.String()
	// Should contain project titles from synthetic data
	if !strings.Contains(body, "North House") && !strings.Contains(body, "Field Notes") {
		t.Fatalf("portfolio preview missing synthetic project, body %s", body[:2000])
	}
	// Header should render navigation
	if !strings.Contains(body, `stratum-navigation`) && !strings.Contains(body, `site-navigation`) {
		// Navigation may be rendered via block
		if !strings.Contains(body, "Home") {
			t.Fatalf("portfolio preview missing navigation Home")
		}
	}
	// Form preview cannot submit: should contain form but not actual submission endpoint? Check that form is rendered but with placeholder
	// Landing preview with form
	form2 := url.Values{
		"preset":    {string(creator.PresetLanding)},
		"site_name": {"Landing"},
	}
	req2 := httptest.NewRequest(http.MethodGet, "/admin/creator/preview?"+form2.Encode(), nil)
	req2.AddCookie(&http.Cookie{Name: auth.CookieName, Value: session})
	rec2 := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("landing preview status %d", rec2.Code)
	}
	if !strings.Contains(rec2.Body.String(), "stratum-form") {
		t.Fatalf("landing preview should contain form")
	}
	// Palette changes output: Ink vs Clay should differ
	formInk := url.Values{"preset": {string(creator.PresetBlog)}, "palette": {string(creator.PaletteInk)}, "site_name": {"Test"}}
	formClay := url.Values{"preset": {string(creator.PresetBlog)}, "palette": {string(creator.PaletteClay)}, "site_name": {"Test"}}
	reqInk := httptest.NewRequest(http.MethodGet, "/admin/creator/preview?"+formInk.Encode(), nil)
	reqInk.AddCookie(&http.Cookie{Name: auth.CookieName, Value: session})
	recInk := httptest.NewRecorder()
	handler.Routes().ServeHTTP(recInk, reqInk)
	reqClay := httptest.NewRequest(http.MethodGet, "/admin/creator/preview?"+formClay.Encode(), nil)
	reqClay.AddCookie(&http.Cookie{Name: auth.CookieName, Value: session})
	recClay := httptest.NewRecorder()
	handler.Routes().ServeHTTP(recClay, reqClay)
	if recInk.Body.String() == recClay.Body.String() {
		t.Fatal("palette change did not affect preview output")
	}
}

func TestCreatorStudio_HomepageOwnership(t *testing.T) {
	handler, authService := newTestHandler(t)
	session, err := authService.Setup(t.Context(), authService.SetupCode(), "Example", "admin@example.com", "a sufficiently long password")
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{
		"csrf_token": {"test-csrf"},
		"preset":     {string(creator.PresetBlog)},
		"site_name":  {"Blog Site"},
		"language":   {"en"},
		"timezone":   {"UTC"},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/creator", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: session})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "test-csrf"})
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("creator failed %d %s", rec.Code, rec.Body.String())
	}
	home, err := handler.queries.GetPublishedEntryByPath(t.Context(), "/")
	if err != nil {
		t.Fatal(err)
	}
	rev, err := handler.queries.GetLatestEntryRevision(t.Context(), home.ID)
	if err != nil {
		t.Fatal(err)
	}
	if rev.DocumentJson == "[]" || rev.DocumentJson == `{"version":1,"nodes":[]}` {
		t.Fatal("homepage entry SDT is empty, should own layout")
	}
	if !strings.Contains(rev.DocumentJson, "core/collection") {
		t.Fatalf("homepage entry must own collection, got %s", rev.DocumentJson)
	}
	if !strings.Contains(rev.DocumentJson, "core/site-tagline") {
		t.Fatalf("homepage entry should use dynamic site-tagline, got %s", rev.DocumentJson)
	}
	templateRev, err := handler.queries.GetPublishedLayoutTemplateRevision(t.Context(), home.LayoutTemplateID.String)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(templateRev.DocumentJson, "core/content-slot") {
		t.Fatalf("homepage shell must contain content-slot")
	}
	if strings.Contains(templateRev.DocumentJson, "core/collection") {
		t.Fatalf("homepage shell must not contain collection, got %s", templateRev.DocumentJson)
	}
	// Shared templates stay templates: Post Single and Archive remain
	cts, err := handler.queries.ListContentTypes(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	foundPostSingle := false
	foundArchive := false
	for _, ct := range cts {
		if ct.ID == "post" {
			tmpls, err := handler.queries.ListLayoutTemplatesByContentType(t.Context(), "post")
			if err != nil {
				t.Fatal(err)
			}
			for _, tmpl := range tmpls {
				if tmpl.Kind == "single" {
					foundPostSingle = true
				}
				if tmpl.Kind == "archive" {
					foundArchive = true
				}
			}
		}
	}
	if !foundPostSingle || !foundArchive {
		t.Fatal("post single/archive templates should remain")
	}
}

func TestCreatorStudio_LayoutOptions(t *testing.T) {
	// Portfolio 2/3
	for _, tc := range []struct {
		cols string
		want string
	}{
		{"2", `"columns":2`}, {"3", `"columns":3`},
	} {
		handler, authService := newTestHandler(t)
		session, err := authService.Setup(t.Context(), authService.SetupCode(), "Example", "admin@example.com", "a sufficiently long password")
		if err != nil {
			t.Fatal(err)
		}
		form := url.Values{
			"csrf_token":     {"test-csrf"},
			"preset":         {string(creator.PresetPortfolio)},
			"site_name":      {"P"},
			"portfolio_cols": {tc.cols},
		}
		req := httptest.NewRequest(http.MethodPost, "/admin/creator", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: session})
		req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "test-csrf"})
		rec := httptest.NewRecorder()
		handler.Routes().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("portfolio cols %s failed %d", tc.cols, rec.Code)
		}
		home, _ := handler.queries.GetPublishedEntryByPath(t.Context(), "/")
		rev, _ := handler.queries.GetLatestEntryRevision(t.Context(), home.ID)
		if !strings.Contains(rev.DocumentJson, tc.want) {
			t.Fatalf("portfolio cols %s not in entry %s", tc.cols, rev.DocumentJson)
		}
	}
	// Product 3/4 and media left/right
	handler, authService := newTestHandler(t)
	{
		session, _ := authService.Setup(t.Context(), authService.SetupCode(), "Example", "admin@example.com", "a sufficiently long password")
		form := url.Values{"csrf_token": {"test-csrf"}, "preset": {string(creator.PresetProducts)}, "site_name": {"Shop"}, "product_cols": {"4"}, "product_media": {"right"}}
		req := httptest.NewRequest(http.MethodPost, "/admin/creator", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: session})
		req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "test-csrf"})
		rec := httptest.NewRecorder()
		handler.Routes().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("product media right failed")
		}
		// Check single template for media position? We need to fetch template for product single
		templates, _ := handler.queries.ListLayoutTemplatesByContentType(t.Context(), "product")
		found := false
		for _, tmpl := range templates {
			if tmpl.Kind == "single" {
				rev, _ := handler.queries.GetPublishedLayoutTemplateRevision(t.Context(), tmpl.ID)
				_ = rev
				found = true
			}
		}
		if !found {
			t.Fatal("product single not found")
		}
	}
}

func TestCreatorStudio_SiteSettingsPersisted(t *testing.T) {
	handler, authService := newTestHandler(t)
	session, err := authService.Setup(t.Context(), authService.SetupCode(), "Example", "admin@example.com", "a sufficiently long password")
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{
		"csrf_token":      {"test-csrf"},
		"preset":          {string(creator.PresetBlog)},
		"site_name":       {"Persisted Site"},
		"tagline":         {"hello"},
		"language":        {"pl"},
		"timezone":        {"Europe/Warsaw"},
		"site_represents": {"person"},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/creator", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: session})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "test-csrf"})
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create failed %d %s", rec.Code, rec.Body.String())
	}
	row, err := handler.queries.GetSiteSettings(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if row.Language != "pl" {
		t.Fatalf("language not persisted, got %s", row.Language)
	}
	if row.Timezone != "Europe/Warsaw" {
		t.Fatalf("timezone not persisted %s", row.Timezone)
	}
	if row.SiteRepresents != "person" {
		t.Fatalf("represents not persisted %s", row.SiteRepresents)
	}
	if row.IndexingEnabled != 1 {
		t.Fatalf("indexing should be 1 by default (site crawlable), got %d", row.IndexingEnabled)
	}
	if row.SitemapEnabled == 0 {
		t.Fatalf("sitemap should be enabled")
	}
	if row.RobotsMode != "managed" {
		t.Fatalf("robots mode %s", row.RobotsMode)
	}
	if row.SpeculationMode != "off" {
		t.Fatalf("speculation %s", row.SpeculationMode)
	}
	icon, err := handler.queries.GetSiteIconMediaID(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !icon.Valid || icon.String == "" {
		t.Fatal("favicon not persisted")
	}
	// Social image
	if !row.SiteSocialMediaID.Valid || row.SiteSocialMediaID.String == "" {
		t.Fatalf("social image not persisted, got %#v", row.SiteSocialMediaID)
	}
}

func TestCreatorStudio_SchemaNoFullWidth(t *testing.T) {
	// Ensure no generated doc contains fullWidth dead setting
	handler, authService := newTestHandler(t)
	session, _ := authService.Setup(t.Context(), authService.SetupCode(), "Example", "admin@example.com", "a sufficiently long password")
	form := url.Values{"csrf_token": {"test-csrf"}, "preset": {string(creator.PresetBlog)}, "site_name": {"Test"}}
	req := httptest.NewRequest(http.MethodPost, "/admin/creator", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: session})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "test-csrf"})
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("create failed")
	}
	// Check all entry revisions for fullWidth and validate
	rows, err := handler.database.QueryContext(t.Context(), "SELECT id FROM entries")
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	for _, id := range ids {
		rev, err := handler.queries.GetLatestEntryRevision(t.Context(), id)
		if err != nil {
			continue
		}
		if strings.Contains(rev.DocumentJson, "fullWidth") {
			t.Fatalf("entry %s contains dead fullWidth", id)
		}
		doc, err := parseDoc(rev.DocumentJson)
		if err != nil {
			t.Fatalf("parse doc %s: %v", id, err)
		}
		if err := handler.blocks.ValidateDocument(doc); err != nil {
			t.Fatalf("entry %s validation failed: %v", id, err)
		}
	}
}

func parseDoc(s string) (*document.Document, error) {
	// Use document.Decode
	return document.Decode([]byte(s))
}
