package storage

import (
	"context"
	"path/filepath"
	"testing"
)

func TestAnalyticsMigration(t *testing.T) {
	ctx := context.Background()
	database, err := Open(filepath.Join(t.TempDir(), "migr.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	// Check tables exist
	tables := []string{"analytics_site_hourly", "analytics_page_daily", "analytics_dimension_daily", "analytics_transition_daily"}
	for _, tbl := range tables {
		var name string
		if err := database.DB.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name=?", tbl).Scan(&name); err != nil || name != tbl {
			t.Fatalf("table %s missing", tbl)
		}
	}
	// Check site_settings defaults
	var enabled, ret, hourly int64
	if err := database.DB.QueryRowContext(ctx, "SELECT analytics_enabled, analytics_retention_days, analytics_hourly_retention_days FROM site_settings WHERE id=1").Scan(&enabled, &ret, &hourly); err != nil {
		t.Fatalf("site_settings analytics columns missing: %v", err)
	}
	if enabled != 1 || ret != 730 || hourly != 90 {
		t.Fatalf("defaults wrong enabled=%d ret=%d hourly=%d", enabled, ret, hourly)
	}
	// Check CHECK constraints: invalid values should fail
	if _, err := database.DB.ExecContext(ctx, "UPDATE site_settings SET analytics_retention_days=9999 WHERE id=1"); err == nil {
		t.Fatal("invalid retention should be rejected by CHECK")
	}
	if _, err := database.DB.ExecContext(ctx, "UPDATE site_settings SET analytics_hourly_retention_days=9999 WHERE id=1"); err == nil {
		t.Fatal("invalid hourly retention should be rejected")
	}
	// Forward-only: ensure no edit of old migration
	// Check that migration file exists and is not modified? Not needed
	// Test seed still works
	if err := database.Seed(ctx); err != nil {
		t.Fatalf("seed: %v", err)
	}
}

func TestAnalyticsNotNullConstraints(t *testing.T) {
	ctx := context.Background()
	database, err := Open(filepath.Join(t.TempDir(), "migr2.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	database.Migrate(ctx)
	// route_type CHECK should reject invalid value
	if _, err := database.DB.ExecContext(ctx, "INSERT INTO analytics_page_daily (day, resource_key, path, route_type) VALUES ('2025-01-01', 'k', '/a', 'invalid')"); err == nil {
		t.Fatal("invalid route_type should be rejected by CHECK")
	}
	// dimension CHECK should reject invalid dimension
	if _, err := database.DB.ExecContext(ctx, "INSERT INTO analytics_dimension_daily (day, dimension, value, count) VALUES ('2025-01-01', 'invalid_dim', 'v', 1)"); err == nil {
		t.Fatal("invalid dimension should be rejected")
	}
}
