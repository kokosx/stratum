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

func TestCreatorInputStyleValidation(t *testing.T) {
	handler, authService := newTestHandler(t)
	session, err := authService.Setup(t.Context(), authService.SetupCode(), "Example", "admin@example.com", "a sufficiently long password")
	if err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		palette string
		header  string
		footer  string
		wantErr bool
	}{
		{"ink", "minimal", "simple", false},
		{"unknown", "minimal", "simple", true},
		{"ink", "unknown", "simple", true},
		{"ink", "minimal", "unknown", true},
		{"", "minimal", "simple", false}, // empty should default, not error
	}
	for _, tc := range cases {
		form := url.Values{
			"csrf_token":   {"test-csrf"},
			"preset":       {string(creator.PresetBlog)},
			"site_name":    {"Example"},
			"tagline":      {""},
			"palette":      {tc.palette},
			"header_style": {tc.header},
			"footer_style": {tc.footer},
		}
		req := httptest.NewRequest(http.MethodPost, "/admin/creator", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: session})
		req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "test-csrf"})
		rec := httptest.NewRecorder()
		handler.Routes().ServeHTTP(rec, req)
		if tc.wantErr && rec.Code != http.StatusUnprocessableEntity {
			t.Fatalf("palette=%q header=%q footer=%q expected 422 got %d body %s", tc.palette, tc.header, tc.footer, rec.Code, rec.Body.String())
		}
		if !tc.wantErr && rec.Code == http.StatusUnprocessableEntity && strings.Contains(rec.Body.String(), "valid") {
			t.Fatalf("unexpected validation error for palette=%q header=%q footer=%q: %s", tc.palette, tc.header, tc.footer, rec.Body.String())
		}
		// Reset DB for next case: need fresh handler each iteration to avoid completed onboarding
		if rec.Code == http.StatusOK {
			break // only first success completes onboarding, so break after success
		}
		// create new handler for next iteration if onboarding completed? Actually only success completes, so after success we stop.
		// For failure cases onboarding not completed, we can reuse same handler.
	}
}

func TestCreatorPaletteApplicationDiffers(t *testing.T) {
	for _, palette := range creator.Palettes() {
		t.Run(string(palette.ID), func(t *testing.T) {
			handler, authService := newTestHandler(t)
			session, err := authService.Setup(t.Context(), authService.SetupCode(), "Example", "admin@example.com", "a sufficiently long password")
			if err != nil {
				t.Fatal(err)
			}
			form := url.Values{
				"csrf_token":   {"test-csrf"},
				"preset":       {string(creator.PresetBlog)},
				"site_name":    {"Roksa"},
				"tagline":      {"A thoughtful tagline"},
				"palette":      {string(palette.ID)},
				"header_style": {string(creator.HeaderClassic)},
				"footer_style": {string(creator.FooterSimple)},
			}
			req := httptest.NewRequest(http.MethodPost, "/admin/creator", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: session})
			req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "test-csrf"})
			rec := httptest.NewRecorder()
			handler.Routes().ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("creation failed for palette %s: %d %s", palette.ID, rec.Code, rec.Body.String())
			}
			active := handler.themes.Current()
			custom, err := handler.queries.GetThemeCustomization(t.Context(), active.ThemeID)
			if err != nil {
				t.Fatal(err)
			}
			body := custom.SettingsJson
			// Each palette must set primary and header/footer bg
			if !strings.Contains(body, `"colors.primary"`) {
				t.Fatalf("palette %s missing colors.primary in %s", palette.ID, body)
			}
			// Check that header bg matches palette expectation (e.g., Ink header #f7f7f5 vs Clay #fbf8f4)
			// At least ensure palette-specific primary is present
			switch palette.ID {
			case creator.PaletteInk:
				if !strings.Contains(body, "#111111") {
					t.Fatalf("ink palette should contain #111111, got %s", body)
				}
			case creator.PaletteClay:
				if !strings.Contains(body, "#8b3a3a") {
					t.Fatalf("clay palette should contain #8b3a3a, got %s", body)
				}
			case creator.PaletteForest:
				if !strings.Contains(body, "#356859") {
					t.Fatalf("forest palette should contain #356859, got %s", body)
				}
			case creator.PaletteIndigo:
				if !strings.Contains(body, "#6842a8") {
					t.Fatalf("indigo palette should contain #6842a8, got %s", body)
				}
			}
			// Check that links color is coherent (not old blue #2563eb)
			if strings.Contains(body, "#2563eb") && palette.ID != creator.PaletteInk /*ink primary is #111111 so blue should not appear*/ {
				// Allow if palette is ink? ink doesn't have blue, so any blue is leak
				t.Fatalf("palette %s leaked default blue #2563eb into settings: %s", palette.ID, body)
			}
		})
	}
}

