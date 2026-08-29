package doctor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/kokosx/stratum/internal/health"
	"github.com/kokosx/stratum/internal/site"
	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
)

// Options controls doctor run.
type Options struct {
	Production bool
	DataDir    string
}

// Run executes all checks and returns a report. It is read-only.
func Run(ctx context.Context, opts Options) (*Report, error) {
	if opts.DataDir == "" {
		opts.DataDir = os.Getenv("STRATUM_DATA_DIR")
		if opts.DataDir == "" {
			opts.DataDir = "data"
		}
	}
	report := &Report{
		SchemaVersion: 1,
		Production:    opts.Production,
		Checks:        []Check{},
	}
	// Data directory first
	report.Checks = append(report.Checks, checkDataDirectory(opts.DataDir))

	// Database
	dbCheck, queries, database := checkDatabase(ctx, opts.DataDir)
	report.Checks = append(report.Checks, dbCheck)
	if database != nil {
		defer database.Close()
	}

	var settings *db.GetSiteSettingsRow
	var siteURL string
	if queries != nil {
		if s, err := queries.GetSiteSettings(ctx); err == nil {
			settings = &s
			siteURL = s.SiteUrl
		}
	}

	report.Checks = append(report.Checks, checkSiteConfiguration(settings, opts.Production))
	report.Checks = append(report.Checks, checkCanonicalOrigin(siteURL, opts.Production))
	report.Checks = append(report.Checks, checkSEO(settings, opts.Production, queries, ctx))
	report.Checks = append(report.Checks, checkRoutes(ctx, queries, database))
	report.Checks = append(report.Checks, checkMedia(ctx, queries, database, opts.DataDir))
	report.Checks = append(report.Checks, checkSearch(ctx, database))
	report.Checks = append(report.Checks, checkTemplatesSiteParts(ctx, queries, database))
	report.Checks = append(report.Checks, checkForms(ctx, queries))
	report.Checks = append(report.Checks, checkBackup(opts.DataDir, database))
	report.Checks = append(report.Checks, checkSecurityProduction(ctx, queries, settings, siteURL, opts.Production))
	report.Checks = append(report.Checks, checkCustomCode(ctx, queries))

	report.computeStatus()
	return report, nil
}

// ExitCode maps report to CLI exit code.
func ExitCode(report *Report, err error) int {
	if err != nil {
		return 2
	}
	if report == nil {
		return 2
	}
	switch report.Status {
	case StatusFail:
		return 1
	case StatusWarn, StatusPass:
		return 0
	default:
		return 0
	}
}

func checkDatabase(ctx context.Context, dataDir string) (Check, *db.Queries, *storage.Database) {
	check := Check{ID: "database", Title: "Database"}
	dbPath := filepath.Join(dataDir, "stratum.db")
	if _, err := os.Stat(dbPath); err != nil && os.IsNotExist(err) {
		check.Status = StatusFail
		check.Message = "Database file not found"
		check.Details = []string{fmt.Sprintf("Expected at %s", dbPath)}
		check.Hint = "Run stratum migrate or check STRATUM_DATA_DIR"
		return check, nil, nil
	}
	database, err := storage.OpenReadOnly(dbPath)
	if err != nil {
		// fallback try normal open to get better error
		if db2, err2 := storage.Open(dbPath); err2 == nil {
			database = db2
		} else {
			check.Status = StatusFail
			check.Message = fmt.Sprintf("Database open failed: %v", err)
			check.Details = []string{dbPath}
			return check, nil, nil
		}
	}
	if err := database.Ping(ctx); err != nil {
		check.Status = StatusFail
		check.Message = fmt.Sprintf("Database ping failed: %v", err)
		return check, nil, database
	}
	queries := db.New(database.DB)
	var integrity string
	if err := database.DB.QueryRowContext(ctx, "PRAGMA quick_check").Scan(&integrity); err != nil {
		check.Status = StatusFail
		check.Message = fmt.Sprintf("SQLite quick_check failed: %v", err)
		return check, queries, database
	}
	if strings.ToLower(strings.TrimSpace(integrity)) != "ok" {
		check.Status = StatusFail
		check.Message = fmt.Sprintf("SQLite quick_check: %s", integrity)
		return check, queries, database
	}
	var fkEnabled int
	if err := database.DB.QueryRowContext(ctx, "PRAGMA foreign_keys").Scan(&fkEnabled); err == nil {
		if fkEnabled != 1 {
			check.Status = StatusWarn
			check.Message = "Foreign keys not enabled"
			check.Details = []string{"PRAGMA foreign_keys is off; expected ON"}
		}
	}
	if err := checkMigrationsCurrent(ctx, database); err != nil {
		if check.Status == StatusWarn {
			check.Details = append(check.Details, fmt.Sprintf("Migrations: %v", err))
		} else {
			check.Status = StatusFail
			check.Message = fmt.Sprintf("Migrations not current: %v", err)
		}
		return check, queries, database
	}
	var pageCount, pageSize int64
	if err := database.DB.QueryRowContext(ctx, "PRAGMA page_count").Scan(&pageCount); err == nil {
		if err := database.DB.QueryRowContext(ctx, "PRAGMA page_size").Scan(&pageSize); err == nil {
			size := pageCount * pageSize
			if check.Status == StatusWarn {
				check.Details = append(check.Details, fmt.Sprintf("Database size: %s", formatBytes(size)))
			} else {
				check.Status = StatusPass
				check.Message = "SQLite quick check passed"
				check.Details = []string{fmt.Sprintf("Database size: %s", formatBytes(size))}
			}
			return check, queries, database
		}
	}
	if check.Status == StatusWarn {
		return check, queries, database
	}
	check.Status = StatusPass
	check.Message = "SQLite quick check passed"
	return check, queries, database
}

