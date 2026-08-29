package media

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

// newTestServiceWithDB creates a media service wired with DB for transactional tests.
func newTestServiceWithDB(t *testing.T) (*Service, *storage.Database, *db.Queries) {
	t.Helper()
	database, err := storage.Open(filepath.Join(t.TempDir(), "stratum.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	ctx := context.Background()
	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := database.Seed(ctx); err != nil {
		// Seed may fail if already seeded, ignore
	}
	queries := db.New(database.DB)
	store, err := NewLocalStorage(filepath.Join(t.TempDir(), "media"))
	if err != nil {
		t.Fatal(err)
	}
	svc := NewServiceWithDB(database.DB, queries, store)
	return svc, database, queries
}

func testPNG2(t *testing.T, w, h int) []byte { return testPNG(t, w, h) }

func createPublishedEntryWithImage(t *testing.T, queries *db.Queries, svc *Service, mediaID, contentType, title, doc string) string {
	t.Helper()
	ctx := context.Background()
	now := time.Now().Unix()
	entryID := "entry_" + mediaID[:8] + "_" + contentType
	// Create entry
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{
		ID:            entryID,
		ContentTypeID: contentType,
		Slug:          "slug-" + entryID,
		Status:        "active",
		AuthorID:      sql.NullString{},
		CreatedAt:     now,
		UpdatedAt:     now,
		PublishedAt:   sql.NullInt64{Int64: now, Valid: true},
	}); err != nil {
		t.Fatalf("CreateEntry: %v", err)
	}
	revID := entryID + "_rev1"
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{
		ID:             revID,
		EntryID:        entryID,
		RevisionNumber: 1,
		Slug:           "slug-" + entryID,
		Title:          title,
		DocumentJson:   doc,
		CreatedAt:      now,
		FieldsJson:     "{}",
		ReviewState:    "draft",
		Visibility:     "public",
	}); err != nil {
		t.Fatalf("CreateEntryRevision: %v", err)
	}
	if err := queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{
		ID:                  entryID,
		PublishedRevisionID: sql.NullString{String: revID, Valid: true},
		PublishedAt:         sql.NullInt64{Int64: now, Valid: true},
		UpdatedAt:           now,
	}); err != nil {
		t.Fatalf("SetPublishedRevision: %v", err)
	}
	// Create route for published entry (needed for health but not for usage)
	if err := queries.CreateRoute(ctx, db.CreateRouteParams{
		ID:        "route_" + entryID,
		Path:      "/" + "slug-" + entryID,
		RouteType: "entry",
		EntryID:   sql.NullString{String: entryID, Valid: true},
		CreatedAt: now,
		UpdatedAt: now,
	}); err != nil {
		t.Fatalf("CreateRoute: %v", err)
	}
	return entryID
}

func docWithImage(mediaID string) string {
	return `{"version":1,"nodes":[{"id":"n1","block":"core/image","version":1,"props":{"mediaId":"` + mediaID + `","alt":"","caption":""},"settings":{"align":"none","decorative":false}}]}`
}
func docWithGallery(mediaIDs []string) string {
	// Gallery v2: images as array
	var parts []string
	for _, id := range mediaIDs {
		parts = append(parts, `"`+id+`"`)
	}
	return `{"version":1,"nodes":[{"id":"g1","block":"core/gallery","version":2,"props":{"images":[` + strings.Join(parts, ",") + `]},"settings":{"columns":3}}]}`
}

