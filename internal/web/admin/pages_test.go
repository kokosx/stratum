package admin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/document"
	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/themes"
)

func TestWritePagePreservesDocumentsAndPublishedRevision(t *testing.T) {
	ctx := context.Background()
	database, err := storage.Open(filepath.Join(t.TempDir(), "stratum.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	queries := db.New(database.DB)
	registry, err := blocks.NewRegistry(ctx, queries)
	if err != nil {
		t.Fatal(err)
	}
	themeRuntime, err := themes.NewRuntime(ctx, queries)
	if err != nil {
		t.Fatal(err)
	}
	h, err := NewHandler(database.DB, queries, nil, registry, themeRuntime, newTestMedia(t, queries))
	if err != nil {
		t.Fatal(err)
	}

	entryID := "page-under-test"
	draftOne := nestedDocument("draft one", "left")
	if err := h.writeEntry(ctx, "page", "author", entryID, entryInput{title: "First", slug: "first", documentJSON: draftOne}, true, false); err != nil {
		t.Fatal(err)
	}
	entry, err := h.queries.GetEntry(ctx, entryID)
	if err != nil {
		t.Fatal(err)
	}
	if entry.PublishedRevisionID.Valid {
		t.Fatal("new draft unexpectedly has a published revision")
	}
	assertLatestDocument(t, h.queries, entryID, 1, draftOne)

	draftTwo := nestedDocument("draft two", "center")
	if err := h.writeEntry(ctx, "page", "author", entryID, entryInput{title: "Second", slug: "first", documentJSON: draftTwo}, false, false); err != nil {
		t.Fatal(err)
	}
	assertLatestDocument(t, h.queries, entryID, 2, draftTwo)

	publishedDocument := nestedDocument("public version", "right")
	if err := h.writeEntry(ctx, "page", "author", entryID, entryInput{title: "Published", slug: "published", documentJSON: publishedDocument}, false, true); err != nil {
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
	assertLatestDocument(t, h.queries, entryID, 3, publishedDocument)

	newDraft := nestedDocument("not public yet", "left")
	if err := h.writeEntry(ctx, "page", "author", entryID, entryInput{title: "New draft", slug: "published", documentJSON: newDraft}, false, false); err != nil {
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
	assertSameDocument(t, public.DocumentJson, publishedDocument)
	assertLatestDocument(t, h.queries, entryID, 4, newDraft)

	revisions, err := h.queries.ListEntryRevisions(ctx, entryID)
	if err != nil {
		t.Fatal(err)
	}
	for _, revision := range revisions {
		doc, err := document.Decode([]byte(revision.DocumentJson))
		if err != nil {
			t.Fatal(err)
		}
		if doc.Nodes[0].ID != "section-stable" || doc.Nodes[0].Children[0].ID != "text-stable" {
			t.Fatalf("stable IDs changed in revision %d: %#v", revision.RevisionNumber, doc.Nodes)
		}
	}
}

func TestWritePageRejectsInvalidDocumentBeforeCreatingRevision(t *testing.T) {
	ctx := context.Background()
	database, err := storage.Open(filepath.Join(t.TempDir(), "stratum.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	queries := db.New(database.DB)
	registry, err := blocks.NewRegistry(ctx, queries)
	if err != nil {
		t.Fatal(err)
	}
	themeRuntime, err := themes.NewRuntime(ctx, queries)
	if err != nil {
		t.Fatal(err)
	}
	h, err := NewHandler(database.DB, queries, nil, registry, themeRuntime, newTestMedia(t, queries))
	if err != nil {
		t.Fatal(err)
	}
	err = h.writeEntry(ctx, "page", "author", "bad-page", entryInput{title: "Bad", slug: "bad", documentJSON: `{"version":1,"nodes":[{"id":"x","block":"missing/block","version":1,"props":{},"settings":{}}]}`}, true, false)
	if err == nil {
		t.Fatal("invalid document was saved")
	}
	if _, err := queries.GetEntry(ctx, "bad-page"); err == nil {
		t.Fatal("entry was created before document validation")
	}
}

func nestedDocument(text, align string) string {
	return `{"version":1,"nodes":[{"id":"section-stable","block":"core/section","version":1,"props":{},"settings":{"width":"content","verticalSpacing":"md","horizontalPadding":"md","align":"left","background":"default","minHeight":"auto","anchorID":""},"children":[{"id":"text-stable","block":"core/text","version":1,"props":{"text":` + mustJSON(text) + `},"settings":{"align":` + mustJSON(align) + `,"tone":"default","size":"md","maxWidth":"none"}}]}]}`
}

func mustJSON(value string) string {
	encoded, _ := json.Marshal(value)
	return string(encoded)
}

func assertLatestDocument(t *testing.T, queries *db.Queries, entryID string, revisionNumber int64, want string) {
	t.Helper()
	revision, err := queries.GetLatestEntryRevision(context.Background(), entryID)
	if err != nil {
		t.Fatal(err)
	}
	if revision.RevisionNumber != revisionNumber {
		t.Fatalf("revision number = %d, want %d", revision.RevisionNumber, revisionNumber)
	}
	assertSameDocument(t, revision.DocumentJson, want)
}

func assertSameDocument(t *testing.T, got, want string) {
	t.Helper()
	var gotValue, wantValue any
	if err := json.Unmarshal([]byte(got), &gotValue); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(want), &wantValue); err != nil {
		t.Fatal(err)
	}
	gotJSON, _ := json.Marshal(gotValue)
	wantJSON, _ := json.Marshal(wantValue)
	if string(gotJSON) != string(wantJSON) {
		t.Fatalf("document = %s, want %s", gotJSON, wantJSON)
	}
}

func TestReadEntryInputRejectsReservedSlugs(t *testing.T) {
	for _, slug := range []string{"sitemap.xml", "robots.txt", "admin", "stratum"} {
		form := url.Values{"title": {"Page"}, "slug": {slug}, "document_json": {"{}"}}
		req := httptest.NewRequest(http.MethodPost, "/admin/pages", strings.NewReader(form.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		if _, err := readEntryInput(req); err == nil {
			t.Fatalf("reserved slug %q should be rejected", slug)
		}
	}
}

func TestReadEntryInputAllowsNormalSlug(t *testing.T) {
	form := url.Values{"title": {"Page"}, "slug": {"about"}, "document_json": {"{}"}}
	req := httptest.NewRequest(http.MethodPost, "/admin/pages", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if _, err := readEntryInput(req); err != nil {
		t.Fatalf("normal slug should be accepted: %v", err)
	}
}
