package media

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/kokosx/stratum/internal/content"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

// Helper to create an entry with multiple revisions for history tests
func createEntryWithRevisions(t *testing.T, queries *db.Queries, entryID, contentType, titlePrefix string, docs []string, publishIdx int) string {
	t.Helper()
	ctx := context.Background()
	now := time.Now().Unix()
	// Create entry if not exists
	_ = queries.CreateEntry(ctx, db.CreateEntryParams{
		ID: entryID, ContentTypeID: contentType, Slug: "slug-" + entryID, Status: "active", CreatedAt: now, UpdatedAt: now,
	})
	for i, doc := range docs {
		revID := fmt.Sprintf("%s_rev%d", entryID, i+1)
		if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{
			ID: revID, EntryID: entryID, RevisionNumber: int64(i + 1), Slug: "slug-" + entryID, Title: fmt.Sprintf("%s-%d", titlePrefix, i+1), DocumentJson: doc, CreatedAt: now + int64(i), FieldsJson: "{}", Visibility: "public", ReviewState: "draft",
		}); err != nil {
			// If duplicate, try update? For simplicity fail
			t.Fatalf("CreateEntryRevision %d: %v", i+1, err)
		}
		if i == publishIdx {
			if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{
				ID: entryID, PublishedRevisionID: sql.NullString{String: revID, Valid: true}, PublishedAt: sql.NullInt64{Int64: now + int64(i), Valid: true}, UpdatedAt: now + int64(i),
			}); err != nil {
				t.Fatalf("SetPublished %v", err)
			}
		}
	}
	// Ensure route exists for published entry if publishIdx >=0
	if publishIdx >= 0 {
		_ = queries.CreateRoute(ctx, db.CreateRouteParams{
			ID: "route_" + entryID, Path: "/slug-" + entryID, EntryID: sql.NullString{String: entryID, Valid: true}, RouteType: "entry", CreatedAt: now, UpdatedAt: now,
		})
	}
	return entryID
}

func TestMediaHistoryOnlyLatestCounts(t *testing.T) {
	svc, _, queries := newTestServiceWithDB(t)
	ctx := context.Background()

	// Create three media assets
	a, _ := svc.Upload(ctx, "a.jpg", "", bytes.NewReader(testPNG(t, 800, 600)))
	b, _ := svc.Upload(ctx, "b.jpg", "", bytes.NewReader(testPNG(t, 800, 600)))
	c, _ := svc.Upload(ctx, "c.jpg", "", bytes.NewReader(testPNG(t, 800, 600)))

	// Create entry with history: rev1 -> A, rev2 -> B, rev3 -> C (published & latest = C)
	createEntryWithRevisions(t, queries, "entry_hist", "page", "Hist", []string{
		docWithImage(a.ID),
		docWithImage(b.ID),
		docWithImage(c.ID),
	}, 2) // publish rev3 (index 2)

	// Build index and check usage
	idx, err := svc.BuildUsageIndex(ctx)
	if err != nil {
		t.Fatalf("BuildUsageIndex: %v", err)
	}
	if idx.IsUsed(a.ID) {
		t.Fatalf("media A should be unused (only in historical rev1)")
	}
	if idx.IsUsed(b.ID) {
		t.Fatalf("media B should be unused (only in historical rev2)")
	}
	if !idx.IsUsed(c.ID) {
		t.Fatalf("media C should be used (published+latest)")
	}
	refsC := idx.Refs(c.ID)
	if len(refsC) != 1 {
		t.Fatalf("C refs %d want 1", len(refsC))
	}
	if !refsC[0].Public {
		t.Fatalf("C should be public")
	}
}

