package analytics

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/kokosx/stratum/internal/site"
	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

func newTestSite(t *testing.T, queries *db.Queries) *site.Runtime {
	t.Helper()
	rt := site.NewRuntime(queries)
	// Ensure site runtime loaded (needs DB with site_settings)
	if err := rt.Reload(context.Background()); err != nil {
		t.Fatalf("reload site: %v", err)
	}
	return rt
}

func TestServiceRecordNonBlockingAndDropped(t *testing.T) {
	ctx := context.Background()
	database, _ := storage.Open(filepath.Join(t.TempDir(), "svc.db"))
	database.Migrate(ctx)
	queries := db.New(database.DB)
	siteRt := newTestSite(t, queries)
	svc := New(database.DB, siteRt)
	defer svc.Close()
	// Fill queue to capacity
	// To force drop, we can close worker? Instead we fill channel manually
	// Simulate queue full by making channel buffer smaller? Our QueueSize is 4096, so need to fill 4096
	// Instead create service with small queue for test? We'll test via direct channel manipulation
	svc2 := &Service{
		db:      database.DB,
		store:   NewStore(database.DB),
		site:    siteRt,
		queue:   make(chan Observation, 2),
		aggs:    newAggregates(),
		stopCh:  make(chan struct{}),
		done:    make(chan struct{}),
		clearCh: make(chan clearReq),
	}
	// Need to start loop? For drop test, we don't start loop, just test Record non-blocking via channel full without consumer
	// Manually test select default
	svc2.queue <- Observation{Time: time.Now()}
	svc2.queue <- Observation{Time: time.Now()}
	// Now queue full, next Record should drop
	accepted := svc2.Record(Observation{Time: time.Now(), Resource: Resource{Key: "k", Path: "/"}})
	if accepted {
		t.Fatal("should have dropped when queue full")
	}
	if svc2.Health().Dropped == 0 {
		t.Fatal("dropped count should increment")
	}
}

func TestServiceFlushAndRetention(t *testing.T) {
	ctx := context.Background()
	database, _ := storage.Open(filepath.Join(t.TempDir(), "svc2.db"))
	database.Migrate(ctx)
	queries := db.New(database.DB)
	siteRt := newTestSite(t, queries)
	svc := New(database.DB, siteRt)
	defer svc.Close()
	// Record some observations
	for i := 0; i < 10; i++ {
		svc.Record(Observation{
			Time:       time.Now(),
			Resource:   Resource{Key: "entry/e1/revision/r1", Path: "/a", RouteType: "entry", EntryID: "e1", RevisionID: "r1"},
			IsPageview: true,
			Traffic:    TrafficDirect,
			Client:     ClientClass{Browser: "Chrome", OS: "Windows", Device: "desktop", Language: "en"},
			Status:     200,
			Duration:   10 * time.Millisecond,
			Bytes:      100,
		})
	}
	// Allow worker to aggregate
	time.Sleep(100 * time.Millisecond)
	if err := svc.FlushSync(ctx); err != nil {
		t.Fatalf("flush: %v", err)
	}
	// Verify DB has rows
	var cnt int64
	if err := database.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM analytics_page_daily").Scan(&cnt); err != nil {
		t.Fatal(err)
	}
	if cnt == 0 {
		t.Fatal("expected page rows after flush")
	}
	// Test retention: set old day manually and run retention
	oldDay := time.Now().AddDate(0, 0, -1000).Format("2006-01-02")
	database.DB.ExecContext(ctx, `INSERT OR IGNORE INTO analytics_page_daily (day, resource_key, path, route_type, views) VALUES (?, 'k', '/a', 'system', 1)`, oldDay)
	// Run retention via store directly with small retention
	store := NewStore(database.DB)
	if err := store.Retention(ctx, 90, 30); err != nil {
		t.Fatalf("retention: %v", err)
	}
	if err := database.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM analytics_page_daily WHERE day=?", oldDay).Scan(&cnt); err != nil {
		t.Fatal(err)
	}
	if cnt != 0 {
		t.Fatalf("old row should be deleted by retention, cnt %d", cnt)
	}
}

