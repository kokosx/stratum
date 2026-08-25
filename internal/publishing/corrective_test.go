package publishing

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/kokosx/stratum/internal/content"
	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func newPublishingHarness(t *testing.T) (*Service, *Scheduler, *storage.Database, *db.Queries) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	database, err := storage.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := database.Migrate(ctx); err != nil {
		t.Fatalf("Migrate: %v", err)
	}
	queries := db.New(database.DB)
	svc := New(database.DB, queries)
	sched := NewScheduler(database.DB, queries)
	return svc, sched, database, queries
}

func createEntryWithRevision(t *testing.T, queries *db.Queries, id, ct, slug, visibility string, sticky int64, reviewState string) string {
	t.Helper()
	ctx := context.Background()
	now := time.Now().Unix()
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: id, ContentTypeID: ct, Slug: slug, Status: "active", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("CreateEntry %s: %v", id, err)
	}
	revID := id + "-r1"
	doc := `{"version":1,"nodes":[]}`
	params := db.CreateEntryRevisionParams{ID: revID, EntryID: id, RevisionNumber: 1, Slug: slug, Title: "Title " + id, DocumentJson: doc, CreatedAt: now, Visibility: visibility, Sticky: sticky, ReviewState: reviewState}
	if visibility == "password" {
		// need hash
		hash, _ := HashPassword("secret")
		params.PasswordHash = sql.NullString{String: hash, Valid: true}
	}
	if err := queries.CreateEntryRevision(ctx, params); err != nil {
		t.Fatalf("CreateRevision %s: %v", id, err)
	}
	return revID
}

func publishEntry(t *testing.T, svc *Service, entryID, revID string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().Unix()
	if err := svc.PublishRevision(ctx, entryID, revID, now); err != nil {
		t.Fatalf("PublishRevision %s %s: %v", entryID, revID, err)
	}
}

// TestUnpublishValidatesDescendantsBeforeMutation ensures Unpublish does not clear published revision if descendants exist.
func TestUnpublishValidatesDescendantsBeforeMutation(t *testing.T) {
	svc, _, database, queries := newPublishingHarness(t)
	defer database.Close()
	ctx := context.Background()
	// Create parent and child hierarchical pages (page type is hierarchical)
	parentRev := createEntryWithRevision(t, queries, "parent", "page", "company", "public", 0, "draft")
	_ = createEntryWithRevision(t, queries, "child", "page", "team", "public", 0, "draft")
	// Need to set parent for child revision
	now := time.Now().Unix()
	// Update child revision to have parent
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "child-r2", EntryID: "child", RevisionNumber: 2, Slug: "team", Title: "Team", DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now, ParentEntryID: sql.NullString{String: "parent", Valid: true}}); err != nil {
		t.Fatalf("child r2: %v", err)
	}
	publishEntry(t, svc, "parent", parentRev)
	publishEntry(t, svc, "child", "child-r2")
	// Ensure child is published at /company/team via hierarchy sync (publish will create route)
	// Now try to unpublish parent – should be rejected with ErrPublishedDescendants and not mutate.
	err := svc.Unpublish(ctx, "parent", time.Now().Unix())
	if err != content.ErrPublishedDescendants {
		t.Fatalf("expected ErrPublishedDescendants, got %v", err)
	}
	// Verify parent still published and route still exists
	entry, _ := queries.GetEntry(ctx, "parent")
	if !entry.PublishedRevisionID.Valid {
		t.Fatalf("parent should still be published after failed unpublish")
	}
	if _, err := queries.GetEntryRoute(ctx, sql.NullString{String: "parent", Valid: true}); err != nil {
		t.Fatalf("parent route should still exist after failed unpublish")
	}
}

// TestUnpublishProtectedPage ensures homepage/posts page cannot be unpublished.
func TestUnpublishProtectedPage(t *testing.T) {
	svc, _, database, queries := newPublishingHarness(t)
	defer database.Close()
	ctx := context.Background()
	rev := createEntryWithRevision(t, queries, "home", "page", "home", "public", 0, "draft")
	publishEntry(t, svc, "home", rev)
	// Set as homepage
	now := time.Now().Unix()
	if err := queries.UpdateReadingSettings(ctx, db.UpdateReadingSettingsParams{HomepageMode: "page", HomepageEntryID: sql.NullString{String: "home", Valid: true}, PostsPageEntryID: sql.NullString{}, PostsPerPage: 10, PostsBasePath: "/blog", UpdatedAt: now}); err != nil {
		t.Fatalf("update reading: %v", err)
	}
	err := svc.Unpublish(ctx, "home", time.Now().Unix())
	if err != content.ErrProtectedPage {
		t.Fatalf("expected ErrProtectedPage for homepage unpublish, got %v", err)
	}
	// Test posts page
	rev2 := createEntryWithRevision(t, queries, "blog", "page", "blog", "public", 0, "draft")
	publishEntry(t, svc, "blog", rev2)
	if err := queries.UpdateReadingSettings(ctx, db.UpdateReadingSettingsParams{HomepageMode: "latest_posts", HomepageEntryID: sql.NullString{}, PostsPageEntryID: sql.NullString{String: "blog", Valid: true}, PostsPerPage: 10, PostsBasePath: "/blog", UpdatedAt: now}); err != nil {
		t.Fatalf("update reading2: %v", err)
	}
	err = svc.Unpublish(ctx, "blog", time.Now().Unix())
	if err != content.ErrProtectedPage {
		t.Fatalf("expected ErrProtectedPage for posts page, got %v", err)
	}
}

