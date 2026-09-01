package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kokosx/stratum/internal/analytics"
	"github.com/kokosx/stratum/internal/auth"
	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/media"
	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/themes"
)

func newAnalyticsHandler(t *testing.T) (*Handler, *auth.Service, *analytics.Service) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	database, err := storage.Open(filepath.Join(dir, "stratum.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	queries := db.New(database.DB)
	service, err := auth.NewService(database.DB, queries, false)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := blocks.NewRegistry(ctx, queries)
	if err != nil {
		t.Fatal(err)
	}
	themeRuntime, err := themes.NewRuntime(ctx, queries)
	if err != nil {
		t.Fatal(err)
	}
	store, err := media.NewLocalStorage(filepath.Join(dir, "media"))
	if err != nil {
		t.Fatal(err)
	}
	mediaSvc := media.NewService(queries, store)
	h, err := NewHandler(database.DB, queries, service, registry, themeRuntime, mediaSvc)
	if err != nil {
		t.Fatal(err)
	}
	// setup admin
	code := service.SetupCode()
	if code != "" {
		_, _ = service.Setup(ctx, code, "Test Site", "admin@example.com", "password123456")
	}
	// create analytics service
	analyticsSvc := analytics.New(database.DB, h.runtime.Site)
	h.SetAnalytics(analyticsSvc)
	t.Cleanup(func() { _ = analyticsSvc.Close() })
	return h, service, analyticsSvc
}

func loginCookie(t *testing.T, svc *auth.Service) *http.Cookie {
	t.Helper()
	token, err := svc.Login(context.Background(), "admin@example.com", "password123456")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	return &http.Cookie{Name: auth.CookieName, Value: token}
}

func loginAs(t *testing.T, h *Handler, svc *auth.Service, email, password, role string) *http.Cookie {
	t.Helper()
	ctx := context.Background()
	// create user with role if not exists
	_ = svc.CreateUser(ctx, email, password, role)
	token, err := svc.Login(ctx, email, password)
	if err != nil {
		t.Fatalf("login %s: %v", email, err)
	}
	return &http.Cookie{Name: auth.CookieName, Value: token}
}

func TestAnalytics_UnauthenticatedRedirectsToLogin(t *testing.T) {
	h, _, _ := newAnalyticsHandler(t)
	req := httptest.NewRequest(http.MethodGet, "/admin/analytics", nil)
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusSeeOther && rec.Code != http.StatusFound {
		t.Fatalf("unauth should redirect to login, got %d", rec.Code)
	}
	if loc := rec.Header().Get("Location"); !strings.Contains(loc, "/admin/login") {
		t.Fatalf("redirect loc %q", loc)
	}
}

func TestAnalytics_NonAdminForbidden(t *testing.T) {
	h, svc, _ := newAnalyticsHandler(t)
	cookie := loginAs(t, h, svc, "author@example.com", "password123456", "author")
	req := httptest.NewRequest(http.MethodGet, "/admin/analytics", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("author should be forbidden, got %d body %s", rec.Code, rec.Body.String())
	}
}

func TestAnalytics_AdminCanAccess(t *testing.T) {
	h, svc, _ := newAnalyticsHandler(t)
	cookie := loginCookie(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/admin/analytics", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("admin should get 200, got %d body %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Analytics") {
		t.Fatalf("body missing Analytics title")
	}
}

func TestAnalytics_NavAppearsAfterDashboard(t *testing.T) {
	h, svc, _ := newAnalyticsHandler(t)
	cookie := loginCookie(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	body := rec.Body.String()
	idxDash := strings.Index(body, "Dashboard")
	idxAnalytics := strings.Index(body, "Analytics")
	idxPages := strings.Index(body, "Pages")
	if idxDash == -1 || idxAnalytics == -1 || idxPages == -1 {
		t.Fatalf("nav missing items %d %d %d", idxDash, idxAnalytics, idxPages)
	}
	if !(idxDash < idxAnalytics && idxAnalytics < idxPages) {
		t.Fatalf("nav order wrong: Dashboard %d Analytics %d Pages %d", idxDash, idxAnalytics, idxPages)
	}
}

func TestAnalytics_ActiveNavForSubroutes(t *testing.T) {
	cases := []string{"/admin/analytics", "/admin/analytics/content", "/admin/analytics/acquisition", "/admin/analytics/technology", "/admin/analytics/crawlers", "/admin/analytics/performance", "/admin/analytics/settings"}
	for _, path := range cases {
		state := ResolveNav(path)
		if state.ActiveSection != "analytics" {
			t.Fatalf("path %s activeSection %q want analytics", path, state.ActiveSection)
		}
	}
}

func TestAnalytics_EmptyState(t *testing.T) {
	h, svc, _ := newAnalyticsHandler(t)
	cookie := loginCookie(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/admin/analytics", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, "No analytics yet") {
		t.Fatalf("empty state missing, got %.500s", body)
	}
}

func TestAnalytics_RangeValidation(t *testing.T) {
	h, svc, _ := newAnalyticsHandler(t)
	cookie := loginCookie(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/admin/analytics?range=invalid", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("invalid range should fallback to 200, got %d", rec.Code)
	}
	// Should not contain error, just default to 30d
	body := rec.Body.String()
	if !strings.Contains(body, "Overview") {
		t.Fatalf("body missing overview after invalid range")
	}
}

func TestAnalytics_DefaultEnabled(t *testing.T) {
	h, _, _ := newAnalyticsHandler(t)
	ctx := context.Background()
	row, err := h.queries.GetSiteSettings(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if row.AnalyticsEnabled != 1 {
		t.Fatalf("analytics_enabled should be 1 by default, got %d", row.AnalyticsEnabled)
	}
	if row.AnalyticsRetentionDays != 730 {
		t.Fatalf("retention default 730 got %d", row.AnalyticsRetentionDays)
	}
	if row.AnalyticsHourlyRetentionDays != 90 {
		t.Fatalf("hourly retention default 90 got %d", row.AnalyticsHourlyRetentionDays)
	}
	snap := h.runtime.Site.Current()
	if snap == nil || !snap.AnalyticsEnabled {
		t.Fatal("runtime snapshot should be enabled")
	}
}

func TestAnalytics_DisableUpdatesRuntime(t *testing.T) {
	h, svc, _ := newAnalyticsHandler(t)
	cookie := loginCookie(t, svc)
	// Generate CSRF via handler helper (as in admin_interaction_test.go)
	csrfRec := httptest.NewRecorder()
	csrfReq := httptest.NewRequest(http.MethodGet, "/admin/analytics/settings", nil)
	csrfReq.AddCookie(cookie)
	csrfToken, _ := h.csrfToken(csrfRec, csrfReq)
	// POST disabling (unchecked -> disabled)
	form := url.Values{
		"csrf_token":                      {csrfToken},
		"analytics_retention_days":        {"730"},
		"analytics_hourly_retention_days": {"90"},
	}
	req2 := httptest.NewRequest(http.MethodPost, "/admin/analytics/settings", strings.NewReader(form.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range csrfRec.Result().Cookies() {
		req2.AddCookie(c)
	}
	req2.AddCookie(cookie)
	req2.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrfToken})
	rec2 := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusSeeOther && rec2.Code != http.StatusOK {
		t.Fatalf("disable should redirect, got %d body %s", rec2.Code, rec2.Body.String())
	}
	snap := h.runtime.Site.Current()
	if snap.AnalyticsEnabled {
		t.Fatal("runtime should be disabled after save")
	}
	// Re-enable with new token
	csrfRec2 := httptest.NewRecorder()
	csrfReq2 := httptest.NewRequest(http.MethodGet, "/admin/analytics/settings", nil)
	csrfReq2.AddCookie(cookie)
	csrfToken2, _ := h.csrfToken(csrfRec2, csrfReq2)
	form2 := url.Values{
		"csrf_token":                      {csrfToken2},
		"analytics_enabled":               {"1"},
		"analytics_retention_days":        {"365"},
		"analytics_hourly_retention_days": {"30"},
	}
	req4 := httptest.NewRequest(http.MethodPost, "/admin/analytics/settings", strings.NewReader(form2.Encode()))
	req4.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range csrfRec2.Result().Cookies() {
		req4.AddCookie(c)
	}
	req4.AddCookie(cookie)
	req4.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrfToken2})
	rec4 := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec4, req4)
	if rec4.Code != http.StatusSeeOther && rec4.Code != http.StatusOK {
		t.Fatalf("re-enable got %d body %s", rec4.Code, rec4.Body.String())
	}
	snap = h.runtime.Site.Current()
	if !snap.AnalyticsEnabled {
		t.Fatal("should be enabled after re-enable")
	}
	if snap.AnalyticsRetentionDays != 365 {
		t.Fatalf("retention should be 365, got %d", snap.AnalyticsRetentionDays)
	}
}

