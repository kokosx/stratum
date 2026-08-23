package integration

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/auth"
	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/media"
	"github.com/kokosx/stratum/internal/runtimehub"
	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/themes"
	adminweb "github.com/kokosx/stratum/internal/web/admin"
	publicweb "github.com/kokosx/stratum/internal/web/public"
)

func mustJSON(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func doc(text string) string {
	return `{"version":1,"nodes":[{"id":"s1","block":"core/section","version":1,"props":{},"settings":{"width":"content","verticalSpacing":"md","horizontalPadding":"md","align":"left","background":"default","minHeight":"auto","anchorID":""},"children":[{"id":"t1","block":"core/text","version":1,"props":{"text":` + mustJSON(text) + `},"settings":{"align":"left","tone":"default","size":"md","maxWidth":"none"}}]}]}`
}

func newIntegrationServer(t *testing.T) (*httptest.Server, *db.Queries, *storage.Database, *auth.Service, func()) {
	t.Helper()
	ctx := context.Background()
	database, err := storage.Open(filepath.Join(t.TempDir(), "stratum.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := database.Seed(ctx); err != nil {
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
	store, err := media.NewLocalStorage(filepath.Join(t.TempDir(), "media"))
	if err != nil {
		t.Fatal(err)
	}
	mediaService := media.NewService(queries, store)

	hub, err := runtimehub.New(queries, registry, themeRuntime, mediaService)
	if err != nil {
		t.Fatal(err)
	}
	adminHandler, err := adminweb.NewHandler(database.DB, queries, service, registry, themeRuntime, mediaService, hub)
	if err != nil {
		t.Fatal(err)
	}
	publicHandler, err := publicweb.NewHandlerWithHub(hub)
	if err != nil {
		t.Fatal(err)
	}
	adminHandler.SetPreviewRenderer(publicHandler.RenderPreview)
	adminHandler.SetDocumentPreviewRenderer(publicHandler.RenderEditableDocument)

	mux := http.NewServeMux()
	mux.Handle("/admin", adminHandler.Routes())
	mux.Handle("/admin/", adminHandler.Routes())
	mux.Handle("/", publicHandler)

	server := httptest.NewServer(mux)
	return server, queries, database, service, func() {
		server.Close()
		_ = database.Close()
	}
}

func csrfToken(t *testing.T, client *http.Client, serverURL, path string) string {
	t.Helper()
	resp, err := client.Get(serverURL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)
	body := string(bodyBytes)
	re := regexp.MustCompile(`name="csrf_token" value="([^"]+)"`)
	match := re.FindStringSubmatch(body)
	if match == nil {
		t.Fatalf("csrf token not found in %s", path)
	}
	return match[1]
}

func postForm(t *testing.T, client *http.Client, serverURL, path string, form url.Values) *http.Response {
	t.Helper()
	resp, err := client.PostForm(serverURL+path, form)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func getPath(t *testing.T, client *http.Client, serverURL, path string) *http.Response {
	t.Helper()
	resp, err := client.Get(serverURL + path)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func bodyString(t *testing.T, resp *http.Response) string {
	t.Helper()
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return string(b)
}

// TestVerticalSliceSetupToPublish drives the full content lifecycle over HTTP:
// setup -> login -> create draft -> publish -> edit draft (public unchanged) ->
// change slug + republish -> old slug 301s to the new one.
func TestVerticalSliceSetupToPublish(t *testing.T) {
	server, queries, _, authService, cleanup := newIntegrationServer(t)
	defer cleanup()
	ctx := context.Background()

	// 1. Setup (creates the admin and logs it in on its own client).
	setupClient := newClient(t)
	setupToken := authService.SetupCode()
	setupForm := url.Values{
		"setup_code": {setupToken},
		"site_title": {"Test Site"},
		"email":      {"admin@example.com"},
		"password":   {"a sufficiently long password"},
		"csrf_token": {csrfToken(t, setupClient, server.URL, "/admin/setup")},
	}
	setupResp := postForm(t, setupClient, server.URL, "/admin/setup", setupForm)
	if setupResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("setup status = %d, want 303; body=%s", setupResp.StatusCode, bodyString(t, setupResp))
	}
	setupResp.Body.Close()

	// 2. Login with a fresh client so the login step is exercised independently.
	client := newClient(t)
	loginForm := url.Values{
		"email":      {"admin@example.com"},
		"password":   {"a sufficiently long password"},
		"csrf_token": {csrfToken(t, client, server.URL, "/admin/login")},
	}
	loginResp := postForm(t, client, server.URL, "/admin/login", loginForm)
	if loginResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login status = %d, want 303", loginResp.StatusCode)
	}
	loginResp.Body.Close()

	// 3. Create a draft page (no publish).
	createForm := url.Values{
		"title":         {"Hello"},
		"slug":          {"hello"},
		"document_json": {doc("stage one draft")},
		"csrf_token":    {csrfToken(t, client, server.URL, "/admin/pages/new")},
	}
	createResp := postForm(t, client, server.URL, "/admin/pages", createForm)
	if createResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create page status = %d, want 303", createResp.StatusCode)
	}
	createResp.Body.Close()

	entry, err := queries.GetEntryBySlug(ctx, db.GetEntryBySlugParams{ContentTypeID: "page", Slug: "hello"})
	if err != nil {
		t.Fatalf("created entry not found: %v", err)
	}
	if entry.PublishedRevisionID.Valid {
		t.Fatal("newly created page should not be published yet")
	}

	// 4. Public URL of an unpublished page is not found.
	prePublish := getPath(t, client, server.URL, "/hello")
	if prePublish.StatusCode != http.StatusNotFound {
		t.Fatalf("pre-publish /hello status = %d, want 404", prePublish.StatusCode)
	}
	prePublish.Body.Close()

	// 5. Publish the page.
	publishForm := url.Values{
		"title":         {"Hello"},
		"slug":          {"hello"},
		"document_json": {doc("stage one public")},
		"csrf_token":    {csrfToken(t, client, server.URL, "/admin/pages/"+entry.ID+"/edit")},
	}
	publishResp := postForm(t, client, server.URL, "/admin/pages/"+entry.ID+"/publish", publishForm)
	if publishResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("publish status = %d, want 303", publishResp.StatusCode)
	}
	publishResp.Body.Close()

	published := getPath(t, client, server.URL, "/hello")
	if published.StatusCode != http.StatusOK {
		t.Fatalf("published /hello status = %d, want 200", published.StatusCode)
	}
	publishedBody := bodyString(t, published)
	if !strings.Contains(publishedBody, "stage one public") {
		t.Fatalf("published page missing public content: %s", publishedBody)
	}
	if strings.Contains(publishedBody, "stage one draft") {
		t.Fatal("published page leaked the draft content")
	}

	// 6. Save a new draft without publishing; public content must stay the same.
	saveForm := url.Values{
		"title":         {"Hello"},
		"slug":          {"hello"},
		"document_json": {doc("stage two draft")},
		"csrf_token":    {csrfToken(t, client, server.URL, "/admin/pages/"+entry.ID+"/edit")},
	}
	saveResp := postForm(t, client, server.URL, "/admin/pages/"+entry.ID, saveForm)
	if saveResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("save draft status = %d, want 303", saveResp.StatusCode)
	}
	saveResp.Body.Close()

	afterSave := getPath(t, client, server.URL, "/hello")
	if afterSave.StatusCode != http.StatusOK {
		t.Fatalf("after-save /hello status = %d, want 200", afterSave.StatusCode)
	}
	afterSaveBody := bodyString(t, afterSave)
	if !strings.Contains(afterSaveBody, "stage one public") {
		t.Fatal("public content changed after saving a draft")
	}
	if strings.Contains(afterSaveBody, "stage two draft") {
		t.Fatal("draft content leaked into the public page")
	}

	// 7. Republish with a changed slug and new content.
	republishForm := url.Values{
		"title":         {"Hello"},
		"slug":          {"hello-world"},
		"document_json": {doc("stage two public")},
		"csrf_token":    {csrfToken(t, client, server.URL, "/admin/pages/"+entry.ID+"/edit")},
	}
	republishResp := postForm(t, client, server.URL, "/admin/pages/"+entry.ID+"/publish", republishForm)
	if republishResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("republish status = %d, want 303", republishResp.StatusCode)
	}
	republishResp.Body.Close()

	newPublic := getPath(t, client, server.URL, "/hello-world")
	if newPublic.StatusCode != http.StatusOK {
		t.Fatalf("new public /hello-world status = %d, want 200", newPublic.StatusCode)
	}
	newBody := bodyString(t, newPublic)
	if !strings.Contains(newBody, "stage two public") {
		t.Fatal("republished page missing the new public content")
	}

	// 8. The old slug now 301-redirects to the new one.
	redirectClient := newClient(t)
	oldResp := getPath(t, redirectClient, server.URL, "/hello")
	if oldResp.StatusCode != http.StatusMovedPermanently {
		t.Fatalf("old slug status = %d, want 301", oldResp.StatusCode)
	}
	if loc := oldResp.Header.Get("Location"); loc != "/hello-world" {
		t.Fatalf("old slug redirects to %q, want /hello-world", loc)
	}
	oldResp.Body.Close()
}

func newClient(t *testing.T) *http.Client {
	t.Helper()
	jar, err := cookiejar.New(nil)
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Jar: jar}
	client.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		return http.ErrUseLastResponse
	}
	return client
}
