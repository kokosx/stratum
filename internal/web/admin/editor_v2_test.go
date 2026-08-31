package admin

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/auth"
)

func TestV1EditRendersLegacy(t *testing.T) {
	handler, authService := newTestHandler(t)
	token, err := authService.Setup(t.Context(), authService.SetupCode(), "Site", "admin@example.com", "correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	entryID := seedPageForV2WithAuth(t, handler, token)
	req := httptest.NewRequest("GET", "/admin/pages/"+entryID+"/edit", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	csrf := "test-token"
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("V1 edit status = %d, want 200 body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if strings.Contains(body, `editor-v2-app`) {
		t.Fatalf("V1 should not contain V2 shell")
	}
	if !strings.Contains(body, `block-editor`) && !strings.Contains(body, `editor-workbench`) {
		t.Fatalf("V1 body missing legacy editor markers")
	}
	if strings.Contains(body, `editor-v2-canvas`) {
		t.Fatalf("V1 should not contain V2 canvas")
	}
}

func TestV2EditRendersNewShell(t *testing.T) {
	handler, authService := newTestHandler(t)
	token, err := authService.Setup(t.Context(), authService.SetupCode(), "Site", "admin@example.com", "correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	entryID := seedPageForV2WithAuth(t, handler, token)
	req := httptest.NewRequest("GET", "/admin/pages/"+entryID+"/edit?editor=v2", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	csrf := "test-token"
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrf})
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("V2 edit status = %d, want 200 body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `editor-v2-app`) {
		t.Fatalf("V2 missing editor-v2-app")
	}
	if !strings.Contains(body, `editor-v2-canvas`) {
		t.Fatalf("V2 missing canvas iframe")
	}
	if !strings.Contains(body, `sandbox="allow-same-origin"`) {
		t.Fatalf("V2 iframe missing safe sandbox")
	}
	if strings.Contains(body, `allow-scripts`) {
		t.Fatalf("V2 iframe must not contain allow-scripts")
	}
	if !strings.Contains(body, `editor-v2/editor.css`) {
		t.Fatalf("V2 missing editor-v2 CSS")
	}
	if !strings.Contains(body, `editor-v2/app.js`) {
		t.Fatalf("V2 missing app.js")
	}
	if !strings.Contains(body, `editor-v2-bootstrap`) {
		t.Fatalf("V2 missing bootstrap")
	}
	if !strings.Contains(body, `Loading preview`) {
		t.Fatalf("V2 missing loading state")
	}
	if strings.Contains(body, `id="admin-sidebar"`) {
		t.Fatalf("V2 should not contain admin sidebar")
	}
	if !strings.Contains(body, `data-viewport="desktop"`) || !strings.Contains(body, `data-viewport="tablet"`) || !strings.Contains(body, `data-viewport="mobile"`) {
		t.Fatalf("V2 missing viewport controls")
	}
	if !strings.Contains(body, `aria-label="Desktop"`) || !strings.Contains(body, `aria-label="Tablet"`) || !strings.Contains(body, `aria-label="Mobile"`) {
		t.Fatalf("V2 viewport buttons missing aria-label")
	}
	bootstrapJSON := extractV2Bootstrap(t, body)
	var boot map[string]any
	if err := json.Unmarshal([]byte(bootstrapJSON), &boot); err != nil {
		t.Fatalf("bootstrap json unmarshal: %v body=%s", err, bootstrapJSON)
	}
	res, ok := boot["resource"].(map[string]any)
	if !ok {
		t.Fatalf("bootstrap missing resource: %v", boot)
	}
	if res["type"] != "entry" {
		t.Fatalf("resource.type = %v want entry", res["type"])
	}
	if res["id"] != entryID {
		t.Fatalf("resource.id = %v want %s", res["id"], entryID)
	}
	previewUrl, _ := boot["previewUrl"].(string)
	if previewUrl == "" {
		if actions, ok := boot["actions"].(map[string]any); ok {
			previewUrl, _ = actions["previewUrl"].(string)
		}
	}
	if previewUrl == "" {
		t.Fatalf("bootstrap missing previewUrl/actions.previewUrl keys=%v", keys(boot))
	}
	if previewUrl != "/admin/editor/preview" {
		t.Fatalf("previewUrl = %q want /admin/editor/preview", previewUrl)
	}
}

func TestV2EditForPosts(t *testing.T) {
	handler, authService := newTestHandler(t)
	token, err := authService.Setup(t.Context(), authService.SetupCode(), "Site", "admin@example.com", "correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	entryID := seedPostForV2WithAuth(t, handler, token)
	req := httptest.NewRequest("GET", "/admin/posts/"+entryID+"/edit?editor=v2", nil)
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "test-token"})
	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("V2 post edit status = %d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, `editor-v2-app`) {
		t.Fatalf("V2 post missing shell")
	}
	bootstrapJSON := extractV2Bootstrap(t, body)
	var boot map[string]any
	_ = json.Unmarshal([]byte(bootstrapJSON), &boot)
	if res, ok := boot["resource"].(map[string]any); ok {
		if res["id"] != entryID {
			t.Fatalf("post resource.id mismatch got %v want %s", res["id"], entryID)
		}
	} else {
		t.Fatalf("post bootstrap missing resource")
	}
}

func TestV2StaticAssetsServed(t *testing.T) {
	handler, authService := newTestHandler(t)
	token, err := authService.Setup(t.Context(), authService.SetupCode(), "Site", "admin@example.com", "correct-horse-battery-staple")
	if err != nil {
		t.Fatalf("setup: %v", err)
	}
	for _, path := range []string{"/admin/static/editor-v2/app.js", "/admin/static/editor-v2/state.js", "/admin/static/editor-v2/preview.js", "/admin/static/editor-v2/editor.css"} {
		req := httptest.NewRequest("GET", path, nil)
		req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
		req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "test-token"})
		rec := httptest.NewRecorder()
		handler.Routes().ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Fatalf("static %s status=%d want 200", path, rec.Code)
		}
		if rec.Body.Len() == 0 {
			t.Fatalf("static %s body empty", path)
		}
	}
}

