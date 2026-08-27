package integration

import (
	"context"
	"net/http"
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

func newProductTestServer(t *testing.T) (*httptest.Server, *db.Queries, *storage.Database, *auth.Service, *runtimehub.Runtime, func()) {
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
	return server, queries, database, service, hub, func() { server.Close(); _ = database.Close() }
}

func TestProductSliceHTTP(t *testing.T) {
	server, queries, _, service, _, cleanup := newProductTestServer(t)
	defer cleanup()
	ctx := context.Background()

	setupClient := newClient(t)
	setupToken := service.SetupCode()
	form := url.Values{"setup_code": {setupToken}, "site_title": {"Test Site"}, "email": {"admin@example.com"}, "password": {"a sufficiently long password"}, "csrf_token": {csrfToken(t, setupClient, server.URL, "/admin/setup")}}
	resp := postForm(t, setupClient, server.URL, "/admin/setup", form)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("setup %d %s", resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()

	client := newClient(t)
	loginForm := url.Values{"email": {"admin@example.com"}, "password": {"a sufficiently long password"}, "csrf_token": {csrfToken(t, client, server.URL, "/admin/login")}}
	resp = postForm(t, client, server.URL, "/admin/login", loginForm)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("login %d", resp.StatusCode)
	}
	resp.Body.Close()

	// Create product type
	createForm := url.Values{
		"id":          {"product"},
		"name":        {"Product"},
		"plural_name": {"Products"},
		"base_path":   {"/products"},
		"public":      {"on"},
		"archive":     {"on"},
		"featured":    {"on"},
		"seo":         {"on"},
		"csrf_token":  {csrfToken(t, client, server.URL, "/admin/settings/content-types/new")},
	}
	resp = postForm(t, client, server.URL, "/admin/settings/content-types", createForm)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create product type %d body %s", resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()

	addField := func(label, key, typ string) {
		t.Helper()
		tok := csrfToken(t, client, server.URL, "/admin/settings/content-types/product")
		f := url.Values{
			"name":        {"Product"},
			"plural_name": {"Products"},
			"base_path":   {"/products"},
			"public":      {"on"},
			"archive":     {"on"},
			"featured":    {"on"},
			"seo":         {"on"},
			"field_label": {label},
			"field_key":   {key},
			"field_type":  {typ},
			"csrf_token":  {tok},
			"add_field":   {"1"},
		}
		resp := postForm(t, client, server.URL, "/admin/settings/content-types/product", f)
		if resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("add field %s %d %s", label, resp.StatusCode, bodyString(t, resp))
		}
		resp.Body.Close()
	}
	addField("Price", "price", "number")
	addField("SKU", "sku", "text")
	addField("Featured", "featured", "boolean")
	addField("Product Image", "product_image", "media")

	// Create MacBook product via HTTP
	createProduct := func(title, slug, price, sku string, featured bool) {
		t.Helper()
		tok := csrfToken(t, client, server.URL, "/admin/content/product/new")
		featuredVal := ""
		if featured {
			featuredVal = "true"
		}
		f := url.Values{
			"title":                  {title},
			"slug":                   {slug},
			"field_price":            {price},
			"field_sku":              {sku},
			"field_featured":         {featuredVal},
			"field_featured_present": {"true"},
			"document_json":          {`{"version":1,"nodes":[]}`},
			"csrf_token":             {tok},
			"publish":                {"true"},
		}
		resp := postForm(t, client, server.URL, "/admin/content/product", f)
		if resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("create product %s %d %s", title, resp.StatusCode, bodyString(t, resp))
		}
		resp.Body.Close()
	}
	createProduct("MacBook Pro", "macbook-pro", "999", "MBP-001", true)
	// Debug DB after product creation
	// Check public
	resp = getPath(t, client, server.URL, "/products/macbook-pro")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET product %d %s", resp.StatusCode, bodyString(t, resp))
	}
	b := bodyString(t, resp)
	if !strings.Contains(b, "MacBook") && !strings.Contains(b, "macbook") {
		t.Fatalf("product page missing title %s", b[:1000])
	}

	resp = getPath(t, client, server.URL, "/products")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("archive %d %s", resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()

	// Create layout template
	tok := csrfToken(t, client, server.URL, "/admin/appearance/templates/new")
	f := url.Values{"name": {"Product Template"}, "content_type_id": {"product"}, "csrf_token": {tok}}
	resp = postForm(t, client, server.URL, "/admin/appearance/templates", f)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create template %d %s", resp.StatusCode, bodyString(t, resp))
	}
	loc := resp.Header.Get("Location")
	resp.Body.Close()
	re := regexp.MustCompile(`/admin/appearance/templates/([^/]+)/edit`)
	m := re.FindStringSubmatch(loc)
	if len(m) < 2 {
		t.Fatalf("no tmpl id %s", loc)
	}
	tmplID := m[1]
	tmplDoc := `{"version":1,"nodes":[{"id":"sec1","block":"core/section","version":1,"props":{},"settings":{},"children":[{"id":"ef1","block":"core/entry-field","version":1,"props":{"source":"entry.title"},"settings":{"tag":"h1"}},{"id":"em1","block":"core/entry-media","version":1,"props":{"source":"fields.product_image"},"settings":{}},{"id":"ef2","block":"core/entry-field","version":1,"props":{"source":"fields.price"},"settings":{"tag":"p","prefix":"$"}},{"id":"slot","block":"core/content-slot","version":1,"props":{},"settings":{}}]}]}`
	tok = csrfToken(t, client, server.URL, "/admin/appearance/templates/"+tmplID+"/edit")
	f = url.Values{"name": {"Product Template"}, "document_json": {tmplDoc}, "csrf_token": {tok}}
	resp = postForm(t, client, server.URL, "/admin/appearance/templates/"+tmplID, f)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("save tmpl draft %d %s", resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
	tok = csrfToken(t, client, server.URL, "/admin/appearance/templates/"+tmplID+"/edit")
	f = url.Values{"name": {"Product Template"}, "document_json": {tmplDoc}, "csrf_token": {tok}}
	resp = postForm(t, client, server.URL, "/admin/appearance/templates/"+tmplID+"/publish", f)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("publish tmpl %d %s", resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
	tok = csrfToken(t, client, server.URL, "/admin/appearance/templates/"+tmplID+"/edit")
	f = url.Values{"csrf_token": {tok}}
	resp = postForm(t, client, server.URL, "/admin/appearance/templates/"+tmplID+"/default", f)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("default tmpl %d %s", resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
	// debug default
	// direct resolve disabled for now to isolate handler logs
	// if prodEntry.ID != "" {
	// 	rev, _ := queries.GetLatestEntryRevision(ctx, prodEntry.ID)
	// 	doc, _ := document.Decode([]byte(rev.DocumentJson))
	// 	eff, revID, err := layouts.ResolveEffectiveDocumentWithID(ctx, queries, doc, "product", sql.NullString{})
	// 	if eff != nil && len(eff.Nodes) >0 {
	// 		pd, err := hub.Blocks.Prepare(eff)
	// 		if err == nil {
	// 			for _, n := range pd.Nodes {
	// 			}
	// 			fields, _ := content.DecodeFieldSnapshot(rev.FieldsJson)
	// 			rc := rendering.RenderContext{
	// 				Entry: rendering.EntryContext{Title: rev.Title, Fields: fields, Permalink: "/products/macbook-pro"},
	// 				Mode: rendering.ModePublic,
	// 			}
	// 			html, err := hub.Blocks.RenderPrepared(ctx, pd, rc)
	// 			snippet := string(html)
	// 			if len(snippet) > 500 { snippet = snippet[:500] }
	// 		}
	// 	}
	// }
	// also test resolver directly
	{
		// Import layouts via dynamic? We'll just use a helper to test fallback logic by querying CT
		// The resolver fallback should have kicked in; let's verify by checking if template doc is used in GET body more thoroughly
	}

	// check product now renders template prefix
	resp = getPath(t, client, server.URL, "/products/macbook-pro")
	b = bodyString(t, resp)
	// find content area more robustly
	snippet := b
	if len(b) > 8000 {
		snippet = b[:8000]
	}
	// also check for title in H1
	if strings.Contains(b, "stratum-entry-field") {
	}
	if strings.Contains(b, "MacBook") {
	}
	if !strings.Contains(b, "$999") {
		// dump middle part
		idx := strings.Index(b, "stratum-entry")
		if idx != -1 {
			end := idx + 1000
			if end > len(b) {
				end = len(b)
			}
		}
		// dump whole body length
		// also check if template composition happened by looking for section
		t.Fatalf("template not applied missing $999 - body has entry-field %v, len %d, snippet %s", strings.Contains(b, "stratum-entry-field"), len(b), snippet[len(snippet)/2:][:1000])
	}

	// Create shop page with collection
	collectionDoc := `{"version":1,"nodes":[{"id":"h1","block":"core/heading","version":2,"props":{"text":{"version":1,"content":[{"text":"Featured Products"}]}},"settings":{"level":2}},{"id":"coll1","block":"core/collection","version":2,"props":{},"settings":{"contentType":"product","limit":6,"excludeCurrent":false,"orderBy":"fields.price","direction":"asc","filters":[{"field":"fields.featured","operator":"is_true"}]},"children":[{"id":"stack1","block":"core/stack","version":1,"props":{},"settings":{},"children":[{"id":"cem","block":"core/entry-media","version":1,"props":{"source":"fields.product_image"},"settings":{}},{"id":"cef1","block":"core/entry-field","version":1,"props":{"source":"entry.title"},"settings":{"tag":"h3"}},{"id":"cef2","block":"core/entry-field","version":1,"props":{"source":"fields.price"},"settings":{"tag":"p","prefix":"$"}},{"id":"el","block":"core/entry-link","version":1,"props":{"text":"View"},"settings":{}}]}]}]}`
	tok = csrfToken(t, client, server.URL, "/admin/pages/new")
	f = url.Values{"title": {"Shop"}, "slug": {"shop"}, "document_json": {collectionDoc}, "csrf_token": {tok}, "publish": {"true"}}
	resp = postForm(t, client, server.URL, "/admin/pages", f)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create shop %d %s", resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
	resp = getPath(t, client, server.URL, "/shop")
	b = bodyString(t, resp)
	if !strings.Contains(b, "MacBook") || !strings.Contains(b, "$999") {
		t.Fatalf("shop missing macbook %s", b[:3000])
	}

	// Add second product
	createProduct("iPhone", "iphone", "499", "IPH-001", true)
	resp = getPath(t, client, server.URL, "/shop")
	b = bodyString(t, resp)
	idx499 := strings.Index(b, "$499")
	idx999 := strings.Index(b, "$999")
	if idx499 == -1 || idx999 == -1 || idx499 > idx999 {
		t.Fatalf("order wrong idx499 %d idx999 %d %s", idx499, idx999, b[:3000])
	}

	// Draft change
	rows, err := queries.ListPublishedEntriesByContentType(ctx, db.ListPublishedEntriesByContentTypeParams{ContentTypeID: "product", Limit: 100, Offset: 0})
	if err != nil {
		t.Fatal(err)
	}
	var iphoneID string
	for _, r := range rows {
		if r.Slug == "iphone" {
			iphoneID = r.ID
		}
	}
	if iphoneID == "" {
		t.Fatal("iphone not found")
	}
	tok = csrfToken(t, client, server.URL, "/admin/content/product/"+iphoneID+"/edit")
	f = url.Values{"title": {"iPhone"}, "slug": {"iphone"}, "field_price": {"1500"}, "field_sku": {"IPH-001"}, "field_featured": {"true"}, "field_featured_present": {"true"}, "document_json": {`{"version":1,"nodes":[]}`}, "csrf_token": {tok}}
	resp = postForm(t, client, server.URL, "/admin/content/product/"+iphoneID, f)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("draft save %d %s", resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
	resp = getPath(t, client, server.URL, "/shop")
	b = bodyString(t, resp)
	if strings.Contains(b, "$1500") {
		t.Fatalf("draft leaked %s", b[:2000])
	}
	if !strings.Contains(b, "$499") {
		t.Fatalf("499 missing after draft %s", b[:2000])
	}
	tok = csrfToken(t, client, server.URL, "/admin/content/product/"+iphoneID+"/edit")
	f = url.Values{"title": {"iPhone"}, "slug": {"iphone"}, "field_price": {"1500"}, "field_sku": {"IPH-001"}, "field_featured": {"true"}, "field_featured_present": {"true"}, "document_json": {`{"version":1,"nodes":[]}`}, "csrf_token": {tok}}
	resp = postForm(t, client, server.URL, "/admin/content/product/"+iphoneID+"/publish", f)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("publish 1500 %d %s", resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
	resp = getPath(t, client, server.URL, "/shop")
	b = bodyString(t, resp)
	if !strings.Contains(b, "$1500") {
		t.Fatalf("after publish missing 1500 %s", b[:2000])
	}
	idx999 = strings.Index(b, "$999")
	idx1500 := strings.Index(b, "$1500")
	if idx999 == -1 || idx1500 == -1 || idx999 > idx1500 {
		t.Fatalf("order after publish wrong %d %d %s", idx999, idx1500, b[:2000])
	}

	// Test base path change
	// Change /products -> /catalog
	tok = csrfToken(t, client, server.URL, "/admin/settings/content-types/product")
	f = url.Values{
		"name":        {"Product"},
		"plural_name": {"Products"},
		"base_path":   {"/catalog"},
		"public":      {"on"},
		"archive":     {"on"},
		"featured":    {"on"},
		"seo":         {"on"},
		"csrf_token":  {tok},
	}
	resp = postForm(t, client, server.URL, "/admin/settings/content-types/product", f)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("change base %d %s", resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
	resp = getPath(t, client, server.URL, "/catalog/macbook-pro")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("new path %d", resp.StatusCode)
	}
	resp.Body.Close()
	// old should redirect
	resp = getPath(t, client, server.URL, "/products/macbook-pro")
	if resp.StatusCode != http.StatusMovedPermanently {
		t.Fatalf("old path should 301 got %d", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/catalog/macbook-pro" {
		t.Fatalf("redirect loc %q", loc)
	}
	resp.Body.Close()

	// Test delete - create temp type and delete it
	tok = csrfToken(t, client, server.URL, "/admin/settings/content-types/new")
	f = url.Values{
		"id":          {"deletable"},
		"name":        {"Deletable"},
		"plural_name": {"Deletables"},
		"base_path":   {"/deletable"},
		"public":      {"on"},
		"archive":     {"on"},
		"csrf_token":  {tok},
	}
	resp = postForm(t, client, server.URL, "/admin/settings/content-types", f)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create deletable %d %s", resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
	// delete it - should succeed (no entries)
	tok = csrfToken(t, client, server.URL, "/admin/settings/content-types/deletable")
	f = url.Values{"csrf_token": {tok}}
	resp = postForm(t, client, server.URL, "/admin/settings/content-types/deletable/delete", f)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("delete deletable %d %s", resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
	// verify gone
	if _, err := queries.GetContentType(ctx, "deletable"); err == nil {
		t.Fatalf("deletable still exists after delete")
	}
	// try delete product with entries - should fail
	tok = csrfToken(t, client, server.URL, "/admin/settings/content-types/product")
	f = url.Values{"csrf_token": {tok}}
	resp = postForm(t, client, server.URL, "/admin/settings/content-types/product/delete", f)
	// should be 200 with error message, not redirect
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete product with entries should fail with 200, got %d", resp.StatusCode)
	}
	body := bodyString(t, resp)
	if !strings.Contains(body, "contains") || !strings.Contains(body, "entries") {
		t.Fatalf("delete product error message missing, body %s", body[:2000])
	}
	// try delete builtin page - should fail
	tok = csrfToken(t, client, server.URL, "/admin/settings/content-types/page")
	f = url.Values{"csrf_token": {tok}}
	resp = postForm(t, client, server.URL, "/admin/settings/content-types/page/delete", f)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("delete page should fail 200, got %d", resp.StatusCode)
	}
	body = bodyString(t, resp)
	if !strings.Contains(body, "built-in") {
		t.Fatalf("delete page should mention built-in, body %s", body[:2000])
	}

}
