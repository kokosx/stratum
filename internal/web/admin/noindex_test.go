package admin

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestAdminResponsesAreNoindex verifies that the admin UI itself is never
// indexed: every /admin response carries X-Robots-Tag: noindex, nofollow.
// robots.txt is not an indexing or security guarantee, so the header must be
// unconditional.
func TestAdminResponsesAreNoindex(t *testing.T) {
	handler, _ := newTestHandler(t)

	rec := httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/login", nil))
	if got := rec.Header().Get("X-Robots-Tag"); got != "noindex, nofollow" {
		t.Fatalf("GET /admin/login X-Robots-Tag = %q, want %q", got, "noindex, nofollow")
	}

	rec = httptest.NewRecorder()
	handler.Routes().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/admin/pages", nil))
	if got := rec.Header().Get("X-Robots-Tag"); got != "noindex, nofollow" {
		t.Fatalf("GET /admin/pages X-Robots-Tag = %q, want %q", got, "noindex, nofollow")
	}
}

// TestEditorPreviewIsNoindex verifies the editor preview fragment endpoint
// sends its own noindex header (defense in depth on top of the admin wrapper).
func TestEditorPreviewIsNoindex(t *testing.T) {
	handler, _ := newTestHandler(t)
	response := previewRequest(handler, `{"version":1,"nodes":[]}`)
	if response.Code != http.StatusOK {
		t.Fatalf("preview status = %d, body = %s", response.Code, response.Body.String())
	}
	if got := response.Header().Get("X-Robots-Tag"); got != "noindex, nofollow" {
		t.Fatalf("preview X-Robots-Tag = %q, want %q", got, "noindex, nofollow")
	}
}
