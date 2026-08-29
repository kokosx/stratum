package public

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/media"
	"github.com/kokosx/stratum/internal/preview"
	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/themes"
)

func newPreviewTestHandler(t *testing.T) (*Handler, *preview.Service, *storage.Database, func()) {
	t.Helper()
	ctx := context.Background()
	dir := t.TempDir()
	dbPath := filepath.Join(dir, "stratum.db")
	database, err := storage.Open(dbPath)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := database.Migrate(ctx); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	if err := database.Seed(ctx); err != nil {
		t.Logf("seed: %v", err)
	}
	queries := db.New(database.DB)
	_, _ = database.DB.ExecContext(ctx, `UPDATE site_settings SET site_url='https://example.com', indexing_enabled=1 WHERE id=1`)
	_, _ = database.DB.ExecContext(ctx, `INSERT OR IGNORE INTO content_types (id, display_name, plural_name, hierarchical, public, config_json, created_at, updated_at) VALUES ('page', 'Page', 'Pages', 0, 1, '{}', unixepoch(), unixepoch())`)
	_, _ = database.DB.ExecContext(ctx, `INSERT OR IGNORE INTO users (id, email, password_hash, role, created_at, updated_at) VALUES ('user1', 'test@example.com', 'hash', 'admin', unixepoch(), unixepoch())`)
	registry, err := blocks.NewRegistry(ctx, queries)
	if err != nil {
		t.Fatalf("registry: %v", err)
	}
	themeRuntime, err := themes.NewRuntime(ctx, queries)
	if err != nil {
		t.Fatalf("theme: %v", err)
	}
	mediaStore, _ := media.NewLocalStorage(filepath.Join(dir, "media"))
	mediaSvc := media.NewService(queries, mediaStore)
	_, _ = database.DB.ExecContext(ctx, `INSERT OR IGNORE INTO forms (id, name, schema_version, definition_json, active, created_at, updated_at) VALUES ('form1', 'Contact', 1, '{"fields":[{"id":"f1","key":"name","label":"Name","type":"text","required":true}],"submitLabel":"Send"}', 1, unixepoch(), unixepoch())`)
	handler, err := NewHandler(queries, registry, themeRuntime, mediaSvc)
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	svc := preview.NewService(database.DB, queries)
	handler.preview = svc
	cleanup := func() { database.Close() }
	return handler, svc, database, cleanup
}

func createEntryWithRevisions(t *testing.T, database *storage.Database, entryID, slug, publishedTitle, publishedDoc, draftTitle, draftDoc string) {
	t.Helper()
	ctx := context.Background()
	_, _ = database.DB.ExecContext(ctx, `DELETE FROM routes WHERE path=?`, "/"+slug)
	_, _ = database.DB.ExecContext(ctx, `DELETE FROM entries WHERE id=?`, entryID)
	_, _ = database.DB.ExecContext(ctx, `DELETE FROM entry_revisions WHERE entry_id=?`, entryID)
	_, err := database.DB.ExecContext(ctx, `INSERT INTO entries (id, content_type_id, slug, status, published_revision_id, created_at, updated_at) VALUES (?, 'page', ?, 'active', NULL, unixepoch(), unixepoch())`, entryID, slug)
	if err != nil {
		t.Fatalf("insert entry: %v", err)
	}
	_, err = database.DB.ExecContext(ctx, `INSERT INTO entry_revisions (id, entry_id, revision_number, title, document_json, slug, created_at) VALUES ('rev-`+entryID+`-pub', ?, 1, ?, ?, ?, unixepoch())`, entryID, publishedTitle, publishedDoc, slug)
	if err != nil {
		t.Fatalf("insert pub rev: %v", err)
	}
	_, err = database.DB.ExecContext(ctx, `UPDATE entries SET published_revision_id='rev-`+entryID+`-pub' WHERE id=?`, entryID)
	if err != nil {
		t.Fatalf("update entry pub: %v", err)
	}
	_, err = database.DB.ExecContext(ctx, `INSERT OR REPLACE INTO routes (id, path, entry_id, route_type, created_at, updated_at) VALUES (?, ?, ?, 'entry', unixepoch(), unixepoch())`, "route-"+entryID, "/"+slug, entryID)
	if err != nil {
		t.Fatalf("insert route: %v", err)
	}
	if draftTitle != "" {
		_, err = database.DB.ExecContext(ctx, `INSERT INTO entry_revisions (id, entry_id, revision_number, title, document_json, slug, created_at) VALUES ('rev-`+entryID+`-draft', ?, 2, ?, ?, ?, unixepoch())`, entryID, draftTitle, draftDoc, slug)
		if err != nil {
			t.Fatalf("insert draft: %v", err)
		}
	}
}

