package siteparts

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func openCycleService(t *testing.T) (*Service, *db.Queries) {
	t.Helper()
	ctx := context.Background()
	database, err := storage.Open(filepath.Join(t.TempDir(), "siteparts.db"))
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
	return NewService(database.DB, queries, registry), queries
}

func seedPart(t *testing.T, queries *db.Queries, id, doc string, published bool) {
	t.Helper()
	now := time.Now().Unix()
	if err := queries.CreateSitePart(context.Background(), db.CreateSitePartParams{ID: id, Name: id, CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatal(err)
	}
	revisionID := id + "-r1"
	if err := queries.CreateSitePartRevision(context.Background(), db.CreateSitePartRevisionParams{ID: revisionID, SitePartID: id, RevisionNumber: 1, DocumentJson: doc, CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	if published {
		if err := queries.SetSitePartPublishedRevision(context.Background(), db.SetSitePartPublishedRevisionParams{PublishedRevisionID: sql.NullString{String: revisionID, Valid: true}, UpdatedAt: now, ID: id}); err != nil {
			t.Fatal(err)
		}
	}
}

func partRef(id string) string {
	return `{"version":1,"nodes":[{"id":"ref-` + id + `","block":"core/site-part","version":1,"props":{},"settings":{"sitePartId":"` + id + `"}}]}`
}

const emptyPart = `{"version":1,"nodes":[]}`

func TestPublishUsesPublishedGraphNotReferencedDraft(t *testing.T) {
	service, queries := openCycleService(t)
	seedPart(t, queries, "a", emptyPart, false)
	seedPart(t, queries, "b", emptyPart, true)
	if err := queries.CreateSitePartRevision(context.Background(), db.CreateSitePartRevisionParams{ID: "b-r2", SitePartID: "b", RevisionNumber: 2, DocumentJson: partRef("a"), CreatedAt: time.Now().Unix()}); err != nil {
		t.Fatal(err)
	}
	if err := service.Publish(context.Background(), "a", "a", partRef("b"), ""); err != nil {
		t.Fatalf("publish should follow published B, not draft B: %v", err)
	}
}

func TestDraftCycleAndUnpublishedChildRules(t *testing.T) {
	service, queries := openCycleService(t)
	seedPart(t, queries, "a", emptyPart, false)
	seedPart(t, queries, "b", partRef("a"), false)
	if err := service.SaveDraft(context.Background(), "a", "a", partRef("b"), ""); err == nil || !strings.Contains(err.Error(), "cyclic") {
		t.Fatalf("draft cycle error = %v", err)
	}
	if err := service.Publish(context.Background(), "a", "a", partRef("b"), ""); err == nil || !strings.Contains(err.Error(), "not published") {
		t.Fatalf("unpublished child error = %v", err)
	}
}