// TestPublishPrivateHierarchicalWithDescendantsRejects ensures private publish is rejected if descendants exist.
func TestPublishPrivateHierarchicalWithDescendantsRejects(t *testing.T) {
	svc, _, database, queries := newPublishingHarness(t)
	defer database.Close()
	ctx := context.Background()
	parentRev := createEntryWithRevision(t, queries, "pParent", "page", "parent-priv", "public", 0, "draft")
	_ = createEntryWithRevision(t, queries, "pChild", "page", "child-priv", "public", 0, "draft")
	now := time.Now().Unix()
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "pChild-r2", EntryID: "pChild", RevisionNumber: 2, Slug: "child-priv", Title: "Child", DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now, ParentEntryID: sql.NullString{String: "pParent", Valid: true}}); err != nil {
		t.Fatalf("child r2: %v", err)
	}
	publishEntry(t, svc, "pParent", parentRev)
	publishEntry(t, svc, "pChild", "pChild-r2")
	// Now try to publish parent as private – should be rejected
	now2 := time.Now().Unix()
	privRevID := "pParent-r2"
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: privRevID, EntryID: "pParent", RevisionNumber: 2, Slug: "parent-priv", Title: "Parent Priv", DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now2, Visibility: "private"}); err != nil {
		t.Fatalf("priv rev: %v", err)
	}
	err := svc.PublishRevision(ctx, "pParent", privRevID, now2)
	if err != content.ErrPublishedDescendants {
		t.Fatalf("expected ErrPublishedDescendants for private parent, got %v", err)
	}
	// Verify still public
	entry, _ := queries.GetEntry(ctx, "pParent")
	rev, _ := queries.GetEntryRevision(ctx, entry.PublishedRevisionID.String)
	if rev.Visibility != "public" {
		t.Fatalf("parent should still be public, got %s", rev.Visibility)
	}
}

// TestPublishPrivateAndPasswordForSpecialPageRejects ensures homepage cannot be private/password.
func TestPublishPrivateAndPasswordForSpecialPageRejects(t *testing.T) {
	svc, _, database, queries := newPublishingHarness(t)
	defer database.Close()
	ctx := context.Background()
	rev := createEntryWithRevision(t, queries, "special", "page", "special-home", "public", 0, "draft")
	publishEntry(t, svc, "special", rev)
	now := time.Now().Unix()
	if err := queries.UpdateReadingSettings(ctx, db.UpdateReadingSettingsParams{HomepageMode: "page", HomepageEntryID: sql.NullString{String: "special", Valid: true}, PostsPageEntryID: sql.NullString{}, PostsPerPage: 10, PostsBasePath: "/blog", UpdatedAt: now}); err != nil {
		t.Fatalf("update: %v", err)
	}
	// Try private
	privID := "special-r2"
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: privID, EntryID: "special", RevisionNumber: 2, Slug: "special-home", Title: "Special", DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now + 1, Visibility: "private"}); err != nil {
		t.Fatalf("priv: %v", err)
	}
	if err := svc.PublishRevision(ctx, "special", privID, now+1); err != content.ErrProtectedPage {
		t.Fatalf("expected ErrProtectedPage for private homepage, got %v", err)
	}
	// Try password
	hash, _ := HashPassword("secret")
	pwdID := "special-r3"
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: pwdID, EntryID: "special", RevisionNumber: 3, Slug: "special-home", Title: "Special", DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now + 2, Visibility: "password", PasswordHash: sql.NullString{String: hash, Valid: true}}); err != nil {
		t.Fatalf("pwd: %v", err)
	}
	if err := svc.PublishRevision(ctx, "special", pwdID, now+2); err != content.ErrProtectedPage {
		t.Fatalf("expected ErrProtectedPage for password homepage, got %v", err)
	}
}

