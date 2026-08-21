package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAppearancePreviewIsTemporaryAndSavePublishes(t *testing.T) {
	handler, _ := newTestHandler(t)
	before := handler.themes.Current().Settings["header.layout"]
	handler.SetPreviewRenderer(func(_ context.Context, path, origin string, settings map[string]any, customCSS string) ([]byte, error) {
		return []byte(path + ":" + origin + ":" + settings["header.layout"].(string) + ":" + customCSS), nil
	})

	previewBody := appearanceRequest{Settings: map[string]any{"header.layout": "stacked"}, CustomCSS: ".preview{}", PreviewPath: "/"}
	preview := appearanceJSONRequest(t, handler, "/admin/appearance/preview", previewBody)
	previewResponse := httptest.NewRecorder()
	handler.previewAppearance(previewResponse, preview)
	if previewResponse.Code != http.StatusOK {
		t.Fatalf("preview = %d %q", previewResponse.Code, previewResponse.Body.String())
	}
	body := previewResponse.Body.String()
	for _, want := range []string{"/:http", ":stacked:.preview{}"} {
		if !strings.Contains(body, want) {
			t.Fatalf("preview body missing %q in %q", want, body)
		}
	}
	if handler.themes.Current().Settings["header.layout"] != before {
		t.Fatal("preview mutated published theme settings")
	}

	save := appearanceJSONRequest(t, handler, "/admin/appearance", previewBody)
	saveResponse := httptest.NewRecorder()
	handler.saveAppearance(saveResponse, save)
	if saveResponse.Code != http.StatusOK {
		t.Fatalf("save = %d %s", saveResponse.Code, saveResponse.Body.String())
	}
	if handler.themes.Current().Settings["header.layout"] != "stacked" || !strings.Contains(handler.themes.Styles(), ".preview{}") {
		t.Fatal("save did not publish settings and custom CSS")
	}
}

func TestAppearancePageRendersWithoutInlineErrorPanel(t *testing.T) {
	handler, _ := newTestHandler(t)
	cookieRecorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/admin/appearance", nil)
	token, err := handler.csrfToken(cookieRecorder, req)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(cookieRecorder.Result().Cookies()[0])
	req.Header.Set("X-CSRF-Token", token)

	out := httptest.NewRecorder()
	handler.appearance(out, req)
	if out.Code != http.StatusOK {
		t.Fatalf("appearance = %d %s", out.Code, out.Body.String())
	}
	body := out.Body.String()
	if strings.Contains(body, "cz-preview__error") {
		t.Fatal("appearance page must not render the full-width inline error panel (use the toast region instead)")
	}
	for _, vp := range []string{"desktop", "tablet", "mobile"} {
		if !strings.Contains(body, `data-viewport="`+vp+`"`) {
			t.Fatalf("appearance page missing %q viewport control", vp)
		}
	}
	if !strings.Contains(body, "admin-toast-region") {
		t.Fatal("appearance page must include the shared toast region for floating toasts")
	}
}

func appearanceJSONRequest(t *testing.T, handler *Handler, path string, payload appearanceRequest) *http.Request {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	cookieRecorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(data))
	request.Header.Set("Content-Type", "application/json")
	token, err := handler.csrfToken(cookieRecorder, request)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-CSRF-Token", token)
	request.AddCookie(cookieRecorder.Result().Cookies()[0])
	return request
}