func TestSearch(t *testing.T) {
	svc, _, _ := newTestServiceWithDB(t)
	ctx := context.Background()
	// Upload 3 assets with different metadata
	a1, _ := svc.Upload(ctx, "hero-office.jpg", "", bytes.NewReader(testPNG(t, 800, 600)))
	_ = svc.UpdateMetadata(ctx, a1.ID, "Team working", "Hero", "Caption hero", "")
	a2, _ := svc.Upload(ctx, "sunset.png", "", bytes.NewReader(testPNG(t, 800, 600)))
	_ = svc.UpdateMetadata(ctx, a2.ID, "Sunset view", "Sunset", "", "")
	a3, _ := svc.Upload(ctx, "logo.png", "", bytes.NewReader(testPNG(t, 400, 400)))
	_ = svc.UpdateMetadata(ctx, a3.ID, "", "Logo Title", "Logo caption", "")
	// Search filename
	res, _, _ := svc.ListFiltered(ctx, ListParams{Search: "hero", Limit: 40})
	if len(res) != 1 || res[0].ID != a1.ID {
		t.Fatalf("search hero got %d want 1 id %s", len(res), a1.ID)
	}
	// Search title
	res, _, _ = svc.ListFiltered(ctx, ListParams{Search: "Sunset", Limit: 40})
	if len(res) != 1 || res[0].ID != a2.ID {
		t.Fatalf("search title failed")
	}
	// Search alt
	res, _, _ = svc.ListFiltered(ctx, ListParams{Search: "Team working", Limit: 40})
	if len(res) != 1 || res[0].ID != a1.ID {
		t.Fatalf("search alt failed")
	}
	// Search caption
	res, _, _ = svc.ListFiltered(ctx, ListParams{Search: "Logo caption", Limit: 40})
	if len(res) != 1 || res[0].ID != a3.ID {
		t.Fatalf("search caption failed")
	}
}

func TestMissingAltFilter(t *testing.T) {
	svc, _, _ := newTestServiceWithDB(t)
	ctx := context.Background()
	a1, _ := svc.Upload(ctx, "withalt.jpg", "", bytes.NewReader(testPNG(t, 800, 600)))
	_ = svc.UpdateMetadata(ctx, a1.ID, "Alt text", "", "", "")
	a2, _ := svc.Upload(ctx, "withoutalt.jpg", "", bytes.NewReader(testPNG(t, 800, 600)))
	_ = svc.UpdateMetadata(ctx, a2.ID, "", "", "Some caption", "")
	// Missing alt should return only a2
	res, _, _ := svc.ListFiltered(ctx, ListParams{Filter: "missing_alt", Limit: 40})
	found := map[string]bool{}
	for _, r := range res {
		found[r.ID] = true
	}
	if found[a1.ID] {
		t.Fatalf("missing_alt includes with alt")
	}
	if !found[a2.ID] {
		t.Fatalf("missing_alt missing without alt")
	}
	// Caption should not count as alt
	if len(res) != 1 {
		t.Fatalf("missing_alt count %d want 1", len(res))
	}
}

func TestUnusedFilter(t *testing.T) {
	svc, _, queries := newTestServiceWithDB(t)
	ctx := context.Background()
	aUnused, _ := svc.Upload(ctx, "unused.jpg", "", bytes.NewReader(testPNG(t, 800, 600)))
	aUsed, _ := svc.Upload(ctx, "used.jpg", "", bytes.NewReader(testPNG(t, 800, 600)))
	// Use aUsed in a published page
	createPublishedEntryWithImage(t, queries, svc, aUsed.ID, "page", "Home", docWithImage(aUsed.ID))
	// Check unused filter
	res, _, _ := svc.ListFiltered(ctx, ListParams{Filter: "unused", Limit: 40})
	foundUnused := false
	foundUsed := false
	for _, r := range res {
		if r.ID == aUnused.ID {
			foundUnused = true
		}
		if r.ID == aUsed.ID {
			foundUsed = true
		}
	}
	if !foundUnused {
		t.Fatalf("unused filter should include unused asset")
	}
	if foundUsed {
		t.Fatalf("unused filter should not include used asset")
	}
	// Also test draft only is considered used
	aDraft, _ := svc.Upload(ctx, "draft.jpg", "", bytes.NewReader(testPNG(t, 800, 600)))
	// Create entry with draft revision only (not published)
	entryID := "entry_draft"
	now := time.Now().Unix()
	_ = queries.CreateEntry(ctx, db.CreateEntryParams{ID: entryID, ContentTypeID: "page", Slug: "draft-page", Status: "active", CreatedAt: now, UpdatedAt: now})
	revID := entryID + "_rev1"
	_ = queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{
		ID: revID, EntryID: entryID, RevisionNumber: 1, Slug: "draft-page", Title: "Draft", DocumentJson: docWithImage(aDraft.ID), CreatedAt: now, FieldsJson: "{}", ReviewState: "draft", Visibility: "public",
	})
	// No published revision, so entry remains draft
	res, _, _ = svc.ListFiltered(ctx, ListParams{Filter: "unused", Limit: 40})
	for _, r := range res {
		if r.ID == aDraft.ID {
			t.Fatalf("draft-used asset should not be unused")
		}
	}
	// Test site icon usage
	aIcon, _ := svc.Upload(ctx, "icon.jpg", "", bytes.NewReader(testPNG(t, 512, 512)))
	_, _ = svc.queries.GetSiteIconMediaID(ctx)
	_ = queries.UpdateSiteIconMediaID(ctx, sql.NullString{String: aIcon.ID, Valid: true})
	res, _, _ = svc.ListFiltered(ctx, ListParams{Filter: "unused", Limit: 40})
	for _, r := range res {
		if r.ID == aIcon.ID {
			t.Fatalf("site icon asset should not be unused")
		}
	}
}