func TestPublishedPlusDraft(t *testing.T) {
	svc, _, queries := newTestServiceWithDB(t)
	ctx := context.Background()
	a, _ := svc.Upload(ctx, "a2.jpg", "", bytes.NewReader(testPNG(t, 800, 600)))
	b, _ := svc.Upload(ctx, "b2.jpg", "", bytes.NewReader(testPNG(t, 800, 600)))

	// Create entry where published uses A, latest draft uses B
	entryID := "entry_pd"
	now := time.Now().Unix()
	_ = queries.CreateEntry(ctx, db.CreateEntryParams{ID: entryID, ContentTypeID: "page", Slug: "slug-pd", Status: "active", CreatedAt: now, UpdatedAt: now})
	revPub := entryID + "_rev1"
	_ = queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{
		ID: revPub, EntryID: entryID, RevisionNumber: 1, Slug: "slug-pd", Title: "Published", DocumentJson: docWithImage(a.ID), CreatedAt: now, FieldsJson: "{}", Visibility: "public", ReviewState: "draft",
	})
	_ = queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{ID: entryID, PublishedRevisionID: sql.NullString{String: revPub, Valid: true}, PublishedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now})
	_ = queries.CreateRoute(ctx, db.CreateRouteParams{ID: "route_" + entryID, Path: "/slug-pd", EntryID: sql.NullString{String: entryID, Valid: true}, RouteType: "entry", CreatedAt: now, UpdatedAt: now})
	revDraft := entryID + "_rev2"
	_ = queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{
		ID: revDraft, EntryID: entryID, RevisionNumber: 2, Slug: "slug-pd", Title: "Draft", DocumentJson: docWithImage(b.ID), CreatedAt: now + 10, FieldsJson: "{}", Visibility: "public", ReviewState: "draft",
	})

	idx, err := svc.BuildUsageIndex(ctx)
	if err != nil {
		t.Fatalf("BuildUsageIndex: %v", err)
	}
	if !idx.IsUsed(a.ID) {
		t.Fatalf("A should be used (published)")
	}
	if !idx.IsUsed(b.ID) {
		t.Fatalf("B should be used (draft)")
	}
	refsA := idx.Refs(a.ID)
	if len(refsA) != 1 || !refsA[0].Public {
		t.Fatalf("A refs should be 1 public, got %+v", refsA)
	}
	refsB := idx.Refs(b.ID)
	if len(refsB) != 1 || refsB[0].Public {
		t.Fatalf("B refs should be 1 draft (Public false), got %+v", refsB)
	}
	// Simulate publish draft B -> now A becomes unused
	_ = queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{ID: entryID, PublishedRevisionID: sql.NullString{String: revDraft, Valid: true}, PublishedAt: sql.NullInt64{Int64: now + 20, Valid: true}, UpdatedAt: now + 20})
	idx2, err := svc.BuildUsageIndex(ctx)
	if err != nil {
		t.Fatalf("second index: %v", err)
	}
	if idx2.IsUsed(a.ID) {
		t.Fatalf("after republish, A should be unused")
	}
	if !idx2.IsUsed(b.ID) {
		t.Fatalf("after republish, B should be used")
	}
}

