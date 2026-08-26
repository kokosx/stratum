package comments

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"

	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func commentHarness(t *testing.T) (*Service, *db.Queries, string) {
	t.Helper()
	ctx := context.Background()
	database, err := storage.Open(filepath.Join(t.TempDir(), "comments.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	q := db.New(database.DB)
	if err := q.CreateEntry(ctx, db.CreateEntryParams{ID: "entry", ContentTypeID: "post", Slug: "post", Status: "active", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := q.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "rev", EntryID: "entry", RevisionNumber: 1, Title: "Post", DocumentJson: `{"version":1,"nodes":[]}`, Visibility: "public", CommentsEnabled: 1, CreatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := q.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{ID: "entry", PublishedRevisionID: sql.NullString{String: "rev", Valid: true}, PublishedAt: sql.NullInt64{Int64: 1, Valid: true}, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	return NewService(database.DB, q), q, "entry"
}

func submitComment(t *testing.T, s *Service, entry, parent string, now int64) db.Comment {
	t.Helper()
	c, err := s.Submit(context.Background(), entry, parent, "Alice", "alice@example.com", "", "Hello", "", "", "", "127.0.0.1:1234", true, now)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestSubmitAnonymousPendingAndApprovedOnlyListing(t *testing.T) {
	s, _, entry := commentHarness(t)
	pending := submitComment(t, s, entry, "", 1)
	if pending.Status != StatusPending {
		t.Fatalf("status=%s", pending.Status)
	}
	if got, err := s.ListApproved(context.Background(), entry); err != nil || len(got) != 0 {
		t.Fatalf("approved=%d err=%v", len(got), err)
	}
	if err := s.Approve(context.Background(), pending.ID, 2); err != nil {
		t.Fatal(err)
	}
	if got, err := s.CountApproved(context.Background(), entry); err != nil || got != 1 {
		t.Fatalf("count=%d err=%v", got, err)
	}
}

func TestSubmitThreadDepthOneTwoThreeAndFourRejected(t *testing.T) {
	s, _, entry := commentHarness(t)
	one := submitComment(t, s, entry, "", 1)
	two := submitComment(t, s, entry, one.ID, 2)
	three := submitComment(t, s, entry, two.ID, 3)
	if _, err := s.Submit(context.Background(), entry, three.ID, "Alice", "alice@example.com", "", "four", "", "", "", "127.0.0.1:9", true, 4); err != ErrDepthExceeded {
		t.Fatalf("err=%v", err)
	}
}

func TestModerationInvalidationAndDeleteParent(t *testing.T) {
	s, q, entry := commentHarness(t)
	var invalidated []string
	s.SetInvalidator(func(id string) { invalidated = append(invalidated, id) })
	parent := submitComment(t, s, entry, "", 1)
	child := submitComment(t, s, entry, parent.ID, 2)
	if err := s.Approve(context.Background(), parent.ID, 3); err != nil {
		t.Fatal(err)
	}
	if len(invalidated) != 1 {
		t.Fatalf("invalidations=%v", invalidated)
	}
	if err := s.Delete(context.Background(), parent.ID); err != nil {
		t.Fatal(err)
	}
	got, err := q.GetComment(context.Background(), child.ID)
	if err != nil || got.ParentID.Valid {
		t.Fatalf("child parent=%v err=%v", got.ParentID, err)
	}
}

func TestSubmitVisibilityAndLimiter(t *testing.T) {
	s, q, entry := commentHarness(t)
	ctx := context.Background()
	if err := q.CreateEntry(ctx, db.CreateEntryParams{ID: "private", ContentTypeID: "post", Slug: "private", Status: "active", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := q.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "private-rev", EntryID: "private", RevisionNumber: 1, Title: "Private", DocumentJson: `{"version":1,"nodes":[]}`, Visibility: "private", CommentsEnabled: 1, CreatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if err := q.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{ID: "private", PublishedRevisionID: sql.NullString{String: "private-rev", Valid: true}, PublishedAt: sql.NullInt64{Int64: 1, Valid: true}, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Submit(ctx, "private", "", "A", "a@b.co", "", "x", "", "", "", "1.1.1.1:1", true, 1); err != ErrNotCommentable {
		t.Fatalf("private err=%v", err)
	}
	for i := int64(2); i < 7; i++ {
		submitComment(t, s, entry, "", i)
	}
	if _, err := s.Submit(context.Background(), entry, "", "A", "a@b.co", "", "x", "", "", "", "127.0.0.1:1234", true, 8); err != ErrRateLimited {
		t.Fatalf("limiter err=%v", err)
	}
}