func TestPreviewDraftDiffersFromPublished(t *testing.T) {
	handler, svc, database, cleanup := newPreviewTestHandler(t)
	defer cleanup()
	ctx := context.Background()
	entryID := "test-page-1"
	createEntryWithRevisions(t, database, entryID, "test-page", "Published Title", `{"version":1,"nodes":[{"id":"n1","block":"core/text","version":1,"props":{"text":"Published content"}}]}`, "Draft Title", `{"version":1,"nodes":[{"id":"n1","block":"core/text","version":1,"props":{"text":"Draft content"}}]}`)
	if err := handler.Hub().Routes.Reload(ctx); err != nil {
		t.Fatalf("reload routes: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/test-page", nil)
	req.Host = "example.com"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("public page status %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Published content") {
		t.Fatalf("public should contain published content, got %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "Draft content") {
		t.Fatalf("public should not contain draft")
	}
	token, _, err := svc.Create(ctx, entryID, "rev-"+entryID+"-draft", "user1", 24*time.Hour)
	if err != nil {
		t.Fatalf("create preview: %v", err)
	}
	req2 := httptest.NewRequest(http.MethodGet, "/_stratum/preview/"+token, nil)
	req2.Host = "example.com"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusOK {
		t.Fatalf("preview status %d: %s", rec2.Code, rec2.Body.String())
	}
	body2 := rec2.Body.String()
	if !strings.Contains(body2, "Draft content") {
		t.Fatalf("preview should contain draft content, got %s", body2)
	}
	if strings.Contains(body2, "Published content") {
		t.Fatalf("preview should not contain published")
	}
	if got := rec2.Header().Get("X-Robots-Tag"); got != "noindex, nofollow, noarchive" {
		t.Fatalf("X-Robots-Tag = %q, want noindex", got)
	}
	if cc := rec2.Header().Get("Cache-Control"); cc != "private, no-store" {
		t.Fatalf("Cache-Control = %q, want private, no-store", cc)
	}
	req3 := httptest.NewRequest(http.MethodGet, "/_stratum/preview/"+token, nil)
	req3.Host = "example.com"
	rec3 := httptest.NewRecorder()
	handler.ServeHTTP(rec3, req3)
	if !strings.Contains(rec3.Body.String(), "Draft content") {
		t.Fatalf("second preview should still be draft")
	}
	req4 := httptest.NewRequest(http.MethodGet, "/test-page", nil)
	req4.Host = "example.com"
	rec4 := httptest.NewRecorder()
	handler.ServeHTTP(rec4, req4)
	if !strings.Contains(rec4.Body.String(), "Published content") {
		t.Fatalf("public after preview should still be published")
	}
}

func TestPreviewSpecificRevisionSemantics(t *testing.T) {
	handler, svc, database, cleanup := newPreviewTestHandler(t)
	defer cleanup()
	ctx := context.Background()
	entryID := "test-page-2"
	createEntryWithRevisions(t, database, entryID, "test-page-2", "Pub", `{"version":1,"nodes":[{"id":"n1","block":"core/text","version":1,"props":{"text":"Pub"}}]}`, "Draft1", `{"version":1,"nodes":[{"id":"n1","block":"core/text","version":1,"props":{"text":"Draft1"}}]}`)
	if err := handler.Hub().Routes.Reload(ctx); err != nil {
		t.Fatalf("reload: %v", err)
	}
	token, _, _ := svc.Create(ctx, entryID, "rev-"+entryID+"-draft", "user1", 24*time.Hour)
	// Create new draft revision after token
	_, _ = database.DB.ExecContext(ctx, `INSERT INTO entry_revisions (id, entry_id, revision_number, title, document_json, slug, created_at) VALUES ('rev-`+entryID+`-draft2', ?, 3, 'Draft2', '{"version":1,"nodes":[{"id":"n1","block":"core/text","version":1,"props":{"text":"Draft2"}}]}', 'test-page-2', unixepoch())`, entryID)
	req := httptest.NewRequest(http.MethodGet, "/_stratum/preview/"+token, nil)
	req.Host = "example.com"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if !strings.Contains(rec.Body.String(), "Draft1") {
		t.Fatalf("old token should still show Draft1, got %s", rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), "Draft2") {
		t.Fatalf("old token should not show Draft2")
	}
}

func TestPreviewExpiryAndRevocation(t *testing.T) {
	handler, svc, database, cleanup := newPreviewTestHandler(t)
	defer cleanup()
	ctx := context.Background()
	entryID := "test-exp"
	createEntryWithRevisions(t, database, entryID, "exp", "Exp", `{"version":1,"nodes":[]}`, "", "")
	// Need a draft revision for preview
	_, _ = database.DB.ExecContext(ctx, `INSERT OR IGNORE INTO entry_revisions (id, entry_id, revision_number, title, document_json, slug, created_at) VALUES ('rev-exp', ?, 2, 'ExpDraft', '{"version":1,"nodes":[]}', 'exp', unixepoch())`, entryID)
	// Ensure we have a revision to preview
	token, link, _ := svc.Create(ctx, entryID, "rev-exp", "user1", time.Hour)
	_, _ = database.DB.ExecContext(ctx, `UPDATE preview_links SET expires_at=? WHERE id=?`, time.Now().Add(-2*time.Hour).Unix(), link.ID)
	req := httptest.NewRequest(http.MethodGet, "/_stratum/preview/"+token, nil)
	req.Host = "example.com"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("expired should be 404, got %d: %s", rec.Code, rec.Body.String())
	}
	token2, link2, _ := svc.Create(ctx, entryID, "rev-exp", "user1", 24*time.Hour)
	if err := svc.Revoke(ctx, link2.ID); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	req2 := httptest.NewRequest(http.MethodGet, "/_stratum/preview/"+token2, nil)
	req2.Host = "example.com"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)
	if rec2.Code != http.StatusNotFound {
		t.Fatalf("revoked should be 404, got %d", rec2.Code)
	}
	req3 := httptest.NewRequest(http.MethodGet, "/_stratum/preview/wrongtoken123", nil)
	req3.Host = "example.com"
	rec3 := httptest.NewRecorder()
	handler.ServeHTTP(rec3, req3)
	if rec3.Code != http.StatusNotFound {
		t.Fatalf("wrong token should be 404, got %d", rec3.Code)
	}
	// Ensure panic not happen on nil DB case
	_ = database
}

