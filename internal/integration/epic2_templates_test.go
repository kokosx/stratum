package integration

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/kokosx/stratum/internal/content"
	"github.com/kokosx/stratum/internal/contenttypes"
	"github.com/kokosx/stratum/internal/navigation"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

// Helpers for EPIC2 E2E suite

func epic2CreateContentType(t *testing.T, database *sql.DB, queries *db.Queries, hub interface{ ReloadRoutes(context.Context) error }, id, name, plural, basePath string, single, archive, hasContent bool, fields []content.FieldDefinition) {
	t.Helper()
	svc := contenttypes.New(database, queries)
	in := content.ContentTypeInput{
		ID:         content.ContentTypeID(id),
		Name:       name,
		PluralName: plural,
		Config: content.ContentTypeConfig{
			SchemaVersion: 2,
			Fields:        fields,
			Features:      content.ContentTypeFeatures{Content: hasContent},
			Routing:       content.ContentTypeRouting{Single: single, BasePath: basePath, Archive: archive},
		},
	}
	if err := svc.Create(context.Background(), in); err != nil {
		t.Fatalf("create content type %s: %v", id, err)
	}
	if hub != nil {
		_ = hub.ReloadRoutes(context.Background())
	}
}

func epic2CreateTemplate(t *testing.T, client *http.Client, serverURL, name, ctID, kind string) string {
	t.Helper()
	csrf := csrfToken(t, client, serverURL, "/admin/appearance/templates/new")
	resp := postForm(t, client, serverURL, "/admin/appearance/templates", url.Values{
		"name": {" " + name + " "}, "content_type_id": {ctID}, "kind": {kind}, "csrf_token": {csrf},
	})
	// handler trims name
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create template %s (%s): %d %s", name, ctID, resp.StatusCode, bodyString(t, resp))
	}
	loc := resp.Header.Get("Location")
	resp.Body.Close()
	m := regexp.MustCompile(`/admin/appearance/templates/([^/]+)/edit`).FindStringSubmatch(loc)
	if len(m) != 2 {
		t.Fatalf("template id missing from %q", loc)
	}
	return m[1]
}

