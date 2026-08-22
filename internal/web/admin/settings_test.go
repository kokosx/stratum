package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/themes"
)

func newSettingsHandler(t *testing.T) (*Handler, *db.Queries) {
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
	if err := database.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	queries := db.New(database.DB)
	registry, err := blocks.NewRegistry(ctx, queries)
	if err != nil {
		t.Fatal(err)
	}
	themeRuntime, err := themes.NewRuntime(ctx, queries)
	if err != nil {
		t.Fatal(err)
	}
	h, err := NewHandler(database.DB, queries, nil, registry, themeRuntime, newTestMedia(t, queries))
	if err != nil {
		t.Fatal(err)
	}
	return h, queries
}

func TestSaveGeneralSettings(t *testing.T) {
	h, queries := newSettingsHandler(t)
	form := settingsForm{
		SiteTitle:            "My Site",
		Tagline:              "A tagline",
		SiteURL:              "https://example.com/",
		Language:             "pl",
		Timezone:             "Europe/Warsaw",
		IndexingEnabled:      true,
		SitemapEnabled:       true,
		RobotsMode:           "managed",
		SpeculationEnabled:   false,
		SpeculationMode:      "off",
		SpeculationEagerness: "conservative",
		TitleSeparator:       "–",
	}
	if err := h.persistSettings(context.Background(), mustCurrent(t, queries), form); err != nil {
		t.Fatal(err)
	}
	row, err := queries.GetSiteSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if row.SiteTitle != "My Site" {
		t.Fatalf("site title = %q", row.SiteTitle)
	}
	if row.SiteUrl != "https://example.com" {
		t.Fatalf("site url not normalized: %q", row.SiteUrl)
	}
	if row.Language != "pl" || row.Timezone != "Europe/Warsaw" {
		t.Fatalf("language/timezone not saved: %q %q", row.Language, row.Timezone)
	}
	if row.IndexingEnabled != 1 || row.SitemapEnabled != 1 {
		t.Fatalf("toggles not saved")
	}
}

func TestParseSettingsRejectsInvalidSiteURL(t *testing.T) {
	h, _ := newSettingsHandler(t)
	req := formPost(t, map[string]string{
		"site_title": "x", "site_url": "https://example.com/?q=1",
		"language": "en", "timezone": "UTC", "robots_mode": "managed",
		"speculation_mode": "off", "speculation_eagerness": "conservative",
	})
	_, errors := h.parseSettingsForm(req)
	if _, ok := errors["site_url"]; !ok {
		t.Fatalf("expected site_url error, got %#v", errors)
	}
}

func TestParseSettingsAcceptsValidTimezone(t *testing.T) {
	h, _ := newSettingsHandler(t)
	req := formPost(t, map[string]string{
		"site_title": "x", "site_url": "https://example.com",
		"language": "en-US", "timezone": "America/New_York", "robots_mode": "managed",
		"speculation_mode": "off", "speculation_eagerness": "conservative",
	})
	_, errors := h.parseSettingsForm(req)
	if _, ok := errors["timezone"]; ok {
		t.Fatalf("valid timezone should be accepted: %#v", errors)
	}
}

func TestParseSettingsRejectsInvalidTimezone(t *testing.T) {
	h, _ := newSettingsHandler(t)
	req := formPost(t, map[string]string{
		"site_title": "x", "site_url": "https://example.com",
		"language": "en", "timezone": "Mars/Phobos", "robots_mode": "managed",
		"speculation_mode": "off", "speculation_eagerness": "conservative",
	})
	_, errors := h.parseSettingsForm(req)
	if _, ok := errors["timezone"]; !ok {
		t.Fatalf("invalid timezone should be rejected, got %#v", errors)
	}
}

func TestParseSettingsHomepageMustBePage(t *testing.T) {
	h, queries := newSettingsHandler(t)
	ctx := context.Background()
	// Create a Post (not a Page) to use as an invalid homepage.
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: "a-post", ContentTypeID: "post", Slug: "a-post", Status: "active", CreatedAt: 1, UpdatedAt: 1}); err != nil {
		t.Fatal(err)
	}
	req := formPost(t, map[string]string{
		"site_title": "x", "site_url": "https://example.com",
		"language": "en", "timezone": "UTC", "robots_mode": "managed",
		"homepage_entry_id": "a-post",
		"speculation_mode":  "off", "speculation_eagerness": "conservative",
	})
	_, errors := h.parseSettingsForm(req)
	if _, ok := errors["homepage_entry_id"]; !ok {
		t.Fatalf("non-page homepage should be rejected, got %#v", errors)
	}
}