func checkMigrationsCurrent(ctx context.Context, database *storage.Database) error {
	latest := storage.LatestAvailableMigration()
	if latest == "" {
		return nil
	}
	current, err := database.CurrentSchemaVersion(ctx)
	if err != nil {
		return err
	}
	if current == "" {
		return fmt.Errorf("no migrations applied, latest available is %s", latest)
	}
	if storage.CompareMigrationVersions(current, latest) < 0 {
		return fmt.Errorf("current %s behind latest %s", current, latest)
	}
	return nil
}

func formatBytes(b int64) string {
	const unit = 1024
	if b < unit {
		return fmt.Sprintf("%d B", b)
	}
	div, exp := int64(unit), 0
	for n := b / unit; n >= unit; n /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(b)/float64(div), "KMGTPE"[exp])
}

func checkDataDirectory(dataDir string) Check {
	check := Check{ID: "data_directory", Title: "Data directory"}
	info, err := os.Stat(dataDir)
	if err != nil {
		check.Status = StatusFail
		check.Message = fmt.Sprintf("Data directory not found: %s", dataDir)
		check.Hint = "Set STRATUM_DATA_DIR or create data directory"
		return check
	}
	if !info.IsDir() {
		check.Status = StatusFail
		check.Message = fmt.Sprintf("Data path is not a directory: %s", dataDir)
		return check
	}
	if _, err := os.ReadDir(dataDir); err != nil {
		check.Status = StatusFail
		check.Message = fmt.Sprintf("Data directory not readable: %v", err)
		return check
	}
	tmpFile := filepath.Join(dataDir, ".doctor_write_test")
	f, err := os.OpenFile(tmpFile, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0600)
	if err != nil {
		check.Status = StatusFail
		check.Message = fmt.Sprintf("Data directory not writable: %v", err)
		return check
	}
	f.Close()
	os.Remove(tmpFile)

	mediaDir := filepath.Join(dataDir, "media")
	if _, err := os.Stat(mediaDir); os.IsNotExist(err) {
		check.Status = StatusWarn
		check.Message = "Media directory not found"
		check.Details = []string{fmt.Sprintf("Expected at %s", mediaDir)}
		check.Hint = "Media uploads will create it"
		return check
	} else if err != nil {
		check.Status = StatusFail
		check.Message = fmt.Sprintf("Media directory error: %v", err)
		return check
	}
	if _, err := os.ReadDir(mediaDir); err != nil {
		check.Status = StatusWarn
		check.Message = fmt.Sprintf("Media directory not readable: %v", err)
		return check
	}
	tmpFile2 := filepath.Join(mediaDir, ".doctor_write_test")
	f2, err := os.OpenFile(tmpFile2, os.O_CREATE|os.O_WRONLY|os.O_EXCL, 0600)
	if err != nil {
		check.Status = StatusWarn
		check.Message = fmt.Sprintf("Media directory not writable: %v", err)
		return check
	}
	f2.Close()
	os.Remove(tmpFile2)

	details := []string{fmt.Sprintf("Path: %s", dataDir)}
	if free, err := diskFreeBytes(dataDir); err == nil {
		details = append(details, fmt.Sprintf("Disk free: %s", formatBytes(int64(free))))
	}
	check.Status = StatusPass
	check.Message = "Data directory is readable and writable"
	check.Details = details
	return check
}