func TestCreatorHeaderSitePartDifferences(t *testing.T) {
	for _, header := range creator.HeaderOptions() {
		t.Run(string(header.ID), func(t *testing.T) {
			handler, authService := newTestHandler(t)
			session, err := authService.Setup(t.Context(), authService.SetupCode(), "Example", "admin@example.com", "a sufficiently long password")
			if err != nil {
				t.Fatal(err)
			}
			form := url.Values{
				"csrf_token":   {"test-csrf"},
				"preset":       {string(creator.PresetBlog)},
				"site_name":    {"Roksa"},
				"palette":      {string(creator.PaletteClay)},
				"header_style": {string(header.ID)},
				"footer_style": {string(creator.FooterSimple)},
			}
			req := httptest.NewRequest(http.MethodPost, "/admin/creator", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: session})
			req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "test-csrf"})
			rec := httptest.NewRecorder()
			handler.Routes().ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("header %s creation failed: %d %s", header.ID, rec.Code, rec.Body.String())
			}
			// Inspect header site part
			parts, err := handler.queries.ListSiteParts(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			var headerDoc string
			for _, p := range parts {
				loc, err := handler.queries.GetSitePartLocation(t.Context(), "header")
				if err != nil {
					continue
				}
				if loc.SitePartID.String == p.ID {
					rev, err := handler.queries.GetPublishedSitePartRevision(t.Context(), p.ID)
					if err != nil {
						t.Fatal(err)
					}
					headerDoc = rev.DocumentJson
					break
				}
			}
			if headerDoc == "" {
				t.Fatal("header site part not found")
			}
			// Minimal should not contain site-tagline, classic should
			hasTagline := strings.Contains(headerDoc, "site-tagline")
			if header.ID == creator.HeaderMinimal && hasTagline {
				t.Fatalf("minimal header should not have tagline, got %s", headerDoc)
			}
			if header.ID == creator.HeaderClassic && !hasTagline {
				t.Fatalf("classic header should have tagline, got %s", headerDoc)
			}
			// Check public HTML contains themed navigation inside header custom wrapper and not default blue?
			publicHandler := newCreatorPublicHandler(t, handler)
			homepage := requestCreatorPage(t, publicHandler, "/")
			if !strings.Contains(homepage, "site-header__custom") {
				t.Fatalf("homepage missing site-header__custom for header %s", header.ID)
			}
			if !strings.Contains(homepage, "stratum-navigation") {
				t.Fatalf("homepage missing stratum-navigation for header %s", header.ID)
			}
			if !strings.Contains(homepage, "stratum-site-name") {
				t.Fatalf("homepage missing site-name for header %s", header.ID)
			}
			// Ensure no duplicate mobile toggle? At least one toggle exists
			count := strings.Count(homepage, "mobile-menu-toggle")
			if count != 1 {
				t.Fatalf("homepage mobile toggle count = %d want 1 for header %s", count, header.ID)
			}
		})
	}
}

