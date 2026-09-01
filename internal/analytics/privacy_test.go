package analytics

import (
	"context"
	"database/sql"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kokosx/stratum/internal/storage"
)

func TestPrivacyRegression(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "priv.db")
	database, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	store := NewStore(database.DB)

	secretIP := "192.0.2.123"
	secretUA := "UNIQUE-RAW-UA-SECRET/123"
	secretReferer := "https://ref.example/private/path?email=alice@example.com"
	secretGCLID := "SUPER-SECRET-CLICK-ID"
	secretEmail := "alice@example.com"

	// Simulate observation building from raw request with secret markers
	rawUA := secretUA
	rawReferer := secretReferer
	rawQuery := "utm_source=newsletter&utm_medium=email&utm_campaign=launch&utm_term=alice@example.com&gclid=" + secretGCLID
	// Classify
	client := ClientClass{
		Browser:  ClassifyBrowser(rawUA),
		OS:       ClassifyOS(rawUA),
		Device:   ClassifyDevice(rawUA),
		Language: ClassifyLanguage("en-US"),
	}
	crawler := ClassifyCrawler(rawUA)
	referrerHost := SanitizeReferrerHost(rawReferer)
	utmSource, utmMedium, utmCampaign := ParseUTMFromQuery(rawQuery)
	// Ensure forbidden values are not persisted: utm_term, gclid must not be in sanitized dimensions
	// Our ParseUTM only extracts source/medium/campaign, so utm_term/gclid ignored

	// Build observation
	obs := Observation{
		Time:         time.Now(),
		Resource:     Resource{Key: "entry/e1/revision/r1", Path: "/hello", RouteType: "entry", EntryID: "e1", RevisionID: "r1"},
		IsPageview:   true,
		Traffic:      ClassifyTraffic(referrerHost, "example.com", utmSource, utmMedium, utmCampaign),
		Client:       client,
		Crawler:      crawler,
		Status:       200,
		Duration:     20 * time.Millisecond,
		Bytes:        1000,
		ReferrerHost: referrerHost,
		UTMSource:    utmSource,
		UTMMedium:    utmMedium,
		UTMCampaign:  utmCampaign,
	}
	// Ensure secrets not in observation fields
	obsStr := strings.Join([]string{obs.ReferrerHost, obs.UTMSource, obs.UTMMedium, obs.UTMCampaign, obs.Client.Browser, obs.Crawler}, "|")
	for _, secret := range []string{secretIP, secretUA, secretGCLID, secretEmail, "/private/path", "full raw Referer"} {
		if strings.Contains(obsStr, secret) {
			t.Fatalf("observation contains secret %q", secret)
		}
	}
	if referrerHost != "ref.example" {
		t.Fatalf("referrer host should be ref.example, got %q", referrerHost)
	}
	if utmSource != "newsletter" {
		t.Fatalf("utm source wrong")
	}
	// Flush
	aggs := newAggregates()
	aggs.add(obs)
	site, page, dim, trans := aggs.snapshotAndReset()
	if err := store.Flush(ctx, site, page, dim, trans); err != nil {
		t.Fatalf("flush: %v", err)
	}
	// Inspect ALL analytics tables for secrets
	tables := []string{"analytics_site_hourly", "analytics_page_daily", "analytics_dimension_daily", "analytics_transition_daily"}
	for _, tbl := range tables {
		// Dump schema to check for forbidden columns
		rows, err := database.DB.QueryContext(ctx, "SELECT sql FROM sqlite_master WHERE type='table' AND name=?", tbl)
		if err != nil {
			t.Fatalf("schema query: %v", err)
		}
		if rows.Next() {
			var sqlStmt sql.NullString
			rows.Scan(&sqlStmt)
			if sqlStmt.Valid {
				s := strings.ToLower(sqlStmt.String)
				for _, forbidden := range []string{"ip", "visitor_id", "session_id", "raw_user_agent", "raw_referrer", "query_string", "properties_json", "visitor", "session"} {
					// Need to check column names not just substring but ensure no column named forbidden
					// Simplistic: check if column definition contains forbidden name as column
					if strings.Contains(s, forbidden) {
						// But ip appears in many places? Check for "ip" as column name: look for " ip " or " ip," or "\"ip\""
						if forbidden == "ip" {
							if strings.Contains(s, " ip ") || strings.Contains(s, " ip,") || strings.Contains(s, "\"ip\"") || strings.Contains(s, "'ip'") {
								t.Fatalf("table %s schema contains forbidden column %q: %s", tbl, forbidden, s)
							}
						} else {
							// For other forbiddens, any occurrence is suspicious
							if strings.Contains(s, forbidden) {
								t.Fatalf("table %s schema contains forbidden %q: %s", tbl, forbidden, s)
							}
						}
					}
				}
			}
		}
		rows.Close()
		// Scan rows for secret content
		var query string
		switch tbl {
		case "analytics_site_hourly":
			query = "SELECT hour, requests, views FROM analytics_site_hourly"
		case "analytics_page_daily":
			query = "SELECT day, resource_key, path FROM analytics_page_daily"
		case "analytics_dimension_daily":
			query = "SELECT day, dimension, value, count FROM analytics_dimension_daily"
		case "analytics_transition_daily":
			query = "SELECT day, from_resource_key, to_resource_key, from_path, to_path FROM analytics_transition_daily"
		}
		rows2, err := database.DB.QueryContext(ctx, query)
		if err != nil {
			continue
		}
		cols, _ := rows2.Columns()
		for rows2.Next() {
			vals := make([]interface{}, len(cols))
			ptrs := make([]interface{}, len(cols))
			for i := range vals {
				ptrs[i] = &vals[i]
			}
			rows2.Scan(ptrs...)
			for _, v := range vals {
				var s string
				switch x := v.(type) {
				case string:
					s = x
				case []byte:
					s = string(x)
				case int64:
					s = ""
				default:
					continue
				}
				for _, secret := range []string{secretIP, secretUA, "/private/path", secretGCLID, secretEmail, "SUPER-SECRET"} {
					if strings.Contains(s, secret) {
						t.Fatalf("table %s contains secret %q in value %q", tbl, secret, s)
					}
				}
				// Also check that raw referrer path not persisted
				if strings.Contains(s, "private") && tbl == "analytics_dimension_daily" {
					// Should not have path, only host
					t.Fatalf("dimension should not contain private path, got %q", s)
				}
			}
		}
		rows2.Close()
	}
	// Verify expected aggregate values are present
	var cnt int64
	err = database.DB.QueryRowContext(ctx, "SELECT count FROM analytics_dimension_daily WHERE dimension='referrer_host' AND value='ref.example'").Scan(&cnt)
	if err != nil {
		t.Fatalf("expected ref.example in referrer_host dimension: %v", err)
	}
	if cnt == 0 {
		t.Fatal("ref.example count should be >0")
	}
	err = database.DB.QueryRowContext(ctx, "SELECT count FROM analytics_dimension_daily WHERE dimension='utm_source' AND value='newsletter'").Scan(&cnt)
	if err != nil {
		t.Fatalf("expected newsletter in utm_source: %v", err)
	}
	// Ensure gclid not stored as dimension value
	row := database.DB.QueryRowContext(ctx, "SELECT count FROM analytics_dimension_daily WHERE value=?", secretGCLID)
	var c sql.NullInt64
	err = row.Scan(&c)
	if err == nil && c.Valid && c.Int64 > 0 {
		t.Fatalf("gclid should not be stored")
	}
}

func TestNoRawEventTable(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "noevent.db")
	database, err := storage.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	// Ensure no table named analytics_events exists
	row := database.DB.QueryRowContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name='analytics_events'")
	var name sql.NullString
	if err := row.Scan(&name); err == nil && name.Valid {
		t.Fatalf("analytics_events table should not exist")
	}
}
