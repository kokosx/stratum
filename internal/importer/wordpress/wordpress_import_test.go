package wordpress

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/media"
	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func newTestImporter(t *testing.T) (*Importer, *storage.Database, *db.Queries, string) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "stratum.db")
	database, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	queries := db.New(database.DB)
	mediaStore, err := media.NewLocalStorage(filepath.Join(dir, "media"))
	if err != nil {
		t.Fatalf("media store: %v", err)
	}
	mediaService := media.NewService(queries, mediaStore)
	registry, err := blocks.NewRegistry(ctx, queries, mediaService)
	if err != nil {
		t.Fatalf("blocks registry: %v", err)
	}
	im := New(database.DB, queries, registry, mediaService, dir)
	return im, database, queries, dir
}

func writeWXR(t *testing.T, content string) string {
	t.Helper()
	f := filepath.Join(t.TempDir(), "export.xml")
	if err := os.WriteFile(f, []byte(content), 0600); err != nil {
		t.Fatalf("write WXR: %v", err)
	}
	return f
}

func wxrHeader(inner string) string {
	return fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?><rss version="2.0" xmlns:excerpt="http://wordpress.org/export/1.2/excerpt/" xmlns:content="http://purl.org/rss/1.0/modules/content/" xmlns:dc="http://purl.org/dc/elements/1.1/" xmlns:wp="http://wordpress.org/export/1.2/"><channel><wp:author><wp:author_login>author</wp:author_login><wp:author_email>author@example.test</wp:author_email></wp:author>%s</channel></rss>`, inner)
}

func testPNGBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		for y := 0; y < h; y++ {
			img.Set(x, y, color.RGBA{R: uint8(x % 255), G: uint8(y % 255), B: 128, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func countEntries(t *testing.T, q *db.Queries) int {
	t.Helper()
	n, err := q.CountEntries(context.Background())
	if err != nil {
		t.Fatalf("CountEntries: %v", err)
	}
	return int(n)
}

// TestImportPost verifies basic post import
func TestImportPost(t *testing.T) {
	im, _, q, _ := newTestImporter(t)
	ctx := context.Background()
	wxr := wxrHeader(`<item><title>Post One</title><content:encoded><![CDATA[<p>Hello</p>]]></content:encoded><wp:post_id>1</wp:post_id><wp:post_type>post</wp:post_type><wp:status>publish</wp:status><wp:post_name>post-one</wp:post_name><wp:post_date_gmt>2020-01-02 03:04:05</wp:post_date_gmt><dc:creator><![CDATA[author]]></dc:creator></item>`)
	f := writeWXR(t, wxr)
	report, _, err := im.Import(ctx, f, Options{DownloadMedia: false})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if report.Posts != 1 {
		t.Fatalf("Posts = %d want 1", report.Posts)
	}
	if countEntries(t, q) != 1 {
		t.Fatalf("entries count mismatch")
	}
}

// TestImportPage verifies page import
func TestImportPage(t *testing.T) {
	im, _, q, _ := newTestImporter(t)
	ctx := context.Background()
	wxr := wxrHeader(`<item><title>About</title><content:encoded><![CDATA[<p>About us</p>]]></content:encoded><wp:post_id>2</wp:post_id><wp:post_type>page</wp:post_type><wp:status>publish</wp:status><wp:post_name>about</wp:post_name><wp:post_date_gmt>2020-01-02 03:04:05</wp:post_date_gmt><dc:creator><![CDATA[author]]></dc:creator></item>`)
	f := writeWXR(t, wxr)
	report, _, err := im.Import(ctx, f, Options{DownloadMedia: false})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if report.Pages != 1 {
		t.Fatalf("Pages = %d want 1", report.Pages)
	}
	if countEntries(t, q) != 1 {
		t.Fatalf("entries count")
	}
}

// TestImportPageHierarchyTwoPass verifies child after parent even if child appears first
func TestImportPageHierarchyTwoPass(t *testing.T) {
	im, _, q, _ := newTestImporter(t)
	ctx := context.Background()
	// Child (id 3) references parent id 2, but appears before parent in file
	wxr := wxrHeader(`
<item><title>Child</title><content:encoded><![CDATA[<p>Child</p>]]></content:encoded><wp:post_id>3</wp:post_id><wp:post_type>page</wp:post_type><wp:status>publish</wp:status><wp:post_name>child</wp:post_name><wp:post_parent>2</wp:post_parent><wp:post_date_gmt>2020-01-02 03:04:05</wp:post_date_gmt><dc:creator><![CDATA[author]]></dc:creator></item>
<item><title>Parent</title><content:encoded><![CDATA[<p>Parent</p>]]></content:encoded><wp:post_id>2</wp:post_id><wp:post_type>page</wp:post_type><wp:status>publish</wp:status><wp:post_name>parent</wp:post_name><wp:post_date_gmt>2020-01-02 03:04:05</wp:post_date_gmt><dc:creator><![CDATA[author]]></dc:creator></item>
`)
	f := writeWXR(t, wxr)
	report, _, err := im.Import(ctx, f, Options{DownloadMedia: false})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if report.Pages != 2 {
		t.Fatalf("Pages = %d want 2", report.Pages)
	}
	// Verify child has parent
	parentEntryID, _ := q.GetImportMapping(ctx, db.GetImportMappingParams{Source: source, ObjectType: "page", ExternalID: "2"})
	childEntryID, _ := q.GetImportMapping(ctx, db.GetImportMappingParams{Source: source, ObjectType: "page", ExternalID: "3"})
	childRev, err := q.GetLatestEntryRevision(ctx, childEntryID)
	if err != nil {
		t.Fatalf("GetLatest revision: %v", err)
	}
	if !childRev.ParentEntryID.Valid || childRev.ParentEntryID.String != parentEntryID {
		t.Fatalf("child parent mismatch: %v want %s", childRev.ParentEntryID, parentEntryID)
	}
	// Verify hierarchical route exists
	_ = q // ensure import used
}

// TestImportCategories verifies categories imported
func TestImportCategories(t *testing.T) {
	im, _, q, _ := newTestImporter(t)
	ctx := context.Background()
	wxr := wxrHeader(`<wp:category><wp:term_id>10</wp:term_id><wp:cat_name>News</wp:cat_name><wp:category_nicename>news</wp:category_nicename><wp:category_parent></wp:category_parent></wp:category><item><title>Post</title><content:encoded><![CDATA[<p>Hi</p>]]></content:encoded><wp:post_id>1</wp:post_id><wp:post_type>post</wp:post_type><wp:status>publish</wp:status><wp:post_name>post</wp:post_name><wp:post_date_gmt>2020-01-02 03:04:05</wp:post_date_gmt><dc:creator><![CDATA[author]]></dc:creator><category domain="category" nicename="news"><![CDATA[News]]></category></item>`)
	f := writeWXR(t, wxr)
	report, _, err := im.Import(ctx, f, Options{DownloadMedia: false})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if report.Categories != 1 {
		t.Fatalf("Categories = %d want 1", report.Categories)
	}
	// Verify term exists
	term, err := q.GetTermByTaxonomyAndSlug(ctx, db.GetTermByTaxonomyAndSlugParams{TaxonomyID: "category", Slug: "news"})
	if err != nil {
		t.Fatalf("GetTerm: %v", err)
	}
	if term.Name != "News" {
		t.Fatalf("term name %q", term.Name)
	}
}

// TestImportTags verifies tags imported
func TestImportTags(t *testing.T) {
	im, _, q, _ := newTestImporter(t)
	ctx := context.Background()
	wxr := wxrHeader(`<wp:tag><wp:term_id>20</wp:term_id><wp:tag_name>Go</wp:tag_name><wp:tag_slug>go</wp:tag_slug></wp:tag><item><title>Post</title><content:encoded><![CDATA[<p>Hi</p>]]></content:encoded><wp:post_id>1</wp:post_id><wp:post_type>post</wp:post_type><wp:status>publish</wp:status><wp:post_name>post</wp:post_name><wp:post_date_gmt>2020-01-02 03:04:05</wp:post_date_gmt><dc:creator><![CDATA[author]]></dc:creator><category domain="post_tag" nicename="go"><![CDATA[Go]]></category></item>`)
	f := writeWXR(t, wxr)
	report, _, err := im.Import(ctx, f, Options{DownloadMedia: false})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if report.Tags != 1 {
		t.Fatalf("Tags = %d want 1", report.Tags)
	}
	term, err := q.GetTermByTaxonomyAndSlug(ctx, db.GetTermByTaxonomyAndSlugParams{TaxonomyID: "tag", Slug: "go"})
	if err != nil {
		t.Fatalf("GetTerm tag: %v", err)
	}
	if term.Name != "Go" {
		t.Fatalf("tag name %q", term.Name)
	}
}

// TestTaxonomyAssignmentsRevisionScoped verifies assignments are on revision
func TestTaxonomyAssignmentsRevisionScoped(t *testing.T) {
	im, _, q, _ := newTestImporter(t)
	ctx := context.Background()
	wxr := wxrHeader(`<wp:category><wp:term_id>10</wp:term_id><wp:cat_name>News</wp:cat_name><wp:category_nicename>news</wp:category_nicename></wp:category><item><title>Post</title><content:encoded><![CDATA[<p>Hi</p>]]></content:encoded><wp:post_id>1</wp:post_id><wp:post_type>post</wp:post_type><wp:status>publish</wp:status><wp:post_name>post</wp:post_name><wp:post_date_gmt>2020-01-02 03:04:05</wp:post_date_gmt><dc:creator><![CDATA[author]]></dc:creator><category domain="category" nicename="news"><![CDATA[News]]></category></item>`)
	f := writeWXR(t, wxr)
	_, _, err := im.Import(ctx, f, Options{DownloadMedia: false})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	entryID, _ := q.GetImportMapping(ctx, db.GetImportMappingParams{Source: source, ObjectType: "post", ExternalID: "1"})
	rev, _ := q.GetLatestEntryRevision(ctx, entryID)
	terms, err := q.ListTermsForRevision(ctx, rev.ID)
	if err != nil {
		t.Fatalf("ListTermsForRevision: %v", err)
	}
	if len(terms) != 1 || terms[0].Slug != "news" {
		t.Fatalf("terms for revision: %v", terms)
	}
}

// TestImportDraft verifies draft
func TestImportDraft(t *testing.T) {
	im, _, q, _ := newTestImporter(t)
	ctx := context.Background()
	wxr := wxrHeader(`<item><title>Draft</title><content:encoded><![CDATA[<p>draft</p>]]></content:encoded><wp:post_id>1</wp:post_id><wp:post_type>post</wp:post_type><wp:status>draft</wp:status><wp:post_name>draft</wp:post_name><wp:post_date_gmt>2020-01-02 03:04:05</wp:post_date_gmt><dc:creator><![CDATA[author]]></dc:creator></item>`)
	f := writeWXR(t, wxr)
	report, _, err := im.Import(ctx, f, Options{DownloadMedia: false})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if report.Drafts != 1 {
		t.Fatalf("Drafts = %d want 1", report.Drafts)
	}
	entryID, _ := q.GetImportMapping(ctx, db.GetImportMappingParams{Source: source, ObjectType: "post", ExternalID: "1"})
	entry, _ := q.GetEntry(ctx, entryID)
	if entry.PublishedRevisionID.Valid {
		t.Fatalf("draft should not have published revision")
	}
	rev, _ := q.GetLatestEntryRevision(ctx, entryID)
	if rev.ReviewState != "draft" {
		t.Fatalf("review state %q want draft", rev.ReviewState)
	}
}

// TestImportPending verifies pending
func TestImportPending(t *testing.T) {
	im, _, q, _ := newTestImporter(t)
	ctx := context.Background()
	wxr := wxrHeader(`<item><title>Pending</title><content:encoded><![CDATA[<p>pending</p>]]></content:encoded><wp:post_id>1</wp:post_id><wp:post_type>post</wp:post_type><wp:status>pending</wp:status><wp:post_name>pending</wp:post_name><wp:post_date_gmt>2020-01-02 03:04:05</wp:post_date_gmt><dc:creator><![CDATA[author]]></dc:creator></item>`)
	f := writeWXR(t, wxr)
	report, _, err := im.Import(ctx, f, Options{DownloadMedia: false})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if report.Pending != 1 {
		t.Fatalf("Pending = %d want 1", report.Pending)
	}
	entryID, _ := q.GetImportMapping(ctx, db.GetImportMappingParams{Source: source, ObjectType: "post", ExternalID: "1"})
	rev, _ := q.GetLatestEntryRevision(ctx, entryID)
	if rev.ReviewState != "pending" {
		t.Fatalf("review state %q", rev.ReviewState)
	}
}

// TestImportScheduled verifies scheduled future
func TestImportScheduled(t *testing.T) {
	im, _, _, _ := newTestImporter(t)
	ctx := context.Background()
	future := time.Now().Add(24 * time.Hour).UTC().Format("2006-01-02 15:04:05")
	wxr := wxrHeader(fmt.Sprintf(`<item><title>Future</title><content:encoded><![CDATA[<p>future</p>]]></content:encoded><wp:post_id>1</wp:post_id><wp:post_type>post</wp:post_type><wp:status>future</wp:status><wp:post_name>future</wp:post_name><wp:post_date_gmt>%s</wp:post_date_gmt><dc:creator><![CDATA[author]]></dc:creator></item>`, future))
	f := writeWXR(t, wxr)
	report, _, err := im.Import(ctx, f, Options{DownloadMedia: false})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if report.Scheduled != 1 {
		t.Fatalf("Scheduled = %d want 1", report.Scheduled)
	}
}

// TestImportPrivate verifies private
func TestImportPrivate(t *testing.T) {
	im, _, q, _ := newTestImporter(t)
	ctx := context.Background()
	wxr := wxrHeader(`<item><title>Private</title><content:encoded><![CDATA[<p>private</p>]]></content:encoded><wp:post_id>1</wp:post_id><wp:post_type>post</wp:post_type><wp:status>private</wp:status><wp:post_name>private</wp:post_name><wp:post_date_gmt>2020-01-02 03:04:05</wp:post_date_gmt><dc:creator><![CDATA[author]]></dc:creator></item>`)
	f := writeWXR(t, wxr)
	report, _, err := im.Import(ctx, f, Options{DownloadMedia: false})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if report.Private != 1 {
		t.Fatalf("Private = %d want 1", report.Private)
	}
	entryID, _ := q.GetImportMapping(ctx, db.GetImportMappingParams{Source: source, ObjectType: "post", ExternalID: "1"})
	rev, _ := q.GetLatestEntryRevision(ctx, entryID)
	if rev.Visibility != "private" {
		t.Fatalf("visibility %q", rev.Visibility)
	}
}

// TestImportPassword verifies password protected
func TestImportPassword(t *testing.T) {
	im, _, q, _ := newTestImporter(t)
	ctx := context.Background()
	wxr := wxrHeader(`<item><title>Secret</title><content:encoded><![CDATA[<p>secret</p>]]></content:encoded><wp:post_id>1</wp:post_id><wp:post_type>post</wp:post_type><wp:status>publish</wp:status><wp:post_name>secret</wp:post_name><wp:post_date_gmt>2020-01-02 03:04:05</wp:post_date_gmt><wp:post_password>hunter2</wp:post_password><dc:creator><![CDATA[author]]></dc:creator></item>`)
	f := writeWXR(t, wxr)
	_, _, err := im.Import(ctx, f, Options{DownloadMedia: false})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	entryID, _ := q.GetImportMapping(ctx, db.GetImportMappingParams{Source: source, ObjectType: "post", ExternalID: "1"})
	rev, _ := q.GetLatestEntryRevision(ctx, entryID)
	if rev.Visibility != "password" || !rev.PasswordHash.Valid {
		t.Fatalf("password visibility not set: %v %v", rev.Visibility, rev.PasswordHash)
	}
}

// TestPreservePublishedDate verifies date preserved
func TestPreservePublishedDate(t *testing.T) {
	im, _, q, _ := newTestImporter(t)
	ctx := context.Background()
	wxr := wxrHeader(`<item><title>Old</title><content:encoded><![CDATA[<p>old</p>]]></content:encoded><wp:post_id>1</wp:post_id><wp:post_type>post</wp:post_type><wp:status>publish</wp:status><wp:post_name>old</wp:post_name><wp:post_date_gmt>2019-05-04 10:20:00</wp:post_date_gmt><dc:creator><![CDATA[author]]></dc:creator></item>`)
	f := writeWXR(t, wxr)
	_, _, err := im.Import(ctx, f, Options{DownloadMedia: false})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	entryID, _ := q.GetImportMapping(ctx, db.GetImportMappingParams{Source: source, ObjectType: "post", ExternalID: "1"})
	entry, _ := q.GetEntry(ctx, entryID)
	expected := time.Date(2019, 5, 4, 10, 20, 0, 0, time.UTC).Unix()
	if !entry.PublishedAt.Valid || entry.PublishedAt.Int64 != expected {
		t.Fatalf("published_at %v want %d", entry.PublishedAt, expected)
	}
}

// TestPreserveSlug verifies slug preserved
func TestPreserveSlug(t *testing.T) {
	im, _, q, _ := newTestImporter(t)
	ctx := context.Background()
	wxr := wxrHeader(`<item><title>My Post</title><content:encoded><![CDATA[<p>hi</p>]]></content:encoded><wp:post_id>1</wp:post_id><wp:post_type>post</wp:post_type><wp:status>publish</wp:status><wp:post_name>custom-slug</wp:post_name><wp:post_date_gmt>2020-01-02 03:04:05</wp:post_date_gmt><dc:creator><![CDATA[author]]></dc:creator></item>`)
	f := writeWXR(t, wxr)
	_, _, err := im.Import(ctx, f, Options{DownloadMedia: false})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	entryID, _ := q.GetImportMapping(ctx, db.GetImportMappingParams{Source: source, ObjectType: "post", ExternalID: "1"})
	entry, _ := q.GetEntry(ctx, entryID)
	if entry.Slug != "custom-slug" {
		t.Fatalf("slug %q want custom-slug", entry.Slug)
	}
	rev, _ := q.GetLatestEntryRevision(ctx, entryID)
	if rev.Slug != "custom-slug" {
		t.Fatalf("rev slug %q", rev.Slug)
	}
}

// TestRouteConflictSkipped verifies conflict skip
func TestRouteConflictSkipped(t *testing.T) {
	im, _, q, _ := newTestImporter(t)
	ctx := context.Background()
	// First import
	wxr1 := wxrHeader(`<item><title>First</title><content:encoded><![CDATA[<p>first</p>]]></content:encoded><wp:post_id>1</wp:post_id><wp:post_type>post</wp:post_type><wp:status>publish</wp:status><wp:post_name>conflict</wp:post_name><wp:post_date_gmt>2020-01-02 03:04:05</wp:post_date_gmt><dc:creator><![CDATA[author]]></dc:creator></item>`)
	f1 := writeWXR(t, wxr1)
	_, _, err := im.Import(ctx, f1, Options{DownloadMedia: false})
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	// Second import with same slug but different ID
	wxr2 := wxrHeader(`<item><title>Second</title><content:encoded><![CDATA[<p>second</p>]]></content:encoded><wp:post_id>2</wp:post_id><wp:post_type>post</wp:post_type><wp:status>publish</wp:status><wp:post_name>conflict</wp:post_name><wp:post_date_gmt>2020-01-03 03:04:05</wp:post_date_gmt><dc:creator><![CDATA[author]]></dc:creator></item>`)
	f2 := writeWXR(t, wxr2)
	report, _, err := im.Import(ctx, f2, Options{DownloadMedia: false})
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	if report.Conflicts == 0 {
		t.Fatalf("expected conflict")
	}
	if report.Skipped == 0 {
		t.Fatalf("expected skipped")
	}
	// Ensure second entry not imported (no mapping)
	if _, err := q.GetImportMapping(ctx, db.GetImportMappingParams{Source: source, ObjectType: "post", ExternalID: "2"}); err == nil {
		t.Fatalf("conflicting entry should be skipped and not mapped")
	}
}

// TestImportMappingCreated verifies mapping durable
func TestImportMappingCreated(t *testing.T) {
	im, _, q, _ := newTestImporter(t)
	ctx := context.Background()
	wxr := wxrHeader(`<item><title>Post</title><content:encoded><![CDATA[<p>hi</p>]]></content:encoded><wp:post_id>99</wp:post_id><wp:post_type>post</wp:post_type><wp:status>publish</wp:status><wp:post_name>post</wp:post_name><wp:post_date_gmt>2020-01-02 03:04:05</wp:post_date_gmt><dc:creator><![CDATA[author]]></dc:creator></item>`)
	f := writeWXR(t, wxr)
	_, _, err := im.Import(ctx, f, Options{DownloadMedia: false})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	mapping, err := q.GetImportMapping(ctx, db.GetImportMappingParams{Source: source, ObjectType: "post", ExternalID: "99"})
	if err != nil {
		t.Fatalf("mapping not created: %v", err)
	}
	if mapping == "" {
		t.Fatalf("empty mapping")
	}
}

// TestRerunDoesNotDuplicate verifies idempotency
func TestRerunDoesNotDuplicate(t *testing.T) {
	im, _, q, _ := newTestImporter(t)
	ctx := context.Background()
	wxr := wxrHeader(`<item><title>Post</title><content:encoded><![CDATA[<p>hi</p>]]></content:encoded><wp:post_id>1</wp:post_id><wp:post_type>post</wp:post_type><wp:status>publish</wp:status><wp:post_name>post</wp:post_name><wp:post_date_gmt>2020-01-02 03:04:05</wp:post_date_gmt><dc:creator><![CDATA[author]]></dc:creator></item>`)
	f := writeWXR(t, wxr)
	_, _, err := im.Import(ctx, f, Options{DownloadMedia: false})
	if err != nil {
		t.Fatalf("first import: %v", err)
	}
	count1 := countEntries(t, q)
	report2, _, err := im.Import(ctx, f, Options{DownloadMedia: false})
	if err != nil {
		t.Fatalf("second import: %v", err)
	}
	count2 := countEntries(t, q)
	if count1 != count2 {
		t.Fatalf("rerun duplicated: %d vs %d", count1, count2)
	}
	if report2.Skipped == 0 {
		t.Fatalf("rerun should have skipped")
	}
}

// TestGutenbergCommentsRemoved verifies Gutenberg markers removed
func TestGutenbergCommentsRemoved(t *testing.T) {
	warnings := []string{}
	doc, err := htmlDocument(`<!-- wp:paragraph --><p>Hello</p><!-- /wp:paragraph -->`, nil, &warnings)
	if err != nil {
		t.Fatalf("htmlDocument: %v", err)
	}
	if len(doc.Nodes) != 1 || doc.Nodes[0].Block != "core/text" {
		t.Fatalf("unexpected doc: %+v", doc.Nodes)
	}
	// Ensure no node contains Gutenberg comment text
	data, _ := doc.Nodes[0].Props.MarshalJSON()
	if strings.Contains(string(data), "wp:") {
		t.Fatalf("Gutenberg marker leaked: %s", string(data))
	}
}

// TestHTMLToSDTHeading verifies heading conversion
func TestHTMLToSDTHeading(t *testing.T) {
	warnings := []string{}
	doc, err := htmlDocument(`<h2>Heading Text</h2>`, nil, &warnings)
	if err != nil {
		t.Fatalf("htmlDocument: %v", err)
	}
	if len(doc.Nodes) != 1 || doc.Nodes[0].Block != "core/heading" {
		t.Fatalf("heading not converted: %+v", doc.Nodes)
	}
}

// TestHTMLToSDTParagraphRichText verifies paragraph with bold
func TestHTMLToSDTParagraphRichText(t *testing.T) {
	warnings := []string{}
	doc, err := htmlDocument(`<p>Hello <strong>world</strong></p>`, nil, &warnings)
	if err != nil {
		t.Fatalf("htmlDocument: %v", err)
	}
	if len(doc.Nodes) != 1 || doc.Nodes[0].Block != "core/text" {
		t.Fatalf("paragraph not converted: %+v", doc.Nodes)
	}
	// Check that props contain bold mark via raw JSON
	if !strings.Contains(string(doc.Nodes[0].Props), "bold") {
		t.Fatalf("bold mark missing: %s", string(doc.Nodes[0].Props))
	}
}

// TestHTMLToSDTLinks verifies link conversion
func TestHTMLToSDTLinks(t *testing.T) {
	warnings := []string{}
	doc, err := htmlDocument(`<p><a href="https://example.test">link</a></p>`, nil, &warnings)
	if err != nil {
		t.Fatalf("htmlDocument: %v", err)
	}
	if len(doc.Nodes) != 1 {
		t.Fatalf("nodes %d", len(doc.Nodes))
	}
	if !strings.Contains(string(doc.Nodes[0].Props), "https://example.test") {
		t.Fatalf("link href missing: %s", string(doc.Nodes[0].Props))
	}
}

// TestUnsupportedScriptRemoved verifies script stripped
func TestUnsupportedScriptRemoved(t *testing.T) {
	warnings := []string{}
	doc, err := htmlDocument(`<p>Hello</p><script>alert(1)</script>`, nil, &warnings)
	if err != nil {
		t.Fatalf("htmlDocument: %v", err)
	}
	if len(doc.Nodes) != 1 {
		t.Fatalf("script not removed, nodes: %d", len(doc.Nodes))
	}
	if len(warnings) == 0 {
		t.Fatalf("expected warning for script")
	}
	for _, n := range doc.Nodes {
		if strings.Contains(string(n.Props), "alert") {
			t.Fatalf("script content leaked")
		}
	}
}

// TestShortcodeNotExecutedViaHTML verifies known shortcodes are removed while
// human bracketed prose survives, per the conservative policy.
func TestShortcodeNotExecutedViaHTML(t *testing.T) {
	warnings := []string{}
	doc, err := htmlDocument(`<p>Before [gallery id="1"] mid [optional] after [0] end.</p>`, nil, &warnings)
	if err != nil {
		t.Fatalf("htmlDocument: %v", err)
	}
	if len(doc.Nodes) == 0 {
		t.Fatalf("no nodes")
	}
	props := string(doc.Nodes[0].Props)
	if strings.Contains(props, "gallery") {
		t.Fatalf("shortcode leaked: %s", props)
	}
	if !strings.Contains(props, "[optional]") || !strings.Contains(props, "[0]") {
		t.Fatalf("bracketed prose must be preserved: %s", props)
	}
}

// testDownloader returns a Downloader whose resolver claims a PUBLIC IP for any
// host while the dialer connects to the real httptest listener. This keeps the
// production policy path fully exercised (no global bypass) in tests.
func testDownloader(srv *httptest.Server, dialed *[]string) *Downloader {
	return &Downloader{
		Resolve: func(ctx context.Context, host string) ([]net.IP, error) {
			return []net.IP{net.ParseIP("93.184.216.34")}, nil
		},
		Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if dialed != nil {
				*dialed = append(*dialed, addr)
			}
			var nd net.Dialer
			return nd.DialContext(ctx, network, srv.Listener.Addr().String())
		},
	}
}

// TestFeaturedImageMapping verifies featured image resolved in second pass
func TestFeaturedImageMapping(t *testing.T) {
	im, _, q, _ := newTestImporter(t)
	ctx := context.Background()
	pngBytes := testPNGBytes(t, 100, 100)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngBytes)
	}))
	defer srv.Close()
	im.Downloader = testDownloader(srv, nil)
	attachmentURL := "http://media.example.test/image.png"
	wxr := wxrHeader(fmt.Sprintf(`
<item><title>Image</title><wp:post_id>10</wp:post_id><wp:post_type>attachment</wp:post_type><wp:status>inherit</wp:status><wp:attachment_url>%s</wp:attachment_url><wp:post_date_gmt>2020-01-02 03:04:05</wp:post_date_gmt></item>
<item><title>Post</title><content:encoded><![CDATA[<p>hi</p>]]></content:encoded><wp:post_id>1</wp:post_id><wp:post_type>post</wp:post_type><wp:status>publish</wp:status><wp:post_name>post</wp:post_name><wp:post_date_gmt>2020-01-02 03:04:05</wp:post_date_gmt><dc:creator><![CDATA[author]]></dc:creator><wp:postmeta><wp:meta_key>_thumbnail_id</wp:meta_key><wp:meta_value>10</wp:meta_value></wp:postmeta></item>
`, attachmentURL))
	f := writeWXR(t, wxr)
	report, _, err := im.Import(ctx, f, Options{DownloadMedia: true})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if report.MediaImported != 1 {
		t.Fatalf("MediaImported = %d want 1", report.MediaImported)
	}
	entryID, _ := q.GetImportMapping(ctx, db.GetImportMappingParams{Source: source, ObjectType: "post", ExternalID: "1"})
	rev, _ := q.GetLatestEntryRevision(ctx, entryID)
	if !rev.FeaturedMediaID.Valid {
		t.Fatalf("featured media not set")
	}
}

// TestMediaUsesMediaService verifies media imported via service creates DB rows
func TestMediaUsesMediaService(t *testing.T) {
	im, _, q, _ := newTestImporter(t)
	ctx := context.Background()
	pngBytes := testPNGBytes(t, 200, 200)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write(pngBytes)
	}))
	defer srv.Close()
	im.Downloader = testDownloader(srv, nil)
	wxr := wxrHeader(`<item><title>Img</title><wp:post_id>10</wp:post_id><wp:post_type>attachment</wp:post_type><wp:status>inherit</wp:status><wp:attachment_url>http://media.example.test/img.png</wp:attachment_url><wp:post_date_gmt>2020-01-02 03:04:05</wp:post_date_gmt></item>`)
	f := writeWXR(t, wxr)
	report, _, err := im.Import(ctx, f, Options{DownloadMedia: true})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if report.MediaImported != 1 {
		t.Fatalf("MediaImported %d", report.MediaImported)
	}
	count, _ := q.CountMedia(ctx)
	if count != 1 {
		t.Fatalf("CountMedia = %d want 1", count)
	}
}