// TestPublishCreatesCorrectVisibility ensures public/private/password routes behave.
func TestPublishCreatesCorrectVisibility(t *testing.T) {
	svc, _, database, queries := newPublishingHarness(t)
	defer database.Close()
	ctx := context.Background()
	// Public
	pubRev := createEntryWithRevision(t, queries, "pubTest", "page", "pub-test", "public", 0, "draft")
	publishEntry(t, svc, "pubTest", pubRev)
	if _, err := queries.GetEntryRoute(ctx, sql.NullString{String: "pubTest", Valid: true}); err != nil {
		t.Fatalf("public should have route")
	}
	entry, _ := queries.GetEntry(ctx, "pubTest")
	rev, _ := queries.GetEntryRevision(ctx, entry.PublishedRevisionID.String)
	if rev.Visibility != "public" {
		t.Fatalf("public visibility")
	}
	// Private
	privRev := createEntryWithRevision(t, queries, "privTest", "page", "priv-test", "public", 0, "draft")
	publishEntry(t, svc, "privTest", privRev)
	// Now publish private revision
	now := time.Now().Unix()
	privID := "privTest-r2"
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: privID, EntryID: "privTest", RevisionNumber: 2, Slug: "priv-test", Title: "Priv", DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now, Visibility: "private"}); err != nil {
		t.Fatalf("priv r2: %v", err)
	}
	if err := svc.PublishRevision(ctx, "privTest", privID, now); err != nil {
		t.Fatalf("publish private: %v", err)
	}
	if _, err := queries.GetEntryRoute(ctx, sql.NullString{String: "privTest", Valid: true}); err == nil {
		t.Fatalf("private should have no route")
	}
	entry2, _ := queries.GetEntry(ctx, "privTest")
	rev2, _ := queries.GetEntryRevision(ctx, entry2.PublishedRevisionID.String)
	if rev2.Visibility != "private" {
		t.Fatalf("private visibility not set")
	}
	// Password
	pwdRev := createEntryWithRevision(t, queries, "pwdTest", "post", "pwd-test", "public", 0, "draft")
	publishEntry(t, svc, "pwdTest", pwdRev)
	now2 := time.Now().Unix()
	hash, _ := HashPassword("secret")
	pwdID := "pwdTest-r2"
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: pwdID, EntryID: "pwdTest", RevisionNumber: 2, Slug: "pwd-test", Title: "Pwd", DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now2, Visibility: "password", PasswordHash: sql.NullString{String: hash, Valid: true}}); err != nil {
		t.Fatalf("pwd r2: %v", err)
	}
	if err := svc.PublishRevision(ctx, "pwdTest", pwdID, now2); err != nil {
		t.Fatalf("publish password: %v", err)
	}
	if _, err := queries.GetEntryRoute(ctx, sql.NullString{String: "pwdTest", Valid: true}); err != nil {
		t.Fatalf("password should have route")
	}
}

// TestUnpublishClearsSchedule verifies unpublish cancels schedule atomically.
func TestUnpublishClearsSchedule(t *testing.T) {
	svc, _, database, queries := newPublishingHarness(t)
	defer database.Close()
	ctx := context.Background()
	rev := createEntryWithRevision(t, queries, "schedParent", "post", "sched-parent", "public", 0, "draft")
	publishEntry(t, svc, "schedParent", rev)
	now := time.Now().Unix()
	// Create scheduled revision
	schedRevID := "schedParent-r2"
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: schedRevID, EntryID: "schedParent", RevisionNumber: 2, Slug: "sched-parent", Title: "Sched", DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now + 10}); err != nil {
		t.Fatalf("sched rev: %v", err)
	}
	if err := svc.Schedule(ctx, "schedParent", schedRevID, now+1000, "", now); err != nil {
		t.Fatalf("schedule: %v", err)
	}
	if _, err := queries.GetActivePublicationJobByEntry(ctx, "schedParent"); err != nil {
		t.Fatalf("should have schedule")
	}
	// Unpublish should cancel schedule
	if err := svc.Unpublish(ctx, "schedParent", now+5); err != nil {
		t.Fatalf("unpublish: %v", err)
	}
	if _, err := queries.GetActivePublicationJobByEntry(ctx, "schedParent"); err == nil {
		t.Fatalf("schedule should be cancelled after unpublish")
	}
	entry, _ := queries.GetEntry(ctx, "schedParent")
	if entry.PublishedRevisionID.Valid {
		t.Fatalf("should be unpublished")
	}
}

// TestUnlockStoreRNGError ensures Create returns error on CSPRNG failure – we test normal path succeeds.
func TestUnlockStoreCreatePropagates(t *testing.T) {
	store := NewUnlockStore()
	now := time.Now().Unix()
	token, expires, err := store.Create("e1", "r1", now)
	if err != nil {
		t.Fatalf("Create should succeed: %v", err)
	}
	if token == "" || expires <= now {
		t.Fatalf("invalid token/expiry")
	}
	if !store.Valid(token, "e1", "r1", now+10) {
		t.Fatalf("token should be valid")
	}
	// Old revision should not be valid after new publish
	if store.Valid(token, "e1", "r2", now+10) {
		t.Fatalf("old token should not unlock new revision")
	}
}