func TestTemplateDraftProtectsMedia(t *testing.T) {
	svc, _, _ := newTestServiceWithDB(t)
	ctx := context.Background()
	a, _ := svc.Upload(ctx, "ta.jpg", "", bytes.NewReader(testPNG(t, 800, 600)))
	b, _ := svc.Upload(ctx, "tb.jpg", "", bytes.NewReader(testPNG(t, 800, 600)))

	// Create template with published A, draft B
	tmplID := "tmpl_hist"
	now := time.Now().Unix()
	if _, err := svc.db.ExecContext(ctx, `INSERT INTO layout_templates (id, name, content_type_id, kind, published_revision_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, tmplID, "My Template", "page", "single", nil, now, now); err != nil {
		t.Fatalf("insert template: %v", err)
	}
	rev1 := tmplID + "_rev1"
	if _, err := svc.db.ExecContext(ctx, `INSERT INTO layout_template_revisions (id, template_id, revision_number, document_json, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?)`, rev1, tmplID, 1, docWithImage(a.ID), "", now); err != nil {
		t.Fatalf("rev1: %v", err)
	}
	if _, err := svc.db.ExecContext(ctx, `UPDATE layout_templates SET published_revision_id = ? WHERE id = ?`, rev1, tmplID); err != nil {
		t.Fatalf("publish: %v", err)
	}
	rev2 := tmplID + "_rev2"
	if _, err := svc.db.ExecContext(ctx, `INSERT INTO layout_template_revisions (id, template_id, revision_number, document_json, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?)`, rev2, tmplID, 2, docWithImage(b.ID), "", now+10); err != nil {
		t.Fatalf("rev2: %v", err)
	}
	idx, err := svc.BuildUsageIndex(ctx)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	if !idx.IsUsed(a.ID) {
		t.Fatalf("published template media A should be used")
	}
	if !idx.IsUsed(b.ID) {
		t.Fatalf("draft template media B should be used")
	}
}

func TestSitePartDraftProtectsMedia(t *testing.T) {
	svc, _, _ := newTestServiceWithDB(t)
	ctx := context.Background()
	a, _ := svc.Upload(ctx, "spa.jpg", "", bytes.NewReader(testPNG(t, 800, 600)))
	b, _ := svc.Upload(ctx, "spb.jpg", "", bytes.NewReader(testPNG(t, 800, 600)))

	spID := "sp_hist"
	now := time.Now().Unix()
	if _, err := svc.db.ExecContext(ctx, `INSERT INTO site_parts (id, name, published_revision_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, spID, "Header", nil, now, now); err != nil {
		t.Fatalf("insert sp: %v", err)
	}
	rev1 := spID + "_rev1"
	if _, err := svc.db.ExecContext(ctx, `INSERT INTO site_part_revisions (id, site_part_id, revision_number, document_json, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?)`, rev1, spID, 1, docWithImage(a.ID), "", now); err != nil {
		t.Fatalf("rev1: %v", err)
	}
	if _, err := svc.db.ExecContext(ctx, `UPDATE site_parts SET published_revision_id = ? WHERE id = ?`, rev1, spID); err != nil {
		t.Fatalf("publish: %v", err)
	}
	rev2 := spID + "_rev2"
	if _, err := svc.db.ExecContext(ctx, `INSERT INTO site_part_revisions (id, site_part_id, revision_number, document_json, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?)`, rev2, spID, 2, docWithImage(b.ID), "", now+10); err != nil {
		t.Fatalf("rev2: %v", err)
	}
	idx, err := svc.BuildUsageIndex(ctx)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	if !idx.IsUsed(a.ID) || !idx.IsUsed(b.ID) {
		t.Fatalf("both published and draft site part media should be used, got A=%v B=%v", idx.IsUsed(a.ID), idx.IsUsed(b.ID))
	}
}

func TestHistoricalMediaDeleteAllowed(t *testing.T) {
	svc, _, queries := newTestServiceWithDB(t)
	ctx := context.Background()
	a, _ := svc.Upload(ctx, "histdel.jpg", "", bytes.NewReader(testPNG(t, 800, 600)))
	// Create entry history where only old revision contains a, latest does not
	createEntryWithRevisions(t, queries, "entry_hist_del", "page", "HistDel", []string{
		docWithImage(a.ID),
		`{"version":1,"nodes":[]}`,
		`{"version":1,"nodes":[]}`,
	}, 2) // published is empty (latest)
	idx, err := svc.BuildUsageIndex(ctx)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	if idx.IsUsed(a.ID) {
		t.Fatalf("historical-only media should be unused")
	}
	if err := svc.DeleteIfUnused(ctx, a.ID); err != nil {
		t.Fatalf("DeleteIfUnused should succeed for historical-only media: %v", err)
	}
	if _, err := svc.Get(ctx, a.ID); err == nil {
		t.Fatalf("media should be deleted")
	}
}

func TestStructuredBlockScanning(t *testing.T) {
	svc, _, queries := newTestServiceWithDB(t)
	ctx := context.Background()
	a, _ := svc.Upload(ctx, "struct.jpg", "", bytes.NewReader(testPNG(t, 800, 600)))

	// Entry with custom block that has string prop equal to media ID but not a media block
	// This should NOT be considered a reference (precise scanning)
	doc := fmt.Sprintf(`{"version":1,"nodes":[{"id":"n1","block":"core/text","version":1,"props":{"text":"%s"},"settings":{}}]}`, a.ID)
	createEntryWithRevisions(t, queries, "entry_struct", "page", "Struct", []string{doc}, 0)
	idx, err := svc.BuildUsageIndex(ctx)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	if idx.IsUsed(a.ID) {
		t.Fatalf("generic text block containing media ID string should not be considered usage")
	}
	// Now with proper image block it should be used
	entry2 := "entry_struct2"
	now := time.Now().Unix()
	_ = queries.CreateEntry(ctx, db.CreateEntryParams{ID: entry2, ContentTypeID: "page", Slug: "slug-struct2", Status: "active", CreatedAt: now, UpdatedAt: now})
	rev := entry2 + "_rev1"
	_ = queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{
		ID: rev, EntryID: entry2, RevisionNumber: 1, Slug: "slug-struct2", Title: "Struct2", DocumentJson: docWithImage(a.ID), CreatedAt: now, FieldsJson: "{}", Visibility: "public", ReviewState: "draft",
	})
	_ = queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{ID: entry2, PublishedRevisionID: sql.NullString{String: rev, Valid: true}, PublishedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now})
	_ = queries.CreateRoute(ctx, db.CreateRouteParams{ID: "route_" + entry2, Path: "/slug-struct2", EntryID: sql.NullString{String: entry2, Valid: true}, RouteType: "entry", CreatedAt: now, UpdatedAt: now})
	idx2, _ := svc.BuildUsageIndex(ctx)
	if !idx2.IsUsed(a.ID) {
		t.Fatalf("image block should be considered usage")
	}
}