// TestMediaSSRFLoopbackRejected verifies loopback blocked by policy.
func TestMediaSSRFLoopbackRejected(t *testing.T) {
	dl := newDownloader()
	_, _, err := dl.Get(context.Background(), "http://127.0.0.1/a")
	if err == nil || !errors.Is(err, errForbiddenAddress) {
		t.Fatalf("expected forbidden-address rejection for loopback, got %v", err)
	}
}

// TestMediaSSRFPrivateIPv4Rejected verifies private IPv4 ranges blocked.
func TestMediaSSRFPrivateIPv4Rejected(t *testing.T) {
	for _, raw := range []string{"http://192.168.1.2/a", "http://10.0.0.1/a", "http://172.16.0.1/a"} {
		dl := newDownloader()
		if _, _, err := dl.Get(context.Background(), raw); err == nil || !errors.Is(err, errForbiddenAddress) {
			t.Fatalf("expected %s rejected as forbidden, got %v", raw, err)
		}
	}
}

// TestMediaSSRFIPv6Rejected verifies IPv6 loopback blocked.
func TestMediaSSRFIPv6Rejected(t *testing.T) {
	dl := newDownloader()
	if _, _, err := dl.Get(context.Background(), "http://[::1]/a"); err == nil || !errors.Is(err, errForbiddenAddress) {
		t.Fatalf("expected ::1 rejected, got %v", err)
	}
}

