package admin

import (
	"database/sql"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/kokosx/stratum/internal/auth"
	"github.com/kokosx/stratum/internal/creator"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	publicweb "github.com/kokosx/stratum/internal/web/public"
)

type creatorPresetExpectation struct {
	contentType string
	archivePath string
	entryPath   string
	sampleTitle string
	pagePaths   []string
	menuLabels  []string
}

var creatorPresetExpectations = map[creator.PresetID]creatorPresetExpectation{
	creator.PresetBlog:          {contentType: "post", archivePath: "/blog", entryPath: "/blog/getting-started", sampleTitle: "Getting started", pagePaths: []string{"/about"}, menuLabels: []string{"Home", "Blog", "About"}},
	creator.PresetPortfolio:     {contentType: "project", archivePath: "/work", entryPath: "/work/north-house", sampleTitle: "North House", pagePaths: []string{"/about", "/contact"}, menuLabels: []string{"Home", "Work", "About", "Contact"}},
	creator.PresetLanding:       {contentType: "testimonial", sampleTitle: "Maya Chen", menuLabels: []string{"Home"}},
	creator.PresetProducts:      {contentType: "product", archivePath: "/products", entryPath: "/products/form-chair", sampleTitle: "Form Chair", pagePaths: []string{"/about", "/contact"}, menuLabels: []string{"Home", "Products", "About", "Contact"}},
	creator.PresetLocalBusiness: {contentType: "service", archivePath: "/services", entryPath: "/services/consultation", sampleTitle: "Consultation", pagePaths: []string{"/about", "/contact"}, menuLabels: []string{"Home", "Services", "About", "Contact"}},
}