func checkSiteConfiguration(settings *db.GetSiteSettingsRow, production bool) Check {
	check := Check{ID: "site_configuration", Title: "Site configuration"}
	if settings == nil {
		check.Status = StatusFail
		check.Message = "Cannot load site settings (database unavailable)"
		return check
	}
	var warnings []string
	var details []string
	if strings.TrimSpace(settings.SiteTitle) == "" {
		warnings = append(warnings, "Site title is empty")
	}
	if strings.TrimSpace(settings.SiteUrl) == "" {
		msg := "Site URL is not configured"
		if production {
			check.Status = StatusFail
			check.Message = msg
			check.Details = []string{"Set it in Settings → General. Required in production."}
			check.Action = "Configure Site URL"
			return check
		}
		warnings = append(warnings, msg)
		details = append(details, "Set it in Settings → General")
	} else {
		if _, err := site.ValidateSiteURL(settings.SiteUrl); err != nil {
			warnings = append(warnings, fmt.Sprintf("Site URL invalid: %v", err))
		} else {
			if production && isLocalHostURL(settings.SiteUrl) {
				warnings = append(warnings, "Site URL is localhost in production")
				details = append(details, "Use a public domain for production")
			}
			details = append(details, fmt.Sprintf("Site URL: %s", settings.SiteUrl))
		}
	}
	if err := site.ValidateLanguage(settings.Language); err != nil {
		warnings = append(warnings, fmt.Sprintf("Language invalid: %v", err))
	}
	if err := site.ValidateTimezone(settings.Timezone); err != nil {
		warnings = append(warnings, fmt.Sprintf("Timezone invalid: %v", err))
	}
	if settings.HomepageMode != "" && settings.HomepageMode != "latest_posts" && settings.HomepageMode != "static" && settings.HomepageMode != "page" && settings.HomepageMode != "latest" {
		warnings = append(warnings, fmt.Sprintf("Homepage mode unexpected: %s", settings.HomepageMode))
	}
	if settings.SiteRepresents != "" && settings.SiteRepresents != "organization" && settings.SiteRepresents != "person" {
		warnings = append(warnings, fmt.Sprintf("Site represents unexpected: %s", settings.SiteRepresents))
	}
	details = append(details, fmt.Sprintf("Homepage mode: %s", ifEmpty(settings.HomepageMode, "(unset)")))
	details = append(details, fmt.Sprintf("Language: %s, Timezone: %s", settings.Language, settings.Timezone))
	if len(warnings) > 0 {
		if production && containsSubstring(warnings, "Site URL is not configured") {
			check.Status = StatusFail
		} else {
			check.Status = StatusWarn
		}
		check.Message = strings.Join(warnings, "; ")
		check.Details = details
		if strings.Contains(check.Message, "Site URL is not configured") {
			check.Hint = "Set it in Settings → General"
		}
		return check
	}
	check.Status = StatusPass
	check.Message = "Site configuration looks good"
	check.Details = details
	return check
}

