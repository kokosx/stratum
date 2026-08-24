package admin

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const wantNoindex = "noindex, nofollow"

func TestAdminResponsesAreNotStored(t *testing.T) {
	handler, _ := newTestHandler(t)

	for _, path := range []string{"/admin", "/admin/login", "/admin/settings/general"} {
		rec := httptest.NewRecorder()
		handler.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if got := rec.Header().Get("Cache-Control"); got != "no-store" {
			t.Fatalf("GET %s Cache-Control = %q, want %q", path, got, "no-store")
		}
	}
}

// TestAdminResponsesCarryNoindexHeader verifies every admin response —
// including auth redirects — is marked noindex, nofollow via X-Robots-Tag.
// robots.txt is a crawler convention, not a security mechanism; the header
// keeps admin URLs out of indexes even when linked publicly.
func TestAdminResponsesCarryNoindexHeader(t *testing.T) {
	handler, _ := newTestHandler(t)

	for _, path := range []string{"/admin", "/admin/pages", "/admin/settings", "/admin/login"} {
		rec := httptest.NewRecorder()
		handler.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if got := rec.Header().Get("X-Robots-Tag"); got != wantNoindex {
			t.Fatalf("GET %s X-Robots-Tag = %q, want %q", path, got, wantNoindex)
		}
	}
}

// TestEditorPreviewNoindexHeader verifies the block editor preview response is
// marked noindex, nofollow and renders the matching robots meta.
func TestEditorPreviewNoindexHeader(t *testing.T) {
	handler, _ := newTestHandler(t)
	response := previewRequest(handler, `{"version":1,"nodes":[{"id":"heading","block":"core/heading","version":1,"props":{"text":"Preview","level":3},"settings":{"align":"center"}}]}`)
	if response.Code != http.StatusOK {
		t.Fatalf("preview status = %d, body = %s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("X-Robots-Tag"); got != wantNoindex {
		t.Fatalf("preview X-Robots-Tag = %q, want %q", got, wantNoindex)
	}
	if body := response.Body.String(); !strings.Contains(body, `name="robots"`) || !strings.Contains(body, "noindex") {
		t.Fatalf("preview HTML should carry a noindex meta robots tag, got:\n%s", body[:min(len(body), 2000)])
	}
}
