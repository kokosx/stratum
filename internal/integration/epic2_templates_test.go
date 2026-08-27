package integration

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func TestEpic2ArchiveTemplatePreviewAndPublicContext(t *testing.T) {
	server, _, _, authService, _, cleanup := newProductTestServer(t)
	defer cleanup()
	setupClient := newClient(t)
	resp := postForm(t, setupClient, server.URL, "/admin/setup", url.Values{
		"setup_code": {authService.SetupCode()}, "site_title": {"Epic 2"}, "email": {"epic2@example.com"}, "password": {"a sufficiently long password"}, "csrf_token": {csrfToken(t, setupClient, server.URL, "/admin/setup")},
	})
	resp.Body.Close()
	client := newClient(t)
	resp = postForm(t, client, server.URL, "/admin/login", url.Values{"email": {"epic2@example.com"}, "password": {"a sufficiently long password"}, "csrf_token": {csrfToken(t, client, server.URL, "/admin/login")}})
	resp.Body.Close()

	resp = postForm(t, client, server.URL, "/admin/settings/content-types", url.Values{
		"id": {"product"}, "name": {"Product"}, "plural_name": {"Products"}, "base_path": {"/products"}, "public": {"on"}, "archive": {"on"}, "csrf_token": {csrfToken(t, client, server.URL, "/admin/settings/content-types/new")},
	})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create content type: %d %s", resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
	for _, product := range []struct{ title, slug string }{{"Alpha", "alpha"}, {"Beta", "beta"}} {
		resp = postForm(t, client, server.URL, "/admin/content/product", url.Values{
			"title": {product.title}, "slug": {product.slug}, "document_json": {`{"version":1,"nodes":[]}`}, "publish": {"true"}, "csrf_token": {csrfToken(t, client, server.URL, "/admin/content/product/new")},
		})
		if resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("create product: %d %s", resp.StatusCode, bodyString(t, resp))
		}
		resp.Body.Close()
	}

	resp = postForm(t, client, server.URL, "/admin/appearance/templates", url.Values{
		"name": {"Products Archive"}, "content_type_id": {"product"}, "kind": {"archive"}, "csrf_token": {csrfToken(t, client, server.URL, "/admin/appearance/templates/new")},
	})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create archive template: %d %s", resp.StatusCode, bodyString(t, resp))
	}
	match := regexp.MustCompile(`/admin/appearance/templates/([^/]+)/edit`).FindStringSubmatch(resp.Header.Get("Location"))
	resp.Body.Close()
	if len(match) != 2 {
		t.Fatal("archive template id missing")
	}
	templateID := match[1]
	doc := `{"version":1,"nodes":[{"id":"title","block":"core/archive-title","version":1,"props":{},"settings":{}},{"id":"collection","block":"core/collection","version":2,"props":{},"settings":{"source":"context","contentType":"post","limit":1},"children":[{"id":"entry-title","block":"core/entry-title","version":1,"props":{},"settings":{}}]}]}`
	csrf := csrfToken(t, client, server.URL, "/admin/appearance/templates/"+templateID+"/edit")
	resp = postForm(t, client, server.URL, "/admin/appearance/templates/"+templateID+"/preview", url.Values{"document_json": {doc}, "csrf_token": {csrf}})
	preview := bodyString(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(preview, "Alpha") || !strings.Contains(preview, "Beta") || strings.Contains(preview, "where the entry content will appear") {
		t.Fatalf("archive preview incorrect (%d): %s", resp.StatusCode, preview)
	}

	resp = postForm(t, client, server.URL, "/admin/appearance/templates/"+templateID+"/publish", url.Values{"name": {"Products Archive"}, "document_json": {doc}, "csrf_token": {csrfToken(t, client, server.URL, "/admin/appearance/templates/"+templateID+"/edit")}})
	resp.Body.Close()
	resp = postForm(t, client, server.URL, "/admin/appearance/templates/"+templateID+"/default-archive", url.Values{"csrf_token": {csrfToken(t, client, server.URL, "/admin/appearance/templates/"+templateID+"/edit")}})
	resp.Body.Close()
	resp = getPath(t, client, server.URL, "/products")
	publicBody := bodyString(t, resp)
	if resp.StatusCode != http.StatusOK || !strings.Contains(publicBody, "Alpha") || !strings.Contains(publicBody, "Beta") || strings.Contains(publicBody, ">Archive title<") {
		t.Fatalf("archive public output incorrect (%d): %s", resp.StatusCode, publicBody)
	}
}

func TestCreateSitePartPOSTFromNewRouteIsAccepted(t *testing.T) {
	server, _, _, authService, cleanup := newIntegrationServer(t)
	defer cleanup()
	client := setupAndLogin(t, server.URL, authService)
	for index, endpoint := range []string{"/admin/appearance/site-parts/new", "/admin/appearance/site-parts"} {
		resp := postForm(t, client, server.URL, endpoint, url.Values{
			"name":       {"Header " + string(rune('A'+index))},
			"location":   {"header"},
			"csrf_token": {csrfToken(t, client, server.URL, "/admin/appearance/site-parts/new?location=header")},
		})
		if resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("POST %s returned %d, want 303; body: %s", endpoint, resp.StatusCode, bodyString(t, resp))
		}
		resp.Body.Close()
	}
}
