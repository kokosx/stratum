package layouts

import (
	"context"
	"database/sql"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/document"
	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func openTestDBWithRegistry(t *testing.T) (*storage.Database, *db.Queries, *blocks.Registry) {
	t.Helper()
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
	// Registry needs block definitions seeded by migrations (including content-slot)
	reg, err := blocks.NewRegistry(ctx, queries)
	if err != nil {
		t.Fatal(err)
	}
	return database, queries, reg
}

func createLayoutTemplate(t *testing.T, queries *db.Queries, id, name, ctID, docJSON string, published bool) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().Unix()
	if err := queries.CreateLayoutTemplate(ctx, db.CreateLayoutTemplateParams{ID: id, Name: name, ContentTypeID: ctID, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create layout %s: %v", id, err)
	}
	revID := id + "-r1"
	if err := queries.CreateLayoutTemplateRevision(ctx, db.CreateLayoutTemplateRevisionParams{ID: revID, TemplateID: id, RevisionNumber: 1, DocumentJson: docJSON, CreatedAt: now}); err != nil {
		t.Fatalf("create rev: %v", err)
	}
	if published {
		if err := queries.SetLayoutTemplatePublishedRevision(ctx, db.SetLayoutTemplatePublishedRevisionParams{PublishedRevisionID: sql.NullString{String: revID, Valid: true}, UpdatedAt: now, ID: id}); err != nil {
			t.Fatalf("publish: %v", err)
		}
	}
}

func TestLayoutTemplatePersistence(t *testing.T) {
	_, queries, _ := openTestDBWithRegistry(t)
	ctx := context.Background()
	doc := `{"version":1,"nodes":[{"id":"slot","block":"core/content-slot","version":1,"props":{},"settings":{}}]}`
	createLayoutTemplate(t, queries, "tmpl1", "Test", "page", doc, false)

	tmpl, err := queries.GetLayoutTemplate(ctx, "tmpl1")
	if err != nil {
		t.Fatal(err)
	}
	if tmpl.PublishedRevisionID.Valid {
		t.Fatal("should be unpublished")
	}
	rev, err := queries.GetLatestLayoutTemplateRevision(ctx, "tmpl1")
	if err != nil {
		t.Fatal(err)
	}
	if rev.RevisionNumber != 1 {
		t.Fatalf("rev = %d", rev.RevisionNumber)
	}
	// Save revision #2 still unpublished
	now := time.Now().Unix()
	if err := queries.CreateLayoutTemplateRevision(ctx, db.CreateLayoutTemplateRevisionParams{ID: "tmpl1-r2", TemplateID: "tmpl1", RevisionNumber: 2, DocumentJson: doc, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	tmpl, _ = queries.GetLayoutTemplate(ctx, "tmpl1")
	if tmpl.PublishedRevisionID.Valid {
		t.Fatal("still unpublished")
	}
	// Publish r2
	if err := queries.SetLayoutTemplatePublishedRevision(ctx, db.SetLayoutTemplatePublishedRevisionParams{PublishedRevisionID: sql.NullString{String: "tmpl1-r2", Valid: true}, UpdatedAt: now, ID: "tmpl1"}); err != nil {
		t.Fatal(err)
	}
	tmpl, _ = queries.GetLayoutTemplate(ctx, "tmpl1")
	if tmpl.PublishedRevisionID.String != "tmpl1-r2" {
		t.Fatalf("published = %s", tmpl.PublishedRevisionID.String)
	}
	latest, _ := queries.GetLatestLayoutTemplateRevision(ctx, "tmpl1")
	if latest.ID != "tmpl1-r2" {
		t.Fatal("latest mismatch")
	}
}

func TestRevisionAwareEntryTemplate(t *testing.T) {
	_, queries, reg := openTestDBWithRegistry(t)
	ctx := context.Background()
	// Layout A and B
	docA := `{"version":1,"nodes":[{"id":"secA","block":"core/section","version":1,"props":{},"settings":{},"children":[{"id":"slotA","block":"core/content-slot","version":1,"props":{},"settings":{}}]}]}`
	docB := `{"version":1,"nodes":[{"id":"secB","block":"core/section","version":1,"props":{},"settings":{},"children":[{"id":"slotB","block":"core/content-slot","version":1,"props":{},"settings":{}}]}]}`
	// but need simpler valid doc for test: use heading + slot
	docA2 := `{"version":1,"nodes":[{"id":"hA","block":"core/heading","version":1,"props":{"text":"LayoutA","level":1},"settings":{}},{"id":"slotA","block":"core/content-slot","version":1,"props":{},"settings":{}}]}`
	docB2 := `{"version":1,"nodes":[{"id":"hB","block":"core/heading","version":1,"props":{"text":"LayoutB","level":1},"settings":{}},{"id":"slotB","block":"core/content-slot","version":1,"props":{},"settings":{}}]}`
	_ = docA
	_ = docB
	createLayoutTemplate(t, queries, "layoutA", "LayoutA", "page", docA2, true)
	createLayoutTemplate(t, queries, "layoutB", "LayoutB", "page", docB2, true)

	// Create entry
	now := time.Now().Unix()
	entryID := "entry1"
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: entryID, ContentTypeID: "page", Slug: "test-page", Status: "active", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	entryDoc := `{"version":1,"nodes":[{"id":"t1","block":"core/text","version":1,"props":{"text":"body"},"settings":{}}]}`
	// revision 1 assigned to A, published
	rev1Doc, _ := document.Decode([]byte(entryDoc))
	if err := ValidateEntryDocument(reg, rev1Doc); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "rev1", EntryID: entryID, RevisionNumber: 1, Title: "T", DocumentJson: entryDoc, LayoutTemplateID: sql.NullString{String: "layoutA", Valid: true}, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: "rev1", Valid: true}, PublishedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now, ID: entryID}); err != nil {
		t.Fatal(err)
	}
	// draft revision 2 assigned to B, not published
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "rev2", EntryID: entryID, RevisionNumber: 2, Title: "T", DocumentJson: entryDoc, LayoutTemplateID: sql.NullString{String: "layoutB", Valid: true}, CreatedAt: now + 1}); err != nil {
		t.Fatal(err)
	}
	// Simulate public render: should use published revision's layout (A)
	publishedRow, err := queries.GetPublishedEntryByID(ctx, entryID)
	if err != nil {
		t.Fatal(err)
	}
	if publishedRow.LayoutTemplateID.String != "layoutA" {
		t.Fatalf("published layout = %s want layoutA", publishedRow.LayoutTemplateID.String)
	}
	doc, _ := document.Decode([]byte(publishedRow.DocumentJson))
	effective, err := ResolveEffectiveDocument(ctx, queries, doc, "page", publishedRow.LayoutTemplateID)
	if err != nil {
		t.Fatal(err)
	}
	// Effective should contain LayoutA heading
	foundA := false
	var walk func([]document.Node)
	walk = func(nodes []document.Node) {
		for _, n := range nodes {
			if n.Props != nil {
				var p map[string]any
				_ = json.Unmarshal(n.Props, &p)
				if p["text"] == "LayoutA" {
					foundA = true
				}
			}
			walk(n.Children)
		}
	}
	walk(effective.Nodes)
	if !foundA {
		t.Fatal("public render should use layoutA")
	}
	// Now publish rev2
	if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: "rev2", Valid: true}, PublishedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now, ID: entryID}); err != nil {
		t.Fatal(err)
	}
	publishedRow2, _ := queries.GetPublishedEntryByID(ctx, entryID)
	if publishedRow2.LayoutTemplateID.String != "layoutB" {
		t.Fatalf("after publish layout = %s want layoutB", publishedRow2.LayoutTemplateID.String)
	}
	doc2, _ := document.Decode([]byte(publishedRow2.DocumentJson))
	effective2, err := ResolveEffectiveDocument(ctx, queries, doc2, "page", publishedRow2.LayoutTemplateID)
	if err != nil {
		t.Fatal(err)
	}
	foundB := false
	walk = func(nodes []document.Node) {
		for _, n := range nodes {
			var p map[string]any
			_ = json.Unmarshal(n.Props, &p)
			if p["text"] == "LayoutB" {
				foundB = true
			}
			walk(n.Children)
		}
	}
	walk(effective2.Nodes)
	if !foundB {
		t.Fatal("after publish should use layoutB")
	}
}

