package admin

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestSetupStartsCreatorWithoutSeedingContent(t *testing.T) {
	handler, service := newTestHandler(t)

	// Boot on a fresh DB loads an empty-but-authoritative snapshot (same as serve).
	if got := handler.runtime.Routes.Count(); got != 0 {
		t.Fatalf("fresh install route snapshot has %d routes, want 0", got)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/admin/setup", nil)
	token, err := handler.csrfToken(recorder, request)
	if err != nil {
		t.Fatal(err)
	}

	form := url.Values{
		"setup_code": {service.SetupCode()},
		"site_title": {"Example"},
		"email":      {"admin@example.com"},
		"password":   {"a sufficiently long password"},
		"csrf_token": {token},
	}
	setupRequest := httptest.NewRequest(http.MethodPost, "/admin/setup", strings.NewReader(form.Encode()))
	setupRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, cookie := range recorder.Result().Cookies() {
		setupRequest.AddCookie(cookie)
	}
	setupResponse := httptest.NewRecorder()
	handler.Routes().ServeHTTP(setupResponse, setupRequest)

	if setupResponse.Code != http.StatusSeeOther {
		t.Fatalf("setup status = %d, want %d", setupResponse.Code, http.StatusSeeOther)
	}
	if location := setupResponse.Header().Get("Location"); location != "/admin/creator" {
		t.Fatalf("setup redirect = %q, want /admin/creator", location)
	}
	if got := handler.runtime.Routes.Count(); got != 0 {
		t.Fatalf("route snapshot has %d routes before Creator runs, want 0", got)
	}
	completed, err := handler.queries.GetOnboardingCompleted(setupRequest.Context())
	if err != nil {
		t.Fatal(err)
	}
	if completed != 0 {
		t.Fatalf("onboarding_completed = %d, want 0", completed)
	}
}