func TestAnalytics_RetentionValidation(t *testing.T) {
	h, svc, _ := newAnalyticsHandler(t)
	cookie := loginCookie(t, svc)
	csrfRec := httptest.NewRecorder()
	csrfReq := httptest.NewRequest(http.MethodGet, "/admin/analytics/settings", nil)
	csrfReq.AddCookie(cookie)
	csrfToken, _ := h.csrfToken(csrfRec, csrfReq)
	form := url.Values{
		"csrf_token":                      {csrfToken},
		"analytics_enabled":               {"1"},
		"analytics_retention_days":        {"9999"},
		"analytics_hourly_retention_days": {"9999"},
	}
	req2 := httptest.NewRequest(http.MethodPost, "/admin/analytics/settings", strings.NewReader(form.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range csrfRec.Result().Cookies() {
		req2.AddCookie(c)
	}
	req2.AddCookie(cookie)
	req2.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrfToken})
	rec2 := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec2, req2)
	ctx := context.Background()
	row, _ := h.queries.GetSiteSettings(ctx)
	if row.AnalyticsRetentionDays == 9999 {
		t.Fatalf("invalid retention should be rejected, got %d", row.AnalyticsRetentionDays)
	}
}

func TestAnalytics_ClearRequiresAuthAndCSRF(t *testing.T) {
	h, _, _ := newAnalyticsHandler(t)
	// No auth
	req := httptest.NewRequest(http.MethodPost, "/admin/analytics/clear", nil)
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code == http.StatusOK {
		t.Fatalf("clear without auth should not be 200")
	}
	// With auth but no CSRF
	h2, svc, _ := newAnalyticsHandler(t)
	cookie := loginCookie(t, svc)
	req = httptest.NewRequest(http.MethodPost, "/admin/analytics/clear", nil)
	req.AddCookie(cookie)
	rec = httptest.NewRecorder()
	h2.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusForbidden {
		t.Fatalf("clear without CSRF should be 403, got %d", rec.Code)
	}
}

