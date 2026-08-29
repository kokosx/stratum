package search

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"testing"
	"time"
)

func TestRebuildAllOrNothing(t *testing.T) {
	svc, _, queries, _ := newSearchHarness(t)
	ctx := context.Background()
	now := int64(1700000000)

	// Create three entries A,B,C
	insertPublishedEntry(t, queries, nil, svc, "entryA", "page", "a-page", "Alpha entry", "", `{"version":1,"nodes":[{"id":"n1","block":"core/text","version":1,"props":{"text":"alpha"},"settings":{}}]}`, `{}`, "public", now)
	insertPublishedEntry(t, queries, nil, svc, "entryB", "page", "b-page", "Beta entry", "", `{"version":1,"nodes":[{"id":"n1","block":"core/text","version":1,"props":{"text":"beta"},"settings":{}}]}`, `{}`, "public", now+1)
	insertPublishedEntry(t, queries, nil, svc, "entryC", "page", "c-page", "Gamma entry", "", `{"version":1,"nodes":[{"id":"n1","block":"core/text","version":1,"props":{"text":"gamma"},"settings":{}}]}`, `{}`, "public", now+2)

	// Ensure initial index has 3
	if n, _ := svc.CountDocuments(ctx); n != 3 {
		t.Fatalf("initial count %d want 3", n)
	}
	// Verify all searchable
	for _, q := range []string{"Alpha", "Beta", "Gamma"} {
		res, _ := mustQuery(t, svc, q)
		if len(res) == 0 {
			t.Fatalf("initial query %q should find result", q)
		}
	}

	// Force BuildDocument(B) to fail
	origBuild := func(ctx context.Context, id string) (Document, error) {
		return svc.BuildDocument(ctx, id)
	}
	svc.SetRebuildHook(func(ctx context.Context, id string) (Document, error) {
		if id == "entryB" {
			return Document{}, fmt.Errorf("forced failure for B")
		}
		return origBuild(ctx, id)
	})
	// Ensure hook is cleared after test
	defer svc.SetRebuildHook(nil)

	n, err := svc.Rebuild(ctx)
	if err == nil {
		t.Fatalf("Rebuild should return error when B fails, got n=%d", n)
	}
	if !strings.Contains(err.Error(), "build search document entryB") {
		t.Fatalf("error should contain context 'build search document entryB', got %q", err.Error())
	}

	// Old index must remain unchanged (3 docs)
	if n2, _ := svc.CountDocuments(ctx); n2 != 3 {
		t.Fatalf("after failed rebuild count %d want 3 (old index preserved)", n2)
	}
	for _, q := range []string{"Alpha", "Beta", "Gamma"} {
		res, _ := mustQuery(t, svc, q)
		if len(res) == 0 {
			t.Fatalf("after failed rebuild query %q should still find result, old index must remain", q)
		}
	}

	// Now clear hook and succeed
	svc.SetRebuildHook(nil)
	n, err = svc.Rebuild(ctx)
	if err != nil {
		t.Fatalf("second rebuild should succeed: %v", err)
	}
	if n != 3 {
		t.Fatalf("second rebuild count %d want 3", n)
	}
}

func TestRebuildSkipsErrNoRows(t *testing.T) {
	svc, database, queries, _ := newSearchHarness(t)
	ctx := context.Background()
	now := int64(1700000000)
	insertPublishedEntry(t, queries, database, svc, "entryA", "page", "a2-page", "Alpha2", "", `{"version":1,"nodes":[]}`, `{}`, "public", now)
	insertPublishedEntry(t, queries, database, svc, "entryB2", "page", "b2-page", "Beta2", "", `{"version":1,"nodes":[]}`, `{}`, "public", now+1)

	// Make B no longer qualify: set it to draft (clear published)
	_, _ = database.DB.ExecContext(ctx, `UPDATE entries SET published_revision_id = NULL WHERE id = ?`, "entryB2")
	// It should still be considered expected? But BuildDocument will now return sql.ErrNoRows and rebuild should skip it
	// Count expected before rebuild is 2 docs (old index still has B), after rebuild should be 1
	n, err := svc.Rebuild(ctx)
	if err != nil {
		t.Fatalf("rebuild should skip ErrNoRows: %v", err)
	}
	if n != 1 {
		t.Fatalf("rebuild count %d want 1 (B skipped)", n)
	}
	// B should not be searchable
	res, _ := mustQuery(t, svc, "Beta2")
	if len(res) != 0 {
		t.Fatalf("B should not be searchable after becoming draft, got %v", res)
	}
}