func TestSettingsSaveIsAtomic(t *testing.T) {
	h, queries := newSettingsHandler(t)
	ctx := context.Background()
	// A valid save updates both settings and the homepage route in one shot.
	form := settingsForm{
		SiteTitle: "Atomic", SiteURL: "https://example.com",
		Language: "en", Timezone: "UTC", RobotsMode: "managed",
		IndexingEnabled: true, SitemapEnabled: true,
		HomepageEntryID: "seed-about", SpeculationMode: "off", SpeculationEagerness: "conservative",
	}
	if err := h.persistSettings(context.Background(), mustCurrent(t, queries), form); err != nil {
		t.Fatal(err)
	}
	row, _ := queries.GetSiteSettings(ctx)
	if row.HomepageEntryID.String != "seed-about" {
		t.Fatalf("homepage not saved: %q", row.HomepageEntryID.String)
	}
	// The previously-homepage page (seed-home, slug "home") must now be served
	// at /home, and the new homepage (seed-about) at "/".
	about, err := queries.GetPublishedEntryByPath(ctx, "/home")
	if err != nil || about.ID != "seed-home" {
		t.Fatalf("old homepage did not move to /home: err=%v id=%q", err, about.ID)
	}
	home, err := queries.GetPublishedEntryByPath(ctx, "/")
	if err != nil || home.ID != "seed-about" {
		t.Fatalf("new homepage not at /: err=%v id=%q", err, home.ID)
	}

	// An invalid form must not mutate anything: re-read and confirm unchanged
	// beyond what we set (title still "Atomic").
	bad := settingsForm{SiteTitle: "Bad", SiteURL: "not-a-url", Language: "en", Timezone: "UTC", RobotsMode: "managed", SpeculationMode: "off", SpeculationEagerness: "conservative"}
	_ = bad
	_, errors := h.parseSettingsForm(formPost(t, map[string]string{
		"site_title": "Bad", "site_url": "not-a-url", "language": "en", "timezone": "UTC",
		"robots_mode": "managed", "speculation_mode": "off", "speculation_eagerness": "conservative",
	}))
	if len(errors) == 0 {
		t.Fatal("expected validation errors")
	}
	row2, _ := queries.GetSiteSettings(ctx)
	if row2.SiteTitle != "Atomic" {
		t.Fatalf("invalid save mutated settings: %q", row2.SiteTitle)
	}
}

// TestSettingsHomepageKeepsPageURL verifies that a page which becomes the
// homepage stays reachable at its own slug (via a 301 redirect to "/") instead
// of silently losing its public URL.
func TestSettingsHomepageKeepsPageURL(t *testing.T) {
	h, queries := newSettingsHandler(t)
	ctx := context.Background()
	// seed-home is the initial homepage at "/"; seed-about lives at /about.
	form := settingsForm{
		SiteTitle: "Atomic", SiteURL: "https://example.com",
		Language: "en", Timezone: "UTC", RobotsMode: "managed",
		IndexingEnabled: true, SitemapEnabled: true,
		HomepageEntryID: "seed-about", SpeculationMode: "off", SpeculationEagerness: "conservative",
	}
	if err := h.persistSettings(ctx, mustCurrent(t, queries), form); err != nil {
		t.Fatal(err)
	}
	if entry, err := queries.GetPublishedEntryByPath(ctx, "/"); err != nil || entry.ID != "seed-about" {
		t.Fatalf("/ should serve seed-about: err=%v id=%q", err, entry.ID)
	}
	// The page's own URL must still work: /about is now a 301 redirect to "/".
	route, err := queries.GetRouteByPath(ctx, "/about")
	if err != nil || route.RouteType != "redirect" || !route.RedirectTo.Valid || route.RedirectTo.String != "/" {
		t.Fatalf("/about should be a 301 redirect to /: err=%v route=%+v", err, route)
	}
	// The previous homepage got its slug route back with no stale redirect left
	// at /home.
	if entry, err := queries.GetPublishedEntryByPath(ctx, "/home"); err != nil || entry.ID != "seed-home" {
		t.Fatalf("/home should serve seed-home: err=%v id=%q", err, entry.ID)
	}
}

