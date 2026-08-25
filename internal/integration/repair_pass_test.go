package integration

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/auth"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

// setupAndLogin creates the administrator and logs in a fresh client.
func setupAndLogin(t *testing.T, serverURL string, authService *auth.Service) *http.Client {
	t.Helper()
	setupClient := newClient(t)
	setupForm := url.Values{
		"setup_code": {authService.SetupCode()},
		"site_title": {"Test Site"},
		"email":      {"admin@example.com"},
		"password":   {"a sufficiently long password"},
		"csrf_token": {csrfToken(t, setupClient, serverURL, "/admin/setup")},
	}
	sr := postForm(t, setupClient, serverURL, "/admin/setup", setupForm)
	sr.Body.Close()

	client := newClient(t)
	loginForm := url.Values{
		"email":      {"admin@example.com"},
		"password":   {"a sufficiently long password"},
		"csrf_token": {csrfToken(t, client, serverURL, "/admin/login")},
	}
	lr := postForm(t, client, serverURL, "/admin/login", loginForm)
	lr.Body.Close()
	return client
}

// docWith builds a single-section document holding the given nodes.
func docWith(nodes string) string {
	return `{"version":1,"nodes":[{"id":"s1","block":"core/section","version":1,"props":{},"settings":{"width":"content","verticalSpacing":"md","horizontalPadding":"md","align":"left","background":"default","minHeight":"auto","anchorID":""},"children":[` + nodes + `]}]}`
}

func clientCookie(c *http.Client, u *url.URL) *http.Cookie {
	for _, cookie := range c.Jar.Cookies(u) {
		if cookie.Name == "stratum_csrf" {
			return cookie
		}
	}
	return &http.Cookie{Name: "stratum_csrf", Value: ""}
}

func previewBody(t *testing.T, serverURL string, _ http.Handler, client *http.Client, csrf, doc, title string) string {
	t.Helper()
	form := url.Values{
		"document_json": {doc},
		"csrf_token":    {csrf},
	}
	if title != "" {
		form.Set("title", title)
	}
	resp, err := client.PostForm(serverURL+"/admin/editor/preview", form)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	return string(body)
}

func serverURLHost(serverURL string) string {
	u, err := url.Parse(serverURL)
	if err != nil {
		return "example.com"
	}
	return u.Host
}

// TestEditorPreviewSharesPublicRenderingPipeline verifies the editor preview and
// the public frontend use the same theme layout, the same stylesheet order
// (blocks.css before theme.css) and the same chrome.
func TestEditorPreviewSharesPublicRenderingPipeline(t *testing.T) {
	server, queries, _, service, cleanup := newIntegrationServer(t)
	defer cleanup()
	client := setupAndLogin(t, server.URL, service)

	createForm := url.Values{
		"title":         {"Preview Parity"},
		"slug":          {"preview-parity"},
		"document_json": {docWith(`{"id":"t1","block":"core/text","version":1,"props":{"text":"public content"},"settings":{"align":"left","tone":"default","size":"md","maxWidth":"none"}}`)},
		"csrf_token":    {csrfToken(t, client, server.URL, "/admin/pages/new")},
	}
	r := postForm(t, client, server.URL, "/admin/pages", createForm)
	r.Body.Close()
	entry, err := queries.GetFlatEntryBySlug(context.Background(), db.GetFlatEntryBySlugParams{ContentTypeID: "page", Slug: "preview-parity"})
	if err != nil {
		t.Fatal(err)
	}
	publishForm := url.Values{
		"title":         {"Preview Parity"},
		"slug":          {"preview-parity"},
		"document_json": {docWith(`{"id":"t1","block":"core/text","version":1,"props":{"text":"public content"},"settings":{"align":"left","tone":"default","size":"md","maxWidth":"none"}}`)},
		"csrf_token":    {csrfToken(t, client, server.URL, "/admin/pages/"+entry.ID+"/edit")},
	}
	pr := postForm(t, client, server.URL, "/admin/pages/"+entry.ID+"/publish", publishForm)
	pr.Body.Close()

	publicBody := bodyString(t, getPath(t, client, server.URL, "/preview-parity"))
	if blocks, theme := strings.Index(publicBody, "/stratum/blocks."), strings.Index(publicBody, "/stratum/theme."); blocks == -1 || theme == -1 || blocks > theme {
		t.Fatalf("public stylesheet order wrong: blocks=%d theme=%d", blocks, theme)
	}

	preview := previewBody(t, server.URL, server.Config.Handler, client, csrfToken(t, client, server.URL, "/admin/pages/"+entry.ID+"/edit"),
		docWith(`{"id":"t1","block":"core/text","version":1,"props":{"text":"preview content"},"settings":{"align":"left","tone":"default","size":"md","maxWidth":"none"}}`), "")
	if pb, pt := strings.Index(preview, "/stratum/blocks."), strings.Index(preview, "/stratum/theme."); pb == -1 || pt == -1 || pb > pt {
		t.Fatalf("editor preview stylesheet order wrong: blocks=%d theme=%d", pb, pt)
	}
	if !strings.Contains(preview, `<main id="content"`) || !strings.Contains(preview, "site-header") {
		t.Fatalf("editor preview missing shared theme chrome")
	}
	if !strings.Contains(preview, "preview content") {
		t.Fatalf("editor preview missing rendered document")
	}
}

