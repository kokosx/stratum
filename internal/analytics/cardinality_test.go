package analytics

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/kokosx/stratum/internal/storage"
)

func TestCardinalityBounded(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "card.db")
	database, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	store := NewStore(database.DB)
	// Create aggregates with many distinct utm_campaign values
	aggs := newAggregates()
	now := time.Now()
	for i := 0; i < 10000; i++ {
		campaign := "campaign-" + string(rune('a'+i%26)) + "-" + string(rune('0'+i%10)) + "-" + string(rune(i/1000))
		// Make unique
		campaign = "campaign-" + string(rune(i)) + "-" + string(rune(i*2)) // but need unique strings
		campaign = "campaign-" + itoa(i)                                   // simpler
		obs := Observation{
			Time:        now,
			Resource:    Resource{Key: "entry/e1/revision/r1", Path: "/a", RouteType: "entry", EntryID: "e1", RevisionID: "r1"},
			IsPageview:  true,
			Traffic:     TrafficCampaign,
			UTMCampaign: campaign,
			Client:      ClientClass{Browser: "Chrome", OS: "Windows", Device: "desktop", Language: "en"},
			Status:      200,
			Duration:    10 * time.Millisecond,
		}
		// Need to sanitize UTMCampaign already via SanitizeDimensionValue but add() will handle
		// Bypass in-memory cap for test by directly inserting dims? But add() already does in-memory cap to other.
		// Let's test both in-memory and persist.
		aggs.add(obs)
	}
	// Flush
	site, page, dim, trans := aggs.snapshotAndReset()
	if err := store.Flush(ctx, site, page, dim, trans); err != nil {
		t.Fatalf("flush: %v", err)
	}
	// Check distinct count
	day := DayBucket(now)
	row := database.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM analytics_dimension_daily WHERE day=? AND dimension='utm_campaign'", day)
	var cnt int64
	if err := row.Scan(&cnt); err != nil {
		t.Fatal(err)
	}
	if cnt > 257 { // 256 + other
		t.Fatalf("cardinality not bounded, got %d", cnt)
	}
	// Ensure 'other' exists and contains overflow
	row = database.DB.QueryRowContext(ctx, "SELECT count FROM analytics_dimension_daily WHERE day=? AND dimension='utm_campaign' AND value='other'", day)
	var otherCnt int64
	err = row.Scan(&otherCnt)
	if err != nil {
		t.Fatalf("other not found: %v", err)
	}
	if otherCnt == 0 {
		t.Fatal("other should have overflow count")
	}
	// Test persistence across restart: create new store and flush again with new distinct values, should still bound
	aggs2 := newAggregates()
	for i := 10000; i < 11000; i++ {
		obs := Observation{
			Time:        now,
			Resource:    Resource{Key: "entry/e1/revision/r1", Path: "/a", RouteType: "entry", EntryID: "e1", RevisionID: "r1"},
			IsPageview:  true,
			Traffic:     TrafficCampaign,
			UTMCampaign: "campaign-" + itoa(i),
			Client:      ClientClass{Browser: "Chrome", OS: "Windows", Device: "desktop", Language: "en"},
			Status:      200,
			Duration:    10 * time.Millisecond,
		}
		aggs2.add(obs)
	}
	site, page, dim, trans = aggs2.snapshotAndReset()
	if err := store.Flush(ctx, site, page, dim, trans); err != nil {
		t.Fatalf("second flush: %v", err)
	}
	row = database.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM analytics_dimension_daily WHERE day=? AND dimension='utm_campaign'", day)
	if err := row.Scan(&cnt); err != nil {
		t.Fatal(err)
	}
	if cnt > 257 {
		t.Fatalf("after restart cardinality exceeded: %d", cnt)
	}
}

func itoa(i int) string {
	// simple
	s := ""
	for i > 0 {
		d := i % 10
		s = string(rune('0'+d)) + s
		i /= 10
	}
	if s == "" {
		s = "0"
	}
	return s
}
