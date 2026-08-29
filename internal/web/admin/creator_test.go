package admin

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/auth"
	"github.com/kokosx/stratum/internal/creator"
	publicweb "github.com/kokosx/stratum/internal/web/public"
)

func TestCreatorBuildsEveryPreset(t *testing.T) {
	for _, preset := range creator.Presets() {
		t.Run(string(preset.ID), func(t *testing.T) {
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
			publicHandler, err := publicweb.NewHandler(handler.queries, handler.blocks, handler.themes, handler.media)
			if err != nil {
				t.Fatal(err)
			}
			publicResponse := httptest.NewRecorder()
			publicHandler.ServeHTTP(publicResponse, httptest.NewRequest(http.MethodGet, "/", nil))
			if publicResponse.Code != http.StatusOK {
				t.Fatalf("generated homepage status = %d, want 200; body: %s", publicResponse.Code, publicResponse.Body.String())
			}
		})
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
