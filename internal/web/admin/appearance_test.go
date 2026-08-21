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
	handler.SetPreviewRenderer(func(_ context.Context, path string, settings map[string]any, customCSS string) ([]byte, error) {
		return []byte(path + ":" + settings["header.layout"].(string) + ":" + customCSS), nil
	})

	previewBody := appearanceRequest{Settings: map[string]any{"header.layout": "stacked"}, CustomCSS: ".preview{}", PreviewPath: "/"}
	preview := appearanceJSONRequest(t, handler, "/admin/appearance/preview", previewBody)
	previewResponse := httptest.NewRecorder()
	handler.previewAppearance(previewResponse, preview)
	if previewResponse.Code != http.StatusOK || previewResponse.Body.String() != "/:stacked:.preview{}" {
		t.Fatalf("preview = %d %q", previewResponse.Code, previewResponse.Body.String())
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

func appearanceJSONRequest(t *testing.T, handler *Handler, path string, payload appearanceRequest) *http.Request {
	t.Helper()
	data, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	cookieRecorder := httptest.NewRecorder()
	token, err := handler.csrfToken(cookieRecorder)
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(data))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", token)
	request.AddCookie(cookieRecorder.Result().Cookies()[0])
	return request
}