func TestTypedCustomMediaFields(t *testing.T) {
	svc, _, queries := newTestServiceWithDB(t)
	ctx := context.Background()
	// Create custom content type with media field and text field
	cat := content.NewCatalog(queries)
	if err := cat.CreateContentType(ctx, content.ContentTypeInput{
		ID: "product", PluralName: "Products", Name: "Product",
		Config: content.ContentTypeConfig{
			SchemaVersion: 1,
			Fields: []content.FieldDefinition{
				{Key: "hero_media", Label: "Hero Media", Type: content.FieldMedia},
				{Key: "description", Label: "Description", Type: content.FieldText},
			},
			Routing: content.ContentTypeRouting{Single: true, BasePath: "/products"},
		},
	}); err != nil && !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("create ct: %v", err)
	}
	a, _ := svc.Upload(ctx, "fieldmedia.jpg", "", bytes.NewReader(testPNG(t, 800, 600)))
	// Create entry with fields_json where hero_media = a, description contains a as text (should not count)
	fields, _ := content.EncodeFieldSnapshot(map[string]any{"hero_media": a.ID, "description": a.ID})
	entryID := "entry_field_test"
	now := time.Now().Unix()
	_ = queries.CreateEntry(ctx, db.CreateEntryParams{ID: entryID, ContentTypeID: "product", Slug: "prod-field", Status: "active", CreatedAt: now, UpdatedAt: now})
	rev := entryID + "_rev1"
	_ = queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{
		ID: rev, EntryID: entryID, RevisionNumber: 1, Slug: "prod-field", Title: "Product with media field", DocumentJson: `{"version":1,"nodes":[]}`, FieldsJson: fields, CreatedAt: now, Visibility: "public", ReviewState: "draft",
	})
	_ = queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{ID: entryID, PublishedRevisionID: sql.NullString{String: rev, Valid: true}, PublishedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now})
	_ = queries.CreateRoute(ctx, db.CreateRouteParams{ID: "route_" + entryID, Path: "/products/prod-field", EntryID: sql.NullString{String: entryID, Valid: true}, RouteType: "entry", CreatedAt: now, UpdatedAt: now})

	idx, err := svc.BuildUsageIndex(ctx)
	if err != nil {
		t.Fatalf("index: %v", err)
	}
	if !idx.IsUsed(a.ID) {
		t.Fatalf("media via typed field should be used")
	}

	// Now test that text field containing media ID does NOT count
	b, _ := svc.Upload(ctx, "fieldmedia2.jpg", "", bytes.NewReader(testPNG(t, 800, 600)))
	fields2, _ := content.EncodeFieldSnapshot(map[string]any{"description": b.ID}) // only text field, no hero_media
	// Need to bypass validation for this test, so insert raw JSON directly
	malFields := fmt.Sprintf(`{"description":"%s"}`, b.ID)
	entryID2 := "entry_field_test2"
	_ = queries.CreateEntry(ctx, db.CreateEntryParams{ID: entryID2, ContentTypeID: "product", Slug: "prod-field2", Status: "active", CreatedAt: now, UpdatedAt: now})
	rev2 := entryID2 + "_rev1"
	// Use raw insert to avoid validation that would require hero_media
	_, err = svc.db.ExecContext(ctx, `INSERT INTO entry_revisions (id, entry_id, revision_number, slug, title, document_json, fields_json, created_at, visibility, review_state) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`, rev2, entryID2, 1, "prod-field2", "Product2", `{"version":1,"nodes":[]}`, malFields, now, "public", "draft")
	if err != nil {
		t.Fatalf("insert rev2: %v", err)
	}
	_ = queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{ID: entryID2, PublishedRevisionID: sql.NullString{String: rev2, Valid: true}, PublishedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now})
	_ = queries.CreateRoute(ctx, db.CreateRouteParams{ID: "route_" + entryID2, Path: "/products/prod-field2", EntryID: sql.NullString{String: entryID2, Valid: true}, RouteType: "entry", CreatedAt: now, UpdatedAt: now})
	// Also test that EncodeFieldSnapshot not needed; we used raw
	_ = fields2

	idx3, _ := svc.BuildUsageIndex(ctx)
	if idx3.IsUsed(b.ID) {
		t.Fatalf("text field containing media ID should not be considered usage")
	}

	// Invalid fields JSON handling is tested at helper level (DB check constraint prevents storing invalid JSON,
	// but helper must return error and not use Contains fallback)
	_, err = mediaIDsFromFields(`{invalid json`, "product", map[string]content.ContentTypeDefinition{
		"product": {ID: "product", Fields: []content.FieldDefinition{{Key: "hero_media", Label: "Hero", Type: content.FieldMedia}}},
	})
	if err == nil {
		t.Fatalf("invalid fields JSON should cause error, not fallback")
	}
	// Ensure that generic string Contains fallback is not used (previous code used strings.Contains)
	// If it used fallback, it would have returned true for media ID inside invalid JSON string.
	m, _ := mediaIDsFromFields(`{"description":"media_123"}`, "product", map[string]content.ContentTypeDefinition{
		"product": {ID: "product", Fields: []content.FieldDefinition{{Key: "description", Label: "Desc", Type: content.FieldText}}},
	})
	if len(m) != 0 {
		t.Fatalf("text field should not be considered media")
	}
}