func TestCreatorBuildsEveryPreset(t *testing.T) {
	for _, preset := range creator.Presets() {
		t.Run(string(preset.ID), func(t *testing.T) {
			expect := creatorPresetExpectations[preset.ID]
			handler, authService := newTestHandler(t)
			session, err := authService.Setup(t.Context(), authService.SetupCode(), "Example", "admin@example.com", "a sufficiently long password")
			if err != nil {
				t.Fatal(err)
			}
			form := url.Values{
				"csrf_token": {"test-csrf"},
				"preset":     {string(preset.ID)},
				"site_name":  {"Example Studio"},
				"tagline":    {"A deterministic starting point"},
			}
			request := httptest.NewRequest(http.MethodPost, "/admin/creator", strings.NewReader(form.Encode()))
			request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			request.AddCookie(&http.Cookie{Name: auth.CookieName, Value: session})
			request.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "test-csrf"})
			response := httptest.NewRecorder()

			handler.Routes().ServeHTTP(response, request)

			if response.Code != http.StatusOK {
				t.Fatalf("creator status = %d, want 200; body: %s", response.Code, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), "Your starting point is live") {
				t.Fatalf("creator did not render completion: %s", response.Body.String())
			}
			completed, err := handler.queries.GetOnboardingCompleted(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if completed != 1 {
				t.Fatalf("onboarding_completed = %d, want 1", completed)
			}
			count, err := handler.queries.CountEntries(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if count < 1 {
				t.Fatal("Creator committed no entries")
			}
			if _, ok := handler.runtime.Routes.Lookup("/"); !ok {
				t.Fatal("homepage route was not refreshed after Creator commit")
			}
			publicHandler := newCreatorPublicHandler(t, handler)
			homepage := requestCreatorPage(t, publicHandler, "/")
			for _, marker := range []string{"site-header", "site-footer", "site-footer__legal", "rel=\"icon\"", expect.sampleTitle} {
				if !strings.Contains(homepage, marker) {
					t.Fatalf("generated homepage is missing %q", marker)
				}
			}
			for _, path := range append(append([]string{}, expect.pagePaths...), expect.archivePath, expect.entryPath) {
				if path != "" {
					requestCreatorPage(t, publicHandler, path)
				}
			}

			home, err := handler.queries.GetPublishedEntryByPath(t.Context(), "/")
			if err != nil {
				t.Fatal(err)
			}
			homeTemplate, err := handler.queries.GetPublishedLayoutTemplateRevision(t.Context(), home.LayoutTemplateID.String)
			if err != nil {
				t.Fatal(err)
			}
			// Homepage template must be minimal shell (only content-slot)
			if !strings.Contains(homeTemplate.DocumentJson, `"block":"core/content-slot"`) {
				t.Fatalf("homepage template must preserve entry content via content-slot: %s", homeTemplate.DocumentJson)
			}
			if strings.Contains(homeTemplate.DocumentJson, `"block":"core/collection"`) {
				t.Fatalf("homepage template must be minimal shell without collection: %s", homeTemplate.DocumentJson)
			}
			// Homepage entry SDT must own the page-specific layout (collection, hero etc)
			homeRev, err := handler.queries.GetLatestEntryRevision(t.Context(), home.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(homeRev.DocumentJson, `"block":"core/collection"`) {
				t.Fatalf("homepage entry must contain collection: %s", homeRev.DocumentJson)
			}
			if !strings.Contains(homeRev.DocumentJson, `"block":"core/section"`) {
				t.Fatalf("homepage entry must contain section: %s", homeRev.DocumentJson)
			}
			isLanding := preset.ID == creator.PresetLanding
			if isLanding {
				if !strings.Contains(homeRev.DocumentJson, `"anchorID":"contact"`) {
					t.Fatalf("landing homepage entry must anchor contact form: %s", homeRev.DocumentJson)
				}
				if !strings.Contains(homeRev.DocumentJson, `"block":"core/form","version":2`) {
					t.Fatalf("landing homepage entry must embed core/form@2: %s", homeRev.DocumentJson)
				}
				if !strings.Contains(homepage, `id="contact"`) {
					t.Fatalf("rendered landing homepage missing contact anchor: %s", homepage)
				}
			}
			if strings.Contains(homepage, `class="stratum-collection`) && !strings.Contains(homepage, `stratum-collection--`) {
				t.Fatalf("rendered collection missing presentation classes: %s", homepage)
			}
			active := handler.themes.Current()
			custom, err := handler.queries.GetThemeCustomization(t.Context(), active.ThemeID)
			if err != nil {
				t.Fatalf("theme customization missing: %v", err)
			}
			if custom.ThemeVersion != int64(active.Version) {
				t.Fatalf("theme customization version = %d, want %d", custom.ThemeVersion, active.Version)
			}
			parts, err := handler.queries.ListSiteParts(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			for _, part := range parts {
				revision, err := handler.queries.GetPublishedSitePartRevision(t.Context(), part.ID)
				if err != nil {
					t.Fatal(err)
				}
				if strings.Contains(revision.DocumentJson, `"block":"core/section"`) {
					t.Fatalf("%s site part contains page-level Section spacing", part.Name)
				}
			}
			menu, err := handler.queries.GetNavigationMenuBySlug(t.Context(), "primary-menu")
			if err != nil {
				t.Fatal(err)
			}
			items, err := handler.queries.ListNavigationItemsByMenu(t.Context(), menu.ID)
			if err != nil {
				t.Fatal(err)
			}
			if len(items) != len(expect.menuLabels) {
				t.Fatalf("menu item count = %d, want %d", len(items), len(expect.menuLabels))
			}
			for i, item := range items {
				if item.Position != int64(i) || item.Label != expect.menuLabels[i] {
					t.Fatalf("menu item %d = (%d, %q), want (%d, %q)", i, item.Position, item.Label, i, expect.menuLabels[i])
				}
			}
			iconID, err := handler.queries.GetSiteIconMediaID(t.Context())
			if err != nil || !iconID.Valid {
				t.Fatalf("site icon reference = %#v, %v", iconID, err)
			}
			if preset.ID == creator.PresetLanding {
				entries, err := handler.queries.ListEntriesByContentType(t.Context(), expect.contentType)
				if err != nil {
					t.Fatal(err)
				}
				for _, entry := range entries {
					if entry.PublicPath.Valid {
						t.Fatalf("non-routable testimonial received public path %q", entry.PublicPath.String)
					}
				}
			} else if expect.archivePath != "" && !strings.Contains(requestCreatorPage(t, publicHandler, expect.archivePath), expect.sampleTitle) {
				t.Fatalf("archive %s does not render dynamic entry %q", expect.archivePath, expect.sampleTitle)
			}
			if preset.ID == creator.PresetBlog {
				assertCreatorCollectionPublishing(t, handler, expect, homepage)
			}
		})
	}
}

func newCreatorPublicHandler(t *testing.T, handler *Handler) http.Handler {
	t.Helper()
	publicHandler, err := publicweb.NewHandler(handler.queries, handler.blocks, handler.themes, handler.media)
	if err != nil {
		t.Fatal(err)
	}
	return publicHandler
}

func requestCreatorPage(t *testing.T, handler http.Handler, path string) string {
	t.Helper()
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, path, nil))
	if response.Code != http.StatusOK {
		t.Fatalf("generated path %s status = %d, want 200; body: %s", path, response.Code, response.Body.String())
	}
	return response.Body.String()
}