func TestV2PreviewEndpointWorks(t *testing.T) {
	handler, _ := newTestHandler(t)
	if handler.editorV2Template == nil {
		t.Fatalf("editorV2Template nil")
	}
	doc := `{"version":1,"nodes":[{"id":"n1","block":"core/heading","version":1,"props":{"text":"Hello V2","level":1},"settings":{"align":"left"}}]}`
	form := url.Values{"csrf_token": {"test-token"}, "document_json": {doc}}
	req := httptest.NewRequest(http.MethodPost, "/admin/editor/preview", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "test-token"})
	rec := httptest.NewRecorder()
	handler.previewDocument(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview status=%d body=%s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Hello V2") {
		t.Fatalf("preview body missing content: %s", body[:min(2000, len(body))])
	}
}

// helpers

func seedPageForV2WithAuth(t *testing.T, handler *Handler, token string) string {
	t.Helper()
	user, err := handler.auth.UserForToken(t.Context(), token)
	if err != nil {
		t.Fatalf("userForToken: %v", err)
	}
	largeText := strings.Repeat("Lorem ipsum dolor sit amet, consectetur adipiscing elit. ", 100)
	doc := `{"version":1,"nodes":[{"id":"h1","block":"core/heading","version":1,"props":{"text":"V2 Test Page","level":1},"settings":{"align":"left"}},{"id":"s1","block":"core/section","version":1,"props":{},"settings":{"width":"full"},"children":[{"id":"t1","block":"core/text","version":1,"props":{"text":"` + largeText + `"},"settings":{}}]}]}`
	entryID, _ := randomID()
	input := entryInput{title: "V2 Test Page", slug: "v2-test-" + entryID[:6], documentJSON: doc}
	if err := handler.writeEntry(t.Context(), "page", user.ID, entryID, input, true, true); err != nil {
		t.Fatalf("writeEntry page: %v", err)
	}
	return entryID
}

func seedPostForV2WithAuth(t *testing.T, handler *Handler, token string) string {
	t.Helper()
	user, err := handler.auth.UserForToken(t.Context(), token)
	if err != nil {
		t.Fatalf("userForToken: %v", err)
	}
	doc := `{"version":1,"nodes":[{"id":"h1","block":"core/heading","version":1,"props":{"text":"V2 Post","level":1},"settings":{"align":"left"}}]}`
	entryID, _ := randomID()
	input := entryInput{title: "V2 Post", slug: "v2-post-" + entryID[:6], documentJSON: doc}
	if err := handler.writeEntry(t.Context(), "post", user.ID, entryID, input, true, true); err != nil {
		t.Fatalf("writeEntry post: %v", err)
	}
	return entryID
}

func extractV2Bootstrap(t *testing.T, body string) string {
	t.Helper()
	start := strings.Index(body, `id="editor-v2-bootstrap"`)
	if start == -1 {
		t.Fatalf("bootstrap not found in body")
	}
	gt := strings.Index(body[start:], ">")
	if gt == -1 {
		t.Fatalf("bootstrap malformed")
	}
	contentStart := start + gt + 1
	end := strings.Index(body[contentStart:], "</script>")
	if end == -1 {
		t.Fatalf("bootstrap end not found")
	}
	return strings.TrimSpace(body[contentStart : contentStart+end])
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func keys(m map[string]any) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

var _ = auth.CookieName
