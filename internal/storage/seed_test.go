package storage

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/document"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func TestSeedCreatesAnIdempotentPublishedHomepage(t *testing.T) {
	ctx := context.Background()
	database, err := Open(filepath.Join(t.TempDir(), "data", "stratum.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })

	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := database.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	if err := database.Seed(ctx); err != nil {
		t.Fatal(err)
	}

	queries := db.New(database.DB)
	entry, err := queries.GetPublishedEntryByPath(ctx, "/")
	if err != nil {
		t.Fatal(err)
	}
	if entry.Title != "Welcome to StratumCMS" {
		t.Errorf("seeded title = %q, want Welcome to StratumCMS", entry.Title)
	}
	doc, err := document.Decode([]byte(entry.DocumentJson))
	if err != nil {
		t.Fatal(err)
	}
	registry, err := blocks.NewRegistry(ctx, queries)
	if err != nil {
		t.Fatal(err)
	}
	content, err := registry.RenderDocument(doc)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), `stratum-align-left`) || !strings.Contains(string(content), `Welcome to StratumCMS`) {
		t.Errorf("seeded templates rendered %q", content)
	}

	settings, err := queries.GetSiteSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if !settings.HomepageEntryID.Valid || settings.HomepageEntryID.String != seedHomeEntryID {
		t.Errorf("homepage entry = %#v, want %q", settings.HomepageEntryID, seedHomeEntryID)
	}
}

func TestSeedBlogSlugIsPublicSemantics(t *testing.T) {
	ctx := context.Background()
	database, err := Open(filepath.Join(t.TempDir(), "data", "stratum.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := database.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	queries := db.New(database.DB)
	// Blog entry slug must be "blog" (public semantics), not internal seed ID
	blogEntry, err := queries.GetEntry(ctx, seedBlogEntryID)
	if err != nil {
		t.Fatal(err)
	}
	if blogEntry.Slug != "blog" {
		t.Fatalf("Blog Entry slug = %q, want %q", blogEntry.Slug, "blog")
	}
	settings, err := queries.GetSiteSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if settings.PostsBasePath != "/blog" {
		t.Fatalf("site_settings.posts_base_path = %q, want /blog", settings.PostsBasePath)
	}
	archRoute, err := queries.GetRouteByPath(ctx, "/blog")
	if err != nil {
		t.Fatalf("archive route /blog not found: %v", err)
	}
	if archRoute.RouteType != "archive" {
		t.Fatalf("route /blog type = %q, want archive", archRoute.RouteType)
	}
	if !archRoute.EntryID.Valid || archRoute.EntryID.String != seedBlogEntryID {
		t.Fatalf("archive entry = %#v, want %q", archRoute.EntryID, seedBlogEntryID)
	}
	if !archRoute.ContentTypeID.Valid || archRoute.ContentTypeID.String != "post" {
		t.Fatalf("archive content_type = %#v, want post", archRoute.ContentTypeID)
	}
	// Verify persisted source-of-truth values are coherent (slug is public semantics, not internal ID).
	derived := "/" + strings.Trim(blogEntry.Slug, "/")
	if derived != "/blog" {
		t.Fatalf("derived PostsBasePath = %q, want /blog", derived)
	}
	if derived != settings.PostsBasePath {
		t.Fatalf("derived %q != stored %q, Settings save would move archive", derived, settings.PostsBasePath)
	}
	// Also verify archive still at /blog after no-op
	archRoute2, err := queries.GetRouteByPath(ctx, "/blog")
	if err != nil {
		t.Fatal(err)
	}
	if archRoute2.Path != "/blog" {
		t.Fatalf("after no-op, archive path = %q, want /blog", archRoute2.Path)
	}
}