// TestSettingsClearHomepageRestoresPageURL verifies that removing the homepage
// deletes the "/" route, clears the slug redirect and restores the page's own
// entry route at /<slug>.
func TestSettingsClearHomepageRestoresPageURL(t *testing.T) {
	h, queries := newSettingsHandler(t)
	ctx := context.Background()
	set := settingsForm{
		SiteTitle: "Atomic", SiteURL: "https://example.com",
		Language: "en", Timezone: "UTC", RobotsMode: "managed",
		IndexingEnabled: true, SitemapEnabled: true,
		HomepageEntryID: "seed-about", SpeculationMode: "off", SpeculationEagerness: "conservative",
	}
	if err := h.persistSettings(ctx, mustCurrent(t, queries), set); err != nil {
		t.Fatal(err)
	}
	clear := set
	clear.HomepageEntryID = ""
	if err := h.persistSettings(ctx, mustCurrent(t, queries), clear); err != nil {
		t.Fatal(err)
	}
	// seed-about is back at its own entry route, not a redirect.
	if entry, err := queries.GetPublishedEntryByPath(ctx, "/about"); err != nil || entry.ID != "seed-about" {
		t.Fatalf("/about should serve seed-about again: err=%v id=%q", err, entry.ID)
	}
	route, err := queries.GetRouteByPath(ctx, "/about")
	if err != nil || route.RouteType != "entry" || !route.EntryID.Valid || route.EntryID.String != "seed-about" {
		t.Fatalf("/about should be an entry route owned by seed-about: err=%v route=%+v", err, route)
	}
	// No stale homepage route remains.
	if _, err := queries.GetRouteByPath(ctx, "/"); err == nil {
		t.Fatal("stale homepage route at / was not removed")
	}
}

// TestDatastarSettingsSaveReturnsFragment verifies the save responds with a
// Datastar SSE patch (no full page reload) and echoes the saved values.
func TestDatastarSettingsSaveReturnsFragment(t *testing.T) {
	h, _ := newSettingsHandler(t)
	values := map[string]string{
		"site_title": "Datastar Site", "site_url": "https://example.com",
		"language": "en", "timezone": "UTC", "robots_mode": "managed",
		"indexing_enabled": "on", "sitemap_enabled": "on",
		"speculation_mode": "off", "speculation_eagerness": "conservative",
	}
	rec, req := authedPost(t, h, values)
	req.Header.Set("Datastar-Request", "true")
	h.saveSettings(rec, req)
	if rec.Header().Get("Content-Type") != "text/event-stream" {
		t.Fatalf("expected text/event-stream, got %q body=%s", rec.Header().Get("Content-Type"), rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "event: datastar-patch-elements") {
		t.Fatalf("expected datastar patch event, got:\n%s", body)
	}
	if !strings.Contains(body, "Datastar Site") {
		t.Fatalf("fragment should contain saved site title:\n%s", body)
	}
	if !strings.Contains(body, "Settings saved.") {
		t.Fatalf("fragment should contain success toast:\n%s", body)
	}
}

// authedPost builds a POST request carrying a valid CSRF cookie + token without
// relying on h.csrfToken (which needs an auth service). validCSRF only compares
// the cookie value with the submitted token, so a self-minted matching pair works.
func authedPost(t *testing.T, h *Handler, values map[string]string) (*httptest.ResponseRecorder, *http.Request) {
	t.Helper()
	token := "test-csrf-token-value"
	values["csrf_token"] = token
	body := make([]string, 0, len(values))
	for k, v := range values {
		body = append(body, k+"="+urlEncode(v))
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/settings", strings.NewReader(strings.Join(body, "&")))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "stratum_csrf", Value: token, Path: "/admin"})
	return httptest.NewRecorder(), req
}

func mustCurrent(t *testing.T, queries *db.Queries) db.GetSiteSettingsRow {
	t.Helper()
	row, err := queries.GetSiteSettings(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return row
}

func formPost(t *testing.T, values map[string]string) *http.Request {
	t.Helper()
	body := make([]string, 0, len(values))
	for k, v := range values {
		body = append(body, k+"="+urlEncode(v))
	}
	req := httptest.NewRequest(http.MethodPost, "/admin/settings", strings.NewReader(strings.Join(body, "&")))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return req
}

func urlEncode(value string) string {
	return strings.NewReplacer(" ", "+", "&", "%26", "=", "%3D", "?", "%3F", "#", "%23").Replace(value)
}
