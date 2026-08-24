package admin

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/kokosx/stratum/internal/auth"
	"github.com/kokosx/stratum/internal/blocks"
	"github.com/kokosx/stratum/internal/storage"
	db "github.com/kokosx/stratum/internal/storage/sqlc"
	"github.com/kokosx/stratum/internal/themes"
)

func TestAdminListStatusTabs(t *testing.T) {
	ctx := context.Background()
	database, err := storage.Open(filepath.Join(t.TempDir(), "stratum.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	if err := database.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	if err := database.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	queries := db.New(database.DB)
	registry, _ := blocks.NewRegistry(ctx, queries)
	themeRuntime, _ := themes.NewRuntime(ctx, queries)
	authService, _ := auth.NewService(database.DB, queries, false)
	h, _ := NewHandler(database.DB, queries, authService, registry, themeRuntime, newTestMedia(t, queries))
	now := int64(123)
	_ = queries.CreateEntry(ctx, db.CreateEntryParams{ID: "list1", ContentTypeID: "page", Slug: "list-one", Status: "active", CreatedAt: now, UpdatedAt: now})
	_ = queries.CreateEntry(ctx, db.CreateEntryParams{ID: "list2", ContentTypeID: "page", Slug: "list-two", Status: "trash", CreatedAt: now, UpdatedAt: now})

	req := httptest.NewRequest(http.MethodGet, "/admin/pages?status=trash", nil)
	rec := httptest.NewRecorder()
	h.listPages(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("list pages %d", rec.Code)
	}
	body := rec.Body.String()
	if !strings.Contains(body, "Trash") {
		t.Fatalf("missing status tabs %s", body)
	}
	req2 := httptest.NewRequest(http.MethodGet, "/admin/pages?search=list-one", nil)
	rec2 := httptest.NewRecorder()
	h.listPages(rec2, req2)
	if !strings.Contains(rec2.Body.String(), "list-one") {
		t.Fatalf("search failed %s", rec2.Body.String())
	}
}