func checkCanonicalOrigin(siteURL string, production bool) Check {
	check := Check{ID: "canonical_origin", Title: "Canonical origin"}
	scheme := strings.TrimSpace(os.Getenv("STRATUM_REDIRECT_SCHEME"))
	if scheme == "" {
		scheme = "off"
	}
	www := strings.TrimSpace(os.Getenv("STRATUM_REDIRECT_WWW"))
	if www == "" {
		www = "off"
	}
	trustProxy := strings.ToLower(strings.TrimSpace(os.Getenv("STRATUM_TRUST_PROXY"))) == "true"
	details := []string{
		fmt.Sprintf("Site URL: %s", ifEmpty(siteURL, "(not set)")),
		fmt.Sprintf("HTTP redirect: %s", scheme),
		fmt.Sprintf("WWW redirect: %s", www),
		fmt.Sprintf("Trust proxy: %v", trustProxy),
	}
	if scheme == "https" && !trustProxy {
		check.Status = StatusWarn
		check.Message = "HTTPS redirect enabled but trust-proxy disabled"
		check.Details = append([]string{"If behind a reverse proxy, enable --trust-proxy to avoid redirect loops"}, details...)
		check.Hint = "If behind a reverse proxy, enable --trust-proxy to avoid redirect loops"
		return check
	}
	if www != "off" && strings.TrimSpace(siteURL) == "" {
		check.Status = StatusWarn
		check.Message = "WWW redirect enabled but Site URL not configured"
		check.Details = details
		return check
	}
	check.Status = StatusPass
	check.Message = "Canonical origin configuration consistent"
	check.Details = details
	return check
}

func checkSEO(settings *db.GetSiteSettingsRow, production bool, queries *db.Queries, ctx context.Context) Check {
	check := Check{ID: "seo", Title: "SEO"}
	if settings == nil {
		check.Status = StatusFail
		check.Message = "Cannot check SEO (database unavailable)"
		return check
	}
	var warnings []string
	var details []string
	if settings.IndexingEnabled == 0 {
		msg := "Indexing disabled (noindex)"
		if production {
			warnings = append(warnings, msg+" - may be intentional for staging/sample sites")
			details = append(details, "Site asks search engines not to index it")
		} else {
			warnings = append(warnings, msg)
		}
	} else {
		details = append(details, "Indexing enabled")
	}
	if settings.SitemapEnabled == 0 {
		warnings = append(warnings, "Sitemap disabled")
	} else {
		if strings.TrimSpace(settings.SiteUrl) == "" {
			warnings = append(warnings, "Sitemap enabled without Site URL")
		} else {
			details = append(details, "Sitemap enabled at /sitemap.xml")
		}
	}
	if strings.TrimSpace(settings.RobotsMode) != "" {
		details = append(details, fmt.Sprintf("Robots mode: %s", settings.RobotsMode))
	}
	if settings.SiteSocialMediaID.Valid && settings.SiteSocialMediaID.String != "" {
		if queries != nil {
			if _, err := queries.GetMedia(ctx, settings.SiteSocialMediaID.String); err != nil {
				warnings = append(warnings, "Social image references missing media")
			} else {
				details = append(details, "Social image configured")
			}
		}
	} else {
		details = append(details, "Social image not set")
	}
	if settings.SiteUrl == "" {
		// already warned in site config, but also for SEO
		if !containsSubstring(warnings, "Sitemap enabled without") {
			// avoid duplicate
		}
	}
	if len(warnings) > 0 {
		// Indexing disabled alone is WARN even in production (may be intentional)
		check.Status = StatusWarn
		check.Message = strings.Join(warnings, "; ")
		check.Details = details
		if containsSubstring(warnings, "Indexing disabled") {
			check.Hint = "If this is production, enable indexing in Settings → SEO"
		}
		return check
	}
	check.Status = StatusPass
	check.Message = "SEO settings look good"
	check.Details = details
	return check
}

func checkRoutes(ctx context.Context, queries *db.Queries, database *storage.Database) Check {
	check := Check{ID: "routes", Title: "Routes"}
	if queries == nil || database == nil {
		check.Status = StatusFail
		check.Message = "Cannot check routes (database unavailable)"
		return check
	}
	svc := health.New(database.DB, queries)
	results, _, err := svc.Run(ctx)
	if err != nil {
		check.Status = StatusFail
		check.Message = fmt.Sprintf("Health check failed: %v", err)
		return check
	}
	var warnings []string
	var fails []string
	for _, r := range results {
		if strings.HasPrefix(r.ID, "homepage") || strings.HasPrefix(r.ID, "posts_page") || strings.HasPrefix(r.ID, "redirect_") {
			if r.Severity == health.SeverityCritical {
				fails = append(fails, r.Title+": "+r.Description)
			} else if r.Severity == health.SeverityWarning {
				warnings = append(warnings, r.Title+": "+r.Description)
			}
		}
	}
	if len(fails) > 0 {
		check.Status = StatusFail
		check.Message = strings.Join(fails, "; ")
		if len(warnings) > 0 {
			check.Details = warnings
		}
		return check
	}
	if len(warnings) > 0 {
		check.Status = StatusWarn
		check.Message = strings.Join(warnings, "; ")
		return check
	}
	check.Status = StatusPass
	check.Message = "No redirect loops or missing routes"
	return check
}