func TestTemplatePublishPropagation(t *testing.T) {
	_, queries, _ := openTestDBWithRegistry(t)
	ctx := context.Background()
	// Initial layout with Old heading
	docOld := `{"version":1,"nodes":[{"id":"hOld","block":"core/heading","version":1,"props":{"text":"Old","level":1},"settings":{}},{"id":"slot","block":"core/content-slot","version":1,"props":{},"settings":{}}]}`
	docNew := `{"version":1,"nodes":[{"id":"hNew","block":"core/heading","version":1,"props":{"text":"New","level":1},"settings":{}},{"id":"slot","block":"core/content-slot","version":1,"props":{},"settings":{}}]}`
	createLayoutTemplate(t, queries, "svc", "Service", "page", docOld, true)

	// Create two entries using svc
	now := time.Now().Unix()
	for _, eid := range []string{"eA", "eB"} {
		if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: eid, ContentTypeID: "page", Slug: "slug-" + eid, Status: "active", CreatedAt: now, UpdatedAt: now}); err != nil {
			t.Fatal(err)
		}
		entryDoc := `{"version":1,"nodes":[{"id":"t` + eid + `","block":"core/text","version":1,"props":{"text":"body"},"settings":{}}]}`
		if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: eid + "-r1", EntryID: eid, RevisionNumber: 1, Title: "T", DocumentJson: entryDoc, LayoutTemplateID: sql.NullString{String: "svc", Valid: true}, CreatedAt: now}); err != nil {
			t.Fatal(err)
		}
		if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: eid + "-r1", Valid: true}, PublishedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now, ID: eid}); err != nil {
			t.Fatal(err)
		}
	}
	// Verify both render Old
	for _, eid := range []string{"eA", "eB"} {
		row, _ := queries.GetPublishedEntryByID(ctx, eid)
		doc, _ := document.Decode([]byte(row.DocumentJson))
		eff, _ := ResolveEffectiveDocument(ctx, queries, doc, "page", row.LayoutTemplateID)
		var hasOld bool
		var walk func([]document.Node)
		walk = func(ns []document.Node) {
			for _, n := range ns {
				var p map[string]any
				_ = json.Unmarshal(n.Props, &p)
				if p["text"] == "Old" {
					hasOld = true
				}
				walk(n.Children)
			}
		}
		walk(eff.Nodes)
		if !hasOld {
			t.Fatalf("%s should contain Old", eid)
		}
	}
	// Save draft r2 not published
	now2 := now + 10
	if err := queries.CreateLayoutTemplateRevision(ctx, db.CreateLayoutTemplateRevisionParams{ID: "svc-r2", TemplateID: "svc", RevisionNumber: 2, DocumentJson: docNew, CreatedAt: now2}); err != nil {
		t.Fatal(err)
	}
	// Still Old
	for _, eid := range []string{"eA", "eB"} {
		row, _ := queries.GetPublishedEntryByID(ctx, eid)
		doc, _ := document.Decode([]byte(row.DocumentJson))
		eff, _ := ResolveEffectiveDocument(ctx, queries, doc, "page", row.LayoutTemplateID)
		var hasOld bool
		var walk func([]document.Node)
		walk = func(ns []document.Node) {
			for _, n := range ns {
				var p map[string]any
				_ = json.Unmarshal(n.Props, &p)
				if p["text"] == "Old" {
					hasOld = true
				}
				walk(n.Children)
			}
		}
		walk(eff.Nodes)
		if !hasOld {
			t.Fatalf("%s should still Old before publish", eid)
		}
	}
	// Publish r2
	if err := queries.CreateLayoutTemplateRevision(ctx, db.CreateLayoutTemplateRevisionParams{ID: "svc-r3", TemplateID: "svc", RevisionNumber: 3, DocumentJson: docNew, CreatedAt: now2 + 1}); err != nil {
		// r2 already exists, so r3 is new; but we already created r2 as draft, now publish r3
		t.Fatal(err)
	}
	if err := queries.SetLayoutTemplatePublishedRevision(ctx, db.SetLayoutTemplatePublishedRevisionParams{PublishedRevisionID: sql.NullString{String: "svc-r3", Valid: true}, UpdatedAt: now2, ID: "svc"}); err != nil {
		t.Fatal(err)
	}
	// Now both should render New, without new entry revisions
	for _, eid := range []string{"eA", "eB"} {
		row, _ := queries.GetPublishedEntryByID(ctx, eid)
		doc, _ := document.Decode([]byte(row.DocumentJson))
		eff, _ := ResolveEffectiveDocument(ctx, queries, doc, "page", row.LayoutTemplateID)
		var hasNew bool
		var walk func([]document.Node)
		walk = func(ns []document.Node) {
			for _, n := range ns {
				var p map[string]any
				_ = json.Unmarshal(n.Props, &p)
				if p["text"] == "New" {
					hasNew = true
				}
				walk(n.Children)
			}
		}
		walk(eff.Nodes)
		if !hasNew {
			t.Fatalf("%s should contain New after publish", eid)
		}
		revs, _ := queries.ListEntryRevisions(ctx, eid)
		if len(revs) != 1 {
			t.Fatalf("%s should still have 1 revision, got %d", eid, len(revs))
		}
	}
}

