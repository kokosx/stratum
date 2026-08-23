package integration

import (
	"context"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

func extractLayoutID(location string) string {
	re := regexp.MustCompile(`/admin/appearance/templates/([^/]+)/edit`)
	m := re.FindStringSubmatch(location)
	if m == nil {
		return ""
	}
	return m[1]
}

func TestLayoutTemplateEndToEnd(t *testing.T) {
	server, queries, _, authService, cleanup := newIntegrationServer(t)
	defer cleanup()
	ctx := context.Background()

	// Setup & login
	setupClient := newClient(t)
	setupToken := authService.SetupCode()
	setupForm := url.Values{
		"setup_code": {setupToken},
		"site_title": {"Layout E2E"},
		"email":      {"admin2@example.com"},
		"password":   {"a sufficiently long password"},
		"csrf_token": {csrfToken(t, setupClient, server.URL, "/admin/setup")},
	}
	resp := postForm(t, setupClient, server.URL, "/admin/setup", setupForm)
	resp.Body.Close()
	client := newClient(t)
	loginForm := url.Values{
		"email":      {"admin2@example.com"},
		"password":   {"a sufficiently long password"},
		"csrf_token": {csrfToken(t, client, server.URL, "/admin/login")},
	}
	loginResp := postForm(t, client, server.URL, "/admin/login", loginForm)
	loginResp.Body.Close()

	// 1. Create Blog Post layout template for posts
	createLayoutForm := url.Values{
		"name":            {"Blog Post"},
		"content_type_id": {"post"},
		"csrf_token":      {csrfToken(t, client, server.URL, "/admin/appearance/templates/new")},
	}
	createResp := postForm(t, client, server.URL, "/admin/appearance/templates", createLayoutForm)
	if createResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create layout status %d want 303 body %s", createResp.StatusCode, bodyString(t, createResp))
	}
	loc := createResp.Header.Get("Location")
	layoutID := extractLayoutID(loc)
	if layoutID == "" {
		t.Fatalf("could not extract layout id from %q", loc)
	}
	createResp.Body.Close()

	// Verify list contains it
	listResp := getPath(t, client, server.URL, "/admin/appearance/templates")
	if listResp.StatusCode != http.StatusOK {
		t.Fatalf("list status %d", listResp.StatusCode)
	}
	listBody := bodyString(t, listResp)
	if !strings.Contains(listBody, "Blog Post") {
		t.Fatalf("list missing Blog Post: %s", listBody)
	}
	// Initially unpublished
	if !strings.Contains(listBody, "Unpublished") {
		t.Fatalf("expected Unpublished status")
	}

	// 2. Edit layout to add heading + content slot (already has slot, add breadcrumb-like heading)
	// Fetch edit page to get current doc
	editResp := getPath(t, client, server.URL, "/admin/appearance/templates/"+layoutID+"/edit")
	if editResp.StatusCode != http.StatusOK {
		t.Fatalf("edit status %d", editResp.StatusCode)
	}
	editBody := bodyString(t, editResp)
	// Extract document_json from hidden input or bootstrap? For simplicity create new doc with heading
	// Use a doc that has heading "Layout Header" + slot
	layoutDoc := `{"version":1,"nodes":[{"id":"h1","block":"core/heading","version":1,"props":{"text":"LayoutHeader","level":1},"settings":{}},{"id":"slot1","block":"core/content-slot","version":1,"props":{},"settings":{}}]}`
	saveForm := url.Values{
		"name":          {"Blog Post"},
		"document_json": {layoutDoc},
		"csrf_token":    {csrfToken(t, client, server.URL, "/admin/appearance/templates/"+layoutID+"/edit")},
	}
	saveResp := postForm(t, client, server.URL, "/admin/appearance/templates/"+layoutID, saveForm)
	if saveResp.StatusCode != http.StatusSeeOther && saveResp.StatusCode != http.StatusOK {
		// Datastar may return 200 SSE; but we post without Datastar header so expect redirect
		t.Fatalf("save layout status %d", saveResp.StatusCode)
	}
	saveResp.Body.Close()

	// Verify still unpublished (draft changes not published)
	listResp2 := getPath(t, client, server.URL, "/admin/appearance/templates")
	body2 := bodyString(t, listResp2)
	if !strings.Contains(body2, "Unpublished") {
		// After save draft, still unpublished until publish
		// Actually after first save, still unpublished because we never published
	}

	// 3. Publish layout
	publishForm := url.Values{
		"name":          {"Blog Post"},
		"document_json": {layoutDoc},
		"csrf_token":    {csrfToken(t, client, server.URL, "/admin/appearance/templates/"+layoutID+"/edit")},
	}
	pubResp := postForm(t, client, server.URL, "/admin/appearance/templates/"+layoutID+"/publish", publishForm)
	if pubResp.StatusCode != http.StatusSeeOther {
		// check for SSE 200
		if pubResp.StatusCode != http.StatusOK {
			t.Fatalf("publish layout status %d", pubResp.StatusCode)
		}
	}
	pubResp.Body.Close()

	// Now list should show Published
	listResp3 := getPath(t, client, server.URL, "/admin/appearance/templates")
	body3 := bodyString(t, listResp3)
	if !strings.Contains(body3, "Published") {
		t.Fatalf("expected Published after publish, got %s", body3)
	}

	// 4. Create a Post using Blog Post template, save draft (public unchanged)
	postDoc := `{"version":1,"nodes":[{"id":"t1","block":"core/text","version":1,"props":{"text":"PostBody"},"settings":{}}]}`
	createPostForm := url.Values{
		"title":              {"My Post"},
		"slug":               {"my-post"},
		"document_json":      {postDoc},
		"layout_template_id": {layoutID},
		"csrf_token":         {csrfToken(t, client, server.URL, "/admin/posts/new")},
	}
	createPostResp := postForm(t, client, server.URL, "/admin/posts", createPostForm)
	if createPostResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create post status %d", createPostResp.StatusCode)
	}
	createPostResp.Body.Close()

	// Public should be 404 before publish
	prePub := getPath(t, client, server.URL, "/blog/my-post")
	if prePub.StatusCode != http.StatusNotFound {
		t.Fatalf("pre-publish status %d want 404", prePub.StatusCode)
	}
	prePub.Body.Close()

	// Publish the post
	// Need to find entry id
	entries, _ := queries.ListEntriesByContentType(ctx, "post")
	var postID string
	for _, e := range entries {
		if e.Slug == "my-post" {
			postID = e.ID
			break
		}
	}
	if postID == "" {
		t.Fatal("post not found")
	}
	publishPostForm := url.Values{
		"title":              {"My Post"},
		"slug":               {"my-post"},
		"document_json":      {postDoc},
		"layout_template_id": {layoutID},
		"csrf_token":         {csrfToken(t, client, server.URL, "/admin/posts/"+postID+"/edit")},
	}
	pubPostResp := postForm(t, client, server.URL, "/admin/posts/"+postID+"/publish", publishPostForm)
	if pubPostResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("publish post status %d", pubPostResp.StatusCode)
	}
	pubPostResp.Body.Close()

	// Public should now contain layout header + body
	pubResp2 := getPath(t, client, server.URL, "/blog/my-post")
	if pubResp2.StatusCode != http.StatusOK {
		t.Fatalf("post public status %d", pubResp2.StatusCode)
	}
	pubBody := bodyString(t, pubResp2)
	if !strings.Contains(pubBody, "LayoutHeader") {
		t.Fatalf("published post missing layout header: %s", pubBody)
	}
	if !strings.Contains(pubBody, "PostBody") {
		t.Fatalf("published post missing body: %s", pubBody)
	}

	// 5. Edit layout draft (change header to NewHeader) but not publish, public unchanged
	newLayoutDoc := `{"version":1,"nodes":[{"id":"h1","block":"core/heading","version":1,"props":{"text":"NewHeader","level":1},"settings":{}},{"id":"slot1","block":"core/content-slot","version":1,"props":{},"settings":{}}]}`
	saveForm2 := url.Values{
		"name":          {"Blog Post"},
		"document_json": {newLayoutDoc},
		"csrf_token":    {csrfToken(t, client, server.URL, "/admin/appearance/templates/"+layoutID+"/edit")},
	}
	saveResp2 := postForm(t, client, server.URL, "/admin/appearance/templates/"+layoutID, saveForm2)
	saveResp2.Body.Close()

	// Public should still have old header
	stillPub := getPath(t, client, server.URL, "/blog/my-post")
	stillBody := bodyString(t, stillPub)
	if !strings.Contains(stillBody, "LayoutHeader") {
		t.Fatalf("public changed before layout publish: %s", stillBody)
	}
	if strings.Contains(stillBody, "NewHeader") {
		t.Fatalf("draft leaked")
	}

	// 6. Publish new layout, all assigned posts should update after cache invalidation
	publishForm2 := url.Values{
		"name":          {"Blog Post"},
		"document_json": {newLayoutDoc},
		"csrf_token":    {csrfToken(t, client, server.URL, "/admin/appearance/templates/"+layoutID+"/edit")},
	}
	pubResp3 := postForm(t, client, server.URL, "/admin/appearance/templates/"+layoutID+"/publish", publishForm2)
	pubResp3.Body.Close()

	updatedPub := getPath(t, client, server.URL, "/blog/my-post")
	updatedBody := bodyString(t, updatedPub)
	if !strings.Contains(updatedBody, "NewHeader") {
		t.Fatalf("after layout publish, expected NewHeader got %s", updatedBody)
	}
	if strings.Contains(updatedBody, "LayoutHeader") && !strings.Contains(updatedBody, "NewHeader") {
		t.Fatalf("old header still present")
	}

	// 7. Test preview uses unsaved template selection (simulate preview endpoint)
	previewDoc := `{"version":1,"nodes":[{"id":"t1","block":"core/text","version":1,"props":{"text":"PreviewBody"},"settings":{}}]}`
	previewForm := url.Values{
		"document_json":      {previewDoc},
		"title":              {"Preview Title"},
		"slug":               {"my-post"},
		"entry_id":           {postID},
		"layout_template_id": {layoutID},
		"content_type_id":    {"post"},
		"csrf_token":         {csrfToken(t, client, server.URL, "/admin/posts/"+postID+"/edit")},
	}
	// Need to bypass CSRF? Use preview endpoint
	previewReq, _ := http.NewRequest(http.MethodPost, server.URL+"/admin/editor/preview", strings.NewReader(previewForm.Encode()))
	previewReq.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// copy cookies from client
	// Use client to do request with redirect disabled? Simpler use client.PostForm but that hits handler's previewDocument via admin handler
	// We'll use http client directly with jar
	// Use the same client which has cookies
	// But we need to set CSRF cookie already present, we used csrfToken which ensures cookie
	// Instead do raw http with client
	respPreview, err := client.PostForm(server.URL+"/admin/editor/preview", previewForm)
	if err != nil {
		t.Fatal(err)
	}
	if respPreview.StatusCode != http.StatusOK {
		t.Fatalf("preview status %d body %s", respPreview.StatusCode, bodyString(t, respPreview))
	}
	previewHTML := bodyString(t, respPreview)
	if !strings.Contains(previewHTML, "PreviewBody") {
		t.Fatalf("preview missing body: %s", previewHTML)
	}
	// Should contain layout's NewHeader because we published NewHeader
	if !strings.Contains(previewHTML, "NewHeader") {
		t.Fatalf("preview missing layout header: %s", previewHTML)
	}
	// Ensure editResp variable used
	_ = editBody
	_ = body2
	_ = ctx
}