func TestUsageDeduplication(t *testing.T) {
	svc, _, queries := newTestServiceWithDB(t)
	ctx := context.Background()
	a, _ := svc.Upload(ctx, "dedup.jpg", "", bytes.NewReader(testPNG(t, 800, 600)))
	// Entry where published and draft both contain same media A (should be one ref, not two)
	entryID := "entry_dedup"
	now := time.Now().Unix()
	_ = queries.CreateEntry(ctx, db.CreateEntryParams{ID: entryID, ContentTypeID: "page", Slug: "dedup", Status: "active", CreatedAt: now, UpdatedAt: now})
	rev1 := entryID + "_rev1"
	_ = queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: rev1, EntryID: entryID, RevisionNumber: 1, Slug: "dedup", Title: "Dedup Pub", DocumentJson: docWithImage(a.ID), CreatedAt: now, FieldsJson: "{}", Visibility: "public", ReviewState: "draft"})
	_ = queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{ID: entryID, PublishedRevisionID: sql.NullString{String: rev1, Valid: true}, PublishedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now})
	_ = queries.CreateRoute(ctx, db.CreateRouteParams{ID: "route_" + entryID, Path: "/dedup", EntryID: sql.NullString{String: entryID, Valid: true}, RouteType: "entry", CreatedAt: now, UpdatedAt: now})
	rev2 := entryID + "_rev2"
	_ = queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: rev2, EntryID: entryID, RevisionNumber: 2, Slug: "dedup", Title: "Dedup Draft", DocumentJson: docWithImage(a.ID), CreatedAt: now + 10, FieldsJson: "{}", Visibility: "public", ReviewState: "draft"})
	idx, _ := svc.BuildUsageIndex(ctx)
	refs := idx.Refs(a.ID)
	count := 0
	for _, r := range refs {
		if r.SourceID == entryID {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("dedup: expected 1 ref for entry with same media in pub+draft, got %d refs %+v", count, refs)
	}
}