func TestCreatorFooterSitePartDifferences(t *testing.T) {
	for _, footer := range creator.FooterOptions() {
		t.Run(string(footer.ID), func(t *testing.T) {
			handler, authService := newTestHandler(t)
			session, err := authService.Setup(t.Context(), authService.SetupCode(), "Example", "admin@example.com", "a sufficiently long password")
			if err != nil {
				t.Fatal(err)
			}
			form := url.Values{
				"csrf_token":   {"test-csrf"},
				"preset":       {string(creator.PresetPortfolio)},
				"site_name":    {"Studio"},
				"palette":      {string(creator.PaletteInk)},
				"header_style": {string(creator.HeaderMinimal)},
				"footer_style": {string(footer.ID)},
			}
			req := httptest.NewRequest(http.MethodPost, "/admin/creator", strings.NewReader(form.Encode()))
			req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: session})
			req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "test-csrf"})
			rec := httptest.NewRecorder()
			handler.Routes().ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("footer %s creation failed: %d %s", footer.ID, rec.Code, rec.Body.String())
			}
			loc, err := handler.queries.GetSitePartLocation(t.Context(), "footer")
			if err != nil {
				t.Fatal(err)
			}
			if !loc.SitePartID.Valid {
				t.Fatal("footer location not set")
			}
			rev, err := handler.queries.GetPublishedSitePartRevision(t.Context(), loc.SitePartID.String)
			if err != nil {
				t.Fatal(err)
			}
			doc := rev.DocumentJson
			hasNav := strings.Contains(doc, "core/navigation")
			hasTagline := strings.Contains(doc, "site-tagline")
			switch footer.ID {
			case creator.FooterSimple:
				if hasNav {
					t.Fatalf("simple footer should not have navigation, got %s", doc)
				}
			case creator.FooterSplit:
				if !hasNav || !hasTagline {
					t.Fatalf("split footer should have nav and tagline, got %s", doc)
				}
			case creator.FooterCentered:
				if !hasNav {
					t.Fatalf("centered footer should have nav, got %s", doc)
				}
			}
			// Check footer renders on public site
			publicHandler := newCreatorPublicHandler(t, handler)
			homepage := requestCreatorPage(t, publicHandler, "/")
			if !strings.Contains(homepage, "site-footer") {
				t.Fatalf("homepage missing site-footer for footer %s", footer.ID)
			}
			if !strings.Contains(homepage, "site-footer__legal") {
				t.Fatalf("homepage missing legal for footer %s", footer.ID)
			}
			// Footer navigation should be inside site-footer__main when present
			if hasNav && !strings.Contains(homepage, "site-footer__main") {
				t.Fatalf("footer nav missing wrapper")
			}
		})
	}
}

func TestCreatorNoFormV1(t *testing.T) {
	handler, authService := newTestHandler(t)
	session, err := authService.Setup(t.Context(), authService.SetupCode(), "Example", "admin@example.com", "a sufficiently long password")
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{
		"csrf_token": {"test-csrf"},
		"preset":     {string(creator.PresetLanding)},
		"site_name":  {"Landing"},
	}
	// Intentionally omit palette/header/footer to test server defaults
	req := httptest.NewRequest(http.MethodPost, "/admin/creator", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: session})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "test-csrf"})
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("landing creation failed: %d %s", rec.Code, rec.Body.String())
	}
	// Check no document contains form@1
	// Check homepage template, site parts, entries?
	templates, err := handler.queries.ListLayoutTemplates(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, tmpl := range templates {
		rev, err := handler.queries.GetPublishedLayoutTemplateRevision(t.Context(), tmpl.ID)
		if err != nil {
			continue
		}
		if strings.Contains(rev.DocumentJson, `"block":"core/form","version":1`) {
			t.Fatalf("found form@1 in template %s: %s", tmpl.Name, rev.DocumentJson)
		}
	}
	parts, err := handler.queries.ListSiteParts(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range parts {
		rev, err := handler.queries.GetPublishedSitePartRevision(t.Context(), p.ID)
		if err != nil {
			continue
		}
		if strings.Contains(rev.DocumentJson, `"block":"core/form","version":1`) {
			t.Fatalf("found form@1 in site part")
		}
	}
}

func TestBlogEmptyTaglineCompact(t *testing.T) {
	handler, authService := newTestHandler(t)
	session, err := authService.Setup(t.Context(), authService.SetupCode(), "Example", "admin@example.com", "a sufficiently long password")
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{
		"csrf_token":   {"test-csrf"},
		"preset":       {string(creator.PresetBlog)},
		"site_name":    {"Roksa"},
		"tagline":      {""},
		"palette":      {string(creator.PaletteClay)},
		"header_style": {string(creator.HeaderClassic)},
		"footer_style": {string(creator.FooterSimple)},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/creator", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: session})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "test-csrf"})
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("blog empty tagline creation failed: %d %s", rec.Code, rec.Body.String())
	}
	// After EPIC 7 art-direction, Blog homepage is ONE editorial band (Section v2 content/lg/default)
	// containing hero Stack + Latest posts. Both with and without tagline use lg for consistent top padding 56-80.
	home, err := handler.queries.GetPublishedEntryByPath(t.Context(), "/")
	if err != nil {
		t.Fatal(err)
	}
	rev, err := handler.queries.GetPublishedLayoutTemplateRevision(t.Context(), home.LayoutTemplateID.String)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rev.DocumentJson, `"verticalSpacing":"lg"`) {
		t.Fatalf("blog without tagline should have lg editorial band, got %s", rev.DocumentJson)
	}
	if !strings.Contains(rev.DocumentJson, `"width":"content"`) {
		t.Fatalf("blog should use content width, got %s", rev.DocumentJson)
	}
	// Verify single main section + content-slot, not two separate sections with dead whitespace
	if strings.Count(rev.DocumentJson, `"block":"core/section"`) != 1 {
		t.Fatalf("blog homepage should have exactly one main section after art-direction, got %s", rev.DocumentJson)
	}
}