func TestDefaultTemplateSemantics(t *testing.T) {
	_, queries, _ := openTestDBWithRegistry(t)
	ctx := context.Background()
	doc := `{"version":1,"nodes":[{"id":"slot","block":"core/content-slot","version":1,"props":{},"settings":{}}]}`
	createLayoutTemplate(t, queries, "tmplA", "A", "page", doc, true)
	createLayoutTemplate(t, queries, "tmplB", "B", "page", doc, true)
	now := time.Now().Unix()
	if err := queries.SetContentTypeDefaultLayoutTemplate(ctx, db.SetContentTypeDefaultLayoutTemplateParams{DefaultLayoutTemplateID: sql.NullString{String: "tmplA", Valid: true}, UpdatedAt: now, ID: "page"}); err != nil {
		t.Fatal(err)
	}
	// Simulate creating Page1: should get A
	ct, _ := queries.GetContentType(ctx, "page")
	if ct.DefaultLayoutTemplateID.String != "tmplA" {
		t.Fatal("default A")
	}
	// Create entry like writeEntry would: use default
	entryID1 := "page1"
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: entryID1, ContentTypeID: "page", Slug: "page-1", Status: "active", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "page1-r1", EntryID: entryID1, RevisionNumber: 1, Title: "P1", DocumentJson: doc, LayoutTemplateID: ct.DefaultLayoutTemplateID, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	// Change default to B
	if err := queries.SetContentTypeDefaultLayoutTemplate(ctx, db.SetContentTypeDefaultLayoutTemplateParams{DefaultLayoutTemplateID: sql.NullString{String: "tmplB", Valid: true}, UpdatedAt: now, ID: "page"}); err != nil {
		t.Fatal(err)
	}
	// Existing Page1 remains A
	rev1, _ := queries.GetLatestEntryRevision(ctx, entryID1)
	if rev1.LayoutTemplateID.String != "tmplA" {
		t.Fatalf("page1 should remain A, got %s", rev1.LayoutTemplateID.String)
	}
	// New Page2 should get B
	ct2, _ := queries.GetContentType(ctx, "page")
	entryID2 := "page2"
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: entryID2, ContentTypeID: "page", Slug: "page-2", Status: "active", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "page2-r1", EntryID: entryID2, RevisionNumber: 1, Title: "P2", DocumentJson: doc, LayoutTemplateID: ct2.DefaultLayoutTemplateID, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	rev2, _ := queries.GetLatestEntryRevision(ctx, entryID2)
	if rev2.LayoutTemplateID.String != "tmplB" {
		t.Fatalf("page2 should be B, got %s", rev2.LayoutTemplateID.String)
	}
}

