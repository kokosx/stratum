package integration

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func TestArchiveShellWithLayoutTemplate(t *testing.T) {
	server, queries, _, authService, cleanup := newIntegrationServer(t)
	defer cleanup()
	ctx := context.Background()
	client := setupAndLogin(t, server.URL, authService)

	// Create Home and Blog pages
	createAndPublishPage(t, client, server.URL, "Home", "home-page", simpleDoc("Home"))
	time.Sleep(200 * time.Millisecond)
	homeEntry, _ := queries.GetFlatEntryBySlug(ctx, db.GetFlatEntryBySlugParams{ContentTypeID: "page", Slug: "home-page"})

	// Clean seeded blog if exists
	if _, err := queries.GetEntry(ctx, "seed-blog"); err == nil {
		_ = queries.DeleteEntry(ctx, "seed-blog")
	}
	if rt, err := queries.GetRouteByPath(ctx, "/blog"); err == nil {
		_ = queries.DeleteRoute(ctx, rt.ID)
	}
	createAndPublishPage(t, client, server.URL, "Blog", "blog", blogDoc())
	time.Sleep(200 * time.Millisecond)
	blogEntry, _ := queries.GetFlatEntryBySlug(ctx, db.GetFlatEntryBySlugParams{ContentTypeID: "page", Slug: "blog"})
	saveReadingSettings(t, client, server.URL, queries, homeEntry.ID, blogEntry.ID, "/blog", "10")

	// Create layout template for page type? Actually archive uses page's content type? Posts page is page, but layout for archive is derived from Posts Page's layout_template_id. Create layout for page.
	createLayoutForm := url.Values{
		"name":            {"Archive Layout"},
		"content_type_id": {"page"},
		"csrf_token":      {csrfToken(t, client, server.URL, "/admin/appearance/templates/new")},
	}
	createResp := postForm(t, client, server.URL, "/admin/appearance/templates", createLayoutForm)
	if createResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create layout status %d", createResp.StatusCode)
	}
	loc := createResp.Header.Get("Location")
	layoutID := extractLayoutID(loc)
	if layoutID == "" {
		t.Fatalf("no layout id")
	}
	createResp.Body.Close()

	// Edit layout to have marker + content-slot
	layoutDoc := `{"version":1,"nodes":[{"id":"h1","block":"core/heading","version":1,"props":{"text":"LAYOUT MARKER","level":1},"settings":{}},{"id":"slot1","block":"core/content-slot","version":1,"props":{},"settings":{}}]}`
	saveForm := url.Values{
		"name":          {"Archive Layout"},
		"document_json": {layoutDoc},
		"csrf_token":    {csrfToken(t, client, server.URL, "/admin/appearance/templates/"+layoutID+"/edit")},
	}
	saveResp := postForm(t, client, server.URL, "/admin/appearance/templates/"+layoutID, saveForm)
	saveResp.Body.Close()
	publishForm := url.Values{
		"name":          {"Archive Layout"},
		"document_json": {layoutDoc},
		"csrf_token":    {csrfToken(t, client, server.URL, "/admin/appearance/templates/"+layoutID+"/edit")},
	}
	pubResp := postForm(t, client, server.URL, "/admin/appearance/templates/"+layoutID+"/publish", publishForm)
	pubResp.Body.Close()

	// Assign layout to Posts Page (Blog)
	// Fetch current Blog doc
	blogRev, _ := queries.GetLatestLayoutTemplateRevision(ctx, layoutID) // just to ensure published
	_ = blogRev
	// Update Blog page to use layout template
	// Need to publish Blog page with layout_template_id
	blogDocWithLayout := blogDoc() // same doc but we will publish with layout
	form := url.Values{
		"title":              {"Blog"},
		"slug":               {"blog"},
		"document_json":      {blogDocWithLayout},
		"layout_template_id": {layoutID},
		"csrf_token":         {csrfToken(t, client, server.URL, "/admin/pages/"+blogEntry.ID+"/edit")},
	}
	resp := postForm(t, client, server.URL, "/admin/pages/"+blogEntry.ID+"/publish", form)
	resp.Body.Close()
	time.Sleep(300 * time.Millisecond)

	// GET /blog should contain LAYOUT MARKER
	body := bodyString(t, getPath(t, client, server.URL, "/blog"))
	if !strings.Contains(body, "LAYOUT MARKER") {
		t.Fatalf("archive missing layout marker: %s", body)
	}

	// Modify template to VERSION 2, save draft (not publish), public should still be MARKER
	layoutDocV2 := `{"version":1,"nodes":[{"id":"h1","block":"core/heading","version":1,"props":{"text":"LAYOUT VERSION 2","level":1},"settings":{}},{"id":"slot1","block":"core/content-slot","version":1,"props":{},"settings":{}}]}`
	saveForm2 := url.Values{
		"name":          {"Archive Layout"},
		"document_json": {layoutDocV2},
		"csrf_token":    {csrfToken(t, client, server.URL, "/admin/appearance/templates/"+layoutID+"/edit")},
	}
	saveResp2 := postForm(t, client, server.URL, "/admin/appearance/templates/"+layoutID, saveForm2)
	saveResp2.Body.Close()
	time.Sleep(200 * time.Millisecond)
	body2 := bodyString(t, getPath(t, client, server.URL, "/blog"))
	if !strings.Contains(body2, "LAYOUT MARKER") {
		t.Fatalf("draft leaked: %s", body2)
	}
	if strings.Contains(body2, "LAYOUT VERSION 2") {
		t.Fatalf("draft should not be public")
	}

	// Publish V2
	publishForm2 := url.Values{
		"name":          {"Archive Layout"},
		"document_json": {layoutDocV2},
		"csrf_token":    {csrfToken(t, client, server.URL, "/admin/appearance/templates/"+layoutID+"/edit")},
	}
	pubResp2 := postForm(t, client, server.URL, "/admin/appearance/templates/"+layoutID+"/publish", publishForm2)
	pubResp2.Body.Close()
	time.Sleep(300 * time.Millisecond)
	body3 := bodyString(t, getPath(t, client, server.URL, "/blog"))
	if !strings.Contains(body3, "LAYOUT VERSION 2") {
		t.Fatalf("after publish, expected VERSION 2: %s", body3)
	}
}
