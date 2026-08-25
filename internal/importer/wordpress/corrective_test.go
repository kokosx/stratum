package wordpress

import (
	"context"
	"database/sql"
	stderrors "errors"
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/datalock"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

// --- §43 DATA LOCK TESTS -----------------------------------------------------

func TestImportDataLockBlocksMutation(t *testing.T) {
	im, _, q, dir := newTestImporter(t)
	ctx := context.Background()
	wxr := wxrHeader(`<item><title>Locked</title><content:encoded><![CDATA[<p>hi</p>]]></content:encoded><wp:post_id>1</wp:post_id><wp:post_type>post</wp:post_type><wp:status>publish</wp:status><wp:post_name>locked</wp:post_name><wp:post_date_gmt>2020-01-02 03:04:05</wp:post_date_gmt><dc:creator><![CDATA[author]]></dc:creator></item>`)
	f := writeWXR(t, wxr)

	// serve-equivalent lock held
	lock, err := datalock.Acquire(dir)
	if err != nil {
		t.Fatalf("acquire serve lock: %v", err)
	}
	defer lock.Close()

	before := countEntries(t, q)
	if _, _, err := im.Import(ctx, f, Options{DownloadMedia: false, DataDir: dir}); err == nil {
		t.Fatal("non-dry import must fail while dataDir is locked")
	} else if !strings.Contains(err.Error(), "cannot import") {
		t.Fatalf("unexpected error: %v", err)
	}
	if after := countEntries(t, q); after != before {
		t.Fatalf("entries changed under lock: %d -> %d", before, after)
	}
	if n, _ := q.CountImportRunsForSource(ctx, source); n != 0 {
		t.Fatalf("import_run created despite lock failure: %d", n)
	}
}

func TestDryRunWithHeldLockSucceeds(t *testing.T) {
	im, _, _, dir := newTestImporter(t)
	ctx := context.Background()
	wxr := wxrHeader(`<item><title>Dry</title><content:encoded><![CDATA[<p>hi</p>]]></content:encoded><wp:post_id>1</wp:post_id><wp:post_type>post</wp:post_type><wp:status>publish</wp:status><wp:post_name>dry</wp:post_name><wp:post_date_gmt>2020-01-02 03:04:05</wp:post_date_gmt><dc:creator><![CDATA[author]]></dc:creator></item>`)
	f := writeWXR(t, wxr)
	lock, err := datalock.Acquire(dir)
	if err != nil {
		t.Fatalf("acquire serve lock: %v", err)
	}
	defer lock.Close()
	report, _, err := im.Import(ctx, f, Options{DryRun: true, DataDir: dir})
	if err != nil {
		t.Fatalf("dry-run must succeed read-only while locked: %v", err)
	}
	if report.Posts != 1 {
		t.Fatalf("dry-run Posts = %d want 1", report.Posts)
	}
}

// --- §39 PARTIAL RESUME TESTS -----------------------------------------------

func TestImportResumeMappedEntryWithoutRevision(t *testing.T) {
	im, _, q, _ := newTestImporter(t)
	ctx := context.Background()
	// Simulate a previous partial run: mapping + entry shell exist, NO revision.
	entryID := "shell-entry-1"
	if err := q.CreateEntry(ctx, db.CreateEntryParams{ID: entryID, ContentTypeID: "post", Slug: "resumable", Status: "active", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	// Run row must exist first (import_mappings.run_id FK).
	if err := q.CreateImportRun(ctx, db.CreateImportRunParams{ID: "old-run", Source: source, CreatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := q.CreateImportMapping(ctx, db.CreateImportMappingParams{Source: source, ObjectType: "post", ExternalID: "1", InternalID: entryID, RunID: "old-run", CreatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	before := countEntries(t, q)
	wxr := wxrHeader(`<item><title>Resumable</title><content:encoded><![CDATA[<p>hi</p>]]></content:encoded><wp:post_id>1</wp:post_id><wp:post_type>post</wp:post_type><wp:status>publish</wp:status><wp:post_name>resumable</wp:post_name><wp:post_date_gmt>2020-01-02 03:04:05</wp:post_date_gmt><dc:creator><![CDATA[author]]></dc:creator></item>`)
	f := writeWXR(t, wxr)
	report, _, rawErr := im.Import(ctx, f, Options{DownloadMedia: false})
	err := rawErr
	if err != nil {
		for e := err; e != nil; e = stderrors.Unwrap(e) {
			t.Logf("unwrap: %v", e)
		}
		t.Fatalf("resume import: %v", err)
	}
	_ = report
	if countEntries(t, q) != before {
		t.Fatalf("resume must reuse shell entry, not duplicate")
	}
	rev, err := q.GetLatestEntryRevision(ctx, entryID)
	if err != nil {
		t.Fatalf("revision must be created during resume: %v", err)
	}
	if rev.Title != "Resumable" {
		t.Fatalf("revision title %q", rev.Title)
	}
	if report.Posts != 1 {
		t.Fatalf("Posts = %d want 1", report.Posts)
	}
}

func TestCompletedImportRerunDoesNotOverwrite(t *testing.T) {
	im, _, q, _ := newTestImporter(t)
	ctx := context.Background()
	wxr := wxrHeader(`<item><title>Original</title><content:encoded><![CDATA[<p>v1</p>]]></content:encoded><wp:post_id>1</wp:post_id><wp:post_type>post</wp:post_type><wp:status>publish</wp:status><wp:post_name>done</wp:post_name><wp:post_date_gmt>2020-01-02 03:04:05</wp:post_date_gmt><dc:creator><![CDATA[author]]></dc:creator></item>`)
	f := writeWXR(t, wxr)
	if _, _, err := im.Import(ctx, f, Options{DownloadMedia: false}); err != nil {
		t.Fatal(err)
	}
	entryID, _ := q.GetImportMapping(ctx, db.GetImportMappingParams{Source: source, ObjectType: "post", ExternalID: "1"})
	revCountBefore := len(mustListRevisions(t, q, entryID))
	// Simulate user editing the completed import.
	if err := q.SetImportedPublishedDates(ctx, db.SetImportedPublishedDatesParams{CreatedAt: 12345, UpdatedAt: 12345, PublishedAt: sqlNull(12345), FirstPublishedAt: sqlNull(12345), ID: entryID}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := im.Import(ctx, f, Options{DownloadMedia: false}); err != nil {
		t.Fatal(err)
	}
	if got := len(mustListRevisions(t, q, entryID)); got != revCountBefore {
		t.Fatalf("rerun added revisions over completed import: %d -> %d", revCountBefore, got)
	}
	e, _ := q.GetEntry(ctx, entryID)
	if e.PublishedAt.Int64 != 12345 {
		t.Fatalf("user-edited state was overwritten: %v", e.PublishedAt)
	}
}

// --- §37/38 ROUTE PLAN TESTS -------------------------------------------------

func TestSameChildSlugUnderDifferentParentsBothImport(t *testing.T) {
	im, _, _, _ := newTestImporter(t)
	ctx := context.Background()
	wxr := wxrHeader(`
<item><title>Company</title><content:encoded><![CDATA[<p>c</p>]]></content:encoded><wp:post_id>2</wp:post_id><wp:post_type>page</wp:post_type><wp:status>publish</wp:status><wp:post_name>company</wp:post_name><wp:post_date_gmt>2020-01-02 03:04:05</wp:post_date_gmt><dc:creator><![CDATA[author]]></dc:creator></item>
<item><title>About</title><content:encoded><![CDATA[<p>a</p>]]></content:encoded><wp:post_id>3</wp:post_id><wp:post_type>page</wp:post_type><wp:status>publish</wp:status><wp:post_name>about</wp:post_name><wp:post_date_gmt>2020-01-02 03:04:05</wp:post_date_gmt><dc:creator><![CDATA[author]]></dc:creator></item>
<item><title>Team Co</title><content:encoded><![CDATA[<p>t</p>]]></content:encoded><wp:post_id>4</wp:post_id><wp:post_type>page</wp:post_type><wp:status>publish</wp:status><wp:post_name>team</wp:post_name><wp:post_parent>2</wp:post_parent><wp:post_date_gmt>2020-01-02 03:04:05</wp:post_date_gmt><dc:creator><![CDATA[author]]></dc:creator></item>
<item><title>Team About</title><content:encoded><![CDATA[<p>t</p>]]></content:encoded><wp:post_id>5</wp:post_id><wp:post_type>page</wp:post_type><wp:status>publish</wp:status><wp:post_name>team</wp:post_name><wp:post_parent>3</wp:post_parent><wp:post_date_gmt>2020-01-02 03:04:05</wp:post_date_gmt><dc:creator><![CDATA[author]]></dc:creator></item>
`)
	f := writeWXR(t, wxr)
	report, _, err := im.Import(ctx, f, Options{DownloadMedia: false})
	if err != nil {
		t.Fatal(err)
	}
	if report.Pages != 4 || report.Conflicts != 0 {
		t.Fatalf("Pages=%d Conflicts=%d; same leaf slug under different parents must both import", report.Pages, report.Conflicts)
	}
	routeCo, err1 := im.q.GetRouteByPath(ctx, "/company/team")
	routeAb, err2 := im.q.GetRouteByPath(ctx, "/about/team")
	if err1 != nil || err2 != nil {
		t.Fatalf("hierarchical routes missing: %v %v", err1, err2)
	}
	if routeCo.EntryID.String == routeAb.EntryID.String {
		t.Fatal("routes point to the same entry")
	}
}

func TestCustomPostsBasePathUsedEverywhere(t *testing.T) {
	im, _, q, _ := newTestImporter(t)
	ctx := context.Background()
	row, _ := q.GetSiteSettings(ctx)
	if err := q.UpdateSiteSettings(ctx, db.UpdateSiteSettingsParams{
		SiteTitle: row.SiteTitle, SiteTagline: row.SiteTagline, HomepageMode: row.HomepageMode,
		HomepageEntryID: row.HomepageEntryID, PostsPageEntryID: row.PostsPageEntryID,
		PostsPerPage: row.PostsPerPage, PostsBasePath: "/news", Language: row.Language, Timezone: row.Timezone,
		ActiveTheme: row.ActiveTheme, IndexingEnabled: row.IndexingEnabled, SiteUrl: row.SiteUrl,
		SitemapEnabled: row.SitemapEnabled, RobotsMode: row.RobotsMode, RobotsCustom: row.RobotsCustom,
		SpeculationMode: row.SpeculationMode, SpeculationEagerness: row.SpeculationEagerness,
		TitleSeparator: row.TitleSeparator, SiteSocialMediaID: row.SiteSocialMediaID,
		TwitterSite: row.TwitterSite, SiteRepresents: row.SiteRepresents, UpdatedAt: row.UpdatedAt,
	}); err != nil {
		t.Fatal(err)
	}
	wxr := wxrHeader(`<item><title>Example Post</title><content:encoded><![CDATA[<p>x</p>]]></content:encoded><wp:post_id>1</wp:post_id><wp:post_type>post</wp:post_type><wp:status>publish</wp:status><wp:post_name>example-post</wp:post_name><wp:post_date_gmt>2020-01-02 03:04:05</wp:post_date_gmt><dc:creator><![CDATA[author]]></dc:creator></item>`)
	f := writeWXR(t, wxr)
	// Dry-run and real import must agree on the effective path.
	dry := Report{}
	if err := im.dryRun(ctx, f, &dry); err != nil {
		t.Fatal(err)
	}
	if dry.Conflicts != 0 || dry.Posts != 1 {
		t.Fatalf("dry-run under /news: %+v", dry)
	}
	report, _, err := im.Import(ctx, f, Options{DownloadMedia: false})
	if err != nil || report.Posts != 1 {
		t.Fatalf("import: %v %+v", err, report)
	}
	if _, err := im.q.GetRouteByPath(ctx, "/news/example-post"); err != nil {
		t.Fatalf("expected /news/example-post (no hardcoded /blog): %v", err)
	}
}

func TestPageHierarchyCycleDeterministicSkip(t *testing.T) {
	im, _, q, _ := newTestImporter(t)
	ctx := context.Background()
	wxr := wxrHeader(`
<item><title>A</title><content:encoded><![CDATA[<p>a</p>]]></content:encoded><wp:post_id>1</wp:post_id><wp:post_type>page</wp:post_type><wp:status>publish</wp:status><wp:post_name>a</wp:post_name><wp:post_parent>2</wp:post_parent><wp:post_date_gmt>2020-01-02 03:04:05</wp:post_date_gmt></item>
<item><title>B</title><content:encoded><![CDATA[<p>b</p>]]></content:encoded><wp:post_id>2</wp:post_id><wp:post_type>page</wp:post_type><wp:status>publish</wp:status><wp:post_name>b</wp:post_name><wp:post_parent>1</wp:post_parent><wp:post_date_gmt>2020-01-02 03:04:05</wp:post_date_gmt></item>
`)
	f := writeWXR(t, wxr)
	report, _, err := im.Import(ctx, f, Options{DownloadMedia: false})
	if err != nil {
		t.Fatalf("cycle must be handled deterministically, not crash: %v", err)
	}
	if countEntries(t, q) != 0 && report.Pages != 0 {
		t.Fatalf("cycled pages must not publish: pages=%d entries=%d", report.Pages, countEntries(t, q))
	}
	if report.Conflicts == 0 && report.Warnings == 0 {
		t.Fatal("cycle must surface as conflict or warning")
	}
}

// --- §40 TAXONOMY TESTS ------------------------------------------------------

func TestImportCategoryChildBeforeParentTwoPassWPID(t *testing.T) {
	im, _, q, _ := newTestImporter(t)
	ctx := context.Background()
	// Child (id 21, slug tech) appears BEFORE its parent (id 20, news).
	wxr := wxrHeader(`
<wp:category><wp:term_id>21</wp:term_id><wp:cat_name>Tech</wp:cat_name><wp:category_nicename>tech</wp:category_nicename><wp:category_parent>news</wp:category_parent></wp:category>
<wp:category><wp:term_id>20</wp:term_id><wp:cat_name>News</wp:cat_name><wp:category_nicename>news</wp:category_nicename></wp:category>
`)
	f := writeWXR(t, wxr)
	report, _, err := im.Import(ctx, f, Options{DownloadMedia: false})
	if err != nil {
		t.Fatal(err)
	}
	if report.Categories < 2 {
		t.Fatalf("Categories = %d want >=2", report.Categories)
	}
	// Durable mapping MUST use the WP numeric term ID, not the slug.
	childID, err := q.GetImportMapping(ctx, db.GetImportMappingParams{Source: source, ObjectType: "term", ExternalID: "category:21"})
	if err != nil || childID == "" {
		t.Fatalf("missing WP-ID term mapping category:21: %v", err)
	}
	parentID, err := q.GetImportMapping(ctx, db.GetImportMappingParams{Source: source, ObjectType: "term", ExternalID: "category:20"})
	if err != nil || parentID == "" {
		t.Fatalf("missing WP-ID term mapping category:20: %v", err)
	}
	child, _ := q.GetTerm(ctx, childID)
	if !child.ParentID.Valid || child.ParentID.String != parentID {
		t.Fatalf("two-pass hierarchy failed: child.ParentID=%v want %s", child.ParentID, parentID)
	}
}

func TestChangedTermSlugSameWPIDDoesNotDuplicate(t *testing.T) {
	im, _, q, _ := newTestImporter(t)
	ctx := context.Background()
	f1 := writeWXR(t, wxrHeader(`<wp:category><wp:term_id>30</wp:term_id><wp:cat_name>News</wp:cat_name><wp:category_nicename>news</wp:category_nicename></wp:category>`))
	if _, _, err := im.Import(ctx, f1, Options{DownloadMedia: false}); err != nil {
		t.Fatal(err)
	}
	// Same WP ID, renamed slug in a later export.
	f2 := writeWXR(t, wxrHeader(`<wp:category><wp:term_id>30</wp:term_id><wp:cat_name>News</wp:cat_name><wp:category_nicename>newsroom</wp:category_nicename></wp:category>`))
	report2, _, err := im.Import(ctx, f2, Options{DownloadMedia: false})
	if err != nil {
		t.Fatal(err)
	}
	n, _ := q.CountTermsByTaxonomy(ctx, "category")
	if n != 1 {
		t.Fatalf("same WP term ID duplicated across slugs: %d terms", n)
	}
	mapped, _ := q.GetImportMapping(ctx, db.GetImportMappingParams{Source: source, ObjectType: "term", ExternalID: "category:30"})
	if mapped == "" {
		t.Fatal("mapping lost")
	}
	_ = report2
}

// --- §41 AUTHOR TESTS --------------------------------------------------------

func stratumUser(t *testing.T, q interface {
	CreateUser(ctx context.Context, arg db.CreateUserParams) error
}, id, email string) {
	t.Helper()
	if err := q.CreateUser(context.Background(), db.CreateUserParams{ID: id, Email: email, PasswordHash: "$2a$10$dummyhashdummyhashdummyhashdummyha", Role: "admin", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
}

func TestAuthorFallbackMatrix(t *testing.T) {
	base := `<item><title>P%s</title><content:encoded><![CDATA[<p>x</p>]]></content:encoded><wp:post_id>%s</wp:post_id><wp:post_type>post</wp:post_type><wp:status>draft</wp:status><wp:post_name>p%s</wp:post_name><wp:post_date_gmt>2020-01-02 03:04:05</wp:post_date_gmt>%s</item>`

	t.Run("WP email matches Stratum user", func(t *testing.T) {
		im, _, q, _ := newTestImporter(t)
		stratumUser(t, q, "u-wp", "author@example.test")
		f := writeWXR(t, wxrHeader(fmt.Sprintf(base, "A", "11", "11", `<dc:creator><![CDATA[author]]></dc:creator>`)))
		if _, _, err := im.Import(ctx0(), f, Options{DownloadMedia: false}); err != nil {
			t.Fatal(err)
		}
		eID, _ := q.GetImportMapping(ctx0(), db.GetImportMappingParams{Source: source, ObjectType: "post", ExternalID: "11"})
		e, _ := q.GetEntry(ctx0(), eID)
		if !e.AuthorID.Valid || e.AuthorID.String != "u-wp" {
			t.Fatalf("want WP-matched author u-wp, got %v", e.AuthorID)
		}
	})

	t.Run("fallback --author used when WP email unknown", func(t *testing.T) {
		im, _, q, _ := newTestImporter(t)
		stratumUser(t, q, "u-fb", "fallback@example.test")
		f := writeWXR(t, wxrHeader(fmt.Sprintf(base, "B", "12", "12", `<dc:creator><![CDATA[stranger]]></dc:creator><wp:author><wp:author_login>stranger</wp:author_login><wp:author_email>stranger@wx.test</wp:author_email></wp:author>`)))
		if _, _, err := im.Import(ctx0(), f, Options{DownloadMedia: false, Author: "fallback@example.test"}); err != nil {
			t.Fatal(err)
		}
		eID, _ := q.GetImportMapping(ctx0(), db.GetImportMappingParams{Source: source, ObjectType: "post", ExternalID: "12"})
		e, _ := q.GetEntry(ctx0(), eID)
		if !e.AuthorID.Valid || e.AuthorID.String != "u-fb" {
			t.Fatalf("want fallback author u-fb, got %v", e.AuthorID)
		}
	})

	t.Run("invalid --author fails before mutation", func(t *testing.T) {
		im, _, q, _ := newTestImporter(t)
		f := writeWXR(t, wxrHeader(fmt.Sprintf(base, "C", "13", "13", `<dc:creator><![CDATA[whoever]]></dc:creator>`)))
		if _, _, err := im.Import(ctx0(), f, Options{DownloadMedia: false, Author: "ghost@example.test"}); err == nil {
			t.Fatal("invalid --author must fail fast")
		}
		if n := countEntries(t, q); n != 0 {
			t.Fatalf("mutation happened despite invalid --author: %d entries", n)
		}
	})

	t.Run("no match anywhere yields NULL author", func(t *testing.T) {
		im, _, q, _ := newTestImporter(t)
		f := writeWXR(t, wxrHeader(fmt.Sprintf(base, "D", "14", "14", `<dc:creator><![CDATA[nobody]]></dc:creator>`)))
		if _, _, err := im.Import(ctx0(), f, Options{DownloadMedia: false}); err != nil {
			t.Fatal(err)
		}
		eID, _ := q.GetImportMapping(ctx0(), db.GetImportMappingParams{Source: source, ObjectType: "post", ExternalID: "14"})
		e, _ := q.GetEntry(ctx0(), eID)
		if e.AuthorID.Valid {
			t.Fatalf("expected NULL author, got %q", e.AuthorID.String)
		}
	})
}

func mustListRevisions(t *testing.T, q *db.Queries, entryID string) []db.EntryRevision {
	t.Helper()
	revs, err := q.ListEntryRevisions(context.Background(), entryID)
	if err != nil {
		t.Fatalf("ListEntryRevisions: %v", err)
	}
	return revs
}

func sqlNull(v int64) sql.NullInt64 { return sql.NullInt64{Int64: v, Valid: true} }

func ctx0() context.Context { return context.Background() }

// TestImportHostileOrdering proves order-independence: grandchild before child
// before root, attachment after the post referencing it, tag after the item,
// author declared last.
func TestImportHostileOrdering(t *testing.T) {
	im, _, q, _ := newTestImporter(t)
	ctx := context.Background()
	report, _, err := im.Import(ctx, filepath.Join("testdata", "20_hostile_order.xml"), Options{DownloadMedia: false})
	if err != nil {
		t.Fatalf("hostile-order import: %v", err)
	}
	if report.Pages != 3 || report.Posts != 1 {
		t.Fatalf("Pages=%d Posts=%d; hostile ordering must not lose items", report.Pages, report.Posts)
	}
	// Deep nested route resolved despite reverse ordering.
	for _, p := range []string{"/rooty", "/rooty/mid", "/rooty/mid/leaf"} {
		if _, err := im.q.GetRouteByPath(ctx, p); err != nil {
			t.Fatalf("expected route %s: %v", p, err)
		}
	}
	// Tag assignment survived term-after-item ordering (revision-scoped).
	eID, _ := q.GetImportMapping(ctx, db.GetImportMappingParams{Source: source, ObjectType: "post", ExternalID: "101"})
	rev, _ := q.GetLatestEntryRevision(ctx, eID)
	terms, _ := q.ListTermsForRevision(ctx, rev.ID)
	if len(terms) != 1 || terms[0].Slug != "hostile-tag" {
		t.Fatalf("revision-scoped term assignment lost: %+v", terms)
	}
	// Term mapping keyed by WP numeric ID.
	if _, err := q.GetImportMapping(ctx, db.GetImportMappingParams{Source: source, ObjectType: "term", ExternalID: "tag:77"}); err != nil {
		t.Fatalf("WP-ID term mapping missing: %v", err)
	}
}
