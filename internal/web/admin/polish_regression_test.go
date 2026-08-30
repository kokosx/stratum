package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/auth"
	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/media"
	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/themes"
)

func TestDashboard_EmptySiteRenders(t *testing.T) {
	h, _ := newPolishHandler(t)
	// Simulate authenticated user via auth service
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	// Create user session
	// Use handler's auth to login: create admin first?
	// newPolishHandler creates auth service with no admin yet; create admin via setup
	ctx := req.Context()
	hasAdmin, _ := h.auth.HasAdmin(ctx)
	if !hasAdmin {
		// Create admin via auth.Setup
		// Get setup code
		code := h.auth.SetupCode()
		_, err := h.auth.Setup(ctx, code, "Test Site", "admin@example.com", "password123456")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	// Login to get token
	token, err := h.auth.Login(ctx, "admin@example.com", "password123456")
	if err != nil {
		t.Fatalf("login: %v", err)
	}
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("dashboard status %d want 200 body %.500s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Quick actions") {
		t.Fatalf("dashboard missing Quick actions")
	}
	if !strings.Contains(body, "Recent content") {
		t.Fatalf("dashboard missing Recent content")
	}
	if !strings.Contains(body, "Content shortcuts") {
		t.Fatalf("dashboard missing Content shortcuts")
	}
	if !strings.Contains(body, "View site") {
		t.Fatalf("dashboard missing View site")
	}
}

func TestDashboard_ShowsRecentEntries(t *testing.T) {
	h, _ := newPolishHandler(t)
	ctx := h.authContext(t)
	// Create a page entry directly via queries to simulate recent content
	// Use helper to create entry via publishing? Simpler: insert entry directly
	queries := h.queries
	now := int64(1000)
	// Need content type page exists
	if err := queries.CreateEntry(ctx, db.CreateEntryParams{ID: "test-page-1", ContentTypeID: "page", Slug: "test-page", Status: "active", CreatedAt: now, UpdatedAt: now}); err != nil {
		t.Fatalf("create entry: %v", err)
	}
	// Create revision
	if err := queries.CreateEntryRevision(ctx, db.CreateEntryRevisionParams{ID: "rev1", EntryID: "test-page-1", RevisionNumber: 1, Slug: "test-page", Title: "Hello Page", DocumentJson: `{"version":1,"nodes":[]}`, CreatedAt: now, Visibility: "public", ReviewState: "draft"}); err != nil {
		t.Fatalf("create rev: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	token, _ := h.auth.Login(ctx, "admin@example.com", "password123456")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), "Hello Page") {
		t.Fatalf("dashboard should show recent entry Hello Page, got %.500s", rec.Body.String())
	}
}

func TestDashboard_NeedsAttention_FormSubmissions(t *testing.T) {
	h, _ := newPolishHandler(t)
	ctx := h.authContext(t)
	queries := h.queries
	now := int64(1000)
	// Create form and submission
	formID := "form-1"
	_ = queries.CreateForm(ctx, db.CreateFormParams{ID: formID, Name: "Contact", SchemaVersion: 1, DefinitionJson: `{"fields":[]}`, Active: 1, CreatedAt: now, UpdatedAt: now})
	_ = queries.CreateFormSubmission(ctx, db.CreateFormSubmissionParams{ID: "sub-1", FormID: formID, Status: "new", ValuesJson: `{}`, CreatedAt: now})
	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	token, _ := h.auth.Login(ctx, "admin@example.com", "password123456")
	req.AddCookie(&http.Cookie{Name: auth.CookieName, Value: token})
	rec := httptest.NewRecorder()
	h.Routes().ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), "new form submission") {
		t.Fatalf("dashboard should show new form submission attention, got %.500s", rec.Body.String())
	}
}

func readFileVariants(t *testing.T, rel string) []byte {
	t.Helper()
	candidates := []string{
		filepath.Join("..", "templates", "admin", rel),
		filepath.Join("..", "..", "web", "templates", "admin", rel),
		filepath.Join("internal", "web", "templates", "admin", rel),
		filepath.Join("..", "..", "..", "internal", "web", "templates", "admin", rel),
		rel,
	}
	for _, p := range candidates {
		if data, err := os.ReadFile(p); err == nil {
			return data
		}
	}
	// try absolute via wd
	if wd, err := os.Getwd(); err == nil {
		for i := 0; i < 5; i++ {
			p := filepath.Join(wd, "internal", "web", "templates", "admin", rel)
			if data, err := os.ReadFile(p); err == nil {
				return data
			}
			wd = filepath.Dir(wd)
		}
	}
	t.Fatalf("could not read %s", rel)
	return nil
}