func TestAnalytics_ClearDoesNotDeleteForms(t *testing.T) {
	h, svc, analyticsSvc := newAnalyticsHandler(t)
	ctx := context.Background()
	now := int64(1000)
	_ = h.queries.CreateForm(ctx, db.CreateFormParams{ID: "form-clear-test", Name: "Test", SchemaVersion: 1, DefinitionJson: `{}`, Active: 1, CreatedAt: now, UpdatedAt: now})
	analyticsSvc.Record(analytics.Observation{
		Time:       time.Now(),
		Resource:   analytics.Resource{Key: "entry/e1/revision/r1", Path: "/test", RouteType: "entry", EntryID: "e1", RevisionID: "r1"},
		IsPageview: true,
		Status:     200,
		Client:     analytics.ClientClass{Browser: "Chrome"},
	})
	_ = analyticsSvc.FlushSync(ctx)
	_, _ = h.database.ExecContext(ctx, `INSERT OR IGNORE INTO not_found_paths (path, hit_count, first_seen_at, last_seen_at) VALUES ('/missing-clear', 1, 1, 1)`)
	cookie := loginCookie(t, svc)
	csrfRec := httptest.NewRecorder()
	csrfReq := httptest.NewRequest(http.MethodGet, "/admin/analytics/settings", nil)
	csrfReq.AddCookie(cookie)
	csrfToken, _ := h.csrfToken(csrfRec, csrfReq)
	form := url.Values{"csrf_token": {csrfToken}}
	req2 := httptest.NewRequest(http.MethodPost, "/admin/analytics/clear", strings.NewReader(form.Encode()))
	req2.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for _, c := range csrfRec.Result().Cookies() {
		req2.AddCookie(c)
	}
	req2.AddCookie(cookie)
	req2.AddCookie(&http.Cookie{Name: csrfCookieName, Value: csrfToken})
	rec2 := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusSeeOther && rec2.Code != http.StatusOK {
		t.Fatalf("clear should succeed, got %d body %s", rec2.Code, rec2.Body.String())
	}
	var acnt int64
	_ = h.database.QueryRowContext(ctx, "SELECT COUNT(*) FROM analytics_page_daily").Scan(&acnt)
	if acnt != 0 {
		t.Fatalf("analytics should be 0 after clear, got %d", acnt)
	}
	var fcnt int64
	_ = h.database.QueryRowContext(ctx, "SELECT COUNT(*) FROM forms WHERE id='form-clear-test'").Scan(&fcnt)
	if fcnt == 0 {
		t.Fatal("form should not be deleted")
	}
	var ncnt int64
	_ = h.database.QueryRowContext(ctx, "SELECT COUNT(*) FROM not_found_paths WHERE path='/missing-clear'").Scan(&ncnt)
	if ncnt == 0 {
		t.Fatal("not_found should not be deleted")
	}
}

