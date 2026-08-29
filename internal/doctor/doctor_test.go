package doctor

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/kokosx/stratum/internal/storage"
)

func newTestDB(t *testing.T) (string, *storage.Database) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("STRATUM_DATA_DIR", dir)
	dbPath := filepath.Join(dir, "stratum.db")
	database, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := database.Migrate(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	// Ensure site_settings exists with healthy defaults
	_, _ = database.DB.ExecContext(context.Background(), `INSERT OR IGNORE INTO site_settings (id, site_title, site_url, language, timezone, homepage_mode, indexing_enabled, sitemap_enabled, robots_mode, site_represents, posts_base_path, active_theme) VALUES (1, 'Test Site', 'https://example.com', 'en', 'UTC', 'latest_posts', 1, 1, 'managed', 'organization', '/blog', 'default')`)
	_, _ = database.DB.ExecContext(context.Background(), `UPDATE site_settings SET site_url='https://example.com', site_title='Test Site', language='en', timezone='UTC', homepage_mode='latest_posts', indexing_enabled=1, sitemap_enabled=1 WHERE id=1`)
	// Ensure content_types page exists
	_, _ = database.DB.ExecContext(context.Background(), `INSERT OR IGNORE INTO content_types (id, display_name, plural_name, hierarchical, public, config_json, created_at, updated_at) VALUES ('page', 'Page', 'Pages', 0, 1, '{}', unixepoch(), unixepoch())`)
	_, _ = database.DB.ExecContext(context.Background(), `INSERT OR IGNORE INTO content_types (id, display_name, plural_name, hierarchical, public, config_json, created_at, updated_at) VALUES ('post', 'Post', 'Posts', 0, 1, '{}', unixepoch(), unixepoch())`)
	_ = os.MkdirAll(filepath.Join(dir, "media"), 0o755)
	return dir, database
}

func TestDoctorHealthyFreshSite(t *testing.T) {
	dir, database := newTestDB(t)
	defer database.Close()
	t.Setenv("STRATUM_REDIRECT_SCHEME", "off")
	t.Setenv("STRATUM_REDIRECT_WWW", "off")
	t.Setenv("STRATUM_TRUST_PROXY", "false")
	t.Setenv("STRATUM_ENV", "")

	opts := Options{DataDir: dir, Production: false}
	report, err := Run(context.Background(), opts)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if report.Status == StatusFail {
		for _, c := range report.Checks {
			t.Logf("%s %s: %s", c.Status, c.Title, c.Message)
		}
		t.Fatalf("expected not FAIL, got %s", report.Status)
	}
	if len(report.Checks) != 13 {
		t.Fatalf("expected 13 checks, got %d", len(report.Checks))
	}
	for _, c := range report.Checks {
		if c.ID == "data_directory" && c.Status != StatusPass {
			t.Fatalf("data directory should pass, got %s: %s", c.Status, c.Message)
		}
		if c.ID == "database" && c.Status != StatusPass {
			t.Fatalf("database should pass, got %s: %s", c.Status, c.Message)
		}
		if c.ID == "site_configuration" && c.Status == StatusFail {
			t.Fatalf("site config should not fail on healthy site: %s", c.Message)
		}
	}
	if code := ExitCode(report, nil); code != 0 {
		t.Fatalf("exit code for healthy should be 0, got %d", code)
	}
}

func TestDoctorDBUnavailable(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("STRATUM_DATA_DIR", dir)
	opts := Options{DataDir: filepath.Join(dir, "nonexistent"), Production: false}
	report, err := Run(context.Background(), opts)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if report.Status != StatusFail {
		t.Fatalf("expected FAIL for missing DB, got %s", report.Status)
	}
	found := false
	for _, c := range report.Checks {
		if c.ID == "database" && c.Status == StatusFail {
			found = true
		}
	}
	if !found {
		t.Fatalf("database check should fail")
	}
	if code := ExitCode(report, nil); code != 1 {
		t.Fatalf("expected exit 1 for failure, got %d", code)
	}
}

