package integration

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"

	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func TestCategoryArchiveUsesPublishedRevisionAssignments(t *testing.T) {
	server, queries, _, authService, cleanup := newIntegrationServer(t)
	defer cleanup()
	client := setupAndLogin(t, server.URL, authService)
	createTerm := func(name, slug string) db.Term {
		t.Helper()
		form := url.Values{"name": {name}, "slug": {slug}, "csrf_token": {csrfToken(t, client, server.URL, "/admin/posts/categories")}}
		resp := postForm(t, client, server.URL, "/admin/posts/categories", form)
		if resp.StatusCode != http.StatusSeeOther {
			t.Fatalf("create category %s: %d", slug, resp.StatusCode)
		}
		resp.Body.Close()
		term, err := queries.GetTermByTaxonomyAndSlug(context.Background(), db.GetTermByTaxonomyAndSlugParams{TaxonomyID: "category", Slug: slug})
		if err != nil {
			t.Fatal(err)
		}
		return term
	}
	a := createTerm("Category A", "category-a")
	b := createTerm("Category B", "category-b")
	publishForm := url.Values{
		"title": {"Taxonomy Post"}, "slug": {"taxonomy-post"}, "document_json": {simpleDoc("Taxonomy Post")},
		"taxonomy_category": {a.ID}, "csrf_token": {csrfToken(t, client, server.URL, "/admin/posts/new")}, "publish": {"1"},
	}
	resp := postForm(t, client, server.URL, "/admin/posts", publishForm)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("publish post: %d", resp.StatusCode)
	}
	resp.Body.Close()
	if count, err := queries.CountPublishedEntriesByTerm(context.Background(), a.ID); err != nil || count != 1 {
		t.Fatalf("published term count = %d, err=%v", count, err)
	}
	if body := bodyString(t, getPath(t, client, server.URL, "/category/category-a")); !strings.Contains(body, "Taxonomy Post") {
		t.Fatalf("published category missing post: %s", body)
	}
	post, err := queries.GetEntryBySlug(context.Background(), db.GetEntryBySlugParams{ContentTypeID: "post", Slug: "taxonomy-post"})
	if err != nil {
		t.Fatal(err)
	}
	draftForm := url.Values{
		"title": {"Taxonomy Post"}, "slug": {"taxonomy-post"}, "document_json": {simpleDoc("Taxonomy Post")},
		"taxonomy_category": {b.ID}, "csrf_token": {csrfToken(t, client, server.URL, "/admin/posts/"+post.ID+"/edit")},
	}
	resp = postForm(t, client, server.URL, "/admin/posts/"+post.ID, draftForm)
	if resp.StatusCode != http.StatusSeeOther {
		t.Fatalf("save draft: %d", resp.StatusCode)
	}
	resp.Body.Close()
	if body := bodyString(t, getPath(t, client, server.URL, "/category/category-a")); !strings.Contains(body, "Taxonomy Post") {
		t.Fatal("draft taxonomy assignment leaked from published revision")
	}
	if body := bodyString(t, getPath(t, client, server.URL, "/category/category-b")); strings.Contains(body, "Taxonomy Post") {
		t.Fatal("draft taxonomy assignment appeared in public archive")
	}
}
