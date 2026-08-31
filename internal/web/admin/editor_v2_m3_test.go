package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/auth"
)

func TestV2M3ShellExposesFloatingPanelControls(t *testing.T) {
	handler, authService := newTestHandler(t)
	token, err := authService.Setup(t.Context(), authService.SetupCode(), "Site", "admin@example.com", "correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	entryID := seedPageForV2WithAuth(t, handler, token)
	req := httptest.NewRequest(http.MethodGet, "/admin/pages/"+entryID+"/edit?editor=v2", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "test-token"})
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	for _, want := range []string{
		`id="editor-v2-blocks-btn"`,
		`id="editor-v2-layers-btn"`,
		`id="editor-v2-document-btn"`,
		`aria-controls="editor-v2-panel-left"`,
		`aria-controls="editor-v2-panel-right"`,
		`id="editor-v2-panel-left"`,
		`id="editor-v2-panel-right"`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("V2 M3 shell missing %q", want)
		}
	}
	if strings.Contains(body, `id="admin-sidebar"`) || strings.Contains(body, `editor-v2-sidebar`) {
		t.Fatal("V2 M3 must not render permanent sidebars")
	}

	var bootstrap map[string]any
	if err := json.Unmarshal([]byte(extractV2Bootstrap(t, body)), &bootstrap); err != nil {
		t.Fatalf("decode bootstrap: %v", err)
	}
	catalog, ok := bootstrap["catalog"].([]any)
	if !ok || len(catalog) == 0 {
		t.Fatalf("V2 bootstrap must expose the real block catalog: %#v", bootstrap["catalog"])
	}
	document, ok := bootstrap["document"].(map[string]any)
	if !ok {
		t.Fatalf("V2 bootstrap must expose the SDT document: %#v", bootstrap["document"])
	}
	if nodes, ok := document["nodes"].([]any); !ok || len(nodes) == 0 {
		t.Fatalf("V2 bootstrap SDT must contain the seeded nodes: %#v", document["nodes"])
	}
}

func TestV2M3ModulesAreServed(t *testing.T) {
	handler, authService := newTestHandler(t)
	token, err := authService.Setup(t.Context(), authService.SetupCode(), "Site", "admin@example.com", "correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	for _, path := range []string{
		"/admin/static/editor-v2/panels.js",
		"/admin/static/editor-v2/navigator.js",
		"/admin/static/editor-v2/inspector.js",
	} {
		req := httptest.NewRequest(http.MethodGet, path, nil)
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "test-token"})
		rec := httptest.NewRecorder()
		handler.Routes().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("static %s status=%d want 200", path, rec.Code)
		}
		if rec.Body.Len() == 0 {
			t.Errorf("static %s body is empty", path)
		}
	}
}