func TestSiteHealth_NoInlineStyle(t *testing.T) {
	data := readFileVariants(t, "tools_health.html")
	if strings.Contains(string(data), `style="`) {
		t.Fatalf("tools_health.html contains inline style=\"\"")
	}
}

func TestSiteHealth_SummarySingleDot(t *testing.T) {
	data := readFileVariants(t, "tools_health.html")
	s := string(data)
	// Ensure not double dot like "● ● Good" by checking absence of that pattern
	if strings.Contains(s, "● ●") {
		t.Fatalf("health template contains double dot ● ●")
	}
	// Ensure health-summary exists and passed checks are inside details
	if !strings.Contains(s, "health-summary") {
		t.Fatalf("missing health-summary class")
	}
	if !strings.Contains(s, "<details") || !strings.Contains(s, "Passed checks") {
		t.Fatalf("passed checks should be inside details")
	}
}

func TestSiteHealth_NoBigGoodTable(t *testing.T) {
	data := readFileVariants(t, "tools_health.html")
	s := string(data)
	// Old template had table with STATUS / CHECK etc. New should not have that header for passed checks table?
	if strings.Contains(s, "<th>Status</th><th>Check</th><th>Details</th><th>Action</th>") {
		t.Fatalf("old table header still present")
	}
}

func TestForms_StatusNotPill(t *testing.T) {
	data := readFileVariants(t, "forms.html")
	s := string(data)
	if strings.Contains(s, `class="status-badge`) {
		t.Fatalf("forms.html still uses status-badge pill class")
	}
	if !strings.Contains(s, "status-indicator") {
		t.Fatalf("forms should use status-indicator")
	}
	// Check data still has Edit Submissions Export CSV
	if !strings.Contains(s, "Edit") || !strings.Contains(s, "Submissions") || !strings.Contains(s, "Export CSV") {
		t.Fatalf("forms row actions missing")
	}
}

func TestTabsAriaLabelledby(t *testing.T) {
	data := readFileVariantsTheme(t, "theme.js")
	if !strings.Contains(string(data), "aria-labelledby") {
		t.Fatalf("theme.js tabs missing aria-labelledby")
	}
}

func readFileVariantsTheme(t *testing.T, name string) []byte {
	t.Helper()
	candidates := []string{
		filepath.Join("..", "..", "themes", "default", "assets", name),
		filepath.Join("internal", "themes", "default", "assets", name),
		filepath.Join("themes", "default", "assets", name),
	}
	for _, p := range candidates {
		if data, err := os.ReadFile(p); err == nil {
			return data
		}
	}
	if wd, err := os.Getwd(); err == nil {
		for i := 0; i < 5; i++ {
			p := filepath.Join(wd, "internal", "themes", "default", "assets", name)
			if data, err := os.ReadFile(p); err == nil {
				return data
			}
			wd = filepath.Dir(wd)
		}
	}
	t.Fatalf("could not read %s", name)
	return nil
}

func TestCreatorUITypoFixed(t *testing.T) {
	data := readFileVariants(t, "creator.html")
	s := string(data)
	// Should contain Latest posts on homepage for blog, and not Services per row for blog
	// Simple check: the string "Services per row" should only appear for service, not for blog
	// After fix, blog should have Latest posts
	if !strings.Contains(s, "Latest posts on homepage") {
		t.Fatalf("creator.html missing Latest posts on homepage")
	}
	// Count occurrences: blog section previously had Services per row, now should be 1 (only local-business)
	count := strings.Count(s, "Services per row")
	if count != 1 {
		t.Fatalf("creator.html Services per row count = %d want 1 (only local-business)", count)
	}
}

// helpers

func newPolishHandler(t *testing.T) (*Handler, *auth.Service) {
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
	return h, service
}

func (h *Handler) authContext(t *testing.T) context.Context {
	t.Helper()
	ctx := context.Background()
	hasAdmin, _ := h.auth.HasAdmin(ctx)
	if !hasAdmin {
		code := h.auth.SetupCode()
		_, err := h.auth.Setup(ctx, code, "Test Site", "admin@example.com", "password123456")
		if err != nil {
			t.Fatalf("setup: %v", err)
		}
	}
	return ctx
}
