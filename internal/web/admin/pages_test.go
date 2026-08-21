package admin

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/kokosx/stratum/internal/document"
	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func TestWritePageKeepsDraftsSeparateFromPublishedRevision(t *testing.T) {
	ctx := context.Background()
	database, err := storage.Open(filepath.Join(t.TempDir(), "stratum.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	h, err := NewHandler(database.DB, db.New(database.DB), nil)
	if err != nil {
		t.Fatal(err)
	}

	entryID := "page-under-test"
	if err := h.writePage(ctx, "author", entryID, pageInput{title: "First", slug: "first", content: "draft one"}, true, false); err != nil {
		t.Fatal(err)
	}
	entry, err := h.queries.GetEntry(ctx, entryID)
	if err != nil {
		t.Fatal(err)
	}
	if entry.PublishedRevisionID.Valid {
		t.Fatal("new draft unexpectedly has a published revision")
	}
	assertLatestText(t, h.queries, entryID, 1, "draft one")

	if err := h.writePage(ctx, "author", entryID, pageInput{title: "Second", slug: "first", content: "draft two"}, false, false); err != nil {
		t.Fatal(err)
	}
	assertLatestText(t, h.queries, entryID, 2, "draft two")

	if err := h.writePage(ctx, "author", entryID, pageInput{title: "Published", slug: "published", content: "public version"}, false, true); err != nil {
		t.Fatal(err)
	}
	published, err := h.queries.GetEntry(ctx, entryID)
	if err != nil {
		t.Fatal(err)
	}
	if !published.PublishedRevisionID.Valid {
		t.Fatal("published page has no published revision")
	}
	publishedRevisionID := published.PublishedRevisionID.String
	assertLatestText(t, h.queries, entryID, 3, "public version")
	route, err := h.queries.GetRouteByPath(ctx, "/published")
	if err != nil || !route.EntryID.Valid || route.EntryID.String != entryID {
		t.Fatalf("published route = %#v, %v", route, err)
	}

	if err := h.writePage(ctx, "author", entryID, pageInput{title: "New draft", slug: "published", content: "not public yet"}, false, false); err != nil {
		t.Fatal(err)
	}
	entry, err = h.queries.GetEntry(ctx, entryID)
	if err != nil {
		t.Fatal(err)
	}
	if !entry.PublishedRevisionID.Valid || entry.PublishedRevisionID.String != publishedRevisionID {
		t.Fatalf("published revision changed after saving draft: %#v", entry.PublishedRevisionID)
	}
	public, err := h.queries.GetPublishedEntryByPath(ctx, "/published")
	if err != nil {
		t.Fatal(err)
	}
	if content, err := textContent(public.DocumentJson); err != nil || content != "public version" {
		t.Fatalf("public content = %q, %v", content, err)
	}
	assertLatestText(t, h.queries, entryID, 4, "not public yet")
	revisions, err := h.queries.ListEntryRevisions(ctx, entryID)
	if err != nil {
		t.Fatal(err)
	}
	var nodeID string
	for _, revision := range revisions {
		doc, err := document.Decode([]byte(revision.DocumentJson))
		if err != nil {
			t.Fatal(err)
		}
		if nodeID == "" {
			nodeID = doc.Nodes[0].ID
			continue
		}
		if doc.Nodes[0].ID != nodeID {
			t.Fatalf("node ID changed between revisions: got %q, want %q", doc.Nodes[0].ID, nodeID)
		}
	}
}

func assertLatestText(t *testing.T, queries *db.Queries, entryID string, revisionNumber int64, want string) {
	t.Helper()
	revision, err := queries.GetLatestEntryRevision(context.Background(), entryID)
	if err != nil {
		t.Fatal(err)
	}
	if revision.RevisionNumber != revisionNumber {
		t.Fatalf("revision number = %d, want %d", revision.RevisionNumber, revisionNumber)
	}
	doc, err := document.Decode([]byte(revision.DocumentJson))
	if err != nil {
		t.Fatal(err)
	}
	if len(doc.Nodes) != 1 || doc.Nodes[0].Block != "core/text" || doc.Nodes[0].ID == "" {
		t.Fatalf("document nodes = %#v", doc.Nodes)
	}
	content, err := textContent(revision.DocumentJson)
	if err != nil || content != want {
		t.Fatalf("content = %q, %v; want %q", content, err, want)
	}
}