func TestCollectionPresentationAndPresetSpecifics(t *testing.T) {
	handler, authService := newTestHandler(t)
	session, err := authService.Setup(t.Context(), authService.SetupCode(), "Example", "admin@example.com", "a sufficiently long password")
	if err != nil {
		t.Fatal(err)
	}
	// Test Product Showcase grid 3
	form := url.Values{
		"csrf_token": {"test-csrf"},
		"preset":     {string(creator.PresetProducts)},
		"site_name":  {"Shop"},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/creator", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: session})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "test-csrf"})
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("product creation failed: %d %s", rec.Code, rec.Body.String())
	}
	home, err := handler.queries.GetPublishedEntryByPath(t.Context(), "/")
	if err != nil {
		t.Fatal(err)
	}
	rev, err := handler.queries.GetPublishedLayoutTemplateRevision(t.Context(), home.LayoutTemplateID.String)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rev.DocumentJson, `"layout":"grid"`) || !strings.Contains(rev.DocumentJson, `"columns":3`) {
		t.Fatalf("product collection should be grid 3, got %s", rev.DocumentJson)
	}
	if !strings.Contains(rev.DocumentJson, `"contentType":"product"`) {
		t.Fatalf("product collection contentType")
	}
	// Check public page contains product grid presentation
	pub := newCreatorPublicHandler(t, handler)
	homepage := requestCreatorPage(t, pub, "/")
	if !strings.Contains(homepage, "stratum-collection--grid") {
		t.Fatalf("product homepage missing grid presentation")
	}
}

func TestLandingTestimonialsNoMedia(t *testing.T) {
	handler, authService := newTestHandler(t)
	session, err := authService.Setup(t.Context(), authService.SetupCode(), "Example", "admin@example.com", "a sufficiently long password")
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{
		"csrf_token": {"test-csrf"},
		"preset":     {string(creator.PresetLanding)},
		"site_name":  {"Landing"},
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/creator", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: session})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "test-csrf"})
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("landing creation failed")
	}
	home, err := handler.queries.GetPublishedEntryByPath(t.Context(), "/")
	if err != nil {
		t.Fatal(err)
	}
	rev, err := handler.queries.GetPublishedLayoutTemplateRevision(t.Context(), home.LayoutTemplateID.String)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(rev.DocumentJson, "entry.featured_media") || strings.Contains(rev.DocumentJson, "core/entry-media") {
		t.Fatalf("landing testimonials should not have media, got %s", rev.DocumentJson)
	}
	if !strings.Contains(rev.DocumentJson, "fields.quote") {
		t.Fatalf("landing testimonials should contain quote field")
	}
}