func TestBackwardCompatibility_NullTemplate(t *testing.T) {
	_, queries, _ := openTestDBWithRegistry(t)
	ctx := context.Background()
	now := time.Now().Unix()
	entryID := "eNull"
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: entryID, ContentTypeID: "page", Slug: "null-page", Status: "active", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	docJSON := `{"version":1,"nodes":[{"id":"t1","block":"core/text","version":1,"props":{"text":"direct"},"settings":{}}]}`
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "rNull", EntryID: entryID, RevisionNumber: 1, Title: "T", DocumentJson: docJSON, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: "rNull", Valid: true}, PublishedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now, ID: entryID}); err != nil {
		t.Fatal(err)
	}
	row, _ := queries.GetPublishedEntryByID(ctx, entryID)
	if row.LayoutTemplateID.Valid {
		t.Fatal("should be null")
	}
	doc, _ := document.Decode([]byte(row.DocumentJson))
	eff, err := ResolveEffectiveDocument(ctx, queries, doc, "page", row.LayoutTemplateID)
	if err != nil {
		t.Fatal(err)
	}
	if len(eff.Nodes) != 1 || eff.Nodes[0].ID != "t1" {
		t.Fatalf("direct render failed %+v", eff)
	}
}

func TestContentTypeMismatch(t *testing.T) {
	_, queries, reg := openTestDBWithRegistry(t)
	// Layout for post
	doc := `{"version":1,"nodes":[{"id":"slot","block":"core/content-slot","version":1,"props":{},"settings":{}}]}`
	createLayoutTemplate(t, queries, "postTmpl", "PostTmpl", "post", doc, true)
	// Try to use it for page -> should be rejected via Validate in writeEntry path or resolver
	ctx := context.Background()
	// Simulate writeEntry validation: attempt to create page revision with post template
	now := time.Now().Unix()
	entryID := "mismatchPage"
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: entryID, ContentTypeID: "page", Slug: "mismatch", Status: "active", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	// Attempt resolver should error
	entryDoc := `{"version":1,"nodes":[{"id":"t","block":"core/text","version":1,"props":{"text":"hi"},"settings":{}}]}`
	d, _ := document.Decode([]byte(entryDoc))
	if err := ValidateEntryDocument(reg, d); err != nil {
		t.Fatal(err)
	}
	_, err := ResolveEffectiveDocument(ctx, queries, d, "page", sql.NullString{String: "postTmpl", Valid: true})
	if err == nil {
		t.Fatal("expected content type mismatch error")
	}
	if !contains(err.Error(), "belongs to") {
		t.Fatalf("unexpected error %v", err)
	}
}