func epic2SaveTemplateDraft(t *testing.T, client *http.Client, serverURL, tmplID, name, doc string) {
	t.Helper()
	csrf := csrfToken(t, client, serverURL, "/admin/appearance/templates/"+tmplID+"/edit")
	resp := postForm(t, client, serverURL, "/admin/appearance/templates/"+tmplID, url.Values{
		"name": {name}, "document_json": {doc}, "csrf_token": {csrf},
	})
	if resp.StatusCode != http.StatusSeeOther {
		// datastar returns 200 SSE for editor; but our post is non-datastar, expect 303
		t.Fatalf("save template draft %s: %d %s", tmplID, resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
}

func epic2PublishTemplate(t *testing.T, client *http.Client, serverURL, tmplID, name, doc string) {
	t.Helper()
	csrf := csrfToken(t, client, serverURL, "/admin/appearance/templates/"+tmplID+"/edit")
	resp := postForm(t, client, serverURL, "/admin/appearance/templates/"+tmplID+"/publish", url.Values{
		"name": {name}, "document_json": {doc}, "csrf_token": {csrf},
	})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("publish template %s: %d %s", tmplID, resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
}

func epic2SetDefaultTemplate(t *testing.T, client *http.Client, serverURL, tmplID string) {
	t.Helper()
	csrf := csrfToken(t, client, serverURL, "/admin/appearance/templates/"+tmplID+"/edit")
	resp := postForm(t, client, serverURL, "/admin/appearance/templates/"+tmplID+"/default", url.Values{"csrf_token": {csrf}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("set default %s: %d %s", tmplID, resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
}

func epic2ClearDefaultTemplate(t *testing.T, client *http.Client, serverURL, tmplID string) {
	t.Helper()
	csrf := csrfToken(t, client, serverURL, "/admin/appearance/templates/"+tmplID+"/edit")
	resp := postForm(t, client, serverURL, "/admin/appearance/templates/"+tmplID+"/clear-default", url.Values{"csrf_token": {csrf}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("clear default %s: %d %s", tmplID, resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
}

func epic2SetDefaultArchiveTemplate(t *testing.T, client *http.Client, serverURL, tmplID string) {
	t.Helper()
	csrf := csrfToken(t, client, serverURL, "/admin/appearance/templates/"+tmplID+"/edit")
	resp := postForm(t, client, serverURL, "/admin/appearance/templates/"+tmplID+"/default-archive", url.Values{"csrf_token": {csrf}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("set default archive %s: %d %s", tmplID, resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
}

func epic2CreateSitePart(t *testing.T, client *http.Client, serverURL, name, location string) string {
	t.Helper()
	csrf := csrfToken(t, client, serverURL, "/admin/appearance/site-parts/new")
	resp := postForm(t, client, serverURL, "/admin/appearance/site-parts/new", url.Values{
		"name": {name}, "location": {location}, "csrf_token": {csrf},
	})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create site part %s: %d %s", name, resp.StatusCode, bodyString(t, resp))
	}
	loc := resp.Header.Get("Location")
	resp.Body.Close()
	m := regexp.MustCompile(`/admin/appearance/site-parts/([^/]+)/edit`).FindStringSubmatch(loc)
	if len(m) != 2 {
		t.Fatalf("site part id missing %q", loc)
	}
	return m[1]
}

func epic2SaveSitePartDraft(t *testing.T, client *http.Client, serverURL, partID, name, doc string) {
	t.Helper()
	csrf := csrfToken(t, client, serverURL, "/admin/appearance/site-parts/"+partID+"/edit")
	resp := postForm(t, client, serverURL, "/admin/appearance/site-parts/"+partID, url.Values{
		"name": {name}, "document_json": {doc}, "csrf_token": {csrf},
	})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("save site part draft %s: %d %s", partID, resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
}

func epic2PublishSitePart(t *testing.T, client *http.Client, serverURL, partID, name, doc string) {
	t.Helper()
	csrf := csrfToken(t, client, serverURL, "/admin/appearance/site-parts/"+partID+"/edit")
	resp := postForm(t, client, serverURL, "/admin/appearance/site-parts/"+partID+"/publish", url.Values{
		"name": {name}, "document_json": {doc}, "csrf_token": {csrf},
	})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("publish site part %s: %d %s", partID, resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
}

func epic2AssignSitePart(t *testing.T, client *http.Client, serverURL, location, partID string) {
	t.Helper()
	csrf := csrfToken(t, client, serverURL, "/admin/appearance/site-parts")
	resp := postForm(t, client, serverURL, "/admin/appearance/site-parts/location", url.Values{
		"location": {" " + location + " "}, "site_part_id": {partID}, "csrf_token": {csrf},
	})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("assign site part %s to %s: %d %s", partID, location, resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
}

func epic2ClearSitePartLocation(t *testing.T, client *http.Client, serverURL, location string) {
	t.Helper()
	csrf := csrfToken(t, client, serverURL, "/admin/appearance/site-parts")
	resp := postForm(t, client, serverURL, "/admin/appearance/site-parts/location/clear", url.Values{
		"location": {location}, "csrf_token": {csrf},
	})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("clear site part %s: %d %s", location, resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
}

func epic2CreateAndPublishPage(t *testing.T, client *http.Client, serverURL, title, slug, doc string) string {
	t.Helper()
	csrf := csrfToken(t, client, serverURL, "/admin/pages/new")
	resp := postForm(t, client, serverURL, "/admin/pages", url.Values{
		"title": {title}, "slug": {slug}, "document_json": {doc}, "publish": {"true"}, "csrf_token": {csrf},
	})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create page %s: %d %s", title, resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
	return slug
}

func epic2CreateAndPublishCustom(t *testing.T, client *http.Client, serverURL, ctID, title, slug, doc string, fields map[string]string, layoutTemplateID string) string {
	t.Helper()
	csrf := csrfToken(t, client, serverURL, "/admin/content/"+ctID+"/new")
	vals := url.Values{
		"title": {title}, "slug": {slug}, "document_json": {doc}, "publish": {"true"}, "csrf_token": {csrf},
	}
	if layoutTemplateID != "" {
		vals.Set("layout_template_id", layoutTemplateID)
	}
	for k, v := range fields {
		vals.Set("field_"+k, v)
		if v != "" {
			// for boolean/present handling, ensure present flag for checkbox types but not needed
		}
	}
	resp := postForm(t, client, serverURL, "/admin/content/"+ctID, vals)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create custom %s %s: %d %s", ctID, title, resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
	return slug
}

func epic2PublishCustom(t *testing.T, client *http.Client, serverURL, ctID, entryID, title, slug, doc string, fields map[string]string, layoutTemplateID string) {
	t.Helper()
	csrf := csrfToken(t, client, serverURL, "/admin/content/"+ctID+"/"+entryID+"/edit")
	vals := url.Values{
		"title": {title}, "slug": {slug}, "document_json": {doc}, "csrf_token": {csrf},
	}
	if layoutTemplateID != "" {
		vals.Set("layout_template_id", layoutTemplateID)
	}
	for k, v := range fields {
		vals.Set("field_"+k, v)
	}
	resp := postForm(t, client, serverURL, "/admin/content/"+ctID+"/"+entryID+"/publish", vals)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("publish custom %s %s: %d %s", ctID, entryID, resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
}

func epic2SaveCustomDraft(t *testing.T, client *http.Client, serverURL, ctID, entryID, title, slug, doc string, fields map[string]string) {
	t.Helper()
	csrf := csrfToken(t, client, serverURL, "/admin/content/"+ctID+"/"+entryID+"/edit")
	vals := url.Values{
		"title": {title}, "slug": {slug}, "document_json": {doc}, "csrf_token": {csrf},
	}
	for k, v := range fields {
		vals.Set("field_"+k, v)
	}
	resp := postForm(t, client, serverURL, "/admin/content/"+ctID+"/"+entryID, vals)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("save draft custom %s %s: %d %s", ctID, entryID, resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
}

func textDoc(text string) string {
	return `{"version":1,"nodes":[{"id":"s1","block":"core/section","version":1,"props":{},"settings":{"width":"content","verticalSpacing":"md","horizontalPadding":"md","align":"left","background":"default","minHeight":"auto","anchorID":""},"children":[{"id":"t1","block":"core/text","version":1,"props":{"text":` + mustJSON(text) + `},"settings":{"align":"left","tone":"default","size":"md","maxWidth":"none"}}]}]}`
}

func sectionWithTexts(texts ...string) string {
	children := ""
	for i, txt := range texts {
		if i > 0 {
			children += ","
		}
		children += `{"id":"t` + fmt.Sprint(i) + `","block":"core/text","version":1,"props":{"text":` + mustJSON(txt) + `},"settings":{}}`
	}
	return `{"version":1,"nodes":[{"id":"sec","block":"core/section","version":1,"props":{},"settings":{},"children":[` + children + `]}]}`
}

func sitePartDoc(text string) string {
	return `{"version":1,"nodes":[{"id":"sec","block":"core/section","version":1,"props":{},"settings":{},"children":[{"id":"t1","block":"core/text","version":1,"props":{"text":` + mustJSON(text) + `},"settings":{}}]}]}`
}

func sitePartRefDoc(partID string) string {
	return `{"version":1,"nodes":[{"id":"ref","block":"core/site-part","version":1,"props":{},"settings":{"sitePartId":` + mustJSON(partID) + `}}]}`
}

func compositeSitePartDoc(ownText, refID string) string {
	return `{"version":1,"nodes":[{"id":"sec","block":"core/section","version":1,"props":{},"settings":{},"children":[{"id":"t1","block":"core/text","version":1,"props":{"text":` + mustJSON(ownText) + `},"settings":{}},{"id":"ref","block":"core/site-part","version":1,"props":{},"settings":{"sitePartId":` + mustJSON(refID) + `}}]}]}`
}

func templateWithSlot(wrapperText string) string {
	return `{"version":1,"nodes":[{"id":"sec","block":"core/section","version":1,"props":{},"settings":{},"children":[{"id":"head","block":"core/text","version":1,"props":{"text":` + mustJSON(wrapperText) + `},"settings":{}},{"id":"slot","block":"core/content-slot","version":1,"props":{},"settings":{}}]}]}`
}

func archiveTemplateDoc() string {
	return `{"version":1,"nodes":[{"id":"title","block":"core/archive-title","version":1,"props":{},"settings":{}},{"id":"collection","block":"core/collection","version":2,"props":{},"settings":{"source":"context","limit":10},"children":[{"id":"entry-title","block":"core/entry-title","version":1,"props":{},"settings":{}}]}]}`
}

func entryTitleTemplateDoc(wrapper string) string {
	return `{"version":1,"nodes":[{"id":"sec","block":"core/section","version":1,"props":{},"settings":{},"children":[{"id":"head","block":"core/text","version":1,"props":{"text":` + mustJSON(wrapper) + `},"settings":{}},{"id":"title","block":"core/entry-title","version":1,"props":{},"settings":{}},{"id":"slot","block":"core/content-slot","version":1,"props":{},"settings":{}}]}]}`
}

func fieldsOnlyTemplateDoc() string {
	// No content-slot: fields-only single
	return `{"version":1,"nodes":[{"id":"sec","block":"core/section","version":1,"props":{},"settings":{},"children":[{"id":"et","block":"core/entry-title","version":1,"props":{},"settings":{}},{"id":"pf","block":"core/entry-field","version":1,"props":{"source":"fields.position"},"settings":{"tag":"p"}},{"id":"bf","block":"core/entry-field","version":1,"props":{"source":"fields.bio"},"settings":{"tag":"p"}}]}]}`
}

func headerPartDoc(text string) string {
	return sitePartDoc(text)
}

func navigationHeaderDoc() string {
	return `{"version":1,"nodes":[{"id":"sec","block":"core/section","version":1,"props":{},"settings":{},"children":[{"id":"nav","block":"core/navigation","version":1,"props":{},"settings":{"location":"primary","style":"horizontal"}}]}]}`
}

func assertContains(t *testing.T, body, needle string) {
	t.Helper()
	if !strings.Contains(body, needle) {
		t.Fatalf("expected body to contain %q, got: %s", needle, body[:minInt(len(body), 3000)])
	}
}

func assertNotContains(t *testing.T, body, needle string) {
	t.Helper()
	if strings.Contains(body, needle) {
		t.Fatalf("expected body NOT to contain %q", needle)
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

// Shared archive preview check from original file
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

// 20 — SINGLE DEFAULT TEMPLATE
func TestEpic2SingleDefaultTemplate(t *testing.T) {
	server2, _, database2, authService2, hub2, cleanup2 := newProductTestServer(t)
	defer cleanup2()
	client2 := setupAndLogin(t, server2.URL, authService2)
	epic2CreateContentType(t, database2.DB, db.New(database2.DB), hub2, "product", "Product", "Products", "/products", true, true, true, nil)
	// Create two products
	for _, p := range []struct{ title, slug, body string }{{"Alpha", "alpha", "ALPHA BODY"}, {"Beta", "beta", "BETA BODY"}} {
		epic2CreateAndPublishCustom(t, client2, server2.URL, "product", p.title, p.slug, textDoc(p.body), nil, "")
	}
	// Template A
	tmplA := epic2CreateTemplate(t, client2, server2.URL, "Template A", "product", "single")
	docA := templateWithSlot("TEMPLATE A")
	epic2SaveTemplateDraft(t, client2, server2.URL, tmplA, "Template A", docA)
	epic2PublishTemplate(t, client2, server2.URL, tmplA, "Template A", docA)
	epic2SetDefaultTemplate(t, client2, server2.URL, tmplA)
	// Verify both products show TEMPLATE A + correct body
	for _, p := range []struct{ slug, body string }{{"alpha", "ALPHA BODY"}, {"beta", "BETA BODY"}} {
		resp := getPath(t, client2, server2.URL, "/products/"+p.slug)
		body := bodyString(t, resp)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET product %s status %d", p.slug, resp.StatusCode)
		}
		assertContains(t, body, "TEMPLATE A")
		assertContains(t, body, p.body)
	}
	// Save draft TEMPLATE DRAFT not published
	docDraft := templateWithSlot("TEMPLATE DRAFT")
	epic2SaveTemplateDraft(t, client2, server2.URL, tmplA, "Template A", docDraft)
	resp := getPath(t, client2, server2.URL, "/products/alpha")
	body := bodyString(t, resp)
	assertContains(t, body, "TEMPLATE A")
	assertNotContains(t, body, "TEMPLATE DRAFT")
	// Publish draft
	epic2PublishTemplate(t, client2, server2.URL, tmplA, "Template A", docDraft)
	resp = getPath(t, client2, server2.URL, "/products/beta")
	body = bodyString(t, resp)
	assertContains(t, body, "TEMPLATE DRAFT")
	assertNotContains(t, body, "TEMPLATE A")
}

// 21 — EXPLICIT TEMPLATE OVERRIDE
func TestEpic2ExplicitTemplateOverride(t *testing.T) {
	server, _, database, authService, hub, cleanup := newProductTestServer(t)
	defer cleanup()
	client := setupAndLogin(t, server.URL, authService)
	epic2CreateContentType(t, database.DB, db.New(database.DB), hub, "product", "Product", "Products", "/products", true, true, true, nil)
	// Create products
	epic2CreateAndPublishCustom(t, client, server.URL, "product", "Alpha", "alpha", textDoc("ALPHA BODY"), nil, "")
	epic2CreateAndPublishCustom(t, client, server.URL, "product", "Beta", "beta", textDoc("BETA BODY"), nil, "")
	queries := db.New(database.DB)
	// Find IDs
	alphaRow, _ := queries.GetFlatEntryBySlug(context.Background(), db.GetFlatEntryBySlugParams{ContentTypeID: "product", Slug: "alpha"})
	betaRow, _ := queries.GetFlatEntryBySlug(context.Background(), db.GetFlatEntryBySlugParams{ContentTypeID: "product", Slug: "beta"})
	// Template A default
	tmplA := epic2CreateTemplate(t, client, server.URL, "Template A", "product", "single")
	docA := templateWithSlot("TEMPLATE A")
	epic2PublishTemplate(t, client, server.URL, tmplA, "Template A", docA)
	epic2SetDefaultTemplate(t, client, server.URL, tmplA)
	// Template B
	tmplB := epic2CreateTemplate(t, client, server.URL, "Template B", "product", "single")
	docB := templateWithSlot("TEMPLATE B")
	epic2PublishTemplate(t, client, server.URL, tmplB, "Template B", docB)
	// Assign Alpha explicitly to B
	epic2PublishCustom(t, client, server.URL, "product", alphaRow.ID, "Alpha", "alpha", textDoc("ALPHA BODY"), nil, tmplB)
	// Beta remains default
	resp := getPath(t, client, server.URL, "/products/alpha")
	assertContains(t, bodyString(t, resp), "TEMPLATE B")
	resp = getPath(t, client, server.URL, "/products/beta")
	assertContains(t, bodyString(t, resp), "TEMPLATE A")
	// Update Template A to A2
	docA2 := templateWithSlot("TEMPLATE A2")
	epic2PublishTemplate(t, client, server.URL, tmplA, "Template A", docA2)
	resp = getPath(t, client, server.URL, "/products/beta")
	assertContains(t, bodyString(t, resp), "TEMPLATE A2")
	resp = getPath(t, client, server.URL, "/products/alpha")
	assertContains(t, bodyString(t, resp), "TEMPLATE B")
	assertNotContains(t, bodyString(t, resp), "TEMPLATE A2")
	// Publish B to B2
	docB2 := templateWithSlot("TEMPLATE B2")
	epic2PublishTemplate(t, client, server.URL, tmplB, "Template B", docB2)
	resp = getPath(t, client, server.URL, "/products/alpha")
	assertContains(t, bodyString(t, resp), "TEMPLATE B2")
	_ = betaRow
}

// 22 — FIELDS-ONLY SINGLE
func TestEpic2FieldsOnlySingle(t *testing.T) {
	server, _, database, authService, hub, cleanup := newProductTestServer(t)
	defer cleanup()
	client := setupAndLogin(t, server.URL, authService)
	fields := []content.FieldDefinition{
		{Key: "position", Label: "Position", Type: content.FieldText},
		{Key: "bio", Label: "Bio", Type: content.FieldText},
	}
	epic2CreateContentType(t, database.DB, db.New(database.DB), hub, "team_member", "Team Member", "Team Members", "/team", true, false, false, fields)
	// Template no slot
	tmpl := epic2CreateTemplate(t, client, server.URL, "Team Template", "team_member", "single")
	fdoc := fieldsOnlyTemplateDoc()
	epic2PublishTemplate(t, client, server.URL, tmpl, "Team Template", fdoc)
	epic2SetDefaultTemplate(t, client, server.URL, tmpl)
	epic2CreateAndPublishCustom(t, client, server.URL, "team_member", "John", "john", `{"version":1,"nodes":[]}`, map[string]string{"position": "CEO", "bio": "Hello"}, "")
	resp := getPath(t, client, server.URL, "/team/john")
	body := bodyString(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /team/john %d %s", resp.StatusCode, body[:2000])
	}
	assertContains(t, body, "John")
	assertContains(t, body, "CEO")
	assertContains(t, body, "Hello")
	// Ensure no content-slot leaked text
}

// 23 — HEADER DRAFT / PUBLISH / ASSIGN
func TestEpic2HeaderLifecycle(t *testing.T) {
	server, _, _, authService, cleanup := newIntegrationServer(t)
	defer cleanup()
	client := setupAndLogin(t, server.URL, authService)
	// Warm homepage before header exists to capture legacy header
	resp := getPath(t, client, server.URL, "/")
	legacy := bodyString(t, resp)
	assertNotContains(t, legacy, "HEADER ONE")
	// Create Main Header draft
	partID := epic2CreateSitePart(t, client, server.URL, "Main Header", "")
	epic2SaveSitePartDraft(t, client, server.URL, partID, "Main Header", headerPartDoc("HEADER ONE"))
	// Before publish still legacy
	resp = getPath(t, client, server.URL, "/")
	assertNotContains(t, bodyString(t, resp), "HEADER ONE")
	// Publish but not assigned still legacy
	epic2PublishSitePart(t, client, server.URL, partID, "Main Header", headerPartDoc("HEADER ONE"))
	resp = getPath(t, client, server.URL, "/")
	assertNotContains(t, bodyString(t, resp), "HEADER ONE")
	// Assign
	epic2AssignSitePart(t, client, server.URL, "header", partID)
	resp = getPath(t, client, server.URL, "/")
	assertContains(t, bodyString(t, resp), "HEADER ONE")
	// Edit draft to HEADER TWO not yet published
	epic2SaveSitePartDraft(t, client, server.URL, partID, "Main Header", headerPartDoc("HEADER TWO"))
	resp = getPath(t, client, server.URL, "/")
	assertContains(t, bodyString(t, resp), "HEADER ONE")
	assertNotContains(t, bodyString(t, resp), "HEADER TWO")
	// Publish -> HEADER TWO
	epic2PublishSitePart(t, client, server.URL, partID, "Main Header", headerPartDoc("HEADER TWO"))
	resp = getPath(t, client, server.URL, "/")
	assertContains(t, bodyString(t, resp), "HEADER TWO")
}

// 24 — HEADER ASSIGNMENT CACHE
func TestEpic2HeaderAssignmentCache(t *testing.T) {
	server, _, database, authService, cleanup := newIntegrationServer(t)
	defer cleanup()
	client := setupAndLogin(t, server.URL, authService)
	queries := db.New(database.DB)
	// Ensure product type exists for product single archive warm
	epic2CreateContentType(t, database.DB, queries, nil, "product", "Product", "Products", "/products", true, true, true, nil)
	// But use service internal hub not exposed; we rely on handler's InvalidateAll via HTTP
	// Create Header A assigned
	partA := epic2CreateSitePart(t, client, server.URL, "Header A", "")
	epic2PublishSitePart(t, client, server.URL, partA, "Header A", headerPartDoc("HEADER A"))
	epic2AssignSitePart(t, client, server.URL, "header", partA)
	// Warm pages
	for _, p := range []string{"/", "/about"} {
		_ = getPath(t, client, server.URL, p)
	}
	// Ensure product exists for warm
	epic2CreateAndPublishCustom(t, client, server.URL, "product", "Alpha", "alpha", textDoc("body"), nil, "")
	_ = getPath(t, client, server.URL, "/products/alpha")
	_ = getPath(t, client, server.URL, "/products")
	time.Sleep(100 * time.Millisecond)
	for _, p := range []string{"/", "/about", "/products/alpha", "/products"} {
		resp := getPath(t, client, server.URL, p)
		assertContains(t, bodyString(t, resp), "HEADER A")
	}
	// Create Header B
	partB := epic2CreateSitePart(t, client, server.URL, "Header B", "")
	epic2PublishSitePart(t, client, server.URL, partB, "Header B", headerPartDoc("HEADER B"))
	epic2AssignSitePart(t, client, server.URL, "header", partB)
	// Without manual invalidation, all pages should show B
	for _, p := range []string{"/", "/about", "/products/alpha", "/products"} {
		resp := getPath(t, client, server.URL, p)
		body := bodyString(t, resp)
		assertContains(t, body, "HEADER B")
		assertNotContains(t, body, "HEADER A")
	}
}

// 25 — FOOTER
func TestEpic2FooterLifecycle(t *testing.T) {
	server, _, database, authService, cleanup := newIntegrationServer(t)
	defer cleanup()
	client := setupAndLogin(t, server.URL, authService)
	queries := db.New(database.DB)
	epic2CreateContentType(t, database.DB, queries, nil, "product", "Product", "Products", "/products", true, true, true, nil)
	// Product for warm (use seeded /about for page warm)
	epic2CreateAndPublishCustom(t, client, server.URL, "product", "Alpha", "alpha", textDoc("body"), nil, "")
	// Archive template needed for /products? Ensure it exists via seed? /products archive may need template; but footer is global not dependent.
	partID := epic2CreateSitePart(t, client, server.URL, "Main Footer", "")
	epic2SaveSitePartDraft(t, client, server.URL, partID, "Main Footer", sitePartDoc("FOOTER ONE"))
	resp := getPath(t, client, server.URL, "/")
	assertNotContains(t, bodyString(t, resp), "FOOTER ONE")
	epic2PublishSitePart(t, client, server.URL, partID, "Main Footer", sitePartDoc("FOOTER ONE"))
	resp = getPath(t, client, server.URL, "/")
	assertNotContains(t, bodyString(t, resp), "FOOTER ONE")
	epic2AssignSitePart(t, client, server.URL, "footer", partID)
	for _, p := range []string{"/", "/about", "/products/alpha"} {
		assertContains(t, bodyString(t, getPath(t, client, server.URL, p)), "FOOTER ONE")
	}
	epic2SaveSitePartDraft(t, client, server.URL, partID, "Main Footer", sitePartDoc("FOOTER TWO"))
	for _, p := range []string{"/", "/about", "/products/alpha"} {
		body := bodyString(t, getPath(t, client, server.URL, p))
		assertContains(t, body, "FOOTER ONE")
		assertNotContains(t, body, "FOOTER TWO")
	}
	epic2PublishSitePart(t, client, server.URL, partID, "Main Footer", sitePartDoc("FOOTER TWO"))
	for _, p := range []string{"/", "/about", "/products/alpha"} {
		assertContains(t, bodyString(t, getPath(t, client, server.URL, p)), "FOOTER TWO")
	}
}

// 26 — GLOBAL SITE PART
func TestEpic2GlobalSitePart(t *testing.T) {
	server, _, database, authService, hub, cleanup := newProductTestServer(t)
	defer cleanup()
	client := setupAndLogin(t, server.URL, authService)
	epic2CreateContentType(t, database.DB, db.New(database.DB), hub, "product", "Product", "Products", "/products", true, true, true, nil)
	// Create Newsletter site part
	part := epic2CreateSitePart(t, client, server.URL, "Newsletter", "")
	epic2PublishSitePart(t, client, server.URL, part, "Newsletter", sitePartDoc("JOIN NOW"))
	// Create page and template referencing it
	epic2CreateAndPublishPage(t, client, server.URL, "Info", "info", sitePartRefDoc(part))
	tmpl := epic2CreateTemplate(t, client, server.URL, "Prod Newsletter", "product", "single")
	// Template doc includes site-part ref plus content slot
	tmplDoc := `{"version":1,"nodes":[{"id":"sec","block":"core/section","version":1,"props":{},"settings":{},"children":[{"id":"ref","block":"core/site-part","version":1,"props":{},"settings":{"sitePartId":` + mustJSON(part) + `}},{"id":"slot","block":"core/content-slot","version":1,"props":{},"settings":{}}]}]}`
	epic2PublishTemplate(t, client, server.URL, tmpl, "Prod Newsletter", tmplDoc)
	epic2SetDefaultTemplate(t, client, server.URL, tmpl)
	epic2CreateAndPublishCustom(t, client, server.URL, "product", "Alpha", "alpha", textDoc("body"), nil, "")
	// Both should show JOIN NOW
	assertContains(t, bodyString(t, getPath(t, client, server.URL, "/info")), "JOIN NOW")
	assertContains(t, bodyString(t, getPath(t, client, server.URL, "/products/alpha")), "JOIN NOW")
	// Draft change
	epic2SaveSitePartDraft(t, client, server.URL, part, "Newsletter", sitePartDoc("SUBSCRIBE"))
	assertContains(t, bodyString(t, getPath(t, client, server.URL, "/info")), "JOIN NOW")
	assertContains(t, bodyString(t, getPath(t, client, server.URL, "/products/alpha")), "JOIN NOW")
	assertNotContains(t, bodyString(t, getPath(t, client, server.URL, "/info")), "SUBSCRIBE")
	epic2PublishSitePart(t, client, server.URL, part, "Newsletter", sitePartDoc("SUBSCRIBE"))
	assertContains(t, bodyString(t, getPath(t, client, server.URL, "/info")), "SUBSCRIBE")
	assertContains(t, bodyString(t, getPath(t, client, server.URL, "/products/alpha")), "SUBSCRIBE")
}

// 27 — NESTED SITE PART
func TestEpic2NestedSitePart(t *testing.T) {
	server, _, _, authService, cleanup := newIntegrationServer(t)
	defer cleanup()
	client := setupAndLogin(t, server.URL, authService)
	partB := epic2CreateSitePart(t, client, server.URL, "Part B", "")
	epic2PublishSitePart(t, client, server.URL, partB, "Part B", sitePartDoc("B ONE"))
	partA := epic2CreateSitePart(t, client, server.URL, "Part A", "")
	epic2PublishSitePart(t, client, server.URL, partA, "Part A", compositeSitePartDoc("A ONE", partB))
	epic2CreateAndPublishPage(t, client, server.URL, "Nested", "nested", sitePartRefDoc(partA))
	resp := getPath(t, client, server.URL, "/nested")
	body := bodyString(t, resp)
	assertContains(t, body, "A ONE")
	assertContains(t, body, "B ONE")
	// Warm cache
	_ = getPath(t, client, server.URL, "/nested")
	// Update B
	epic2PublishSitePart(t, client, server.URL, partB, "Part B", sitePartDoc("B TWO"))
	resp = getPath(t, client, server.URL, "/nested")
	body = bodyString(t, resp)
	assertContains(t, body, "A ONE")
	assertContains(t, body, "B TWO")
	assertNotContains(t, body, "B ONE")
}

// 28 — TEMPLATE RESTORE
func TestEpic2TemplateRestore(t *testing.T) {
	server, _, database, authService, cleanup := newIntegrationServer(t)
	defer cleanup()
	client := setupAndLogin(t, server.URL, authService)
	queries := db.New(database.DB)
	// Create product type
	epic2CreateContentType(t, database.DB, queries, nil, "product", "Product", "Products", "/products", true, true, true, nil)
	tmpl := epic2CreateTemplate(t, client, server.URL, "Tmpl", "product", "single")
	epic2PublishTemplate(t, client, server.URL, tmpl, "Tmpl", templateWithSlot("ONE"))
	// Publish TWO
	epic2PublishTemplate(t, client, server.URL, tmpl, "Tmpl", templateWithSlot("TWO"))
	// Create entry using template default
	epic2SetDefaultTemplate(t, client, server.URL, tmpl)
	epic2CreateAndPublishCustom(t, client, server.URL, "product", "Alpha", "alpha", textDoc("body"), nil, "")
	// THREE draft
	epic2SaveTemplateDraft(t, client, server.URL, tmpl, "Tmpl", templateWithSlot("THREE"))
	// List revisions to find Revision 1 ID
	// Use HTTP to get revision history page and parse? Simpler use DB
	revs, _ := queries.ListLayoutTemplateRevisions(context.Background(), tmpl)
	var rev1ID string
	for _, r := range revs {
		if strings.Contains(r.DocumentJson, `"ONE"`) {
			rev1ID = r.ID
			break
		}
	}
	if rev1ID == "" {
		t.Fatalf("rev ONE not found")
	}
	// Restore Revision 1
	csrf := csrfToken(t, client, server.URL, "/admin/appearance/templates/"+tmpl+"/edit")
	resp := postForm(t, client, server.URL, "/admin/appearance/templates/"+tmpl+"/revisions/"+rev1ID+"/restore", url.Values{"csrf_token": {csrf}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("restore %d %s", resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
	// Public should remain TWO
	assertContains(t, bodyString(t, getPath(t, client, server.URL, "/products/alpha")), "TWO")
	assertNotContains(t, bodyString(t, getPath(t, client, server.URL, "/products/alpha")), "ONE")
	// Publish restored draft (latest is ONE copy)
	latest, _ := queries.GetLatestLayoutTemplateRevision(context.Background(), tmpl)
	epic2PublishTemplate(t, client, server.URL, tmpl, "Tmpl", latest.DocumentJson)
	assertContains(t, bodyString(t, getPath(t, client, server.URL, "/products/alpha")), "ONE")
	assertNotContains(t, bodyString(t, getPath(t, client, server.URL, "/products/alpha")), "TWO")
}

// 29 — SITE PART RESTORE
func TestEpic2SitePartRestore(t *testing.T) {
	server, _, database, authService, cleanup := newIntegrationServer(t)
	defer cleanup()
	client := setupAndLogin(t, server.URL, authService)
	queries := db.New(database.DB)
	part := epic2CreateSitePart(t, client, server.URL, "Header", "")
	epic2PublishSitePart(t, client, server.URL, part, "Header", headerPartDoc("ONE"))
	epic2PublishSitePart(t, client, server.URL, part, "Header", headerPartDoc("TWO"))
	epic2AssignSitePart(t, client, server.URL, "header", part)
	epic2CreateAndPublishPage(t, client, server.URL, "Home2", "home2", textDoc("home"))
	// Draft THREE? Actually need TWO published, restore ONE
	revs, _ := queries.ListSitePartRevisions(context.Background(), part)
	var rev1 string
	for _, r := range revs {
		if strings.Contains(r.DocumentJson, `"ONE"`) {
			rev1 = r.ID
			break
		}
	}
	csrf := csrfToken(t, client, server.URL, "/admin/appearance/site-parts/"+part+"/edit")
	resp := postForm(t, client, server.URL, "/admin/appearance/site-parts/"+part+"/revisions/"+rev1+"/restore", url.Values{"csrf_token": {csrf}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("restore site part %d %s", resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
	assertContains(t, bodyString(t, getPath(t, client, server.URL, "/home2")), "TWO")
	assertNotContains(t, bodyString(t, getPath(t, client, server.URL, "/home2")), "ONE")
	// Publish restored draft
	latest, _ := queries.GetLatestSitePartRevision(context.Background(), part)
	epic2PublishSitePart(t, client, server.URL, part, "Header", latest.DocumentJson)
	assertContains(t, bodyString(t, getPath(t, client, server.URL, "/home2")), "ONE")
}

// 30 — NAVIGATION IN HEADER
func TestEpic2NavigationInHeader(t *testing.T) {
	server, _, database, authService, hub, cleanup := newProductTestServer(t)
	defer cleanup()
	client := setupAndLogin(t, server.URL, authService)
	// Create menu via service directly
	svc := navigation.NewService(database.DB, db.New(database.DB))
	menu, err := svc.CreateMenu(context.Background(), "Primary")
	if err != nil {
		t.Fatalf("create menu: %v", err)
	}
	homeID := "home"
	aboutID := "about"
	teamID := "team"
	items := []navigation.ItemInput{
		{ID: homeID, Label: "Home", TargetType: "url", URL: "/", Position: 0},
		{ID: aboutID, Label: "About", TargetType: "url", URL: "/about", Position: 1},
		{ID: teamID, Label: "Team", TargetType: "url", URL: "/team", Position: 0, ParentID: aboutID},
	}
	if err := svc.SaveMenu(context.Background(), menu.ID, "Primary", items, []string{"primary"}); err != nil {
		t.Fatalf("save menu: %v", err)
	}
	if err := hub.ReloadNavigation(context.Background()); err != nil {
		t.Fatalf("reload navigation: %v", err)
	}
	// Create header with navigation block
	part := epic2CreateSitePart(t, client, server.URL, "Header Nav", "")
	epic2PublishSitePart(t, client, server.URL, part, "Header Nav", navigationHeaderDoc())
	epic2AssignSitePart(t, client, server.URL, "header", part)
	resp := getPath(t, client, server.URL, "/")
	body := bodyString(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET / %d", resp.StatusCode)
	}
	for _, needle := range []string{"Home", "About", "Team"} {
		assertContains(t, body, needle)
	}
	// Check nested structure - team under about should be inside a nested <ul>
	if strings.Count(body, "<ul>") < 2 {
		t.Fatalf("expected nested list in navigation: %s", body[:4000])
	}
}

// 31 — ARCHIVE PAGINATION
func TestEpic2ArchivePagination(t *testing.T) {
	server, _, database, authService, hub, cleanup := newProductTestServer(t)
	defer cleanup()
	client := setupAndLogin(t, server.URL, authService)
	epic2CreateContentType(t, database.DB, db.New(database.DB), hub, "product", "Product", "Products", "/products", true, true, true, nil)
	tmpl := epic2CreateTemplate(t, client, server.URL, "Products Archive", "product", "archive")
	arcDoc := archiveTemplateDoc()
	epic2PublishTemplate(t, client, server.URL, tmpl, "Products Archive", arcDoc)
	epic2SetDefaultArchiveTemplate(t, client, server.URL, tmpl)
	// Create 25 products
	for i := 1; i <= 25; i++ {
		title := fmt.Sprintf("Product %02d", i)
		slug := fmt.Sprintf("product-%02d", i)
		epic2CreateAndPublishCustom(t, client, server.URL, "product", title, slug, textDoc(title), nil, "")
	}
	// Determine perPage
	queries := db.New(database.DB)
	row, _ := queries.GetSiteSettings(context.Background())
	perPage := int(row.PostsPerPage)
	if perPage < 1 {
		perPage = 10
	}
	t.Logf("perPage %d", perPage)
	// Page 2 should contain correct slice: products sorted by published desc? Default order is published_desc, so newest first.
	// We created product-01 .. product-25 in order, so newest is 25. Page 1: 25-16, Page2: 15-06 etc.
	// Just ensure page2 does not contain product-25 or product-01 incorrectly.
	resp := getPath(t, client, server.URL, "/products/page/2")
	body := bodyString(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /products/page/2 %d %s", resp.StatusCode, body[:2000])
	}
	var page2Products []string
	for i := 1; i <= 25; i++ {
		title := fmt.Sprintf("Product %02d", i)
		if strings.Contains(body, title) {
			page2Products = append(page2Products, title)
		}
	}
	t.Logf("page2 perPage %d products %v", perPage, page2Products)
	resp1 := getPath(t, client, server.URL, "/products")
	body1 := bodyString(t, resp1)
	var page1Products []string
	for i := 1; i <= 25; i++ {
		title := fmt.Sprintf("Product %02d", i)
		if strings.Contains(body1, title) {
			page1Products = append(page1Products, title)
		}
	}
	t.Logf("page1 products %v", page1Products)
	// Ensure no overlap between page1 and page2 (critical: collection must not re-query without pagination)
	for _, p := range page1Products {
		if strings.Contains(body, p) {
			t.Fatalf("page2 should not contain %q which is on page1 (overlap), page1 %v page2 %v", p, page1Products, page2Products)
		}
	}
	if len(page2Products) != perPage {
		t.Fatalf("page2 should have %d products, got %d: %v", perPage, len(page2Products), page2Products)
	}
	if len(page1Products) != perPage {
		t.Fatalf("page1 should have %d products, got %d: %v", perPage, len(page1Products), page1Products)
	}
	_ = perPage
}

// 32 — DELETE PROTECTION
func TestEpic2DeleteProtection(t *testing.T) {
	server, _, database, authService, cleanup := newIntegrationServer(t)
	defer cleanup()
	client := setupAndLogin(t, server.URL, authService)
	queries := db.New(database.DB)
	// Template delete protection
	tmpl := epic2CreateTemplate(t, client, server.URL, "Deletable", "page", "single")
	epic2PublishTemplate(t, client, server.URL, tmpl, "Deletable", templateWithSlot("X"))
	epic2SetDefaultTemplate(t, client, server.URL, tmpl)
	csrf := csrfToken(t, client, server.URL, "/admin/appearance/templates/"+tmpl+"/edit")
	resp := postForm(t, client, server.URL, "/admin/appearance/templates/"+tmpl+"/delete", url.Values{"csrf_token": {csrf}})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("delete default should block 400, got %d %s", resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
	epic2ClearDefaultTemplate(t, client, server.URL, tmpl)
	// Create entry referencing it explicitly
	epic2CreateAndPublishPage(t, client, server.URL, "Ref", "ref", textDoc("ref"))
	// Need to find its ID and assign template
	row, _ := queries.GetFlatEntryBySlug(context.Background(), db.GetFlatEntryBySlugParams{ContentTypeID: "page", Slug: "ref"})
	// Update to use template
	csrf = csrfToken(t, client, server.URL, "/admin/pages/"+row.ID+"/edit")
	resp = postForm(t, client, server.URL, "/admin/pages/"+row.ID+"/publish", url.Values{
		"title": {"Ref"}, "slug": {"ref"}, "document_json": {textDoc("ref")}, "layout_template_id": {tmpl}, "csrf_token": {csrf},
	})
	resp.Body.Close()
	resp = postForm(t, client, server.URL, "/admin/appearance/templates/"+tmpl+"/delete", url.Values{"csrf_token": {csrfToken(t, client, server.URL, "/admin/appearance/templates/"+tmpl+"/edit")}})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("delete referenced should block, got %d %s", resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
	// Remove reference
	csrf = csrfToken(t, client, server.URL, "/admin/pages/"+row.ID+"/edit")
	resp = postForm(t, client, server.URL, "/admin/pages/"+row.ID+"/publish", url.Values{
		"title": {"Ref"}, "slug": {"ref"}, "document_json": {textDoc("ref")}, "csrf_token": {csrf},
	})
	resp.Body.Close()
	resp = postForm(t, client, server.URL, "/admin/appearance/templates/"+tmpl+"/delete", url.Values{"csrf_token": {csrfToken(t, client, server.URL, "/admin/appearance/templates/"+tmpl+"/edit")}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("delete after clearing should succeed, got %d %s", resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
	// Site part delete protection: assigned header
	part := epic2CreateSitePart(t, client, server.URL, "Header Del", "")
	epic2PublishSitePart(t, client, server.URL, part, "Header Del", sitePartDoc("H"))
	epic2AssignSitePart(t, client, server.URL, "header", part)
	csrf = csrfToken(t, client, server.URL, "/admin/appearance/site-parts/"+part+"/edit")
	resp = postForm(t, client, server.URL, "/admin/appearance/site-parts/"+part+"/delete", url.Values{"csrf_token": {csrf}})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("delete assigned header should block, got %d %s", resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
	epic2ClearSitePartLocation(t, client, server.URL, "header")
	// Create page draft referencing site part
	epic2CreateAndPublishPage(t, client, server.URL, "Page Ref", "page-ref", sitePartRefDoc(part))
	// Need to test draft reference blocking: create a draft that references it
	// Use query to check IsReferenced: it checks latest revisions, so draft should block
	// To simulate, we need a page that references part but not yet published? Our page is published with ref, so it blocks
	resp = postForm(t, client, server.URL, "/admin/appearance/site-parts/"+part+"/delete", url.Values{"csrf_token": {csrfToken(t, client, server.URL, "/admin/appearance/site-parts/"+part+"/edit")}})
	if resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("delete referenced site part should block, got %d %s", resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
	// Remove reference by editing page to not reference
	pag, _ := queries.GetFlatEntryBySlug(context.Background(), db.GetFlatEntryBySlugParams{ContentTypeID: "page", Slug: "page-ref"})
	csrf = csrfToken(t, client, server.URL, "/admin/pages/"+pag.ID+"/edit")
	resp = postForm(t, client, server.URL, "/admin/pages/"+pag.ID+"/publish", url.Values{
		"title": {"Page Ref"}, "slug": {"page-ref"}, "document_json": {textDoc("no ref")}, "csrf_token": {csrf},
	})
	resp.Body.Close()
	resp = postForm(t, client, server.URL, "/admin/appearance/site-parts/"+part+"/delete", url.Values{"csrf_token": {csrfToken(t, client, server.URL, "/admin/appearance/site-parts/"+part+"/edit")}})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("delete after removing ref should succeed, got %d %s", resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
}

// 35 — PREVIEW AUTH / NO-STORE LIGHTLY
func TestEpic2PreviewNoStore(t *testing.T) {
	server3, queries3, _, authService3, cleanup3 := newIntegrationServer(t)
	defer cleanup3()
	client3 := setupAndLogin(t, server3.URL, authService3)
	tmpl3 := epic2CreateTemplate(t, client3, server3.URL, "Preview3", "page", "single")
	epic2PublishTemplate(t, client3, server3.URL, tmpl3, "Preview3", templateWithSlot("P"))
	revs, _ := queries3.ListLayoutTemplateRevisions(context.Background(), tmpl3)
	if len(revs) == 0 {
		t.Fatal("no revs")
	}
	revID := revs[0].ID
	resp := getPath(t, client3, server3.URL, "/admin/appearance/templates/"+tmpl3+"/revisions/"+revID+"/preview")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("preview %d", resp.StatusCode)
	}
	if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Fatalf("Cache-Control missing no-store: %q", cc)
	}
	if xr := resp.Header.Get("X-Robots-Tag"); !strings.Contains(xr, "noindex") {
		t.Fatalf("X-Robots-Tag missing noindex: %q", xr)
	}
	resp.Body.Close()
	// Site part preview
	part := epic2CreateSitePart(t, client3, server3.URL, "PrevPart", "")
	epic2PublishSitePart(t, client3, server3.URL, part, "PrevPart", sitePartDoc("X"))
	revs2, _ := queries3.ListSitePartRevisions(context.Background(), part)
	rev2 := revs2[0].ID
	resp = getPath(t, client3, server3.URL, "/admin/appearance/site-parts/"+part+"/revisions/"+rev2+"/preview")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("site part preview %d", resp.StatusCode)
	}
	if cc := resp.Header.Get("Cache-Control"); !strings.Contains(cc, "no-store") {
		t.Fatalf("site part Cache-Control missing no-store: %q", cc)
	}
	if xr := resp.Header.Get("X-Robots-Tag"); !strings.Contains(xr, "noindex") {
		t.Fatalf("site part X-Robots-Tag missing noindex: %q", xr)
	}
	resp.Body.Close()
}

// Revision history UI uses shared template: verify no inline style and shared view
func TestEpic2RevisionHistoryUI(t *testing.T) {
	server, _, _, authService, cleanup := newIntegrationServer(t)
	defer cleanup()
	client := setupAndLogin(t, server.URL, authService)
	tmpl := epic2CreateTemplate(t, client, server.URL, "Hist", "page", "single")
	epic2PublishTemplate(t, client, server.URL, tmpl, "Hist", templateWithSlot("ONE"))
	epic2SaveTemplateDraft(t, client, server.URL, tmpl, "Hist", templateWithSlot("TWO"))
	resp := getPath(t, client, server.URL, "/admin/appearance/templates/"+tmpl+"/revisions")
	body := bodyString(t, resp)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("revisions %d %s", resp.StatusCode, body[:minInt(len(body), 1000)])
	}
	if strings.Contains(body, `style=`) {
		t.Fatalf("revision history should not contain inline style, got %s", body[:minInt(len(body), 2000)])
	}
	if !strings.Contains(body, "Revision history") {
		t.Fatalf("missing heading")
	}
	if !strings.Contains(body, "Current draft") {
		t.Fatalf("missing Current draft status")
	}
	if !strings.Contains(body, "Published") {
		t.Fatalf("missing Published status")
	}
	if strings.Contains(body, "Revision 1 · Revision") {
		t.Fatalf("duplicate revision copy")
	}
	if !strings.Contains(body, `title="Restore as new draft"`) {
		t.Fatalf("restore button missing title")
	}
	// Site part same template
	part := epic2CreateSitePart(t, client, server.URL, "HistPart", "")
	epic2PublishSitePart(t, client, server.URL, part, "HistPart", sitePartDoc("A"))
	resp = getPath(t, client, server.URL, "/admin/appearance/site-parts/"+part+"/revisions")
	body = bodyString(t, resp)
	if strings.Contains(body, `style=`) {
		t.Fatalf("site part revision history should not contain inline style")
	}
	assertContains(t, body, "Site Part")
}

func TestEpic2SlugCanonical(t *testing.T) {
	server, _, _, authService, cleanup := newIntegrationServer(t)
	defer cleanup()
	client := setupAndLogin(t, server.URL, authService)
	// Try manual slug with uppercase and Polish
	csrf := csrfToken(t, client, server.URL, "/admin/pages/new")
	resp := postForm(t, client, server.URL, "/admin/pages", url.Values{
		"title": {"Test"}, "slug": {"My CUSTOM Ślug!!!"}, "document_json": {textDoc("body")}, "publish": {"true"}, "csrf_token": {csrf},
	})
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create page canonical slug %d %s", resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
	// Find entry via slug canonicalized: expect "my-custom-slug" (slugify lowercases, transliterates Ś->s)
	// Use direct GET to verify path is canonical
	resp = getPath(t, client, server.URL, "/my-custom-slug")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET canonical slug %d", resp.StatusCode)
	}
	body := bodyString(t, resp)
	assertContains(t, body, "Test")
	// Ensure original manual slug with caps not used as path
	resp = getPath(t, client, server.URL, "/My CUSTOM Ślug!!!")
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("original uncanonical slug should not resolve")
	}
	resp.Body.Close()
}