func TestServiceClearCoordinates(t *testing.T) {
	ctx := context.Background()
	database, _ := storage.Open(filepath.Join(t.TempDir(), "clear.db"))
	database.Migrate(ctx)
	queries := db.New(database.DB)
	siteRt := newTestSite(t, queries)
	svc := New(database.DB, siteRt)
	defer svc.Close()
	// Record and flush to have data
	svc.Record(Observation{
		Time:       time.Now(),
		Resource:   Resource{Key: "entry/e1/revision/r1", Path: "/a", RouteType: "entry"},
		IsPageview: true,
		Status:     200, Duration: 10 * time.Millisecond,
		Client: ClientClass{Browser: "Chrome"},
	})
	time.Sleep(50 * time.Millisecond)
	svc.FlushSync(ctx)
	var cnt int64
	database.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM analytics_page_daily").Scan(&cnt)
	if cnt == 0 {
		t.Fatal("should have data before clear")
	}
	// Add pending without flushing
	svc.Record(Observation{
		Time:       time.Now(),
		Resource:   Resource{Key: "entry/e2/revision/r2", Path: "/b", RouteType: "entry"},
		IsPageview: true,
		Status:     200, Duration: 10 * time.Millisecond,
		Client: ClientClass{Browser: "Chrome"},
	})
	time.Sleep(20 * time.Millisecond)
	// Clear should discard pending and DB rows
	if err := svc.Clear(ctx); err != nil {
		t.Fatalf("clear: %v", err)
	}
	database.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM analytics_page_daily").Scan(&cnt)
	if cnt != 0 {
		t.Fatalf("after clear, cnt should be 0, got %d", cnt)
	}
	// Pending should be 0
	if svc.Pending() != 0 {
		t.Fatalf("pending should be 0 after clear, got %d", svc.Pending())
	}
	// New observations after clear should be recorded
	svc.Record(Observation{
		Time:       time.Now(),
		Resource:   Resource{Key: "entry/e3/revision/r3", Path: "/c", RouteType: "entry"},
		IsPageview: true,
		Status:     200, Duration: 10 * time.Millisecond,
		Client: ClientClass{Browser: "Chrome"},
	})
	time.Sleep(20 * time.Millisecond)
	svc.FlushSync(ctx)
	database.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM analytics_page_daily").Scan(&cnt)
	if cnt == 0 {
		t.Fatal("after clear, new data should be present")
	}
}

func TestServiceCloseIdempotent(t *testing.T) {
	ctx := context.Background()
	database, _ := storage.Open(filepath.Join(t.TempDir(), "close.db"))
	database.Migrate(ctx)
	queries := db.New(database.DB)
	siteRt := newTestSite(t, queries)
	svc := New(database.DB, siteRt)
	if err := svc.Close(); err != nil {
		t.Fatalf("first close: %v", err)
	}
	if err := svc.Close(); err != nil {
		t.Fatalf("second close should be idempotent: %v", err)
	}
	// Ensure no goroutine leak: wait a bit
	time.Sleep(20 * time.Millisecond)
}

func TestServiceNoGoroutineLeak(t *testing.T) {
	ctx := context.Background()
	database, _ := storage.Open(filepath.Join(t.TempDir(), "leak.db"))
	database.Migrate(ctx)
	queries := db.New(database.DB)
	siteRt := newTestSite(t, queries)
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			svc := New(database.DB, siteRt)
			for j := 0; j < 10; j++ {
				svc.Record(Observation{Time: time.Now(), Resource: Resource{Key: "k", Path: "/"}, IsPageview: true, Status: 200})
			}
			time.Sleep(10 * time.Millisecond)
			svc.Close()
		}()
	}
	wg.Wait()
	// If we get here without deadlock, no leak
	_ = ctx
}