func TestUnpublishedTemplateNotUsable(t *testing.T) {
	_, queries, _ := openTestDBWithRegistry(t)
	ctx := context.Background()
	doc := `{"version":1,"nodes":[{"id":"slot","block":"core/content-slot","version":1,"props":{},"settings":{}}]}`
	createLayoutTemplate(t, queries, "unpub", "Unpub", "page", doc, false)
	d := mustDocLifecycle(`{"version":1,"nodes":[{"id":"t","block":"core/text","version":1,"props":{"text":"hi"},"settings":{}}]}`)
	_, err := ResolveEffectiveDocument(ctx, queries, d, "page", sql.NullString{String: "unpub", Valid: true})
	if err == nil {
		t.Fatal("expected unpublished error")
	}
}

func mustDocLifecycle(s string) *document.Document {
	var d document.Document
	_ = json.Unmarshal([]byte(s), &d)
	return &d
}

func TestLCPViaComposedDocument(t *testing.T) {
	_, _, reg := openTestDBWithRegistry(t)
	// Layout with featured-image + slot
	layoutJSON := `{"version":1,"nodes":[{"id":"feat","block":"core/featured-image","version":1,"props":{},"settings":{"objectFit":"cover","aspectRatio":"16:9","align":"center"}},{"id":"slot","block":"core/content-slot","version":1,"props":{},"settings":{}}]}`
	entryJSON := `{"version":1,"nodes":[{"id":"t1","block":"core/text","version":1,"props":{"text":"body"},"settings":{}}]}`
	layoutDoc, _ := document.Decode([]byte(layoutJSON))
	entryDoc, _ := document.Decode([]byte(entryJSON))
	composed, err := Compose(layoutDoc, entryDoc)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := reg.Prepare(composed)
	if err != nil {
		t.Fatal(err)
	}
	// Prepared should have featured-image as LCP candidate
	found := false
	for _, c := range prepared.AutoCandidates {
		if c.ID == "feat" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected feat as auto candidate, got %+v", prepared.AutoCandidates)
	}
	// If we had prepared only entry doc, feat would be missing
	entryPrepared, _ := reg.Prepare(entryDoc)
	for _, c := range entryPrepared.AutoCandidates {
		if c.ID == "feat" {
			t.Fatal("entry alone should not have feat")
		}
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (func() bool {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})()
}