func checkMedia(ctx context.Context, queries *db.Queries, database *storage.Database, dataDir string) Check {
	check := Check{ID: "media", Title: "Media"}
	if queries == nil || database == nil {
		check.Status = StatusFail
		check.Message = "Cannot check media (database unavailable)"
		return check
	}
	var count int
	if err := database.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM media").Scan(&count); err != nil {
		check.Status = StatusFail
		check.Message = fmt.Sprintf("Media count query failed: %v", err)
		return check
	}
	if count == 0 {
		check.Status = StatusPass
		check.Message = "No media assets"
		return check
	}
	rows, err := database.DB.QueryContext(ctx, "SELECT id, storage_key FROM media ORDER BY created_at DESC LIMIT 50")
	if err != nil {
		check.Status = StatusFail
		check.Message = fmt.Sprintf("Media query failed: %v", err)
		return check
	}
	defer rows.Close()
	missing := 0
	checked := 0
	for rows.Next() {
		var id, key string
		if err := rows.Scan(&id, &key); err != nil {
			continue
		}
		checked++
		path := filepath.Join(dataDir, "media", filepath.FromSlash(key))
		if _, err := os.Stat(path); err != nil {
			missing++
		}
	}
	details := []string{fmt.Sprintf("%d assets checked", checked)}
	if missing > 0 {
		check.Status = StatusWarn
		check.Message = fmt.Sprintf("%d of %d media blobs missing", missing, checked)
		check.Details = details
		check.Hint = "Some media files are missing from disk"
		return check
	}
	// favicon/social check
	if icon, err := queries.GetSiteIconMediaID(ctx); err == nil && icon.Valid && icon.String != "" {
		if _, err := queries.GetMedia(ctx, icon.String); err != nil {
			check.Status = StatusWarn
			check.Message = "Favicon references missing media"
			check.Details = details
			return check
		}
	}
	if social, err := queries.GetSiteSocialMediaID(ctx); err == nil && social.Valid && social.String != "" {
		if _, err := queries.GetMedia(ctx, social.String); err != nil {
			check.Status = StatusWarn
			check.Message = "Social image references missing media"
			check.Details = details
			return check
		}
	}
	check.Status = StatusPass
	check.Message = fmt.Sprintf("%d assets checked", checked)
	check.Details = details
	return check
}

func checkSearch(ctx context.Context, database *storage.Database) Check {
	check := Check{ID: "search", Title: "Search"}
	if database == nil {
		check.Status = StatusFail
		check.Message = "Cannot check search (database unavailable)"
		return check
	}
	var expected int
	if err := database.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM entries WHERE status='active' AND published_revision_id IS NOT NULL").Scan(&expected); err != nil {
		check.Status = StatusFail
		check.Message = fmt.Sprintf("Search expected count failed: %v", err)
		return check
	}
	var actual int
	if err := database.DB.QueryRowContext(ctx, "SELECT COUNT(*) FROM search_documents").Scan(&actual); err != nil {
		if strings.Contains(err.Error(), "no such table") {
			if expected == 0 {
				check.Status = StatusPass
				check.Message = "Search index empty (no public content)"
				return check
			}
			check.Status = StatusWarn
			check.Message = "Search index not built"
			check.Hint = "Run: stratum search rebuild"
			return check
		}
		check.Status = StatusFail
		check.Message = fmt.Sprintf("Search actual count failed: %v", err)
		return check
	}
	if expected != actual {
		diff := expected - actual
		if diff < 0 {
			diff = -diff
		}
		check.Status = StatusWarn
		check.Message = fmt.Sprintf("Search index stale: %d documents differ (expected %d, got %d)", diff, expected, actual)
		check.Hint = "Run: stratum search rebuild"
		check.Details = []string{fmt.Sprintf("Expected %d, actual %d", expected, actual)}
		return check
	}
	check.Status = StatusPass
	check.Message = "Search index in sync"
	check.Details = []string{fmt.Sprintf("%d documents", actual)}
	return check
}