func TestPreviewNoAdminCookieNeeded(t *testing.T) {
	handler, svc, database, cleanup := newPreviewTestHandler(t)
	defer cleanup()
	ctx := context.Background()
	entryID := "test-noauth"
	createEntryWithRevisions(t, database, entryID, "noauth", "NoAuth", `{"version":1,"nodes":[{"id":"n1","block":"core/text","version":1,"props":{"text":"Secret draft"}}]}`, "", "")
	// Create a draft revision specifically
	_, _ = database.DB.ExecContext(ctx, `INSERT OR IGNORE INTO entry_revisions (id, entry_id, revision_number, title, document_json, slug, created_at) VALUES ('rev-noauth', ?, 2, 'NoAuthDraft', '{"version":1,"nodes":[{"id":"n1","block":"core/text","version":1,"props":{"text":"Secret draft"}}]}', 'noauth', unixepoch())`, entryID)
	token, _, _ := svc.Create(ctx, entryID, "rev-noauth", "user1", 24*time.Hour)
	req := httptest.NewRequest(http.MethodGet, "/_stratum/preview/"+token, nil)
	req.Host = "example.com"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("preview without auth should be 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Secret draft") {
		t.Fatalf("preview content missing")
	}
}

func TestPreviewFormsInert(t *testing.T) {
	handler, svc, database, cleanup := newPreviewTestHandler(t)
	defer cleanup()
	ctx := context.Background()
	entryID := "test-form"
	createEntryWithRevisions(t, database, entryID, "form-page", "Form Page", `{"version":1,"nodes":[{"id":"n1","block":"core/form","version":1,"props":{},"settings":{"formId":"form1"}}]}`, "", "")
	_, _ = database.DB.ExecContext(ctx, `INSERT OR IGNORE INTO entry_revisions (id, entry_id, revision_number, title, document_json, slug, created_at) VALUES ('rev-form', ?, 2, 'Form Page Draft', '{"version":1,"nodes":[{"id":"n1","block":"core/form","version":1,"props":{},"settings":{"formId":"form1"}}]}', 'form-page', unixepoch())`, entryID)
	token, _, _ := svc.Create(ctx, entryID, "rev-form", "user1", 24*time.Hour)
	req := httptest.NewRequest(http.MethodGet, "/_stratum/preview/"+token, nil)
	req.Host = "example.com"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, "Preview — submissions disabled") && !strings.Contains(body, "stratum-form--preview") {
		t.Fatalf("preview form should be inert, got %s", body)
	}
	if strings.Contains(body, `action="/_stratum/forms/form1"`) && !strings.Contains(body, "disabled") {
		t.Fatalf("form should not have active submit in preview, got %s", body)
	}
}

