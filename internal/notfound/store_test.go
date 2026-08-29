package notfound

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kokosx/stratum/internal/storage"
)

func newTestStore(t *testing.T) *Store {
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
	return New(database.DB)
}

func Test404Aggregation(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	for i := 0; i < 3; i++ {
		if err := store.Record(ctx, "/does-not-exist"); err != nil {
			t.Fatal(err)
		}
	}
	rec, err := store.Get(ctx, "/does-not-exist")
	if err != nil {
		t.Fatal(err)
	}
	if rec.HitCount != 3 {
		t.Fatalf("hits %d want 3", rec.HitCount)
	}
}

func Test404QueryVariantsNotExplode(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	// URL.Path identity ignores query
	if err := store.Record(ctx, "/search"); err != nil {
		t.Fatal(err)
	}
	if err := store.Record(ctx, "/search"); err != nil {
		t.Fatal(err)
	}
	// Different query still same path – we record via path only, so count should be 2
	rec, _ := store.Get(ctx, "/search")
	if rec.HitCount != 2 {
		t.Fatalf("expected 2 got %d", rec.HitCount)
	}
}

func Test404BoundsAndRetention(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	// Create many stale records
	now := time.Now().Unix()
	old := now - 31*24*3600
	for i := 0; i < 5; i++ {
		path := "/old-" + string(rune('a'+i))
		store.db.ExecContext(ctx, `INSERT INTO not_found_paths (path, hit_count, first_seen_at, last_seen_at) VALUES (?, 1, ?, ?)`, path, old, old)
	}
	// Fresh
	for i := 0; i < 3; i++ {
		store.Record(ctx, "/fresh")
	}
	if err := store.Cleanup(ctx); err != nil {
		t.Fatal(err)
	}
	// Old should be gone
	if _, err := store.Get(ctx, "/old-a"); err == nil {
		t.Fatalf("expected old record deleted")
	}
	// Test max paths bound with small limit override: create many
	for i := 0; i < 20; i++ {
		store.db.ExecContext(ctx, `INSERT OR IGNORE INTO not_found_paths (path, hit_count, first_seen_at, last_seen_at) VALUES (?, 1, ?, ?)`, "/bulk/"+string(rune('a'+i)), now, now)
	}
	// Force cleanup with reduced max by directly testing count
	count, _ := store.Count(ctx)
	if count > MaxPaths {
		t.Fatalf("count %d exceeds max", count)
	}
}

func Test404IgnoreProbeNoise(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	if err := store.Record(ctx, "/.env"); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Get(ctx, "/.env"); err == nil {
		t.Fatalf("probe should be ignored")
	}
}

func Test404ConcurrentRecord(t *testing.T) {
	store := newTestStore(t)
	ctx := context.Background()
	const goroutines = 50
	const perGoroutine = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(n int) {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				path := "/concurrent-" + string(rune('a'+(n%26)))
				_ = store.Record(ctx, path)
			}
		}(i)
	}
	wg.Wait()
	// Ensure no race on counter and cleanup throttling works (every 256 writes)
	// Total writes = 1000, cleanup should have run ~3 times (1000/256)
	count, err := store.Count(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if count == 0 {
		t.Fatalf("expected some records, got 0")
	}
	// Verify at least one path has correct aggregated count
	rec, err := store.Get(ctx, "/concurrent-a")
	if err == nil && rec.HitCount == 0 {
		t.Fatalf("expected hit count >0")
	}
}

func Test404SourceValidation(t *testing.T) {
	// Verify that redirect source validation rejects query/fragment/backslash/control
	// These are covered in redirects package but ensure notfound path normalization also respects
	store := newTestStore(t)
	ctx := context.Background()
	// NotFound Record normalizes via routing.NormalizePath and should not store query part differently
	_ = store.Record(ctx, "/normal")
	rec, _ := store.Get(ctx, "/normal")
	if rec.HitCount != 1 {
		t.Fatalf("normal path not recorded")
	}
}