func TestUnusedFilterPerformance(t *testing.T) {
	svc, _, queries := newTestServiceWithDB(t)
	ctx := context.Background()
	// Create 200 entries each with one media? Simpler: create 100 entries and 200 media
	// Use fewer for speed but still demonstrate N+1 fix via scan count

	// Create 50 media
	var medias []string
	for i := 0; i < 50; i++ {
		m, _ := svc.Upload(ctx, fmt.Sprintf("perf%d.jpg", i), "", bytes.NewReader(testPNG(t, 400, 300)))
		medias = append(medias, m.ID)
	}
	// Create 20 entries using first 10 medias
	for i := 0; i < 20; i++ {
		entryID := fmt.Sprintf("perf_entry_%d", i)
		mediaID := medias[i%10]
		now := time.Now().Unix() + int64(i)
		_ = queries.CreateEntry(ctx, db.CreateEntryParams{ID: entryID, ContentTypeID: "page", Slug: fmt.Sprintf("perf-%d", i), Status: "active", CreatedAt: now, UpdatedAt: now})
		rev := entryID + "_rev1"
		_ = queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: rev, EntryID: entryID, RevisionNumber: 1, Slug: fmt.Sprintf("perf-%d", i), Title: fmt.Sprintf("Perf %d", i), DocumentJson: docWithImage(mediaID), CreatedAt: now, FieldsJson: "{}", Visibility: "public", ReviewState: "draft"})
		_ = queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{ID: entryID, PublishedRevisionID: sql.NullString{String: rev, Valid: true}, PublishedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now})
		_ = queries.CreateRoute(ctx, db.CreateRouteParams{ID: "route_" + entryID, Path: fmt.Sprintf("/perf-%d", i), EntryID: sql.NullString{String: entryID, Valid: true}, RouteType: "entry", CreatedAt: now, UpdatedAt: now})
	}

	before := svc.UsageBuildCount()
	assets, total, err := svc.ListFiltered(ctx, ListParams{Filter: "unused", Limit: 40, Offset: 0})
	if err != nil {
		t.Fatalf("ListFiltered unused: %v", err)
	}
	after := svc.UsageBuildCount()
	if after-before != 1 {
		t.Fatalf("unused filter should build usage index once per request, got %d builds", after-before)
	}
	// Verify correctness: unused should be 40 (50 total -10 used)
	if total != 40 {
		t.Fatalf("unused total %d want 40", total)
	}
	if len(assets) != 40 {
		t.Fatalf("len assets %d want 40", len(assets))
	}
	// Also ensure no N+1: we already proved via count, but could also check that per-media queries not issued
	_ = assets
}

func TestFaviconFailurePreservesOld(t *testing.T) {
	svc, _, _ := newTestServiceWithDB(t)
	ctx := context.Background()
	a, _ := svc.Upload(ctx, "icon_fail.jpg", "", bytes.NewReader(testPNG(t, 512, 512)))
	if err := svc.GenerateFaviconVariants(ctx, a.ID); err != nil {
		t.Fatalf("initial favicon: %v", err)
	}
	beforeView, ok := svc.FaviconView(ctx, a.ID)
	if !ok {
		t.Fatalf("before view missing")
	}
	// Capture old variant rows
	oldVars, _ := svc.queries.ListMediaVariants(ctx, a.ID)
	var oldFavCount int
	for _, v := range oldVars {
		if strings.HasPrefix(v.Kind, "favicon-") {
			oldFavCount++
		}
	}
	// Wrap store to fail on third Put (favicon)
	origStore := svc.store
	fStore := &failingStorage{Storage: origStore, failAfter: 2}
	svc.store = fStore
	err := svc.GenerateFaviconVariants(ctx, a.ID)
	if err == nil {
		t.Fatalf("expected favicon generation failure")
	}
	svc.store = origStore
	afterView, ok := svc.FaviconView(ctx, a.ID)
	if !ok {
		t.Fatalf("after view missing, old should remain")
	}
	if afterView.Size32 != beforeView.Size32 {
		t.Fatalf("old favicon should remain after failure, before %q after %q", beforeView.Size32, afterView.Size32)
	}
	varsAfter, _ := svc.queries.ListMediaVariants(ctx, a.ID)
	var afterFavCount int
	for _, v := range varsAfter {
		if strings.HasPrefix(v.Kind, "favicon-") {
			afterFavCount++
			// Ensure old blobs still readable
			if _, _, err := svc.ReadVariant(ctx, a.ID, v.Kind); err != nil {
				t.Fatalf("old favicon blob not readable after failure: %v", err)
			}
		}
	}
	if afterFavCount != oldFavCount {
		t.Fatalf("fav count after failure %d want %d", afterFavCount, oldFavCount)
	}
}

func TestFaviconSuccess(t *testing.T) {
	svc, _, _ := newTestServiceWithDB(t)
	ctx := context.Background()
	a, _ := svc.Upload(ctx, "icon_ok.jpg", "", bytes.NewReader(testPNG(t, 512, 512)))
	if err := svc.GenerateFaviconVariants(ctx, a.ID); err != nil {
		t.Fatalf("initial: %v", err)
	}
	// Set as site icon
	if _, err := svc.db.ExecContext(ctx, `UPDATE site_settings SET site_icon_media_id = ? WHERE id=1`, a.ID); err != nil {
		t.Fatalf("set icon: %v", err)
	}
	beforeView, _ := svc.FaviconView(ctx, a.ID)
	// Replace site icon media
	newImg := testPNG(t, 800, 800)
	replaced, err := svc.Replace(ctx, a.ID, "newicon.jpg", bytes.NewReader(newImg))
	if err != nil {
		t.Fatalf("replace: %v", err)
	}
	afterView, ok := svc.FaviconView(ctx, replaced.ID)
	if !ok {
		t.Fatalf("after favicon missing")
	}
	if afterView.Size32 == beforeView.Size32 {
		t.Fatalf("favicon URL should change after replace")
	}
	// Old keys should be removed after commit (we can't easily check old keys without capturing)
	// Check that new variants exist and are readable
	for _, kind := range []string{"favicon-16", "favicon-32", "favicon-180", "favicon-192", "favicon-512"} {
		if _, _, err := svc.ReadVariant(ctx, a.ID, kind); err != nil {
			t.Fatalf("new favicon %s not readable: %v", kind, err)
		}
	}
}

