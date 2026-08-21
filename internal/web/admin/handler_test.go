package admin

import (
	"bytes"
	"context"
	"encoding/json"
	"html/template"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/auth"
	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/media"
	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/themes"
)

func TestPageTemplateSafelyEmbedsEditorBootstrap(t *testing.T) {
	handler, _ := newTestHandler(t)
	encoded, err := json.Marshal(editorBootstrap{
		Document: json.RawMessage(`{"version":1,"nodes":[{"id":"x","block":"core/text","version":1,"props":{"text":"</script><script>alert(1)</script>"}}]}`),
		Catalog:  []any{}, Definitions: []any{}, PreviewURL: "/admin/editor/preview",
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(encoded), "</script>") {
		t.Fatalf("bootstrap JSON contains a script terminator: %s", encoded)
	}
	var output bytes.Buffer
	data := entryFormData{Heading: "Edit", Action: "/admin/pages/x", PublishAction: "/admin/pages/x/publish", EditorJSON: template.JS(encoded), CSRFToken: "token"}
	if err := handler.entryTemplate.ExecuteTemplate(&output, "layout.html", LayoutData{Title: "Edit", Content: data}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(output.String(), "ZgotmplZ") || !strings.Contains(output.String(), `"previewUrl":"/admin/editor/preview"`) {
		t.Fatalf("editor bootstrap was not embedded safely: %s", output.String())
	}
}

func TestSetupRequiresCSRFToken(t *testing.T) {
	handler, service := newTestHandler(t)
	form := url.Values{
		"setup_code": {service.SetupCode()},
		"site_title": {"Example"},
		"email":      {"admin@example.com"},
		"password":   {"a sufficiently long password"},
	}
	request := httptest.NewRequest(http.MethodPost, "/admin/setup", strings.NewReader(form.Encode()))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	response := httptest.NewRecorder()

	handler.Routes().ServeHTTP(response, request)

	if response.Code != http.StatusForbidden {
		t.Fatalf("setup without CSRF status = %d, want %d", response.Code, http.StatusForbidden)
	}
	if hasAdmin, err := service.HasAdmin(context.Background()); err != nil || hasAdmin {
		t.Fatalf("administrator created without CSRF: hasAdmin=%v, err=%v", hasAdmin, err)
	}
}

func TestCSRFTokenReuseAcrossMultipleTabs(t *testing.T) {
	handler, _ := newTestHandler(t)

	tab1Recorder := httptest.NewRecorder()
	tab1Request := httptest.NewRequest(http.MethodGet, "/admin/pages", nil)
	token1, err := handler.csrfToken(tab1Recorder, tab1Request)
	if err != nil {
		t.Fatal(err)
	}
	cookies := tab1Recorder.Result().Cookies()
	if len(cookies) != 1 {
		t.Fatalf("expected 1 cookie, got %d", len(cookies))
	}

	tab2Request := httptest.NewRequest(http.MethodGet, "/admin/pages", nil)
	tab2Request.AddCookie(cookies[0])
	tab2Recorder := httptest.NewRecorder()
	token2, err := handler.csrfToken(tab2Recorder, tab2Request)
	if err != nil {
		t.Fatal(err)
	}

	if token1 != token2 {
		t.Fatalf("second tab got a different token: %q != %q", token1, token2)
	}
	if len(tab2Recorder.Result().Cookies()) != 0 {
		t.Fatal("second tab received a new cookie despite having a valid one")
	}
}

func newTestHandler(t *testing.T) (*Handler, *auth.Service) {
	t.Helper()
	ctx := context.Background()
	database, err := storage.Open(filepath.Join(t.TempDir(), "stratum.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	queries := db.New(database.DB)
	service, err := auth.NewService(database.DB, queries, false)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := blocks.NewRegistry(ctx, queries)
	if err != nil {
		t.Fatal(err)
	}
	themeRuntime, err := themes.NewRuntime(ctx, queries)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := NewHandler(database.DB, queries, service, registry, themeRuntime, newTestMedia(t, queries))
	if err != nil {
		t.Fatal(err)
	}
	return handler, service
}

func newTestMedia(t *testing.T, queries *db.Queries) *media.Service {
	t.Helper()
	store, err := media.NewLocalStorage(filepath.Join(t.TempDir(), "media"))
	if err != nil {
		t.Fatal(err)
	}
	return media.NewService(queries, store)
}
