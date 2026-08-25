package admin

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// Regression: after completing setup on a fresh install, the seeded starter
// content creates routes. The public frontend treats a loaded route snapshot as
// authoritative, so setup MUST reload routes; otherwise every public page 404s
// until the server restarts.
func TestSetupReloadsRouteSnapshot(t *testing.T) {
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

	if rt, ok := handler.runtime.Routes.Lookup("/"); !ok || rt.RouteType != "entry" {
		t.Fatalf("seeded homepage route missing from route snapshot after setup: %+v ok=%v", rt, ok)
	}
}
