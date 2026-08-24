package taxonomy

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func TestRevisionAssignmentDraftIsolation(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	database, err := storage.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	queries := db.New(database.DB)
	svc := New(database.DB, queries)

	catA, _ := svc.CreateTerm(ctx, "category", "Category A", "cat-a", "", "")
	catB, _ := svc.CreateTerm(ctx, "category", "Category B", "cat-b", "", "")

	now := time.Now().Unix()
	entryID := "post-tax-1"
	_ = queries.CreateEntry(ctx, db.CreateEntryParams{ID: entryID, ContentTypeID: "post", Slug: "post-tax-1", Status: "active", CreatedAt: now, UpdatedAt: now})
	rev1ID := "rev1"
	_ = queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: rev1ID, EntryID: entryID, RevisionNumber: 1, Title: "Post", DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now})
	_ = queries.SetTermsForRevision(ctx, db.SetTermsForRevisionParams{RevisionID: rev1ID, TermID: catA.ID})
	_ = queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: rev1ID, Valid: true}, PublishedAt: sql.NullInt64{Int64: now, Valid: true}, UpdatedAt: now, ID: entryID})

	rev2ID := "rev2"
	_ = queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: rev2ID, EntryID: entryID, RevisionNumber: 2, Title: "Post", DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now + 1})
	_ = queries.SetTermsForRevision(ctx, db.SetTermsForRevisionParams{RevisionID: rev2ID, TermID: catB.ID})

	termsPub, _ := queries.ListTermsForRevision(ctx, rev1ID)
	if len(termsPub) != 1 || termsPub[0].ID != catA.ID {
		t.Fatalf("published terms wrong")
	}
	termsDraft, _ := queries.ListTermsForRevision(ctx, rev2ID)
	if len(termsDraft) != 1 || termsDraft[0].ID != catB.ID {
		t.Fatalf("draft terms wrong")
	}
	_ = queries.SetPublishedRevision(ctx, db.SetPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: rev2ID, Valid: true}, PublishedAt: sql.NullInt64{Int64: now + 2, Valid: true}, UpdatedAt: now + 2, ID: entryID})
	termsPub2, _ := queries.ListTermsForRevision(ctx, rev2ID)
	if len(termsPub2) != 1 || termsPub2[0].ID != catB.ID {
		t.Fatalf("after publish, published should be catB")
	}
	countA, _ := queries.CountPublishedEntriesByTerm(ctx, catA.ID)
	if countA != 0 {
		t.Fatalf("catA count should be 0 after publish moved, got %d", countA)
	}
	countB, _ := queries.CountPublishedEntriesByTerm(ctx, catB.ID)
	if countB != 1 {
		t.Fatalf("catB count should be 1, got %d", countB)
	}
}
