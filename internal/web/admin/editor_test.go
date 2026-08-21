package admin

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func TestPreviewValidatesAndUsesRegistryRenderer(t *testing.T) {
	handler, _ := newTestHandler(t)
	documentJSON := `{"version":1,"nodes":[{"id":"heading","block":"core/heading","version":1,"props":{"text":"Preview","level":3},"settings":{"align":"center"}}]}`
	response := previewRequest(handler, documentJSON)
	if response.Code != http.StatusOK {
		t.Fatalf("preview status = %d, body = %s", response.Code, response.Body.String())
	}
	if body := response.Body.String(); !strings.Contains(body, `stratum-align-center`) || !strings.Contains(body, `Preview`) {
		t.Fatalf("preview body = %q", body)
	}

	invalid := previewRequest(handler, `{"version":1,"nodes":[{"id":"x","block":"missing/block","version":1,"props":{},"settings":{}}]}`)
	if invalid.Code != http.StatusUnprocessableEntity || !strings.Contains(invalid.Body.String(), "unknown block missing/block@1") {
		t.Fatalf("invalid preview status = %d, body = %s", invalid.Code, invalid.Body.String())
	}
}

func previewRequest(handler *Handler, documentJSON string) *httptest.ResponseRecorder {
	form := url.Values{"csrf_token": {"test-token"}, "document_json": {documentJSON}}
	request := httptest.NewRequest(http.MethodPost, "/admin/editor/preview", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	request.AddCookie(&http.Cookie{Name: csrfCookieName, Value: "test-token"})
	response := httptest.NewRecorder()
	handler.previewDocument(response, request)
	return response
}