func TestPreviewNoIndexHeaders(t *testing.T) {
	handler, svc, database, cleanup := newPreviewTestHandler(t)
	defer cleanup()
	ctx := context.Background()
	entryID := "test-noindex"
	createEntryWithRevisions(t, database, entryID, "noindex", "NoIndex", `{"version":1,"nodes":[]}`, "", "")
	_, _ = database.DB.ExecContext(ctx, `INSERT OR IGNORE INTO entry_revisions (id, entry_id, revision_number, title, document_json, slug, created_at) VALUES ('rev-noindex', ?, 2, 'NoIndexDraft', '{"version":1,"nodes":[]}', 'noindex', unixepoch())`, entryID)
	token, _, _ := svc.Create(ctx, entryID, "rev-noindex", "user1", 24*time.Hour)
	req := httptest.NewRequest(http.MethodGet, "/_stratum/preview/"+token, nil)
	req.Host = "example.com"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Header().Get("X-Robots-Tag") != "noindex, nofollow, noarchive" {
		t.Fatalf("X-Robots-Tag wrong: %q", rec.Header().Get("X-Robots-Tag"))
	}
	if rec.Header().Get("Cache-Control") != "private, no-store" {
		t.Fatalf("Cache-Control wrong: %q", rec.Header().Get("Cache-Control"))
	}
	body := rec.Body.String()
	if !strings.Contains(body, `name="robots"`) || !strings.Contains(body, "noindex") {
		t.Fatalf("robots meta missing or not noindex: %s", body)
	}
	if strings.Contains(body, "/_stratum/preview/") && strings.Contains(body, `rel="canonical"`) {
		if strings.Contains(body, token) {
			t.Fatalf("canonical should not contain preview token")
		}
	}
}

func TestPreviewSameRenderer(t *testing.T) {
	handler, svc, database, cleanup := newPreviewTestHandler(t)
	defer cleanup()
	ctx := context.Background()
	entryID := "test-renderer"
	createEntryWithRevisions(t, database, entryID, "renderer", "Renderer", `{"version":1,"nodes":[{"id":"n1","block":"core/text","version":1,"props":{"text":"Hello"}}]}`, "", "")
	_, _ = database.DB.ExecContext(ctx, `INSERT OR IGNORE INTO entry_revisions (id, entry_id, revision_number, title, document_json, slug, created_at) VALUES ('rev-rend', ?, 2, 'RendererDraft', '{"version":1,"nodes":[{"id":"n1","block":"core/text","version":1,"props":{"text":"Hello"}}]}', 'renderer', unixepoch())`, entryID)
	token, _, _ := svc.Create(ctx, entryID, "rev-rend", "user1", 24*time.Hour)
	req := httptest.NewRequest(http.MethodGet, "/_stratum/preview/"+token, nil)
	req.Host = "example.com"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	body := rec.Body.String()
	if !strings.Contains(body, "site-header") && !strings.Contains(body, "stratum") {
		t.Logf("body: %s", body)
	}
	if strings.Contains(body, "data-stratum-custom-code") {
		t.Fatalf("preview should not contain custom code JS")
	}
}

func TestPreviewEntryDeletion404(t *testing.T) {
	handler, svc, database, cleanup := newPreviewTestHandler(t)
	defer cleanup()
	ctx := context.Background()
	entryID := "test-del"
	createEntryWithRevisions(t, database, entryID, "del", "Del", `{"version":1,"nodes":[]}`, "", "")
	_, _ = database.DB.ExecContext(ctx, `INSERT OR IGNORE INTO entry_revisions (id, entry_id, revision_number, title, document_json, slug, created_at) VALUES ('rev-del', ?, 2, 'DelDraft', '{"version":1,"nodes":[]}', 'del', unixepoch())`, entryID)
	token, _, _ := svc.Create(ctx, entryID, "rev-del", "user1", 24*time.Hour)
	_, _ = database.DB.ExecContext(ctx, `DELETE FROM entries WHERE id=?`, entryID)
	req := httptest.NewRequest(http.MethodGet, "/_stratum/preview/"+token, nil)
	req.Host = "example.com"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("deleted entry preview should be 404, got %d", rec.Code)
	}
}