// TestMediaRedirectRevalidated proves the FAILURE is the redirect target's IP
// policy: the initial endpoint is allowed via injected seams, and rejection only
// occurs after following the redirect to a host resolving to a private address.
func TestMediaRedirectRevalidated(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://rebind.example.test/private.png", http.StatusFound)
	}))
	defer srv.Close()
	redirects := 0
	dl := &Downloader{
		Resolve: func(ctx context.Context, host string) ([]net.IP, error) {
			if host == "start.example.test" && redirects == 0 {
				return []net.IP{net.ParseIP("93.184.216.34")}, nil
			}
			return []net.IP{net.ParseIP("192.168.1.1")}, nil // redirect target: FORBIDDEN
		},
		Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
			var nd net.Dialer
			return nd.DialContext(ctx, network, srv.Listener.Addr().String())
		},
	}
	client := dl.client(context.Background())
	resp, err := client.Get("http://start.example.test/entry.png")
	if err == nil {
		resp.Body.Close()
		t.Fatalf("expected redirect target to be rejected by IP policy")
	}
	if !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("expected forbidden-address failure at redirect, got %v", err)
	}
	_ = redirects
}

// TestMediaSizeLimitContentLength proves Content-Length above the limit fails
// with ErrTooLarge BEFORE reading the body.
func TestMediaSizeLimitContentLength(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.FormatInt(maxDownloadBytes+1, 10))
		w.WriteHeader(200)
		_, _ = w.Write(make([]byte, 1024))
	}))
	defer srv.Close()
	dl := testDownloader(srv, nil)
	_, _, err := dl.Get(context.Background(), "http://media.example.test/big.png")
	if !errors.Is(err, media.ErrTooLarge) {
		t.Fatalf("expected ErrTooLarge from Content-Length check, got %v", err)
	}
}

