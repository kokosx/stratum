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

// TestPostsPageSlugSyncOnPublish verifies P0 invariant:
// changing the Posts Page slug and publishing must immediately move the archive
// route, create a redirect from old archive, update posts_base_path and remap
// post singles, without requiring a separate Settings save.
func TestPostsPageSlugSyncOnPublish(t *testing.T) {
	server, queries, _, authService, cleanup := newIntegrationServer(t)
	defer cleanup()
	ctx := context.Background()
	client := setupAndLogin(t, server.URL, authService)

	// Create Home page
	createAndPublishPage(t, client, server.URL, "Home", "home-page", simpleDoc("Home content"))
	time.Sleep(100 * time.Millisecond)
	homeEntry, err := queries.GetEntryBySlug(ctx, db.GetEntryBySlugParams{ContentTypeID: "page", Slug: "home-page"})
	if err != nil {
		t.Fatalf("home entry not found: %v", err)
	}

	// Seed creates a blog page at /blog; remove it so this test can create its own and verify routing from scratch.
	if _, err := queries.GetEntry(ctx, "seed-blog"); err == nil {
		_ = queries.DeleteEntry(ctx, "seed-blog")
	}
	if rt, rerr := queries.GetRouteByPath(ctx, "/blog"); rerr == nil {
		_ = queries.DeleteRoute(ctx, rt.ID)
	}
	// Create Blog page slug=blog with archive block
	createAndPublishPage(t, client, server.URL, "Blog", "blog", blogDoc())
	time.Sleep(100 * time.Millisecond)
	blogEntry, err := queries.GetEntryBySlug(ctx, db.GetEntryBySlugParams{ContentTypeID: "page", Slug: "blog"})
	if err != nil {
		t.Fatalf("blog entry not found: %v", err)
	}

	// Assign Reading: Homepage=Home, Posts Page=Blog, base /blog
	saveReadingSettings(t, client, server.URL, queries, homeEntry.ID, blogEntry.ID, "/blog", "10")
	rt, err := queries.GetRouteByPath(ctx, "/blog")
	if err != nil || rt.RouteType != "archive" {
		t.Fatalf("archive /blog not ready: %v %+v", err, rt)
	}
	if rt.ContentTypeID.String != "post" {
		t.Fatalf("archive content_type_id = %q, want post", rt.ContentTypeID.String)
	}

	// Publish a post
	publishPostViaAPI(t, client, server.URL, "Hello Post", "hello-post", simpleDoc("Hello post content"))
	time.Sleep(100 * time.Millisecond)
	p, err := queries.GetEntryBySlug(ctx, db.GetEntryBySlugParams{ContentTypeID: "post", Slug: "hello-post"})
	if err != nil {
		t.Fatalf("post not found: %v", err)
	}
	pr, err := queries.GetEntryRoute(ctx, sql.NullString{String: p.ID, Valid: true})
	if err != nil {
		t.Fatalf("post route not found: %v", err)
	}
	if pr.Path != "/blog/hello-post" {
		t.Fatalf("post route = %s, want /blog/hello-post", pr.Path)
	}

	// Now change Blog slug to news via publish WITHOUT touching Settings
	form := url.Values{
		"title":         {"Blog"},
		"slug":          {"news"},
		"document_json": {blogDoc()},
		"csrf_token":    {csrfToken(t, client, server.URL, "/admin/pages/"+blogEntry.ID+"/edit")},
	}
	resp := postForm(t, client, server.URL, "/admin/pages/"+blogEntry.ID+"/publish", form)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("publish slug change status = %d, want 303; body=%s", resp.StatusCode, bodyString(t, resp))
	}
	resp.Body.Close()
	time.Sleep(200 * time.Millisecond)

	// /news should be archive immediately
	resp2 := getPath(t, client, server.URL, "/news")
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("GET /news after slug publish status = %d, want 200", resp2.StatusCode)
	}
	body := bodyString(t, resp2)
	if !strings.Contains(body, "Welcome to the blog") {
		t.Fatalf("GET /news missing blog shell content: %s", body)
	}

	// /blog should redirect to /news
	redirectClient := newClient(t)
	rOld := getPath(t, redirectClient, server.URL, "/blog")
	if rOld.StatusCode != http.StatusMovedPermanently {
		t.Fatalf("GET /blog after move status = %d, want 301", rOld.StatusCode)
	}
	if loc := rOld.Header.Get("Location"); loc != "/news" {
		t.Fatalf("GET /blog redirect Location = %q, want /news", loc)
	}
	rOld.Body.Close()

	// Settings posts_base_path should have been updated to /news atomically
	row, _ := queries.GetSiteSettings(ctx)
	if row.PostsBasePath != "/news" {
		t.Fatalf("site_settings posts_base_path = %q, want /news (should be synced on publish)", row.PostsBasePath)
	}

	// Post single should have been remapped to /news/hello-post
	pr2, err := queries.GetEntryRoute(ctx, sql.NullString{String: p.ID, Valid: true})
	if err != nil {
		t.Fatalf("post route after move not found: %v", err)
	}
	if pr2.Path != "/news/hello-post" {
		t.Fatalf("post route after move = %s, want /news/hello-post", pr2.Path)
	}
	resp3 := getPath(t, client, server.URL, "/news/hello-post")
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("GET /news/hello-post status = %d, want 200", resp3.StatusCode)
	}
	body3 := bodyString(t, resp3)
	if !strings.Contains(body3, "Hello post content") {
		t.Fatalf("GET /news/hello-post body missing content")
	}
	// Old post URL should redirect
	rOldPost := getPath(t, redirectClient, server.URL, "/blog/hello-post")
	if rOldPost.StatusCode != http.StatusMovedPermanently {
		t.Fatalf("GET /blog/hello-post status = %d, want 301", rOldPost.StatusCode)
	}
	if loc := rOldPost.Header.Get("Location"); loc != "/news/hello-post" {
		t.Fatalf("GET /blog/hello-post redirect = %q, want /news/hello-post", loc)
	}
	rOldPost.Body.Close()

	// Archive should still have content_type_id = post
	rt2, _ := queries.GetRouteByPath(ctx, "/news")
	if rt2.RouteType != "archive" || rt2.ContentTypeID.String != "post" {
		t.Fatalf("archive /news route not correct: %+v", rt2)
	}
}