func TestMediaDetailFailureDoesNotReportZero(t *testing.T) {
	svc, database, _ := newTestServiceWithDB(t)
	ctx := context.Background()
	a, _ := svc.Upload(ctx, "detail.jpg", "", bytes.NewReader(testPNG(t, 800, 600)))
	// Close DB to force scanner failure
	_ = database.Close()
	// Now UsageRefs should error
	_, err := svc.UsageRefs(ctx, a.ID)
	if err == nil {
		t.Fatalf("expected scanner error after DB close")
	}
	// Simulate handler logic: should not report 0 usages
	refs, err := svc.UsageRefs(ctx, a.ID)
	if err == nil {
		t.Fatalf("expected error")
	}
	if refs != nil && len(refs) == 0 {
		// Handler should treat this as not authoritative, not as 0
		// Our handler returns UsageError; here we just verify err != nil
	}
	// Delete must fail safe (not delete)
	err = svc.DeleteIfUnused(ctx, a.ID)
	if err == nil {
		t.Fatalf("DeleteIfUnused should fail safe when scanner fails")
	}
}

func TestDeleteIfUnusedHistoricalAllowed(t *testing.T) {
	svc, _, queries := newTestServiceWithDB(t)
	ctx := context.Background()
	m, _ := svc.Upload(ctx, "histallow.jpg", "", bytes.NewReader(testPNG(t, 800, 600)))
	// Old revision contains m, latest does not
	createEntryWithRevisions(t, queries, "entry_hist_allow", "page", "HistAllow", []string{
		docWithImage(m.ID),
		`{"version":1,"nodes":[]}`,
	}, 1) // publish latest empty, so historical only
	if err := svc.DeleteIfUnused(ctx, m.ID); err != nil {
		t.Fatalf("historical-only should allow delete, got %v", err)
	}
}

func TestGalleryBlockScanning(t *testing.T) {
	svc, _, queries := newTestServiceWithDB(t)
	ctx := context.Background()
	a, _ := svc.Upload(ctx, "g1.jpg", "", bytes.NewReader(testPNG(t, 800, 600)))
	b, _ := svc.Upload(ctx, "g2.jpg", "", bytes.NewReader(testPNG(t, 800, 600)))
	c, _ := svc.Upload(ctx, "g3.jpg", "", bytes.NewReader(testPNG(t, 800, 600)))

	// Gallery v2 array
	galleryDoc := fmt.Sprintf(`{"version":1,"nodes":[{"id":"g1","block":"core/gallery","version":2,"props":{"images":["%s","%s"]},"settings":{"columns":3}}]}`, a.ID, b.ID)
	entryID := "gallery_test"
	now := time.Now().Unix()
	_ = queries.CreateEntry(ctx, db.CreateEntryParams{ID: entryID, ContentTypeID: "page", Slug: "gallery", Status: "active", CreatedAt: now, UpdatedAt: now})
	rev := entryID + "_rev1"
	_ = queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: rev, EntryID: entryID, RevisionNumber: 1, Slug: "gallery", Title: "Gallery", DocumentJson: galleryDoc, CreatedAt: now, FieldsJson: "{}", Visibility: "public", ReviewState: "draft"})
	_ = queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{ID: entryID, PublishedRevisionID: sql.NullString{String: rev, Valid: true}, PublishedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now})
	_ = queries.CreateRoute(ctx, db.CreateRouteParams{ID: "route_" + entryID, Path: "/gallery", EntryID: sql.NullString{String: entryID, Valid: true}, RouteType: "entry", CreatedAt: now, UpdatedAt: now})

	idx, _ := svc.BuildUsageIndex(ctx)
	if !idx.IsUsed(a.ID) || !idx.IsUsed(b.ID) {
		t.Fatalf("gallery media should be used")
	}
	if idx.IsUsed(c.ID) {
		t.Fatalf("c should not be used")
	}
	// Legacy string form
	galleryLegacy := fmt.Sprintf(`{"version":1,"nodes":[{"id":"g1","block":"core/gallery","version":1,"props":{"images":"%s,%s"},"settings":{"columns":3}}]}`, b.ID, c.ID)
	entryID2 := "gallery_legacy"
	_ = queries.CreateEntry(ctx, db.CreateEntryParams{ID: entryID2, ContentTypeID: "page", Slug: "gallery-legacy", Status: "active", CreatedAt: now, UpdatedAt: now})
	rev2 := entryID2 + "_rev1"
	_ = queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: rev2, EntryID: entryID2, RevisionNumber: 1, Slug: "gallery-legacy", Title: "GalleryLegacy", DocumentJson: galleryLegacy, CreatedAt: now, FieldsJson: "{}", Visibility: "public", ReviewState: "draft"})
	_ = queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{ID: entryID2, PublishedRevisionID: sql.NullString{String: rev2, Valid: true}, PublishedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now})
	_ = queries.CreateRoute(ctx, db.CreateRouteParams{ID: "route_" + entryID2, Path: "/gallery-legacy", EntryID: sql.NullString{String: entryID2, Valid: true}, RouteType: "entry", CreatedAt: now, UpdatedAt: now})
	idx2, _ := svc.BuildUsageIndex(ctx)
	if !idx2.IsUsed(c.ID) {
		t.Fatalf("legacy gallery c should be used")
	}
}