func assertCreatorCollectionPublishing(t *testing.T, handler *Handler, expect creatorPresetExpectation, before string) {
	t.Helper()
	const title = "Published after onboarding"
	const entryID = "creator-dynamic-entry"
	const revisionID = "creator-dynamic-entry-r1"
	now := time.Now().Unix() + 10
	if err := handler.queries.CreateEntry(t.Context(), db.CreateEntryParams{ID: entryID, ContentTypeID: expect.contentType, Slug: "published-after-onboarding", Status: "active", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := handler.queries.CreateEntryRevision(t.Context(), db.CreateEntryRevisionParams{ID: revisionID, EntryID: entryID, RevisionNumber: 1, Slug: "published-after-onboarding", Title: title, DocumentJson: `{"version":1,"root":[]}`, SchemaMode: "default", FieldsJson: `{}`, CreatedAt: now, Visibility: "public", ReviewState: "draft"}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(before, title) {
		t.Fatalf("draft collection entry %q was visible", title)
	}
	if body := requestCreatorPage(t, newCreatorPublicHandler(t, handler), "/"); strings.Contains(body, title) {
		t.Fatalf("unpublished collection entry %q was visible", title)
	}
	if err := handler.queries.SetPublishedRevision(t.Context(), db.SetPublishedRevisionParams{ID: entryID, PublishedRevisionID: sql.NullString{String: revisionID, Valid: true}, PublishedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if body := requestCreatorPage(t, newCreatorPublicHandler(t, handler), "/"); !strings.Contains(body, title) {
		t.Fatalf("published collection entry %q was not visible", title)
	}
}

func TestCreatorSkipCompletesOnboardingWithoutContent(t *testing.T) {
	handler, authService := newTestHandler(t)
	session, err := authService.Setup(t.Context(), authService.SetupCode(), "Example", "admin@example.com", "a sufficiently long password")
	if err != nil {
		t.Fatal(err)
	}
	form := url.Values{"csrf_token": {"test-csrf"}}
	request := httptest.NewRequest(http.MethodPost, "/admin/creator/skip", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(&http.Cookie{Name: auth.CookieName, Value: session})
	request.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "test-csrf"})
	response := httptest.NewRecorder()

	handler.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusSeeOther || response.Header().Get("Location") != "/admin" {
		t.Fatalf("skip response = %d %q", response.Code, response.Header().Get("Location"))
	}
	count, err := handler.queries.CountEntries(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("skip created %d entries, want 0", count)
	}
}