func TestDoctorMissingSiteURL(t *testing.T) {
	dir, database := newTestDB(t)
	defer database.Close()
	_, err := database.DB.ExecContext(context.Background(), `UPDATE site_settings SET site_url='' WHERE id=1`)
	if err != nil {
		t.Fatalf("clear site_url: %v", err)
	}
	t.Setenv("STRATUM_REDIRECT_SCHEME", "off")
	t.Setenv("STRATUM_REDIRECT_WWW", "off")
	t.Setenv("STRATUM_TRUST_PROXY", "false")
	opts := Options{DataDir: dir, Production: false}
	report, _ := Run(context.Background(), opts)
	found := false
	for _, c := range report.Checks {
		if c.ID == "site_configuration" && c.Status == StatusWarn {
			if c.Message != "" && containsSubstring([]string{c.Message}, "Site URL is not configured") {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("missing Site URL should be WARN")
	}
	opts.Production = true
	report2, _ := Run(context.Background(), opts)
	foundFail := false
	for _, c := range report2.Checks {
		if c.ID == "site_configuration" && c.Status == StatusFail {
			foundFail = true
		}
	}
	if !foundFail {
		t.Fatalf("missing Site URL in production should be FAIL")
	}
	if code := ExitCode(report2, nil); code != 1 {
		t.Fatalf("expected exit 1 for production missing URL")
	}
}

func TestDoctorBrokenRouteReference(t *testing.T) {
	dir, database := newTestDB(t)
	defer database.Close()
	_, err := database.DB.ExecContext(context.Background(), `INSERT INTO routes (id, path, route_type, redirect_to, redirect_status, created_at, updated_at) VALUES ('test-loop-a', '/a', 'redirect', '/b', 301, unixepoch(), unixepoch())`)
	if err != nil {
		t.Fatalf("insert route: %v", err)
	}
	_, _ = database.DB.ExecContext(context.Background(), `INSERT INTO routes (id, path, route_type, redirect_to, redirect_status, created_at, updated_at) VALUES ('test-loop-b', '/b', 'redirect', '/a', 301, unixepoch(), unixepoch())`)
	_, _ = database.DB.ExecContext(context.Background(), `INSERT INTO routes (id, path, route_type, redirect_to, redirect_status, created_at, updated_at) VALUES ('test-broken', '/broken', 'redirect', '/missing-target', 301, unixepoch(), unixepoch())`)

	opts := Options{DataDir: dir, Production: false}
	report, _ := Run(context.Background(), opts)
	found := false
	for _, c := range report.Checks {
		if c.ID == "routes" && (c.Status == StatusWarn || c.Status == StatusFail) {
			found = true
		}
	}
	if !found {
		t.Fatalf("broken route should cause warning/fail")
	}
}

func TestDoctorSearchDrift(t *testing.T) {
	dir, database := newTestDB(t)
	defer database.Close()
	if _, err := database.DB.ExecContext(context.Background(), `INSERT OR IGNORE INTO entries (id, content_type_id, slug, status, published_revision_id, created_at, updated_at) VALUES ('test-entry-1', 'page', 'test-page', 'active', NULL, unixepoch(), unixepoch())`); err != nil {
		t.Fatalf("insert entry: %v", err)
	}
	if _, err := database.DB.ExecContext(context.Background(), `INSERT OR IGNORE INTO entry_revisions (id, entry_id, revision_number, title, document_json, created_by, created_at) VALUES ('rev-1', 'test-entry-1', 1, 'Test', '{"version":1,"nodes":[]}', NULL, unixepoch())`); err != nil {
		t.Fatalf("insert rev: %v", err)
	}
	if _, err := database.DB.ExecContext(context.Background(), `UPDATE entries SET published_revision_id='rev-1' WHERE id='test-entry-1'`); err != nil {
		t.Fatalf("update entry: %v", err)
	}
	if _, err := database.DB.ExecContext(context.Background(), `INSERT OR IGNORE INTO routes (id, path, entry_id, route_type, created_at, updated_at) VALUES ('route-1', '/test-page', 'test-entry-1', 'entry', unixepoch(), unixepoch())`); err != nil {
		t.Fatalf("insert route: %v", err)
	}
	if _, err := database.DB.ExecContext(context.Background(), `DELETE FROM search_documents`); err != nil {
		t.Fatalf("delete search: %v", err)
	}
	var cnt int
	if err := database.DB.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM entries WHERE status='active' AND published_revision_id IS NOT NULL`).Scan(&cnt); err != nil {
		t.Fatalf("count: %v", err)
	}
	t.Logf("expected count after insert: %d", cnt)

	opts := Options{DataDir: dir, Production: false}
	report, _ := Run(context.Background(), opts)
	found := false
	for _, c := range report.Checks {
		if c.ID == "search" && c.Status == StatusWarn {
			if containsSubstring([]string{c.Message}, "Search index stale") || containsSubstring([]string{c.Message}, "Search index not built") {
				if !containsSubstring([]string{c.Hint}, "stratum search rebuild") {
					t.Fatalf("expected hint rebuild, got %s", c.Hint)
				}
				found = true
			}
		}
	}
	if !found {
		t.Logf("report checks: %+v", report.Checks)
		t.Fatalf("search drift should warn")
	}
}

func TestDoctorMissingMediaBlob(t *testing.T) {
	dir, database := newTestDB(t)
	defer database.Close()
	_, err := database.DB.ExecContext(context.Background(), `INSERT INTO media (id, original_filename, storage_key, mime_type, asset_type, file_size, alt_text, title, caption, description, created_at, updated_at) VALUES ('media-missing', 'missing.jpg', 'missing/missing.jpg', 'image/jpeg', 'image', 1234, '', '', '', '', unixepoch(), unixepoch())`)
	if err != nil {
		t.Fatalf("insert media: %v", err)
	}
	opts := Options{DataDir: dir, Production: false}
	report, _ := Run(context.Background(), opts)
	found := false
	for _, c := range report.Checks {
		if c.ID == "media" && c.Status == StatusWarn {
			found = true
		}
	}
	if !found {
		t.Fatalf("missing media blob should warn")
	}
}

func TestDoctorProductionIndexingDisabled(t *testing.T) {
	dir, database := newTestDB(t)
	defer database.Close()
	_, _ = database.DB.ExecContext(context.Background(), `UPDATE site_settings SET indexing_enabled=0 WHERE id=1`)
	t.Setenv("STRATUM_REDIRECT_SCHEME", "off")
	opts := Options{DataDir: dir, Production: true}
	report, _ := Run(context.Background(), opts)
	found := false
	for _, c := range report.Checks {
		if c.ID == "seo" && c.Status == StatusWarn {
			if containsSubstring([]string{c.Message}, "Indexing disabled") {
				found = true
			}
		}
	}
	if !found {
		t.Fatalf("production indexing disabled should warn")
	}
	if report.Status == StatusFail {
		t.Fatalf("indexing disabled alone should not be FAIL")
	}
}

func TestDoctorJSONOutput(t *testing.T) {
	dir, database := newTestDB(t)
	defer database.Close()
	opts := Options{DataDir: dir, Production: false}
	report, _ := Run(context.Background(), opts)
	data, err := report.JSON()
	if err != nil {
		t.Fatalf("json marshal: %v", err)
	}
	var out map[string]interface{}
	if err := json.Unmarshal(data, &out); err != nil {
		t.Fatalf("json unmarshal: %v", err)
	}
	if out["schemaVersion"] != float64(1) {
		t.Fatalf("expected schemaVersion 1, got %v", out["schemaVersion"])
	}
	if out["status"] == nil {
		t.Fatalf("status missing")
	}
	if out["checks"] == nil {
		t.Fatalf("checks missing")
	}
}

func TestDoctorExitCodes(t *testing.T) {
	dir, database := newTestDB(t)
	defer database.Close()
	opts := Options{DataDir: dir, Production: false}
	report, _ := Run(context.Background(), opts)
	if ExitCode(report, fmt.Errorf("error")) != 2 {
		t.Fatalf("error should be 2")
	}
	report.Status = StatusFail
	if ExitCode(report, nil) != 1 {
		t.Fatalf("expected 1 for fail")
	}
	report.Status = StatusWarn
	if ExitCode(report, nil) != 0 {
		t.Fatalf("warn should be 0")
	}
	report.Status = StatusPass
	if ExitCode(report, nil) != 0 {
		t.Fatalf("pass should be 0")
	}
}

func TestDoctorReadOnly(t *testing.T) {
	dir, database := newTestDB(t)
	defer database.Close()
	var beforeCount int
	_ = database.DB.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM media").Scan(&beforeCount)
	opts := Options{DataDir: dir, Production: false}
	_, _ = Run(context.Background(), opts)
	_, _ = Run(context.Background(), opts)
	var afterCount int
	_ = database.DB.QueryRowContext(context.Background(), "SELECT COUNT(*) FROM media").Scan(&afterCount)
	if beforeCount != afterCount {
		t.Fatalf("doctor mutated DB")
	}
	if _, err := os.Stat(filepath.Join(dir, ".doctor_write_test")); err == nil {
		t.Fatalf("doctor left temp file")
	}
}