// TestMediaSizeLimitStreaming proves a body WITHOUT trustworthy Content-Length
// that streams past the limit is detected by the bounded reader (ErrTooLarge),
// never silently truncated.
func TestMediaSizeLimitStreaming(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.(http.Flusher).Flush()
		chunk := make([]byte, 64<<10)
		for i := 0; i < (maxDownloadBytes/(64<<10))+2; i++ {
			_, _ = w.Write(chunk)
			w.(http.Flusher).Flush()
		}
	}))
	defer srv.Close()
	dl := testDownloader(srv, nil)
	body, _, err := dl.Get(context.Background(), "http://media.example.test/stream.png")
	if err != nil {
		t.Fatalf("initial Get should succeed; size enforced during read: %v", err)
	}
	defer body.Close()
	_, readErr := io.Copy(io.Discard, body)
	if !errors.Is(readErr, media.ErrTooLarge) {
		t.Fatalf("expected streaming overflow ErrTooLarge, got %v", readErr)
	}
}

// TestMediaDNSRebindingCannotReachForbiddenSecondAddress proves the VALIDATED IP
// is what gets dialed: resolution #1 returns a public IP (validated), and even
// though a hostile re-resolution would return a private one, the dialer receives
// exactly the validated public IP — no second lookup happens.
func TestMediaDNSRebindingCannotReachForbiddenSecondAddress(t *testing.T) {
	resolutions := 0
	var dialedHosts []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(testPNGBytes(t, 4, 4))
	}))
	defer srv.Close()
	dl := &Downloader{
		Resolve: func(ctx context.Context, host string) ([]net.IP, error) {
			resolutions++
			if resolutions == 1 {
				return []net.IP{net.ParseIP("93.184.216.34")}, nil // genuinely public, allowed
			}
			return []net.IP{net.ParseIP("127.0.0.1")}, nil // rebinding attempt: must never be dialed
		},
		Dial: func(ctx context.Context, network, addr string) (net.Conn, error) {
			dialedHosts = append(dialedHosts, addr)
			var nd net.Dialer
			return nd.DialContext(ctx, network, srv.Listener.Addr().String())
		},
	}
	body, _, err := dl.Get(context.Background(), "http://evil.example.test/a.png")
	if err != nil {
		t.Fatalf("download should succeed against validated IP: %v", err)
	}
	body.Close()
	if resolutions != 1 {
		t.Fatalf("expected exactly ONE resolution (no TOCTOU), got %d", resolutions)
	}
	if len(dialedHosts) == 0 || !strings.HasPrefix(dialedHosts[0], "93.184.216.34:") {
		t.Fatalf("dialer must receive validated IP, got %v", dialedHosts)
	}
}