func TestAnalytics_HistoricalVisibleWhenDisabled(t *testing.T) {
	h, svc, analyticsSvc := newAnalyticsHandler(t)
	ctx := context.Background()
	analyticsSvc.Record(analytics.Observation{
		Time:       time.Now(),
		Resource:   analytics.Resource{Key: "entry/e1/revision/r1", Path: "/hist", RouteType: "entry", EntryID: "e1", RevisionID: "r1"},
		IsPageview: true,
		Status:     200,
		Client:     analytics.ClientClass{Browser: "Chrome"},
	})
	_ = analyticsSvc.FlushSync(ctx)
	_, _ = h.database.ExecContext(ctx, `UPDATE site_settings SET analytics_enabled=0 WHERE id=1`)
	_ = h.runtime.Site.Reload(ctx)
	var cnt int64
	_ = h.database.QueryRowContext(ctx, "SELECT COUNT(*) FROM analytics_page_daily").Scan(&cnt)
	if cnt == 0 {
		t.Fatalf("historical should still be in DB after disabled, cnt %d", cnt)
	}
	// Admin should still show data (overview should have HasData)
	cookie := loginCookie(t, svc)
	req := httptest.NewRequest(http.MethodGet, "/admin/analytics", nil)
	req.AddCookie(cookie)
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("should be 200, got %d", rec.Code)
	}
	if strings.Contains(rec.Body.String(), "No analytics yet") {
		t.Fatalf("should not show empty when historical exists")
	}
}

func extractCSRF(body string) string {
	// Look for name="csrf_token" value="
	idx := strings.Index(body, `name="csrf_token"`)
	if idx == -1 {
		idx = strings.Index(body, `name="csrf_token" `)
	}
	if idx == -1 {
		return ""
	}
	sub := body[idx:]
	idx2 := strings.Index(sub, `value="`)
	if idx2 == -1 {
		return ""
	}
	start := idx2 + len(`value="`)
	end := strings.Index(sub[start:], `"`)
	if end == -1 {
		return ""
	}
	return sub[start : start+end]
}