// TestEditorPreviewDynamicContext verifies an unsaved title drives dynamic blocks.
func TestEditorPreviewDynamicContext(t *testing.T) {
	server, _, _, service, cleanup := newIntegrationServer(t)
	defer cleanup()
	client := setupAndLogin(t, server.URL, service)
	csrf := csrfToken(t, client, server.URL, "/admin/pages/new")
	body := previewBody(t, server.URL, server.Config.Handler, client, csrf,
		docWith(`{"id":"et","block":"core/entry-title","version":1,"props":{},"settings":{"level":1,"align":"left","visualSize":"auto","tone":"default","maxWidth":"none"}}`), "Draft Title From Editor")
	if !strings.Contains(body, "Draft Title From Editor") {
		t.Fatalf("editor preview did not apply unsaved title to dynamic block: %s", body)
	}
}

// TestEmptyButtonNeverRenders verifies a button with no label/url emits nothing.
func TestEmptyButtonNeverRenders(t *testing.T) {
	server, _, _, service, cleanup := newIntegrationServer(t)
	defer cleanup()
	client := setupAndLogin(t, server.URL, service)
	csrf := csrfToken(t, client, server.URL, "/admin/pages/new")
	body := previewBody(t, server.URL, server.Config.Handler, client, csrf,
		docWith(`{"id":"b","block":"core/button","version":1,"props":{"label":"","url":""},"settings":{"variant":"primary","size":"md","width":"auto","align":"left","openInNewTab":false}}`), "")
	if strings.Contains(body, "stratum-button") {
		t.Fatalf("empty button still rendered a control: %s", body)
	}
}

// TestImageCenterAlignmentVerified confirms the cascade centres a block image.
// Blocks CSS is fingerprinted per page (UsedBlocks), so we publish a page
// containing an image block and fetch its blocks.*.css URL from the HTML.
func TestImageCenterAlignmentVerified(t *testing.T) {
	server, queries, _, service, cleanup := newIntegrationServer(t)
	defer cleanup()
	ctx := context.Background()
	client := setupAndLogin(t, server.URL, service)

	imgDoc := `{"version":1,"nodes":[{"id":"s1","block":"core/section","version":1,"props":{},"settings":{"width":"content","verticalSpacing":"md","horizontalPadding":"md","align":"left","background":"default","minHeight":"auto","anchorID":""},"children":[{"id":"img1","block":"core/image","version":1,"props":{"mediaId":"00000000-0000-0000-0000-000000000000","alt":"test"},"settings":{"align":"center"}}]}]}`

	createForm := url.Values{
		"title":         {"Image Test"},
		"slug":          {"image-test"},
		"document_json": {imgDoc},
		"csrf_token":    {csrfToken(t, client, server.URL, "/admin/pages/new")},
	}
	cr := postForm(t, client, server.URL, "/admin/pages", createForm)
	cr.Body.Close()
	entry, err := queries.GetFlatEntryBySlug(ctx, db.GetFlatEntryBySlugParams{ContentTypeID: "page", Slug: "image-test"})
	if err != nil {
		t.Fatal(err)
	}
	publishForm := url.Values{
		"title":         {"Image Test"},
		"slug":          {"image-test"},
		"document_json": {imgDoc},
		"csrf_token":    {csrfToken(t, client, server.URL, "/admin/pages/"+entry.ID+"/edit")},
	}
	pr := postForm(t, client, server.URL, "/admin/pages/"+entry.ID+"/publish", publishForm)
	pr.Body.Close()

	publicResp := getPath(t, client, server.URL, "/image-test")
	if publicResp.StatusCode != http.StatusOK {
		t.Fatalf("published page status = %d", publicResp.StatusCode)
	}
	html := bodyString(t, publicResp)
	re := regexp.MustCompile(`/stratum/blocks\.[a-f0-9]+\.css`)
	match := re.FindString(html)
	if match == "" {
		t.Fatalf("no fingerprinted blocks CSS found in HTML: %s", html)
	}
	cssResp := getPath(t, client, server.URL, match)
	css := bodyString(t, cssResp)
	if !strings.Contains(css, ".stratum-image-align-center img{margin-inline:auto}") {
		t.Fatalf("image center alignment rule missing from blocks.css: %s", css)
	}
}

// TestDropdownIndicatorCssRule confirms the setting produces a matching rule so
// a disabled indicator truly removes the chevron.
func TestDropdownIndicatorCssRule(t *testing.T) {
	server, _, _, _, cleanup := newIntegrationServer(t)
	defer cleanup()
	client := newClient(t)
	resp := getPath(t, client, server.URL, "/stratum/theme.css")
	if resp.StatusCode == http.StatusFound {
		loc := resp.Header.Get("Location")
		resp.Body.Close()
		resp = getPath(t, client, server.URL, loc)
	}
	css := bodyString(t, resp)
	if !strings.Contains(css, ".st-dropdown-indicator--false .submenu-toggle") {
		t.Fatalf("dropdown indicator CSS rule missing: %s", css)
	}
}