func TestUsageOrderingDeterministic(t *testing.T) {
	svc, _, queries := newTestServiceWithDB(t)
	ctx := context.Background()
	m, _ := svc.Upload(ctx, "order.jpg", "", bytes.NewReader(testPNG(t, 800, 600)))
	// Create two entries using same media: one published, one draft-only
	createPublishedEntryWithImage(t, queries, svc, m.ID, "page", "B Title", docWithImage(m.ID))
	// Second entry draft
	entry2 := "entry_order2"
	now := time.Now().Unix()
	_ = queries.CreateEntry(ctx, db.CreateEntryParams{ID: entry2, ContentTypeID: "page", Slug: "order2", Status: "active", CreatedAt: now, UpdatedAt: now})
	rev2 := entry2 + "_rev1"
	_ = queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: rev2, EntryID: entry2, RevisionNumber: 1, Slug: "order2", Title: "A Title", DocumentJson: docWithImage(m.ID), CreatedAt: now, FieldsJson: "{}", Visibility: "public", ReviewState: "draft"})
	// Create template using same media
	tmplID := "tmpl_order"
	if _, err := svc.db.ExecContext(ctx, `INSERT INTO layout_templates (id, name, content_type_id, kind, published_revision_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`, tmplID, "Z Template", "page", "single", nil, now, now); err != nil {
		t.Fatalf("insert tmpl: %v", err)
	}
	revT := tmplID + "_rev1"
	if _, err := svc.db.ExecContext(ctx, `INSERT INTO layout_template_revisions (id, template_id, revision_number, document_json, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?)`, revT, tmplID, 1, docWithImage(m.ID), "", now); err != nil {
		t.Fatalf("revT: %v", err)
	}
	if _, err := svc.db.ExecContext(ctx, `UPDATE layout_templates SET published_revision_id = ? WHERE id = ?`, revT, tmplID); err != nil {
		t.Fatalf("pub tmpl: %v", err)
	}
	idx, _ := svc.BuildUsageIndex(ctx)
	refs := idx.Refs(m.ID)
	if len(refs) < 3 {
		t.Fatalf("want >=3 refs, got %d %+v", len(refs), refs)
	}
	// Check published first
	if !refs[0].Public {
		t.Fatalf("first ref should be published, got %+v", refs[0])
	}
	// Check deterministic: call again, should be same order
	idx2, _ := svc.BuildUsageIndex(ctx)
	refs2 := idx2.Refs(m.ID)
	for i := range refs {
		if refs[i] != refs2[i] {
			t.Fatalf("ordering not deterministic at %d: %v vs %v", i, refs[i], refs2[i])
		}
	}
}

// Helper to test failing storage (already defined in epic6_test, but redefine if not)
func init() {
	// Ensure failingStorage is available; epic6_test defines it but we need duplicate if not
}
