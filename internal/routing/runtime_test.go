package routing

import (
	"context"
	"database/sql"
	"path/filepath"
	"sync"
	"testing"

	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func newTestRuntime(t *testing.T) (*Runtime, *db.Queries, func()) {
	t.Helper()
	ctx := context.Background()
	database, err := storage.Open(filepath.Join(t.TempDir(), "rt.db"))
	if err != nil {
		t.Fatal(err)
	}
	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	// Disable FK for test convenience (routes with entry_id without actual entry row)
	_, _ = database.DB.ExecContext(ctx, "PRAGMA foreign_keys = OFF")
	queries := db.New(database.DB)
	rt := NewRuntime(queries)
	if err := rt.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	return rt, queries, func() { _ = database.Close() }
}

func TestRouteRuntimeLookupHit(t *testing.T) {
	rt, queries, cleanup := newTestRuntime(t)
	defer cleanup()
	ctx := context.Background()
	// Need entries for FK
	_ = queries.CreateContentType(ctx, db.CreateContentTypeParams{ID: "page", DisplayName: "Page", PluralName: "Pages", ConfigJson: "{}"})
	_ = queries.CreateContentType(ctx, db.CreateContentTypeParams{ID: "post", DisplayName: "Post", PluralName: "Posts", ConfigJson: "{}"})
	_ = queries.CreateEntry(ctx, db.CreateEntryParams{ID: "e1", ContentTypeID: "page", Slug: "about", Status: "active", CreatedAt: 1, UpdatedAt: 1})
	if err := queries.CreateRoute(ctx, db.CreateRouteParams{ID: "r1", Path: "/about", EntryID: sql.NullString{String: "e1", Valid: true}, RouteType: RouteTypeEntry, CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatalf("create r1: %v", err)
	}
	if err := queries.CreateRoute(ctx, db.CreateRouteParams{ID: "r2", Path: "/blog", RouteType: RouteTypeArchive, ContentTypeID: sql.NullString{String: "post", Valid: true}, CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatalf("create r2: %v", err)
	}
	if err := queries.CreateRoute(ctx, db.CreateRouteParams{ID: "r3", Path: "/old", RouteType: RouteTypeRedirect, RedirectTo: sql.NullString{String: "/new", Valid: true}, RedirectStatus: sql.NullInt64{Int64: 301, Valid: true}, CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatalf("create r3: %v", err)
	}
	if err := rt.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	if r, ok := rt.Lookup("/about"); !ok || r.RouteType != RouteTypeEntry || !r.EntryID.Valid || r.EntryID.String != "e1" {
		t.Fatalf("lookup /about = %#v ok=%v", r, ok)
	}
	if r, ok := rt.Lookup("/blog"); !ok || r.RouteType != RouteTypeArchive || r.ContentTypeID.String != "post" {
		t.Fatalf("lookup /blog = %#v", r)
	}
	if r, ok := rt.Lookup("/old"); !ok || r.RouteType != RouteTypeRedirect || r.RedirectTo.String != "/new" {
		t.Fatalf("lookup /old = %#v", r)
	}
}

func TestRouteRuntimeLookupMiss(t *testing.T) {
	rt, _, cleanup := newTestRuntime(t)
	defer cleanup()
	if _, ok := rt.Lookup("/not-found"); ok {
		t.Fatal("expected miss")
	}
	if _, ok := rt.Lookup("/wp-login.php"); ok {
		t.Fatal("expected miss for bot path")
	}
}

func TestRouteRuntimeReloadPublishesImmutableSnapshot(t *testing.T) {
	rt, queries, cleanup := newTestRuntime(t)
	defer cleanup()
	ctx := context.Background()
	_ = queries.CreateRoute(ctx, db.CreateRouteParams{ID: "r1", Path: "/a", RouteType: RouteTypeEntry, EntryID: sql.NullString{String: "e1", Valid: true}, CreatedAt: 1, UpdatedAt: 1})
	if err := rt.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	snap1 := rt.Current()
	if _, ok := snap1.ByPath["/a"]; !ok {
		t.Fatal("snap1 should have /a")
	}
	// Add new route and reload
	_ = queries.CreateRoute(ctx, db.CreateRouteParams{ID: "r2", Path: "/b", RouteType: RouteTypeEntry, EntryID: sql.NullString{String: "e2", Valid: true}, CreatedAt: 2, UpdatedAt: 2})
	if err := rt.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	snap2 := rt.Current()
	if _, ok := snap2.ByPath["/b"]; !ok {
		t.Fatal("snap2 should have /b")
	}
	// snap1 must remain unchanged (immutable)
	if _, ok := snap1.ByPath["/b"]; ok {
		t.Fatal("snap1 should not see /b (immutable)")
	}
	if _, ok := rt.Lookup("/b"); !ok {
		t.Fatal("lookup after reload should find /b")
	}
}

func TestRouteRuntimeFailedReloadPreservesOldSnapshot(t *testing.T) {
	rt, queries, cleanup := newTestRuntime(t)
	defer cleanup()
	ctx := context.Background()
	_ = queries.CreateRoute(ctx, db.CreateRouteParams{ID: "r1", Path: "/a", RouteType: RouteTypeEntry, EntryID: sql.NullString{String: "e1", Valid: true}, CreatedAt: 1, UpdatedAt: 1})
	if err := rt.Reload(ctx); err != nil {
		t.Fatal(err)
	}
	snapBefore := rt.Current()
	countBefore := len(snapBefore.ByPath)
	// Simulate failure by using a closed DB or invalid queries
	// Create a new runtime with same DB but after closing DB, reload should fail
	// Instead, close underlying DB and try reload
	badQueries := queries // same underlying DB
	// Make DB fail by creating a new database with broken state? Simpler: use context with cancelled?
	// We can simulate failure by passing a queries that wraps a closed DB
	// For this test, we close the database and then try reload – it should error
	// But we need to keep old snapshot, so we use the same rt but with a failing queries
	// To avoid corrupting rt.queries, we test by directly checking that Reload returns error when DB is closed
	// and that snapshot remains same count
	_ = badQueries // not used, we need to test via rt with closed DB
	// Close DB to make next query fail
	// Need to get underlying DB from storage? We have database handle via closure, but we lost reference.
	// Recreate a failing scenario by using a new Runtime with a bad DB
	// Instead, test that a second reload with no new routes but with a failing DB keeps old snapshot
	// We can create a temporary bad DB
	badDB, _ := storage.Open(filepath.Join(t.TempDir(), "bad.db"))
	_ = badDB.Migrate(ctx)
	badQ := db.New(badDB.DB)
	// Close it to make queries fail
	_ = badDB.Close()
	badRT := NewRuntime(badQ)
	// Seed badRT with a known snapshot first via good data then try reload with closed DB
	// Copy snapshot from rt
	badRT.snapshot.Store(snapBefore)
	if err := badRT.Reload(ctx); err == nil {
		t.Fatal("expected reload failure with closed DB")
	}
	snapAfter := badRT.Current()
	if len(snapAfter.ByPath) != countBefore {
		t.Fatalf("failed reload should preserve snapshot: before %d after %d", countBefore, len(snapAfter.ByPath))
	}
}

func TestRouteRuntimeConcurrentLookupAndReload(t *testing.T) {
	rt, queries, cleanup := newTestRuntime(t)
	defer cleanup()
	ctx := context.Background()
	_ = queries.CreateRoute(ctx, db.CreateRouteParams{ID: "r1", Path: "/a", RouteType: RouteTypeEntry, EntryID: sql.NullString{String: "e1", Valid: true}, CreatedAt: 1, UpdatedAt: 1})
	_ = rt.Reload(ctx)
	var wg sync.WaitGroup
	// Concurrent lookups
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				_, _ = rt.Lookup("/a")
				_, _ = rt.Lookup("/missing")
			}
		}()
	}
	// Concurrent reloads
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(n int) {
			defer wg.Done()
			_ = queries.CreateRoute(ctx, db.CreateRouteParams{ID: "r-conc-" + string(rune('a'+n)), Path: "/conc-" + string(rune('a'+n)), RouteType: RouteTypeEntry, EntryID: sql.NullString{String: "e" + string(rune('a'+n)), Valid: true}, CreatedAt: int64(n), UpdatedAt: int64(n)})
			_ = rt.Reload(ctx)
		}(i)
	}
	wg.Wait()
	// Should not panic or race
}