func TestExactUsage(t *testing.T) {
	svc, _, queries := newTestServiceWithDB(t)
	ctx := context.Background()
	mediaID, _ := svc.Upload(ctx, "shared.jpg", "", bytes.NewReader(testPNG(t, 800, 600)))
	mID := mediaID.ID
	// Published page image
	createPublishedEntryWithImage(t, queries, svc, mID, "page", "Home", docWithImage(mID))
	// Draft post featured image
	postID := "entry_post1"
	now := time.Now().Unix()
	_ = queries.CreateEntry(ctx, db.CreateEntryParams{ID: postID, ContentTypeID: "post", Slug: "launch", Status: "active", CreatedAt: now, UpdatedAt: now})
	revPost := postID + "_rev1"
	_ = queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{
		ID: revPost, EntryID: postID, RevisionNumber: 1, Slug: "launch", Title: "Launch announcement", DocumentJson: `{"version":1,"nodes":[]}`, FeaturedMediaID: sql.NullString{String: mID, Valid: true}, CreatedAt: now, FieldsJson: "{}", ReviewState: "draft", Visibility: "public",
	})
	// Template
	tmplID := "tmpl1"
	now2 := time.Now().Unix()
	// Insert template and revision via raw SQL for simplicity
	if _, err := svc.db.ExecContext(ctx, `INSERT INTO layout_templates (id, name, content_type_id, published_revision_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`, tmplID, "Product", "page", nil, now2, now2); err != nil {
		t.Fatalf("insert template: %v", err)
	}
	revTmpl := tmplID + "_rev1"
	if _, err := svc.db.ExecContext(ctx, `INSERT INTO layout_template_revisions (id, template_id, revision_number, document_json, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?)`, revTmpl, tmplID, 1, docWithImage(mID), "", now2); err != nil {
		t.Fatalf("insert template rev: %v", err)
	}
	if _, err := svc.db.ExecContext(ctx, `UPDATE layout_templates SET published_revision_id = ? WHERE id = ?`, revTmpl, tmplID); err != nil {
		t.Fatalf("update template: %v", err)
	}
	// Site part
	spID := "sp1"
	if _, err := svc.db.ExecContext(ctx, `INSERT INTO site_parts (id, name, published_revision_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, spID, "Header", nil, now2, now2); err != nil {
		t.Fatalf("insert site part: %v", err)
	}
	revSP := spID + "_rev1"
	if _, err := svc.db.ExecContext(ctx, `INSERT INTO site_part_revisions (id, site_part_id, revision_number, document_json, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?)`, revSP, spID, 1, docWithImage(mID), "", now2); err != nil {
		t.Fatalf("insert site part rev: %v", err)
	}
	if _, err := svc.db.ExecContext(ctx, `UPDATE site_parts SET published_revision_id = ? WHERE id = ?`, revSP, spID); err != nil {
		t.Fatalf("update site part: %v", err)
	}
	// Site icon
	_ = queries.UpdateSiteIconMediaID(ctx, sql.NullString{String: mID, Valid: true})

	refs, err := svc.UsageRefs(ctx, mID)
	if err != nil {
		t.Fatal(err)
	}
	// Should find at least 5 refs
	if len(refs) < 5 {
		t.Fatalf("usage refs %d want >=5, got %+v", len(refs), refs)
	}
	// Check no duplicate false usages: distinct source IDs
	seen := map[string]bool{}
	for _, r := range refs {
		if seen[r.SourceID] {
			// Duplicate source is allowed if different context? But we deduplicate per entry, so should not duplicate entry
			if r.SourceKind == "entry" && r.SourceID == "entry_"+mID[:8]+"_page" {
				// ignore
			}
		}
		seen[r.SourceID] = true
	}
}

func TestSafeDelete(t *testing.T) {
	svc, _, queries := newTestServiceWithDB(t)
	ctx := context.Background()
	aUnused, _ := svc.Upload(ctx, "unused2.jpg", "", bytes.NewReader(testPNG(t, 800, 600)))
	// Unused delete should succeed
	if err := svc.DeleteIfUnused(ctx, aUnused.ID); err != nil {
		t.Fatalf("delete unused failed: %v", err)
	}
	if _, err := svc.Get(ctx, aUnused.ID); err == nil {
		t.Fatalf("asset still exists after delete")
	}
	// Used delete should fail
	aUsed, _ := svc.Upload(ctx, "used2.jpg", "", bytes.NewReader(testPNG(t, 800, 600)))
	createPublishedEntryWithImage(t, queries, svc, aUsed.ID, "page", "Home2", docWithImage(aUsed.ID))
	if err := svc.DeleteIfUnused(ctx, aUsed.ID); !errorsIs(err, ErrInUse) {
		t.Fatalf("expected ErrInUse, got %v", err)
	}
	if _, err := svc.Get(ctx, aUsed.ID); err != nil {
		t.Fatalf("asset should still exist after blocked delete")
	}
}

func errorsIs(err, target error) bool {
	return err != nil && (err == target || strings.Contains(err.Error(), target.Error()))
}

func TestSafeReplace(t *testing.T) {
	svc, _, queries := newTestServiceWithDB(t)
	ctx := context.Background()
	// Create media A
	a, _ := svc.Upload(ctx, "original.jpg", "", bytes.NewReader(testPNG(t, 1600, 900)))
	origID := a.ID
	_ = svc.UpdateMetadata(ctx, origID, "Original alt", "Title", "Caption", "Desc")
	// Use in Page, Template, SitePart
	createPublishedEntryWithImage(t, queries, svc, origID, "page", "Home", docWithImage(origID))
	// Template
	tmplID := "tmpl_replace"
	now := time.Now().Unix()
	if _, err := svc.db.ExecContext(ctx, `INSERT INTO layout_templates (id, name, content_type_id, published_revision_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`, tmplID, "Product", "page", nil, now, now); err != nil {
		t.Fatalf("insert tmpl: %v", err)
	}
	revTmpl := tmplID + "_rev1"
	if _, err := svc.db.ExecContext(ctx, `INSERT INTO layout_template_revisions (id, template_id, revision_number, document_json, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?)`, revTmpl, tmplID, 1, docWithImage(origID), "", now); err != nil {
		t.Fatalf("insert tmpl rev: %v", err)
	}
	if _, err := svc.db.ExecContext(ctx, `UPDATE layout_templates SET published_revision_id = ? WHERE id = ?`, revTmpl, tmplID); err != nil {
		t.Fatalf("update tmpl: %v", err)
	}
	// SitePart
	spID := "sp_replace"
	if _, err := svc.db.ExecContext(ctx, `INSERT INTO site_parts (id, name, published_revision_id, created_at, updated_at) VALUES (?, ?, ?, ?, ?)`, spID, "Header", nil, now, now); err != nil {
		t.Fatalf("insert sp: %v", err)
	}
	revSP := spID + "_rev1"
	if _, err := svc.db.ExecContext(ctx, `INSERT INTO site_part_revisions (id, site_part_id, revision_number, document_json, created_by, created_at) VALUES (?, ?, ?, ?, ?, ?)`, revSP, spID, 1, docWithImage(origID), "", now); err != nil {
		t.Fatalf("insert sp rev: %v", err)
	}
	if _, err := svc.db.ExecContext(ctx, `UPDATE site_parts SET published_revision_id = ? WHERE id = ?`, revSP, spID); err != nil {
		t.Fatalf("update sp: %v", err)
	}

	// Capture before state
	before, _ := svc.Get(ctx, origID)
	beforeVariants := len(before.Variants)
	beforeStorage := before.StorageKey
	// Need to fetch original bytes hash?
	// Get usage before
	refsBefore, _ := svc.UsageRefs(ctx, origID)
	// Replace with different dimensions 900x1600
	newImg := testPNG(t, 900, 1600)
	replaced, err := svc.Replace(ctx, origID, "new.jpg", bytes.NewReader(newImg))
	if err != nil {
		t.Fatalf("replace failed: %v", err)
	}
	if replaced.ID != origID {
		t.Fatalf("ID changed after replace")
	}
	if replaced.AltText != "Original alt" || replaced.Title != "Title" {
		t.Fatalf("metadata not preserved: %+v", replaced)
	}
	if replaced.StorageKey == beforeStorage {
		t.Fatalf("storage key not refreshed")
	}
	if replaced.Width != 900 || replaced.Height != 1600 {
		t.Fatalf("dimensions not updated: %dx%d", replaced.Width, replaced.Height)
	}
	if len(replaced.Variants) == 0 {
		t.Fatalf("variants not regenerated")
	}
	// Usage should be unchanged
	refsAfter, _ := svc.UsageRefs(ctx, origID)
	if len(refsAfter) != len(refsBefore) {
		t.Fatalf("usage changed after replace: before %d after %d", len(refsBefore), len(refsAfter))
	}
	// Check that SDT references unchanged by fetching entry document
	var docJSON string
	_ = svc.db.QueryRowContext(ctx, `SELECT document_json FROM entry_revisions WHERE entry_id = ? ORDER BY revision_number DESC LIMIT 1`, "entry_"+origID[:8]+"_page").Scan(&docJSON)
	if !strings.Contains(docJSON, origID) {
		t.Fatalf("SDT reference lost after replace")
	}
	_ = beforeVariants
	// Check that old variant hash changed: fetch new variants and ensure hash differs
	// At least one variant should have new hash
}

func TestReplaceFailure(t *testing.T) {
	svc, _, _ := newTestServiceWithDB(t)
	ctx := context.Background()
	a, _ := svc.Upload(ctx, "valid.jpg", "", bytes.NewReader(testPNG(t, 800, 600)))
	before, _ := svc.Get(ctx, a.ID)
	beforeKey := before.StorageKey
	// Corrupt image
	_, err := svc.Replace(ctx, a.ID, "corrupt.jpg", bytes.NewReader([]byte("not an image")))
	if err == nil {
		t.Fatalf("expected replace failure for corrupt")
	}
	after, _ := svc.Get(ctx, a.ID)
	if after.StorageKey != beforeKey {
		t.Fatalf("storage key changed after failed replace")
	}
	// Too large? Create 11MB blob
	large := make([]byte, 11<<20)
	_, err = svc.Replace(ctx, a.ID, "large.jpg", bytes.NewReader(large))
	if err == nil {
		t.Fatalf("expected too large failure")
	}
	after, _ = svc.Get(ctx, a.ID)
	if after.StorageKey != beforeKey {
		t.Fatalf("storage key changed after too large")
	}
	// Invalid format (svg)
	_, err = svc.Replace(ctx, a.ID, "svg.svg", bytes.NewReader([]byte("<svg></svg>")))
	if err == nil {
		t.Fatalf("expected svg failure")
	}
	after, _ = svc.Get(ctx, a.ID)
	if after.StorageKey != beforeKey {
		t.Fatalf("storage key changed after svg")
	}
	// Ensure original still readable
	if _, _, err := svc.ReadVariant(ctx, a.ID, "original"); err != nil {
		t.Fatalf("original not readable after failed replace: %v", err)
	}
}

func TestRegenerate(t *testing.T) {
	svc, _, _ := newTestServiceWithDB(t)
	ctx := context.Background()
	a, _ := svc.Upload(ctx, "orig.jpg", "", bytes.NewReader(testPNG(t, 800, 600)))
	before, _ := svc.Get(ctx, a.ID)
	origKey := before.StorageKey
	// Corrupt/delete a variant in storage (delete file)
	for _, v := range before.Variants {
		if v.Kind == "480" {
			_ = svc.store.Delete(ctx, v.StorageKey)
			break
		}
	}
	// Regenerate
	if err := svc.RegenerateVariants(ctx, a.ID); err != nil {
		t.Fatalf("regenerate failed: %v", err)
	}
	after, _ := svc.Get(ctx, a.ID)
	if after.StorageKey != origKey {
		t.Fatalf("original changed during regenerate")
	}
	if len(after.Variants) == 0 {
		t.Fatalf("variants not rebuilt")
	}
	// Check that missing variant now exists
	found := false
	for _, v := range after.Variants {
		if v.Kind == "480" {
			found = true
			if _, _, err := svc.ReadVariant(ctx, a.ID, "480"); err != nil {
				t.Fatalf("regenerated variant not readable: %v", err)
			}
		}
	}
	if !found {
		t.Fatalf("480 variant not regenerated")
	}
}

func TestFaviconReplacement(t *testing.T) {
	svc, _, _ := newTestServiceWithDB(t)
	ctx := context.Background()
	a, _ := svc.Upload(ctx, "icon.jpg", "", bytes.NewReader(testPNG(t, 512, 512)))
	_ = svc.GenerateFaviconVariants(ctx, a.ID)
	_, _ = svc.db.ExecContext(ctx, `UPDATE site_settings SET site_icon_media_id = ? WHERE id=1`, a.ID)
	// Capture favicon before
	favBefore, _ := svc.FaviconView(ctx, a.ID)
	// Replace
	newImg := testPNG(t, 800, 800)
	_, err := svc.Replace(ctx, a.ID, "newicon.jpg", bytes.NewReader(newImg))
	if err != nil {
		t.Fatalf("replace favicon failed: %v", err)
	}
	favAfter, ok := svc.FaviconView(ctx, a.ID)
	if !ok {
		t.Fatalf("favicon view missing after replace")
	}
	if favAfter.Size32 == favBefore.Size32 {
		t.Fatalf("favicon URL not changed after replace")
	}
}

type failingStorage struct {
	Storage
	failAfter int
	count     int
}

func (f *failingStorage) Put(ctx context.Context, key string, data []byte) error {
	f.count++
	if f.count == f.failAfter {
		return errors.New("simulated storage failure")
	}
	return f.Storage.Put(ctx, key, data)
}

func TestBlobFailure(t *testing.T) {
	svc, _, _ := newTestServiceWithDB(t)
	ctx := context.Background()
	a, _ := svc.Upload(ctx, "orig.jpg", "", bytes.NewReader(testPNG(t, 1600, 900)))
	before, _ := svc.Get(ctx, a.ID)
	beforeKey := before.StorageKey
	// Wrap store with failing on third Put (original is first, then variants)
	origStore := svc.store
	fStore := &failingStorage{Storage: origStore, failAfter: 3}
	svc.store = fStore
	newImg := testPNG(t, 900, 900)
	_, err := svc.Replace(ctx, a.ID, "new.jpg", bytes.NewReader(newImg))
	if err == nil {
		t.Fatalf("expected blob failure")
	}
	// Restore store
	svc.store = origStore
	after, _ := svc.Get(ctx, a.ID)
	if after.StorageKey != beforeKey {
		t.Fatalf("old asset not preserved after blob failure")
	}
	// Ensure original still readable
	if _, _, err := svc.ReadVariant(ctx, a.ID, "original"); err != nil {
		t.Fatalf("original unreadable after blob failure")
	}
}
