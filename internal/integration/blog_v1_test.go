package integration

import (
	"context"
	"database/sql"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

// blogDoc returns a Blog Page SDT with entry-title, text, posts block (archive) and CTA button.
// Used to verify that posts list appears exactly where the block is placed (heading -> posts -> CTA order).
func blogDoc() string {
	return `{"version":1,"nodes":[{"id":"s1","block":"core/section","version":1,"props":{},"settings":{"width":"content","verticalSpacing":"md","horizontalPadding":"md","align":"left","background":"default","minHeight":"auto","anchorID":""},"children":[{"id":"et1","block":"core/entry-title","version":1,"props":{},"settings":{"level":1,"align":"left","visualSize":"auto","tone":"default","maxWidth":"none"}},{"id":"t1","block":"core/text","version":1,"props":{"text":"Welcome to the blog"},"settings":{"align":"left","tone":"default","size":"md","maxWidth":"none"}},{"id":"p1","block":"core/posts","version":1,"props":{},"settings":{"source":"archive","layout":"list","columns":1,"showImage":true,"showDate":true,"showExcerpt":true,"pagination":true}},{"id":"cta1","block":"core/button","version":1,"props":{"label":"Subscribe","url":"/subscribe"},"settings":{"variant":"primary","size":"md","width":"auto","align":"left","openInNewTab":false}}]}]}`
}

func simpleDoc(text string) string {
	return `{"version":1,"nodes":[{"id":"s1","block":"core/section","version":1,"props":{},"settings":{"width":"content","verticalSpacing":"md","horizontalPadding":"md","align":"left","background":"default","minHeight":"auto","anchorID":""},"children":[{"id":"t1","block":"core/text","version":1,"props":{"text":` + mustJSON(text) + `},"settings":{"align":"left","tone":"default","size":"md","maxWidth":"none"}}]}]}`
}

func createAndPublishPage(t *testing.T, client *http.Client, serverURL, title, slug, docJSON string) string {
	t.Helper()
	ctx := context.Background()
	_ = ctx
	form := url.Values{
		"title":         {title},
		"slug":          {slug},
		"document_json": {docJSON},
		"csrf_token":    {csrfToken(t, client, serverURL, "/admin/pages/new")},
		"publish":       {"1"},
	}
	resp := postForm(t, client, serverURL, "/admin/pages", form)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create page %s status = %d, want 303; body=%s", slug, resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
	// entry ID via DB lookup for the test server? need queries – caller passes
	return slug
}

func publishPostViaAPI(t *testing.T, client *http.Client, serverURL, title, slug, docJSON string) {
	t.Helper()
	form := url.Values{
		"title":         {title},
		"slug":          {slug},
		"document_json": {docJSON},
		"csrf_token":    {csrfToken(t, client, serverURL, "/admin/posts/new")},
		"publish":       {"1"},
	}
	resp := postForm(t, client, serverURL, "/admin/posts", form)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("create post %s status = %d, want 303; body=%s", slug, resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
}

func saveReadingSettings(t *testing.T, client *http.Client, serverURL string, queries *db.Queries, homeID, blogID, base, perPage string) {
	t.Helper()
	row, _ := queries.GetSiteSettings(context.Background())
	csrf := csrfToken(t, client, serverURL, "/admin/settings")
	form := url.Values{
		"site_title":             {row.SiteTitle},
		"tagline":                {row.SiteTagline},
		"site_url":               {serverURL},
		"language":               {row.Language},
		"timezone":               {row.Timezone},
		"site_represents":        {"organization"},
		"indexing_enabled":       {"on"},
		"sitemap_enabled":        {"on"},
		"robots_mode":            {row.RobotsMode},
		"robots_custom":          {row.RobotsCustom},
		"speculation_mode":       {row.SpeculationMode},
		"speculation_eagerness":  {row.SpeculationEagerness},
		"title_separator":        {row.TitleSeparator},
		"homepage_mode_choice":   {"page"},
		"homepage_entry_id":      {homeID},
		"posts_page_entry_id":    {blogID},
		"posts_base_path":        {base},
		"posts_per_page":         {perPage},
		"csrf_token":             {csrf},
	}
	if row.SiteTitle == "" {
		form.Set("site_title", "Test Site")
	}
	if row.Language == "" {
		form.Set("language", "en")
	}
	if row.Timezone == "" {
		form.Set("timezone", "UTC")
	}
	if row.RobotsMode == "" {
		form.Set("robots_mode", "managed")
	}
	if row.SpeculationMode == "" {
		form.Set("speculation_mode", "off")
	}
	if row.SpeculationEagerness == "" {
		form.Set("speculation_eagerness", "conservative")
	}
	if row.TitleSeparator == "" {
		form.Set("title_separator", "–")
	}
	// Ensure required toggles
	form.Set("indexing_enabled", "on")
	form.Set("sitemap_enabled", "on")
	resp := postForm(t, client, serverURL, "/admin/settings", form)
	if resp.StatusCode != http.StatusSeeOther {
		body := bodyString(t, resp)
		t.Fatalf("save reading status = %d, want 303; body=%s", resp.StatusCode, body)
	}
	resp.Body.Close()
}

// TestBlogV1HappyPath is the full end-to-end acceptance test for Blog V1 per spec 16.
// It drives the UI over HTTP and checks public URLs, routing, rendering and SEO.
func TestBlogV1HappyPath(t *testing.T) {
	server, queries, _, authService, cleanup := newIntegrationServer(t)
	defer cleanup()
	ctx := context.Background()
	client := setupAndLogin(t, server.URL, authService)

	// 1. Create + Publish Page "Home"
	createAndPublishPage(t, client, server.URL, "Home", "home-page", simpleDoc("Home content"))
	time.Sleep(1100 * time.Millisecond)
	homeEntry, err := queries.GetEntryBySlug(ctx, db.GetEntryBySlugParams{ContentTypeID: "page", Slug: "home-page"})
	if err != nil {
		t.Fatalf("home entry not found: %v", err)
	}

	// 2. Create + Publish Page "Blog" with posts block (use a unique slug to avoid colliding with seeded /blog)
	// Seed creates a blog page at /blog; remove it so this test can create its own and verify routing from scratch.
	if e, err := queries.GetEntry(ctx, "seed-blog"); err == nil {
		t.Logf("deleting seeded blog entry %s slug=%s", e.ID, e.Slug)
		_ = queries.DeleteEntry(ctx, "seed-blog")
	}
	if rt, rerr := queries.GetRouteByPath(ctx, "/blog"); rerr == nil {
		t.Logf("deleting leftover route at /blog type=%s entry=%v", rt.RouteType, rt.EntryID)
		_ = queries.DeleteRoute(ctx, rt.ID)
	}
createAndPublishPage(t, client, server.URL, "Blog", "blog", blogDoc())
	time.Sleep(1100 * time.Millisecond)
	blogEntry, err := queries.GetEntryBySlug(ctx, db.GetEntryBySlugParams{ContentTypeID: "page", Slug: "blog"})
	if err != nil {
		t.Fatalf("blog entry not found: %v", err)
	}

	// 3. Save Reading: Homepage=Home, Posts Page=Blog, Base /blog, per page 2
	saveReadingSettings(t, client, server.URL, queries, homeEntry.ID, blogEntry.ID, "/blog", "2")
	// Reload hub site snapshot is done by saveSettings handler (uses shared hub)

	// Verify archive route was atomically converted to archive with shell
	rt, err := queries.GetRouteByPath(ctx, "/blog")
	if err != nil {
		t.Fatalf("archive route /blog not found: %v", err)
	}
	if rt.RouteType != "archive" {
		t.Fatalf("archive route type = %s, want archive (entry %s, redirect %v, path %s)", rt.RouteType, rt.EntryID.String, rt.RedirectTo.String, rt.Path)
	}
	if !rt.EntryID.Valid || rt.EntryID.String != blogEntry.ID {
		t.Fatalf("archive entry_id = %v, want %s", rt.EntryID, blogEntry.ID)
	}
	// Blog page should not have a duplicate single route at its old slug path (same as archive path here, so ok)
	// but ensure no separate entry route exists at /blog as entry
	if r2, err := queries.GetEntryRoute(ctx, sql.NullString{String: blogEntry.ID, Valid: true}); err == nil && r2.Path != "/blog" {
		t.Fatalf("posts page duplicate single route found at %s", r2.Path)
	}

	// 4. Publish: Post A, B, C
	publishPostViaAPI(t, client, server.URL, "Post A", "post-a", simpleDoc("Post A content"))
	time.Sleep(1100 * time.Millisecond)
	publishPostViaAPI(t, client, server.URL, "Post B", "post-b", simpleDoc("Post B content"))
	time.Sleep(1100 * time.Millisecond)
	publishPostViaAPI(t, client, server.URL, "Post C", "post-c", simpleDoc("Post C content"))
	time.Sleep(1100 * time.Millisecond)

	// Verify routes are /blog/slug (central routing policy)
	for _, slug := range []string{"post-a", "post-b", "post-c"} {
		p, _ := queries.GetEntryBySlug(ctx, db.GetEntryBySlugParams{ContentTypeID: "post", Slug: slug})
		r, err := queries.GetEntryRoute(ctx, sql.NullString{String: p.ID, Valid: true})
		if err != nil {
			t.Fatalf("post %s route not found: %v", slug, err)
		}
		expected := "/blog/" + slug
		if r.Path != expected {
			t.Fatalf("post %s route = %s, want %s", slug, r.Path, expected)
		}
	}

	// 5. Checks
	// GET / -> Home
	resp := getPath(t, client, server.URL, "/")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET / status = %d, want 200 body=%s", resp.StatusCode, bodyString(t, resp))
	}
	body := bodyString(t, resp)
	if !strings.Contains(body, "Home content") {
		t.Fatalf("GET / missing Home content: %s", body)
	}
	// Ensure home not showing blog posts (should be single)
	if strings.Contains(body, "Post C") {
		t.Fatalf("GET / should not contain blog posts")
	}

	// GET /blog -> archive Blog shell + Post C + B (per page 2), ordered newest first
	resp = getPath(t, client, server.URL, "/blog")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /blog status = %d, want 200", resp.StatusCode)
	}
	body = bodyString(t, resp)
	if !strings.Contains(body, "Welcome to the blog") {
		t.Fatalf("GET /blog missing blog intro/text")
	}
	// Check block order: Heading -> posts -> CTA vs template auto-list
	// Our blogDoc has entry-title, text, posts, button. Verify order in HTML:
	headIdx := strings.Index(body, "Welcome to the blog")
	postsIdx := strings.Index(body, "Post C")
	ctaIdx := strings.Index(body, "Subscribe")
	if headIdx == -1 || postsIdx == -1 || ctaIdx == -1 {
		t.Fatalf("GET /blog missing expected sections head=%d posts=%d cta=%d body=%s", headIdx, postsIdx, ctaIdx, body)
	}
	if !(headIdx < postsIdx && postsIdx < ctaIdx) {
		t.Fatalf("GET /blog order wrong: heading %d posts %d cta %d", headIdx, postsIdx, ctaIdx)
	}
	if !strings.Contains(body, "Post C") || !strings.Contains(body, "Post B") {
		t.Fatalf("GET /blog missing expected posts C+B: %s", body)
	}
	if strings.Contains(body, "Post A") {
		t.Fatalf("GET /blog page 1 should not contain Post A (perPage 2)")
	}
	// Verify CSS for archive shell UsedBlocks (posts block present)
	if !strings.Contains(body, "/stratum/blocks.") {
		t.Fatalf("GET /blog missing blocks CSS")
	}
	// Verify Post URLs use route_path (DB source of truth) – check hrefs are /blog/...
	if !strings.Contains(body, `href="/blog/post-c"`) {
		t.Fatalf("GET /blog card URL not using route_path, body=%s", body)
	}

	// GET /blog/page/2 -> Post A
	resp = getPath(t, client, server.URL, "/blog/page/2")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /blog/page/2 status = %d, want 200", resp.StatusCode)
	}
	body = bodyString(t, resp)
	if !strings.Contains(body, "Post A") {
		t.Fatalf("GET /blog/page/2 missing Post A: %s", body)
	}
	if strings.Contains(body, "Post B") || strings.Contains(body, "Post C") {
		t.Fatalf("GET /blog/page/2 should not contain B/C")
	}

	// GET /blog/page/1 -> 301 /blog
	redirectClient := newClient(t)
	resp = getPath(t, redirectClient, server.URL, "/blog/page/1")
	if resp.StatusCode != http.StatusMovedPermanently {
		t.Fatalf("GET /blog/page/1 status = %d, want 301", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/blog" {
		t.Fatalf("GET /blog/page/1 Location = %q, want /blog", loc)
	}
	resp.Body.Close()

	// Invalid pagination -> 404
	for _, p := range []string{"/blog/page/0", "/blog/page/-1", "/blog/page/foo", "/blog/page/999"} {
		resp = getPath(t, client, server.URL, p)
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("GET %s status = %d, want 404", p, resp.StatusCode)
		}
		resp.Body.Close()
	}

	// GET /blog/post-a -> Post A single
	resp = getPath(t, client, server.URL, "/blog/post-a")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /blog/post-a status = %d, want 200", resp.StatusCode)
	}
	body = bodyString(t, resp)
	if !strings.Contains(body, "Post A content") {
		t.Fatalf("GET /blog/post-a missing content")
	}
	// Ensure page kind is single post (content type post) and not archive
	if !strings.Contains(body, "Post A") {
		t.Fatalf("single post missing title")
	}

	// GET /post-a (old slug before base) – never existed, should be 404 (no history yet)
	resp = getPath(t, redirectClient, server.URL, "/post-a")
	if resp.StatusCode != http.StatusNotFound {
		// Could be 301 if history exists from earlier slug change – accept either but not 200
		if resp.StatusCode == http.StatusOK {
			t.Fatalf("GET /post-a should not be 200")
		}
	}
	resp.Body.Close()

	// GET /feed.xml -> correct Post URLs
	resp = getPath(t, client, server.URL, "/feed.xml")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /feed.xml status = %d, want 200", resp.StatusCode)
	}
	feed := bodyString(t, resp)
	for _, u := range []string{"/blog/post-a", "/blog/post-b", "/blog/post-c"} {
		if !strings.Contains(feed, u) {
			t.Fatalf("feed missing %s: %s", u, feed)
		}
	}
	// Ensure feed not containing guessed /post-a without base
	if strings.Contains(feed, `">/post-a<`) || strings.Contains(feed, `"/post-a"`) {
		t.Fatalf("feed contains non-prefixed post URL")
	}

	// GET /sitemap.xml -> /blog + /blog/post-a etc., no pagination, no redirects
	resp = getPath(t, client, server.URL, "/sitemap.xml")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /sitemap.xml status = %d, want 200", resp.StatusCode)
	}
	sm := bodyString(t, resp)
	if !strings.Contains(sm, "/blog</loc>") && !strings.Contains(sm, "/blog<") {
		t.Fatalf("sitemap missing /blog archive: %s", sm)
	}
	for _, u := range []string{"/blog/post-a", "/blog/post-b", "/blog/post-c"} {
		if !strings.Contains(sm, u) {
			t.Fatalf("sitemap missing %s: %s", u, sm)
		}
	}
	if strings.Contains(sm, "/blog/page/2") {
		t.Fatalf("sitemap should not contain pagination: %s", sm)
	}

	// 6. Draft update Blog Page: should not change /blog
	draftDoc := `{"version":1,"nodes":[{"id":"s1","block":"core/section","version":1,"props":{},"settings":{"width":"content","verticalSpacing":"md","horizontalPadding":"md","align":"left","background":"default","minHeight":"auto","anchorID":""},"children":[{"id":"t1","block":"core/text","version":1,"props":{"text":"Draft blog intro changed"},"settings":{"align":"left","tone":"default","size":"md","maxWidth":"none"}}]}]}`
	saveForm := url.Values{
		"title":         {"Blog"},
		"slug":          {"blog"},
		"document_json": {draftDoc},
		"csrf_token":    {csrfToken(t, client, server.URL, "/admin/pages/"+blogEntry.ID+"/edit")},
	}
	saveResp := postForm(t, client, server.URL, "/admin/pages/"+blogEntry.ID, saveForm)
	if saveResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("save draft blog status = %d, want 303", saveResp.StatusCode)
	}
	saveResp.Body.Close()
	// GET /blog still shows old published intro
	resp = getPath(t, client, server.URL, "/blog")
	body = bodyString(t, resp)
	if strings.Contains(body, "Draft blog intro changed") {
		t.Fatalf("draft leaked to public /blog")
	}
	if !strings.Contains(body, "Welcome to the blog") {
		t.Fatalf("published /blog changed after draft")
	}

	// 7. Publish Blog revision: archive changes
	publishForm := url.Values{
		"title":         {"Blog"},
		"slug":          {"blog"},
		"document_json": {draftDoc},
		"csrf_token":    {csrfToken(t, client, server.URL, "/admin/pages/"+blogEntry.ID+"/edit")},
	}
	pubResp := postForm(t, client, server.URL, "/admin/pages/"+blogEntry.ID+"/publish", publishForm)
	if pubResp.StatusCode != http.StatusSeeOther {
		t.Fatalf("publish blog status = %d, want 303", pubResp.StatusCode)
	}
	pubResp.Body.Close()
	resp = getPath(t, client, server.URL, "/blog")
	body = bodyString(t, resp)
	if !strings.Contains(body, "Draft blog intro changed") {
		t.Fatalf("published blog update not visible: %s", body)
	}

	// 8. Archive path is derived from Posts Page slug, so PostsBase is synced.
	// Changing base to /news while PostsPage slug is still "blog" should keep archive at /blog.
	saveReadingSettings(t, client, server.URL, queries, homeEntry.ID, blogEntry.ID, "/news", "2")
	resp = getPath(t, client, server.URL, "/blog")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /blog after ignored base change status = %d, want 200", resp.StatusCode)
	}
	body = bodyString(t, resp)
	if !strings.Contains(body, "Draft blog intro changed") {
		t.Fatalf("GET /blog missing shell after base change (should remain)")
	}
	resp = getPath(t, client, server.URL, "/blog/post-a")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /blog/post-a still should be 200 after base ignored, got %d", resp.StatusCode)
	}
	resp.Body.Close()
	// /news should not be archive when derived base is /blog
	resp = getPath(t, redirectClient, server.URL, "/news")
	if resp.StatusCode == http.StatusOK {
		t.Fatalf("GET /news should not be 200 when archive is /blog")
	}
	resp.Body.Close()
	// Now change Blog slug to "news" via direct entry update to move archive to /news (derived)
	// Simulate slug change: edit Blog page
	form2 := url.Values{
		"title":         {"Blog"},
		"slug":          {"news"},
		"document_json": {draftDoc},
		"csrf_token":    {csrfToken(t, client, server.URL, "/admin/pages/"+blogEntry.ID+"/edit")},
	}
	resp2 := postForm(t, client, server.URL, "/admin/pages/"+blogEntry.ID+"/publish", form2)
	if resp2.StatusCode != http.StatusSeeOther {
		t.Fatalf("publish blog slug news status = %d", resp2.StatusCode)
	}
	resp2.Body.Close()
	time.Sleep(200 * time.Millisecond)
	// After slug change, re-save reading settings to sync derived base (still PostsPage=Blog)
	saveReadingSettings(t, client, server.URL, queries, homeEntry.ID, blogEntry.ID, "/news", "2")
	resp = getPath(t, client, server.URL, "/news")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /news after slug change status = %d, want 200", resp.StatusCode)
	}
	body = bodyString(t, resp)
	if !strings.Contains(body, "Draft blog intro changed") {
		t.Fatalf("GET /news missing blog shell after slug change")
	}
	resp = getPath(t, client, server.URL, "/news/post-a")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /news/post-a status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()
	// Old /blog should redirect to /news now
	resp = getPath(t, redirectClient, server.URL, "/blog")
	if resp.StatusCode != http.StatusMovedPermanently {
		t.Fatalf("GET /blog after slug change status = %d, want 301", resp.StatusCode)
	}
	if loc := resp.Header.Get("Location"); loc != "/news" {
		t.Fatalf("GET /blog redirect Location = %q, want /news", loc)
	}
	resp.Body.Close()
	// No redirect chains: ensure /news/page/2 still works (if enough posts)
	resp = getPath(t, client, server.URL, "/news/page/2")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /news/page/2 status = %d, want 200", resp.StatusCode)
	}
	resp.Body.Close()
}