func checkTemplatesSiteParts(ctx context.Context, queries *db.Queries, database *storage.Database) Check {
	check := Check{ID: "templates", Title: "Templates / Site Parts"}
	if queries == nil || database == nil {
		check.Status = StatusFail
		check.Message = "Cannot check templates (database unavailable)"
		return check
	}
	svc := health.New(database.DB, queries)
	results, _, err := svc.Run(ctx)
	if err != nil {
		check.Status = StatusFail
		check.Message = fmt.Sprintf("Health check failed: %v", err)
		return check
	}
	var warnings []string
	var fails []string
	for _, r := range results {
		if strings.HasPrefix(r.ID, "default_template") || strings.HasPrefix(r.ID, "integrity_") {
			if r.Severity == health.SeverityCritical {
				fails = append(fails, r.Title)
			} else if r.Severity == health.SeverityWarning {
				if strings.Contains(r.ID, "template") || strings.Contains(r.ID, "sitePart") {
					warnings = append(warnings, r.Title+": "+r.Description)
				}
			}
		}
	}
	if len(fails) > 0 {
		check.Status = StatusFail
		check.Message = strings.Join(fails, "; ")
		return check
	}
	if len(warnings) > 0 {
		check.Status = StatusWarn
		check.Message = strings.Join(warnings, "; ")
		return check
	}
	check.Status = StatusPass
	check.Message = "Templates and site parts look good"
	return check
}

func checkForms(ctx context.Context, queries *db.Queries) Check {
	check := Check{ID: "forms", Title: "Forms"}
	if queries == nil {
		check.Status = StatusFail
		check.Message = "Cannot check forms (database unavailable)"
		return check
	}
	var formCount int
	if err := queries.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM forms").Scan(&formCount); err != nil {
		if strings.Contains(err.Error(), "no such table") {
			check.Status = StatusPass
			check.Message = "No forms configured"
			return check
		}
		check.Status = StatusFail
		check.Message = fmt.Sprintf("Forms count failed: %v", err)
		return check
	}
	if formCount == 0 {
		check.Status = StatusPass
		check.Message = "No forms configured"
		return check
	}
	details := []string{fmt.Sprintf("%d forms", formCount)}
	smtpHost := os.Getenv("SMTP_HOST")
	smtpUser := os.Getenv("SMTP_USER")
	hasSMTP := smtpHost != "" && smtpUser != ""
	if !hasSMTP {
		var withNotify int
		if err := queries.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM forms WHERE notify_email IS NOT NULL AND notify_email != ''").Scan(&withNotify); err == nil && withNotify > 0 {
			check.Status = StatusWarn
			check.Message = fmt.Sprintf("%d forms with email notifications but SMTP not configured", withNotify)
			check.Details = details
			check.Hint = "Set SMTP_HOST/SMTP_USER for email delivery"
			return check
		}
		details = append(details, "SMTP not configured (not required)")
	}
	check.Status = StatusPass
	check.Message = fmt.Sprintf("%d forms", formCount)
	check.Details = details
	return check
}

func checkBackup(dataDir string, database *storage.Database) Check {
	check := Check{ID: "backup", Title: "Backup"}
	dbPath := filepath.Join(dataDir, "stratum.db")
	if _, err := os.Stat(dbPath); err != nil {
		check.Status = StatusWarn
		check.Message = "Database file not found, cannot create backup"
		return check
	}
	candidates := []string{dataDir, ".", filepath.Join(dataDir, "backups")}
	var newest string
	var newestTime int64
	for _, dir := range candidates {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			name := e.Name()
			if strings.HasPrefix(name, "stratum-backup-") && strings.HasSuffix(name, ".zip") {
				info, err := e.Info()
				if err != nil {
					continue
				}
				mod := info.ModTime().Unix()
				if mod > newestTime {
					newestTime = mod
					newest = filepath.Join(dir, name)
				}
			}
		}
	}
	if newest != "" {
		age := time.Now().Unix() - newestTime
		days := age / 86400
		hours := (age % 86400) / 3600
		var ageStr string
		if days > 0 {
			ageStr = fmt.Sprintf("%d days ago", days)
		} else if hours > 0 {
			ageStr = fmt.Sprintf("%d hours ago", hours)
		} else {
			ageStr = fmt.Sprintf("%d minutes ago", age/60)
		}
		check.Status = StatusPass
		check.Message = fmt.Sprintf("Most recent backup: %s (%s)", newest, ageStr)
		return check
	}
	check.Status = StatusPass
	check.Message = "Backup prerequisites valid"
	check.Details = []string{"No backup history found; run stratum backup create"}
	return check
}