func TestRebuildPreservesOldOnGenericError(t *testing.T) {
	svc, _, queries, _ := newSearchHarness(t)
	ctx := context.Background()
	now := int64(1700000000)
	insertPublishedEntry(t, queries, nil, svc, "e1", "page", "e1-page", "E1", "", `{"version":1,"nodes":[]}`, `{}`, "public", now)
	insertPublishedEntry(t, queries, nil, svc, "e2", "page", "e2-page", "E2", "", `{"version":1,"nodes":[]}`, `{}`, "public", now+1)

	// Force generic error via hook (simulating malformed document / transient DB error)
	svc.SetRebuildHook(func(ctx context.Context, id string) (Document, error) {
		if id == "e2" {
			return Document{}, fmt.Errorf("malformed document")
		}
		return svc.BuildDocument(ctx, id)
	})
	defer svc.SetRebuildHook(nil)

	n, err := svc.Rebuild(ctx)
	if err == nil {
		t.Fatalf("rebuild should fail on malformed document, got n=%d", n)
	}
	if !strings.Contains(err.Error(), "build search document e2") {
		t.Fatalf("error should wrap entry id, got %q", err.Error())
	}
	// Old index still has both
	var cnt int
	_ = svc.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM search_documents`).Scan(&cnt)
	if cnt != 2 {
		t.Fatalf("old projection should remain 2, got %d", cnt)
	}
	// Ensure error not leaked to public via search query (search should still work)
	_, _, _, err = svc.QueryFiltered(ctx, "E1", "", 1)
	if err != nil {
		t.Fatalf("query after failed rebuild should not leak internal error: %v", err)
	}
}

func TestRebuildStillCountsValid(t *testing.T) {
	svc, _, queries, _ := newSearchHarness(t)
	ctx := context.Background()
	now := time.Now().Unix()
	for i := 0; i < 5; i++ {
		id := fmt.Sprintf("rebuild-ok-%d", i)
		insertPublishedEntry(t, queries, nil, svc, id, "page", fmt.Sprintf("ok-%d", i), fmt.Sprintf("Title %d", i), "", `{"version":1,"nodes":[]}`, `{}`, "public", now+int64(i))
	}
	// Delete projection manually
	_, _ = svc.db.ExecContext(ctx, `DELETE FROM search_documents_fts; DELETE FROM search_documents`)
	n, err := svc.Rebuild(ctx)
	if err != nil {
		t.Fatalf("rebuild ok: %v", err)
	}
	if n != 5 {
		t.Fatalf("rebuild count %d want 5", n)
	}
	var dbCount int
	if err := svc.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM search_documents`).Scan(&dbCount); err != nil {
		t.Fatalf("count: %v", err)
	}
	if dbCount != 5 {
		t.Fatalf("db count %d want 5", dbCount)
	}
	// Verify sql.ErrNoRows is properly handled via errors.Is path: use wrapped error
	svc.SetRebuildHook(func(ctx context.Context, id string) (Document, error) {
		return Document{}, sql.ErrNoRows
	})
	defer svc.SetRebuildHook(nil)
	n, err = svc.Rebuild(ctx)
	if err != nil {
		t.Fatalf("wrapped ErrNoRows should be skipped, got %v", err)
	}
	if n != 0 {
		t.Fatalf("all ErrNoRows should yield 0 docs, got %d", n)
	}
}