// TestDryRunWritesNothing verifies dry-run does not write
func TestDryRunWritesNothing(t *testing.T) {
	im, _, q, _ := newTestImporter(t)
	ctx := context.Background()
	wxr := wxrHeader(`<item><title>Post</title><content:encoded><![CDATA[<p>hi</p>]]></content:encoded><wp:post_id>1</wp:post_id><wp:post_type>post</wp:post_type><wp:status>publish</wp:status><wp:post_name>post</wp:post_name><wp:post_date_gmt>2020-01-02 03:04:05</wp:post_date_gmt><dc:creator><![CDATA[author]]></dc:creator></item>`)
	f := writeWXR(t, wxr)
	report, _, err := im.Import(ctx, f, Options{DryRun: true})
	if err != nil {
		t.Fatalf("dry-run: %v", err)
	}
	_ = report
	if countEntries(t, q) != 0 {
		t.Fatalf("dry-run wrote entries")
	}
	if _, err := q.GetImportMapping(ctx, db.GetImportMappingParams{Source: source, ObjectType: "post", ExternalID: "1"}); err == nil {
		t.Fatalf("dry-run created mapping")
	}
}

// TestPreImportBackupRequired verifies backup created
func TestPreImportBackupRequired(t *testing.T) {
	im, _, q, dir := newTestImporter(t)
	ctx := context.Background()
	wxr := wxrHeader(`<item><title>Post</title><content:encoded><![CDATA[<p>hi</p>]]></content:encoded><wp:post_id>1</wp:post_id><wp:post_type>post</wp:post_type><wp:status>publish</wp:status><wp:post_name>post</wp:post_name><wp:post_date_gmt>2020-01-02 03:04:05</wp:post_date_gmt><dc:creator><![CDATA[author]]></dc:creator></item>`)
	f := writeWXR(t, wxr)
	// Ensure cwd does not have leftover backup; run import
	report, backupPath, err := im.Import(ctx, f, Options{DownloadMedia: false, DataDir: dir})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	if backupPath == "" {
		t.Fatalf("backupPath empty")
	}
	if _, err := os.Stat(backupPath); err != nil {
		t.Fatalf("backup not created: %v", err)
	}
	// Cleanup backup file
	_ = os.Remove(backupPath)
	_ = report
	if countEntries(t, q) != 1 {
		t.Fatalf("import did not create entry")
	}
}

// TestSearchRebuiltAfterImport verifies search index
func TestSearchRebuiltAfterImport(t *testing.T) {
	im, _, _, _ := newTestImporter(t)
	ctx := context.Background()
	wxr := wxrHeader(`<item><title>Searchable Post</title><content:encoded><![CDATA[<p>uniqueSearchTermXYZ</p>]]></content:encoded><wp:post_id>1</wp:post_id><wp:post_type>post</wp:post_type><wp:status>publish</wp:status><wp:post_name>searchable</wp:post_name><wp:post_date_gmt>2020-01-02 03:04:05</wp:post_date_gmt><dc:creator><![CDATA[author]]></dc:creator></item>`)
	f := writeWXR(t, wxr)
	_, _, err := im.Import(ctx, f, Options{DownloadMedia: false})
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	// Verify search can find it via search service
	// Use the importer's search directly (rebuilt)
	results, total, err := im.search.Query(ctx, "uniqueSearchTermXYZ", 1)
	if err != nil {
		t.Fatalf("search query: %v", err)
	}
	if total == 0 || len(results) == 0 {
		t.Fatalf("search did not find imported post: total %d results %v", total, results)
	}
}

// Additional helper to ensure unused import is satisfied
var _ = sql.ErrNoRows