func checkSecurityProduction(ctx context.Context, queries *db.Queries, settings *db.GetSiteSettingsRow, siteURL string, production bool) Check {
	check := Check{ID: "security", Title: "Security / Production"}
	env := strings.TrimSpace(os.Getenv("STRATUM_ENV"))
	var warnings []string
	var details []string
	if production {
		if env != "production" {
			warnings = append(warnings, "STRATUM_ENV is not 'production'")
			details = append(details, fmt.Sprintf("Current: %q", env))
		} else {
			details = append(details, "STRATUM_ENV=production")
		}
		if queries != nil {
			var userCount int
			if err := queries.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM users").Scan(&userCount); err == nil {
				if userCount == 0 {
					warnings = append(warnings, "No admin users (setup not completed)")
				} else {
					details = append(details, fmt.Sprintf("%d admin users", userCount))
				}
			}
		}
		if isLocalHostURL(siteURL) {
			warnings = append(warnings, "Canonical origin is localhost in production")
		}
		if strings.TrimSpace(siteURL) == "" {
			warnings = append(warnings, "Site URL not configured for production")
		}
	} else {
		details = append(details, fmt.Sprintf("STRATUM_ENV=%q", ifEmpty(env, "(not set)")))
		if siteURL != "" {
			details = append(details, fmt.Sprintf("Site URL: %s", siteURL))
		}
	}
	if len(warnings) > 0 {
		check.Status = StatusWarn
		check.Message = strings.Join(warnings, "; ")
		check.Details = details
		return check
	}
	check.Status = StatusPass
	check.Message = "Security checks passed"
	check.Details = details
	return check
}

func checkCustomCode(ctx context.Context, queries *db.Queries) Check {
	check := Check{ID: "custom_code", Title: "Custom Code"}
	if queries == nil {
		check.Status = StatusFail
		check.Message = "Cannot check custom code (database unavailable)"
		return check
	}
	var globalCount, templateCount int
	if err := queries.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM custom_code_snippets WHERE enabled=1 AND scope='global'").Scan(&globalCount); err != nil {
		if strings.Contains(err.Error(), "no such table") {
			check.Status = StatusPass
			check.Message = "No custom code"
			return check
		}
		check.Status = StatusFail
		check.Message = fmt.Sprintf("Custom code query failed: %v", err)
		return check
	}
	if err := queries.DB().QueryRowContext(ctx, "SELECT COUNT(*) FROM custom_code_snippets WHERE enabled=1 AND scope='template'").Scan(&templateCount); err != nil {
		templateCount = 0
	}
	total := globalCount + templateCount
	if total == 0 {
		check.Status = StatusPass
		check.Message = "No custom code"
		return check
	}
	check.Status = StatusPass
	check.Message = fmt.Sprintf("%d global, %d template snippets enabled", globalCount, templateCount)
	check.Details = []string{fmt.Sprintf("Total %d enabled snippets", total)}
	return check
}

// Helpers

func ifEmpty(s, fallback string) string {
	if strings.TrimSpace(s) == "" {
		return fallback
	}
	return s
}

func containsSubstring(list []string, substr string) bool {
	for _, s := range list {
		if strings.Contains(s, substr) {
			return true
		}
	}
	return false
}

func isLocalHostURL(u string) bool {
	u = strings.ToLower(strings.TrimSpace(u))
	return strings.Contains(u, "localhost") || strings.Contains(u, "127.0.0.1") || strings.Contains(u, "::1")
}
